package router

import (
	"net/http"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/handlers"
)

func New(h *handlers.Handler, frontendURL string) http.Handler {
	mux := http.NewServeMux()

	// Public game data.
	mux.HandleFunc("GET /api/games", h.GetGames)
	mux.HandleFunc("GET /api/games/{appID}", h.GetGame)
	mux.HandleFunc("GET /api/games/{appID}/reviews", h.GetReviews)
	mux.HandleFunc("GET /api/games/{appID}/price-history", h.GetPriceHistory)
	mux.HandleFunc("GET /api/filters", h.GetFilterOptions)
	mux.HandleFunc("GET /api/bundles", h.GetBundles)
	mux.HandleFunc("GET /api/bundles/{bundleID}", h.GetBundle)
	mux.HandleFunc("GET /api/health", getHealthHandler())

	// Auth.
	mux.HandleFunc("POST /api/auth/register", h.Register)
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.HandleFunc("POST /api/auth/logout", h.Logout)
	mux.HandleFunc("GET /api/auth/me", h.Me)
	mux.HandleFunc("GET /api/auth/steam/login", h.SteamLogin)
	mux.HandleFunc("GET /api/auth/steam/link", h.SteamLink)
	mux.HandleFunc("GET /api/auth/steam/callback", h.SteamCallback)

	// Logged-in user.
	mux.HandleFunc("GET /api/me/preferences", auth.RequireAuth(h.GetPreferences))
	mux.HandleFunc("PUT /api/me/preferences", auth.RequireAuth(h.UpdatePreferences))
	mux.HandleFunc("GET /api/me/watchlist", auth.RequireAuth(h.GetWatchlist))
	mux.HandleFunc("PUT /api/me/watchlist/{appID}", auth.RequireAuth(h.WatchGame))
	mux.HandleFunc("DELETE /api/me/watchlist/{appID}", auth.RequireAuth(h.UnwatchGame))
	mux.HandleFunc("GET /api/me/notifications", auth.RequireAuth(h.GetNotifications))
	mux.HandleFunc("POST /api/me/notifications/read", auth.RequireAuth(h.MarkNotificationsRead))
	mux.HandleFunc("GET /api/me/recent-searches", auth.RequireAuth(h.GetRecentSearches))
	mux.HandleFunc("POST /api/me/recent-searches", auth.RequireAuth(h.AddRecentSearch))
	mux.HandleFunc("GET /api/me/steam-profile", auth.RequireAuth(h.GetSteamProfile))
	mux.HandleFunc("DELETE /api/me/steam", auth.RequireAuth(h.UnlinkSteam))
	mux.HandleFunc("GET /api/me/feed", auth.RequireAuth(h.GetFeed))
	mux.HandleFunc("GET /api/me/submissions", auth.RequireAuth(h.GetSubmissions))
	mux.HandleFunc("POST /api/submissions", auth.RequireAuth(h.SubmitGame))

	// Admin only.
	mux.HandleFunc("GET /api/admin/users", auth.RequireAdmin(h.AdminListUsers))
	mux.HandleFunc("DELETE /api/admin/users/{userID}", auth.RequireAdmin(h.AdminDeleteUser))
	mux.HandleFunc("GET /api/admin/items", auth.RequireAdmin(h.AdminListItems))
	mux.HandleFunc("DELETE /api/admin/items/{kind}/{id}", auth.RequireAdmin(h.AdminDeleteItem))
	mux.HandleFunc("POST /api/admin/items/{kind}/{id}/approve", auth.RequireAdmin(h.AdminApproveItem))
	mux.HandleFunc("POST /api/admin/items/{kind}/{id}/reject", auth.RequireAdmin(h.AdminRejectItem))

	var handler http.Handler = mux
	handler = h.Sessions.Middleware(h.DB.GetUserByID)(handler)
	handler = withOriginCheck(handler, frontendURL)
	handler = withCORS(handler, frontendURL)
	return handler
}

func withCORS(next http.Handler, frontendURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", frontendURL)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Add("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withOriginCheck is CSRF protection on top of SameSite=Lax: any
// state-changing request must come from the frontend's origin. Browsers
// always send Origin on cross-origin (and same-origin non-GET) requests.
func withOriginCheck(next http.Handler, frontendURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("Origin") != frontendURL {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"cross-origin request blocked"}`))
				return
			}
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
