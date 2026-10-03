package grabber

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSABRejectsJSONAuthenticationErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":false,"error":"API Key Incorrect"}`)
	}))
	defer srv.Close()
	g := New(srv.Client(), srv.Client(), srv.URL, "secret")
	if err := g.TestConnection(); err == nil {
		t.Error("reported invalid API key as healthy")
	}
	if _, err := g.GetHistory(); err == nil {
		t.Error("accepted failed history response")
	}
	if err := g.DeleteFromQueue("job"); err == nil {
		t.Error("accepted failed deletion")
	}
}
func TestSABEncodesAPIKeyAndRejectsInvalidNZB(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/api" {
			if r.URL.Query().Get("apikey") != "a&b+#" {
				t.Error("API key corrupted")
			}
			io.WriteString(w, `{"queue":{"slots":[]}}`)
			return
		}
		io.WriteString(w, `<error code="100" description="bad API key"/>`)
	}))
	defer srv.Close()
	g := New(srv.Client(), srv.Client(), srv.URL+"/", "a&b+#")
	if _, err := g.GetQueue(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GrabNZB(srv.URL+"/nzb?apikey=secret", "title"); err == nil {
		t.Error("sent indexer error as NZB")
	}
	if calls != 2 {
		t.Errorf("unexpected upload after invalid NZB: %d calls", calls)
	}
}
func TestNetworkErrorsDoNotExposeIndexerKey(t *testing.T) {
	g := New(&http.Client{}, &http.Client{}, "", "")
	_, err := g.GrabNZB(":bad?apikey=secret", "title")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error: %v", err)
	}
}
