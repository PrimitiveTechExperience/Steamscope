package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// AuditEntry is one thing an admin did.
type AuditEntry struct {
	AuditID       int64           `json:"audit_id"`
	AdminID       *int64          `json:"admin_id"`
	AdminUsername string          `json:"admin_username"`
	Action        string          `json:"action"`
	TargetType    string          `json:"target_type"`
	TargetID      string          `json:"target_id"`
	Details       json.RawMessage `json:"details"`
	IP            string          `json:"ip"`
	CreatedAt     time.Time       `json:"created_at"`
}

// AuditFilter narrows the audit log. Before is an audit_id to page back from.
type AuditFilter struct {
	Action string
	Before int64
	Limit  int
}

func (db *DB) AddAuditEntry(ctx context.Context, e AuditEntry) error {
	details := e.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO admin_audit_log (admin_id, admin_username, action, target_type, target_id, details, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.AdminID, e.AdminUsername, e.Action, e.TargetType, e.TargetID, details, e.IP,
	)
	if err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}
	return nil
}

// ListAuditLog returns entries newest first.
func (db *DB) ListAuditLog(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT audit_id, admin_id, admin_username, action, target_type, target_id, details, ip, created_at
		FROM admin_audit_log
		WHERE ($1 = '' OR action = $1) AND ($2 = 0 OR audit_id < $2)
		ORDER BY audit_id DESC
		LIMIT $3`, f.Action, f.Before, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit log: %w", err)
	}
	defer rows.Close()
	entries := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.AuditID, &e.AdminID, &e.AdminUsername, &e.Action, &e.TargetType, &e.TargetID, &e.Details, &e.IP, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to read audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ItemName returns a game's or bundle's name for the audit log, or "" when
// it isn't stored (yet).
func (db *DB) ItemName(ctx context.Context, kind string, id int) string {
	query := `SELECT name FROM games WHERE app_id = $1`
	if kind == "bundle" {
		query = `SELECT name FROM bundles WHERE bundle_id = $1`
	}
	var name string
	db.Pool.QueryRow(ctx, query, id).Scan(&name)
	return name
}
