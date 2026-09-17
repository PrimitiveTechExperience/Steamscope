package models

type GameFilters struct {
	Search    string
	Genre     string
	Developer string
	Publisher string
	Languages []string
	Tags      []string
	Genres	  []string
	MinPrice  float64
	MaxPrice  float64
	Limit     int
	Offset    int
}
