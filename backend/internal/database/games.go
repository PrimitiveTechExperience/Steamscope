package database

import (
	"context"
	"fmt"
	"log"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) InsertGame(ctx context.Context, game models.Game) error {
	// Insert the game details into the games table
	if err := db.InsertGameDetails(ctx, game); err != nil {
		return fmt.Errorf("failed to insert game details: %w", err)
	}
	// Insert the supported languages into the languages table and the game_languages table
	for _, language := range game.SupportedLanguages {
		languageID, err := db.InsertLanguage(ctx, language)
		if err != nil {
			return fmt.Errorf("failed to insert language: %w", err)
		}
		if err := db.InsertGameLanguage(ctx, game.AppID, languageID); err != nil {
			return fmt.Errorf("failed to insert game language: %w", err)
		}
	}
	// Insert the developers into the developers table and the game_developers table
	for _, developer := range game.Developers {
		developerID, err := db.InsertDeveloper(ctx, developer)
		if err != nil {
			return fmt.Errorf("failed to insert developer: %w", err)
		}
		if err := db.InsertGameDeveloper(ctx, game.AppID, developerID); err != nil {
			return fmt.Errorf("failed to insert game developer: %w", err)
		}
	}
	
	// Insert the publishers into the publishers table and the game_publishers table
	for _, publisher := range game.Publishers {
		publisherID, err := db.InsertPublisher(ctx, publisher)
		if err != nil {
			return fmt.Errorf("failed to insert publisher: %w", err)
		}
		if err := db.InsertGamePublisher(ctx, game.AppID, publisherID); err != nil {
			return fmt.Errorf("failed to insert game publisher: %w", err)
		}
	}
	// Insert the genres into the genres table and the game_genres table
	for _, genre := range game.Genres {
		genreID, err := db.InsertGenre(ctx, genre)
		if err != nil {
			return fmt.Errorf("failed to insert genre: %w", err)
		}
		if err := db.InsertGameGenre(ctx, game.AppID, genreID); err != nil {
			return fmt.Errorf("failed to insert game genre: %w", err)
		}
	}
	// Insert the tags into the tags table and the game_tags table
	for _, tag := range game.Tags {
		tagID, err := db.InsertTag(ctx, tag)
		if err != nil {
			return fmt.Errorf("failed to insert tag: %w", err)
		}
		if err := db.InsertGameTag(ctx, game.AppID, tagID); err != nil {
			return fmt.Errorf("failed to insert game tag: %w", err)
		}
	}
	// Insert the reviews into the reviews table
	for _, review := range game.Reviews {
		if err := db.InsertReview(ctx, review); err != nil {
			return fmt.Errorf("failed to insert review: %w", err)
		}
	}
	// Prune the reviews for the game to keep only the most recent 10 reviews
	if err := db.PruneReviews(ctx, game.AppID, 10); err != nil {
		return fmt.Errorf("failed to prune reviews: %w", err)
	}
	return nil
}

func (db *DB) InsertGameDetails(ctx context.Context, game models.Game) error {
	// Convert review score from string to int2 for storage in the database
	// index:
	// | `review_score` | Steam description       |
	// | -------------: | ----------------------- |
	// |            `0` | No user reviews         |
	// |            `1` | Overwhelmingly Negative |
	// |            `2` | Very Negative           |
	// |            `3` | Negative                |
	// |            `4` | Mostly Negative         |
	// |            `5` | Mixed                   |
	// |            `6` | Mostly Positive         |
	// |            `7` | Positive                |
	// |            `8` | Very Positive           |
	// |            `9` | Overwhelmingly Positive |
	reviewScore := 0
	switch game.ReviewScore {
	case "Overwhelmingly Negative":
		reviewScore = 1
	case "Very Negative":
		reviewScore = 2
	case "Negative":
		reviewScore = 3
	case "Mostly Negative":
		reviewScore = 4
	case "Mixed":
		reviewScore = 5
	case "Mostly Positive":
		reviewScore = 6
	case "Positive":
		reviewScore = 7
	case "Very Positive":
		reviewScore = 8
	case "Overwhelmingly Positive":
		reviewScore = 9
	}

	
	_, err := db.Pool.Exec(
		ctx, 
		`
		INSERT INTO games (
			app_id, name, url, description, release_date, price, original_price, discount_percentage, review_score, review_count, windows_compatible, linux_compatible, mac_compatible 
		)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (app_id) DO UPDATE SET
			name = EXCLUDED.name,
			url = EXCLUDED.url,
			description = EXCLUDED.description,
			release_date = EXCLUDED.release_date,	
			price = EXCLUDED.price,
			original_price = EXCLUDED.original_price,
			discount_percentage = EXCLUDED.discount_percentage,
			review_score = EXCLUDED.review_score,
			review_count = EXCLUDED.review_count,
			windows_compatible = EXCLUDED.windows_compatible,
			linux_compatible = EXCLUDED.linux_compatible,
			mac_compatible = EXCLUDED.mac_compatible
		`,
		game.AppID, game.Name, game.URL, game.Description, game.ReleaseDate, game.Price, game.OriginalPrice, game.DiscountPercentage, reviewScore, game.ReviewCount, game.WindowsCompatible, game.LinuxCompatible, game.MacCompatible,
	)
	if err != nil {
		log.Printf("Failed to insert game into database: %v", err)
		return fmt.Errorf("failed to insert game into database: %w", err)
	}
	return nil
}

func (db *DB) InsertLanguage(ctx context.Context, language string) (int, error) {
	var language_id int
	err := db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO languages (language)
			VALUES ($1)
		ON CONFLICT (language) DO UPDATE SET language = EXCLUDED.language RETURNING language_id
		`,
		language,
	).Scan(&language_id)
	if err != nil {
		log.Printf("Failed to insert language into database: %v", err)
		return -1, fmt.Errorf("failed to insert language into database: %w", err)
	}
	return language_id, nil
}

func (db *DB) InsertGameLanguage(ctx context.Context, appID int, languageID int) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_languages (app_id, language_id)
			VALUES ($1, $2)
		ON CONFLICT (app_id, language_id) DO UPDATE SET language_id = EXCLUDED.language_id
		`,
		appID, languageID,
	)
	if err != nil {
		log.Printf("Failed to insert game language into database: %v", err)
		return fmt.Errorf("failed to insert game language into database: %w", err)
	}
	return nil
}
func (db *DB) InsertDeveloper(ctx context.Context, developer string) (int, error) {
	var developer_id int
	err := db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO developers (developer)
			VALUES ($1)
		ON CONFLICT (developer) DO UPDATE SET developer = EXCLUDED.developer RETURNING developer_id
		`,
		developer,
	).Scan(&developer_id)
	if err != nil {
		log.Printf("Failed to insert developer into database: %v", err)
		return -1, fmt.Errorf("failed to insert developer into database: %w", err)
	}
	return developer_id, nil
}

func (db *DB) InsertGameDeveloper(ctx context.Context, appID int, developerID int) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_developers (app_id, developer_id)
			VALUES ($1, $2)
		ON CONFLICT (app_id, developer_id) DO UPDATE SET developer_id = EXCLUDED.developer_id
		`,
		appID, developerID,
	)
	if err != nil {
		log.Printf("Failed to insert game developer into database: %v", err)
		return fmt.Errorf("failed to insert game developer into database: %w", err)
	}
	return nil
}
func (db *DB) InsertPublisher(ctx context.Context, publisher string) (int, error) {
	var publisher_id int
	err := db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO publishers (publisher)
			VALUES ($1)
		ON CONFLICT (publisher) DO UPDATE SET publisher = EXCLUDED.publisher RETURNING publisher_id
		`,
		publisher,
	).Scan(&publisher_id)
	if err != nil {
		log.Printf("Failed to insert publisher into database: %v", err)
		return -1, fmt.Errorf("failed to insert publisher into database: %w", err)
	}
	return publisher_id, nil
}

func (db *DB) InsertGamePublisher(ctx context.Context, appID int, publisherID int) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_publishers (app_id, publisher_id)
			VALUES ($1, $2)
		ON CONFLICT (app_id, publisher_id) DO UPDATE SET publisher_id = EXCLUDED.publisher_id
		`,
		appID, publisherID,
	)
	if err != nil {
		log.Printf("Failed to insert game publisher into database: %v", err)
		return fmt.Errorf("failed to insert game publisher into database: %w", err)
	}
	return nil
}
func (db *DB) InsertGenre(ctx context.Context, genre string) (int, error) {
	var genre_id int
	err := db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO genres (genre)
			VALUES ($1)
		ON CONFLICT (genre) DO UPDATE SET genre = EXCLUDED.genre RETURNING genre_id
		`,
		genre,
	).Scan(&genre_id)
	if err != nil {
		log.Printf("Failed to insert genre into database: %v", err)
		return -1, fmt.Errorf("failed to insert genre into database: %w", err)
	}
	return genre_id, nil
}

func (db *DB) InsertGameGenre(ctx context.Context, appID int, genreID int) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_genres (app_id, genre_id)
			VALUES ($1, $2)
		ON CONFLICT (app_id, genre_id) DO UPDATE SET genre_id = EXCLUDED.genre_id
		`,
		appID, genreID,
	)
	if err != nil {
		log.Printf("Failed to insert game genre into database: %v", err)
		return fmt.Errorf("failed to insert game genre into database: %w", err)
	}
	return nil
}
func (db *DB) InsertTag(ctx context.Context, tag string) (int, error) {
	var tag_id int
	err := db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO tags (tag)
			VALUES ($1)
		ON CONFLICT (tag) DO UPDATE SET tag = EXCLUDED.tag RETURNING tag_id
		`,
		tag,
	).Scan(&tag_id)
	if err != nil {
		log.Printf("Failed to insert tag into database: %v", err)
		return -1, fmt.Errorf("failed to insert tag into database: %w", err)
	}
	return tag_id, nil
}

func (db *DB) InsertGameTag(ctx context.Context, appID int, tagID int) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO game_tags (app_id, tag_id)
			VALUES ($1, $2)
		ON CONFLICT (app_id, tag_id) DO UPDATE SET tag_id = EXCLUDED.tag_id
		`,
		appID, tagID,
	)
	if err != nil {
		log.Printf("Failed to insert game tag into database: %v", err)
		return fmt.Errorf("failed to insert game tag into database: %w", err)
	}
	return nil
}
