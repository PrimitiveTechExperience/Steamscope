package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/itad"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/prediction"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
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
}

func New(d Deps) *Handler {
	var itadClient *itad.Client
	if d.Config.ITADAPIKey != "" {
		itadClient = itad.New(d.Config.ITADAPIKey)
	}
	return &Handler{
		DB:          d.DB,
		Redis:       d.Redis,
		Sessions:    d.Sessions,
		Config:      d.Config,
		SteamAPI:    steam.NewWebAPI(d.Config.Auth.SteamWebAPIKey),
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
