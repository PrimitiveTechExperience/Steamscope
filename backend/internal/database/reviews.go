package database

import (
	"context"
	"fmt"
	"log"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

func (db *DB) InsertReview(ctx context.Context, tx pgx.Tx, review models.Review) error {
	// Need to convert timestamp_created and timestamp_updated from int64 to time.Time
	createdTime := review.TimestampCreated
	updatedTime := review.TimestampUpdated
	_, err := tx.Exec(
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
			timestamp_updated = EXCLUDED.timestamp_updated,
			playtime_forever = EXCLUDED.playtime_forever,
			playtime_at_review = EXCLUDED.playtime_at_review,
			helpful_votes = EXCLUDED.helpful_votes,
			funny_votes = EXCLUDED.funny_votes
		`,
		review.RecommendationID, review.AppID, review.SteamID, review.Language, review.Review, review.VotedUp, createdTime, updatedTime, review.PlaytimeForever, review.PlaytimeAtReview, review.HelpfulVotes, review.FunnyVotes,
	)
	if err != nil {
		log.Printf("Failed to insert review into database: %v", err)
		return fmt.Errorf("failed to insert review into database: %w", err)
	}
	return nil
}

func (db *DB) PruneReviews(ctx context.Context, tx pgx.Tx, appID int, maxReviews int) error {
	// Delete reviews for the given appID, keeping only the most recent maxReviews
	_, err := tx.Exec(
		ctx,
		`
		DELETE FROM REVIEWS
		WHERE app_id = $1 AND recommendation_id NOT IN (
			SELECT recommendation_id FROM REVIEWS
			WHERE app_id = $1
			ORDER BY timestamp_created DESC
			LIMIT $2
		)
		`,
		appID, maxReviews,
	)
	if err != nil {
		log.Printf("Failed to prune reviews for appID %d: %v", appID, err)
		return fmt.Errorf("failed to prune reviews for appID %d: %w", appID, err)
	}
	log.Printf("Successfully pruned reviews for appID %d, keeping only the most recent %d reviews.", appID, maxReviews)
	return nil
}

func (db *DB) StoreReviews(ctx context.Context, appID int, reviews []models.Review, maxReviews int) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		log.Printf("Failed to begin transaction: %v", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	// Lock the reviews for the given appID to prevent concurrent modifications
	_, err = tx.Exec(
		ctx,
		`SELECT app_id FROM games WHERE app_id = $1 FOR UPDATE`,
		appID,
	)
	if err != nil {
		log.Printf("Failed to lock reviews for appID %d: %v", appID, err)
		return fmt.Errorf("failed to lock reviews for appID %d: %w", appID, err)
	}
	for _, review := range reviews {
		err := db.InsertReview(ctx, tx, review)
		if err != nil {
			log.Printf("Failed to insert review: %v", err)
			return fmt.Errorf("failed to insert review: %w", err)
		}
	}
	err = db.PruneReviews(ctx, tx, appID, maxReviews)
	if err != nil {
		log.Printf("Failed to prune reviews: %v", err)
		return fmt.Errorf("failed to prune reviews: %w", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	log.Printf("Successfully stored reviews for appID %d and pruned to keep only the most recent %d reviews.", appID, maxReviews)
	return nil
}