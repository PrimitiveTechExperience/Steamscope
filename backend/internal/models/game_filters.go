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
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
}
