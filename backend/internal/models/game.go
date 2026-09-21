package models

import "time"

type Game struct {
	AppID              int `json:"app_id"`
	// General information
	Name               string `json:"name"`
	URL                string `json:"url"`
	Description        string `json:"description"`
	HeaderImage        string `json:"header_image"`
	ReleaseDate        time.Time `json:"release_date"`
	// Supported languages are stored as a slice of strings, as a game can support multiple languages.
	SupportedLanguages  []string `json:"supported_languages"`
	// Developers and Publishers are stored as slices of strings, as a game can have multiple developers and publishers.
	Developers         []string `json:"developers"`
	Publishers         []string `json:"publishers"`
	// Price and discount information
	Price              float64 `json:"price"`
	OriginalPrice      float64 `json:"original_price"`
	DiscountPercentage int `json:"discount_percentage"`
	// The genres and tags are stored as slices of strings, as a game can have multiple genres and tags.
	Genres             []string `json:"genres"`
	Tags               []string `json:"tags"`
	// Review Overview
	ReviewScore        string `json:"review_score"`
	ReviewCount        int `json:"review_count"`
	Reviews 		   []Review `json:"reviews"`
	// Compatibility flags
	WindowsCompatible  bool`json:"windows_compatible"`
	LinuxCompatible    bool`json:"linux_compatible"`
	MacCompatible      bool`json:"mac_compatible"`
}

// PricePoint represents a single day's recorded price for a game.
type PricePoint struct {
	Date               time.Time `json:"date"`
	Price              float64   `json:"price"`
	OriginalPrice      float64   `json:"original_price"`
	DiscountPercentage int       `json:"discount_percentage"`
}