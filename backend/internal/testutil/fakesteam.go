package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

// FakeSteam stands in for Steam's Web API and store API.
type FakeSteam struct {
	mu        sync.Mutex
	wishlists map[string][]int // by Steam ID
	names     map[int]string
	Calls     int // wishlist requests served
	Fail      bool
}

// SetWishlist sets what Steam reports for a Steam ID.
func (f *FakeSteam) SetWishlist(steamID string, appIDs ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wishlists[steamID] = appIDs
}

// SetName sets a store name for an app.
func (f *FakeSteam) SetName(appID int, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names[appID] = name
}

// FakeSteam points the API at a fake Steam with a key configured.
func (a *App) FakeSteam() *FakeSteam {
	a.T.Helper()
	f := &FakeSteam{wishlists: map[string][]int{}, names: map[int]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/IWishlistService/GetWishlist/v1/":
			f.Calls++
			if f.Fail {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			items := []map[string]int{}
			for _, id := range f.wishlists[r.URL.Query().Get("steamid")] {
				items = append(items, map[string]int{"appid": id})
			}
			if len(items) == 0 {
				w.Write([]byte(`{"response":{}}`)) // Steam answers a private wishlist like an empty one
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"items": items}})
		case "/api/appdetails":
			id := r.URL.Query().Get("appids")
			n, _ := strconv.Atoi(id)
			name, ok := f.names[n]
			entry := map[string]any{"success": ok}
			if ok {
				entry["data"] = map[string]any{"name": name}
			}
			json.NewEncoder(w).Encode(map[string]any{id: entry})
		default:
			http.NotFound(w, r)
		}
	}))
	a.T.Cleanup(srv.Close)
	a.H.SteamAPI = steam.NewWebAPIAt("test-key", srv.URL, srv.URL)
	return f
}
