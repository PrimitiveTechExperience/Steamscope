package scraper

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PrimitiveTechExperience/Steamscope/internal/models"
	"github.com/PuerkitoBio/goquery"
)

func parseGamePage(doc *goquery.Selection, appID int, url string) models.Game {
	game := models.Game{
		AppID: appID,
		URL:   url,
	}

	game.Name = strings.TrimSpace(doc.Find(".apphub_AppName").First().Text())
	game.Developer = strings.TrimSpace(doc.Find(".dev_row .summary.column a").First().Text())
	game.Publisher = strings.TrimSpace(doc.Find(".dev_row .summary.column a").Eq(1).Text())
	game.ReleaseDate = strings.TrimSpace(doc.Find(".date").First().Text())
	// Check if game is on discount by basing off the existance of .game_purchas_price or .discount_final_price
	if doc.Find(".discount_final_price").Length() > 0 {
		game.Price = strings.TrimSpace(doc.Find(".discount_final_price").First().Text())
		game.OriginalPrice = strings.TrimSpace(doc.Find(".discount_original_price").First().Text())
	}else{
		game.Price = strings.TrimSpace(doc.Find(".game_purchase_price").First().Text())
		game.OriginalPrice = game.Price
	}
	game.DiscountPercentage = parseDiscountPercentage(doc.Find(".discount_pct").First().Text())
	game.Genres = []string{}
	doc.Find(".details_block a[href*='/genre/']").Each(func(i int, s *goquery.Selection) {
		game.Genres = append(game.Genres, strings.TrimSpace(s.Text()))
	})
	game.Tags = []string{}
	doc.Find(".glance_tags.popular_tags a").Each(func(i int, s *goquery.Selection) {
		game.Tags = append(game.Tags, strings.TrimSpace(s.Text()))
	})
	// fetch review score and review count (count is in (number,number,number), so we need to remove the parentheses and commas and convert to int)
	reviewCountText := strings.TrimSpace(doc.Find(".responsive_hidden").Eq(1).Text())
	// fmt.Printf("Review Count Text: %s\n", doc.Find(".responsive_hidden").Eq(1).Text())
	// fmt.Printf("Review count text: %s\n", reviewCountText)
	reviewCountText = strings.TrimPrefix(reviewCountText, "(")
	reviewCountText = strings.TrimSuffix(reviewCountText, ")")
	reviewCountText = strings.ReplaceAll(reviewCountText, ",", "")
	game.ReviewCount, _ = strconv.Atoi(reviewCountText)
	// Need to fetch from second or third instance of .user_reviews_summary_row as html layout is odd.
	game.ReviewScore = strings.TrimSpace(doc.Find(".user_reviews_summary_row").Eq(1).Find(".game_review_summary").First().Text())

	game.Description = strings.TrimSpace(doc.Find(".game_area_description").First().Text())
	game.WindowsCompatible = doc.Find(".sysreq_tabs [data-os='win']").Length() > 0
	game.LinuxCompatible = doc.Find(".sysreq_tabs [data-os='linux']").Length() > 0
	game.MacCompatible = doc.Find(".sysreq_tabs [data-os='mac']").Length() > 0

	return game
}

func parseAppIDFromURL(url string) (int, error) {
	parts := strings.Split(url, "/")
	for i, part := range parts {
		if part == "app" && i+1 < len(parts) {
			appID, err := strconv.Atoi(parts[i+1])
			if err != nil {
				return 0, err
			}
			return appID, nil
		}
	}
	return 0, fmt.Errorf("app ID not found in URL: %s", url)
}

func parseDiscountPercentage(discountText string) int {
	discountText = strings.TrimSpace(discountText)
	if discountText == "" {
		return 0
	}
	discountText = strings.TrimPrefix(discountText, "-")
	discountText = strings.TrimSuffix(discountText, "%")
	discount, err := strconv.Atoi(discountText)
	if err != nil {
		return 0
	}
	return discount
}