package models

import "time"

type Bundle struct {
	BundleID           int          `json:"bundle_id"`
	Name               string       `json:"name"`
	URL                string       `json:"url"`
	HeaderImage        string       `json:"header_image"`
	Price              float64      `json:"price"`
	OriginalPrice      float64      `json:"original_price"`
	DiscountPercentage int          `json:"discount_percentage"`
	Status             string       `json:"status"`
	GameCount          int          `json:"game_count"`
	Games              []BundleGame `json:"games"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// BundleGame is one game in a bundle. Tracked is true when the game is also
// in our games table (so it has its own page).
type BundleGame struct {
	AppID   int    `json:"app_id"`
	Name    string `json:"name"`
	Tracked bool   `json:"tracked"`
	// TrackStatus is the game's tracked_games status when it isn't on the
	// site yet (awaiting_approval, pending, failed, rejected), else empty.
	TrackStatus string `json:"track_status"`
}

type BundleDetail struct {
	Bundle
	PriceHistory []PricePoint `json:"price_history"`
}
