package router

import (
	"net/http"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/handlers"
)

func New(h *handlers.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/games", h.GetGames)
	mux.HandleFunc("GET /api/games/{appID}", h.GetGame)
	mux.HandleFunc("GET /api/games/{appID}/reviews", h.GetReviews)
	mux.HandleFunc("GET /api/games/{appID}/price-history", h.GetPriceHistory)
	mux.HandleFunc("GET /api/filters", h.GetFilterOptions)
	mux.HandleFunc("GET /api/health", getHealthHandler())
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:4200")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}
}
