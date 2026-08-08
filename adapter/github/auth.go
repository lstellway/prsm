package github

import (
	"net/http"
	"time"

	"github.com/bartventer/httpcache"
	_ "github.com/bartventer/httpcache/store/memcache"
	"golang.org/x/oauth2"
)

// newHTTPClient creates an authenticated HTTP client with transparent ETag caching.
// The oauth2 transport handles PAT injection; httpcache wraps it to send
// If-None-Match on repeat requests and serve 304 responses from cache at zero
// rate-limit cost (GitHub exempts 304s from the primary request budget).
func newHTTPClient(token string) *http.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthTransport := &oauth2.Transport{
		Source: tokenSource,
		Base:   http.DefaultTransport,
	}
	return &http.Client{
		Transport: httpcache.NewTransport(
			"memcache://",
			httpcache.WithUpstream(oauthTransport),
		),
		Timeout: 30 * time.Second,
	}
}
