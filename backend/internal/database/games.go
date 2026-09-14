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
		if err := db.InsertLanguage(ctx, language); err != nil {
			return fmt.Errorf("failed to insert language: %w", err)
		}
		if err := db.InsertGameLanguage(ctx, game.AppID, language); err != nil {
			return fmt.Errorf("failed to insert game language: %w", err)
		}
	}
	// Insert the developers into the developers table and the game_developers table
	for _, developer := range game.Developers {
		if err := db.InsertDeveloper(ctx, developer); err != nil {
			return fmt.Errorf("failed to insert developer: %w", err)
		}
		if err := db.InsertGameDeveloper(ctx, game.AppID, developer); err != nil {
			return fmt.Errorf("failed to insert game developer: %w", err)
		}
	}
	// Insert the publishers into the publishers table and the game_publishers table
	for _, publisher := range game.Publishers {
		if err := db.InsertPublisher(ctx, publisher); err != nil {
			return fmt.Errorf("failed to insert publisher: %w", err)
		}
		if err := db.InsertGamePublisher(ctx, game.AppID, publisher); err != nil {
			return fmt.Errorf("failed to insert game publisher: %w", err)
		}
	}
	// Insert the genres into the genres table and the game_genres table
	for _, genre := range game.Genres {
		if err := db.InsertGenre(ctx, genre); err != nil {
			return fmt.Errorf("failed to insert genre: %w", err)
		}
		if err := db.InsertGameGenre(ctx, game.AppID, genre); err != nil {
			return fmt.Errorf("failed to insert game genre: %w", err)
		}
	}
	// Insert the tags into the tags table and the game_tags table
	for _, tag := range game.Tags {
		if err := db.InsertTag(ctx, tag); err != nil {
			return fmt.Errorf("failed to insert tag: %w", err)
		}
		if err := db.InsertGameTag(ctx, game.AppID, tag); err != nil {
			return fmt.Errorf("failed to insert game tag: %w", err)
		}
	}
	// Insert the reviews into the reviews table
	for _, review := range game.Reviews {
		if err := db.InsertReview(ctx, review); err != nil {
			return fmt.Errorf("failed to insert review: %w", err)
		}
	}
	return nil
}

func (db *DB) InsertGameDetails(ctx context.Context, game models.Game) error {
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
		game.AppID, game.Name, game.URL, game.Description, game.ReleaseDate, game.Price, game.OriginalPrice, game.DiscountPercentage, game.ReviewScore, game.ReviewCount, game.WindowsCompatible, game.LinuxCompatible, game.MacCompatible,
	)
	if err != nil {
		log.Printf("Failed to insert game into database: %v", err)
		return fmt.Errorf("failed to insert game into database: %w", err)
	}
	return nil
}

func (db *DB) InsertLanguage(ctx context.Context, language string) error {
	_, err := db.Pool.Exec(
		ctx,
		`
		INSERT INTO languages (language)
			VALUES ($1)
		ON CONFLICT (language) DO UPDATE SET language = EXCLUDED.language
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
		ON CONFLICT (app_id, language) DO UPDATE SET language = EXCLUDED.language
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
		ON CONFLICT (developer) DO UPDATE SET developer = EXCLUDED.developer
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
		ON CONFLICT (app_id, developer) DO UPDATE SET developer = EXCLUDED.developer
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
		ON CONFLICT (publisher) DO UPDATE SET publisher = EXCLUDED.publisher
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
		ON CONFLICT (app_id, publisher) DO UPDATE SET publisher = EXCLUDED.publisher
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
		ON CONFLICT (genre) DO UPDATE SET genre = EXCLUDED.genre
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
		ON CONFLICT (app_id, genre) DO UPDATE SET genre = EXCLUDED.genre
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
		ON CONFLICT (tag) DO UPDATE SET tag = EXCLUDED.tag
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
		ON CONFLICT (app_id, tag) DO UPDATE SET tag = EXCLUDED.tag
		`,
		appID, tag,
	)
	if err != nil {
		log.Printf("Failed to insert game tag into database: %v", err)
		return fmt.Errorf("failed to insert game tag into database: %w", err)
	}
	return nil
}
