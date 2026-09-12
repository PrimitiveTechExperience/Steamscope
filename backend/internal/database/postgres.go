// nothing
package database

import (
	"context"
	"fmt"
	"log"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection pool: %w", err)
	}	
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	log.Println("Database connection established successfully.")
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
	log.Println("Database connection closed successfully.")
}

func (db *DB) InsertGame(ctx context.Context, game models.Game) error {
	_, err := db.Pool.Exec(
		ctx, 
		`
		INSERT INTO games (
			app_id, name
		)
			VALUES ($1, $2)
		ON CONFLICT (app_id) DO UPDATE SET
			name = EXCLUDED.name	
		`,
		game.AppID, game.Name,
	)
	if err != nil {
		log.Printf("Failed to insert game into database: %v", err)
		return fmt.Errorf("failed to insert game into database: %w", err)
	}
	return nil
}

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

func (db *DB) InsertLanguage(ctx context.Context, language string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO languages (language)
			VALUES ($1)
		ON CONFLICT (language) DO NOTHING
		`,
		language,
	)
	if err != nil {
		log.Printf("Failed to insert language into database: %v", err)
		return fmt.Errorf("failed to insert language into database: %w", err)
	}
	return nil
}

func (db *DB) InsertGameLanguage(ctx context.Context, appID int, language string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_languages (app_id, language)
			VALUES ($1, $2)
		ON CONFLICT (app_id, language) DO NOTHING
		`,
		appID, language,
	)
	if err != nil {
		log.Printf("Failed to insert game language into database: %v", err)
		return fmt.Errorf("failed to insert game language into database: %w", err)
	}
	return nil
}

func (db *DB) InsertDeveloper(ctx context.Context, developer string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO developers (developer)
			VALUES ($1)
		ON CONFLICT (developer) DO NOTHING
		`,
		developer,
	)
	if err != nil {
		log.Printf("Failed to insert developer into database: %v", err)
		return fmt.Errorf("failed to insert developer into database: %w", err)
	}
	return nil
}

func (db *DB) InsertGameDeveloper(ctx context.Context, appID int, developer string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_developers (app_id, developer)
			VALUES ($1, $2)
		ON CONFLICT (app_id, developer) DO NOTHING
		`,
		appID, developer,
	)
	if err != nil {
		log.Printf("Failed to insert game developer into database: %v", err)
		return fmt.Errorf("failed to insert game developer into database: %w", err)
	}
	return nil
}

func (db *DB) InsertPublisher(ctx context.Context, publisher string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO publishers (publisher)
			VALUES ($1)
		ON CONFLICT (publisher) DO NOTHING
		`,
		publisher,
	)
	if err != nil {
		log.Printf("Failed to insert publisher into database: %v", err)
		return fmt.Errorf("failed to insert publisher into database: %w", err)
	}
	return nil
}

func (db *DB) InsertGamePublisher(ctx context.Context, appID int, publisher string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_publishers (app_id, publisher)
			VALUES ($1, $2)
		ON CONFLICT (app_id, publisher) DO NOTHING
		`,
		appID, publisher,
	)
	if err != nil {
		log.Printf("Failed to insert game publisher into database: %v", err)
		return fmt.Errorf("failed to insert game publisher into database: %w", err)
	}
	return nil
}

func (db *DB) InsertGenre(ctx context.Context, genre string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO genres (genre)
			VALUES ($1)
		ON CONFLICT (genre) DO NOTHING
		`,
		genre,
	)
	if err != nil {
		log.Printf("Failed to insert genre into database: %v", err)
		return fmt.Errorf("failed to insert genre into database: %w", err)
	}
	return nil
}

func (db *DB) InsertGameGenre(ctx context.Context, appID int, genre string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_genres (app_id, genre)
			VALUES ($1, $2)
		ON CONFLICT (app_id, genre) DO NOTHING
		`,
		appID, genre,
	)
	if err != nil {
		log.Printf("Failed to insert game genre into database: %v", err)
		return fmt.Errorf("failed to insert game genre into database: %w", err)
	}
	return nil
}

func (db *DB) InsertTag(ctx context.Context, tag string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO tags (tag)
			VALUES ($1)
		ON CONFLICT (tag) DO NOTHING
		`,
		tag,
	)
	if err != nil {
		log.Printf("Failed to insert tag into database: %v", err)
		return fmt.Errorf("failed to insert tag into database: %w", err)
	}
	return nil
}

func (db *DB) InsertGameTag(ctx context.Context, appID int, tag string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_tags (app_id, tag)
			VALUES ($1, $2)
		ON CONFLICT (app_id, tag) DO NOTHING
		`,
		appID, tag,
	)
	if err != nil {
		log.Printf("Failed to insert game tag into database: %v", err)
		return fmt.Errorf("failed to insert game tag into database: %w", err)
	}
	return nil
}