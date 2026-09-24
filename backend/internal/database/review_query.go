package database

import (
	"context"
	"fmt"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) GetReviews(ctx context.Context, appID int, limit int, offset int) ([]models.Review, error) {
	query := `
	SELECT recommendation_id, app_id, steam_id, author_name, author_avatar, num_games_owned, num_reviews, language, review, voted_up, timestamp_created, timestamp_updated, playtime_forever, playtime_at_review, helpful_votes, funny_votes
	FROM reviews
	WHERE app_id = $1
	ORDER BY timestamp_created DESC
	LIMIT $2 OFFSET $3
	`
	rows, err := db.Pool.Query(ctx, query, appID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get reviews: %w", err)
	}
	defer rows.Close()

	var reviews []models.Review
	for rows.Next() {
		var review models.Review
		err := rows.Scan(
			&review.RecommendationID,
			&review.AppID,
			&review.SteamID,
			&review.AuthorName,
			&review.AuthorAvatar,
			&review.NumGamesOwned,
			&review.NumReviews,
			&review.Language,
			&review.Review,
			&review.VotedUp,
			&review.TimestampCreated,
			&review.TimestampUpdated,
			&review.PlaytimeForever,
			&review.PlaytimeAtReview,
			&review.HelpfulVotes,
			&review.FunnyVotes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan review: %w", err)
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func (db *DB) GetCountOfReviews(ctx context.Context, appID int) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM reviews WHERE app_id = $1`, appID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count of reviews: %w", err)
	}
	return count, nil
}
