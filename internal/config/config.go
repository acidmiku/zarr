package config

import (
	"log/slog"
	"mediaforge/internal/database"
	"os"
)

type Config struct {
	ConfigDir    string
	MediaRoot    string
	TMDBApiKey   string
	Proxy        string
	SABnzbdURL   string
	SABnzbdKey   string
	LogLevel     string
	Port         string
}

// envMapping maps env vars to settings keys.
var envMapping = map[string]string{
	"MEDIAFORGE_TMDB_API_KEY":    "tmdb_api_key",
	"MEDIAFORGE_PROXY":           "proxy",
	"MEDIAFORGE_MEDIA_ROOT":      "media_root",
	"MEDIAFORGE_SABNZBD_URL":     "sabnzbd_url",
	"MEDIAFORGE_SABNZBD_API_KEY": "sabnzbd_api_key",
}

// Load reads configuration. Environment variables seed the DB on first run.
func Load(db *database.DB) *Config {
	// Seed settings from env vars if not already set
	for envKey, dbKey := range envMapping {
		if val := os.Getenv(envKey); val != "" {
			existing, _ := db.GetSetting(dbKey)
			if existing == "" {
				slog.Info("seeding setting from env", "key", dbKey)
				db.SetSetting(dbKey, val)
			}
		}
	}

	cfg := &Config{
		ConfigDir: getEnvDefault("MEDIAFORGE_CONFIG_DIR", "/config"),
		Port:      getEnvDefault("MEDIAFORGE_PORT", "9876"),
		LogLevel:  getEnvDefault("MEDIAFORGE_LOG_LEVEL", "info"),
	}

	cfg.TMDBApiKey, _ = db.GetSetting("tmdb_api_key")
	cfg.Proxy, _ = db.GetSetting("proxy")
	cfg.MediaRoot, _ = db.GetSetting("media_root")
	if cfg.MediaRoot == "" {
		cfg.MediaRoot = "/data/media"
	}
	cfg.SABnzbdURL, _ = db.GetSetting("sabnzbd_url")
	if cfg.SABnzbdURL == "" {
		cfg.SABnzbdURL = "http://sabnzbd:8080"
	}
	cfg.SABnzbdKey, _ = db.GetSetting("sabnzbd_api_key")

	return cfg
}

// Reload re-reads settings from the database.
func (c *Config) Reload(db *database.DB) {
	c.TMDBApiKey, _ = db.GetSetting("tmdb_api_key")
	c.Proxy, _ = db.GetSetting("proxy")
	c.MediaRoot, _ = db.GetSetting("media_root")
	if c.MediaRoot == "" {
		c.MediaRoot = "/data/media"
	}
	c.SABnzbdURL, _ = db.GetSetting("sabnzbd_url")
	c.SABnzbdKey, _ = db.GetSetting("sabnzbd_api_key")
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
