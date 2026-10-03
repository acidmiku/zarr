package bootstrap

import (
	"encoding/json"
	"mediaforge/internal/database"
	"os"
	"path/filepath"
	"testing"
)

func TestFreshBootstrapAndExistingConfigurationPreserved(t *testing.T) {
	root := t.TempDir()
	cfg, sab, qbt := filepath.Join(root, "cfg"), filepath.Join(root, "sab"), filepath.Join(root, "qbt")
	if err := Init(cfg, sab, qbt); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg, "bundled-connections.json")
	first, _ := os.ReadFile(path)
	var credentials map[string]string
	json.Unmarshal(first, &credentials)
	if len(credentials["sabnzbd_api_key"]) < 32 || len(credentials["qbittorrent_password"]) < 32 {
		t.Fatal("credentials were not generated")
	}
	custom := []byte("existing settings must survive")
	qbtPath := filepath.Join(qbt, "qBittorrent", "qBittorrent.conf")
	os.WriteFile(qbtPath, custom, 0600)
	if err := Init(cfg, sab, qbt); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("credentials rotated during restart")
	}
	actual, _ := os.ReadFile(qbtPath)
	if string(actual) != string(custom) {
		t.Fatal("existing configuration overwritten")
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Seed(db, cfg); err != nil {
		t.Fatal(err)
	}
	key, _ := db.GetSetting("sabnzbd_api_key")
	if key != credentials["sabnzbd_api_key"] {
		t.Fatal("key not seeded")
	}
	db.SetSetting("sabnzbd_api_key", "user-change")
	Seed(db, cfg)
	key, _ = db.GetSetting("sabnzbd_api_key")
	if key != "user-change" {
		t.Fatal("user settings overwritten")
	}
}
