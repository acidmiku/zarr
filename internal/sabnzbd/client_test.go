package sabnzbd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerLifecycleUsesPostAndMasksCredentials(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.RawQuery != "" {
			t.Error("credentials must be sent as form data")
		}
		r.ParseForm()
		if r.Form.Get("apikey") != "private-key" {
			t.Error("missing authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("mode") {
		case "get_config":
			io.WriteString(w, `{"config":{"servers":[{"name":"stable","displayname":"Provider","host":"news.example","port":563,"username":"user","password":"********","connections":10,"ssl":1,"enable":1,"priority":0}]}}`)
		case "config":
			if r.Form.Get("server") != "stable" || r.Form.Get("password") != "********" {
				t.Error("saved credential not used")
			}
			io.WriteString(w, `{"value":{"result":true,"message":"ok"}}`)
		case "set_config":
			if r.Form.Has("password") {
				t.Error("empty password must not replace saved value")
			}
			io.WriteString(w, `{"config":{}}`)
		default:
			io.WriteString(w, `{"status":false,"error":"API Key Incorrect"}`)
		}
	}))
	defer endpoint.Close()
	client := New(endpoint.Client(), endpoint.URL, "private-key")
	servers, err := client.Servers(context.Background())
	if err != nil || len(servers) != 1 {
		t.Fatal(servers, err)
	}
	server := servers[0]
	if server.Password != "" || !server.PasswordConfigured {
		t.Fatal("password exposed")
	}
	if err := client.TestServer(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	if err := client.Save(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	if err := client.Test(context.Background()); err == nil {
		t.Fatal("auth rejection treated as connected")
	}
}
