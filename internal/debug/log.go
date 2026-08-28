package debug

import (
	"log"

	"github.com/PrimitiveTechExperience/Steamscope/internal/models"
)

func OutputGameToConsole(game models.Game) {
	log.Printf("Scraped Game: %s\n", game.Name)
	log.Printf("Developer: %s\n", game.Developer)
	log.Printf("Publisher: %s\n", game.Publisher)
	log.Printf("Release Date: %s\n", game.ReleaseDate)
	log.Printf("Price: %s\n", game.Price)
	log.Printf("Original Price: %s\n", game.OriginalPrice)
	log.Printf("Discount Percentage: %d%%\n", game.DiscountPercentage)
	log.Printf("Genres: %v\n", game.Genres)
	log.Printf("Tags: %v\n", game.Tags)
	log.Printf("Review Score: %s\n", game.ReviewScore)
	log.Printf("Review Count: %d\n", game.ReviewCount)
	log.Printf("Description: %s\n", game.Description)
	log.Printf("Windows Compatible: %t\n", game.WindowsCompatible)
	log.Printf("Linux Compatible: %t\n", game.LinuxCompatible)
	log.Printf("Mac Compatible: %t\n", game.MacCompatible)
}