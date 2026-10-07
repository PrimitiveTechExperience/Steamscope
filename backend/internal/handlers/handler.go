package handlers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type Handler struct {
	DB          *database.DB
	Redis       *redis.Client
	Sessions    *auth.SessionManager
	Config      *config.Config
	SteamAPI    *steam.WebAPI
	Submissions *SubmissionQueue
	// Predictions caches the most recent price forecasts (at most five).
	Predictions *prediction.Store
	// ITAD extends price history for forecasts; nil when no API key is set.
	ITAD          *itad.Client
	predictFlight singleflight.Group
	httpClient    *http.Client
}

type Deps struct {
	DB          *database.DB
	Redis       *redis.Client
	Sessions    *auth.SessionManager
	Config      *config.Config
	Submissions *SubmissionQueue
	// SteamAPI overrides the Steam Web API client; tests use it to fake Steam.
	SteamAPI *steam.WebAPI
}

func New(d Deps) *Handler {
	var itadClient *itad.Client
	if d.Config.ITADAPIKey != "" {
		itadClient = itad.New(d.Config.ITADAPIKey)
	}
	steamAPI := d.SteamAPI
	if steamAPI == nil {
		steamAPI = steam.NewWebAPI(d.Config.Auth.SteamWebAPIKey)
	}
	return &Handler{
		DB:          d.DB,
		Redis:       d.Redis,
		Sessions:    d.Sessions,
		Config:      d.Config,
		SteamAPI:    steamAPI,
		Submissions: d.Submissions,
		Predictions: prediction.NewStore(d.Redis, prediction.DefaultMaxEntries, prediction.DefaultTTL),
		ITAD:        itadClient,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

type GameResponse struct {
	Games  []models.Game `json:"games"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

type ReviewResponse struct {
	Reviews []models.Review `json:"reviews"`
	Total   int             `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
}

const maxJSONBody = 64 << 10

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// serverError answers a request that failed on our side. The client gets a
// short message and the request ID (also in the X-Request-ID header), never
// internals. The cause goes to the log through the middleware, as one
// structured line carrying the same request ID, the operation, the error and,
// for database errors, the Postgres code, table and constraint.
func serverError(w http.ResponseWriter, r *http.Request, op string, err error, message string, extra ...any) {
	attrs := append([]any{}, extra...)
	if user := auth.CurrentUser(r.Context()); user != nil {
		attrs = append(attrs, "user_id", user.UserID)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		attrs = append(attrs, "pg_code", pgErr.Code, "pg_table", pgErr.TableName, "pg_constraint", pgErr.ConstraintName, "pg_detail", pgErr.Detail)
	}
	observability.RecordError(r.Context(), op, err, attrs...)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error":      message,
		"request_id": observability.RequestID(r.Context()),
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
