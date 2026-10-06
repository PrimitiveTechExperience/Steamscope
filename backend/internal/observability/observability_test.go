package observability

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func quietLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

func serve(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	logger, _ := quietLogger()
	rec := httptest.NewRecorder()
	Middleware(logger, false)(h).ServeHTTP(rec, req)
	return rec
}

func TestGeneratesRequestID(t *testing.T) {
	rec := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestID(r.Context()) == "" {
			t.Error("handler context has no request ID")
		}
	}), httptest.NewRequest("GET", "/x", nil))

	if id := rec.Header().Get(RequestIDHeader); !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(id) {
		t.Errorf("generated request ID = %q, want 16 hex characters", id)
	}
}

func TestEchoesWellFormedRequestID(t *testing.T) {
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set(RequestIDHeader, "trace-abc_123.DEF")
	rec := serve(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), req)

	if got := rec.Header().Get(RequestIDHeader); got != "trace-abc_123.DEF" {
		t.Errorf("request ID = %q, want the caller's ID echoed back", got)
	}
}

func TestReplacesMalformedRequestID(t *testing.T) {
	for _, bad := range []string{"short", "has spaces in it!!", strings.Repeat("a", 65), "new\nline-injection"} {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set(RequestIDHeader, bad)
		rec := serve(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), req)
		if got := rec.Header().Get(RequestIDHeader); got == bad || got == "" {
			t.Errorf("malformed ID %q was not replaced (got %q)", bad, got)
		}
	}
}

func TestRecoversFromPanic(t *testing.T) {
	logger, logs := quietLogger()
	rec := httptest.NewRecorder()
	h := Middleware(logger, false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("body = %q, want a JSON error", rec.Body.String())
	}
	if rec.Header().Get(RequestIDHeader) == "" {
		t.Error("panic response lacks a request ID")
	}
	if !strings.Contains(logs.String(), "panic recovered") || !strings.Contains(logs.String(), "boom") {
		t.Errorf("panic not logged: %s", logs.String())
	}
}

func TestAccessLogHasStructuredFields(t *testing.T) {
	logger, logs := quietLogger()
	h := Middleware(logger, true)(WithRoute("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/things/42", nil))

	line := logs.String()
	for _, want := range []string{`"request_id"`, `"route":"GET /things/{id}"`, `"path":"/things/42"`, `"status":418`, `"duration_ms"`} {
		if !strings.Contains(line, want) {
			t.Errorf("access log missing %s: %s", want, line)
		}
	}
}

func TestMetricsScrapesAreNotAccessLogged(t *testing.T) {
	logger, logs := quietLogger()
	h := Middleware(logger, true)(WithRoute("GET /metrics", func(w http.ResponseWriter, r *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
	if logs.Len() != 0 {
		t.Errorf("a Prometheus scrape was logged (it would flood the log every 15s): %s", logs.String())
	}
}

func TestMetricsLabelRoutePatternNotRawPath(t *testing.T) {
	h := Middleware(slog.New(slog.NewJSONHandler(io.Discard, nil)), false)(
		WithRoute("GET /widgets/{id}", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	for _, p := range []string{"/widgets/1", "/widgets/2", "/widgets/3"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}

	rec := httptest.NewRecorder()
	MetricsHandler("").ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()

	if !strings.Contains(body, `route="GET /widgets/{id}"`) {
		t.Errorf("metrics missing the route pattern label:\n%s", firstLines(body, "steamscope_http"))
	}
	if strings.Contains(body, `/widgets/1`) {
		t.Error("metrics leaked a raw path as a label (unbounded cardinality)")
	}
	for _, name := range []string{"steamscope_http_requests_total", "steamscope_http_request_duration_seconds_bucket", "steamscope_build_info"} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics output missing %s", name)
		}
	}
}

func TestUnmatchedRouteIsLabelledUnmatched(t *testing.T) {
	serve(t, http.NotFoundHandler(), httptest.NewRequest("GET", "/random/scanner/path", nil))
	rec := httptest.NewRecorder()
	MetricsHandler("").ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rec.Body.String(), `route="unmatched"`) {
		t.Error("requests to unknown paths should be grouped under route=\"unmatched\"")
	}
}

func TestMetricsTokenProtection(t *testing.T) {
	h := MetricsHandler("s3cret")
	cases := []struct {
		auth string
		want int
	}{
		{"", http.StatusUnauthorized},
		{"Bearer wrong", http.StatusUnauthorized},
		{"s3cret", http.StatusUnauthorized},
		{"Bearer s3cret", http.StatusOK},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/metrics", nil)
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("Authorization %q: status = %d, want %d", c.auth, rec.Code, c.want)
		}
	}
}

func TestBusinessMetricsAreExposed(t *testing.T) {
	LoginAttempts.WithLabelValues("success").Inc()
	Submissions.WithLabelValues("app", "awaiting_approval").Inc()
	ScrapeRuns.WithLabelValues("success").Inc()

	rec := httptest.NewRecorder()
	MetricsHandler("").ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	for _, name := range []string{
		`steamscope_login_attempts_total{outcome="success"}`,
		`steamscope_submissions_total{kind="app",outcome="awaiting_approval"}`,
		`steamscope_scrape_runs_total{result="success"}`,
	} {
		if !strings.Contains(rec.Body.String(), name) {
			t.Errorf("metrics output missing %s", name)
		}
	}
}

func firstLines(s, contains string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, contains) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestRequestsAreNotLoggedByDefault(t *testing.T) {
	// A busy page makes dozens of API calls; logging each one buries real problems.
	for _, status := range []int{200, 201, 204, 301, 400, 401, 403, 404, 429} {
		logger, logs := quietLogger()
		h := Middleware(logger, false)(WithRoute("GET /x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
		if logs.Len() != 0 {
			t.Errorf("a %d response was logged by default: %s", status, logs.String())
		}
	}
}

func TestServerErrorsAreAlwaysLogged(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		logger, logs := quietLogger()
		h := Middleware(logger, false)(WithRoute("GET /x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

		if !strings.Contains(logs.String(), fmt.Sprintf(`"status":%d`, status)) || !strings.Contains(logs.String(), rec.Header().Get(RequestIDHeader)) {
			t.Errorf("a %d response should be logged with its request ID: %q", status, logs.String())
		}
	}
}

func TestEveryRequestIsLoggedWhenOptedIn(t *testing.T) {
	logger, logs := quietLogger()
	h := Middleware(logger, true)(WithRoute("GET /x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if !strings.Contains(logs.String(), `"status":200`) {
		t.Errorf("ACCESS_LOG=true should log successful requests: %q", logs.String())
	}
}

func TestQuietModeStillSetsRequestIDsAndCountsMetrics(t *testing.T) {
	logger, logs := quietLogger()
	h := Middleware(logger, false)(WithRoute("GET /quiet-route", func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/quiet-route", nil))

	if rec.Header().Get(RequestIDHeader) == "" {
		t.Error("request IDs must not depend on logging")
	}
	metrics := httptest.NewRecorder()
	MetricsHandler("").ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `route="GET /quiet-route"`) {
		t.Error("metrics must be recorded even when requests are not logged")
	}
	if logs.Len() != 0 {
		t.Errorf("logged: %s", logs.String())
	}
}

func TestAbandonedRequestsAreNotServerErrors(t *testing.T) {
	// The browser cancels in-flight calls when you navigate away. The handler
	// then fails with "context canceled" and writes a 500 to nobody.
	logger, logs := quietLogger()
	ctx, cancel := context.WithCancel(context.Background())
	h := Middleware(logger, false)(WithRoute("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		cancel() // the client disconnects while the handler is working
		w.WriteHeader(http.StatusInternalServerError)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/slow", nil).WithContext(ctx))

	if logs.Len() != 0 {
		t.Errorf("an abandoned request was logged as a server error: %s", logs.String())
	}
	metrics := httptest.NewRecorder()
	MetricsHandler("").ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `route="GET /slow",status="499"`) {
		t.Errorf("want the request counted as 499:\n%s", firstLines(metrics.Body.String(), "/slow"))
	}
}

func TestRealServerErrorsStillLogWhenTheClientIsStillThere(t *testing.T) {
	logger, logs := quietLogger()
	h := Middleware(logger, false)(WithRoute("GET /broken", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/broken", nil))
	if !strings.Contains(logs.String(), `"status":500`) {
		t.Errorf("a genuine 500 must be logged: %q", logs.String())
	}
}

func TestATimeoutIsStillAServerError(t *testing.T) {
	// Only a client hanging up is excused; our own deadline expiring is a fault.
	logger, logs := quietLogger()
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	h := Middleware(logger, false)(WithRoute("GET /timeout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/timeout", nil).WithContext(ctx))
	if !strings.Contains(logs.String(), `"status":504`) {
		t.Errorf("a deadline expiry should still be logged as an error: %q", logs.String())
	}
}
