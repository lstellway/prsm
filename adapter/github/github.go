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
	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

const defaultAPIBaseURL = "https://api.github.com"

// Config holds the parameters needed to construct a GitHubAdapter.
// The assembly layer maps config.ProviderConfig into this type so the adapter
// package has no dependency on the config package.
type Config struct {
	Name    string
	Token   string
	BaseURL string
	Repos   []adapter.RepoRef
}

// GitHubAdapter is the prsm provider adapter for GitHub.
// It is safe for concurrent use and must not be copied once constructed.
type GitHubAdapter struct {
	providerName string
	instance     model.ProviderInstance // immutable after New(); Account is excluded
	repos        []adapter.RepoRef
	rest         *gogithub.Client

	// mu guards state resolved after construction. ADR-009's assembly layer
	// resolves identity at startup while fetches are already fanning out, so
	// ResolveIdentity and Instance genuinely run concurrently on one adapter.
	// Holding Account outside instance avoids tearing the struct; it does not
	// on its own make the string safe to read while another goroutine writes it.
	mu      sync.RWMutex
	account string // written by ResolveIdentity, read by Instance
}

// New constructs a GitHubAdapter from a Config.
// The token in cfg.Token must already be expanded (no "$VAR" references).
func New(cfg Config) (*GitHubAdapter, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("github adapter %q: token is required", cfg.Name)
	}

	httpClient := newHTTPClient(cfg.Token)

	apiBase := strings.TrimRight(cfg.BaseURL, "/")
	if apiBase == "" {
		apiBase = defaultAPIBaseURL
	}

	// Pass our pre-configured http.Client (oauth2 auth + ETag caching) directly.
	// Do NOT also call WithAuthToken — auth is already handled by the transport.
	restOpts := []gogithub.ClientOptionsFunc{gogithub.WithHTTPClient(httpClient)}
	if apiBase != defaultAPIBaseURL {
		restOpts = append(restOpts, gogithub.WithEnterpriseURLs(apiBase+"/", apiBase+"/"))
	}
	restClient, err := gogithub.NewClient(restOpts...)
	if err != nil {
		return nil, fmt.Errorf("github adapter %q: %w", cfg.Name, err)
	}

	host := "github.com"
	if cfg.BaseURL != "" {
		host = extractHost(cfg.BaseURL)
	}

	return &GitHubAdapter{
		providerName: cfg.Name,
		instance: model.ProviderInstance{
			Name: cfg.Name,
			Kind: model.ProviderGitHub,
			Host: host,
			// Account is populated by ResolveIdentity once called at startup.
		},
		// Cloned so a caller mutating its own slice after New() cannot reach
		// adapter state. New() is callable without the config layer, so the
		// caller is not guaranteed to hand over a freshly built slice.
		repos: slices.Clone(cfg.Repos),
		rest:  restClient,
	}, nil
}

// Kind returns the provider kind for this adapter instance.
func (a *GitHubAdapter) Kind() model.ProviderKind { return model.ProviderGitHub }

// Instance returns the full ProviderInstance this adapter serves.
// Account is composed at call time under the read lock, so callers always see
// either the value set by ResolveIdentity or the empty string it started as —
// never a partially written one.
func (a *GitHubAdapter) Instance() model.ProviderInstance {
	a.mu.RLock()
	defer a.mu.RUnlock()
	inst := a.instance
	inst.Account = a.account
	return inst
}

// ResolveIdentity returns the authenticated user's identity and populates
// Instance().Account with the resolved GitHub login for "me" sentinel resolution.
func (a *GitHubAdapter) ResolveIdentity(ctx context.Context) (model.Identity, error) {
	user, resp, err := a.rest.Users.Get(ctx, "")
	if err != nil {
		return model.Identity{}, fmt.Errorf("github %q: resolve identity: %w", a.providerName, err)
	}
	if err := checkRateLimit(a.Instance(), resp.Response); err != nil {
		return model.Identity{}, err
	}

	name := user.GetName()
	if name == "" {
		name = user.GetLogin()
	}

	a.mu.Lock()
	a.account = user.GetLogin()
	a.mu.Unlock()

	return model.Identity{
		Username:    user.GetLogin(),
		DisplayName: name,
		AvatarURL:   user.GetAvatarURL(),
	}, nil
}

// ListPullRequests fetches all open pull requests across configured repos via REST.
// Results from repos that succeed are returned even when other repos fail;
// all errors are joined and returned alongside the partial result.
func (a *GitHubAdapter) ListPullRequests(ctx context.Context) ([]model.PullRequest, error) {
	var all []model.PullRequest
	var errs []error
	for _, ref := range a.repos {
		prs, err := a.listRepoPRs(ctx, ref.Owner, ref.Repo)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, prs...)
	}
	return all, errors.Join(errs...)
}

func (a *GitHubAdapter) listRepoPRs(ctx context.Context, owner, repo string) ([]model.PullRequest, error) {
	// 50 pages × 100 PRs = 5,000 PRs maximum per repo per call.
	const maxPages = 50

	opts := &gogithub.PullRequestListOptions{
		State:       "open",
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	// Read once so every PR in this call is stamped with the same instance,
	// rather than later pages picking up an identity resolved mid-fetch.
	inst := a.Instance()

	var all []model.PullRequest
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("github %q: list PRs %s/%s: exceeded %d-page limit",
				a.providerName, owner, repo, maxPages)
		}

		prs, resp, err := a.rest.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("github %q: list PRs %s/%s: %w", a.providerName, owner, repo, err)
		}
		if rlErr := checkRateLimit(inst, resp.Response); rlErr != nil {
			return nil, rlErr
		}

		for _, pr := range prs {
			all = append(all, normalizePR(pr, owner, repo, inst))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}

// LoadCI fetches CI/check-run status for a PR's head commit via REST.
func (a *GitHubAdapter) LoadCI(ctx context.Context, pr model.PullRequest) (model.CIStatus, error) {
	if pr.HeadSHA == "" {
		return model.CIStatus{State: model.CIStateNone}, nil
	}

	// 50 pages × 100 runs = 5,000 check runs maximum per SHA.
	const maxPages = 50

	opts := &gogithub.ListCheckRunsOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	var allRuns []*gogithub.CheckRun
	for page := 1; ; page++ {
		if page > maxPages {
			return model.CIStatus{}, fmt.Errorf("github %q: load CI for %s#%d: exceeded %d-page limit",
				a.providerName, pr.Repo.Name, pr.Number, maxPages)
		}
		runs, resp, err := a.rest.Checks.ListCheckRunsForRef(
			ctx, pr.Repo.Owner, pr.Repo.Name, pr.HeadSHA, opts)
		if err != nil {
			return model.CIStatus{}, fmt.Errorf("github %q: load CI for %s#%d: %w",
				a.providerName, pr.Repo.Name, pr.Number, err)
		}
		if rlErr := checkRateLimit(a.Instance(), resp.Response); rlErr != nil {
			return model.CIStatus{}, rlErr
		}
		allRuns = append(allRuns, runs.CheckRuns...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return normalizeCIStatus(allRuns), nil
}

// LoadReviewerStates fetches individual review decisions for a PR via REST.
func (a *GitHubAdapter) LoadReviewerStates(ctx context.Context, pr model.PullRequest) ([]model.ReviewerState, error) {
	// 50 pages × 100 reviews = 5,000 reviews maximum per PR.
	const maxPages = 50

	opts := &gogithub.ListOptions{PerPage: 100}
	var allReviews []*gogithub.PullRequestReview

	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("github %q: load reviews for %s#%d: exceeded %d-page limit",
				a.providerName, pr.Repo.Name, pr.Number, maxPages)
		}
		reviews, resp, err := a.rest.PullRequests.ListReviews(
			ctx, pr.Repo.Owner, pr.Repo.Name, pr.Number, opts)
		if err != nil {
			return nil, fmt.Errorf("github %q: load reviews for %s#%d: %w",
				a.providerName, pr.Repo.Name, pr.Number, err)
		}
		if rlErr := checkRateLimit(a.Instance(), resp.Response); rlErr != nil {
			return nil, rlErr
		}
		allReviews = append(allReviews, reviews...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return normalizeReviewerStates(allReviews), nil
}

// LoadDiff fetches commit and file-change counts for a PR via the REST detail endpoint.
func (a *GitHubAdapter) LoadDiff(ctx context.Context, pr model.PullRequest) (model.DiffStats, error) {
	ghPR, resp, err := a.rest.PullRequests.Get(ctx, pr.Repo.Owner, pr.Repo.Name, pr.Number)
	if err != nil {
		return model.DiffStats{}, fmt.Errorf("github %q: load diff for %s#%d: %w",
			a.providerName, pr.Repo.Name, pr.Number, err)
	}
	if rlErr := checkRateLimit(a.Instance(), resp.Response); rlErr != nil {
		return model.DiffStats{}, rlErr
	}
	return model.DiffStats{
		Commits:      ghPR.GetCommits(),
		ChangedFiles: ghPR.GetChangedFiles(),
		Additions:    ghPR.GetAdditions(),
		Deletions:    ghPR.GetDeletions(),
	}, nil
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Hostname()
}

