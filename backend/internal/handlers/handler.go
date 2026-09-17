package handlers

import (
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

type Handler struct {
	DB *database.DB
}

type GameResponse struct {
	Games []models.Game `json:"games"`
	Total int 		 `json:"total"`
	Limit int 		 `json:"limit"`
	Offset int 		 `json:"offset"`
}

type ReviewResponse struct {
	Reviews []models.Review `json:"reviews"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

func New(db *database.DB) *Handler {
	return &Handler{
		DB: db,
	}
}