// Actually scrape the game data from the Steam store page.

package scraper

import (
	"testing"
)

func TestScrapeSteamGame(t *testing.T) {
	s := New()

	err := s.ScrapeGame(730)

	if err != nil {
		t.Fatalf("Error scraping game: %v", err)
	}
}
// Note: Steam Game 730 is CS:GO, which is a popular game and should always be available on the Steam store.

// func TestScrapeSteam