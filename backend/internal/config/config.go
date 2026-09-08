package config

import (
	"encoding/json"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Steam SteamConfig
}

type SteamConfig struct {
	CookieFilePath string
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
		},
	}
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