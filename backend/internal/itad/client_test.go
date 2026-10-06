package itad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeITAD serves a lookup and a history response, and records the history
// request's query so tests can check what was asked for.
func fakeITAD(t *testing.T, historyQuery *string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/games/lookup/v1":
			w.Write([]byte(`{"found":true,"game":{"id":"abc-123"}}`))
		case "/games/history/v2":
			*historyQuery = r.URL.RawQuery
			w.Write([]byte(`[{"timestamp":"2026-10-01T00:00:00Z","deal":{"price":{"amount":4.99},"regular":{"amount":9.99},"cut":50}},
				{"timestamp":"2013-02-03T10:00:00Z","deal":{"price":{"amount":0},"regular":{"amount":9.99},"cut":100}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	old := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = old; srv.Close() })
}

func TestHistoryRequestsTheWholeLog(t *testing.T) {
	// ITAD returns about three months of events unless a "since" date is given.
	// Every caller must ask for everything, or histories are silently cut short.
	var query string
	fakeITAD(t, &query)
	c := New("key")

	t.Run("request-time history", func(t *testing.T) {
		events, err := c.History(context.Background(), 4000)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 || events[1].Timestamp.Year() != 2013 {
			t.Errorf("events = %+v", events)
		}
		if !strings.Contains(query, "since=2000-01-01") {
			t.Errorf("history query %q lacks since=, so ITAD would return only its default recent window", query)
		}
		if !strings.Contains(query, "country=US") || !strings.Contains(query, "shops=61") {
			t.Errorf("history query %q should be limited to Steam in USD", query)
		}
	})

	t.Run("backfill history", func(t *testing.T) {
		query = ""
		if _, err := c.GetHistory("abc-123"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(query, "since=2000-01-01") {
			t.Errorf("history query %q lacks since=", query)
		}
	})
}

func TestHistoryErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/games/lookup/v1" {
			w.Write([]byte(`{"found":false}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	old := baseURL
	baseURL = srv.URL
	defer func() { baseURL = old }()

	if _, err := New("k").History(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "no ITAD game") {
		t.Errorf("unknown game: err = %v", err)
	}
	if _, err := New("k").LookupGameID(1); err == nil {
		t.Error("a game ITAD does not know should be an error")
	}
}
