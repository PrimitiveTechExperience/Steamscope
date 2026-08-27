package scraper

import (
	"fmt"
	"log"

	"github.com/PrimitiveTechExperience/Steamscope/internal/models"
	"github.com/gocolly/colly/v2"
)

type Scraper struct {
	Collector *colly.Collector
	results chan models.Game
}

func New() *Scraper {
	c := colly.NewCollector(
		colly.AllowedDomains("store.steampowered.com"),
	)
	c.Async = true

	c.OnRequest(func(r *colly.Request){
		log.Printf("Visiting: %s", r.URL.String())
	})
	// Test to ensure that we are connecting to Steam.
	// c.OnHTML(".apphub_AppName", func(e *colly.HTMLElement) {
	// 	log.Printf("Game Name: %s\n", e.Text)
	// })

	// c.OnHTML(".dev_row .summary.column a", func(e *colly.HTMLElement) {
	// 	log.Printf("Developer: %s\n", e.Text)
	// })
	// c.OnHTML("html", func(e *colly.HTMLElement) {
	// 	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(e.Response.Body)))
	// 	if err != nil {
	// 		log.Printf("Failed to create document: %v", err)
	// 		return
	// 	}
	// 	appID, err := parseAppIDFromURL(e.Request.URL.String())
	// 	if err != nil {
	// 		log.Printf("Failed to parse AppID from URL %s: %v", e.Request.URL.String(), err)
	// 		return
	// 	}
	// 	game := parseGamePage(doc, appID, e.Request.URL.String())
	// 	log.Printf("Scraped Game: %+v\n", game)
	// })

	c.OnResponse(func(r *colly.Response) {
		fmt.Printf("Received %d bytes\n", len(r.Body))
	})	

	c.OnError(func(r *colly.Response, err error){
		log.Printf("Ran into Error while visiting %s: %v", r.Request.URL.String(), err)
	})
	
	return &Scraper{
		Collector: c,
	}
}

func (s *Scraper) ScrapeGame(appID int) (models.Game, error) {
	url := fmt.Sprintf("https://store.steampowered.com/app/%d/", appID)
	var game models.Game
	s.Collector.OnHTML("html", func(e *colly.HTMLElement) {
		doc := e.DOM
		game = parseGamePage(doc, appID, url)
	})

	err := s.Collector.Visit(url)
	if err != nil {
		return models.Game{}, fmt.Errorf("failed to visit URL %s: %w", url, err)
	}
	s.Collector.Wait() // Wait for all asynchronous requests to complete

	if game.AppID == 0 {
		return models.Game{}, fmt.Errorf("failed to scrape game data for AppID %d", appID)
	}
	return game, nil
}