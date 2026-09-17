package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/pashagolub/pgxmock/v5"
)

func TestInsertGame(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	releaseDate := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	game := models.Game{
		AppID:              123,
		Name:               "Test Game",
		URL:                "https://example.com/game/123",
		Description:        "A test game",
		ReleaseDate:        releaseDate,
		Price:              19.99,
		OriginalPrice:      29.99,
		DiscountPercentage: 33,
		ReviewScore:        "Very Positive",
		ReviewCount:        100,
		WindowsCompatible:  true,
		LinuxCompatible:    true,
		MacCompatible:      false,
	}

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`SELECT app_id FROM games WHERE app_id = \$1 FOR UPDATE`).
		WithArgs(game.AppID).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mockPool.ExpectExec(`INSERT INTO games \(\s*app_id, name, url, description, release_date, price, original_price, discount_percentage, review_score, review_count, windows_compatible, linux_compatible, mac_compatible\s*\)\s*VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12, \$13\)\s*ON CONFLICT \(app_id\) DO UPDATE SET`).
		WithArgs(
			game.AppID,
			game.Name,
			game.URL,
			game.Description,
			game.ReleaseDate,
			game.Price,
			game.OriginalPrice,
			game.DiscountPercentage,
			8,
			game.ReviewCount,
			game.WindowsCompatible,
			game.LinuxCompatible,
			game.MacCompatible,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mockPool.ExpectCommit()

	db := &DB{Pool: mockPool}
	if err := db.InsertGame(ctx, game); err != nil {
		t.Fatalf("InsertGame() returned an error: %v", err)
	}

	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGame_Error(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	game := models.Game{
		AppID: 123,
	}

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`SELECT app_id FROM games WHERE app_id = \$1 FOR UPDATE`).
		WithArgs(game.AppID).
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mockPool.ExpectExec(`INSERT INTO games \(\s*app_id, name, url, description, release_date, price, original_price, discount_percentage, review_score, review_count, windows_compatible, linux_compatible, mac_compatible\s*\)\s*VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12, \$13\)\s*ON CONFLICT \(app_id\) DO UPDATE SET`).
		WithArgs(
			game.AppID,
			game.Name,
			game.URL,
			game.Description,
			game.ReleaseDate,
			game.Price,
			game.OriginalPrice,
			game.DiscountPercentage,
			0,
			game.ReviewCount,
			game.WindowsCompatible,
			game.LinuxCompatible,
			game.MacCompatible,
		).
		WillReturnError(fmt.Errorf("insert error"))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	if err := db.InsertGame(ctx, game); err == nil {
		t.Fatalf("InsertGame() did not return an error when expected")
	}

	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameTag(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_tags \(app_id, tag_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, tag_id\) DO UPDATE SET tag_id = EXCLUDED.tag_id`).
		WithArgs(123, 456).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameTag(ctx, tx, 123, 456); err != nil {
		t.Fatalf("InsertGameTag() returned an error: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameTag_Error(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_tags \(app_id, tag_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, tag_id\) DO UPDATE SET tag_id = EXCLUDED.tag_id`).
		WithArgs(123, 456).
		WillReturnError(fmt.Errorf("insert error"))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameTag(ctx, tx, 123, 456); err == nil {
		t.Fatalf("InsertGameTag() did not return an error when expected")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameGenre(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_genres \(app_id, genre_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, genre_id\) DO UPDATE SET genre_id = EXCLUDED.genre_id`).
		WithArgs(123, 456).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameGenre(ctx, tx, 123, 456); err != nil {
		t.Fatalf("InsertGameGenre() returned an error: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameGenre_Error(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_genres \(app_id, genre_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, genre_id\) DO UPDATE SET genre_id = EXCLUDED.genre_id`).
		WithArgs(123, 456).
		WillReturnError(fmt.Errorf("insert error"))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameGenre(ctx, tx, 123, 456); err == nil {
		t.Fatalf("InsertGameGenre() did not return an error when expected")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameDeveloper(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_developers \(app_id, developer_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, developer_id\) DO UPDATE SET developer_id = EXCLUDED.developer_id`).
		WithArgs(123, 456).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameDeveloper(ctx, tx, 123, 456); err != nil {
		t.Fatalf("InsertGameDeveloper() returned an error: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameDeveloper_Error(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_developers \(app_id, developer_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, developer_id\) DO UPDATE SET developer_id = EXCLUDED.developer_id`).
		WithArgs(123, 456).
		WillReturnError(fmt.Errorf("insert error"))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameDeveloper(ctx, tx, 123, 456); err == nil {
		t.Fatalf("InsertGameDeveloper() did not return an error when expected")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGamePublisher(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_publishers \(app_id, publisher_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, publisher_id\) DO UPDATE SET publisher_id = EXCLUDED.publisher_id`).
		WithArgs(123, 456).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGamePublisher(ctx, tx, 123, 456); err != nil {
		t.Fatalf("InsertGamePublisher() returned an error: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGamePublisher_Error(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_publishers \(app_id, publisher_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, publisher_id\) DO UPDATE SET publisher_id = EXCLUDED.publisher_id`).
		WithArgs(123, 456).
		WillReturnError(fmt.Errorf("insert error"))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGamePublisher(ctx, tx, 123, 456); err == nil {
		t.Fatalf("InsertGamePublisher() did not return an error when expected")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameLanguage(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_languages \(app_id, language_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, language_id\) DO UPDATE SET language_id = EXCLUDED.language_id`).
		WithArgs(123, 456).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameLanguage(ctx, tx, 123, 456); err != nil {
		t.Fatalf("InsertGameLanguage() returned an error: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}

func TestInsertGameLanguage_Error(t *testing.T) {
	ctx := context.Background()
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mockPool.Close()

	mockPool.ExpectBegin()
	mockPool.ExpectExec(`INSERT INTO game_languages \(app_id, language_id\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(app_id, language_id\) DO UPDATE SET language_id = EXCLUDED.language_id`).
		WithArgs(123, 456).
		WillReturnError(fmt.Errorf("insert error"))
	mockPool.ExpectRollback()

	db := &DB{Pool: mockPool}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin mock transaction: %v", err)
	}

	if err := db.InsertGameLanguage(ctx, tx, 123, 456); err == nil {
		t.Fatalf("InsertGameLanguage() did not return an error when expected")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to roll back mock transaction: %v", err)
	}
	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations were not met: %v", err)
	}
}
