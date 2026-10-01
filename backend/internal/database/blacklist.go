package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/moderation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrRuleExists = errors.New("that rule already exists")

type BlacklistRule struct {
	RuleID    int64     `json:"rule_id"`
	Field     string    `json:"field"`
	Pattern   string    `json:"pattern"`
	Note      string    `json:"note"`
	CreatedBy *string   `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (db *DB) ListBlacklist(ctx context.Context) ([]BlacklistRule, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT r.rule_id, r.field, r.pattern, r.note, u.username, r.created_at
		FROM blacklist_rules r
		LEFT JOIN users u ON u.user_id = r.created_by
		ORDER BY r.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list blacklist: %w", err)
	}
	defer rows.Close()
	rules := []BlacklistRule{}
	for rows.Next() {
		var r BlacklistRule
		if err := rows.Scan(&r.RuleID, &r.Field, &r.Pattern, &r.Note, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (db *DB) GetBlacklistRule(ctx context.Context, id int64) (*BlacklistRule, error) {
	var r BlacklistRule
	err := db.Pool.QueryRow(ctx, `
		SELECT r.rule_id, r.field, r.pattern, r.note, u.username, r.created_at
		FROM blacklist_rules r
		LEFT JOIN users u ON u.user_id = r.created_by
		WHERE r.rule_id = $1`, id,
	).Scan(&r.RuleID, &r.Field, &r.Pattern, &r.Note, &r.CreatedBy, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}
	return &r, nil
}

func (db *DB) AddBlacklistRule(ctx context.Context, field, pattern, note string, createdBy int64) (int64, error) {
	var id int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO blacklist_rules (field, pattern, note, created_by)
		VALUES ($1, $2, $3, $4) RETURNING rule_id`, field, pattern, note, createdBy,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, ErrRuleExists
		}
		return 0, fmt.Errorf("failed to add rule: %w", err)
	}
	return id, nil
}

func (db *DB) DeleteBlacklistRule(ctx context.Context, id int64) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM blacklist_rules WHERE rule_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CompiledBlacklist loads every rule and compiles it. Rules that no longer
// compile are skipped (they were validated on the way in).
func (db *DB) CompiledBlacklist(ctx context.Context) ([]moderation.Rule, error) {
	stored, err := db.ListBlacklist(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]moderation.Rule, 0, len(stored))
	for _, s := range stored {
		if r, err := moderation.CompileRule(s.Field, s.Pattern); err == nil {
			rules = append(rules, r)
		}
	}
	return rules, nil
}

// GamesMeta returns name, developers and publishers for one game (appID > 0)
// or every stored game (appID == 0), for matching against blacklist rules.
func (db *DB) GamesMeta(ctx context.Context, appID int) ([]moderation.GameMeta, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT g.app_id, g.name,
			COALESCE((SELECT array_agg(d.developer) FROM game_developers gd JOIN developers d USING (developer_id) WHERE gd.app_id = g.app_id), '{}'),
			COALESCE((SELECT array_agg(p.publisher) FROM game_publishers gp JOIN publishers p USING (publisher_id) WHERE gp.app_id = g.app_id), '{}')
		FROM games g
		WHERE $1 = 0 OR g.app_id = $1`, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to load game metadata: %w", err)
	}
	defer rows.Close()
	var metas []moderation.GameMeta
	for rows.Next() {
		var m moderation.GameMeta
		if err := rows.Scan(&m.AppID, &m.Name, &m.Developers, &m.Publishers); err != nil {
			return nil, fmt.Errorf("failed to scan game metadata: %w", err)
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

// RejectGame removes a game's stored data but keeps its tracked_games row,
// marked rejected, so it isn't scraped again and the admin can see why.
func (db *DB) RejectGame(ctx context.Context, appID int) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM games WHERE app_id = $1`, appID); err != nil {
		return fmt.Errorf("failed to delete game: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tracked_games (app_id, status) VALUES ($1, 'rejected')
		ON CONFLICT (app_id) DO UPDATE SET status = 'rejected'`, appID); err != nil {
		return fmt.Errorf("failed to mark game rejected: %w", err)
	}
	return tx.Commit(ctx)
}
