package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

// maxMatchedGames is how many of a bundle's matching games are named in a result.
const maxMatchedGames = 3

// SearchBundles finds tracked bundles for a search: those whose own name matches
// the text, and those that contain a game whose name does. Name matches come
// first, then the biggest discount.
//
// The price and discount filters apply to the bundle itself. Filters that
// describe a game (genre, developer and so on) cannot be judged for a bundle, so
// callers leave bundles out when any are set. Without search text it lists
// bundles by discount.
func (db *DB) SearchBundles(ctx context.Context, f models.GameFilters, limit int) ([]models.Bundle, error) {
	args := make([]any, 0, 12)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	where := []string{"b.status = 'tracked'"}
	matchedGames := "'{}'::text[]"
	order := []string{}

	if strings.TrimSpace(f.Search) != "" {
		if terms := SearchTerms(f.Search); len(terms) > 0 {
			q := prefixQuery(terms, addArg)
			where = append(where, fmt.Sprintf(`(b.search_vector @@ %[1]s OR EXISTS (
				SELECT 1 FROM bundle_games bg WHERE bg.bundle_id = b.bundle_id AND bg.name_tsv @@ %[1]s))`, q))
			matchedGames = fmt.Sprintf(`ARRAY(SELECT bg.name FROM bundle_games bg
				WHERE bg.bundle_id = b.bundle_id AND bg.name_tsv @@ %s ORDER BY bg.name LIMIT %d)`, q, maxMatchedGames)
			order = append(order, fmt.Sprintf("(b.search_vector @@ %s) DESC", q), fmt.Sprintf("ts_rank_cd(b.search_vector, %s) DESC", q))
		} else {
			// Only punctuation: fall back to matching the text as typed.
			like := addArg("%" + escapeLike(strings.TrimSpace(f.Search)) + "%")
			where = append(where, fmt.Sprintf(`(b.name ILIKE %[1]s OR EXISTS (
				SELECT 1 FROM bundle_games bg WHERE bg.bundle_id = b.bundle_id AND bg.name ILIKE %[1]s))`, like))
		}
	}
	if f.MinPrice > 0 {
		where = append(where, "b.price >= "+addArg(f.MinPrice))
	}
	if f.MaxPrice > 0 {
		where = append(where, "b.price <= "+addArg(f.MaxPrice))
	}
	if f.MinDiscount > 0 {
		where = append(where, "b.discount_percentage >= "+addArg(f.MinDiscount))
	}
	order = append(order, "b.discount_percentage DESC", "b.name")

	query := fmt.Sprintf(`SELECT %s, %s
		FROM bundles b
		WHERE %s
		ORDER BY %s
		LIMIT %s`, bundleColumns, matchedGames, strings.Join(where, " AND "), strings.Join(order, ", "), addArg(limit))

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search bundles: %w", err)
	}
	defer rows.Close()

	bundles := []models.Bundle{}
	for rows.Next() {
		var b models.Bundle
		if err := rows.Scan(&b.BundleID, &b.Name, &b.URL, &b.HeaderImage, &b.Price, &b.OriginalPrice,
			&b.DiscountPercentage, &b.Status, &b.UpdatedAt, &b.GameCount, &b.AtRecordLow, &b.MatchedGames); err != nil {
			return nil, fmt.Errorf("failed to scan bundle: %w", err)
		}
		b.Games = []models.BundleGame{}
		bundles = append(bundles, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := db.attachPreviewGames(ctx, bundles); err != nil {
		return nil, err
	}
	return bundles, nil
}
