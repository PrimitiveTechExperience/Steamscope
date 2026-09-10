package scraper

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/internal/models"
	"github.com/gocolly/colly/v2"
)

type Scraper struct {
	// Collector *colly.Collector
	Jar http.CookieJar
	Client *http.Client
	// GameResult chan GameResult
}

type GameResult struct{
	Game *models.Game
	Error error
}

func (s *Scraper) newCollector() *colly.Collector {
	c := colly.NewCollector(
		colly.AllowedDomains("store.steampowered.com"),
	)
	c.Async = true
	c.SetCookieJar(s.Jar)
	c.Limit(&colly.LimitRule{
		DomainGlob: "*store.steampowered.com*",
		Parallelism: 4,
		Delay: 250*time.Millisecond,
		RandomDelay: 250*time.Millisecond,
	})
	c.OnRequest(func(r *colly.Request){
		log.Printf("Visiting: %s", r.URL.String())
	})
	c.OnResponse(func(r *colly.Response) {
		log.Printf("Received %d bytes\n", len(r.Body))
	})	

	return c
}

func New() *Scraper {
	// Set cookie jar for cookies
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create cookie jar: %v", err))
	}

	client := &http.Client{
		Jar: jar,
		Timeout: 30 * time.Second,
	}

	s := &Scraper{
		Jar: jar,
		Client: client,
	}
	return s
}
// 
// GAME SCRAPING FUNCTIONS
// 
func (s *Scraper) ScrapeGame(appID int, c *colly.Collector) (error) {
	url := fmt.Sprintf("https://store.steampowered.com/app/%d", appID)
	ctx := colly.NewContext()
	ctx.Put("appID", strconv.Itoa(appID))
	return c.Request("GET", url, nil, ctx, nil)
}

func (s *Scraper) ScrapeGamePages(appIDs []int) ([]models.Game, error) {
	// We let colly do the work
	c := s.newCollector()
	results := make(chan GameResult, len(appIDs))
	c.OnHTML("html", func(e *colly.HTMLElement){
		appIDstr := e.Request.Ctx.Get("appID")

		appID, err := strconv.Atoi(appIDstr)
		if err != nil {
			results <- GameResult{Game: nil, Error: fmt.Errorf("failed to convert appID %s to int: %w", appIDstr, err)}
			return
		}
		game := parseGamePage(e.DOM, appID, e.Request.URL.String())

		results <- GameResult{Game: &game, Error: nil}
	})

	c.OnError(func(r *colly.Response, err error) {
		appIDstr := r.Request.Ctx.Get("appID")
		appID, errConv := strconv.Atoi(appIDstr)
		if errConv != nil {
			results <- GameResult{Game: nil, Error: fmt.Errorf("failed to convert appID %s to int: %w", appIDstr, errConv)}
			return
		}
		results <- GameResult{Game: nil, Error: fmt.Errorf("failed to scrape game page for appID %d: %w", appID, err)}
		log.Printf("Error scraping game page for appID %d: %v", appID, err)
	})

	// Start scraping
	for _, appID := range appIDs {
		if err := s.ScrapeGame(appID, c); err != nil {
			results <- GameResult{Game: nil, Error: fmt.Errorf("failed to scrape game page for appID %d: %w", appID, err)}
		}
	}

	c.Wait()
	close(results)
	
	var games []models.Game
	var scrapeErrors []error

	for result := range results {
		if result.Error != nil {
			scrapeErrors = append(scrapeErrors, result.Error)
			continue
		}
		if result.Game != nil {
			games = append(games, *result.Game)
		}
	}

	if len(scrapeErrors) > 0 {
		return games, fmt.Errorf("failed to scrape some game pages: %v", scrapeErrors)
	}

	return games, nil
}

func (s *Scraper) ScrapeGames(appIDs []int, options ReviewOption) ([]models.Game, error) {
	// Get game info
	games, err := s.ScrapeGamePages(appIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to scrape game pages: %w", err)
	}
	// Get reviews for each game
	reviews, err := s.FetchReviewsForGames(games, options)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reviews for games: %w", err)
	}
	// Assign reviews to each game
	for i := range games{
		games[i].Reviews = reviews[games[i].AppID]
	}

	return games, nil
}
// 
// UTILITY FUNCTIONS
// 

func (s *Scraper) SetSteamCookies(cookies []config.Cookie) error {
	u, err := url.Parse("https://store.steampowered.com")
	if err != nil {
		return fmt.Errorf("failed to parse domain %s: %w", "https://store.steampowered.com", err)
	}
	httpCookies := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		httpCookies = append(httpCookies, &http.Cookie{
			Name:  cookie.Name,
			Value: cookie.Value,
			Domain: cookie.Domain,
		})
	}
	s.Jar.SetCookies(u, httpCookies)
	
	return nil
}

func (s *Scraper) JarCookies(u *url.URL) []*http.Cookie {
	return s.Jar.Cookies(u)
}