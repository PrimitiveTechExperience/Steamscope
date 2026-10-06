package router_test

import (
	"context"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func TestScrapeWithoutAPriceKeepsTheLastKnownOne(t *testing.T) {
	app := testutil.NewApp(t)
	app.SeedGame(820001, "Delisted", []string{"d"}, []string{"p"}) // $9.99, was $19.99, 50% off
	ctx := context.Background()

	game := models.Game{
		AppID: 820001, Name: "Delisted Renamed", URL: "https://store.steampowered.com/app/820001", ReleaseDate: time.Now(),
		Price: 0, OriginalPrice: 0, DiscountPercentage: 0, PriceUnknown: true,
		Developers: []string{"d"}, Publishers: []string{"p"}, Genres: []string{"Action"}, Tags: []string{"Co-op"}, SupportedLanguages: []string{"English"},
	}
	if err := app.DB.InsertGame(ctx, game); err != nil {
		t.Fatal(err)
	}
	var name string
	var price, original, discount float64
	if err := app.Pool.QueryRow(ctx, `SELECT name, price, original_price, discount_percentage FROM games WHERE app_id = 820001`).Scan(&name, &price, &original, &discount); err != nil {
		t.Fatal(err)
	}
	if price != 9.99 || original != 19.99 || discount != 50 {
		t.Errorf("price = %v / %v / %v, want the previous 9.99 / 19.99 / 50 kept", price, original, discount)
	}
	if name != "Delisted Renamed" {
		t.Errorf("other fields should still update, name = %q", name)
	}

	// A page that does show a price (including a real $0) still updates it.
	game.PriceUnknown = false
	if err := app.DB.InsertGame(ctx, game); err != nil {
		t.Fatal(err)
	}
	if err := app.Pool.QueryRow(ctx, `SELECT price, original_price, discount_percentage FROM games WHERE app_id = 820001`).Scan(&price, &original, &discount); err != nil {
		t.Fatal(err)
	}
	if price != 0 || original != 0 || discount != 0 {
		t.Errorf("a known $0 price should be stored, got %v / %v / %v", price, original, discount)
	}
}
