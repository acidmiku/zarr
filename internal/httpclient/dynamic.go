package httpclient

import (
	"net/http"
	"sync"
)

// switchTransport changes the proxy for all services sharing a client without
// mutating http.Client while requests are in flight.
type switchTransport struct {
	mu      sync.RWMutex
	current http.RoundTripper
}

func (t *switchTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.mu.RLock()
	transport := t.current
	t.mu.RUnlock()
	return transport.RoundTrip(r)
}

func UpdateProxy(client *http.Client, proxyURL string) error {
	clients, err := New(proxyURL)
	if err != nil {
		return err
	}
	next := clients.Proxy.Transport.(*switchTransport).current
	current, ok := client.Transport.(*switchTransport)
	if !ok {
		return nil
	} // Test clients may supply a custom transport.
	current.mu.Lock()
	old := current.current
	current.current = next
	current.mu.Unlock()
	if closer, ok := old.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	return nil
}
