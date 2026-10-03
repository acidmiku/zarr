package httpclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// Clients holds the two HTTP clients used throughout the application.
type Clients struct {
	// Proxy routes through user-configured SOCKS5/HTTP proxy. Used for TMDB, AniList, indexers.
	Proxy *http.Client
	// Direct connects without proxy. Used exclusively for SABnzbd.
	Direct *http.Client
}

// New creates the client pair. proxyURL can be empty for no proxy.
func New(proxyURL string) (*Clients, error) {
	// Create custom dialer
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		// Use system DNS (which Docker configures to use custom DNS servers)
	}

	// Direct client ignores all proxy environment variables.
	// Retry transport handles transient EOFs / 5xx automatically.
	direct := &http.Client{
		Timeout: 30 * time.Second,
		Transport: wrapRetry(&http.Transport{
			Proxy:                 nil, // Explicitly disable proxy
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}),
	}

	var proxyClient *http.Client
	if proxyURL != "" {
		transport, err := makeProxyTransport(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("create proxy transport: %w", err)
		}
		proxyClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: wrapRetry(transport),
		}
	} else {
		// When no proxy configured, explicitly ignore environment proxy vars
		proxyClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: wrapRetry(&http.Transport{
				Proxy:                 nil, // Explicitly disable proxy
				DialContext:           dialer.DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			}),
		}
	}

	proxyClient.Transport = &switchTransport{current: proxyClient.Transport}
	return &Clients{
		Proxy:  proxyClient,
		Direct: direct,
	}, nil
}

func makeProxyTransport(proxyURL string) (*http.Transport, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL: %w", err)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("proxy URL requires a host")
	}

	switch u.Scheme {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if u.User != nil {
			password, _ := u.User.Password()
			auth = &proxy.Auth{User: u.User.Username(), Password: password}
		}
		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: 30 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("create SOCKS5 dialer: %w", err)
		}
		return &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.(proxy.ContextDialer).DialContext(ctx, network, addr)
			},
		}, nil

	case "http", "https":
		return &http.Transport{
			Proxy: http.ProxyURL(u),
		}, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", u.Scheme)
	}
}
