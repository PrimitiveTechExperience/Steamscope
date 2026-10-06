package steam

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// maxWishlist bounds how many wishlist entries are used. Steam allows 10,000.
const maxWishlist = 2000

// GetWishlist returns the app IDs on a player's wishlist, in Steam's order.
//
// Steam answers a private wishlist exactly like an empty one (an empty
// response object), so an empty result means "empty or private".
func (a *WebAPI) GetWishlist(ctx context.Context, steamID string) ([]int, error) {
	if a.apiKey == "" {
		return nil, ErrNoAPIKey
	}
	var out struct {
		Response struct {
			Items []struct {
				AppID int `json:"appid"`
			} `json:"items"`
		} `json:"response"`
	}
	if err := a.get(ctx, "/IWishlistService/GetWishlist/v1/", url.Values{"steamid": {steamID}}, &out); err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	ids := make([]int, 0, len(out.Response.Items))
	for _, item := range out.Response.Items {
		if item.AppID <= 0 || seen[item.AppID] {
			continue
		}
		seen[item.AppID] = true
		ids = append(ids, item.AppID)
		if len(ids) == maxWishlist {
			break
		}
	}
	return ids, nil
}

// AppName looks up a store item's name, or returns "" if it can't be found
// quickly. It is only used to make a list of games readable, so a failure is
// not an error.
func (a *WebAPI) AppName(ctx context.Context, appID int) string {
	q := url.Values{"appids": {fmt.Sprint(appID)}, "filters": {"basic"}, "cc": {"us"}, "l": {"english"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.storeBase+"/api/appdetails?"+q.Encode(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := a.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if decodeJSON(resp.Body, &out) != nil {
		return ""
	}
	entry := out[fmt.Sprint(appID)]
	if !entry.Success {
		return ""
	}
	return entry.Data.Name
}
