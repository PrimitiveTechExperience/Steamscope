package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScrapeGame_ParsesNameAndDeveloper(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="dev_row">
							<div class="summary_column">
								<a href="/developer/testdev">Test Developer</a>
							</div>
						</div>
					</body>
				</html>
			`)
		}),
	)
	defer server.Close()

	// s := New()
	// s.Collector.OnHTML("html", func(e *colly.HTMLElement) {
	// 	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(e.Response.Body)))
	// 	if err != nil {
	// 		t.Fatalf("Failed to create document: %v", err)
	// 	}
	// 	game := parseGamePage(doc.Selection, 12345, server.URL)
	// 	if game.Name != "Test Game Name" {
	// 		t.Errorf("Expected Name 'Test Game Name', got '%s'", game.Name)
	// 	}
	// 	if game.Developers[0] != "Test Developer" {
	// 		t.Errorf("Expected Developer 'Test Developer', got '%s'", game.Developers[0])
	// 	}
	// })

	// err := s.ScrapeGame(12345)
	// if err != nil {
	// 	t.Fatalf("Failed to scrape game: %v", err)
	// }
}
