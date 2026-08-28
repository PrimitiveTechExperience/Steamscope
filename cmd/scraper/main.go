package main

import (
	"log"

	"github.com/PrimitiveTechExperience/Steamscope/internal/scraper"
)
	
func main() {
	s := scraper.New()

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

	for _, appID := range appIDs {
		err := s.ScrapeGame(appID)
		if err != nil {
			log.Printf("Error scraping game with AppID %d: %v", appID, err)
			continue
		}
		// debug.OutputGameToConsole(game)
	}
	s.Wait() // Wait for all asynchronous requests to complete
}

