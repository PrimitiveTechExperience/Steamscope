// Package observability provides request IDs, structured access logs,
// Prometheus metrics and panic recovery for the HTTP API.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Version is the build version, set at build time with
// -ldflags "-X .../observability.Version=1.2.3".
var Version = "dev"

var startedAt = time.Now()

// Uptime is how long this process has been running.
func Uptime() time.Duration { return time.Since(startedAt) }

const RequestIDHeader = "X-Request-ID"

type ctxKey int

const (
	requestIDKey ctxKey = iota
	routeKey
)

// A client-supplied request ID is only trusted if it looks like an ID, so a
// caller can't inject newlines or huge strings into our logs.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// RequestID returns the ID of the request in ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Registry holds every metric the API exposes.
var Registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "steamscope_http_requests_total",
		Help: "HTTP requests handled, by method, route pattern and status code.",
	}, []string{"method", "route", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "steamscope_http_request_duration_seconds",
		Help:    "HTTP request latency, by method and route pattern.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})

	httpInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "steamscope_http_requests_in_flight",
		Help: "HTTP requests currently being served.",
	})

	httpPanics = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "steamscope_http_panics_total",
		Help: "Handler panics recovered and turned into a 500.",
	})

	// LoginAttempts counts logins by outcome: success, bad_credentials,
	// banned or rate_limited.
	LoginAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "steamscope_login_attempts_total",
		Help: "Login attempts, by outcome.",
	}, []string{"outcome"})

	// Submissions counts game/bundle suggestions by kind and outcome.
	Submissions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "steamscope_submissions_total",
		Help: "Game and bundle submissions, by kind (app/bundle) and outcome.",
	}, []string{"kind", "outcome"})

	// ScrapeRuns counts full scrape runs by result (success/error).
	ScrapeRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "steamscope_scrape_runs_total",
		Help: "Full scrape runs, by result.",
	}, []string{"result"})

	ScrapeDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "steamscope_scrape_last_duration_seconds",
		Help: "Duration of the most recent scrape run.",
	})

	ScrapeGames = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "steamscope_scrape_last_games",
		Help: "Games included in the most recent scrape run.",
	})

	ScrapeLastSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "steamscope_scrape_last_success_timestamp_seconds",
		Help: "Unix time of the last successful scrape run.",
	})

	// PredictionRequests counts forecast lookups by result: hit (served from the
	// cache), miss (computed and cached) or insufficient (too little history).
	PredictionRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "steamscope_prediction_requests_total",
		Help: "Price forecast lookups, by result.",
	}, []string{"result"})

	PredictionCompute = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "steamscope_prediction_compute_seconds",
		Help:    "Time to compute a price forecast (including any ITAD calls).",
		Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 20},
	})

	buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "steamscope_build_info",
		Help: "Build information; always 1.",
	}, []string{"version"})
)

func init() {
	Registry.MustRegister(
		httpRequests, httpDuration, httpInFlight, httpPanics,
		LoginAttempts, Submissions, PredictionRequests, PredictionCompute, ScrapeRuns, ScrapeDuration, ScrapeGames, ScrapeLastSuccess, buildInfo,
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	buildInfo.WithLabelValues(Version).Set(1)
}

// MetricsHandler serves the Prometheus exposition format. If token is not
// empty the caller must send "Authorization: Bearer <token>".
func MetricsHandler(token string) http.Handler {
	inner := promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
	if token == "" {
		return inner
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// routeHolder lets a route handler report which pattern matched back to the
// outer middleware (the mux sets Request.Pattern on a copy of the request
// when middleware in between has replaced it).
type routeHolder struct{ pattern string }

// WithRoute wraps h so the outer Middleware can label metrics with the
// route pattern instead of the raw path (which would explode cardinality).
func WithRoute(pattern string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if holder, ok := r.Context().Value(routeKey).(*routeHolder); ok {
			holder.pattern = pattern
		}
		h(w, r)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Middleware assigns each request an ID (honouring a well-formed incoming
// X-Request-ID), recovers panics into a JSON 500, records metrics and writes
// one structured access-log line per request.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			id := r.Header.Get(RequestIDHeader)
			if !validRequestID.MatchString(id) {
				id = newRequestID()
			}
			w.Header().Set(RequestIDHeader, id)

			holder := &routeHolder{}
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			ctx = context.WithValue(ctx, routeKey, holder)
			r = r.WithContext(ctx)

			sw := &statusWriter{ResponseWriter: w}
			httpInFlight.Inc()
			defer func() {
				httpInFlight.Dec()
				if rec := recover(); rec != nil {
					httpPanics.Inc()
					logger.Error("panic recovered", "request_id", id, "panic", rec, "stack", string(debug.Stack()))
					if sw.status == 0 {
						sw.Header().Set("Content-Type", "application/json")
						sw.WriteHeader(http.StatusInternalServerError)
						sw.Write([]byte(`{"error":"internal server error"}`))
					}
				}
				route := holder.pattern
				if route == "" {
					route = "unmatched"
				}
				status := sw.status
				if status == 0 {
					status = http.StatusOK
				}
				elapsed := time.Since(start)
				httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
				httpDuration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())
				if route != "GET /metrics" {
					logger.Info("request",
						"request_id", id,
						"method", r.Method,
						"route", route,
						"path", r.URL.Path,
						"status", status,
						"duration_ms", float64(elapsed.Microseconds())/1000,
						"bytes", sw.bytes,
						"remote", r.RemoteAddr,
					)
				}
			}()
			next.ServeHTTP(sw, r)
		})
	}
}
