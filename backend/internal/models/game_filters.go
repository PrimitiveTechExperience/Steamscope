package models

type GameFilters struct {
	Search     string   `json:"search"`
	Genre      string   `json:"genre"`
	Developer  string   `json:"developer"`
	Publisher  string   `json:"publisher"`
	Languages  []string `json:"languages"`
	Tags       []string `json:"tags"`
	Genres     []string `json:"genres"`
	Developers []string `json:"developers"`
	Publishers []string `json:"publishers"`
	MinPrice   float64  `json:"min_price"`
	MaxPrice   float64  `json:"max_price"`
	// MinDiscount keeps games at least this many percent off (1-100); 0 means any.
	MinDiscount int `json:"min_discount"`
	Limit       int `json:"limit"`
	Offset      int `json:"offset"`
}
