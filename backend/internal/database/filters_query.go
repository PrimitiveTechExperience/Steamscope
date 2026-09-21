package database

import (
	"context"
	"fmt"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

func (db *DB) queryDistinctColumn(ctx context.Context, query string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query distinct values: %w", err)
	}
	defer rows.Close()

	values := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("failed to scan distinct value: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read distinct values: %w", err)
	}
	return values, nil
}

func (db *DB) GetFilterOptions(ctx context.Context) (*models.FilterOptions, error) {
	genres, err := db.queryDistinctColumn(ctx, "SELECT genre FROM genres ORDER BY genre")
	if err != nil {
		return nil, err
	}
	tags, err := db.queryDistinctColumn(ctx, "SELECT tag FROM tags ORDER BY tag")
	if err != nil {
		return nil, err
	}
	developers, err := db.queryDistinctColumn(ctx, "SELECT developer FROM developers ORDER BY developer")
	if err != nil {
		return nil, err
	}
	publishers, err := db.queryDistinctColumn(ctx, "SELECT publisher FROM publishers ORDER BY publisher")
	if err != nil {
		return nil, err
	}
	languages, err := db.queryDistinctColumn(ctx, "SELECT language FROM languages ORDER BY language")
	if err != nil {
		return nil, err
	}

	return &models.FilterOptions{
		Genres:     genres,
		Tags:       tags,
		Developers: developers,
		Publishers: publishers,
		Languages:  languages,
	}, nil
}
