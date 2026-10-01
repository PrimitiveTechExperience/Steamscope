package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrUsernameTaken = errors.New("username is already taken")
	ErrEmailTaken    = errors.New("email is already registered")
	ErrSteamIDLinked = errors.New("that Steam account is linked to another user")
	ErrNotFound      = errors.New("not found")
)

const userColumns = `user_id, username, email, password_hash, steam_id, is_admin, created_at`

func scanUser(row pgx.Row) (*models.User, error) {
	var u models.User
	if err := row.Scan(&u.UserID, &u.Username, &u.Email, &u.PasswordHash, &u.SteamID, &u.IsAdmin, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// CreateUser inserts a user and their default preferences in one
// transaction, translating unique-index violations into friendly errors.
func (db *DB) CreateUser(ctx context.Context, username, email, passwordHash string) (*models.User, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	user, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING `+userColumns,
		username, email, passwordHash,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "idx_users_username_lower":
				return nil, ErrUsernameTaken
			case "idx_users_email_lower":
				return nil, ErrEmailTaken
			}
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO user_preferences (user_id) VALUES ($1)`, user.UserID); err != nil {
		return nil, fmt.Errorf("failed to create preferences: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit user: %w", err)
	}
	return user, nil
}

func (db *DB) GetUserByID(ctx context.Context, userID int64) (*models.User, error) {
	return scanUser(db.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE user_id = $1`, userID))
}

// GetUserByLogin looks a user up by username or email, case-insensitively.
func (db *DB) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	return scanUser(db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(username) = lower($1) OR lower(email) = lower($1) LIMIT 1`,
		login,
	))
}

func (db *DB) GetUserBySteamID(ctx context.Context, steamID string) (*models.User, error) {
	return scanUser(db.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE steam_id = $1`, steamID))
}

// SetSteamID links (or, with nil, unlinks) a Steam account.
func (db *DB) SetSteamID(ctx context.Context, userID int64, steamID *string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET steam_id = $2, updated_at = now() WHERE user_id = $1`,
		userID, steamID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrSteamIDLinked
		}
		return fmt.Errorf("failed to set steam id: %w", err)
	}
	return nil
}
