package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImageProxyRejectsHTMLAndUnexpectedRedirects(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				http.Redirect(w, r, "http://127.0.0.1/private", 302)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("<html><script>alert('x')</script></html>"))
		}))
		cache := newImageCache(t.TempDir())
		result := cache.fetchOrJoin(context.Background(), endpoint.Client(), endpoint.URL)
		if result.status == 200 || result.err == nil {
			t.Fatal("untrusted image response accepted")
		}
		endpoint.Close()
	}
	for _, raw := range []string{"https://image.tmdb.org@evil.example/a", "https://image.tmdb.org.evil.example/a", "http://image.tmdb.org/a", "https://image.tmdb.org:444/a"} {
		if allowedImageURL(raw) {
			t.Fatal(raw)
		}
	}
}
