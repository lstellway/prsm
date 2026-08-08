package adapter

import (
	"fmt"
	"time"

	"github.com/lstellway/prsm/model"
)

// RateLimitError is returned when the provider's API rate limit has been exceeded.
// RetryAfter is the absolute time at which the caller may resume polling.
// A zero RetryAfter means the provider did not supply a reset time; callers should
// apply a 60-second default backoff before retrying.
// Instance identifies the provider that returned the rate limit response.
type RateLimitError struct {
	Instance   model.ProviderInstance
	RetryAfter time.Time
}

func (rateLimitError RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded on %s, retry after %s", rateLimitError.Instance.Host, rateLimitError.RetryAfter)
}

// AuthError is returned when the adapter's credentials are invalid or have expired.
type AuthError struct{}

func (authError AuthError) Error() string { return "authentication failed" }

// NotFoundError is returned when the requested resource does not exist.
type NotFoundError struct{}

func (notFoundError NotFoundError) Error() string { return "not found" }
