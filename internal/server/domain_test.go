package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediaforge/internal/ai"
	"mediaforge/internal/metadata"
)

type domainTransport func(*http.Request) (*http.Response, error)

func (f domainTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func callDomain(handler http.HandlerFunc, method, path, body, param, value string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if param != "" {
		r.SetPathValue(param, value)
	}
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}

func TestRatingValidationAndMusicRead(t *testing.T) {
	s := newConnectionTestServer(t)
	s.db.Exec(`INSERT INTO artists(id,mbid,name) VALUES (1,'artist','Artist')`)
	s.db.Exec(`INSERT INTO albums(id,artist_id,title) VALUES (1,1,'Album')`)
	for _, rating := range []int{-1, 0, 6, 999} {
		w := callDomain(s.handleRateMusicAlbum, "PUT", "/", fmt.Sprintf(`{"rating":%d}`, rating), "id", "1")
		if w.Code != 400 {
			t.Fatalf("rating %d accepted: %d", rating, w.Code)
		}
	}
	w := callDomain(s.handleRateMusicAlbum, "PUT", "/", `{"rating":4,"comment":"great"}`, "id", "1")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = callDomain(s.handleGetRating, "GET", "/?type=music", "", "tmdbID", "1")
	var rating struct {
		Rating  int
		Comment string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rating); err != nil || rating.Rating != 4 || rating.Comment != "great" {
		t.Fatalf("music rating read: %s", w.Body.String())
	}
	w = callDomain(s.handleRateMusicAlbum, "PUT", "/", `{"rating":4}`, "id", "999")
	if w.Code != 404 {
		t.Fatal("missing album accepted")
	}
	// Legacy invalid data must not panic recommendation generation.
	s.db.Exec(`UPDATE albums SET rating=999 WHERE id=1`)
	s.buildRatingsMessage()
}

func TestProfileValidationAndDeletion(t *testing.T) {
	s := newConnectionTestServer(t)
	for _, body := range []string{`{"name":"bad"}`, `{"name":"bad","qualities":{}}`, `{"name":"bad","qualities":["web-1080p"],"tags":[1]}`, `{"name":"bad","qualities":["web-1080p"],"reject_patterns":[""]}`} {
		w := callDomain(s.handleCreateProfile, "POST", "/", body, "", "")
		if w.Code != 400 {
			t.Fatalf("accepted %s: %d", body, w.Code)
		}
	}
	s.db.Exec(`INSERT INTO media_items(type,title,quality_profile_id) VALUES ('movie','Film',1)`)
	w := callDomain(s.handleDeleteProfile, "DELETE", "/", "", "id", "1")
	if w.Code != 409 {
		t.Fatalf("in-use deletion: %d %s", w.Code, w.Body.String())
	}
	w = callDomain(s.handleUpdateLibraryItem, "PUT", "/", `{"quality_profile_id":999}`, "id", "1")
	if w.Code != 400 {
		t.Fatal("invalid profile update accepted")
	}
	var profile int
	s.db.QueryRow(`SELECT quality_profile_id FROM media_items WHERE id=1`).Scan(&profile)
	if profile != 1 {
		t.Fatal("failed update modified profile")
	}
}

func TestSeriesCreationDoesNotLeavePartialLibraryOnMetadataFailure(t *testing.T) {
	s := newConnectionTestServer(t)
	s.tmdb = metadata.NewTMDBClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"id":42,"name":"Series","seasons":[{"season_number":1,"name":"Season 1"}]}`
		status := 200
		if strings.Contains(r.URL.Path, "/season/") {
			body = `{"error":"unavailable"}`
			status = 503
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}, "test")
	w := callDomain(s.handleAddToLibrary, "POST", "/", `{"tmdb_id":42,"type":"series"}`, "", "")
	if w.Code != 502 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var count int
	s.db.QueryRow(`SELECT count(*) FROM media_items`).Scan(&count)
	if count != 0 {
		t.Fatal("half-created series persisted")
	}
}

func TestLooseMovieScanUsesExactFileAndMoveNeverClobbers(t *testing.T) {
	s := newConnectionTestServer(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "Film (2025).mkv")
	target := filepath.Join(dir, "Other (2024).mkv")
	os.WriteFile(source, []byte("first"), 0600)
	os.WriteFile(target, []byte("second"), 0600)
	items := s.scanMovies(dir)
	if len(items) != 2 {
		t.Fatalf("scan: %+v", items)
	}
	for _, item := range items {
		if item.SourcePath == dir || len(findAllVideoFiles(item.SourcePath)) != 1 {
			t.Fatalf("scan selected sibling movies: %+v", item)
		}
	}
	if err := moveFileForImport(source, target); err == nil {
		t.Fatal("overwrote existing library file")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "second" {
		t.Fatal("destination changed")
	}
	data, _ = os.ReadFile(source)
	if string(data) != "first" {
		t.Fatal("source lost")
	}
	if err := moveFileForImport(source, source); err != nil {
		t.Fatal("same-file import failed", err)
	}
	dest := filepath.Join(dir, "library", "Film.mkv")
	if err := moveFileForImport(source, dest); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(dest)
	if string(data) != "first" {
		t.Fatal("move lost content")
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("source not removed")
	}
}

func TestLibraryDeletionRejectsBroadOrEscapingPath(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{root, filepath.Join(root, "movies"), filepath.Join(root, "..", "outside")} {
		if err := validateLibraryDeletePath(root, path); err == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	if err := validateLibraryDeletePath(root, filepath.Join(root, "movies", "Film")); err != nil {
		t.Fatal(err)
	}
}

func TestAIChatMissingSessionAndToolReasoningRoundTrip(t *testing.T) {
	s := newConnectionTestServer(t)
	chatCalls := 0
	s.aiOpenRouter = ai.NewOpenRouterClient(&http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/models" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"moonshotai/kimi-k3","context_length":1000000}]}`))}, nil
		}
		var req ai.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Reasoning == nil || req.Reasoning.Effort != "high" {
			t.Fatal("high reasoning not requested")
		}
		chatCalls++
		body := ""
		if chatCalls == 1 {
			body = `data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.encrypted","data":"opaque"}],"tool_calls":[{"index":0,"id":"call","function":{"name":"unknown_tool","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
		} else {
			if len(req.Messages) != 4 || len(req.Messages[2].ReasoningDetails) != 1 || req.Messages[3].ToolCallID != "call" {
				t.Fatalf("lost reasoning/tool results: %+v", req.Messages)
			}
			body = `data: {"choices":[{"delta":{"content":"Here is an answer"},"finish_reason":"stop"}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body + "\n\ndata: [DONE]\n\n"))}, nil
	})}, "test")
	s.aiSession = ai.NewSessionManager(s.db.DB, s.aiOpenRouter)
	w := callDomain(s.handleAIChat, "POST", "/", `{"content":"hello"}`, "id", "999")
	if w.Code != 404 || chatCalls != 0 {
		t.Fatal("missing session reached provider")
	}
	session, err := s.aiSession.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	w = callDomain(s.handleAIChat, "POST", "/", `{"content":"hello"}`, "id", fmt.Sprint(session.ID))
	if chatCalls != 2 || !strings.Contains(w.Body.String(), "Here is an answer") || strings.Contains(w.Body.String(), "event: error") {
		t.Fatalf("tool flow: %s", w.Body.String())
	}
	_, messages, err := s.aiSession.GetSession(session.ID)
	if err != nil || len(messages) != 4 {
		t.Fatalf("persisted messages: %+v %v", messages, err)
	}
}
