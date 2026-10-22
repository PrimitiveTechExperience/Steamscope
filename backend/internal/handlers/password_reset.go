package handlers

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/cache"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/sanitize"
)

const resetTokenTTL = time.Hour

// resetRequestedMessage is the answer to every reset request, whether or not
// an account uses that email, so the form can't be used to find accounts.
const resetRequestedMessage = "If an account uses that email, a link to reset its password is on its way. It works for one hour."

type passwordResetRequest struct {
	Email string `json:"email"`
}

// RequestPasswordReset emails a reset link to the account with that email.
// The lookup and the email happen after the response is sent, so the answer
// and how long it takes are the same for every address.
func (h *Handler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	if h.Alerts == nil || h.Alerts.Mail == nil {
		writeError(w, http.StatusServiceUnavailable, "password reset by email isn't set up on this server. Ask an admin for help")
		return
	}
	var req passwordResetRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	email, clean := sanitize.Identifier(req.Email, 254)
	if addr, err := mail.ParseAddress(email); !clean || err != nil || addr.Address != email {
		writeError(w, http.StatusBadRequest, "please enter a valid email address")
		return
	}
	if !cache.Allow(r.Context(), h.Redis, "ratelimit:reset:"+clientIP(r), 5, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "too many reset requests from this address, try again later")
		return
	}
	// A cap per email too, so nobody can flood one inbox. Going over it is not
	// reported: the answer must not depend on the address.
	if cache.Allow(r.Context(), h.Redis, "ratelimit:reset:email:"+strings.ToLower(email), 3, time.Hour) {
		go h.sendResetLink(email)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": resetRequestedMessage})
}

func hashResetToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (h *Handler) sendResetLink(email string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user, err := h.DB.GetUserByEmail(ctx, email)
	if errors.Is(err, database.ErrNotFound) {
		return
	}
	if err != nil {
		log.Printf("password reset: lookup failed: %v", err)
		return
	}
	if user.IsBanned {
		return
	}
	token, err := auth.RandomToken(32)
	if err != nil {
		log.Printf("password reset: %v", err)
		return
	}
	if err := h.DB.CreatePasswordResetToken(ctx, user.UserID, hashResetToken(token), resetTokenTTL); err != nil {
		log.Printf("password reset: %v", err)
		return
	}
	link := h.Config.FrontendURL + "/reset-password?token=" + url.QueryEscape(token)
	body := fmt.Sprintf("Hi %s,\n\n"+
		"Someone asked to reset the password of your Steamscope account. To choose a new one, open this link:\n\n%s\n\n"+
		"It works once, for one hour. If you didn't ask for this, ignore this email: your password stays as it is.\n",
		user.Username, link)
	if err := h.Alerts.Mail.Send(ctx, user.Email, "Steamscope: reset your password", body); err != nil {
		log.Printf("password reset: email to user %d failed: %v", user.UserID, err)
	}
}

type passwordResetConfirm struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// ConfirmPasswordReset sets a new password from a reset link. It signs the
// account out everywhere and lifts any sign-in lockout.
func (h *Handler) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetConfirm
	if !decodeJSON(w, r, &req) {
		return
	}
	if !cache.Allow(r.Context(), h.Redis, "ratelimit:reset-confirm:"+clientIP(r), 10, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "too many attempts from this address, try again later")
		return
	}
	// Tokens are 43 base64url characters; anything else can't be one.
	if len(req.Token) != 43 {
		writeError(w, http.StatusBadRequest, "this reset link is invalid or has expired. Ask for a new one")
		return
	}
	if !checkNewPassword(w, req.Password) {
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		serverError(w, r, "password reset: hash failed", err, "failed to reset the password")
		return
	}
	userID, err := h.DB.ResetPassword(r.Context(), hashResetToken(req.Token), hash)
	if errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "this reset link is invalid or has expired. Ask for a new one")
		return
	}
	if err != nil {
		serverError(w, r, "password reset", err, "failed to reset the password")
		return
	}

	if err := h.Sessions.DestroyAll(r.Context(), userID); err != nil {
		log.Printf("password reset: could not end the sessions of user %d: %v", userID, err)
	}
	h.Sessions.Destroy(r.Context(), w, r)
	h.clearLockout(r.Context(), userLockoutKey(userID))
	if user, err := h.DB.GetUserByID(r.Context(), userID); err == nil {
		h.sendPasswordChangedEmail(user.Username, user.Email, userID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Your password has been changed. Sign in with the new one."})
}

func (h *Handler) sendPasswordChangedEmail(username, email string, userID int64) {
	if h.Alerts == nil || h.Alerts.Mail == nil {
		return
	}
	body := fmt.Sprintf("Hi %s,\n\n"+
		"The password of your Steamscope account was just changed with a reset link, and every device was signed out.\n\n"+
		"If that wasn't you, reset it again straight away: %s/forgot-password\n",
		username, h.Config.FrontendURL)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.Alerts.Mail.Send(ctx, email, "Steamscope: your password was changed", body); err != nil {
			log.Printf("password reset: confirmation email to user %d failed: %v", userID, err)
		}
	}()
}
