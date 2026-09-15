// parser_tests.go

package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PuerkitoBio/goquery"
)

func TestParseGamePage(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected models.Game
	}{
		{
			name: "Basic Game Page",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/developer/testdev">Test Developer</a>
							</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{"Test Developer"},
				URL:        "https://store.steampowered.com/app/12345/",
			},
		},
		{
			name: "Game Page with Multiple Developers",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/developer/testdev1">Test Developer 1</a>, 
								<a href="/developer/testdev2">Test Developer 2</a>
							</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{"Test Developer 1", "Test Developer 2"},
				URL:        "https://store.steampowered.com/app/12345/",
			},
		},
		{
			name: "Game Page with No Developers",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{},
				URL:        "https://store.steampowered.com/app/12345/",
			},
		},
		{
			name: "Game Page with No Name",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "",
				Developers: []string{},
				URL:        "https://store.steampowered.com/app/12345/",
			},
		},
		{
			name: "Game Page with a description and a release date",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="game_area_description">Test Game Description</div>
						<div class="date">19 Sep, 2023</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{},
				URL:        "https://store.steampowered.com/app/12345/",
				Description: "Test Game Description",
				ReleaseDate: time.Date(2023, time.September, 19, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "Game Page with some genres and tags",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="details_block">
							<a href="/genre/action">Action</a>
							<a href="/genre/adventure">Adventure</a>
						</div>
						<div class="glance_tags popular_tags">
							<a href="/tag/multiplayer">Multiplayer</a>
							<a href="/tag/indie">Indie</a>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{},
				URL:        "https://store.steampowered.com/app/12345/",
				Genres:     []string{"Action", "Adventure"},
				Tags:       []string{"Multiplayer", "Indie"},
			},
		},
		{
			name: "Game Page with a price and discount",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="discount_final_price">9.99</div>
						<div class="discount_original_price">19.99</div>
						<div class="discount_pct">-50%</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:              12345,
				Name:               "Test Game Name",
				Developers:         []string{},
				URL:                "https://store.steampowered.com/app/12345/",
				Price:              9.99,
				OriginalPrice:      19.99,
				DiscountPercentage: 50,
			},
		},
		{
			name: "Game Page with a price and no discount",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="game_purchase_price">19.99</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:              12345,
				Name:               "Test Game Name",
				Developers:         []string{},
				URL:                "https://store.steampowered.com/app/12345/",
				Price:              19.99,
				OriginalPrice:      19.99,
				DiscountPercentage: 0,
			},
		},
		{
			name: "Game Page with compatability for different platforms",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="game_area_sys_req">
							<div class="game_area_sys_req sysreq_content">
								<div class="sysreq_tabs">
									<div class="sysreq_tab" data-os="win">Windows</div>
									<div class="sysreq_tab" data-os="mac">Mac</div>
									<div class="sysreq_tab" data-os="linux">Linux</div>
								</div>
							</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{},
				URL:        "https://store.steampowered.com/app/12345/",
				WindowsCompatible: true,
				MacCompatible:     true,
				LinuxCompatible:   true,
			},
		},
		{
			name: "Game Page with review score and count",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="user_reviews_summary_row">
							<div class="summary column">
								<span class="game_review_summary">Mostly Positive</span>
							</div>
							<div class="responsive_hidden">123</div>
						</div>
						<div class="user_reviews_summary_row">
							<div class="summary column">
								<span class="game_review_summary">Very Positive</span>
							</div>
							<div class="responsive_hidden">1,234</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:       12345,
				Name:        "Test Game Name",
				Developers:  []string{},
				URL:         "https://store.steampowered.com/app/12345/",
				ReviewScore: "Very Positive",
				ReviewCount: 1234,
			},
		},
		{
			name: "Game Page with languages",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="game_language_options">
							<div class="ellipsis">English</div>
							<div class="ellipsis">French</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:              12345,
				Name:               "Test Game Name",
				Developers:         []string{},
				URL:                "https://store.steampowered.com/app/12345/",
				SupportedLanguages: []string{"English", "French"},
			},
		},
		{
			name: "Game Page with publisher",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/developer/testdev">Test Developer</a>
							</div>
						</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/publisher/testpub">Test Publisher</a>
							</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{"Test Developer"},
				Publishers: []string{"Test Publisher"},
				URL:        "https://store.steampowered.com/app/12345/",
			},
		},
		{
			name: "Game Page with multiple publishers",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/developer/testdev">Test Developer</a>
							</div>
						</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/publisher/testpub1">Test Publisher 1</a>, <a href="/publisher/testpub2">Test Publisher 2</a>
							</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{"Test Developer"},
				Publishers: []string{"Test Publisher 1", "Test Publisher 2"},
				URL:        "https://store.steampowered.com/app/12345/",
			},
		},
		{
			name: "Game Page with a bit of everything",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/developer/testdev">Test Developer A</a>
								<a href="/developer/testdev2">Test Developer B</a>
							</div>
						</div>
						<div class="game_language_options">
							<div class="ellipsis">English</div>
							<div class="ellipsis">French</div>
						</div>
						<div class="dev_row">
							<div class="summary column">
								<a href="/publisher/testpub">Test Publisher</a>
								<a href="/publisher/testpub2">Test Publisher 2</a>
							</div>
						</div>
						<div class="details_block">
							<a href="/genre/action">Action</a>
							<a href="/genre/adventure">Adventure</a>
						</div>
						<div class="glance_tags popular_tags">
							<a href="/tag/multiplayer">Multiplayer</a>
							<a href="/tag/indie">Indie</a>
						</div>
						<div class="discount_final_price">9.99</div>
						<div class="discount_original_price">19.99</div>
						<div class="discount_pct">-50%</div>
						<div class="game_area_sys_req">
							<h4>System Requirements</h4>
							<p>Minimum:</p>
							<p>Recommended:</p>
							<div class="game_area_sys_req sysreq_content">
								<div class="sysreq_tabs">
									<div class="sysreq_tab" data-os="win">Windows</div>
									<div class="sysreq_tab" data-os="linux">Linux</div>
								</div>
							</div>
						</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:      12345,
				Name:       "Test Game Name",
				Developers: []string{"Test Developer A", "Test Developer B"},
				Publishers: []string{"Test Publisher", "Test Publisher 2"},
				URL:        "https://store.steampowered.com/app/12345/",
				Tags:       []string{"Multiplayer", "Indie"},
				Genres:     []string{"Action", "Adventure"},
				Price:      9.99,
				OriginalPrice: 19.99,
				DiscountPercentage: 50,
				SupportedLanguages: []string{"English", "French"},
				WindowsCompatible: true,
				MacCompatible:     false,
				LinuxCompatible:   true,
			},
		},
		{
			name: "Game Page with Price in Canadian Dollars",
			html: `
				<html>
					<head><title>Test Game</title></head>
					<body>
						<div class="apphub_AppName">Test Game Name</div>
						<div class="discount_final_price">CDN$ 9.99</div>
						<div class="discount_original_price">CDN$ 19.99</div>
						<div class="discount_pct">-50%</div>
					</body>
				</html>
			`,
			expected: models.Game{
				AppID:              12345,
				Name:               "Test Game Name",
				Developers:         []string{},
				URL:                "https://store.steampowered.com/app/12345/",
				Price:              9.99,
				OriginalPrice:      19.99,
				DiscountPercentage: 50,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(test.html))
			if err != nil {
				t.Fatalf("Failed to create document: %v", err)
			}
			game := parseGamePage(doc.Selection, 12345, "https://store.steampowered.com/app/12345/")
			if game.AppID != test.expected.AppID {
				t.Errorf("Expected AppID %d, got %d", test.expected.AppID, game.AppID)
			}
			if game.Name != test.expected.Name {
				t.Errorf("Expected Name '%s', got '%s'", test.expected.Name, game.Name)
			}
			if game.URL != test.expected.URL {
				t.Errorf("Expected URL '%s', got '%s'", test.expected.URL, game.URL)
			}
			if game.Description != test.expected.Description {
				t.Errorf("Expected Description '%s', got '%s'", test.expected.Description, game.Description)
			}
			if game.ReleaseDate != test.expected.ReleaseDate {
				t.Errorf("Expected ReleaseDate '%s', got '%s'", test.expected.ReleaseDate, game.ReleaseDate)
			}
			if len(game.SupportedLanguages) != len(test.expected.SupportedLanguages) {
				t.Errorf("Excpeted %d SupportedLanguages, got %d", len(test.expected.SupportedLanguages), len(game.SupportedLanguages))
			}
			if game.SupportedLanguages != nil && test.expected.SupportedLanguages != nil {
				for i, lang := range game.SupportedLanguages {
					if lang != test.expected.SupportedLanguages[i] {
						t.Errorf("Expected SupportedLanguage '%s', got '%s'", test.expected.SupportedLanguages[i], lang)
					}
				}
			}
			if len(game.Developers) != len(test.expected.Developers) {
				t.Errorf("Expected %d Developers, got %d", len(test.expected.Developers), len(game.Developers))
			}
			if game.Developers != nil && test.expected.Developers != nil {
				for i, dev := range game.Developers {
					if dev != test.expected.Developers[i] {
						t.Errorf("Expected Developer '%s', got '%s'", test.expected.Developers[i], dev)
					}
				}
			}
			if len(game.Publishers) != len(test.expected.Publishers) {
				t.Errorf("Expected %d Publishers, got %d", len(test.expected.Publishers), len(game.Publishers))
			}
			if game.Publishers != nil && test.expected.Publishers != nil {
				for i, pub := range game.Publishers {
					if pub != test.expected.Publishers[i] {
						t.Errorf("Expected Publisher '%s', got '%s'", test.expected.Publishers[i], pub)
					}
				}
			}
			if game.Price != test.expected.Price {
				t.Errorf("Expected Price %f, got %f", test.expected.Price, game.Price)
			}
			if game.OriginalPrice != test.expected.OriginalPrice {
				t.Errorf("Expected OriginalPrice %f, got %f", test.expected.OriginalPrice, game.OriginalPrice)
			}
			if game.DiscountPercentage != test.expected.DiscountPercentage {
				t.Errorf("Expected DiscountPercentage %d, got %d", test.expected.DiscountPercentage, game.DiscountPercentage)
			}
			if len(game.Genres) != len(test.expected.Genres) {
				t.Errorf("Expected %d Genres, got %d", len(test.expected.Genres), len(game.Genres))
			}
			if game.Genres != nil && test.expected.Genres != nil {
				for i, genre := range game.Genres {
					if genre != test.expected.Genres[i] {
						t.Errorf("Expected Genre '%s', got '%s'", test.expected.Genres[i], genre)
					}
				}
			}
			if len(game.Tags) != len(test.expected.Tags) {
				t.Errorf("Expected %d Tags, got %d", len(test.expected.Tags), len(game.Tags))
			}
			if game.Tags != nil && test.expected.Tags != nil {
				for i, tag := range game.Tags {
					if tag != test.expected.Tags[i] {
						t.Errorf("Expected Tag '%s', got '%s'", test.expected.Tags[i], tag)
					}
				}
			}
			if game.ReviewScore != test.expected.ReviewScore {
				t.Errorf("Expected ReviewScore '%s', got '%s'", test.expected.ReviewScore, game.ReviewScore)
			}
			if game.ReviewCount != test.expected.ReviewCount {
				t.Errorf("Expected ReviewCount %d, got %d", test.expected.ReviewCount, game.ReviewCount)
			}
			// Parser doesn't handle reviews, as that is handled by a different function, so we skip checks here for reviews
			if game.WindowsCompatible != test.expected.WindowsCompatible {
				t.Errorf("Expected WindowsCompatible %t, got %t", test.expected.WindowsCompatible, game.WindowsCompatible)
			}
			if game.LinuxCompatible != test.expected.LinuxCompatible {
				t.Errorf("Expected LinuxCompatible %t, got %t", test.expected.LinuxCompatible, game.LinuxCompatible)
			}
			if game.MacCompatible != test.expected.MacCompatible {
				t.Errorf("Expected MacCompatible %t, got %t", test.expected.MacCompatible, game.MacCompatible)
			}
		})
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
