// Package testutil builds a complete in-process API (real router, real
// handlers, real Postgres schema, in-memory Redis) for integration tests.
//
// Postgres comes from TEST_DATABASE_URL. Each test gets its own schema,
// created from backend/postgres/setup.sql and dropped afterwards, so tests are
// isolated from each other and never touch real tables. When the variable is
// unset, integration tests are skipped.
package testutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/handlers"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/router"
	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	FrontendURL = "http://localhost:4200"
	BackendURL  = "http://localhost:8080"
)

// App is a running API under test.
type App struct {
	T       *testing.T
	DB      *database.DB
	Pool    *pgxpool.Pool
	Redis   *redis.Client
	Mini    *miniredis.Miniredis
	Handler http.Handler
	Queue   *handlers.SubmissionQueue
	nextIP  int
}

// NewApp starts an isolated API. It skips the test when no database is
// configured.
func NewApp(t *testing.T) *App {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	schema := "t_" + randomHex(6)
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close(ctx)
		t.Fatalf("create schema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	cfg.MaxConns = 3
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+schema)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(context.Background())
	})

	if _, err := pool.Exec(ctx, schemaSQL(t)); err != nil {
		t.Fatalf("apply setup.sql: %v", err)
	}

	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { rdb.Close() })

	hashKey := base64.StdEncoding.EncodeToString(randomBytes(64))
	blockKey := base64.StdEncoding.EncodeToString(randomBytes(32))
	sessions, err := auth.NewSessionManager(rdb, hashKey, blockKey, false)
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}

	db := &database.DB{Pool: pool}
	queue := handlers.NewSubmissionQueue(100) // never run: tests inspect state instead of scraping
	h := handlers.New(handlers.Deps{
		DB: db, Redis: rdb, Sessions: sessions, Submissions: queue,
		Config: &config.Config{FrontendURL: FrontendURL, BackendURL: BackendURL, MetricsToken: "test-token"},
	})
	return &App{T: t, DB: db, Pool: pool, Redis: rdb, Mini: mini, Queue: queue, Handler: router.New(h, FrontendURL)}
}

// schemaSQL returns setup.sql without its trailing row-level-security block,
// which loops over the real "public" schema and must not run in a test.
func schemaSQL(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the test directory")
		}
		dir = parent
	}
	raw, err := os.ReadFile(filepath.Join(dir, "backend", "postgres", "setup.sql"))
	if err != nil {
		t.Fatalf("read setup.sql: %v", err)
	}
	sql := string(raw)
	if i := strings.Index(sql, "-- Row level security"); i >= 0 {
		sql = sql[:i]
	}
	return sql
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func randomHex(n int) string { return hex.EncodeToString(randomBytes(n)) }

// ---- seeding ----

// SeedGame inserts a minimal tracked game.
func (a *App) SeedGame(appID int, name string, developers, publishers []string) {
	a.T.Helper()
	game := models.Game{
		AppID: appID, Name: name, URL: fmt.Sprintf("https://store.steampowered.com/app/%d", appID),
		Description: "desc", DescriptionHTML: "<p>desc</p>", HeaderImage: "https://example.com/h.jpg",
		ReleaseDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Price: 9.99, OriginalPrice: 19.99, DiscountPercentage: 50,
		SupportedLanguages: []string{"English"}, Developers: developers, Publishers: publishers,
		Genres: []string{"Action"}, Tags: []string{"Co-op"}, ReviewScore: "Very Positive",
	}
	if err := a.DB.InsertGame(context.Background(), game); err != nil {
		a.T.Fatalf("seed game: %v", err)
	}
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO tracked_games (app_id, status) VALUES ($1, 'tracked') ON CONFLICT (app_id) DO UPDATE SET status = 'tracked'`, appID); err != nil {
		a.T.Fatalf("seed tracked game: %v", err)
	}
}

// SeedPriceHistory writes one price_history row per point for a game.
func (a *App) SeedPriceHistory(appID int, points []models.PricePoint) {
	a.T.Helper()
	if err := a.DB.UpsertPriceHistoryBatch(context.Background(), appID, points); err != nil {
		a.T.Fatalf("seed price history: %v", err)
	}
}

// SeedBundle stores a tracked bundle containing the given games.
func (a *App) SeedBundle(id int, name string, games ...models.BundleGame) {
	a.T.Helper()
	b := models.Bundle{
		BundleID: id, Name: name, URL: fmt.Sprintf("https://store.steampowered.com/bundle/%d", id),
		Price: 20, OriginalPrice: 40, DiscountPercentage: 50, Games: games,
	}
	if err := a.DB.UpsertBundle(context.Background(), b, time.Now()); err != nil {
		a.T.Fatalf("seed bundle: %v", err)
	}
}

// SetFlag sets a boolean users column (is_admin, is_banned, ...).
func (a *App) SetFlag(userID int64, column string, value bool) {
	a.T.Helper()
	if _, err := a.Pool.Exec(context.Background(),
		fmt.Sprintf("UPDATE users SET %s = $2 WHERE user_id = $1", column), userID, value); err != nil {
		a.T.Fatalf("set %s: %v", column, err)
	}
}

// ---- HTTP client ----

// Client is a browser-like client: it keeps cookies, sends the frontend's
// Origin on every request, and has its own remote IP (so per-IP rate limits
// don't leak between clients).
type Client struct {
	app     *App
	cookies map[string]*http.Cookie
	IP      string
	// NoOrigin omits the Origin header (to test CSRF protection).
	NoOrigin bool
	Origin   string
	UserID   int64
	// Headers are extra request headers sent on every request.
	Headers map[string]string
}

func (a *App) NewClient() *Client {
	a.nextIP++
	return &Client{app: a, cookies: map[string]*http.Cookie{}, IP: fmt.Sprintf("10.0.%d.%d:4000", a.nextIP/250, a.nextIP%250+1), Origin: FrontendURL}
}

// Response is a recorded reply.
type Response struct {
	*httptest.ResponseRecorder
}

func (r Response) Code() int { return r.ResponseRecorder.Code }

// JSON decodes the body into a generic map.
func (r Response) JSON() map[string]any {
	var m map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &m); err != nil {
		panic(fmt.Sprintf("response is not a JSON object: %q (%v)", r.Body.String(), err))
	}
	return m
}

// JSONArray decodes the body into a JSON array.
func (r Response) JSONArray() []any {
	var a []any
	if err := json.Unmarshal(r.Body.Bytes(), &a); err != nil {
		panic(fmt.Sprintf("response is not a JSON array: %q (%v)", r.Body.String(), err))
	}
	return a
}

// Error returns the "error" message of a JSON error response.
func (r Response) Error() string {
	s, _ := r.JSON()["error"].(string)
	return s
}

// Do sends a request. body may be nil, a string (sent verbatim) or any value
// (sent as JSON).
func (c *Client) Do(method, path string, body any) Response {
	c.app.T.Helper()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			c.app.T.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = c.IP
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !c.NoOrigin {
		req.Header.Set("Origin", c.Origin)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.app.Handler.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 || ck.Value == "" {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck
		}
	}
	return Response{rec}
}

func (c *Client) Get(path string) Response            { return c.Do(http.MethodGet, path, nil) }
func (c *Client) Post(path string, body any) Response { return c.Do(http.MethodPost, path, body) }
func (c *Client) Put(path string, body any) Response  { return c.Do(http.MethodPut, path, body) }
func (c *Client) Delete(path string) Response         { return c.Do(http.MethodDelete, path, nil) }

// Cookie returns the named cookie the client currently holds.
func (c *Client) Cookie(name string) *http.Cookie { return c.cookies[name] }

// SetCookie overrides a cookie value (to test tampered sessions).
func (c *Client) SetCookie(name, value string) {
	c.cookies[name] = &http.Cookie{Name: name, Value: value}
}

// Register creates an account through the API and leaves the client logged in.
func (c *Client) Register(username, email, password string) Response {
	c.app.T.Helper()
	resp := c.Post("/api/auth/register", map[string]string{"username": username, "email": email, "password": password})
	if resp.Code() == http.StatusCreated {
		if u, ok := resp.JSON()["user"].(map[string]any); ok {
			c.UserID = int64(u["user_id"].(float64))
		}
	}
	return resp
}

// NewUser registers a fresh logged-in user with a unique name.
func (a *App) NewUser(prefix string) *Client {
	a.T.Helper()
	c := a.NewClient()
	name := prefix + randomHex(3)
	if resp := c.Register(name, name+"@example.com", "correct-horse-battery"); resp.Code() != http.StatusCreated {
		a.T.Fatalf("register %s: %d %s", name, resp.Code(), resp.Body.String())
	}
	return c
}

// NewAdmin registers a user and promotes them to admin. Admin status is read
// per request, so the existing session is enough.
func (a *App) NewAdmin() *Client {
	a.T.Helper()
	c := a.NewUser("admin")
	a.SetFlag(c.UserID, "is_admin", true)
	return c
}
