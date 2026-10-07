package router

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/handlers"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
)

func New(h *handlers.Handler, frontendURL string) http.Handler {
	mux := http.NewServeMux()
	// handle registers a route and records its pattern for metrics labels.
	handle := func(pattern string, fn http.HandlerFunc) {
		mux.HandleFunc(pattern, observability.WithRoute(pattern, fn))
	}

	// Public game data.
	handle("GET /api/games", h.GetGames)
	handle("GET /api/games/{appID}", h.GetGame)
	handle("GET /api/games/{appID}/reviews", h.GetReviews)
	handle("GET /api/games/{appID}/price-history", h.GetPriceHistory)
	handle("GET /api/games/{appID}/prediction", h.GetPrediction)
	handle("GET /api/games/{appID}/advice", h.GetAdvice)
	handle("GET /api/filters", h.GetFilterOptions)
	handle("GET /api/bundles", h.GetBundles)
	handle("GET /api/bundles/{bundleID}", h.GetBundle)
	handle("GET /api/bundles/{bundleID}/prediction", h.GetBundlePrediction)
	handle("GET /api/bundles/{bundleID}/advice", h.GetBundleAdvice)
	handle("GET /api/health", h.Health)
	handle("GET /api/ready", h.Ready)
	handle("GET /metrics", observability.MetricsHandler(h.Config.MetricsToken).ServeHTTP)

	// Auth.
	handle("POST /api/auth/register", h.Register)
	handle("POST /api/auth/login", h.Login)
	handle("POST /api/auth/logout", h.Logout)
	handle("GET /api/auth/me", h.Me)
	handle("GET /api/auth/steam/login", h.SteamLogin)
	handle("GET /api/auth/steam/link", h.SteamLink)
	handle("GET /api/auth/steam/callback", h.SteamCallback)

	// Logged-in user.
	handle("GET /api/me/preferences", auth.RequireAuth(h.GetPreferences))
	handle("PUT /api/me/preferences", auth.RequireAuth(h.UpdatePreferences))
	handle("GET /api/me/watchlist", auth.RequireAuth(h.GetWatchlist))
	handle("PUT /api/me/watchlist/{appID}", auth.RequireAuth(h.WatchGame))
	handle("DELETE /api/me/watchlist/{appID}", auth.RequireAuth(h.UnwatchGame))
	handle("GET /api/me/notifications", auth.RequireAuth(h.GetNotifications))
	handle("POST /api/me/notifications/read", auth.RequireAuth(h.MarkNotificationsRead))
	handle("GET /api/me/recent-searches", auth.RequireAuth(h.GetRecentSearches))
	handle("POST /api/me/recent-searches", auth.RequireAuth(h.AddRecentSearch))
	handle("GET /api/me/steam-profile", auth.RequireAuth(h.GetSteamProfile))
	handle("DELETE /api/me/steam", auth.RequireAuth(h.UnlinkSteam))
	handle("GET /api/me/wishlist/status", auth.RequireAuth(h.GetWishlistStatus))
	handle("POST /api/me/wishlist/import", auth.RequireAuth(h.ImportWishlist))
	handle("POST /api/me/wishlist/request", auth.RequireAuth(h.RequestWishlistGames))
	handle("GET /api/me/feed", auth.RequireAuth(h.GetFeed))
	handle("GET /api/me/submissions", auth.RequireAuth(h.GetSubmissions))
	handle("POST /api/submissions", auth.RequireAuth(h.SubmitGame))

	// Admin only.
	handle("GET /api/admin/users", auth.RequireAdmin(h.AdminListUsers))
	handle("DELETE /api/admin/users/{userID}", auth.RequireAdmin(h.AdminDeleteUser))
	handle("PUT /api/admin/users/{userID}/moderation", auth.RequireAdmin(h.AdminModerateUser))
	handle("GET /api/admin/stats", auth.RequireAdmin(h.AdminStats))
	handle("GET /api/admin/blacklist", auth.RequireAdmin(h.AdminListBlacklist))
	handle("POST /api/admin/blacklist", auth.RequireAdmin(h.AdminAddBlacklistRule))
	handle("DELETE /api/admin/blacklist/{ruleID}", auth.RequireAdmin(h.AdminDeleteBlacklistRule))
	handle("GET /api/admin/blacklist/{ruleID}/matches", auth.RequireAdmin(h.AdminBlacklistMatches))
	handle("POST /api/admin/blacklist/{ruleID}/purge", auth.RequireAdmin(h.AdminPurgeBlacklistMatches))
	handle("GET /api/admin/items", auth.RequireAdmin(h.AdminListItems))
	handle("DELETE /api/admin/items/{kind}/{id}", auth.RequireAdmin(h.AdminDeleteItem))
	handle("POST /api/admin/items/{kind}/{id}/approve", auth.RequireAdmin(h.AdminApproveItem))
	handle("POST /api/admin/items/{kind}/{id}/reject", auth.RequireAdmin(h.AdminRejectItem))

	var handler http.Handler = mux
	handler = h.Sessions.Middleware(h.DB.GetUserByID)(handler)
	handler = withOriginCheck(handler, frontendURL)
	handler = withCORS(handler, frontendURL)
	handler = observability.Middleware(slog.New(slog.NewJSONHandler(os.Stdout, nil)), h.Config.AccessLog)(handler)
	return handler
}

func withCORS(next http.Handler, frontendURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", frontendURL)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
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
