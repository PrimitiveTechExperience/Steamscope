package models

import "time"

type Game struct {
	AppID              int
	// General information
	Name               string
	URL                string
	Description        string
	ReleaseDate        time.Time
	// Supported languages are stored as a slice of strings, as a game can support multiple languages.
	SupportedLanguages  []string
	// Developers and Publishers are stored as slices of strings, as a game can have multiple developers and publishers.
	Developers         []string
	Publishers         []string
	// Price and discount information
	Price              float64
	OriginalPrice      float64
	DiscountPercentage int
	// The genres and tags are stored as slices of strings, as a game can have multiple genres and tags.
	Genres             []string
	Tags               []string
	// Review Overview
	ReviewScore        string
	ReviewCount        int
	// Compatibility flags
	WindowsCompatible  bool
	LinuxCompatible    bool
	MacCompatible      bool
}