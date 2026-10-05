package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/moderation"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/sanitize"
)

const (
	minPasswordLength = 8
	maxPasswordLength = 128
)

// A real hash of a random password, verified against when a login name
// doesn't exist so that "no such user" and "wrong password" take the same
// time and can't be told apart.
var dummyPasswordHash, _ = auth.HashPassword("steamscope-dummy-password")

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// Usernames are then held to a strict allow-list by ValidateUsername;
	// emails must be a single clean token with no stray characters.
	req.Username = strings.TrimSpace(req.Username)
	email, cleanEmail := sanitize.Identifier(req.Email, 254)
	if !cleanEmail {
		writeError(w, http.StatusBadRequest, "please enter a valid email address")
		return
	}
	req.Email = email

	if err := moderation.ValidateUsername(req.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email || len(req.Email) > 254 {
		writeError(w, http.StatusBadRequest, "please enter a valid email address")
		return
	}
	if len(req.Password) < minPasswordLength || len(req.Password) > maxPasswordLength {
		writeError(w, http.StatusBadRequest, "password must be 8-128 characters")
		return
	}
	if !sanitize.ValidPassword(req.Password) {
		writeError(w, http.StatusBadRequest, "password contains invalid characters")
		return
	}
	if !cache.Allow(r.Context(), h.Redis, "ratelimit:register:"+clientIP(r), 10, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "too many sign-ups from this address, try again later")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Printf("register: hash failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	user, err := h.DB.CreateUser(r.Context(), req.Username, req.Email, hash)
	switch {
	case errors.Is(err, database.ErrUsernameTaken), errors.Is(err, database.ErrEmailTaken):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		log.Printf("register: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	if err := h.Sessions.Create(r.Context(), w, user.UserID); err != nil {
		log.Printf("register: session: %v", err)
		writeError(w, http.StatusInternalServerError, "account created, but failed to log in")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// Bound the login before it's used in a rate-limit key or a query.
	req.Login = sanitize.Text(req.Login, 254)
	if !sanitize.ValidPassword(req.Password) || len(req.Password) > maxPasswordLength {
		writeError(w, http.StatusUnauthorized, "invalid username/email or password")
		return
	}

	limitKey := "ratelimit:login:" + clientIP(r) + ":" + strings.ToLower(req.Login)
	if !cache.Allow(r.Context(), h.Redis, limitKey, 10, 15*time.Minute) {
		observability.LoginAttempts.WithLabelValues("rate_limited").Inc()
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again in a few minutes")
		return
	}

	user, err := h.DB.GetUserByLogin(r.Context(), req.Login)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		log.Printf("login: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to log in")
		return
	}

	hash := dummyPasswordHash
	if user != nil {
		hash = user.PasswordHash
	}
	ok, _ := auth.VerifyPassword(req.Password, hash)
	if user == nil || !ok {
		observability.LoginAttempts.WithLabelValues("bad_credentials").Inc()
		writeError(w, http.StatusUnauthorized, "invalid username/email or password")
		return
	}

	if user.IsBanned {
		observability.LoginAttempts.WithLabelValues("banned").Inc()
		writeError(w, http.StatusForbidden, "this account has been suspended")
		return
	}

	if err := h.Sessions.Create(r.Context(), w, user.UserID); err != nil {
		log.Printf("login: session: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to log in")
		return
	}
	observability.LoginAttempts.WithLabelValues("success").Inc()
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.Sessions.Destroy(r.Context(), w, r)
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the current user, or {"user": null} when logged out - a 200
// either way, so checking login state doesn't spam the console with 401s.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": auth.CurrentUser(r.Context())})
}
