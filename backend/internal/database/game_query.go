package database

import (
	"context"
	"fmt"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) GetGame(ctx context.Context, appID int) (*models.Game, error) {
	var game models.Game
	var score int
	err := db.Pool.QueryRow(ctx, `
	SELECT app_id, name, url, description, release_date, price, original_price, discount_percentage, review_score, review_count, windows_compatible, mac_compatible, linux_compatible
	FROM games
	WHERE app_id = $1
	`, appID).Scan(
		&game.AppID,
		&game.Name,
		&game.URL,
		&game.Description,
		&game.ReleaseDate,
		&game.Price,
		&game.OriginalPrice,
		&game.DiscountPercentage,
		&score,
		&game.ReviewCount,
		&game.WindowsCompatible,
		&game.MacCompatible,
		&game.LinuxCompatible,
	)
	game.ReviewScore = GetReviewScoreDescription(score)
	if err != nil {
		return nil, fmt.Errorf("failed to get game with app_id %d: %w", appID, err)
	}
	game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game developers: %w", err)
	}
	game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game publishers: %w", err)
	}
	game.Tags, err = db.getGameTags(ctx, game.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game tags: %w", err)
	}
	game.Genres, err = db.getGameGenres(ctx, game.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game genres: %w", err)
	}
	game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game languages: %w", err)
	}
	game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 1) // Fetch the first 10 reviews for the game
	if err != nil {
		return nil, fmt.Errorf("failed to get reviews for game with app_id %d: %w", appID, err)
	}
	return &game, nil
}

func (db *DB) GetGames(ctx context.Context, appIDs []int, page int) ([]models.Game, error) {
	query := `
	SELECT app_id, name, url, description, release_date, price, original_price, discount_percentage, review_score, review_count, windows_compatible, mac_compatible, linux_compatible
	FROM games
	WHERE app_id = ANY($1)
	LIMIT 100 OFFSET $2
	`
	rows, err := db.Pool.Query(ctx, query, appIDs, (page-1)*100)
	if err != nil {
		return nil, fmt.Errorf("failed to get games: %w", err)
	}
	defer rows.Close()
	var score int
	var games []models.Game
	for rows.Next() {
		var game models.Game
		err := rows.Scan(
			&game.AppID,
			&game.Name,
			&game.URL,
			&game.Description,
			&game.ReleaseDate,
			&game.Price,
			&game.OriginalPrice,
			&game.DiscountPercentage,
			&score,
			&game.ReviewCount,
			&game.WindowsCompatible,
			&game.MacCompatible,
			&game.LinuxCompatible,
		)
		game.ReviewScore = GetReviewScoreDescription(score)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game developers: %w", err)
		}
		game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game publishers: %w", err)
		}
		game.Tags, err = db.getGameTags(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game tags: %w", err)
		}
		game.Genres, err = db.getGameGenres(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game genres: %w", err)
		}
		game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game languages: %w", err)
		}
		game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 1) // Fetch the first 10 reviews for the game
		if err != nil {
			return nil, fmt.Errorf("failed to get reviews for game with app_id %d: %w", game.AppID, err)
		}
		games = append(games, game)
	}

	return games, nil
}

func (db *DB) GetCountOfGames(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
	SELECT COUNT(*)
	FROM games
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count of games: %w", err)
	}
	return count, nil
}

func (db *DB) GetGamesByGenre(ctx context.Context, genre string, page int) ([]models.Game, error) {
	query := `
	SELECT g.app_id, g.name, g.url, g.description, g.release_date, g.price, g.original_price, g.discount_percentage, g.review_score, g.review_count, g.windows_compatible, g.mac_compatible, g.linux_compatible
	FROM games g
	JOIN game_genres gg ON g.app_id = gg.app_id
	JOIN genres ge ON gg.genre_id = ge.genre_id
	WHERE ge.genre = $1
	LIMIT 100 OFFSET $2
	`
	rows, err := db.Pool.Query(ctx, query, genre, (page-1)*100)
	if err != nil {
		return nil, fmt.Errorf("failed to get games by genre: %w", err)
	}
	defer rows.Close()
	var score int
	var games []models.Game
	for rows.Next() {
		var game models.Game
		err := rows.Scan(
			&game.AppID,
			&game.Name,
			&game.URL,
			&game.Description,
			&game.ReleaseDate,
			&game.Price,
			&game.OriginalPrice,
			&game.DiscountPercentage,
			&score,
			&game.ReviewCount,
			&game.WindowsCompatible,
			&game.MacCompatible,
			&game.LinuxCompatible,
		)
		game.ReviewScore = GetReviewScoreDescription(score)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game developers: %w", err)
		}
		game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game publishers: %w", err)
		}
		game.Tags, err = db.getGameTags(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game tags: %w", err)
		}
		game.Genres, err = db.getGameGenres(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game genres: %w", err)
		}
		game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game languages: %w", err)
		}
		game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 1) // Fetch the first 10 reviews for the game
		if err != nil {
			return nil, fmt.Errorf("failed to get reviews for game with app_id %d: %w", game.AppID, err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (db *DB) GetCountOfGamesByGenre(ctx context.Context, genre string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
	SELECT COUNT(*)
	FROM games g
	JOIN game_genres gg ON g.app_id = gg.app_id
	JOIN genres ge ON gg.genre_id = ge.genre_id
	WHERE ge.genre = $1
	`, genre).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count of games by genre: %w", err)
	}
	return count, nil
}

func (db *DB) GetGamesByTag(ctx context.Context, tag string, page int) ([]models.Game, error) {
	query := `
	SELECT g.app_id, g.name, g.url, g.description, g.release_date, g.price, g.original_price, g.discount_percentage, g.review_score, g.review_count, g.windows_compatible, g.mac_compatible, g.linux_compatible
	FROM games g
	JOIN game_tags gt ON g.app_id = gt.app_id
	JOIN tags t ON gt.tag_id = t.tag_id
	WHERE t.tag = $1
	LIMIT 100 OFFSET $2
	`
	rows, err := db.Pool.Query(ctx, query, tag, (page-1)*100)
	if err != nil {
		return nil, fmt.Errorf("failed to get games by tag: %w", err)
	}
	defer rows.Close()
	var score int
	var games []models.Game
	for rows.Next() {
		var game models.Game
		err := rows.Scan(
			&game.AppID,
			&game.Name,
			&game.URL,
			&game.Description,
			&game.ReleaseDate,
			&game.Price,
			&game.OriginalPrice,
			&game.DiscountPercentage,
			&score,
			&game.ReviewCount,
			&game.WindowsCompatible,
			&game.MacCompatible,
			&game.LinuxCompatible,
		)
		game.ReviewScore = GetReviewScoreDescription(score)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game developers: %w", err)
		}
		game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game publishers: %w", err)
		}
		game.Tags, err = db.getGameTags(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game tags: %w", err)
		}
		game.Genres, err = db.getGameGenres(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game genres: %w", err)
		}
		game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game languages: %w", err)
		}
		game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 1) // Fetch the first 10 reviews for the game
		if err != nil {
			return nil, fmt.Errorf("failed to get reviews for game with app_id %d: %w", game.AppID, err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (db *DB) GetCountOfGamesByTag(ctx context.Context, tag string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
	SELECT COUNT(*)
	FROM games g
	JOIN game_tags gt ON g.app_id = gt.app_id
	JOIN tags t ON gt.tag_id = t.tag_id
	WHERE t.tag = $1
	`, tag).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count of games by tag: %w", err)
	}
	return count, nil
}

func (db *DB) GetGamesByDeveloper(ctx context.Context, developer string, page int) ([]models.Game, error) {
	query := `
	SELECT g.app_id, g.name, g.url, g.description, g.release_date, g.price, g.original_price, g.discount_percentage, g.review_score, g.review_count, g.windows_compatible, g.mac_compatible, g.linux_compatible
	FROM games g
	JOIN game_developers gd ON g.app_id = gd.app_id
	JOIN developers d ON gd.developer_id = d.developer_id
	WHERE d.developer = $1
	LIMIT 100 OFFSET $2
	`
	rows, err := db.Pool.Query(ctx, query, developer, (page-1)*100)
	if err != nil {
		return nil, fmt.Errorf("failed to get games by developer: %w", err)
	}
	defer rows.Close()
	var score int
	var games []models.Game
	for rows.Next() {
		var game models.Game
		err := rows.Scan(
			&game.AppID,
			&game.Name,
			&game.URL,
			&game.Description,
			&game.ReleaseDate,
			&game.Price,
			&game.OriginalPrice,
			&game.DiscountPercentage,
			&score,
			&game.ReviewCount,
			&game.WindowsCompatible,
			&game.MacCompatible,
			&game.LinuxCompatible,
		)
		game.ReviewScore = GetReviewScoreDescription(score)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game developers: %w", err)
		}
		game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game publishers: %w", err)
		}
		game.Tags, err = db.getGameTags(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game tags: %w", err)
		}
		game.Genres, err = db.getGameGenres(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game genres: %w", err)
		}
		game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game languages: %w", err)
		}
		game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 1) // Fetch the first 10 reviews for the game
		if err != nil {
			return nil, fmt.Errorf("failed to get reviews for game with app_id %d: %w", game.AppID, err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (db *DB) GetCountOfGamesByDeveloper(ctx context.Context, developer string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
	SELECT COUNT(*)
	FROM games g
	JOIN game_developers gd ON g.app_id = gd.app_id
	JOIN developers d ON gd.developer_id = d.developer_id
	WHERE d.developer = $1
	`, developer).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count of games by developer: %w", err)
	}
	return count, nil
}

func (db *DB) GetGamesByPublisher(ctx context.Context, publisher string, page int) ([]models.Game, error) {
	query := `
	SELECT g.app_id, g.name, g.url, g.description, g.release_date, g.price, g.original_price, g.discount_percentage, g.review_score, g.review_count, g.windows_compatible, g.mac_compatible, g.linux_compatible
	FROM games g
	JOIN game_publishers gp ON g.app_id = gp.app_id
	JOIN publishers p ON gp.publisher_id = p.publisher_id
	WHERE p.publisher = $1
	LIMIT 100 OFFSET $2
	`
	rows, err := db.Pool.Query(ctx, query, publisher, (page-1)*100)
	if err != nil {
		return nil, fmt.Errorf("failed to get games by publisher: %w", err)
	}
	defer rows.Close()
	var score int
	var games []models.Game
	for rows.Next() {
		var game models.Game
		err := rows.Scan(
			&game.AppID,
			&game.Name,
			&game.URL,
			&game.Description,
			&game.ReleaseDate,
			&game.Price,
			&game.OriginalPrice,
			&game.DiscountPercentage,
			&score,
			&game.ReviewCount,
			&game.WindowsCompatible,
			&game.MacCompatible,
			&game.LinuxCompatible,
		)
		game.ReviewScore = GetReviewScoreDescription(score)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game developers: %w", err)
		}
		game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game publishers: %w", err)
		}
		game.Tags, err = db.getGameTags(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game tags: %w", err)
		}
		game.Genres, err = db.getGameGenres(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game genres: %w", err)
		}
		game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game languages: %w", err)
		}
		game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 1) // Fetch the first 10 reviews for the game
		if err != nil {
			return nil, fmt.Errorf("failed to get reviews for game with app_id %d: %w", game.AppID, err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (db *DB) GetCountOfGamesByPublisher(ctx context.Context, publisher string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
	SELECT COUNT(*)
	FROM games g
	JOIN game_publishers gp ON g.app_id = gp.app_id
	JOIN publishers p ON gp.publisher_id = p.publisher_id
	WHERE p.publisher = $1
	`, publisher).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count of games by publisher: %w", err)
	}
	return count, nil
}

func (db *DB) GetGamesByLanguage(ctx context.Context, language string, page int) ([]models.Game, error) {
	query := `
	SELECT g.app_id, g.name, g.url, g.description, g.release_date, g.price, g.original_price, g.discount_percentage, g.review_score, g.review_count, g.windows_compatible, g.mac_compatible, g.linux_compatible
	FROM games g
	JOIN game_languages gl ON g.app_id = gl.app_id
	JOIN languages l ON gl.language_id = l.language_id
	WHERE l.language = $1
	LIMIT 100 OFFSET $2
	`
	rows, err := db.Pool.Query(ctx, query, language, (page-1)*100)
	if err != nil {
		return nil, fmt.Errorf("failed to get games by language: %w", err)
	}
	defer rows.Close()
	var score int
	var games []models.Game
	for rows.Next() {
		var game models.Game
		err := rows.Scan(
			&game.AppID,
			&game.Name,
			&game.URL,
			&game.Description,
			&game.ReleaseDate,
			&game.Price,
			&game.OriginalPrice,
			&game.DiscountPercentage,
			&score,
			&game.ReviewCount,
			&game.WindowsCompatible,
			&game.MacCompatible,
			&game.LinuxCompatible,
		)
		game.ReviewScore = GetReviewScoreDescription(score)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game developers: %w", err)
		}
		game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game publishers: %w", err)
		}
		game.Tags, err = db.getGameTags(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game tags: %w", err)
		}
		game.Genres, err = db.getGameGenres(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game genres: %w", err)
		}
		game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get game languages: %w", err)
		}
		game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 1) // Fetch the first 10 reviews for the game
		if err != nil {
			return nil, fmt.Errorf("failed to get reviews for game with app_id %d: %w", game.AppID, err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (db *DB) GetCountOfGamesByLanguage(ctx context.Context, language string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
	SELECT COUNT(*)
	FROM games g
	JOIN game_languages gl ON g.app_id = gl.app_id
	JOIN languages l ON gl.language_id = l.language_id
	WHERE l.language = $1
	`, language).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count of games by language: %w", err)
	}
	return count, nil
}

func (db *DB) getGameDevelopers(ctx context.Context, appID int) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
        SELECT d.developer
        FROM developers d
        JOIN game_developers gd ON gd.developer_id = d.developer_id
        WHERE gd.app_id = $1
    `, appID)
    if err != nil {
        return nil, fmt.Errorf("failed to get game developers for app_id %d: %w", appID, err)
    }
    defer rows.Close()

    var developers []string
    for rows.Next() {
        var developer string
        if err := rows.Scan(&developer); err != nil {
            return nil, fmt.Errorf("failed to scan developer for app_id %d: %w", appID, err)
        }
        developers = append(developers, developer)
    }

    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("failed to read developers for app_id %d: %w", appID, err)
    }
    return developers, nil
}


func (db *DB) getGamePublishers(ctx context.Context, appID int) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
        SELECT p.publisher
        FROM publishers p
        JOIN game_publishers gp ON gp.publisher_id = p.publisher_id
        WHERE gp.app_id = $1
    `, appID)
    if err != nil {
        return nil, fmt.Errorf("failed to get game publishers for app_id %d: %w", appID, err)
    }
    defer rows.Close()

    var publishers []string
    for rows.Next() {
        var publisher string
        if err := rows.Scan(&publisher); err != nil {
            return nil, fmt.Errorf("failed to scan publisher for app_id %d: %w", appID, err)
        }
        publishers = append(publishers, publisher)
    }

    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("failed to read publishers for app_id %d: %w", appID, err)
    }
    return publishers, nil
}

func (db *DB) getGameTags(ctx context.Context, appID int) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT t.tag
		FROM tags t
		JOIN game_tags gt ON gt.tag_id = t.tag_id
		WHERE gt.app_id = $1
	`, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game tags for app_id %d: %w", appID, err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("failed to scan tag for app_id %d: %w", appID, err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read tags for app_id %d: %w", appID, err)
	}
	return tags, nil
}

func (db *DB) getGameGenres(ctx context.Context, appID int) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT g.genre
		FROM genres g
		JOIN game_genres gg ON gg.genre_id = g.genre_id
		WHERE gg.app_id = $1
	`, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game genres for app_id %d: %w", appID, err)
	}
	defer rows.Close()

	var genres []string
	for rows.Next() {
		var genre string
		if err := rows.Scan(&genre); err != nil {
			return nil, fmt.Errorf("failed to scan genre for app_id %d: %w", appID, err)
		}
		genres = append(genres, genre)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read genres for app_id %d: %w", appID, err)
	}
	return genres, nil
}

func (db *DB) getGameLanguages(ctx context.Context, appID int) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT l.language
		FROM languages l
		JOIN game_languages gl ON gl.language_id = l.language_id
		WHERE gl.app_id = $1
	`, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game languages for app_id %d: %w", appID, err)
	}
	defer rows.Close()

	var languages []string
	for rows.Next() {
		var language string
		if err := rows.Scan(&language); err != nil {
			return nil, fmt.Errorf("failed to scan language for app_id %d: %w", appID, err)
		}
		languages = append(languages, language)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read languages for app_id %d: %w", appID, err)
	}
	return languages, nil
}

func GetReviewScoreDescription(score int) string {
	switch {
		case score == 0:
			return "No Reviews"
		case score == 1:
			return "Overwhelmingly Negative"
		case score == 2:
			return "Very Negative"
		case score == 3:
			return "Negative"
		case score == 4:
			return "Mostly Negative"
		case score == 5:
			return "Mixed"
		case score == 6:
			return "Mostly Positive"
		case score == 7:
			return "Positive"
		case score == 8:
			return "Very Positive"
		case score == 9:
			return "Overwhelmingly Positive"
		default:
			return "Unknown"
	}
}
