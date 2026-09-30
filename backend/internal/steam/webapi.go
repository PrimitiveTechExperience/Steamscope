package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
)

const webAPIBase = "https://api.steampowered.com"

var ErrNoAPIKey = errors.New("STEAM_WEB_API_KEY is not configured")

type WebAPI struct {
	apiKey string
	client *http.Client
}

func NewWebAPI(apiKey string) *WebAPI {
	return &WebAPI{apiKey: apiKey, client: &http.Client{Timeout: 10 * time.Second}}
}

var personaStates = map[int]string{
	0: "Offline", 1: "Online", 2: "Busy", 3: "Away", 4: "Snooze", 5: "Looking to trade", 6: "Looking to play",
}

// GetProfile returns a player's summary plus their recently played games.
// Recently played is best-effort: it's empty for private profiles.
func (a *WebAPI) GetProfile(ctx context.Context, steamID string) (*models.SteamProfile, error) {
	if a.apiKey == "" {
		return nil, ErrNoAPIKey
	}

	var summary struct {
		Response struct {
			Players []struct {
				SteamID       string `json:"steamid"`
				PersonaName   string `json:"personaname"`
				ProfileURL    string `json:"profileurl"`
				AvatarFull    string `json:"avatarfull"`
				PersonaState  int    `json:"personastate"`
				GameExtraInfo string `json:"gameextrainfo"`
			} `json:"players"`
		} `json:"response"`
	}
	if err := a.get(ctx, "/ISteamUser/GetPlayerSummaries/v2/", url.Values{"steamids": {steamID}}, &summary); err != nil {
		return nil, err
	}
	if len(summary.Response.Players) == 0 {
		return nil, fmt.Errorf("steam profile %s not found", steamID)
	}
	player := summary.Response.Players[0]

	profile := &models.SteamProfile{
		SteamID:          player.SteamID,
		PersonaName:      player.PersonaName,
		AvatarURL:        player.AvatarFull,
		ProfileURL:       player.ProfileURL,
		Status:           personaStates[player.PersonaState],
		CurrentlyPlaying: player.GameExtraInfo,
		RecentlyPlayed:   []models.SteamPlayedGame{},
	}

	var recent struct {
		Response struct {
			Games []struct {
				AppID           int    `json:"appid"`
				Name            string `json:"name"`
				Playtime2Weeks  int    `json:"playtime_2weeks"`
				PlaytimeForever int    `json:"playtime_forever"`
				ImgIconURL      string `json:"img_icon_url"`
			} `json:"games"`
		} `json:"response"`
	}
	params := url.Values{"steamid": {steamID}, "count": {"5"}}
	if err := a.get(ctx, "/IPlayerService/GetRecentlyPlayedGames/v1/", params, &recent); err == nil {
		for _, g := range recent.Response.Games {
			icon := ""
			if g.ImgIconURL != "" {
				icon = fmt.Sprintf("https://media.steampowered.com/steamcommunity/public/images/apps/%d/%s.jpg", g.AppID, g.ImgIconURL)
			}
			profile.RecentlyPlayed = append(profile.RecentlyPlayed, models.SteamPlayedGame{
				AppID:            g.AppID,
				Name:             g.Name,
				IconURL:          icon,
				PlaytimeTwoWeeks: g.Playtime2Weeks,
				PlaytimeForever:  g.PlaytimeForever,
			})
		}
	}
	return profile, nil
}

func (a *WebAPI) get(ctx context.Context, path string, params url.Values, out any) error {
	params.Set("key", a.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, webAPIBase+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		// *url.Error embeds the full request URL, which contains the API key.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("steam web api %s failed: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("steam web api %s returned %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
