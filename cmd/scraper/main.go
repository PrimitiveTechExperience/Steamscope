package main

import (
	"log"
	"net/url"
	"os"

	"github.com/PrimitiveTechExperience/Steamscope/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/internal/scraper"
)
	
func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Working directory:", cwd)

	cfg := config.LoadConfig()

	s := scraper.New()

	cookies, err := config.LoadCookies(cfg.Steam.CookieFilePath)
	if err != nil {
		log.Fatalf("Failed to load cookies: %v", err)
	}

	if err := s.SetSteamCookies(cookies); err != nil {
		log.Fatalf("Failed to set Steam cookies: %v", err)
	}

	u, _ := url.Parse("https://store.steampowered.com")
	for _, c := range s.JarCookies(u) {
		log.Printf("Loaded cookie: %s", c.Name)
	}


	appIDs := []int{
		730,    // Counter-Strike: Global Offensive
		570,    // Dota 2
		440,    // Team Fortress 2
		578080, // PLAYERUNKNOWN'S BATTLEGROUNDS
		4000,   // Garry's Mod
		550,    // Left 4 Dead 2
		252490, // Rust
		304930, // Unturned
		271590, // Grand Theft Auto V
		1174180, // Cyberpunk 2077
	}

	games, errors := s.ScrapeGamePages(appIDs)
	if errors != nil {
		log.Printf("Errors occurred while scraping game pages: %v", errors)
	}
	for _, games := range games {
		log.Printf("Scraped game: %s (AppID: %d)", games.Name, games.AppID)
	}

}

