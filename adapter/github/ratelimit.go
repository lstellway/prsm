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
func checkRateLimit(instance model.ProviderInstance, response *http.Response) error {
	if response == nil {
		return nil
	}

	remaining := response.Header.Get("X-RateLimit-Remaining")
	if remaining == "" {
		return nil
	}

	remainingCount, err := strconv.Atoi(remaining)
	if err != nil || remainingCount > 0 {
		return nil
	}

	rateLimitErr := adapter.RateLimitError{Instance: instance}

	// X-RateLimit-Reset is a Unix timestamp; prefer it over Retry-After.
	if reset := response.Header.Get("X-RateLimit-Reset"); reset != "" {
		if resetTimestamp, err := strconv.ParseInt(reset, 10, 64); err == nil {
			rateLimitErr.RetryAfter = time.Unix(resetTimestamp, 0)
		}
	} else if retryAfterHeader := response.Header.Get("Retry-After"); retryAfterHeader != "" {
		if seconds, err := strconv.Atoi(retryAfterHeader); err == nil {
			rateLimitErr.RetryAfter = time.Now().Add(time.Duration(seconds) * time.Second)
		}
	}

	// Cap RetryAfter to 1 hour: a server-supplied value beyond this is either
	// a mistake or adversarial. The poller sleeps until RetryAfter, so an
	// uncapped value would freeze polling indefinitely.
	const maxRetryAfter = time.Hour
	if !rateLimitErr.RetryAfter.IsZero() && rateLimitErr.RetryAfter.After(time.Now().Add(maxRetryAfter)) {
		rateLimitErr.RetryAfter = time.Now().Add(maxRetryAfter)
	}

	return rateLimitErr
}
