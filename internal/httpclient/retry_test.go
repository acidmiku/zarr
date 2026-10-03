package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMutationsAreNeverRetried(t *testing.T) {
	for _, test := range []struct{ method, url string }{{"POST", "https://api.example/chat"}, {"POST", "http://qbt/api/v2/torrents/add"}, {"GET", "http://sab/api?mode=queue&name=delete"}, {"GET", "http://sab/api?mode=addurl"}} {
		calls := 0
		transport := wrapRetry(testRoundTripper(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Header: http.Header{}}, nil
		}))
		req, _ := http.NewRequest(test.method, test.url, strings.NewReader("body"))
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if calls != 1 {
			t.Fatalf("%s %s retried %d times", test.method, test.url, calls)
		}
	}
}
