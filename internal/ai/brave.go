package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type BraveClient struct {
	client *http.Client
	apiKey string
}

func NewBraveClient(client *http.Client, apiKey string) *BraveClient {
	return &BraveClient{client: client, apiKey: apiKey}
}

func (c *BraveClient) SetAPIKey(key string) {
	c.apiKey = key
}

type BraveSearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Search performs a web search via Brave Search API. Returns up to count results.
func (c *BraveClient) Search(query string, count int) ([]BraveSearchResult, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("brave API key not configured")
	}
	if count <= 0 {
		count = 5
	}

	u, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
	q := u.Query()
	q.Set("q", query)
	q.Set("count", strconv.Itoa(count))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brave %d: %s", resp.StatusCode, string(b))
	}

	var result braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("brave decode: %w", err)
	}

	var results []BraveSearchResult
	for _, r := range result.Web.Results {
		results = append(results, BraveSearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		})
	}
	return results, nil
}
