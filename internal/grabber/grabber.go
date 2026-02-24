package grabber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
)

// Grabber downloads NZB files and sends them to SABnzbd.
type Grabber struct {
	proxyClient  *http.Client // for downloading NZBs from indexers
	directClient *http.Client // for communicating with SABnzbd
	sabnzbdURL   string
	sabnzbdKey   string
}

func New(proxyClient, directClient *http.Client, sabnzbdURL, sabnzbdKey string) *Grabber {
	return &Grabber{
		proxyClient:  proxyClient,
		directClient: directClient,
		sabnzbdURL:   sabnzbdURL,
		sabnzbdKey:   sabnzbdKey,
	}
}

// UpdateConfig updates the SABnzbd connection details.
func (g *Grabber) UpdateConfig(sabnzbdURL, sabnzbdKey string) {
	g.sabnzbdURL = sabnzbdURL
	g.sabnzbdKey = sabnzbdKey
}

// GrabNZB downloads an NZB from the indexer and sends it to SABnzbd.
// Returns the SABnzbd nzo_id for tracking.
func (g *Grabber) GrabNZB(nzbURL, nzbName string) (string, error) {
	slog.Info("grabbing NZB", "name", nzbName, "url", nzbURL)

	// Download NZB via proxy (it's on the indexer's domain)
	resp, err := g.proxyClient.Get(nzbURL)
	if err != nil {
		return "", fmt.Errorf("download NZB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("NZB download HTTP %d", resp.StatusCode)
	}

	nzbData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read NZB: %w", err)
	}

	// Send NZB to SABnzbd via direct client (local network)
	nzoID, err := g.sendToSABnzbd(nzbData, nzbName)
	if err != nil {
		return "", fmt.Errorf("send to SABnzbd: %w", err)
	}

	slog.Info("NZB sent to SABnzbd", "name", nzbName, "nzo_id", nzoID)
	return nzoID, nil
}

// sendToSABnzbd uploads an NZB file to SABnzbd.
func (g *Grabber) sendToSABnzbd(nzbData []byte, nzbName string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("nzbfile", nzbName+".nzb")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(nzbData); err != nil {
		return "", err
	}
	w.Close()

	u := fmt.Sprintf("%s/api?mode=addfile&cat=mediaforge&apikey=%s&nzbname=%s&output=json",
		g.sabnzbdURL, g.sabnzbdKey, url.QueryEscape(nzbName))

	req, err := http.NewRequest("POST", u, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := g.directClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Status bool     `json:"status"`
		NzoIDs []string `json:"nzo_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode SABnzbd response: %w", err)
	}

	if !result.Status || len(result.NzoIDs) == 0 {
		return "", fmt.Errorf("SABnzbd rejected the NZB")
	}

	return result.NzoIDs[0], nil
}

// -- SABnzbd Queue/History polling --

type SABQueue struct {
	Slots []SABQueueSlot `json:"slots"`
	Speed string         `json:"speed"`
}

type SABQueueSlot struct {
	NzoID      string `json:"nzo_id"`
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	Percentage string `json:"percentage"`
	MBLeft     string `json:"mbleft"`
	MB         string `json:"mb"`
	TimeLeft   string `json:"timeleft"`
}

type SABHistory struct {
	Slots []SABHistorySlot `json:"slots"`
}

type SABHistorySlot struct {
	NzoID       string `json:"nzo_id"`
	Name        string `json:"name"`
	Status      string `json:"status"` // Completed, Failed, etc.
	Storage     string `json:"storage"` // final path
	FailMessage string `json:"fail_message"`
}

// GetQueue returns the current SABnzbd download queue.
func (g *Grabber) GetQueue() (*SABQueue, error) {
	u := fmt.Sprintf("%s/api?mode=queue&output=json&apikey=%s", g.sabnzbdURL, g.sabnzbdKey)
	resp, err := g.directClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Queue SABQueue `json:"queue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result.Queue, nil
}

// GetHistory returns the SABnzbd download history.
func (g *Grabber) GetHistory() (*SABHistory, error) {
	u := fmt.Sprintf("%s/api?mode=history&output=json&apikey=%s", g.sabnzbdURL, g.sabnzbdKey)
	resp, err := g.directClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		History SABHistory `json:"history"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result.History, nil
}

// DeleteFromQueue removes a download from the SABnzbd queue.
func (g *Grabber) DeleteFromQueue(nzoID string) error {
	u := fmt.Sprintf("%s/api?mode=queue&name=delete&value=%s&output=json&apikey=%s",
		g.sabnzbdURL, nzoID, g.sabnzbdKey)
	resp, err := g.directClient.Get(u)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// TestConnection verifies SABnzbd is reachable.
func (g *Grabber) TestConnection() error {
	u := fmt.Sprintf("%s/api?mode=version&output=json&apikey=%s", g.sabnzbdURL, g.sabnzbdKey)
	resp, err := g.directClient.Get(u)
	if err != nil {
		return fmt.Errorf("SABnzbd unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SABnzbd HTTP %d", resp.StatusCode)
	}
	return nil
}
