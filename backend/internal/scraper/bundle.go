package scraper

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

var bundleLinkPattern = regexp.MustCompile(`/bundle/(\d+)`)

// discoverBundleIDs finds the bundles a game's store page advertises, via
// links to /bundle/<id> and the data-ds-bundleid attribute on bundle blocks.
func discoverBundleIDs(doc *goquery.Selection) []int {
	seen := map[int]bool{}
	var ids []int
	add := func(raw string) {
		if id, err := strconv.Atoi(raw); err == nil && id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	doc.Find("a[href*='/bundle/']").Each(func(_ int, s *goquery.Selection) {
		if href, ok := s.Attr("href"); ok {
			if m := bundleLinkPattern.FindStringSubmatch(href); m != nil {
				add(m[1])
			}
		}
	})
	doc.Find("[data-ds-bundleid]").Each(func(_ int, s *goquery.Selection) {
		if id, ok := s.Attr("data-ds-bundleid"); ok {
			add(id)
		}
	})
	return ids
}

// parseBundlePage reads a store.steampowered.com/bundle/<id> page. Prices
// are whatever region the request was made in (the scraper pins cc=us).
func parseBundlePage(doc *goquery.Selection, bundleID int, url string) (models.Bundle, error) {
	bundle := models.Bundle{BundleID: bundleID, URL: url, Status: "tracked", Games: []models.BundleGame{}}

	bundle.Name = strings.TrimSpace(doc.Find(".pageheader").First().Text())
	if bundle.Name == "" {
		return bundle, fmt.Errorf("bundle %d has no name (not a valid bundle page?)", bundleID)
	}
	bundle.HeaderImage, _ = doc.Find("img.package_header").First().Attr("src")

	// "Price of individual products" is the undiscounted sum; "Your cost" is
	// what the bundle actually costs after the bundle discount.
	original, _ := parsePrice(strings.TrimSpace(doc.Find(".bundle_final_package_price").First().Text()))
	price, _ := parsePrice(strings.TrimSpace(doc.Find(".bundle_final_price_with_discount").First().Text()))
	bundle.DiscountPercentage = parseDiscountPercentage(doc.Find(".bundle_discount").First().Text())

	if original == 0 && price == 0 {
		return bundle, fmt.Errorf("bundle %d (%s): no price found", bundleID, bundle.Name)
	}
	if price == 0 {
		price = math.Round(original*(100-float64(bundle.DiscountPercentage))) / 100
	}
	if original == 0 {
		original = price
	}
	bundle.Price, bundle.OriginalPrice = price, original

	seen := map[int]bool{}
	doc.Find(".bundle_package_item [data-ds-appid]").Each(func(_ int, s *goquery.Selection) {
		raw, _ := s.Attr("data-ds-appid")
		appID, err := strconv.Atoi(raw)
		if err != nil || appID <= 0 || seen[appID] {
			return
		}
		seen[appID] = true
		bundle.Games = append(bundle.Games, models.BundleGame{
			AppID: appID,
			Name:  strings.TrimSpace(s.Find(".tab_item_name").First().Text()),
		})
	})
	if len(bundle.Games) == 0 {
		return bundle, fmt.Errorf("bundle %d (%s): no games found", bundleID, bundle.Name)
	}
	return bundle, nil
}

type bundleResult struct {
	bundle *models.Bundle
	err    error
}

// ScrapeBundles fetches and parses the given bundle pages. Bundles that
// fail (removed, region-locked, layout change) are reported in the returned
// errors and skipped.
func (s *Scraper) ScrapeBundles(bundleIDs []int) ([]models.Bundle, []error) {
	if len(bundleIDs) == 0 {
		return nil, nil
	}
	c := s.newCollector()
	results := make(chan bundleResult, len(bundleIDs)*2)

	c.OnHTML("html", func(e *colly.HTMLElement) {
		id, err := strconv.Atoi(e.Request.Ctx.Get("bundleID"))
		if err != nil {
			results <- bundleResult{err: fmt.Errorf("bad bundle id in context: %w", err)}
			return
		}
		bundle, err := parseBundlePage(e.DOM, id, fmt.Sprintf("%s/bundle/%d", s.BaseURL, id))
		if err != nil {
			results <- bundleResult{err: err}
			return
		}
		results <- bundleResult{bundle: &bundle}
	})
	c.OnError(func(r *colly.Response, err error) {
		results <- bundleResult{err: fmt.Errorf("failed to scrape bundle %s: %w", r.Request.Ctx.Get("bundleID"), err)}
	})

	for _, id := range bundleIDs {
		ctx := colly.NewContext()
		ctx.Put("bundleID", strconv.Itoa(id))
		url := fmt.Sprintf("%s/bundle/%d?cc=%s&l=english", s.BaseURL, id, storeCountry)
		if err := c.Request("GET", url, nil, ctx, nil); err != nil {
			results <- bundleResult{err: fmt.Errorf("failed to request bundle %d: %w", id, err)}
		}
	}
	c.Wait()
	close(results)

	var bundles []models.Bundle
	var errs []error
	for r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			log.Printf("Bundle scrape: %v", r.err)
			continue
		}
		bundles = append(bundles, *r.bundle)
	}
	return bundles, errs
}
