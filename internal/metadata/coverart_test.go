package metadata

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type coverTransport func(*http.Request) (*http.Response, error)

func (f coverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCoverRejectsTraversalAndCachesCompletedDownload(t *testing.T) {
	requests := 0
	client := NewCoverArtClient(&http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("image bytes"))}, nil
	})}, t.TempDir())
	for _, id := range []string{"../escape", "x/../../../escape", "", `x\..\..\escape`} {
		if _, err := client.GetCover(id); err == nil || client.CoverPath(id) != "" {
			t.Fatalf("unsafe ID accepted: %q", id)
		}
	}
	if requests != 0 {
		t.Fatal("invalid IDs sent to network")
	}
	id := "12345678-1234-1234-1234-123456789abc"
	path, err := client.GetCover(id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "image bytes" {
		t.Fatal("cache incomplete", err)
	}
	if _, err := client.GetCover(id); err != nil || requests != 1 {
		t.Fatal("cache miss", err)
	}
}
