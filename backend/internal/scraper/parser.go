package scraper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PuerkitoBio/goquery"
)

func parseGamePage(doc *goquery.Selection, appID int, url string) models.Game {
	game := models.Game{
		AppID: appID,
		URL:   url,
	}

	game.Name = strings.TrimSpace(doc.Find(".apphub_AppName").First().Text())
	// Devs and publishers are distinguished from the dev_row class, where 1 is devs and 2 is publishers.
	// Each dev_row has a "summary column" class, which contains the devs/publishers as <a> tags.
	game.Developers = []string{}
	doc.Find(".dev_row").Eq(0).Find(".summary.column a").Each(func(i int, s *goquery.Selection) {
		game.Developers = append(game.Developers, strings.TrimSpace(s.Text()))
	})
	game.Publishers = []string{}
	doc.Find(".dev_row").Eq(1).Find(".summary.column a").Each(func(i int, s *goquery.Selection) {
		game.Publishers = append(game.Publishers, strings.TrimSpace(s.Text()))
	})
	// Convert ReleaseDate to time.Time
	releaseDateStr := strings.TrimSpace(doc.Find(".date").First().Text())
	if releaseDate, err := time.Parse("2 Jan, 2006", releaseDateStr); err == nil {
		game.ReleaseDate = releaseDate
	}
	// The purchase area can contain multiple ".game_area_purchase_game" blocks:
	// the game's own purchase/play section, plus promotional "buy as a bundle"
	// sections (e.g. class "game_area_purchase_game_dropdown_subscription", or
	// a "Buy The Orange Box"-style upsell) that advertise the game packaged
	// with other titles at a different price. The game's own section is always
	// the first ".game_area_purchase_game" block on the page, with any bundle
	// upsells appearing after it, so price selectors must be scoped to it to
	// avoid picking up a bundle's price.
	purchaseSection := doc.Find(".game_area_purchase_game").Not(".game_area_purchase_game_dropdown_subscription").First()
	if purchaseSection.Length() == 0 {
		// No recognizable purchase container (e.g. in tests, or a page layout
		// change) - fall back to searching the whole document like before.
		purchaseSection = doc
	}
	// Check if game is on discount by basing off the existance of .game_purchas_price or .discount_final_price
	if purchaseSection.Find(".discount_final_price").Length() > 0 {
		// Remove currency symbols and convert to float64
		priceStr := strings.TrimSpace(purchaseSection.Find(".discount_final_price").First().Text())
		originalPriceStr := strings.TrimSpace(purchaseSection.Find(".discount_original_price").First().Text())
		discountPrice, _ := parsePrice(priceStr)
		originalPrice, _ := parsePrice(originalPriceStr)
		game.Price = discountPrice
		game.OriginalPrice = originalPrice
		game.DiscountPercentage = parseDiscountPercentage(purchaseSection.Find(".discount_pct").First().Text())
	}else{
		priceStr := strings.TrimSpace(purchaseSection.Find(".game_purchase_price").First().Text())
		price, _ := parsePrice(priceStr)
		game.Price = price
		game.OriginalPrice = price
		game.DiscountPercentage = 0
	}
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
	// Keep the description as HTML (headings, paragraphs, bold text, images,
	// lists) rather than flattening it to plain text, so the frontend can
	// render it the way Steam's own store page does.
	descriptionHTML, _ := doc.Find("#game_area_description").First().Html()
	game.Description = strings.TrimSpace(descriptionHTML)
	game.HeaderImage, _ = doc.Find(".game_header_image_full").First().Attr("src")
	if game.HeaderImage == "" {
		// Some page variants (layout experiments, interstitial banners, etc.)
		// omit .game_header_image_full. Steam's header image otherwise lives
		// at a stable, predictable CDN path, so fall back to it rather than
		// leaving the game with no image at all.
		game.HeaderImage = fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%d/header.jpg", appID)
	}
	game.WindowsCompatible = doc.Find(".sysreq_tabs [data-os='win']").Length() > 0
	game.LinuxCompatible = doc.Find(".sysreq_tabs [data-os='linux']").Length() > 0
	game.MacCompatible = doc.Find(".sysreq_tabs [data-os='mac']").Length() > 0

	// Fetch supported languages from .game_language_options .ellipsis
	game.SupportedLanguages = []string{}
	doc.Find(".game_language_options .ellipsis").Each(func(i int, s *goquery.Selection) {
		game.SupportedLanguages = append(game.SupportedLanguages, strings.TrimSpace(s.Text()))
	})

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

func parsePrice(priceText string) (float64, string) {
    priceText = strings.TrimSpace(priceText)

    re := regexp.MustCompile(`^\s*([^\d]*?)\s*(\d+(?:\.\d+)?)\s*([^\d]*)\s*$`)
    matches := re.FindStringSubmatch(priceText)
    if len(matches) != 4 {
        return 0, ""
    }

    price, err := strconv.ParseFloat(matches[2], 64)
    if err != nil {
        return 0, ""
    }

    currency := strings.TrimSpace(matches[1] + matches[3])
    return price, currency
}