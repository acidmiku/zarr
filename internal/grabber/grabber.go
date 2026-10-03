package grabber

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Grabber downloads NZB files and sends them to SABnzbd.
type Grabber struct {
	mu           sync.RWMutex
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
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sabnzbdURL = sabnzbdURL
	g.sabnzbdKey = sabnzbdKey
}

// GrabNZB downloads an NZB from the indexer and sends it to SABnzbd.
// Returns the SABnzbd nzo_id for tracking.
func (g *Grabber) GrabNZB(nzbURL, nzbName string) (string, error) {
	slog.Info("grabbing NZB", "name", nzbName)

	// Download NZB via proxy (it's on the indexer's domain)
	resp, err := g.proxyClient.Get(nzbURL)
	if err != nil {
		return "", fmt.Errorf("download NZB: request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("NZB download HTTP %d", resp.StatusCode)
	}

	nzbData, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20+1))
	if err != nil {
		return "", fmt.Errorf("read NZB: %w", err)
	}

	if len(nzbData) > 32<<20 {
		return "", fmt.Errorf("NZB exceeds 32 MiB limit")
	}
	var document struct {
		XMLName xml.Name `xml:"nzb"`
	}
	if err := xml.Unmarshal(nzbData, &document); err != nil {
		return "", fmt.Errorf("indexer did not return a valid NZB")
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

	u := g.apiURL("addfile", url.Values{"cat": {"mediaforge"}, "nzbname": {nzbName}})

	req, err := http.NewRequest("POST", u, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := g.directClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("SABnzbd request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SABnzbd HTTP %d", resp.StatusCode)
	}

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
	Status      string `json:"status"`  // Completed, Failed, etc.
	Storage     string `json:"storage"` // final path
	FailMessage string `json:"fail_message"`
}

// apiURL safely encodes credentials and supports a reverse-proxy base path.
func (g *Grabber) apiURL(mode string, extra url.Values) string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := url.Parse(strings.TrimRight(g.sabnzbdURL, "/") + "/api")
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("mode", mode)
	q.Set("output", "json")
	q.Set("apikey", g.sabnzbdKey)
	for key, values := range extra {
		q[key] = values
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (g *Grabber) call(mode string, params url.Values, target any) error {
	resp, err := g.directClient.Get(g.apiURL(mode, params))
	if err != nil {
		return fmt.Errorf("SABnzbd request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SABnzbd HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read SABnzbd response: %w", err)
	}
	var envelope struct {
		Status *bool  `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("invalid SABnzbd response: %w", err)
	}
	if envelope.Error != "" || (envelope.Status != nil && !*envelope.Status) {
		return fmt.Errorf("SABnzbd rejected the request; check credentials and configuration")
	}
	return json.Unmarshal(body, target)
}

func (g *Grabber) GetQueue() (*SABQueue, error) {
	var result struct {
		Queue *SABQueue `json:"queue"`
	}
	if err := g.call("queue", nil, &result); err != nil {
		return nil, err
	}
	if result.Queue == nil {
		return nil, fmt.Errorf("SABnzbd response missing queue")
	}
	return result.Queue, nil
}

func (g *Grabber) GetHistory() (*SABHistory, error) {
	var result struct {
		History *SABHistory `json:"history"`
	}
	if err := g.call("history", url.Values{"limit": {"1000"}}, &result); err != nil {
		return nil, err
	}
	if result.History == nil {
		return nil, fmt.Errorf("SABnzbd response missing history")
	}
	return result.History, nil
}

func (g *Grabber) DeleteFromQueue(nzoID string) error {
	var result struct {
		Status bool `json:"status"`
	}
	if err := g.call("queue", url.Values{"name": {"delete"}, "value": {nzoID}}, &result); err != nil {
		return err
	}
	if !result.Status {
		return fmt.Errorf("SABnzbd did not confirm deletion")
	}
	return nil
}

// TestConnection uses an authenticated endpoint; version is public on some SABnzbd versions.
func (g *Grabber) TestConnection() error {
	_, err := g.GetQueue()
	return err
}
