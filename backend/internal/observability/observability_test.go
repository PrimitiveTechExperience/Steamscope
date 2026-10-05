package observability

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func quietLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

func serve(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	logger, _ := quietLogger()
	rec := httptest.NewRecorder()
	Middleware(logger)(h).ServeHTTP(rec, req)
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
	h := Middleware(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
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
	h := Middleware(logger)(WithRoute("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
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
	h := Middleware(logger)(WithRoute("GET /metrics", func(w http.ResponseWriter, r *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
	if logs.Len() != 0 {
		t.Errorf("a Prometheus scrape was logged (it would flood the log every 15s): %s", logs.String())
	}
}

func TestMetricsLabelRoutePatternNotRawPath(t *testing.T) {
	h := Middleware(slog.New(slog.NewJSONHandler(io.Discard, nil)))(
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
