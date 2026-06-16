package github

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
)

// newHTTPClient creates an authenticated HTTP client using a Personal Access Token.
// The returned client is used for both REST and GraphQL requests.
func newHTTPClient(ctx context.Context, token string) *http.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return oauth2.NewClient(ctx, ts)
}
