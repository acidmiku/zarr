package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLastFMArtworkUsesImageCache(t *testing.T) {
	imageURL := "https://lastfm-img.freetls.fastly.net/i/u/300x300/album.png"
	var requests atomic.Int32
	s := &Server{imgCache: newImageCache(t.TempDir()), proxyClient: &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return artworkResponse(200, artworkPNG(t)), nil
	})}}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		s.handleImageProxy(w, httptest.NewRequest("GET", "/api/image?url="+url.QueryEscape(imageURL), nil))
		if w.Code != http.StatusOK {
			t.Fatalf("Last.fm cover rejected: %d", w.Code)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("fetched cached Last.fm image %d times", requests.Load())
	}
	for _, raw := range []string{"https://lastfm-img.freetls.fastly.net.evil.example/album.png", "https://lastfm-img.freetls.fastly.net@evil.example/album.png"} {
		if allowedImageURL(raw) {
			t.Fatalf("allowed spoofed image host: %s", raw)
		}
	}
}

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

func TestImageCacheLateOwnersRecheckCompletedFetch(t *testing.T) {
	cache := newImageCache(t.TempDir())
	imageURL := "https://image.tmdb.org/example.png"
	var requests atomic.Int32
	client := &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return artworkResponse(200, artworkPNG(t)), nil
	})}
	var ready, finished sync.WaitGroup
	ready.Add(16)
	finished.Add(16)
	published := make(chan struct{})
	for i := 0; i < 16; i++ {
		go func() {
			defer finished.Done()
			if cache.get(imageURL) != nil {
				t.Error("test cache unexpectedly prepopulated")
			}
			ready.Done()
			<-published
			result := cache.fetchOrJoin(context.Background(), client, imageURL)
			if result.status != 200 {
				t.Error("cached result unavailable", result.status)
			}
		}()
	}
	ready.Wait()
	if result := cache.fetchOrJoin(context.Background(), client, imageURL); result.status != 200 {
		t.Fatal(result.err)
	}
	close(published)
	finished.Wait()
	if requests.Load() != 1 {
		t.Fatal("late cache misses repeated completed download", requests.Load())
	}
}
