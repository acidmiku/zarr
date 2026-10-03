package metadata

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTMDBKeyAndReadTokenUseConsistentAuthentication(t *testing.T) {
	for _, credential := range []string{strings.Repeat("a", 32), "eyJ.read-access-token.with+reserved&characters"} {
		t.Run(fmt.Sprint(len(credential)), func(t *testing.T) {
			requests := 0
			client := NewTMDBClient(&http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if len(credential) == 32 {
					if r.URL.Query().Get("api_key") != credential || r.Header.Get("Authorization") != "" {
						t.Fatal("legacy key auth incorrect")
					}
				} else if r.URL.Query().Has("api_key") || r.Header.Get("Authorization") != "Bearer "+credential {
					t.Fatal("read token was exposed in URL or omitted")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})}, credential)
			if _, err := client.SearchMovies("Film"); err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetMovie(1); err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetTV(1); err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetSeason(1, 1); err != nil {
				t.Fatal(err)
			}
			if _, err := client.TrendingMovies(1); err != nil {
				t.Fatal(err)
			}
			if requests != 5 {
				t.Fatal(requests)
			}
		})
	}
}

func TestTMDBErrorsNeverExposeCredential(t *testing.T) {
	credential := strings.Repeat("s", 32)
	for _, status := range []int{0, 401} {
		client := NewTMDBClient(&http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
			if status == 0 {
				return nil, fmt.Errorf("failed %s", r.URL.String())
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(credential))}, nil
		})}, credential)
		_, err := client.GetMovie(1)
		if err == nil || strings.Contains(err.Error(), credential) {
			t.Fatalf("credential exposed: %v", err)
		}
	}
}
