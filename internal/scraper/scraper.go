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
	"github.com/gocolly/colly/v2"
)

type Scraper struct {
	Collector *colly.Collector
	Jar http.CookieJar
}

func New() *Scraper {
	c := colly.NewCollector(
		colly.AllowedDomains("store.steampowered.com"),
	)

	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create cookie jar: %v", err))
	}
	c.SetCookieJar(jar)
	c.Async = true

	c.Limit(&colly.LimitRule{
		DomainGlob: "*store.steampowered.com*",
		Parallelism: 4,
		Delay: 250*time.Millisecond,
		RandomDelay: 250*time.Millisecond,
	})

	c.OnRequest(func(r *colly.Request){
		log.Printf("Visiting: %s", r.URL.String())
	})

	c.OnHTML("html", func(e *colly.HTMLElement) {
		appIDStr := e.Request.Ctx.Get("appID")
		appID, err := strconv.Atoi(appIDStr)
		if err != nil {
			log.Printf("Failed to convert appID %s to int: %v", appIDStr, err)
			return
		}
		game := parseGamePage(e.DOM, appID, e.Request.URL.String())
		fmt.Printf(
			"%d: %s\n",
			game.AppID,
			game.Name,
		)
		// log.Printf(
		// 	"AppID %s title: %q",
		// 	e.Request.Ctx.Get("appID"),
		// 	e.Text,
		// )
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
		Jar: jar,
	}
}

func (s *Scraper) ScrapeGame(appID int) (error) {
	url := fmt.Sprintf(
		"https://store.steampowered.com/app/%d/",
		appID,
	)

	ctx := colly.NewContext()

	ctx.Put(
		"appID",
		strconv.Itoa(appID),
	)

	return s.Collector.Request(
		"GET",
		url,
		nil,
		ctx,
		nil,
	)
}

func (s *Scraper) Wait(){
	// Move to here so that we wait in main rather than waiting per game scrape.
	s.Collector.Wait()
}

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