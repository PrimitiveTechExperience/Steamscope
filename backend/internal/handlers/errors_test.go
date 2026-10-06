package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestServerErrorHidesTheCauseFromTheClientButLogsIt(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	cause := fmt.Errorf("failed to get bundle: %w", &pgconn.PgError{
		Code: "42703", Message: `column "foo" does not exist`, TableName: "bundles", ConstraintName: "bundles_pkey",
	})
	h := observability.Middleware(logger, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverError(w, r, "get bundle", cause, "failed to load bundle", "bundle_id", 233)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/bundles/233", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	id := rec.Header().Get(observability.RequestIDHeader)
	if body["error"] != "failed to load bundle" || body["request_id"] != id || id == "" {
		t.Errorf("body = %v, header id = %q", body, id)
	}
	if strings.Contains(rec.Body.String(), "foo") || strings.Contains(rec.Body.String(), "42703") {
		t.Errorf("response leaks internals: %s", rec.Body.String())
	}

	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("expected one structured log line: %v\n%s", err, logs.String())
	}
	if line["request_id"] != id || line["op"] != "get bundle" || line["bundle_id"] != float64(233) {
		t.Errorf("line = %v", line)
	}
	if line["pg_code"] != "42703" || line["pg_table"] != "bundles" || line["pg_constraint"] != "bundles_pkey" {
		t.Errorf("database details missing from the log: %v", line)
	}
	if !strings.Contains(fmt.Sprint(line["error"]), `column "foo" does not exist`) {
		t.Errorf("error = %v", line["error"])
	}
}

func TestServerErrorWorksWithAPlainError(t *testing.T) {
	rec := httptest.NewRecorder()
	serverError(rec, httptest.NewRequest("GET", "/x", nil), "op", errors.New("boom"), "failed")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"error":"failed"`) {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
}
