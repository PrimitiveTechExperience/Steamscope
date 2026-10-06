package router_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/testutil"
)

func gameNames(t *testing.T, resp testutil.Response) []string {
	t.Helper()
	if resp.Code() != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code(), resp.Body.String())
	}
	var names []string
	for _, g := range resp.JSON()["games"].([]any) {
		names = append(names, g.(map[string]any)["name"].(string))
	}
	return names
}

func TestFilterGamesByDiscount(t *testing.T) {
	app := testutil.NewApp(t)
	for i, d := range []int{0, 10, 25, 50, 75, 100} {
		id := 810100 + i
		app.SeedGame(id, fmt.Sprintf("Deal %d", d), []string{"Dev"}, []string{"Pub"})
		if _, err := app.Pool.Exec(context.Background(), `UPDATE games SET discount_percentage = $1 WHERE app_id = $2`, d, id); err != nil {
			t.Fatal(err)
		}
	}
	c := app.NewClient()

	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"Deal 0", "Deal 10", "Deal 100", "Deal 25", "Deal 50", "Deal 75"}},
		{"?minDiscount=0", []string{"Deal 0", "Deal 10", "Deal 100", "Deal 25", "Deal 50", "Deal 75"}},
		{"?minDiscount=1", []string{"Deal 10", "Deal 100", "Deal 25", "Deal 50", "Deal 75"}}, // "on sale"
		{"?minDiscount=25", []string{"Deal 100", "Deal 25", "Deal 50", "Deal 75"}},           // inclusive
		{"?minDiscount=50", []string{"Deal 100", "Deal 50", "Deal 75"}},
		{"?minDiscount=100", []string{"Deal 100"}},
	}
	for _, tc := range cases {
		if got := gameNames(t, c.Get("/api/games"+tc.query)); fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%q: got %v, want %v", tc.query, got, tc.want)
		}
	}

	t.Run("combines with other filters", func(t *testing.T) {
		got := gameNames(t, c.Get("/api/games?minDiscount=25&search=Deal%207"))
		if fmt.Sprint(got) != "[Deal 75]" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("each setting is cached on its own", func(t *testing.T) {
		c.Get("/api/games?minDiscount=75")
		if got := gameNames(t, c.Get("/api/games?minDiscount=75")); fmt.Sprint(got) != "[Deal 100 Deal 75]" {
			t.Errorf("cached 75 = %v", got)
		}
		if got := gameNames(t, c.Get("/api/games?minDiscount=100")); fmt.Sprint(got) != "[Deal 100]" {
			t.Errorf("a different threshold was served the 75 result: %v", got)
		}
	})

	t.Run("rejects values that are not a percentage", func(t *testing.T) {
		for _, q := range []string{"abc", "-1", "101", "5.5", "1e2"} {
			resp := c.Get("/api/games?minDiscount=" + q)
			if resp.Code() != http.StatusBadRequest || resp.Error() == "" {
				t.Errorf("minDiscount=%s: status %d body %s, want a JSON 400", q, resp.Code(), resp.Body.String())
			}
		}
	})
}

func TestRecentSearchRemembersTheDiscountFilter(t *testing.T) {
	app := testutil.NewApp(t)
	user := app.NewUser("disc")

	if got := user.Post("/api/me/recent-searches", map[string]any{"min_discount": 50}).Code(); got != http.StatusNoContent {
		t.Fatalf("status = %d", got)
	}
	// A discount alone is a real search; an out-of-range one is dropped and leaves nothing to save.
	user.Post("/api/me/recent-searches", map[string]any{"min_discount": 500})
	list := user.Get("/api/me/recent-searches").JSONArray()
	if len(list) != 1 || list[0].(map[string]any)["min_discount"] != float64(50) {
		t.Errorf("saved searches = %v, want just the 50%% one", list)
	}
}
