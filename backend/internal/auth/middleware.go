package auth

import (
	"context"
	"net/http"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

type ctxKey struct{}

// UserLoader fetches a user by ID; injected so this package doesn't depend
// on the database layer.
type UserLoader func(ctx context.Context, userID int64) (*models.User, error)

// Middleware attaches the logged-in user (if any) to the request context.
// It never rejects a request - use RequireAuth for that.
func (m *SessionManager) Middleware(load UserLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userID, ok := m.Resolve(r.Context(), r); ok {
				if user, err := load(r.Context(), userID); err == nil && user != nil {
					r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, user))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CurrentUser returns the logged-in user for this request, or nil.
func CurrentUser(ctx context.Context) *models.User {
	user, _ := ctx.Value(ctxKey{}).(*models.User)
	return user
}

// RequireAuth rejects requests without a logged-in user with 401.
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if CurrentUser(r.Context()) == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"not logged in"}`))
			return
		}
		next(w, r)
	}
}

// RequireAdmin rejects anyone who isn't a logged-in admin: 401 when logged
// out, 403 when logged in without admin rights.
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r.Context())
		if user == nil || !user.IsAdmin {
			status, msg := http.StatusUnauthorized, `{"error":"not logged in"}`
			if user != nil {
				status, msg = http.StatusForbidden, `{"error":"admin access required"}`
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			w.Write([]byte(msg))
			return
		}
		next(w, r)
	}
}
