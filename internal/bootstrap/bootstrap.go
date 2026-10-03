// Package bootstrap wires fresh bundled download clients without manual key copying.
package bootstrap

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mediaforge/internal/database"
)

func token() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func writeNew(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if exists(path) {
		return fmt.Errorf("refusing to replace an existing downloader configuration")
	}
	return writeAtomic(path, data)
}

func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".connections-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// Init never replaces an existing downloader configuration. Generated credentials
// are saved before creating clients, so retrying an interrupted first run is safe.
func Init(configDir, sabDir, qbtDir string) error {
	path := filepath.Join(configDir, "bundled-connections.json")
	credentials := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(data, &credentials); err != nil {
			return fmt.Errorf("read bundled connections: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	sabPath := filepath.Join(sabDir, "sabnzbd.ini")
	qbtPath := filepath.Join(qbtDir, "qBittorrent", "qBittorrent.conf")
	if !exists(sabPath) && credentials["sabnzbd_api_key"] == "" {
		key, err := token()
		if err != nil {
			return err
		}
		credentials["sabnzbd_api_key"] = key
		credentials["sabnzbd_url"] = "http://sabnzbd:8080"
	}
	if !exists(qbtPath) && credentials["qbittorrent_password"] == "" {
		password, err := token()
		if err != nil {
			return err
		}
		credentials["qbittorrent_password"] = password
		credentials["qbittorrent_username"] = "admin"
		credentials["qbittorrent_url"] = "http://qbittorrent:9090"
		credentials["qbittorrent_enabled"] = "true"
	}
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	if err := writeAtomic(path, data); err != nil {
		return err
	}
	if !exists(sabPath) {
		content := fmt.Sprintf("[misc]\nhost = 0.0.0.0\nport = 8080\napi_key = %s\ncomplete_dir = /data/usenet/complete\ndownload_dir = /data/usenet/incomplete\npermissions = 0775\nhost_whitelist = sabnzbd, localhost\n[servers]\n[categories]\n[[mediaforge]]\nname = mediaforge\npp = 3\nscript = None\ndir = \n", credentials["sabnzbd_api_key"])
		if err := writeNew(sabPath, []byte(content)); err != nil {
			return err
		}
	}
	if !exists(qbtPath) {
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return err
		}
		hash, err := pbkdf2.Key(sha512.New, credentials["qbittorrent_password"], salt, 100000, 64)
		if err != nil {
			return err
		}
		encoded := base64.StdEncoding.EncodeToString(salt) + ":" + base64.StdEncoding.EncodeToString(hash)
		content := fmt.Sprintf("[LegalNotice]\nAccepted=true\n[BitTorrent]\nSession\\DefaultSavePath=/data/torrents\nSession\\Port=6881\n[Preferences]\nWebUI\\Address=*\nWebUI\\Port=9090\nWebUI\\Username=admin\nWebUI\\Password_PBKDF2=\"@ByteArray(%s)\"\nWebUI\\ServerDomains=\"qbittorrent;localhost;127.0.0.1\"\n", encoded)
		if err := writeNew(qbtPath, []byte(content)); err != nil {
			return err
		}
	}
	return nil
}
func Seed(db *database.DB, configDir string) error {
	data, err := os.ReadFile(filepath.Join(configDir, "bundled-connections.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var credentials map[string]string
	if err = json.Unmarshal(data, &credentials); err != nil {
		return err
	}
	for key, value := range credentials {
		if !strings.HasPrefix(key, "sabnzbd_") && !strings.HasPrefix(key, "qbittorrent_") {
			continue
		}
		if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, key, value); err != nil {
			return err
		}
	}
	return nil
}
