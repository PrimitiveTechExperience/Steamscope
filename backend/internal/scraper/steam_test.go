package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

func TestParseGamePage(t *testing.T) {
	html := `
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
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}

	game := parseGamePage(doc.Selection, 12345, "https://store.steampowered.com/app/12345/")

	if game.AppID != 12345 {
		t.Errorf("Expected AppID 12345, got %d", game.AppID)
	}
	if game.Name != "Test Game Name" {
		t.Errorf("Expected Name 'Test Game Name', got '%s'", game.Name)
	}
	if game.Developers[0] != "Test Developer" {
		t.Errorf("Expected Developer 'Test Developer', got '%s'", game.Developers[0])
	}
	if game.URL != "https://store.steampowered.com/app/12345/" {
		t.Errorf("Expected URL 'https://store.steampowered.com/app/12345/', got '%s'", game.URL)
	}
}

func TestParseAppIDFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected int
	}{
		{"https://store.steampowered.com/app/12345/", 12345},
		{"https://store.steampowered.com/app/67890/SomeGame/", 67890},
		{"https://store.steampowered.com/app/54321/AnotherGame", 54321},
	}
	for _, test := range tests {
		result, err := parseAppIDFromURL(test.url)
		if err != nil {
			t.Fatalf("Failed to parse AppID from URL %s: %v", test.url, err)
		}
		if result != test.expected {
			t.Errorf("Expected AppID %d for URL %s, got %d", test.expected, test.url, result)
		}
	}
}

func TestScrapeGame(t *testing.T) {
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

	s := New()
	s.Collector.OnHTML("html", func(e *colly.HTMLElement) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(e.Response.Body)))
		if err != nil {
			t.Fatalf("Failed to create document: %v", err)
		}
		game := parseGamePage(doc.Selection, 12345, server.URL)
		if game.Name != "Test Game Name" {
			t.Errorf("Expected Name 'Test Game Name', got '%s'", game.Name)
		}
		if game.Developers[0] != "Test Developer" {
			t.Errorf("Expected Developer 'Test Developer', got '%s'", game.Developers[0])
		}
	})

	err := s.ScrapeGame(12345)
	if err != nil {
		t.Fatalf("Failed to scrape game: %v", err)
	}
}
