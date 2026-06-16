package github

import (
	"net/http"
	"strconv"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// checkRateLimit inspects GitHub rate-limit headers and returns a RateLimitError
// when the remaining budget is zero. Returns nil when within budget.
func checkRateLimit(instance model.ProviderInstance, resp *http.Response) error {
	if resp == nil {
		return nil
	}

	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining == "" {
		return nil
	}

	n, err := strconv.Atoi(remaining)
	if err != nil || n > 0 {
		return nil
	}

	rl := adapter.RateLimitError{Instance: instance}

	// X-RateLimit-Reset is a Unix timestamp; prefer it over Retry-After.
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			rl.RetryAfter = time.Unix(ts, 0)
		}
	} else if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			rl.RetryAfter = time.Now().Add(time.Duration(secs) * time.Second)
		}
	}

	return rl
}
