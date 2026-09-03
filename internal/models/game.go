package models

import "time"

type Game struct {
	AppID              int
	Name               string
	URL                string
	Developer          string
	Publisher          string
	ReleaseDate        time.Time
	Price              float64
	OriginalPrice      float64
	DiscountPercentage int
	Genres             []string
	Tags               []string
	ReviewScore        string
	ReviewCount        int
	Description        string
	WindowsCompatible  bool
	LinuxCompatible    bool
	MacCompatible      bool
}