package router

import (
	"net/http"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/handlers"
)

func New(h *handlers.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/games", h.GetGames)
	mux.HandleFunc("GET /api/games/{appID}", h.GetGame)
	mux.HandleFunc("GET /api/games/{appID}/reviews", h.GetReviews)
	mux.HandleFunc("GET /api/health", getHealthHandler())
	return mux
}

func getHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}
}
