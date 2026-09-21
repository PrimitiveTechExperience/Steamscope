package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Steam SteamConfig
	ITADAPIKey string
}

type SteamConfig struct {
	CookieFilePath string
	ReviewFilter string
	ReviewMaxReviews int
	ReviewLanguage string
	BaseURL string
	TrackedAppIDs []int
}

type Cookie struct{
	Name  string `json:"name"`
	Value string `json:"value"`
	Domain string `json:"domain"`
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found or could not be loaded")
	}

	return &Config{
		Steam: SteamConfig{
			CookieFilePath: getEnv("STEAM_COOKIE_FILE_PATH", "./internal/config/config.json"),
			ReviewFilter: getEnv("REVIEW_FILTER", "recent"),
			ReviewMaxReviews: func() int {
				value, err := strconv.Atoi(getEnv("REVIEW_MAX_REVIEWS", "25"))
				if err != nil {
					log.Printf("Invalid REVIEW_MAX_REVIEWS value, using default: 25")
					return 25
				}
				return value
			}(),
			ReviewLanguage: getEnv("REVIEW_LANGUAGE", "english"),
			BaseURL: getEnv("STEAM_BASE_URL", "https://store.steampowered.com"),
			TrackedAppIDs: parseAppIDs(getEnv("TRACKED_APP_IDS", "730,570,440,578080,4000,550,252490")),
		},
		ITADAPIKey: getEnv("ITAD_API_KEY", ""),
	}
}

func parseAppIDs(csv string) []int {
	parts := strings.Split(csv, ",")
	appIDs := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		appID, err := strconv.Atoi(part)
		if err != nil {
			log.Printf("Invalid app ID %q in TRACKED_APP_IDS, skipping", part)
			continue
		}
		appIDs = append(appIDs, appID)
	}
	return appIDs
}


func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Printf("Environment variable %s not set, using default: %s", key, defaultValue)
		return defaultValue
	}
	return value
}

func LoadCookies(path string) ([]Cookie, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cookies []Cookie
	if err := json.Unmarshal(file, &cookies); err != nil {
		return nil, err
	}
	return cookies, nil
}