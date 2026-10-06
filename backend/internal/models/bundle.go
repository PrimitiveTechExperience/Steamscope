package models

import "time"

type Bundle struct {
	BundleID           int     `json:"bundle_id"`
	Name               string  `json:"name"`
	URL                string  `json:"url"`
	HeaderImage        string  `json:"header_image"`
	Price              float64 `json:"price"`
	OriginalPrice      float64 `json:"original_price"`
	DiscountPercentage int     `json:"discount_percentage"`
	Status             string  `json:"status"`
	// AtRecordLow is true when the bundle is discounted and at the lowest
	// price we have recorded for it.
	AtRecordLow bool         `json:"at_record_low"`
	GameCount   int          `json:"game_count"`
	Games       []BundleGame `json:"games"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// BundleGame is one game in a bundle. Tracked is true when the game is also
// in our games table (so it has its own page).
type BundleGame struct {
	AppID   int    `json:"app_id"`
	Name    string `json:"name"`
	Tracked bool   `json:"tracked"`
	// Price and RegularPrice are the game's current and undiscounted price, in
	// USD (0 when unknown). Tracked games use our own scraped prices; others use
	// what the bundle page showed.
	Price        float64 `json:"price"`
	RegularPrice float64 `json:"regular_price"`
	// TrackStatus is the game's tracked_games status when it isn't on the
	// site yet (awaiting_approval, pending, failed, rejected), else empty.
	TrackStatus string `json:"track_status"`
}

type BundleDetail struct {
	Bundle
	PriceHistory []PricePoint `json:"price_history"`
	// RecordLow is the lowest bundle price we have recorded, and HistoryDays how
	// many days that record spans. A bundle's history starts when it was found.
	RecordLow   float64 `json:"record_low"`
	HistoryDays int     `json:"history_days"`
}
