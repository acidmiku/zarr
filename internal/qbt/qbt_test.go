package qbt

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddTorrentReplaysBodyAfterExpiredSession(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "fresh"})
			io.WriteString(w, "Ok.")
			return
		}
		calls++
		if calls == 1 {
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(403)
			return
		}
		cookie, err := r.Cookie("SID")
		if err != nil || cookie.Value != "fresh" {
			t.Error("missing refreshed cookie")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("retry body missing: %v", err)
			w.WriteHeader(400)
			return
		}
		file, _, err := r.FormFile("torrents")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if string(data) != "torrent-data" {
			t.Errorf("retry body = %q", data)
		}
		if r.FormValue("savepath") != "" {
			t.Error("must respect configured client save path")
		}
		io.WriteString(w, "Ok.")
	}))
	defer srv.Close()
	c := New(srv.Client(), srv.URL+"/", "user", "password")
	if _, err := c.AddTorrent([]byte("torrent-data"), "example.torrent", 42); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("got %d attempts", calls)
	}
}
func TestDeleteReportsClientFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	if err := New(srv.Client(), srv.URL, "", "").DeleteTorrent("hash", true); err == nil {
		t.Fatal("accepted failed deletion")
	}
}

func TestModernQBTAuthenticationAndAddNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "modern"})
			w.WriteHeader(204)
			return
		}
		if _, err := r.Cookie("SID"); err != nil {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/v2/app/version" {
			io.WriteString(w, "v5.2.0")
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := New(srv.Client(), srv.URL, "user", "password")
	if err := c.TestConnection(); err != nil {
		t.Fatal(err)
	}
	c.sid = ""
	if _, err := c.AddTorrent([]byte("torrent"), "test.torrent", 1); err != nil {
		t.Fatal(err)
	}
}

func TestQBT52PortScopedCookieAndJSONAddResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9090", Value: "new-cookie"})
			w.WriteHeader(204)
			return
		}
		if _, err := r.Cookie("QBT_SID_9090"); err != nil {
			w.WriteHeader(401)
			return
		}
		io.WriteString(w, `{"added_torrent_ids":["abc123"],"failure_count":0}`)
	}))
	defer srv.Close()
	c := New(srv.Client(), srv.URL, "user", "password")
	hash, err := c.AddTorrent([]byte("data"), "example.torrent", 1)
	if err != nil || hash != "abc123" {
		t.Fatalf("modern API rejected: hash=%q error=%v", hash, err)
	}
}
