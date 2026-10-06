package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/moderation"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/observability"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/sanitize"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.DB.ListUsers(r.Context(), 1000)
	if err != nil {
		serverError(w, r, "admin list users", err, "failed to load users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) AdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	admin := auth.CurrentUser(r.Context())
	id, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if id == admin.UserID {
		writeError(w, http.StatusBadRequest, "you can't delete your own account here")
		return
	}
	switch err := h.DB.DeleteUser(r.Context(), id); {
	case errors.Is(err, database.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such user (admins can't be deleted)")
	case err != nil:
		serverError(w, r, "admin delete user", err, "failed to delete user")
	default:
		log.Printf("admin %s deleted user %d", admin.Username, id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) AdminListItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.DB.ListItems(r.Context(), 1000)
	if err != nil {
		serverError(w, r, "admin list items", err, "failed to load games")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// itemFromPath parses the {kind} and {id} path values of the admin item routes.
func itemFromPath(w http.ResponseWriter, r *http.Request) (kind string, id int, ok bool) {
	kind = r.PathValue("kind")
	id, err := strconv.Atoi(r.PathValue("id"))
	if (kind != "app" && kind != "bundle") || err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid item")
		return "", 0, false
	}
	return kind, id, true
}

func (h *Handler) AdminDeleteItem(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := itemFromPath(w, r)
	if !ok {
		return
	}
	switch err := h.DB.DeleteItem(r.Context(), kind, id); {
	case errors.Is(err, database.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such game or bundle")
	case err != nil:
		serverError(w, r, "admin delete item", err, "failed to delete")
	default:
		if kind == "app" {
			if err := h.DB.DeleteWishlistRequests(r.Context(), id); err != nil {
				log.Printf("admin delete item: %v", err)
			}
		}
		h.InvalidateCaches(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminApproveItem lets a submitted game or bundle through to scraping.
func (h *Handler) AdminApproveItem(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := itemFromPath(w, r)
	if !ok {
		return
	}
	submitter, err := h.DB.ReviewItem(r.Context(), kind, id, "pending")
	if errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "that submission isn't awaiting approval")
		return
	}
	if err != nil {
		serverError(w, r, "admin approve", err, "failed to approve")
		return
	}
	var userID int64
	if submitter != nil {
		userID = *submitter
	}
	if !h.enqueueSubmission(r.Context(), steam.StoreKind(kind), id, userID) {
		writeError(w, http.StatusServiceUnavailable, "too many submissions are being processed right now, try again shortly")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdminRejectItem turns a submission down and tells the submitter.
func (h *Handler) AdminRejectItem(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := itemFromPath(w, r)
	if !ok {
		return
	}
	submitter, err := h.DB.ReviewItem(r.Context(), kind, id, "rejected")
	if errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "that submission isn't awaiting approval")
		return
	}
	if err != nil {
		serverError(w, r, "admin reject", err, "failed to reject")
		return
	}
	if kind == "app" {
		if err := h.DB.DeleteWishlistRequests(r.Context(), id); err != nil {
			log.Printf("admin reject: %v", err)
		}
	}
	if submitter != nil {
		what := fmt.Sprintf("game %d", id)
		if kind == "bundle" {
			what = fmt.Sprintf("bundle %d", id)
		}
		h.DB.CreateNotification(r.Context(), *submitter, nil, "submission_rejected",
			fmt.Sprintf("Your submission of Steam %s wasn't approved.", what))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) AdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.DB.GetAdminStats(r.Context())
	if err != nil {
		serverError(w, r, "admin stats", err, "failed to load stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type moderationRequest struct {
	IsBanned           *bool `json:"is_banned"`
	SubmissionsBlocked *bool `json:"submissions_blocked"`
}

// AdminModerateUser bans/unbans a user or blocks/unblocks their game
// suggestions. Admins (including the caller) can't be moderated this way.
func (h *Handler) AdminModerateUser(w http.ResponseWriter, r *http.Request) {
	admin := auth.CurrentUser(r.Context())
	id, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req moderationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.IsBanned == nil && req.SubmissionsBlocked == nil {
		writeError(w, http.StatusBadRequest, "nothing to change")
		return
	}
	switch err := h.DB.SetUserModeration(r.Context(), id, req.IsBanned, req.SubmissionsBlocked); {
	case errors.Is(err, database.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such user (admins can't be moderated)")
	case err != nil:
		serverError(w, r, "admin moderate user", err, "failed to update user")
	default:
		log.Printf("admin %s moderated user %d: %+v", admin.Username, id, req)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) AdminListBlacklist(w http.ResponseWriter, r *http.Request) {
	rules, err := h.DB.ListBlacklist(r.Context())
	if err != nil {
		serverError(w, r, "admin list blacklist", err, "failed to load the blacklist")
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

type blacklistRequest struct {
	Field   string `json:"field"`
	Pattern string `json:"pattern"`
	Note    string `json:"note"`
}

func (h *Handler) AdminAddBlacklistRule(w http.ResponseWriter, r *http.Request) {
	admin := auth.CurrentUser(r.Context())
	var req blacklistRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	pattern, ok := sanitize.Pattern(req.Pattern, 200)
	if !ok {
		writeError(w, http.StatusBadRequest, "pattern must be 1-200 characters with no control characters")
		return
	}
	req.Pattern = pattern
	req.Note = sanitize.Text(req.Note, 200)
	if err := moderation.ValidateRule(req.Field, req.Pattern); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := h.DB.AddBlacklistRule(r.Context(), req.Field, req.Pattern, req.Note, admin.UserID)
	switch {
	case errors.Is(err, database.ErrRuleExists):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		serverError(w, r, "admin add blacklist rule", err, "failed to add rule")
		return
	}
	rule, err := h.DB.GetBlacklistRule(r.Context(), id)
	if err != nil {
		serverError(w, r, "admin reload blacklist rule", err, "rule added, but failed to reload it")
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (h *Handler) AdminDeleteBlacklistRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("ruleID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	switch err := h.DB.DeleteBlacklistRule(r.Context(), id); {
	case errors.Is(err, database.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such rule")
	case err != nil:
		serverError(w, r, "admin delete blacklist rule", err, "failed to delete rule")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// matchingGames returns the stored games a rule would block.
func (h *Handler) matchingGames(r *http.Request) (rule *database.BlacklistRule, matches []moderation.GameMeta, status int, err error) {
	id, perr := strconv.ParseInt(r.PathValue("ruleID"), 10, 64)
	if perr != nil || id <= 0 {
		return nil, nil, http.StatusBadRequest, errors.New("invalid rule id")
	}
	rule, err = h.DB.GetBlacklistRule(r.Context(), id)
	if errors.Is(err, database.ErrNotFound) {
		return nil, nil, http.StatusNotFound, errors.New("no such rule")
	}
	if err != nil {
		observability.RecordError(r.Context(), "blacklist matches: load rule", err)
		return nil, nil, http.StatusInternalServerError, errors.New("failed to load the rule")
	}
	compiled, err := moderation.CompileRule(rule.Field, rule.Pattern)
	if err != nil {
		observability.RecordError(r.Context(), "blacklist matches: compile rule", err, "rule_id", id)
		return nil, nil, http.StatusInternalServerError, errors.New("that rule is no longer valid")
	}
	metas, err := h.DB.GamesMeta(r.Context(), 0)
	if err != nil {
		observability.RecordError(r.Context(), "blacklist matches: load games", err)
		return nil, nil, http.StatusInternalServerError, errors.New("failed to load games")
	}
	for _, m := range metas {
		if compiled.Matches(m) {
			matches = append(matches, m)
		}
	}
	return rule, matches, http.StatusOK, nil
}

// AdminBlacklistMatches previews which games already on the site a rule blocks.
func (h *Handler) AdminBlacklistMatches(w http.ResponseWriter, r *http.Request) {
	_, matches, status, err := h.matchingGames(r)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		out = append(out, map[string]any{"app_id": m.AppID, "name": m.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// AdminPurgeBlacklistMatches removes every game on the site that a rule blocks.
func (h *Handler) AdminPurgeBlacklistMatches(w http.ResponseWriter, r *http.Request) {
	_, matches, status, err := h.matchingGames(r)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	removed := 0
	for _, m := range matches {
		if err := h.DB.RejectGame(r.Context(), m.AppID); err != nil {
			log.Printf("blacklist purge %d: %v", m.AppID, err)
			continue
		}
		removed++
	}
	h.InvalidateCaches(r.Context())
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}
