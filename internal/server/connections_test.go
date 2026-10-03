package server

import (
	"encoding/json"
	"mediaforge/internal/config"
	"mediaforge/internal/database"
	"mediaforge/internal/httpclient"
	"mediaforge/internal/qbt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newConnectionTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clients, _ := httpclient.New("")
	return &Server{db: db, cfg: config.Load(db), proxyClient: clients.Proxy, directClient: clients.Direct, mux: http.NewServeMux()}
}
func callConnection(t *testing.T, handler http.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
func TestSettingsSecretPreservationAndAtomicValidation(t *testing.T) {
	s := newConnectionTestServer(t)
	s.db.SetSetting("tmdb_api_key", "private-tmdb-key")
	s.cfg.Reload(s.db)
	response := callConnection(t, s.handleGetSettings, "GET", "/api/settings", "")
	if strings.Contains(response.Body.String(), "private-tmdb-key") {
		t.Fatal("secret leaked")
	}
	var settings map[string]interface{}
	json.Unmarshal(response.Body.Bytes(), &settings)
	if settings["tmdb_configured"] != true || settings["tmdb_api_key"] != "" {
		t.Fatal(settings)
	}
	for _, body := range []string{`{"tmdb_api_key":""}`, `{"tmdb_api_key":"********"}`, `{"tmdb_api_key":"priv...ey"}`} {
		if w := callConnection(t, s.handleUpdateSettings, "PUT", "/api/settings", body); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		value, _ := s.db.GetSetting("tmdb_api_key")
		if value != "private-tmdb-key" {
			t.Fatal("saved credential overwritten")
		}
	}
	w := callConnection(t, s.handleUpdateSettings, "PUT", "/api/settings", `{"tmdb_api_key":"replacement","torrent_seed_time_hours":"-1"}`)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	value, _ := s.db.GetSetting("tmdb_api_key")
	if value != "private-tmdb-key" {
		t.Fatal("invalid settings partially persisted")
	}
	w = callConnection(t, s.handleUpdateSettings, "PUT", "/api/settings", `{"clear_tmdb_api_key":true}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	value, _ = s.db.GetSetting("tmdb_api_key")
	if value != "" || s.tmdb != nil {
		t.Fatal("explicit key removal did not disable service")
	}
}
func TestIndexerReadRedactsAndPatchPreservesSecrets(t *testing.T) {
	s := newConnectionTestServer(t)
	w := callConnection(t, s.handleCreateIndexer, "POST", "/api/indexers", `{"name":"NZB","url":"https://index.example","api_key":"secret-indexer"}`)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	w = callConnection(t, s.handleListIndexers, "GET", "/api/indexers", "")
	if strings.Contains(w.Body.String(), "secret-indexer") {
		t.Fatal("indexer secret leaked")
	}
	r := httptest.NewRequest("PUT", "/api/indexers/1", strings.NewReader(`{"name":"Renamed","api_key":""}`))
	r.SetPathValue("id", "1")
	w = httptest.NewRecorder()
	s.handleUpdateIndexer(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var key, name string
	var enabled bool
	s.db.QueryRow(`SELECT name,api_key,enabled FROM indexers WHERE id=1`).Scan(&name, &key, &enabled)
	if name != "Renamed" || key != "secret-indexer" || !enabled {
		t.Fatal("partial patch changed omitted fields")
	}
}
func TestConnectionTestDoesNotReplaceActiveQBittorrent(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "login") {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test", Path: "/"})
			w.Write([]byte("Ok."))
			return
		}
		w.Write([]byte("v5.1.0"))
	}))
	defer endpoint.Close()
	s := newConnectionTestServer(t)
	s.qbt = qbt.New(s.directClient, "http://active.invalid", "old-user", "old-secret")
	original := s.qbt
	body, _ := json.Marshal(map[string]string{"url": endpoint.URL, "username": "test-user", "password": "test-secret"})
	w := callConnection(t, s.handleTestQBittorrent, "POST", "/api/settings/test-qbittorrent", string(body))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatal(w.Body.String())
	}
	if s.qbt != original || s.cfg.QBTURL == endpoint.URL {
		t.Fatal("connection test replaced the active client")
	}
}
func TestLocalAPICrossOriginProtection(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, origin := range []string{"https://attacker.example", "null"} {
		r := httptest.NewRequest("POST", "http://localhost:9876/api/settings", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("origin %s accepted", origin)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost:9876/api/settings", nil)
	r.Header.Set("Origin", "http://localhost:9876")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
}
func TestNullBooleanSettingsAreRejected(t *testing.T) {
	s := newConnectionTestServer(t)
	for _, body := range []string{`{"setup_complete":null}`, `{"qbittorrent_enabled":null}`, `{"clear_tmdb_api_key":null}`} {
		w := callConnection(t, s.handleUpdateSettings, "PUT", "/api/settings", body)
		if w.Code != 400 {
			t.Fatalf("accepted %s: %d", body, w.Code)
		}
	}
}
func TestSlowRequestDoesNotBlockSettings(t *testing.T) {
	s := newConnectionTestServer(t)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	slow := s.runtimeHandler(func(snapshot *Server, w http.ResponseWriter, r *http.Request) { close(started); <-release })
	go func() {
		slow.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/slow", nil))
		close(done)
	}()
	<-started
	updated := make(chan struct{})
	go func() {
		handler := s.runtimeHandler((*Server).handleUpdateSettings)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"setup_complete":true}`)))
		close(updated)
	}()
	select {
	case <-updated:
	case <-time.After(time.Second):
		t.Error("settings were blocked by an unrelated slow request")
	}
	close(release)
	<-done
}
