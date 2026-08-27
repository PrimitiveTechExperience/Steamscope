package models

type Game struct {
	AppID              int
	Name               string
	URL                string
	Developer          string
	Publisher          string
	ReleaseDate        string
	Price              string
	OriginalPrice      string
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