package config

import (
	"log/slog"
	"mediaforge/internal/database"
	"os"
	"strconv"
)

type Config struct {
	ConfigDir              string
	MediaRoot              string
	TMDBApiKey             string
	Proxy                  string
	SABnzbdURL             string
	SABnzbdKey             string
	LogLevel               string
	Port                   string
	LastFMApiKey           string
	QBTEnabled             bool
	QBTURL                 string
	QBTUsername             string
	QBTPassword            string
	TorrentSeedHours       int
	TorrentRemoveAfterSeed bool
}

// envMapping maps env vars to settings keys.
var envMapping = map[string]string{
	"MEDIAFORGE_TMDB_API_KEY":    "tmdb_api_key",
	"MEDIAFORGE_PROXY":           "proxy",
	"MEDIAFORGE_MEDIA_ROOT":      "media_root",
	"MEDIAFORGE_SABNZBD_URL":     "sabnzbd_url",
	"MEDIAFORGE_SABNZBD_API_KEY": "sabnzbd_api_key",
	"MEDIAFORGE_LASTFM_API_KEY":  "lastfm_api_key",
	"MEDIAFORGE_QBT_URL":         "qbittorrent_url",
	"MEDIAFORGE_QBT_USERNAME":    "qbittorrent_username",
	"MEDIAFORGE_QBT_PASSWORD":    "qbittorrent_password",
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
	cfg.LastFMApiKey, _ = db.GetSetting("lastfm_api_key")

	// qBittorrent
	qbtEnabled, _ := db.GetSetting("qbittorrent_enabled")
	cfg.QBTEnabled = qbtEnabled == "true"
	cfg.QBTURL, _ = db.GetSetting("qbittorrent_url")
	if cfg.QBTURL == "" {
		cfg.QBTURL = "http://qbittorrent:8080"
	}
	cfg.QBTUsername, _ = db.GetSetting("qbittorrent_username")
	if cfg.QBTUsername == "" {
		cfg.QBTUsername = "admin"
	}
	cfg.QBTPassword, _ = db.GetSetting("qbittorrent_password")
	cfg.TorrentSeedHours = 24
	if v, _ := db.GetSetting("torrent_seed_time_hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.TorrentSeedHours = n
		}
	}
	removeAfter, _ := db.GetSetting("torrent_remove_after_seed")
	cfg.TorrentRemoveAfterSeed = removeAfter != "false" // default true

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
	c.LastFMApiKey, _ = db.GetSetting("lastfm_api_key")

	// qBittorrent
	qbtEnabled, _ := db.GetSetting("qbittorrent_enabled")
	c.QBTEnabled = qbtEnabled == "true"
	c.QBTURL, _ = db.GetSetting("qbittorrent_url")
	if c.QBTURL == "" {
		c.QBTURL = "http://qbittorrent:8080"
	}
	c.QBTUsername, _ = db.GetSetting("qbittorrent_username")
	if c.QBTUsername == "" {
		c.QBTUsername = "admin"
	}
	c.QBTPassword, _ = db.GetSetting("qbittorrent_password")
	c.TorrentSeedHours = 24
	if v, _ := db.GetSetting("torrent_seed_time_hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.TorrentSeedHours = n
		}
	}
	removeAfter, _ := db.GetSetting("torrent_remove_after_seed")
	c.TorrentRemoveAfterSeed = removeAfter != "false"
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
