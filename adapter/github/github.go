// Package github implements the prsm provider adapter for GitHub.com and
// GitHub Enterprise Server. It uses the GraphQL API for ListPullRequests
// (batches PR list + requested reviewers in one query per repo) and the
// REST API for LoadCI, LoadReviewerStates, and LoadDiff.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/lstellway/prsm/config"
	"github.com/lstellway/prsm/model"
)

const (
	defaultAPIBaseURL     = "https://api.github.com"
	defaultGraphQLBaseURL = "https://api.github.com/graphql"
	graphQLPageSize       = 100
)

// GitHubAdapter is the prsm provider adapter for GitHub.
type GitHubAdapter struct {
	providerName string
	instance     model.ProviderInstance
	repos        []config.RepoRef
	rest         *gogithub.Client
	graphqlURL   string
	httpClient   *http.Client
	etags        *etagCache
}

// New constructs a GitHubAdapter from a ProviderConfig.
// The token in cfg.Auth.Token must already be expanded (no "$VAR" references).
func New(cfg config.ProviderConfig) (*GitHubAdapter, error) {
	if cfg.Auth.Token == "" {
		return nil, fmt.Errorf("github adapter %q: auth.token is required", cfg.Name)
	}

	httpClient := newHTTPClient(context.Background(), cfg.Auth.Token)

	apiBase := strings.TrimRight(cfg.BaseURL, "/")
	if apiBase == "" {
		apiBase = defaultAPIBaseURL
	}

	graphqlURL := defaultGraphQLBaseURL
	if cfg.BaseURL != "" {
		// GitHub Enterprise: strip /api/v3 suffix and append /api/graphql.
		host := strings.TrimSuffix(strings.TrimRight(cfg.BaseURL, "/"), "/api/v3")
		graphqlURL = host + "/api/graphql"
	}

	restOpts := []gogithub.ClientOptionsFunc{gogithub.WithAuthToken(cfg.Auth.Token)}
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
			Name:    cfg.Name,
			Kind:    model.ProviderGitHub,
			Host:    host,
			Account: cfg.Name,
		},
		repos:      cfg.Repos,
		rest:       restClient,
		graphqlURL: graphqlURL,
		httpClient: httpClient,
		etags:      newETagCache(),
	}, nil
}

// Kind returns the provider kind for this adapter instance.
func (a *GitHubAdapter) Kind() model.ProviderKind { return model.ProviderGitHub }

// Instance returns the full ProviderInstance this adapter serves.
func (a *GitHubAdapter) Instance() model.ProviderInstance { return a.instance }

// ResolveIdentity returns the authenticated user's identity.
func (a *GitHubAdapter) ResolveIdentity(ctx context.Context) (model.Identity, error) {
	user, resp, err := a.rest.Users.Get(ctx, "")
	if err != nil {
		return model.Identity{}, fmt.Errorf("github %q: resolve identity: %w", a.providerName, err)
	}
	if err := checkRateLimit(a.instance,resp.Response); err != nil {
		return model.Identity{}, err
	}

	name := user.GetName()
	if name == "" {
		name = user.GetLogin()
	}
	return model.Identity{
		Username:    user.GetLogin(),
		DisplayName: name,
		AvatarURL:   user.GetAvatarURL(),
	}, nil
}

// ListPullRequests fetches all open pull requests across configured repos via GraphQL.
func (a *GitHubAdapter) ListPullRequests(ctx context.Context) ([]model.PullRequest, error) {
	var all []model.PullRequest
	for _, ref := range a.repos {
		prs, err := a.listRepoPRs(ctx, ref.Owner, ref.Repo)
		if err != nil {
			return nil, err
		}
		all = append(all, prs...)
	}
	return all, nil
}

func (a *GitHubAdapter) listRepoPRs(ctx context.Context, owner, repo string) ([]model.PullRequest, error) {
	return a.listRepoPRsWithCursor(ctx, owner, repo, "")
}

func (a *GitHubAdapter) listRepoPRsWithCursor(ctx context.Context, owner, repo, cursor string) ([]model.PullRequest, error) {
	const query = `
query ListPullRequests($owner: String!, $name: String!, $after: String, $first: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequests(first: $first, after: $after, states: [OPEN]) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id
        number
        title
        body
        url
        state
        isDraft
        headRefName
        baseRefName
        headRefOid
        mergeable
        createdAt
        updatedAt
        mergedAt
        author {
          login
          ... on User { name avatarUrl }
        }
        labels(first: 20) {
          nodes { name color }
        }
        reviewRequests(first: 20) {
          nodes {
            requestedReviewer {
              ... on User { login name }
            }
          }
        }
        comments { totalCount }
        repository {
          name
          owner { login }
        }
      }
    }
  }
}`

	vars := map[string]any{
		"owner": owner,
		"name":  repo,
		"first": graphQLPageSize,
		"after": nil,
	}
	if cursor != "" {
		vars["after"] = cursor
	}

	var result struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
					Nodes []prNode
				}
			}
		}
		Errors []struct{ Message string }
	}

	if err := a.runGraphQL(ctx, query, vars, &result); err != nil {
		return nil, err
	}
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("github %q: graphql: %s", a.providerName, strings.Join(msgs, "; "))
	}

	nodes := result.Data.Repository.PullRequests.Nodes
	prs := make([]model.PullRequest, len(nodes))
	for i, node := range nodes {
		prs[i] = normalizePR(node, a.instance)
	}

	if result.Data.Repository.PullRequests.PageInfo.HasNextPage {
		next, err := a.listRepoPRsWithCursor(ctx, owner, repo,
			result.Data.Repository.PullRequests.PageInfo.EndCursor)
		if err != nil {
			return nil, err
		}
		prs = append(prs, next...)
	}

	return prs, nil
}

// LoadCI fetches CI/check-run status for a PR's head commit via REST.
func (a *GitHubAdapter) LoadCI(ctx context.Context, pr model.PullRequest) (model.CIStatus, error) {
	if pr.HeadSHA == "" {
		return model.CIStatus{State: model.CIStateNone}, nil
	}

	opts := &gogithub.ListCheckRunsOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	var allRuns []*gogithub.CheckRun
	for {
		runs, resp, err := a.rest.Checks.ListCheckRunsForRef(
			ctx, pr.Repo.Owner, pr.Repo.Name, pr.HeadSHA, opts)
		if err != nil {
			return model.CIStatus{}, fmt.Errorf("github %q: load CI for %s#%d: %w",
				a.providerName, pr.Repo.Name, pr.Number, err)
		}
		if rlErr := checkRateLimit(a.instance,resp.Response); rlErr != nil {
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
	opts := &gogithub.ListOptions{PerPage: 100}
	var allReviews []*gogithub.PullRequestReview

	for {
		reviews, resp, err := a.rest.PullRequests.ListReviews(
			ctx, pr.Repo.Owner, pr.Repo.Name, pr.Number, opts)
		if err != nil {
			return nil, fmt.Errorf("github %q: load reviews for %s#%d: %w",
				a.providerName, pr.Repo.Name, pr.Number, err)
		}
		if rlErr := checkRateLimit(a.instance,resp.Response); rlErr != nil {
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
	if rlErr := checkRateLimit(a.instance,resp.Response); rlErr != nil {
		return model.DiffStats{}, rlErr
	}
	return model.DiffStats{
		Commits:      ghPR.GetCommits(),
		ChangedFiles: ghPR.GetChangedFiles(),
		Additions:    ghPR.GetAdditions(),
		Deletions:    ghPR.GetDeletions(),
	}, nil
}

// runGraphQL executes a GitHub GraphQL query and decodes the response into result.
// It attaches If-None-Match headers and handles 304 responses from the ETag cache.
func (a *GitHubAdapter) runGraphQL(ctx context.Context, query string, variables map[string]any, result any) error {
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return fmt.Errorf("github %q: graphql marshal: %w", a.providerName, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.graphqlURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("github %q: graphql request: %w", a.providerName, err)
	}
	req.Header.Set("Content-Type", "application/json")
	a.etags.setRequestHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github %q: graphql: %w", a.providerName, err)
	}
	defer resp.Body.Close()

	if rlErr := checkRateLimit(a.instance,resp); rlErr != nil {
		return rlErr
	}

	key := a.graphqlURL + "?" + cacheKey(query)

	if resp.StatusCode == http.StatusNotModified {
		if cached, ok := a.etags.loadValue(key); ok {
			data, err := json.Marshal(cached)
			if err == nil {
				return json.Unmarshal(data, result)
			}
		}
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github %q: graphql: HTTP %d: %s", a.providerName, resp.StatusCode, string(b))
	}

	a.etags.recordResponse(a.graphqlURL, resp)

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("github %q: graphql decode: %w", a.providerName, err)
	}

	a.etags.storeValue(key, result)
	return nil
}

func extractHost(rawURL string) string {
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	if i := strings.Index(rawURL, "/"); i >= 0 {
		rawURL = rawURL[:i]
	}
	return rawURL
}

func cacheKey(query string) string {
	const maxLen = 64
	if len(query) > maxLen {
		return query[:maxLen]
	}
	return query
}
