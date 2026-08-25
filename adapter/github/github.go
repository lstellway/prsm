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
	"time"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/lstellway/prsm/model"
)

const defaultAPIBaseURL = "https://api.github.com"

// defaultPaginationTimeout bounds a whole paginated operation (all pages,
// not one request) when Config.PaginationTimeout is unset. It matches the
// 30s per-request http.Client timeout in newHTTPClient — tolerating one
// fully slow page before bailing — rather than a larger multiple: a GitHub
// connection with several configured repos pays this deadline once per repo
// in a plain sequential loop (see ListPullRequests), so a looser per-repo
// budget compounds quickly across a poll tick.
const defaultPaginationTimeout = 30 * time.Second

// maxPaginationPages bounds every paginated operation (PR lists, CI check
// runs, reviews) to at most this many pages — 5,000 items at the
// 100-per-page size used throughout this file. It is a generic safety cap
// against a runaway or pathologically large listing, not a value tuned
// per resource kind, so all call sites share it rather than each declaring
// their own copy of the same number.
const maxPaginationPages = 50

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
	// PaginationTimeout bounds how long one paginating operation
	// (ListPullRequests per repo, LoadCI, LoadReviewerStates) may run across
	// all of its pages. Zero means "use defaultPaginationTimeout".
	PaginationTimeout time.Duration
}

// GitHubAdapter is the prsm provider adapter for GitHub.
// It is safe for concurrent use and must not be copied once constructed.
type GitHubAdapter struct {
	providerName      string
	instance          model.ProviderInstance // immutable after New(); Account is excluded
	repos             []RepoRef
	rest              *gogithub.Client
	paginationTimeout time.Duration

	// mutex guards state resolved after construction. The assembly layer
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
		repos:             slices.Clone(adapterConfig.Repos),
		rest:              restClient,
		paginationTimeout: adapterConfig.PaginationTimeout,
	}, nil
}

// effectivePaginationTimeout returns the deadline a paginated operation
// should run under: the configured PaginationTimeout, or
// defaultPaginationTimeout when that was left unset (zero value) — including
// when a GitHubAdapter is constructed directly as a struct literal, bypassing
// New(), a path this package's own tests use.
func (githubAdapter *GitHubAdapter) effectivePaginationTimeout() time.Duration {
	if githubAdapter.paginationTimeout <= 0 {
		return defaultPaginationTimeout
	}
	return githubAdapter.paginationTimeout
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
		// Appended before the error check: a repo whose fetch failed partway
		// through pagination still contributes whatever pages it got back.
		allPullRequests = append(allPullRequests, pullRequests...)
		if err != nil {
			fetchErrors = append(fetchErrors, err)
		}
	}
	return allPullRequests, errors.Join(fetchErrors...)
}

// paginationError builds the error for a page-fetch failure inside a
// paginated loop. It attributes the failure to the pagination deadline only
// when ctx's own error is specifically context.DeadlineExceeded — not any
// other reason ctx ended, such as an outer caller cancelling it first —
// so the message never blames a timeout that did not actually fire. err is
// preserved via %w either way, so errors.Is/As — including reaching
// context.DeadlineExceeded, adapter.RateLimitError, or adapter.AuthError —
// still works through the result.
func paginationError(ctx context.Context, timeout time.Duration, prefix string, page int, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s: exceeded %s pagination timeout after %d page(s): %w", prefix, timeout, page, err)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// paginate drives a classic GitHub REST list endpoint through every page
// fetchPage exposes. page starts at the sentinel 0 so the first call omits
// the page query parameter entirely (GitHub's own page-1 default), and every
// call after that uses whatever NextPage the previous response reported —
// the API's own account of what "next" means, not a locally computed
// counter.
//
// The whole operation, not each individual request, is bounded by timeout:
// paginate wraps ctx in its own context.WithTimeout, so a caller that never
// sets a deadline still cannot be stalled by an arbitrarily slow multi-page
// fetch. Whatever items were accumulated before a failure — a fetch error, a
// rate limit, the timeout firing, or the maxPages safety cap — are always
// returned alongside the error, never discarded; it is on callers to decide
// how to interpret a partial result (see LoadCI, which treats zero
// accumulated items on error differently from a genuine empty success).
func paginate[Item any](
	ctx context.Context, timeout time.Duration, maxPages int, prefix string, instance model.ProviderInstance,
	fetchPage func(ctx context.Context, page int) ([]Item, *gogithub.Response, error),
) ([]Item, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var allItems []Item
	page := 0
	for pageCount := 1; ; pageCount++ {
		if pageCount > maxPages {
			return allItems, fmt.Errorf("%s: exceeded %d-page limit", prefix, maxPages)
		}

		items, response, err := fetchPage(ctx, page)
		if err != nil {
			return allItems, paginationError(ctx, timeout, prefix, pageCount, err)
		}
		if rateLimitErr := checkRateLimit(instance, response.Response); rateLimitErr != nil {
			return allItems, rateLimitErr
		}
		allItems = append(allItems, items...)

		if response.NextPage == 0 {
			break
		}
		page = response.NextPage
	}

	return allItems, nil
}

func (githubAdapter *GitHubAdapter) listRepoPullRequests(
	ctx context.Context, owner, repo string,
) ([]model.PullRequest, error) {
	// Read once so every PR in this call is stamped with the same instance,
	// rather than later pages picking up an identity resolved mid-fetch.
	instance := githubAdapter.Instance()
	prefix := fmt.Sprintf("github %q: list PRs %s/%s", githubAdapter.providerName, owner, repo)

	listOptions := &gogithub.PullRequestListOptions{
		State:       "open",
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	rawPullRequests, err := paginate(ctx, githubAdapter.effectivePaginationTimeout(), maxPaginationPages, prefix, instance,
		func(ctx context.Context, page int) ([]*gogithub.PullRequest, *gogithub.Response, error) {
			listOptions.Page = page
			return githubAdapter.rest.PullRequests.List(ctx, owner, repo, listOptions)
		})

	var allPullRequests []model.PullRequest
	for _, pullRequest := range rawPullRequests {
		allPullRequests = append(allPullRequests, normalizePR(pullRequest, owner, repo, instance))
	}
	return allPullRequests, err
}

// LoadCI fetches CI/check-run status for a PR's head commit via REST.
// The whole paginated fetch is bounded by Config.PaginationTimeout; if a page
// fails or the deadline fires partway through, the status returned still
// summarizes whichever check runs were fetched before that point, alongside
// the error describing why pagination stopped — unless zero check runs were
// fetched at all, in which case the status is reported as unknown rather
// than CIStateNone, which is model.CIStatus's documented, more specific
// claim that CI ran nowhere on this PR; a failed fetch has not established
// that.
func (githubAdapter *GitHubAdapter) LoadCI(
	ctx context.Context, pullRequestRef model.PullRequestRef,
) (model.CIStatus, error) {
	if pullRequestRef.HeadSHA == "" {
		return model.CIStatus{State: model.CIStateNone}, nil
	}

	instance := githubAdapter.Instance()
	prefix := fmt.Sprintf("github %q: load CI for %s#%d",
		githubAdapter.providerName, pullRequestRef.Repo.Name, pullRequestRef.Number)

	listOptions := &gogithub.ListCheckRunsOptions{ListOptions: gogithub.ListOptions{PerPage: 100}}

	allCheckRuns, err := paginate(ctx, githubAdapter.effectivePaginationTimeout(), maxPaginationPages, prefix, instance,
		func(ctx context.Context, page int) ([]*gogithub.CheckRun, *gogithub.Response, error) {
			listOptions.Page = page
			checkRunsResponse, response, err := githubAdapter.rest.Checks.ListCheckRunsForRef(
				ctx, pullRequestRef.Repo.Owner, pullRequestRef.Repo.Name, pullRequestRef.HeadSHA, listOptions)
			if err != nil {
				return nil, response, err
			}
			return checkRunsResponse.CheckRuns, response, nil
		})

	if len(allCheckRuns) == 0 && err != nil {
		return model.CIStatus{}, err
	}
	return normalizeCIStatus(allCheckRuns), err
}

// LoadReviewerStates fetches individual review decisions for a PR via REST.
// The whole paginated fetch is bounded by Config.PaginationTimeout; if a page
// fails or the deadline fires partway through, the states returned still
// reflect whichever reviews were fetched before that point, alongside the
// error describing why pagination stopped.
func (githubAdapter *GitHubAdapter) LoadReviewerStates(
	ctx context.Context, pullRequestRef model.PullRequestRef,
) ([]model.ReviewerState, error) {
	instance := githubAdapter.Instance()
	prefix := fmt.Sprintf("github %q: load reviews for %s#%d",
		githubAdapter.providerName, pullRequestRef.Repo.Name, pullRequestRef.Number)

	listOptions := &gogithub.ListOptions{PerPage: 100}

	allReviews, err := paginate(ctx, githubAdapter.effectivePaginationTimeout(), maxPaginationPages, prefix, instance,
		func(ctx context.Context, page int) ([]*gogithub.PullRequestReview, *gogithub.Response, error) {
			listOptions.Page = page
			return githubAdapter.rest.PullRequests.ListReviews(
				ctx, pullRequestRef.Repo.Owner, pullRequestRef.Repo.Name, pullRequestRef.Number, listOptions)
		})

	return normalizeReviewerStates(allReviews), err
}

// LoadDiff fetches commit and file-change counts for a PR via the REST detail endpoint.
func (githubAdapter *GitHubAdapter) LoadDiff(
	ctx context.Context, pullRequestRef model.PullRequestRef,
) (model.DiffStats, error) {
	githubPullRequest, response, err := githubAdapter.rest.PullRequests.Get(
		ctx, pullRequestRef.Repo.Owner, pullRequestRef.Repo.Name, pullRequestRef.Number)
	if err != nil {
		return model.DiffStats{}, fmt.Errorf("github %q: load diff for %s#%d: %w",
			githubAdapter.providerName, pullRequestRef.Repo.Name, pullRequestRef.Number, err)
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
