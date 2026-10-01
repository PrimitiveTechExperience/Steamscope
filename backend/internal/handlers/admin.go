package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.DB.ListUsers(r.Context())
	if err != nil {
		log.Printf("admin list users: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load users")
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
		log.Printf("admin delete user: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete user")
	default:
		log.Printf("admin %s deleted user %d", admin.Username, id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) AdminListItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.DB.ListItems(r.Context())
	if err != nil {
		log.Printf("admin list items: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load games")
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
		log.Printf("admin delete item: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete")
	default:
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
		log.Printf("admin approve: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to approve")
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
		log.Printf("admin reject: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to reject")
		return
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
