package qbt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"sync"
)

// Client communicates with a qBittorrent instance via its Web API.
type Client struct {
	client   *http.Client
	baseURL  string
	username string
	password string
	sid      string // SID cookie from auth
	mu       sync.Mutex
}

// Torrent represents a torrent in qBittorrent.
type Torrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	DlSpeed     int64   `json:"dlspeed"`
	UpSpeed     int64   `json:"upspeed"`
	ETA         int64   `json:"eta"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
	Tags        string  `json:"tags"`
	Ratio       float64 `json:"ratio"`
	AddedOn     int64   `json:"added_on"`
	Size        int64   `json:"size"`
}

// New creates a new qBittorrent client.
func New(httpClient *http.Client, baseURL, username, password string) *Client {
	return &Client{
		client:   httpClient,
		baseURL:  baseURL,
		username: username,
		password: password,
	}
}

// UpdateConfig updates the connection details.
func (c *Client) UpdateConfig(baseURL, username, password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = baseURL
	c.username = username
	c.password = password
	c.sid = "" // force re-login
}

// Login authenticates with qBittorrent and stores the SID cookie.
func (c *Client) Login() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.login()
}

func (c *Client) login() error {
	data := url.Values{
		"username": {c.username},
		"password": {c.password},
	}

	resp, err := c.client.PostForm(c.baseURL+"/api/v2/auth/login", data)
	if err != nil {
		return fmt.Errorf("qbt login: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Ok." {
		return fmt.Errorf("qbt login failed: %s", string(body))
	}

	// Extract SID from cookies
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SID" {
			c.sid = cookie.Value
			slog.Debug("qbt login successful")
			return nil
		}
	}

	return fmt.Errorf("qbt login: no SID cookie returned")
}

// doRequest performs an authenticated request with auto-reauth on 403.
func (c *Client) doRequest(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	sid := c.sid
	c.mu.Unlock()

	if sid != "" {
		req.AddCookie(&http.Cookie{Name: "SID", Value: sid})
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	// Auto-reauth on 403
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		c.mu.Lock()
		err = c.login()
		sid = c.sid
		c.mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("qbt reauth: %w", err)
		}

		// Rebuild cookies on the original request
		req.Header.Del("Cookie")
		req.AddCookie(&http.Cookie{Name: "SID", Value: sid})

		resp, err = c.client.Do(req)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// AddTorrent sends a .torrent file to qBittorrent.
// Returns the info hash.
func (c *Client) AddTorrent(torrentData []byte, filename string, downloadID int) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Torrent file
	fw, err := w.CreateFormFile("torrents", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(torrentData); err != nil {
		return "", err
	}

	// Category and tags
	w.WriteField("category", "zarr")
	w.WriteField("tags", fmt.Sprintf("dl_%d", downloadID))
	w.WriteField("savepath", "/data/torrents")
	w.Close()

	req, err := http.NewRequest("POST", c.baseURL+"/api/v2/torrents/add", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.doRequest(req)
	if err != nil {
		return "", fmt.Errorf("qbt add torrent: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Ok." {
		return "", fmt.Errorf("qbt add torrent: %s", string(body))
	}

	slog.Info("torrent sent to qBittorrent", "filename", filename, "download_id", downloadID)
	return "", nil // hash is extracted later via polling
}

// GetTorrents returns all torrents in the "zarr" category.
func (c *Client) GetTorrents() ([]Torrent, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v2/torrents/info?category=zarr", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("qbt get torrents: %w", err)
	}
	defer resp.Body.Close()

	var torrents []Torrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("qbt decode torrents: %w", err)
	}

	return torrents, nil
}

// DeleteTorrent removes a torrent from qBittorrent.
func (c *Client) DeleteTorrent(hash string, deleteFiles bool) error {
	data := url.Values{
		"hashes":      {hash},
		"deleteFiles": {fmt.Sprintf("%t", deleteFiles)},
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/v2/torrents/delete", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.doRequest(req)
	if err != nil {
		return fmt.Errorf("qbt delete torrent: %w", err)
	}
	resp.Body.Close()
	return nil
}

// TestConnection verifies qBittorrent is reachable by logging in and fetching the version.
func (c *Client) TestConnection() error {
	c.mu.Lock()
	err := c.login()
	c.mu.Unlock()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/v2/app/version", nil)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return fmt.Errorf("qbt version check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbt HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	slog.Info("qBittorrent connected", "version", string(body))
	return nil
}
