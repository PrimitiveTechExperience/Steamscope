package scraper

import (
	"fmt"
	"log"
	"net/http/cookiejar"
	"strconv"
	"time"

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

	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create cookie jar: %v", err))
	}
	c.SetCookieJar(jar)


	c.Limit(&colly.LimitRule{
		DomainGlob: "*store.steampowered.com*",
		Parallelism: 4,
		Delay: 250*time.Millisecond,
	})

	c.OnRequest(func(r *colly.Request){
		log.Printf("Visiting: %s", r.URL.String())
	})
	// Debugging: Find body
	// c.OnHTML("body", func(h *colly.HTMLElement) {
	// 	if strings.Contains(h.Text, "not appropriate for all ages") {
	// 		log.Printf("Game with AppID %s is not appropriate for all ages.", h.Request.Ctx.Get("appID"))
	// 		// Check the inputs (they are selects)
	// 		h.DOM.Find("select").Each(func(i int, s *goquery.Selection) {
	// 			name, exists := s.Attr("name")
	// 			if !exists {
	// 				return
	// 			}
	// 			log.Printf("Found select with name: %s", name)
	// 		})
	// 	}
	// })

	c.OnHTML(".agegate_birthday_selector", func(e *colly.HTMLElement) {
		if e.DOM.Find("select[name='ageDay']").Length() == 0 {
			log.Printf("No age day select found for AppID: %s", e.Request.Ctx.Get("appID"))
			return	
		}

		if e.DOM.Find("select[name='ageMonth']").Length() == 0 {
			log.Printf("No age month select found for AppID: %s", e.Request.Ctx.Get("appID"))
			return
		}

		if e.DOM.Find("select[name='ageYear']").Length() == 0 {
			log.Printf("No age year select found for AppID: %s", e.Request.Ctx.Get("appID"))
			return
		}

		log.Printf(
			"Submitting an age check for AppID: %s",
			e.Request.Ctx.Get("appID"),
		)
		log.Printf("URL: %s", e.Request.URL.String())
		action, _ := e.DOM.Attr("action")

		log.Printf("Age verification form:")
		log.Printf("  Action: %s", action)
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