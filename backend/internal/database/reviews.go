package database

import (
	"context"
	"fmt"
	"log"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) InsertReview(ctx context.Context, review models.Review) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO REVIEWS(
			recommendation_id, app_id, steam_id, language, review, voted_up, timestamp_created, timestamp_updated, playtime_forever, playtime_at_review, helpful_votes, funny_votes
		)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (recommendation_id) DO UPDATE SET
			app_id = EXCLUDED.app_id,
			steam_id = EXCLUDED.steam_id,
			language = EXCLUDED.language,
			review = EXCLUDED.review,
			voted_up = EXCLUDED.voted_up,
			timestamp_created = EXCLUDED.timestamp_created,
			timestamp_updated = EXCLUDED.timestamp_updated,
			playtime_forever = EXCLUDED.playtime_forever,
			playtime_at_review = EXCLUDED.playtime_at_review,
			helpful_votes = EXCLUDED.helpful_votes,
			funny_votes = EXCLUDED.funny_votes
		`,
		review.RecommendationID, review.AppID, review.SteamID, review.Language, review.Review, review.VotedUp, review.TimestampCreated, review.TimestampUpdated, review.PlaytimeForever, review.PlaytimeAtReview, review.HelpfulVotes, review.FunnyVotes,
	)
	if err != nil {
		log.Printf("Failed to insert review into database: %v", err)
		return fmt.Errorf("failed to insert review into database: %w", err)
	}
	return nil
}