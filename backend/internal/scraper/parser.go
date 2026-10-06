package scraper

import (
	"fmt"
	"math"
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
	game.ReleaseDate = parseReleaseDate(doc.Find(".release_date .date, .date").First().Text())
	// The purchase area can contain multiple ".game_area_purchase_game" blocks:
	// the game's own purchase/play section, plus promotional "buy as a bundle"
	// sections (e.g. class "game_area_purchase_game_dropdown_subscription", or
	// a "Buy The Orange Box"-style upsell) that advertise the game packaged
	// with other titles at a different price. The game's own section is always
	// the first ".game_area_purchase_game" block on the page, with any bundle
	// upsells appearing after it, so price selectors must be scoped to it to
	// avoid picking up a bundle's price.
	//
	// Only real purchasable packages carry an add_to_cart id; the demo
	// download block above the purchase area (which has no price - Persona 3
	// Reload scraped as $0 because of it) and the bundle dropdowns do not.
	//
	// A free-to-play game has a "Free To Play" block first, and its page may
	// still list paid DLC or upgrade packages (CS2's Prime upgrade, TF2's
	// items) with add_to_cart ids after it. Taking the first add_to_cart block
	// outright gave those games the price of the upgrade.
	purchaseSection, free := findPurchaseSection(doc)
	if !free && !hasPrice(purchaseSection) {
		game.PriceUnknown = true
	}
	if free {
		game.Price, game.OriginalPrice, game.DiscountPercentage = 0, 0, 0
	} else {
		game.Price, game.OriginalPrice, game.DiscountPercentage = readPurchasePrices(purchaseSection)
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
	// The description is stored twice: as sanitized HTML (headings,
	// paragraphs, bold text, images, lists) for the game page to render the
	// way Steam's store page does, and as plain text for cards and search.
	descriptionSel := doc.Find("#game_area_description").First()
	game.DescriptionHTML = sanitizeDescriptionHTML(descriptionSel)
	game.Description = descriptionPlainText(descriptionSel)
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

// releaseDateLayouts are the formats Steam writes release dates in. Which one
// it uses depends on the store region (US pages say "Sep 2, 2026", others
// "2 Sep, 2026"). Month-only and year-only dates ("Sep 2026") resolve to the
// first day of that period.
var releaseDateLayouts = []string{
	"2 Jan, 2006", "2 January, 2006",
	"Jan 2, 2006", "January 2, 2006",
	"Jan 2006", "January 2006", "2006",
}

// parseReleaseDate returns the zero time for anything unparseable, such as
// "Coming soon" or "To be announced".
func parseReleaseDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range releaseDateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

// findPurchaseSection returns the game's own purchase block, and whether it is
// a "Free To Play" one. Blocks are looked at in page order and the first
// recognisable one wins: a free block, or a purchasable package.
func findPurchaseSection(doc *goquery.Selection) (section *goquery.Selection, free bool) {
	candidates := doc.Find(".game_area_purchase_game").Not(".game_area_purchase_game_dropdown_subscription, .demo_above_purchase")
	candidates.EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if isFreeSection(s) {
			section, free = s, true
			return false
		}
		if id, _ := s.Attr("id"); strings.HasPrefix(id, "game_area_purchase_section_add_to_cart") {
			section = s
			return false
		}
		return true
	})
	if section != nil {
		return section, free
	}
	if first := candidates.First(); first.Length() > 0 {
		return first, false
	}
	// No recognizable purchase container (e.g. in tests, or a page layout
	// change) - fall back to searching the whole document like before.
	return doc, false
}

// hasPrice is true when the block shows a price of any kind.
func hasPrice(s *goquery.Selection) bool {
	return s.Find(".discount_final_price, .game_purchase_price").Length() > 0
}

var freeLabel = regexp.MustCompile(`(?i)^free( to play)?$`)

// isFreeSection is true for the block Steam shows on a free-to-play game. A
// "free weekend" or a demo is not one: they carry no "Free To Play" price label.
func isFreeSection(s *goquery.Selection) bool {
	id, _ := s.Attr("aria-labelledby")
	if !strings.HasPrefix(id, "game_area_purchase_section_free") {
		return false
	}
	return freeLabel.MatchString(strings.TrimSpace(s.Find(".game_purchase_price").First().Text()))
}

// readPurchasePrices reads the price, the undiscounted price and the discount
// percentage from a purchase block.
//
// Steam states the discount twice: as text ("-50%") and as data-discount /
// data-price-final attributes on the same block. The text was the only thing
// read before, so a block without it stored a discounted game as 0% off and the
// discount badge vanished. The attributes are preferred, then the text, and if
// both are missing the percentage is worked out from the two prices.
func readPurchasePrices(section *goquery.Selection) (price, original float64, discount int) {
	if final := section.Find(".discount_final_price").First(); final.Length() > 0 {
		price, _ = parsePrice(strings.TrimSpace(final.Text()))
		original, _ = parsePrice(strings.TrimSpace(section.Find(".discount_original_price").First().Text()))
		block := section.Find(".discount_block").First()
		if price == 0 {
			if cents, err := strconv.Atoi(block.AttrOr("data-price-final", "")); err == nil {
				price = float64(cents) / 100
			}
		}
		if d, err := strconv.Atoi(block.AttrOr("data-discount", "")); err == nil {
			discount = d
		} else {
			discount = parseDiscountPercentage(section.Find(".discount_pct").First().Text())
		}
	} else {
		price, _ = parsePrice(strings.TrimSpace(section.Find(".game_purchase_price").First().Text()))
		original = price
	}
	return normalizeDiscount(price, original, discount)
}

// normalizeDiscount makes the three numbers agree with each other, filling in
// whichever one is missing from the other two.
func normalizeDiscount(price, original float64, discount int) (float64, float64, int) {
	if original < price {
		original = price
	}
	switch {
	case discount <= 0 && original > price && original > 0:
		discount = int(math.Round((1 - price/original) * 100))
	case discount > 0 && discount < 100 && original <= price:
		original = math.Round(price/(1-float64(discount)/100)*100) / 100
	}
	if discount < 0 || discount > 100 {
		discount = 0
	}
	return price, original, discount
}
