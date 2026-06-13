# ADR-003: Liveness Model

## Status

Proposed

## Context

prsm is a local CLI/TUI tool. It runs entirely on the developer's machine and calls provider REST APIs directly — there is no server component. "Live" in this context means the displayed PR list reflects the current state at the provider within a window that is short enough to be useful for triage, but not necessarily instantaneous.

Five approaches were evaluated:

### 1. Plain polling

Re-fetch all relevant endpoints on a fixed interval. Simple to implement and universally supported by any REST API.

**Provider rate limits (authenticated):**

| Provider        | Limit                                | Notes                                                                                          |
|-----------------|--------------------------------------|------------------------------------------------------------------------------------------------|
| GitHub          | 5,000 req/hr (PAT/OAuth)             | 15,000 req/hr for GitHub Enterprise Cloud orgs. Secondary limit: 900 points/min (GET = 1 pt). |
| GitLab.com      | 2,000 req/min (~120,000 req/hr)       | Very generous; self-managed default is 7,200 req/hr (configurable).                           |
| Gitea           | Instance-configured; no built-in cap | Codeberg enforces ~2,000 req/5 min (24,000 req/hr) via HAProxy.                              |
| Codeberg        | ~24,000 req/hr (HAProxy-enforced)    | Gitea-based; individual instances may vary.                                                   |

**Rate budget math for a typical user (50 PRs across 10 repos on GitHub):**

- Fetching open PRs across 10 repos: 10 `GET /repos/{owner}/{repo}/pulls` calls per cycle
- Fetching reviews/status for 50 PRs on first load: up to 50 calls (can be batched via search API)
- Steady-state polling at 60-second intervals: ~10 calls/cycle × 60 cycles/hr = 600 calls/hr
- That is 12% of the 5,000 req/hr budget, leaving 88% headroom for other API consumers (CI, IDE plugins, other tools using the same PAT)

At a 30-second interval: ~1,200 calls/hr (24% of budget) — still acceptable.

**Staleness window:** equal to the polling interval. At 60 s, events such as "review requested" or "CI passed" are visible within one minute.

### 2. WebSockets / Server-Sent Events (SSE)

Push-based real-time streams from the provider to the client.

| Provider   | WebSocket/SSE support for PR events                                                                                       |
|------------|---------------------------------------------------------------------------------------------------------------------------|
| GitHub     | No public WebSocket or SSE API. GraphQL API does not support subscriptions. Only mechanism is webhooks (see below).       |
| GitLab     | Internal WebSocket (Action Cable / GraphQL subscriptions) used by the web UI for MR status. Not exposed as a public API. |
| Gitea      | No WebSocket or SSE API for PR events.                                                                                    |
| Codeberg   | Gitea-based; same — no streaming API.                                                                                     |

None of the target providers expose a public streaming API for PR events that a local CLI tool can consume. This approach is not viable as a primary strategy.

### 3. Webhooks

The provider pushes events to a URL when something changes.

**Coverage:**

- GitHub: `pull_request` (opened, closed, review_requested, synchronize, ready_for_review, etc.) and `pull_request_review` (submitted, dismissed) events are available at repository and organization scope. No user-level webhook exists, so every repo or org of interest must be registered separately.
- GitLab: `merge_request` events available at project and group scope. No user-level webhook.
- Gitea/Codeberg: `pull_request` and `pull_request_review_approved`/`rejected` events at repository scope. Known gaps: review code-comment webhooks incomplete as of mid-2026.

**Local delivery problem:** Webhooks require a publicly reachable HTTPS URL. For a local CLI tool this means one of:

- A tunnel (ngrok, Hookdeck, Cloudflare Tunnel) — requires a running daemon, an account, and ongoing configuration.
- `gh webhook forward` (GitHub CLI extension) — forwards to localhost with no public URL, but: (a) GitHub-only, (b) explicitly marked as a development/testing tool, not for production, (c) one forwarder per repo/org at a time.
- No equivalent exists for GitLab, Gitea, or Codeberg.

**Registration complexity:** Each repo or org must have a webhook configured pointing at the local listener. For users who monitor dozens of repos across multiple orgs and providers, this is a significant setup burden. Webhooks are also silent-fail by default — if the listener is not running, events are dropped.

**Verdict:** Webhooks are unsuitable as the primary refresh mechanism for a local CLI tool. They may be an opt-in enhancement for power users in a future version.

### 4. Conditional polling (ETags / If-Modified-Since)

Standard polling but with HTTP conditional request headers. When the data has not changed, the provider returns `304 Not Modified` and the response does not count against the primary rate limit budget (GitHub explicitly documents this; GitLab's behavior is less clearly documented but standard HTTP caching semantics apply).

**Provider support:**

| Provider   | ETag / If-None-Match | Last-Modified / If-Modified-Since | Rate limit exemption on 304 |
|------------|----------------------|-----------------------------------|-----------------------------|
| GitHub     | Yes — all REST endpoints return `ETag` | Yes — most endpoints return `Last-Modified` | Yes — 304 does not decrement the 5,000/hr counter |
| GitLab     | Partial — ETag support exists but has had bugs (no-store header broke it in 11.5+); proposal for full ETag support on resource paths open | Limited | Not explicitly documented |
| Gitea      | Not documented in API usage docs; no evidence of ETag support in REST API | No | No |
| Codeberg   | Gitea-based; same | No | No |

GitHub's conditional request exemption is a meaningful benefit: a polling cycle where nothing has changed costs zero rate-limit points (only the secondary 900 points/min limit could apply, but at 10 calls/cycle that is not a concern).

### 5. Hybrid approach

Combine the above into layered behavior:

- **Startup burst:** fetch all data immediately on launch, regardless of interval.
- **Background polling:** tick-based, configurable interval (default 60 s), using conditional requests where supported.
- **Manual refresh key (`r`):** triggers an immediate out-of-cycle fetch for the focused provider or all providers.
- **Rate limit backoff:** detect `429` / `X-RateLimit-Remaining: 0` responses and automatically slow to a safe interval until the reset timestamp.
- **Offline/cache mode:** display stale data with a staleness indicator when all requests fail (network down or rate-limited to zero).

---

### Comparable tools

**k9s (Kubernetes):** Uses the Kubernetes Watch API — a long-lived HTTP stream where the API server pushes delta events (Added, Modified, Deleted) for each resource type. Kubernetes exposes this as a first-class API primitive. The git hosting providers do not offer an equivalent public stream, making this architecture inapplicable directly. k9s also exposes a configurable `refreshRate` (default 2 s) as a fallback.

**lazygit:** Fetches PR state via `gh` CLI on branch load; no continuous background polling of PR status from providers. Manual refresh is the primary mechanism.

**`gh` CLI watch mode:** Uses a `-i` interval flag (seconds); pure polling with no conditional request optimization.

**Bubble Tea (Go TUI framework, candidate tech stack):** Natively supports `tea.Cmd` dispatched to background goroutines returning `tea.Msg` values, and `program.Send()` for external push. This maps cleanly to a tick-based polling loop per provider, with each provider client running as an independent background command chain.

---

## Decision

Adopt a **conditional-polling hybrid** as the default liveness model for v1:

1. **Default interval: 60 seconds**, configurable per-provider via config file (e.g., `refresh_interval_seconds: 30`).
2. **Conditional requests on every poll cycle**: send `If-None-Match` (ETag) and `If-Modified-Since` headers where the provider returned them in the previous response. This eliminates rate-limit cost for unchanged data on GitHub, and is a no-op on providers that ignore these headers.
3. **Startup fetch**: bypass the interval and fetch immediately when prsm launches (or when a new provider/filter is activated).
4. **Manual refresh** via `r` key: triggers an immediate out-of-cycle fetch for all visible providers. This is the primary escape valve for staleness.
5. **Rate limit backoff**: on `429` or exhausted `X-RateLimit-Remaining`, pause that provider's polling loop until `X-RateLimit-Reset` (or a fixed 60 s if the header is absent), then resume. Display a provider-level status indicator in the UI ("GitHub: rate limited, retry in 42 s").
6. **Offline/cache mode**: if a provider returns network errors for two consecutive cycles, mark it as offline and display the last-known data with a visible staleness timestamp. Do not crash or clear the list.
7. **No webhooks in v1**: the setup complexity and GitHub-only forwarding support make webhooks incompatible with the local-tool, multi-provider goals.

The recommended interval (60 s) is the same GitHub recommends in its best-practice documentation for polling integrations. At 60 s with 10 repos and conditional requests, a typical session (4 hours) consumes roughly 2,400 calls against a 20,000-call budget, leaving ample headroom.

## Consequences

### UX implications

- A persistent header line should show per-provider refresh status: last-refreshed timestamp, cycle countdown, and any error/rate-limit state.
- The `r` key must be discoverable in the `?` help panel.
- On first launch, there is a brief loading state before data appears. Subsequent cycles update the list in place without clearing it (delta updates to the in-memory model, not a full redraw from scratch).
- Stale data should be clearly marked (e.g., greyed out or with a clock icon and age) rather than silently shown as current.

### Rate limit budget (typical user, GitHub)

| Scenario                                 | Calls/hr | % of 5,000/hr budget |
|------------------------------------------|----------|----------------------|
| Steady state, 60 s interval, 10 repos   | ~600     | 12%                  |
| Steady state, 30 s interval, 10 repos   | ~1,200   | 24%                  |
| Startup burst (50 PRs, 10 repos)        | ~60 once | one-time             |
| Manual refresh (r key), 10 repos        | ~10 once | one-time             |

Conditional request exemption on 304 responses means the "steady state" numbers above are upper bounds; in practice, most cycles cost zero when the PR list is stable.

### When rate limits are hit

GitHub returns `X-RateLimit-Remaining: 0` and `X-RateLimit-Reset: <unix timestamp>`. prsm should:
1. Stop polling that provider immediately.
2. Display a status line: "GitHub: rate limited — resumes at HH:MM:SS".
3. Resume polling automatically after the reset time.
4. If the user presses `r` while rate-limited, show a brief toast notification rather than making an API call.

### Future upgrade path

If GitHub or GitLab expose a public streaming API or GraphQL subscriptions in future (GitLab's internal GraphQL subscriptions are an early signal this could happen), the polling loop can be replaced per-provider with a streaming subscriber without changing the normalized data model or the UI. The conditional-polling architecture is forward-compatible: the provider client interface just needs a `Subscribe()` method alongside `Poll()`.

Webhook support could be added as an opt-in mode for GitHub power users who already have `gh` installed and are comfortable with the one-time setup per repo/org. This would require a local HTTP listener goroutine, webhook registration via the API, and graceful deregistration on exit.
