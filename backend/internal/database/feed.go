package database

import (
	"context"
	"fmt"
	"math"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

const (
	dealThreshold  = 0.9 // "below usual" = at least 10% under the 90-day average
	maxDeals       = 12
	maxSuggestions = 8
)

// GetDeals returns games currently priced at least 10% below their 90-day
// average price, the user's watched games first.
func (db *DB) GetDeals(ctx context.Context, userID int64) ([]models.Deal, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH avg90 AS (
			SELECT app_id, avg(price) AS avg_price
			FROM price_history
			WHERE recorded_date >= CURRENT_DATE - 90
			GROUP BY app_id
		)
		SELECT `+gameCardColumns+`, a.avg_price, (w.app_id IS NOT NULL) AS watched
		FROM games g
		JOIN avg90 a ON a.app_id = g.app_id
		LEFT JOIN watched_games w ON w.app_id = g.app_id AND w.user_id = $1
		WHERE a.avg_price > 0 AND g.price <= a.avg_price * $2::numeric
		ORDER BY watched DESC, (1 - g.price / a.avg_price) DESC
		LIMIT $3`, userID, dealThreshold, maxDeals)
	if err != nil {
		return nil, fmt.Errorf("failed to get deals: %w", err)
	}
	defer rows.Close()

	deals := []models.Deal{}
	for rows.Next() {
		var d models.Deal
		game, err := scanGameCard(rows, &d.AveragePrice, &d.Watched)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deal: %w", err)
		}
		d.Game = game
		d.PercentBelowUsual = int(math.Round((1 - game.Price/d.AveragePrice) * 100))
		deals = append(deals, d)
	}
	return deals, rows.Err()
}

// GetSuggestions ranks unwatched games by how many genres (weighted x3) and
// tags they share with the user's watchlist and preferred genres. Falls
// back to popular discounted games when there's nothing to go on yet.
func (db *DB) GetSuggestions(ctx context.Context, userID int64) ([]models.Game, error) {
	suggestions, err := db.queryGameCards(ctx, `
		WITH watched AS (
			SELECT app_id FROM watched_games WHERE user_id = $1
		),
		liked_genres AS (
			SELECT gg.genre_id FROM game_genres gg JOIN watched w ON w.app_id = gg.app_id
			UNION
			SELECT ge.genre_id FROM genres ge
			JOIN user_preferences p ON p.user_id = $1
			WHERE ge.genre = ANY(p.preferred_genres)
		),
		liked_tags AS (
			SELECT gt.tag_id FROM game_tags gt JOIN watched w ON w.app_id = gt.app_id
		),
		scores AS (
			SELECT g.app_id,
				3 * (SELECT count(*) FROM game_genres gg WHERE gg.app_id = g.app_id AND gg.genre_id IN (SELECT genre_id FROM liked_genres))
				+ (SELECT count(*) FROM game_tags gt WHERE gt.app_id = g.app_id AND gt.tag_id IN (SELECT tag_id FROM liked_tags))
				AS score
			FROM games g
			WHERE g.app_id NOT IN (SELECT app_id FROM watched)
		)
		SELECT `+gameCardColumns+`
		FROM games g
		JOIN scores s ON s.app_id = g.app_id
		WHERE s.score > 0
		ORDER BY s.score DESC, g.discount_percentage DESC, g.review_count DESC
		LIMIT $2`, userID, maxSuggestions)
	if err != nil {
		return nil, fmt.Errorf("failed to get suggestions: %w", err)
	}
	if len(suggestions) > 0 {
		return suggestions, nil
	}

	fallback, err := db.queryGameCards(ctx, `
		SELECT `+gameCardColumns+`
		FROM games g
		WHERE g.app_id NOT IN (SELECT app_id FROM watched_games WHERE user_id = $1)
		ORDER BY g.discount_percentage DESC, g.review_count DESC
		LIMIT $2`, userID, maxSuggestions)
	if err != nil {
		return nil, fmt.Errorf("failed to get fallback suggestions: %w", err)
	}
	return fallback, nil
}

func (db *DB) queryGameCards(ctx context.Context, query string, args ...any) ([]models.Game, error) {
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	games, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.Game, error) {
		return scanGameCard(row)
	})
	if games == nil {
		games = []models.Game{}
	}
	return games, err
}
