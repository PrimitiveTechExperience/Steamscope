package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) GetGame(ctx context.Context, appID int) (*models.Game, error) {
	var game models.Game
	var score int
	err := db.Pool.QueryRow(ctx, `
	SELECT app_id, name, url, description, header_image, release_date, price, original_price, discount_percentage, review_score, review_count, windows_compatible, mac_compatible, linux_compatible
	FROM games
	WHERE app_id = $1
	`, appID).Scan(
		&game.AppID, &game.Name, &game.URL, &game.Description, &game.HeaderImage, &game.ReleaseDate,
		&game.Price, &game.OriginalPrice, &game.DiscountPercentage, &score,
		&game.ReviewCount, &game.WindowsCompatible, &game.MacCompatible, &game.LinuxCompatible,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get game with app_id %d: %w", appID, err)
	}
	game.ReviewScore = GetReviewScoreDescription(score)
	if err := db.loadGameRelations(ctx, &game); err != nil {
		return nil, err
	}
	return &game, nil
}

func (db *DB) GetGames(ctx context.Context, filters models.GameFilters) ([]models.Game, error) {
	query, args := buildGameQuery(filters, false)
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get games: %w", err)
	}
	defer rows.Close()

	var games []models.Game
	for rows.Next() {
		var game models.Game
		var score int
		if err := rows.Scan(
			&game.AppID, &game.Name, &game.URL, &game.Description, &game.HeaderImage, &game.ReleaseDate,
			&game.Price, &game.OriginalPrice, &game.DiscountPercentage, &score,
			&game.ReviewCount, &game.WindowsCompatible, &game.MacCompatible, &game.LinuxCompatible,
		); err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		game.ReviewScore = GetReviewScoreDescription(score)
		if err := db.loadGameRelations(ctx, &game); err != nil {
			return nil, err
		}
		games = append(games, game)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read games: %w", err)
	}
	return games, nil
}

func (db *DB) GetGamesCount(ctx context.Context, filters models.GameFilters) (int, error) {
	query, args := buildGameQuery(filters, true)
	var count int
	if err := db.Pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to get games count: %w", err)
	}
	return count, nil
}

func buildGameQuery(filters models.GameFilters, countOnly bool) (string, []any) {
	selectClause := `SELECT g.app_id, g.name, g.url, g.description, g.header_image, g.release_date, g.price, g.original_price, g.discount_percentage, g.review_score, g.review_count, g.windows_compatible, g.mac_compatible, g.linux_compatible`
	if countOnly {
		selectClause = "SELECT COUNT(*)"
	}
	query := []string{selectClause, "FROM games g", "WHERE 1 = 1"}
	args := make([]any, 0, 8)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filters.Search != "" {
		placeholder := addArg("%" + filters.Search + "%")
		query = append(query, "AND (g.name ILIKE "+placeholder+" OR g.description ILIKE "+placeholder+")")
	}
	if filters.Genre != "" {
		placeholder := addArg(filters.Genre)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_genres gg JOIN genres ge ON ge.genre_id = gg.genre_id WHERE gg.app_id = g.app_id AND ge.genre ILIKE "+placeholder+")")
	}
	if len(filters.Genres) > 0 {
		placeholder := addArg(filters.Genres)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_genres gg JOIN genres ge ON ge.genre_id = gg.genre_id WHERE gg.app_id = g.app_id AND ge.genre ILIKE ANY("+placeholder+"))")
	}
	if filters.Developer != "" {
		placeholder := addArg(filters.Developer)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_developers gd JOIN developers d ON d.developer_id = gd.developer_id WHERE gd.app_id = g.app_id AND d.developer ILIKE "+placeholder+")")
	}
	if filters.Publisher != "" {
		placeholder := addArg(filters.Publisher)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_publishers gp JOIN publishers p ON p.publisher_id = gp.publisher_id WHERE gp.app_id = g.app_id AND p.publisher ILIKE "+placeholder+")")
	}
	if len(filters.Developers) > 0 {
		placeholder := addArg(filters.Developers)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_developers gd JOIN developers d ON d.developer_id = gd.developer_id WHERE gd.app_id = g.app_id AND d.developer ILIKE ANY("+placeholder+"))")
	}
	if len(filters.Publishers) > 0 {
		placeholder := addArg(filters.Publishers)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_publishers gp JOIN publishers p ON p.publisher_id = gp.publisher_id WHERE gp.app_id = g.app_id AND p.publisher ILIKE ANY("+placeholder+"))")
	}
	if len(filters.Tags) > 0 {
		placeholder := addArg(filters.Tags)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_tags gt JOIN tags t ON t.tag_id = gt.tag_id WHERE gt.app_id = g.app_id AND t.tag ILIKE ANY("+placeholder+"))")
	}
	if len(filters.Languages) > 0 {
		placeholder := addArg(filters.Languages)
		query = append(query, "AND EXISTS (SELECT 1 FROM game_languages gl JOIN languages l ON l.language_id = gl.language_id WHERE gl.app_id = g.app_id AND l.language ILIKE ANY("+placeholder+"))")
	}
	if filters.MinPrice > 0 {
		query = append(query, "AND g.price >= "+addArg(filters.MinPrice))
	}
	if filters.MaxPrice > 0 {
		query = append(query, "AND g.price <= "+addArg(filters.MaxPrice))
	}
	if !countOnly {
		query = append(query, "ORDER BY g.name")
		if filters.Limit > 0 {
			query = append(query, "LIMIT "+addArg(filters.Limit))
		}
		if filters.Offset > 0 {
			query = append(query, "OFFSET "+addArg(filters.Offset))
		}
	}
	return strings.Join(query, "\n"), args
}

func (db *DB) loadGameRelations(ctx context.Context, game *models.Game) error {
	var err error
	game.Developers, err = db.getGameDevelopers(ctx, game.AppID)
	if err != nil {
		return fmt.Errorf("failed to get game developers: %w", err)
	}
	game.Publishers, err = db.getGamePublishers(ctx, game.AppID)
	if err != nil {
		return fmt.Errorf("failed to get game publishers: %w", err)
	}
	game.Tags, err = db.getGameTags(ctx, game.AppID)
	if err != nil {
		return fmt.Errorf("failed to get game tags: %w", err)
	}
	game.Genres, err = db.getGameGenres(ctx, game.AppID)
	if err != nil {
		return fmt.Errorf("failed to get game genres: %w", err)
	}
	game.SupportedLanguages, err = db.getGameLanguages(ctx, game.AppID)
	if err != nil {
		return fmt.Errorf("failed to get game languages: %w", err)
	}
	game.Reviews, err = db.GetReviews(ctx, game.AppID, 10, 0)
	if err != nil {
		return fmt.Errorf("failed to get reviews for game with app_id %d: %w", game.AppID, err)
	}
	return nil
}

func (db *DB) getGameDevelopers(ctx context.Context, appID int) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `SELECT d.developer FROM developers d JOIN game_developers gd ON gd.developer_id = d.developer_id WHERE gd.app_id = $1`, appID)
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
	rows, err := db.Pool.Query(ctx, `SELECT p.publisher FROM publishers p JOIN game_publishers gp ON gp.publisher_id = p.publisher_id WHERE gp.app_id = $1`, appID)
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
	rows, err := db.Pool.Query(ctx, `SELECT t.tag FROM tags t JOIN game_tags gt ON gt.tag_id = t.tag_id WHERE gt.app_id = $1`, appID)
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
	rows, err := db.Pool.Query(ctx, `SELECT g.genre FROM genres g JOIN game_genres gg ON gg.genre_id = g.genre_id WHERE gg.app_id = $1`, appID)
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
	rows, err := db.Pool.Query(ctx, `SELECT l.language FROM languages l JOIN game_languages gl ON gl.language_id = l.language_id WHERE gl.app_id = $1`, appID)
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
