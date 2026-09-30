package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

// The OpenID round trip carries a random, single-use `state` token whose
// Redis value records what the sign-in is for. Without it, an attacker could
// get a victim's browser to complete a sign-in that links the attacker's
// Steam account (or logs the victim into the attacker's account).
const steamStateTTL = 10 * time.Minute

const (
	steamIntentLogin      = "login"
	steamIntentLinkPrefix = "link:"
)

func steamStateKey(state string) string {
	return "steamstate:" + state
}

func (h *Handler) steamCallbackURL(state string) string {
	return h.Config.BackendURL + "/api/auth/steam/callback?state=" + url.QueryEscape(state)
}

func (h *Handler) startSteamSignIn(w http.ResponseWriter, r *http.Request, intent string) {
	state, err := auth.RandomToken(24)
	if err != nil {
		h.redirectToFrontend(w, r, "/login", "steam", "error")
		return
	}
	if err := h.Redis.Set(r.Context(), steamStateKey(state), intent, steamStateTTL).Err(); err != nil {
		log.Printf("steam sign-in: store state: %v", err)
		h.redirectToFrontend(w, r, "/login", "steam", "error")
		return
	}
	http.Redirect(w, r, steam.LoginURL(h.steamCallbackURL(state), h.Config.BackendURL), http.StatusFound)
}

// SteamLogin starts "Sign in through Steam" for an account that already has
// Steam linked.
func (h *Handler) SteamLogin(w http.ResponseWriter, r *http.Request) {
	h.startSteamSignIn(w, r, steamIntentLogin)
}

// SteamLink starts linking Steam to the logged-in account.
func (h *Handler) SteamLink(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r.Context())
	if user == nil {
		h.redirectToFrontend(w, r, "/login", "returnUrl", "/account")
		return
	}
	h.startSteamSignIn(w, r, steamIntentLinkPrefix+strconv.FormatInt(user.UserID, 10))
}

func (h *Handler) SteamCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	state := query.Get("state")
	if state == "" {
		h.redirectToFrontend(w, r, "/login", "steam", "error")
		return
	}
	intent, err := h.Redis.GetDel(r.Context(), steamStateKey(state)).Result()
	if err != nil {
		h.redirectToFrontend(w, r, "/login", "steam", "expired")
		return
	}

	failurePage := "/login"
	if strings.HasPrefix(intent, steamIntentLinkPrefix) {
		failurePage = "/account"
	}

	steamID, err := steam.Verify(r.Context(), h.httpClient, query, h.steamCallbackURL(state))
	if errors.Is(err, steam.ErrCancelled) {
		h.redirectToFrontend(w, r, failurePage, "steam", "cancelled")
		return
	}
	if err != nil {
		log.Printf("steam callback: verify: %v", err)
		h.redirectToFrontend(w, r, failurePage, "steam", "error")
		return
	}

	if intent == steamIntentLogin {
		user, err := h.DB.GetUserBySteamID(r.Context(), steamID)
		if errors.Is(err, database.ErrNotFound) {
			h.redirectToFrontend(w, r, "/login", "steam", "notlinked")
			return
		}
		if err != nil {
			log.Printf("steam callback: find user: %v", err)
			h.redirectToFrontend(w, r, "/login", "steam", "error")
			return
		}
		if err := h.Sessions.Create(r.Context(), w, user.UserID); err != nil {
			log.Printf("steam callback: session: %v", err)
			h.redirectToFrontend(w, r, "/login", "steam", "error")
			return
		}
		h.redirectToFrontend(w, r, "/feed", "", "")
		return
	}

	userID, err := strconv.ParseInt(strings.TrimPrefix(intent, steamIntentLinkPrefix), 10, 64)
	if err != nil {
		h.redirectToFrontend(w, r, "/account", "steam", "error")
		return
	}
	err = h.DB.SetSteamID(r.Context(), userID, &steamID)
	if errors.Is(err, database.ErrSteamIDLinked) {
		h.redirectToFrontend(w, r, "/account", "steam", "taken")
		return
	}
	if err != nil {
		log.Printf("steam callback: link: %v", err)
		h.redirectToFrontend(w, r, "/account", "steam", "error")
		return
	}
	h.Redis.Del(r.Context(), "steamprofile:"+steamID)
	h.redirectToFrontend(w, r, "/account", "steam", "linked")
}

func (h *Handler) redirectToFrontend(w http.ResponseWriter, r *http.Request, path, key, value string) {
	target := h.Config.FrontendURL + path
	if key != "" {
		target += "?" + url.Values{key: {value}}.Encode()
	}
	http.Redirect(w, r, target, http.StatusFound)
}
