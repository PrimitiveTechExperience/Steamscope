package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/pashagolub/pgxmock/v5"
)

func TestInsertReview(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	createdAt := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	review := models.Review{
		RecommendationID: "recommendation-123",
		AppID:            123,
		SteamID:          "steam-456",
		Language:         "english",
		Review:           "A helpful review",
		VotedUp:          true,
		TimestampCreated: createdAt,
		TimestampUpdated: updatedAt,
		PlaytimeForever:  120,
		PlaytimeAtReview: 90,
		HelpfulVotes:     10,
		FunnyVotes:       2,
	}

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO REVIEWS\(\s*recommendation_id, app_id, steam_id, language, review, voted_up, timestamp_created, timestamp_updated, playtime_forever, playtime_at_review, helpful_votes, funny_votes\s*\)\s*VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12\)\s*ON CONFLICT \(recommendation_id\) DO UPDATE SET`).
		WithArgs(
			review.RecommendationID,
			review.AppID,
			review.SteamID,
			review.Language,
			review.Review,
			review.VotedUp,
			review.TimestampCreated,
			review.TimestampUpdated,
			review.PlaytimeForever,
			review.PlaytimeAtReview,
			review.HelpfulVotes,
			review.FunnyVotes,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertReview(ctx, tx, review); err != nil {
		t.Fatalf("InsertReview() returned an error: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestPruneReviews(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()
	
	appID := 123
	maxReviews := 5
	mockPool.ExpectBegin()
	mockPool.ExpectExec(`DELETE FROM REVIEWS\s*WHERE app_id = \$1 AND recommendation_id NOT IN \(\s*SELECT recommendation_id FROM REVIEWS\s*WHERE app_id = \$1\s*ORDER BY timestamp_created DESC\s*LIMIT \$2\s*\)`).
		WithArgs(appID, maxReviews).
		WillReturnResult(pgxmock.NewResult("DELETE", 3))
	mockPool.ExpectRollback()
	
	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}
	
	if err := db.PruneReviews(ctx, tx, appID, maxReviews); err != nil {
		t.Fatalf("PruneReviews() returned an error: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestStoreReviews(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	appID := 123
	maxReviews := 5
	reviews := []models.Review{
		{
			RecommendationID: "recommendation-1",
			AppID:            appID,
			SteamID:          "steam-1",
			Language:         "english",
			Review:           "A helpful review",
			VotedUp:          true,
			TimestampCreated: time.Now(),
			TimestampUpdated: time.Now(),
			PlaytimeForever:  120,
			PlaytimeAtReview: 90,
			HelpfulVotes:     10,
			FunnyVotes:       2,
		},
	}

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`SELECT app_id FROM games WHERE app_id = \$1 FOR UPDATE`).
		WithArgs(appID).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	for _, review := range reviews {
		mockPool.ExpectExec(`INSERT INTO REVIEWS\(\s*recommendation_id, app_id, steam_id, language, review, voted_up, timestamp_created, timestamp_updated, playtime_forever, playtime_at_review, helpful_votes, funny_votes\s*\)\s*VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12\)\s*ON CONFLICT \(recommendation_id\) DO UPDATE SET`).
			WithArgs(
				review.RecommendationID,
				review.AppID,
				review.SteamID,
				review.Language,
				review.Review,
				review.VotedUp,
				review.TimestampCreated,
				review.TimestampUpdated,
				review.PlaytimeForever,
				review.PlaytimeAtReview,
				review.HelpfulVotes,
				review.FunnyVotes,
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
	}
	mockPool.ExpectExec(`DELETE FROM REVIEWS\s*WHERE app_id = \$1 AND recommendation_id NOT IN \(\s*SELECT recommendation_id FROM REVIEWS\s*WHERE app_id = \$1\s*ORDER BY timestamp_created DESC\s*LIMIT \$2\s*\)`).
		WithArgs(appID, maxReviews).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mockPool.ExpectCommit()

	db := &DB{Pool: mockPool}
	if err := db.StoreReviews(ctx, appID, reviews, maxReviews); err != nil {
		t.Fatalf("StoreReviews() returned an error: %v", err)
	}

	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestStorReviews_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()
	
	appID := 123
	maxReviews := 5
	reviews := []models.Review{
		{
			RecommendationID: "recommendation-1",
			AppID:            appID,
			SteamID:          "steam-1",
			Language:         "english",
			Review:           "A helpful review",
			VotedUp:          true,
			TimestampCreated: time.Now(),
			TimestampUpdated: time.Now(),
			PlaytimeForever:  120,
			PlaytimeAtReview: 90,
			HelpfulVotes:     10,
			FunnyVotes:       2,
		},
	}
	mockPool.ExpectBegin()
	mockPool.ExpectExec(`SELECT app_id FROM games WHERE app_id = \$1 FOR UPDATE`).
		WithArgs(appID).
		WillReturnError(fmt.Errorf("mock error"))

	db := &DB{Pool: mockPool}
	err = db.StoreReviews(ctx, appID, reviews, maxReviews)
	if err == nil {
		t.Fatalf("StoreReviews() did not return an error when expected")
	}
	
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}