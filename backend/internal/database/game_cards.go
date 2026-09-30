package database

import (
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// gameCardColumns is the subset of games needed to render a game card,
// without the per-game relation queries GetGame does (reviews, tags, ...).
// Prefix-qualified with "g." so it can be used in joins.
const gameCardColumns = `g.app_id, g.name, g.url, g.description, g.header_image, g.release_date,
	g.price, g.original_price, g.discount_percentage, g.review_score, g.review_count,
	g.windows_compatible, g.mac_compatible, g.linux_compatible`

// scanGameCard scans gameCardColumns plus any extra trailing destinations.
func scanGameCard(row pgx.Row, extra ...any) (models.Game, error) {
	var g models.Game
	var score int
	dest := []any{
		&g.AppID, &g.Name, &g.URL, &g.Description, &g.HeaderImage, &g.ReleaseDate,
		&g.Price, &g.OriginalPrice, &g.DiscountPercentage, &score, &g.ReviewCount,
		&g.WindowsCompatible, &g.MacCompatible, &g.LinuxCompatible,
	}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return g, err
	}
	g.ReviewScore = GetReviewScoreDescription(score)
	g.Developers, g.Publishers, g.Genres, g.Tags, g.SupportedLanguages = []string{}, []string{}, []string{}, []string{}, []string{}
	g.Reviews = []models.Review{}
	return g, nil
}
