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
					<head>
						<title>Test Game</title>
					</head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>

						<div class="dev_row">
							<div class="summary column">
								<b>Developer:</b>
								<a href="/developer/testdev">Test Developer</a>,
								<a href="/developer/testdev2">Second Developer</a>
							</div>
						</div>

						<div class="dev_row">
							<div class="summary column">
								<b>Publisher:</b>
								<a href="/publisher/testpub">Test Publisher</a>,
								<a href="/publisher/testpub2">Second Publisher</a>
							</div>
						</div>

						<div id="game_area_description">
							This is a test game description.
						</div>

						<div class="game_purchase_price">
							$19.99
						</div>

						<div class="release_date">
							<div class="date">14 Sep, 2026</div>
						</div>

						<div class="game_area_sys_req">
							<div class="game_area_sys_req sysreq_content">
								<div class="sysreq_tabs">
									<div class="sysreq_tab" data-os="win">Windows</div>
									<div class="sysreq_tab" data-os="mac">Mac</div>
									<div class="sysreq_tab" data-os="linux">Linux</div>
								</div>
							</div>
						</div>

						<div class="glance_tags popular_tags">
							<a href="/tag/action">Action</a>
							<a href="/tag/adventure">Adventure</a>
							<a href="/tag/indie">Indie</a>
						</div>

						<div class="details_block">
							<a href="/genre/action">Action</a>
							<a href="/genre/adventure">Adventure</a>
						</div>
					</body>
				</html>
			`)
		}),
	)
	defer server.Close()

	s := New(server.URL)
	games, err := s.ScrapeGamePages( []int{12345} )
	if err != nil {
		t.Fatalf("Failed to scrape game pages: %v", err)
	}
	if len(games) == 0 {
		t.Fatal("Expected at least one game, got zero")
	}
	if games[0].Name != "Test Game Name" {
		t.Errorf("Expected Name 'Test Game Name', got '%s'", games[0].Name)
	}
	if len(games[0].Developers) != 2 || games[0].Developers[0] != "Test Developer" || games[0].Developers[1] != "Second Developer" {
		t.Errorf("Expected Developers ['Test Developer', 'Second Developer'], got %v", games[0].Developers)
	}
	if len(games[0].Publishers) != 2 || games[0].Publishers[0] != "Test Publisher" || games[0].Publishers[1] != "Second Publisher" {
		t.Errorf("Expected Publishers ['Test Publisher', 'Second Publisher'], got %v", games[0].Publishers)
	}
	if games[0].Description != "This is a test game description." {
		t.Errorf("Expected Description 'This is a test game description.', got '%s'", games[0].Description)
	}
	if games[0].Price != 19.99 {
		t.Errorf("Expected Price 19.99, got %f", games[0].Price)
	}
	if !games[0].WindowsCompatible || !games[0].MacCompatible || !games[0].LinuxCompatible {
		t.Errorf("Expected all platforms to be compatible, got Windows: %v, Mac: %v, Linux: %v", games[0].WindowsCompatible, games[0].MacCompatible, games[0].LinuxCompatible)
	}
	if len(games[0].Tags) != 3 || games[0].Tags[0] != "Action" || games[0].Tags[1] != "Adventure" || games[0].Tags[2] != "Indie" {
		t.Errorf("Expected Tags ['Action', 'Adventure', 'Indie'], got %v", games[0].Tags)
	}
	if len(games[0].Genres) != 2 || games[0].Genres[0] != "Action" || games[0].Genres[1] != "Adventure" {
		t.Errorf("Expected Genres ['Action', 'Adventure'], got %v", games[0].Genres)
	}
	if games[0].ReleaseDate.Format("Jan 2, 2006") != "Sep 14, 2026" {
		t.Errorf("Expected ReleaseDate 'Sep 14, 2026', got '%s'", games[0].ReleaseDate.Format("Jan 2, 2006"))
	}
	if games[0].AppID != 12345 {
		t.Errorf("Expected AppID 12345, got %d", games[0].AppID)
	}
}
