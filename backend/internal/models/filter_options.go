package models

type FilterOptions struct {
	Genres     []string `json:"genres"`
	Tags       []string `json:"tags"`
	Developers []string `json:"developers"`
	Publishers []string `json:"publishers"`
	Languages  []string `json:"languages"`
}
