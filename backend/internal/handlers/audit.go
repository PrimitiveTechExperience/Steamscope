package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/auth"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// Audit actions. The admin page filters on these.
const (
	auditUserDelete        = "user.delete"
	auditUserBan           = "user.ban"
	auditUserUnban         = "user.unban"
	auditBlockSubmissions  = "user.block_submissions"
	auditUnblockSubmission = "user.unblock_submissions"
	auditItemDelete        = "item.delete"
	auditItemApprove       = "item.approve"
	auditItemReject        = "item.reject"
	auditRuleAdd           = "blacklist.add"
	auditRuleDelete        = "blacklist.delete"
	auditRulePurge         = "blacklist.purge"
)

var auditActions = map[string]bool{
	auditUserDelete: true, auditUserBan: true, auditUserUnban: true,
	auditBlockSubmissions: true, auditUnblockSubmission: true,
	auditItemDelete: true, auditItemApprove: true, auditItemReject: true,
	auditRuleAdd: true, auditRuleDelete: true, auditRulePurge: true,
}

// audit records what the current admin just did. It runs after the change
// has been made; a failure to record it is logged rather than undoing the
// change or failing the request.
func (h *Handler) audit(r *http.Request, action, targetType, targetID string, details map[string]any) {
	admin := auth.CurrentUser(r.Context())
	if admin == nil {
		return
	}
	raw, err := json.Marshal(details)
	if err != nil || details == nil {
		raw = []byte(`{}`)
	}
	adminID := admin.UserID
	entry := database.AuditEntry{
		AdminID: &adminID, AdminUsername: admin.Username,
		Action: action, TargetType: targetType, TargetID: targetID,
		Details: raw, IP: clientIP(r),
	}
	if err := h.DB.AddAuditEntry(r.Context(), entry); err != nil {
		log.Printf("AUDIT LOG WRITE FAILED: admin %d %s %s %s %s: %v", adminID, action, targetType, targetID, raw, err)
	}
}

// AdminAuditLog lists audit entries, newest first: ?action= filters to one
// kind and ?before=<audit_id> pages back.
func (h *Handler) AdminAuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := database.AuditFilter{Action: q.Get("action"), Limit: 50}
	if f.Action != "" && !auditActions[f.Action] {
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}
	if v := q.Get("before"); v != "" {
		before, err := strconv.ParseInt(v, 10, 64)
		if err != nil || before <= 0 {
			writeError(w, http.StatusBadRequest, "invalid before")
			return
		}
		f.Before = before
	}
	if v := q.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 200 {
			writeError(w, http.StatusBadRequest, "limit must be 1-200")
			return
		}
		f.Limit = limit
	}
	entries, err := h.DB.ListAuditLog(r.Context(), f)
	if err != nil {
		serverError(w, r, "admin audit log", err, "failed to load the audit log")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
