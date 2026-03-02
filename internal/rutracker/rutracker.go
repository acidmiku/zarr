package rutracker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// DefaultForumIDs maps content types to Rutracker forum IDs.
var DefaultForumIDs = map[string][]int{
	"movie":  {7, 236, 2198, 22},
	"series": {189, 2366, 911, 842},
	"anime":  {33, 1105, 599},
	"music":  {737, 738, 1660, 429, 425, 1125, 409},
}

// Result represents a single search result from Rutracker.
type Result struct {
	TopicID  int
	Title    string
	Size     int64
	Seeders  int
	Leechers int
}

// Client communicates with the Rutracker forum.
type Client struct {
	client  *http.Client
	baseURL string
	session string // bb_session cookie
	limiter *rate.Limiter
	mu      sync.Mutex
}

// New creates a new Rutracker client.
func New(httpClient *http.Client, baseURL string) *Client {
	return &Client{
		client:  httpClient,
		baseURL: strings.TrimRight(baseURL, "/"),
		limiter: rate.NewLimiter(rate.Every(2*1e9), 1), // 1 request per 2 seconds
	}
}

// Login authenticates with Rutracker and stores the bb_session cookie.
func (c *Client) Login(username, password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := url.Values{
		"login_username": {username},
		"login_password": {password},
		"login":          {"Вход"},
	}

	// Don't follow redirects — we need the Set-Cookie header
	noRedirect := *c.client
	noRedirect.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := noRedirect.PostForm(c.baseURL+"/forum/login.php", data)
	if err != nil {
		return fmt.Errorf("rutracker login: %w", err)
	}
	defer resp.Body.Close()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "bb_session" {
			c.session = cookie.Value
			slog.Debug("rutracker login successful")
			return nil
		}
	}

	return fmt.Errorf("rutracker login failed: no bb_session cookie")
}

// Search queries Rutracker for releases matching the query in the given forum IDs.
func (c *Client) Search(query string, forumIDs []int, username, password string) ([]Result, error) {
	c.limiter.Wait(context.Background())

	// Ensure we're logged in
	if c.session == "" {
		if err := c.Login(username, password); err != nil {
			return nil, err
		}
	}

	// Build forum filter
	var forumParts []string
	for _, id := range forumIDs {
		forumParts = append(forumParts, fmt.Sprintf("f[]=%d", id))
	}

	data := url.Values{
		"nm": {query},
	}

	reqURL := c.baseURL + "/forum/tracker.php"
	bodyStr := data.Encode()
	if len(forumParts) > 0 {
		bodyStr += "&" + strings.Join(forumParts, "&")
	}

	req, err := http.NewRequest("POST", reqURL, strings.NewReader(bodyStr))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "bb_session", Value: c.session})

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rutracker search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rutracker search HTTP %d", resp.StatusCode)
	}

	// Rutracker serves Windows-1251 encoded HTML
	reader := decodeWindows1251(resp.Body)
	return parseSearchResults(reader)
}

// DownloadTorrent downloads a .torrent file from Rutracker.
func (c *Client) DownloadTorrent(topicID int, username, password string) ([]byte, error) {
	c.limiter.Wait(context.Background())

	// Ensure we're logged in
	if c.session == "" {
		if err := c.Login(username, password); err != nil {
			return nil, err
		}
	}

	dlURL := fmt.Sprintf("%s/forum/dl.php?t=%d", c.baseURL, topicID)
	req, err := http.NewRequest("GET", dlURL, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{Name: "bb_session", Value: c.session})

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rutracker download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rutracker download HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rutracker read torrent: %w", err)
	}

	slog.Info("downloaded torrent from rutracker", "topic_id", topicID, "size", len(data))
	return data, nil
}

// TestConnection tests connectivity to Rutracker by logging in.
func (c *Client) TestConnection(username, password string) error {
	return c.Login(username, password)
}

// parseSearchResults parses the HTML search results page from Rutracker.
func parseSearchResults(r io.Reader) ([]Result, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse rutracker HTML: %w", err)
	}

	var results []Result
	var f func(*html.Node)
	f = func(n *html.Node) {
		// Look for rows in the tracker search results table
		if n.Type == html.ElementNode && n.Data == "tr" && hasClass(n, "tCenter") {
			result := parseResultRow(n)
			if result != nil {
				results = append(results, *result)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return results, nil
}

// parseResultRow extracts a Result from a search result table row.
func parseResultRow(tr *html.Node) *Result {
	var result Result

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Topic link with ID
			if n.Data == "a" && hasClass(n, "tLink") {
				for _, a := range n.Attr {
					if a.Key == "href" {
						result.TopicID = extractTopicID(a.Val)
					}
				}
				result.Title = textContent(n)
			}

			// Torrent size
			if n.Data == "a" && hasClass(n, "dl-stub") {
				result.Size = parseSize(textContent(n))
			}
			if n.Data == "td" && hasClass(n, "tor-size") {
				// Size may also be in a data attribute
				for _, a := range n.Attr {
					if a.Key == "data-ts_text" {
						if v, err := strconv.ParseInt(a.Val, 10, 64); err == nil {
							result.Size = v
						}
					}
				}
			}

			// Seeders
			if n.Data == "b" || n.Data == "td" {
				if hasClass(n, "seedmed") || hasClass(n, "seed") {
					if v, err := strconv.Atoi(strings.TrimSpace(textContent(n))); err == nil {
						result.Seeders = v
					}
				}
			}

			// Leechers
			if n.Data == "td" && hasClass(n, "leechmed") {
				if v, err := strconv.Atoi(strings.TrimSpace(textContent(n))); err == nil {
					result.Leechers = v
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(tr)

	if result.TopicID == 0 || result.Title == "" {
		return nil
	}
	return &result
}

// hasClass checks if an HTML node has a specific CSS class.
func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == class {
					return true
				}
			}
		}
	}
	return false
}

// textContent returns the concatenated text content of an HTML node.
func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(textContent(c))
	}
	return sb.String()
}

var topicIDRegex = regexp.MustCompile(`t=(\d+)`)

// extractTopicID extracts the topic ID from a URL like "viewtopic.php?t=12345".
func extractTopicID(href string) int {
	m := topicIDRegex.FindStringSubmatch(href)
	if len(m) < 2 {
		return 0
	}
	id, _ := strconv.Atoi(m[1])
	return id
}

var sizeRegex = regexp.MustCompile(`([\d.]+)\s*(GB|MB|KB|TB)`)

// parseSize converts a human-readable size string to bytes.
func parseSize(s string) int64 {
	m := sizeRegex.FindStringSubmatch(s)
	if len(m) < 3 {
		return 0
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	switch m[2] {
	case "TB":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	case "GB":
		return int64(val * 1024 * 1024 * 1024)
	case "MB":
		return int64(val * 1024 * 1024)
	case "KB":
		return int64(val * 1024)
	}
	return 0
}
