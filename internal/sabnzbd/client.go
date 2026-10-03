// Package sabnzbd provides configuration access without exposing downloader secrets.
package sabnzbd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type Client struct {
	http          *http.Client
	endpoint, key string
}

func New(client *http.Client, endpoint, key string) *Client {
	return &Client{client, strings.TrimRight(endpoint, "/"), key}
}
func (c *Client) call(ctx context.Context, values url.Values) (map[string]json.RawMessage, error) {
	if c.key == "" {
		return nil, fmt.Errorf("configure the SABnzbd API key first")
	}
	values.Set("apikey", c.key)
	values.Set("output", "json")
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("invalid SABnzbd URL")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach SABnzbd; check its URL and network")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("SABnzbd returned HTTP %d", resp.StatusCode)
	}
	var data map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil || data == nil {
		return nil, fmt.Errorf("invalid SABnzbd response")
	}
	if raw, ok := data["error"]; ok && string(raw) != "null" && string(raw) != `""` {
		return nil, fmt.Errorf("SABnzbd rejected the request; check its API key and configuration")
	}
	if raw, ok := data["status"]; ok && string(raw) == "false" {
		return nil, fmt.Errorf("SABnzbd rejected the request")
	}
	return data, nil
}
func (c *Client) Test(ctx context.Context) error {
	data, err := c.call(ctx, url.Values{"mode": {"queue"}, "limit": {"0"}})
	if err != nil {
		return err
	}
	if _, ok := data["queue"]; !ok {
		return fmt.Errorf("SABnzbd did not return an authenticated queue")
	}
	return nil
}

type Server struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	Password           string `json:"password,omitempty"`
	PasswordConfigured bool   `json:"password_configured"`
	Connections        int    `json:"connections"`
	SSL                bool   `json:"ssl"`
	Enabled            bool   `json:"enabled"`
	Priority           int    `json:"priority"`
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func number(v interface{}) int { n, _ := strconv.Atoi(str(v)); return n }
func truth(v interface{}) bool { return str(v) == "1" || str(v) == "true" }
func (c *Client) Servers(ctx context.Context) ([]Server, error) {
	data, err := c.call(ctx, url.Values{"mode": {"get_config"}, "section": {"servers"}})
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Servers json.RawMessage `json:"servers"`
	}
	if err := json.Unmarshal(data["config"], &cfg); err != nil {
		return nil, fmt.Errorf("invalid SABnzbd server configuration")
	}
	// SABnzbd omits empty sections on a fresh installation.
	if len(cfg.Servers) == 0 || string(cfg.Servers) == "null" {
		return []Server{}, nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(cfg.Servers, &items); err != nil {
		var named map[string]map[string]interface{}
		if err := json.Unmarshal(cfg.Servers, &named); err != nil {
			return nil, fmt.Errorf("invalid SABnzbd server list")
		}
		for id, item := range named {
			item["name"] = id
			items = append(items, item)
		}
	}
	result := []Server{}
	for _, item := range items {
		id := str(item["name"])
		name := str(item["displayname"])
		if name == "" {
			name = id
		}
		result = append(result, Server{ID: id, Name: name, Host: str(item["host"]), Port: number(item["port"]), Username: str(item["username"]), PasswordConfigured: str(item["password"]) != "", Connections: number(item["connections"]), SSL: truth(item["ssl"]), Enabled: truth(item["enable"]), Priority: number(item["priority"])})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Priority < result[j].Priority })
	return result, nil
}
func (s Server) Validate() error {
	if strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("name and host are required")
	}
	if strings.ContainsAny(s.Host, "/\\ \t\r\n") {
		return fmt.Errorf("host must be a hostname without a URL scheme")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if s.Connections < 1 || s.Connections > 100 {
		return fmt.Errorf("connections must be between 1 and 100")
	}
	if s.Priority < 0 || s.Priority > 99 {
		return fmt.Errorf("priority must be between 0 and 99")
	}
	return nil
}
func bit(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
func (s Server) values() url.Values {
	v := url.Values{"section": {"servers"}, "name": {s.ID}, "displayname": {s.Name}, "host": {s.Host}, "port": {strconv.Itoa(s.Port)}, "username": {s.Username}, "connections": {strconv.Itoa(s.Connections)}, "ssl": {bit(s.SSL)}, "ssl_verify": {"3"}, "enable": {bit(s.Enabled)}, "priority": {strconv.Itoa(s.Priority)}}
	if s.Password != "" {
		v.Set("password", s.Password)
	}
	return v
}
func (c *Client) Save(ctx context.Context, s Server) error {
	if err := s.Validate(); err != nil {
		return err
	}
	v := s.values()
	v.Set("mode", "set_config")
	data, err := c.call(ctx, v)
	if err == nil {
		if _, ok := data["config"]; !ok {
			return fmt.Errorf("SABnzbd did not confirm the saved server")
		}
	}
	return err
}
func (c *Client) Delete(ctx context.Context, id string) error {
	_, err := c.call(ctx, url.Values{"mode": {"del_config"}, "section": {"servers"}, "keyword": {id}})
	return err
}
func (c *Client) TestServer(ctx context.Context, s Server) error {
	v := s.values()
	v.Set("mode", "config")
	v.Set("name", "test_server")
	v.Set("server", s.ID)
	if s.Password == "" && s.PasswordConfigured {
		v.Set("password", "********")
	}
	data, err := c.call(ctx, v)
	if err != nil {
		return err
	}
	var result struct {
		Result  bool   `json:"result"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data["value"], &result); err != nil {
		return fmt.Errorf("invalid SABnzbd server-test response")
	}
	if !result.Result {
		message := result.Message
		if s.Password != "" {
			message = strings.ReplaceAll(message, s.Password, "[redacted]")
		}
		if message == "" {
			message = "Usenet server test failed"
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}
