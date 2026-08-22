// Package github implements the prsm provider adapter for GitHub.com and
// GitHub Enterprise Server. It uses the REST API throughout:
//   - ListPullRequests: GET /repos/{owner}/{repo}/pulls (ETag-cached via httpcache)
//   - LoadCI: GET /repos/{owner}/{repo}/commits/{ref}/check-runs
//   - LoadReviewerStates: GET /repos/{owner}/{repo}/pulls/{number}/reviews
//   - LoadDiff: GET /repos/{owner}/{repo}/pulls/{number}
package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/lstellway/prsm/model"
)

const defaultAPIBaseURL = "https://api.github.com"

// RepoRef identifies a repository to poll. Owner/repo pairs are GitHub's own
// addressing vocabulary, so the type lives here rather than in the shared
// adapter package: a Jenkins connection polls job paths and a local checkout
// polls a filesystem root, and neither is expressible in these two fields.
type RepoRef struct {
	Owner string
	Repo  string
}

// Config holds the parameters needed to construct a GitHubAdapter.
// The assembly layer maps config.ProviderConfig into this type so the adapter
// package has no dependency on the config package.
type Config struct {
	Name    string
	Token   string
	BaseURL string
	Repos   []RepoRef
}

// GitHubAdapter is the prsm provider adapter for GitHub.
// It is safe for concurrent use and must not be copied once constructed.
type GitHubAdapter struct {
	providerName string
	instance     model.ProviderInstance // immutable after New(); Account is excluded
	repos        []RepoRef
	rest         *gogithub.Client

	// mutex guards state resolved after construction. ADR-009's assembly layer
	// resolves identity at startup while fetches are already fanning out, so
	// ResolveIdentity and Instance genuinely run concurrently on one adapter.
	// Holding Account outside instance avoids tearing the struct; it does not
	// on its own make the string safe to read while another goroutine writes it.
	mutex   sync.RWMutex
	account string // written by ResolveIdentity, read by Instance
}

// New constructs a GitHubAdapter from a Config.
// The token in adapterConfig.Token must already be expanded (no "$VAR" references).
func New(adapterConfig Config) (*GitHubAdapter, error) {
	if adapterConfig.Token == "" {
		return nil, fmt.Errorf("github adapter %q: token is required", adapterConfig.Name)
	}

	httpClient := newHTTPClient(adapterConfig.Token)

	apiBaseURL := strings.TrimRight(adapterConfig.BaseURL, "/")
	if apiBaseURL == "" {
		apiBaseURL = defaultAPIBaseURL
	}

	// Pass our pre-configured http.Client (oauth2 auth + ETag caching) directly.
	// Do NOT also call WithAuthToken — auth is already handled by the transport.
	restClientOptions := []gogithub.ClientOptionsFunc{gogithub.WithHTTPClient(httpClient)}
	if apiBaseURL != defaultAPIBaseURL {
		restClientOptions = append(restClientOptions,
			gogithub.WithEnterpriseURLs(apiBaseURL+"/", apiBaseURL+"/"))
	}
	restClient, err := gogithub.NewClient(restClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("github adapter %q: %w", adapterConfig.Name, err)
	}

	host := "github.com"
	if adapterConfig.BaseURL != "" {
		host = extractHost(adapterConfig.BaseURL)
	}

	return &GitHubAdapter{
		providerName: adapterConfig.Name,
		instance: model.ProviderInstance{
			Name: adapterConfig.Name,
			Kind: model.ProviderGitHub,
			Host: host,
			// Account is populated by ResolveIdentity once called at startup.
		},
		// Cloned so a caller mutating its own slice after New() cannot reach
		// adapter state. New() is callable without the config layer, so the
		// caller is not guaranteed to hand over a freshly built slice.
		repos: slices.Clone(adapterConfig.Repos),
		rest:  restClient,
	}, nil
}

// Instance returns the full ProviderInstance this adapter serves.
// Account is composed at call time under the read lock, so callers always see
// either the value set by ResolveIdentity or the empty string it started as —
// never a partially written one.
func (githubAdapter *GitHubAdapter) Instance() model.ProviderInstance {
	githubAdapter.mutex.RLock()
	defer githubAdapter.mutex.RUnlock()

	instance := githubAdapter.instance
	instance.Account = githubAdapter.account
	return instance
}

// ResolveIdentity returns the authenticated user's identity and populates
// Instance().Account with the resolved GitHub login for "me" sentinel resolution.
func (githubAdapter *GitHubAdapter) ResolveIdentity(ctx context.Context) (model.Identity, error) {
	user, response, err := githubAdapter.rest.Users.Get(ctx, "")
	if err != nil {
		return model.Identity{}, fmt.Errorf("github %q: resolve identity: %w",
			githubAdapter.providerName, err)
	}
	if err := checkRateLimit(githubAdapter.Instance(), response.Response); err != nil {
		return model.Identity{}, err
	}

	displayName := user.GetName()
	if displayName == "" {
		displayName = user.GetLogin()
	}

	githubAdapter.mutex.Lock()
	githubAdapter.account = user.GetLogin()
	githubAdapter.mutex.Unlock()

	return model.Identity{
		Username:    user.GetLogin(),
		DisplayName: displayName,
		AvatarURL:   user.GetAvatarURL(),
	}, nil
}

// ListPullRequests fetches all open pull requests across configured repos via REST.
// Results from repos that succeed are returned even when other repos fail;
// all errors are joined and returned alongside the partial result.
func (githubAdapter *GitHubAdapter) ListPullRequests(ctx context.Context) ([]model.PullRequest, error) {
	var allPullRequests []model.PullRequest
	var fetchErrors []error

	for _, repoRef := range githubAdapter.repos {
		pullRequests, err := githubAdapter.listRepoPullRequests(ctx, repoRef.Owner, repoRef.Repo)
		if err != nil {
			fetchErrors = append(fetchErrors, err)
			continue
		}
		allPullRequests = append(allPullRequests, pullRequests...)
	}
	return allPullRequests, errors.Join(fetchErrors...)
}

func (githubAdapter *GitHubAdapter) listRepoPullRequests(
	ctx context.Context, owner, repo string,
) ([]model.PullRequest, error) {
	// 50 pages × 100 PRs = 5,000 PRs maximum per repo per call.
	const maxPages = 50

	listOptions := &gogithub.PullRequestListOptions{
		State:       "open",
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	// Read once so every PR in this call is stamped with the same instance,
	// rather than later pages picking up an identity resolved mid-fetch.
	instance := githubAdapter.Instance()

	var allPullRequests []model.PullRequest
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("github %q: list PRs %s/%s: exceeded %d-page limit",
				githubAdapter.providerName, owner, repo, maxPages)
		}

		pullRequestPage, response, err := githubAdapter.rest.PullRequests.List(ctx, owner, repo, listOptions)
		if err != nil {
			return nil, fmt.Errorf("github %q: list PRs %s/%s: %w",
				githubAdapter.providerName, owner, repo, err)
		}
		if rateLimitErr := checkRateLimit(instance, response.Response); rateLimitErr != nil {
			return nil, rateLimitErr
		}

		for _, pullRequest := range pullRequestPage {
			allPullRequests = append(allPullRequests, normalizePR(pullRequest, owner, repo, instance))
		}

		if response.NextPage == 0 {
			break
		}
		listOptions.Page = response.NextPage
	}

	return allPullRequests, nil
}

// LoadCI fetches CI/check-run status for a PR's head commit via REST.
func (githubAdapter *GitHubAdapter) LoadCI(
	ctx context.Context, pullRequest model.PullRequest,
) (model.CIStatus, error) {
	if pullRequest.HeadSHA == "" {
		return model.CIStatus{State: model.CIStateNone}, nil
	}

	// 50 pages × 100 runs = 5,000 check runs maximum per SHA.
	const maxPages = 50

	listOptions := &gogithub.ListCheckRunsOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	var allCheckRuns []*gogithub.CheckRun
	for page := 1; ; page++ {
		if page > maxPages {
			return model.CIStatus{}, fmt.Errorf("github %q: load CI for %s#%d: exceeded %d-page limit",
				githubAdapter.providerName, pullRequest.Repo.Name, pullRequest.Number, maxPages)
		}
		checkRunsResponse, response, err := githubAdapter.rest.Checks.ListCheckRunsForRef(
			ctx, pullRequest.Repo.Owner, pullRequest.Repo.Name, pullRequest.HeadSHA, listOptions)
		if err != nil {
			return model.CIStatus{}, fmt.Errorf("github %q: load CI for %s#%d: %w",
				githubAdapter.providerName, pullRequest.Repo.Name, pullRequest.Number, err)
		}
		if rateLimitErr := checkRateLimit(githubAdapter.Instance(), response.Response); rateLimitErr != nil {
			return model.CIStatus{}, rateLimitErr
		}
		allCheckRuns = append(allCheckRuns, checkRunsResponse.CheckRuns...)
		if response.NextPage == 0 {
			break
		}
		listOptions.Page = response.NextPage
	}

	return normalizeCIStatus(allCheckRuns), nil
}

// LoadReviewerStates fetches individual review decisions for a PR via REST.
func (githubAdapter *GitHubAdapter) LoadReviewerStates(
	ctx context.Context, pullRequest model.PullRequest,
) ([]model.ReviewerState, error) {
	// 50 pages × 100 reviews = 5,000 reviews maximum per PR.
	const maxPages = 50

	listOptions := &gogithub.ListOptions{PerPage: 100}
	var allReviews []*gogithub.PullRequestReview

	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("github %q: load reviews for %s#%d: exceeded %d-page limit",
				githubAdapter.providerName, pullRequest.Repo.Name, pullRequest.Number, maxPages)
		}
		reviewPage, response, err := githubAdapter.rest.PullRequests.ListReviews(
			ctx, pullRequest.Repo.Owner, pullRequest.Repo.Name, pullRequest.Number, listOptions)
		if err != nil {
			return nil, fmt.Errorf("github %q: load reviews for %s#%d: %w",
				githubAdapter.providerName, pullRequest.Repo.Name, pullRequest.Number, err)
		}
		if rateLimitErr := checkRateLimit(githubAdapter.Instance(), response.Response); rateLimitErr != nil {
			return nil, rateLimitErr
		}
		allReviews = append(allReviews, reviewPage...)
		if response.NextPage == 0 {
			break
		}
		listOptions.Page = response.NextPage
	}

	return normalizeReviewerStates(allReviews), nil
}

// LoadDiff fetches commit and file-change counts for a PR via the REST detail endpoint.
func (githubAdapter *GitHubAdapter) LoadDiff(
	ctx context.Context, pullRequest model.PullRequest,
) (model.DiffStats, error) {
	githubPullRequest, response, err := githubAdapter.rest.PullRequests.Get(
		ctx, pullRequest.Repo.Owner, pullRequest.Repo.Name, pullRequest.Number)
	if err != nil {
		return model.DiffStats{}, fmt.Errorf("github %q: load diff for %s#%d: %w",
			githubAdapter.providerName, pullRequest.Repo.Name, pullRequest.Number, err)
	}
	if rateLimitErr := checkRateLimit(githubAdapter.Instance(), response.Response); rateLimitErr != nil {
		return model.DiffStats{}, rateLimitErr
	}
	return model.DiffStats{
		Commits:      githubPullRequest.GetCommits(),
		ChangedFiles: githubPullRequest.GetChangedFiles(),
		Additions:    githubPullRequest.GetAdditions(),
		Deletions:    githubPullRequest.GetDeletions(),
	}, nil
}

func extractHost(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" {
		return rawURL
	}
	return parsedURL.Hostname()
}
