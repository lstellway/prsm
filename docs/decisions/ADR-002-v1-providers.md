# ADR-002: v1 Provider Set

## Status
Proposed

## Context

prsm needs to decide which git hosting providers to support at launch and in what priority order. The target user base is engineers who work across multiple repositories and organizations — both open-source contributors (who skew heavily toward GitHub) and team engineers in companies (who are split between GitHub and GitLab, with a long tail of Bitbucket, Azure DevOps, and self-hosted instances).

### Provider landscape

| Provider | Estimated users / scale | API for PR data | Rate limits | Auth options | Self-hosted |
|---|---|---|---|---|---|
| **GitHub** | 150M+ developers; 67.8% VCS platform share; 90%+ Fortune 100 | Excellent (REST + GraphQL; all required fields in list endpoint) | 5,000 req/hr (PAT/OAuth); 5,000–15,000/hr (GitHub App) | PAT, OAuth App, GitHub App | Yes (GitHub Enterprise Server; different base URL, same schema) |
| **GitLab** | 50M+ users; 50%+ Fortune 100; strong in enterprise/regulated | Very good (REST + GraphQL; `head_pipeline`, `reviewers[]`, `detailed_merge_status`, `draft` in list response) | 7,200/hr per user (self-hosted, off by default); search endpoint rate-limited separately | PAT, OAuth, project/group tokens | Yes (self-managed; rate limits off by default, admin-configurable) |
| **Gitea** | 40,000+ tracked production deployments (mid-2025); growing in air-gapped/regulated environments | Good (REST only; `draft`, `mergeable`, `review_comments`, `labels`, `created` supported) | Configurable per instance; no hard platform default | Basic auth, PAT, API token, OAuth | Yes (primary use case) |
| **Forgejo** | Powers Codeberg; maintained fork of Gitea with faster iteration; 3,039 commits vs. Gitea's 1,228 since 2024 hard fork | Good; API strives for Gitea compatibility but diverges (new `/api/forgejo/v1/` namespace; additional endpoints for Actions) | Configurable per instance | Same as Gitea | Yes (primary use case) |
| **Codeberg** | 300,000+ repositories; 200,000+ users (Nov 2025); runs Forgejo | Forgejo API (identical paths to Gitea API at `/api/v1/`) | Shared instance limits; conservative defaults | PAT, OAuth | No (hosted service) |
| **Bitbucket** | ~7.2% VCS market share; concentrated in Atlassian/Jira shops | Good REST API; PR data complete | 1,000 req/hr (free); higher on paid plans | App passwords, OAuth, Atlassian API tokens | Yes (Bitbucket Data Center) |
| **Azure DevOps** | ~9.71% of tracked companies; predominantly Microsoft-stack enterprises | Mature REST API; pull request fields complete | Generous (global rate limits, not per-resource) | PAT, OAuth, AAD | Yes (Azure DevOps Server) |
| **Sourcehut** | Small, niche; email-based review model | REST API exists but review model is email-based, not PR-based | — | Tokens | Yes |

### GitHub API completeness

The GitHub REST list-pulls endpoint (`GET /repos/{owner}/{repo}/pulls`) returns `title`, `user` (author), `draft`, `labels`, `created_at`, `state`, `comments`, `review_comments`, `mergeable`, `mergeable_state`, and `requested_reviewers` directly. CI/check status requires a separate call (`/commits/{ref}/check-runs`). GitHub's GraphQL API can batch PR list + review status + check status into a single query, making it the preferred implementation approach. Authentication supports PAT (classic and fine-grained), OAuth Apps, and GitHub Apps. GitHub Enterprise Server uses the same REST schema at a different base URL (`HOSTNAME/api/v3`), with rate limits disabled by default.

### GitLab API completeness

The GitLab REST list-MRs endpoint returns `title`, `author`, `draft`, `labels`, `reviewers[]`, `head_pipeline` (CI status), `detailed_merge_status`, `user_notes_count` (comments), and `created_at` directly in the list response — notably including pipeline status inline, which GitHub requires an extra call for. GraphQL support exists but has gaps (approver-filter parity with REST is an open issue). Self-hosted GitLab provides full API parity; rate limits are admin-configurable (7,200/hr per user when enabled). The terminology difference (merge requests vs. pull requests) is cosmetic; the data model is equivalent.

### Gitea/Forgejo/Codeberg relationship and API implications

Gitea and Forgejo shared the same codebase until early 2024, when Forgejo became a hard fork. Both expose a compatible REST API at `/api/v1/`. The fields needed by prsm (`draft`, `mergeable`, `review_comments`, `labels`, `user`, `created`, `state`) are present in both. Forgejo adds a parallel `/api/forgejo/v1/` namespace for Forgejo-specific extensions (Actions endpoints, repository-scoped tokens in v15.0+) but preserves the base Gitea API for backwards compatibility. In practice: a single Gitea/Forgejo adapter covers Gitea instances, Forgejo instances, and Codeberg. There is no need for a separate Codeberg provider — Codeberg is a Forgejo instance and responds to the same API calls.

The meaningful divergence between Gitea and Forgejo does not affect the PR data fields prsm needs today; it affects webhooks, branch protection internals, and admin endpoints. A thin Forgejo-specific client can be layered on top of the shared Gitea client in the future if federation or Forgejo-exclusive endpoints become relevant.

### Providers not recommended for v1

- **Bitbucket**: API is complete, but its user base (7.2% market share) is heavily concentrated in Atlassian shops. It is a valid future target but does not materially expand prsm's addressable audience at launch.
- **Azure DevOps**: Mature API, but users are predominantly Microsoft-stack enterprises not well-served by a terminal tool targeting open-source contributors and individual engineers.
- **Sourcehut**: Niche; email-based review model is fundamentally incompatible with prsm's PR-inbox mental model.

## Decision

Target three providers for v1, implemented in this priority order:

1. **GitHub** (github.com + GitHub Enterprise Server) — implemented first. GitHub establishes the provider adapter interface and normalization patterns that all subsequent providers will follow.
2. **GitLab** (gitlab.com + self-hosted) — added once the GitHub adapter and provider interface are stable and proven.
3. **Gitea/Forgejo** as a single adapter covering Gitea instances, Forgejo instances, and Codeberg — added after GitLab, inheriting the same adapter pattern.

**Implementation approach:** Build GitHub first and treat it as the pattern-setter. The provider adapter interface (fetching, normalization, auth, polling) is defined through the GitHub implementation. GitLab and Gitea/Forgejo then follow that interface, making each addition a matter of filling in a known shape rather than re-architecting.

Codeberg does not require a separate adapter — it is a Forgejo instance and is covered by the Gitea/Forgejo adapter at no extra cost. Bitbucket and Azure DevOps are deferred to a post-v1 roadmap.

## Consequences

### Data model

The generic PR schema must accommodate:
- **Terminology**: GitHub "pull requests" vs. GitLab "merge requests" — internal schema uses a single normalized type; provider adapters translate.
- **CI status**: GitHub requires a separate check-runs call per PR; GitLab includes `head_pipeline` inline; Gitea/Forgejo expose CI status via separate Gitea Actions endpoints (not always present). The data model should treat CI status as optional/nullable at the list level and fetched lazily or in a background refresh.
- **Review status**: GitHub list endpoint includes `requested_reviewers` but not submitted review states (requires `/reviews` call or GraphQL); GitLab includes `reviewers[]` inline but approval state requires `/approvals`; Gitea/Forgejo include `review_comments` count but not individual reviewer state at list level. Same lazy-fetch approach applies.
- **Draft state**: All three providers return `draft: bool` in the list response — safe to include in the primary view.
- **Self-hosted base URLs**: GitHub Enterprise and self-hosted GitLab/Gitea need a configurable `base_url` per provider instance in the config schema. This is an explicit design requirement, not a future concern.

### Implementation scope

v1 scope is read-only: fetch, normalize, and display PR data. No write operations (approve, comment, merge). Three provider adapters is achievable for an initial release without overloading the scope. The Gitea adapter is the smallest lift since Forgejo and Codeberg inherit it for free.

### Users not served at v1

Engineers using Bitbucket, Azure DevOps, or Sourcehut exclusively will not have a working tool at launch. This is an acceptable tradeoff given their smaller share of the target audience and the added complexity of implementing and maintaining additional adapters before the core product is validated.
