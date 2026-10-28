package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// GetUserByEmail looks a user up by email only, case-insensitively.
func (db *DB) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return scanUser(db.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email))
}

// CreatePasswordResetToken stores the hash of a new reset token. Tokens long
// past their expiry are cleared out at the same time.
func (db *DB) CreatePasswordResetToken(ctx context.Context, userID int64, tokenHash []byte, ttl time.Duration) error {
	if _, err := db.Pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < now() - interval '1 day'`); err != nil {
		return fmt.Errorf("failed to clear old reset tokens: %w", err)
	}
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		tokenHash, userID, fmt.Sprintf("%d seconds", int(ttl.Seconds())),
	)
	if err != nil {
		return fmt.Errorf("failed to store reset token: %w", err)
	}
	return nil
}

// ResetPassword sets a new password using a reset token, in one transaction:
// the token must be unused and unexpired, and afterwards every outstanding
// token of that user stops working. Returns ErrNotFound for a bad token.
func (db *DB) ResetPassword(ctx context.Context, tokenHash []byte, passwordHash string) (int64, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID int64
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM password_reset_tokens
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		FOR UPDATE`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to look up reset token: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE user_id = $1`, userID, passwordHash); err != nil {
		return 0, fmt.Errorf("failed to set password: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE password_reset_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return 0, fmt.Errorf("failed to use up reset tokens: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit password reset: %w", err)
	}
	return userID, nil
}
