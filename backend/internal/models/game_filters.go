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
	// IncludeBundles also returns bundles that match the search.
	IncludeBundles bool `json:"include_bundles"`
	Limit          int  `json:"limit"`
	Offset         int  `json:"offset"`
}

// HasGameAttributeFilters reports whether any filter describes a game (genre,
// tag, developer, publisher, language). A bundle has none of these of its own.
func (f GameFilters) HasGameAttributeFilters() bool {
	return f.Genre != "" || f.Developer != "" || f.Publisher != "" ||
		len(f.Genres) > 0 || len(f.Tags) > 0 || len(f.Languages) > 0 || len(f.Developers) > 0 || len(f.Publishers) > 0
}
