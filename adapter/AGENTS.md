# Vendor adapters

Each vendor package here implements the resource-kind interfaces in `adapter.go` for one vendor. Read `adapter.go`'s package doc for why the interfaces are split per resource kind, and the root `AGENTS.md` for the three kinds of "no" an adapter must keep distinguishable.

## Writing an adapter

- `github/` is the reference implementation: constructor shape, normalization, ETag caching, rate limiting, per-operation pagination deadlines, and the naming convention. Follow its shape.
- Implement only the interfaces the vendor actually serves. Never add a method that returns "unsupported" — not implementing the interface is how a source says it does not serve a kind.
- Scope types (repository, project, group references) are vendor-local and stay in the vendor package. No vendor vocabulary in `adapter/`, `model/`, `query/`, or the assembly layer.
- Return the shared error types in `errors.go` (rate limit, auth, not found) for vendor failures, so the assembly layer can classify a connection's state without knowing the vendor.

## Vendor order (v1)

1. **GitHub** (github.com + GitHub Enterprise Server) — done
2. **GitLab** (gitlab.com + self-hosted) — #27
3. **Gitea/Forgejo** — #29; one adapter covering Gitea, Forgejo, and Codeberg

A shared conformance suite (#26) must land before the GitLab adapter, so the second vendor is verified by the same assertions as the first.

Codeberg is a Forgejo instance and shares the Gitea adapter. Forgejo forked from Gitea and their APIs are drifting; treat the shared adapter as current fact, not a permanent guarantee.

## Testing

- Unit tests run the adapter against Go's in-process `httptest` server. No containers, no HTTP recording tools.
- `mock/` is a small in-memory adapter for unit tests elsewhere in the module. Keep it simple.
- Tests behind the `integration` build tag hit the real vendor API and need `INTEGRATION_GITHUB_TOKEN`. CI vets them so they keep compiling; `.github/workflows/integration.yml` runs them nightly.

## Reference

- **herdr** — Rust terminal multiplexer for coding-agent CLIs. Per-vendor behavior lives in declarative TOML manifests that ship independently of the binary behind an engine-version handshake, while vendor identity stays compiled. The fallback design if prsm's per-connection capability tables ever outgrow the release cadence.
- **Renovate** (11 code hosts) and **rclone** (~70 backends) — comparable tools that reached broad vendor coverage with writes by compiling vendors in, which is why prsm has no plugin runtime.
