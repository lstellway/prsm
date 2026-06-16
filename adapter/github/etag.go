package github

import (
	"net/http"
	"sync"
)

// etagCache stores the last ETag returned per endpoint URL.
// Sending If-None-Match on the next request allows the server to return 304
// (Not Modified) without consuming rate-limit budget (GitHub-specific exemption).
type etagCache struct {
	mu    sync.RWMutex
	tags  map[string]string // URL → ETag value
	cache map[string]any    // URL → last successful response body (typed by caller)
}

func newETagCache() *etagCache {
	return &etagCache{
		tags:  make(map[string]string),
		cache: make(map[string]any),
	}
}

// setRequestHeaders adds If-None-Match to the request if a cached ETag exists.
func (c *etagCache) setRequestHeaders(req *http.Request) {
	c.mu.RLock()
	tag := c.tags[req.URL.String()]
	c.mu.RUnlock()
	if tag != "" {
		req.Header.Set("If-None-Match", tag)
	}
}

// recordResponse stores the ETag from a successful (2xx) response.
func (c *etagCache) recordResponse(url string, resp *http.Response) {
	if tag := resp.Header.Get("ETag"); tag != "" {
		c.mu.Lock()
		c.tags[url] = tag
		c.mu.Unlock()
	}
}

// storeValue caches the parsed response body for a URL.
func (c *etagCache) storeValue(url string, v any) {
	c.mu.Lock()
	c.cache[url] = v
	c.mu.Unlock()
}

// loadValue returns the cached body for a URL and whether one exists.
func (c *etagCache) loadValue(url string) (any, bool) {
	c.mu.RLock()
	v, ok := c.cache[url]
	c.mu.RUnlock()
	return v, ok
}
