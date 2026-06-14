# Go Project Structure Research

## Key constraints

From the ADRs:

1. **The library is a first-class consumer, not an afterthought.** ADR-000 names "library" as one of four planned consumers alongside the TUI, MCP server, and HTTP API. The Event Engine's `Subscribe()` API (ADR-007) is explicitly designed for library consumers. This means the resource model, query layer, and event stream packages must be importable by third-party code.

2. **The TUI is the first consumer but must not be privileged.** Nothing in the model, query layer, or adapters encodes TUI-specific concerns. The `PullRequest` type carries no presentation logic (ADR-004). Provider adapters carry no consumer knowledge (ADR-000). Packages that exist solely to serve the TUI should not appear in the import path of a library consumer.

3. **Bubble Tea is a TUI-only dependency.** `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, and `charm.land/bubbles/v2` are exclusively needed by the TUI consumer. A library consumer using only the resource model and event stream should not transitively pull in Bubble Tea. This requires the TUI code to live in a package that library consumers do not import.

4. **Strict downward-only layer dependencies** (ADR-000). Adapters must not import consumer packages. The resource model must not import adapters or consumers. The query layer must not import adapters or consumers. The event engine must not import consumers. Any package organization that allows upward imports is wrong by definition.

5. **gRPC, MCP, and HTTP are future consumers.** The architecture must leave room for `cmd/prsm-server` (or a `serve` subcommand) and the generated gRPC stubs, MCP handler code, and HTTP handler code that will accompany them. These should not require reorganizing the repository when they arrive.

6. **Single provider adapter interface, multiple implementations.** Three v1 providers (GitHub, GitLab, Gitea/Forgejo) each implement the same `ProviderAdapter` interface. New providers are additions, not modifications. They need an obvious home in the package tree.

7. **Config is shared across all consumers.** The TOML config (ADR-005) is loaded at startup by every consumer — TUI, server, library consumer. It is not TUI-specific and must live outside any TUI-only package.

---

## Research findings

### Pattern 1: Go CLI that is also an importable library — wishlist, atlas, charm

**charmbracelet/wishlist** (`github.com/charmbracelet/wishlist`) is the clearest analog in the Charm ecosystem. It is a single Go module with this structure:
- Library code lives at the root package and in subdirectories (`blocking/`, `home/`, `srv/`, etc.)
- The binary entry point lives in `cmd/wishlist/`
- Examples live in `_example/`
- No `internal/` for the core library — everything that matters to callers is exported

This is the "library at root" pattern. It works well when the core abstraction is the library and the binary is a thin wrapper. The downside is it places the library import at `github.com/charmbracelet/wishlist` (root), which creates ambiguity when the package name and module name diverge or when there are multiple logical sub-libraries.

**ariga/atlas** takes the opposite approach: separate concerns into named subdirectories, with `sdk/` for the public library, `cmd/atlas/` for the binary, and `internal/` for private implementation. The root module covers all of them. This is the pattern to follow when the binary and library have meaningfully distinct package APIs.

**hashicorp/vault** goes further, extracting `api/` as a separate Go module (`github.com/hashicorp/vault/api`) with its own `go.mod`. This is appropriate when the library has a much smaller dependency surface than the application — Vault's CLI has thousands of dependencies; `vault/api` needs only a few HTTP client packages. The cost is coordination overhead: two versioning streams, two `go.sum` files, a Go workspace for local development.

### Pattern 2: CLI with `pkg/` for exported utilities — gh CLI

The **gh CLI** (`github.com/cli/cli/v2`) uses:
- `cmd/gh/` — the binary
- `internal/` — all application logic (the vast majority of the codebase)
- `pkg/` — a handful of packages intended for external use by extension authors

This is notably *not* a library-first design. gh's core is `internal/`; `pkg/` is a collection of utilities extension authors can use. The project separately maintains `github.com/cli/go-gh/v2` as a proper library with its own module, `internal/` packages, and `pkg/` surface. The split happened precisely because pulling in the full gh module for library use was prohibitive — too many TUI, authentication, and platform-specific dependencies.

**The lesson from gh/go-gh:** if the binary and library share a module and the binary has heavy TUI dependencies (Bubble Tea), library consumers transitively pull in Bubble Tea even if they never render anything. This is the multi-module argument.

### Pattern 3: Pure library with `internal/` for implementation details — bubbletea, huh, mcp-go

**charmbracelet/bubbletea** and **charm.land/huh/v2** are single-module libraries with a flat structure — most code at the root package with minimal subdirectories. Their `internal/` directories contain rendering primitives and implementation details that are not part of the stable API.

**mark3labs/mcp-go** and the **official MCP Go SDK** (`github.com/modelcontextprotocol/go-sdk`) both use named sub-packages: `mcp/` for core types, `server/` for server logic, `client/` for client logic, `jsonrpc/` for transport. This maps well to prsm's layered architecture.

### Pattern 4: Single binary with subcommands — most Go CLIs

Almost all modern Go CLI tools (gh, atlas, flyctl, ollama) use a single binary with subcommands rather than multiple binaries. The Go CLI community has converged on Cobra for command trees. The practical rationale: users install one binary, distribution is simpler, and subcommands can share config loading, logging, and startup logic without an IPC layer.

flyctl ships one binary (`flyctl` or `fly`) with dozens of subcommands. ollama ships one binary with `serve`, `run`, `pull`, etc. The same `main.go` mounts all subcommand trees.

**Exception pattern:** some projects ship two binaries when one is a daemon and one is a client (e.g., `dockerd`/`docker`, `containerd`/`ctr`). This is appropriate when the server must run continuously as a system daemon separate from user sessions. prsm does not have this requirement — the HTTP/gRPC server is an optional mode of a tool that users choose to run as a server, not a system-level daemon.

### Pattern 5: Proto/gRPC placement — buf, connect-go, grpc-go

**buf** keeps `.proto` files in a top-level `proto/` directory and generated Go code either alongside them or in a generated sub-package. **connect-go** puts protos and generated code in `internal/proto/` and `internal/gen/` respectively, keeping them away from the public API surface. **grpc-go** uses `cmd/protoc-gen-go-grpc/` for the code generator tool and keeps generated stubs in feature-specific subdirectories.

The common pattern for projects where gRPC is a transport (not the product): `.proto` files in `api/proto/` or `proto/`, generated Go code in `api/gen/` or `gen/`. The generator is typically not checked in; `go generate` or a `make` target invokes `protoc` or `buf`. Generated files are committed so consumers don't need the Protobuf toolchain.

### golang-standards/project-layout critique

This is not an official Go standard. The Go team's own guidance is minimal: put `main.go` in `cmd/<name>/`, use `internal/` for private packages, and avoid `src/`. The `pkg/` convention from golang-standards is contested — many Go projects (including in the standard library) skip it entirely. The useful take: `internal/` is the Go compiler-enforced boundary; `pkg/` is a social convention with no enforcement. For a library-first project, leading with named packages at the module root (`model`, `adapter`, `query`) is cleaner than nesting them under `pkg/`.

### Single module vs. multi-module verdict

The Go team's own multi-module documentation recommends against multiple `go.mod` files in a single repository unless there is a "compelling reason" such as dramatically different dependency graphs or independent versioning requirements. The tooling cost is real: `go work` files, separate `go.sum` files, and the risk of accidental API surface breakage between modules.

**For prsm, the compelling reason exists but is avoidable.** The TUI-specific concern (Bubble Tea pulling in as a transitive dependency for library consumers) is real but can be solved within a single module by keeping Bubble Tea imports confined to packages that library consumers never import (`internal/tui/` or `cmd/prsm/`). A library consumer that imports `github.com/lstellway/prsm/model` or `github.com/lstellway/prsm/adapter/github` does not import Bubble Tea unless those packages themselves import it — which they must not by the layer boundary rules in ADR-000.

The single-module approach is recommended. Multi-module is worth revisiting if and when `go mod tidy` on a downstream library consumer shows Bubble Tea appearing in their `go.sum`.

---

## Recommended structure

```
github.com/lstellway/prsm/          ← module root
├── go.mod                           ← module: github.com/lstellway/prsm
├── go.sum
├── CLAUDE.md
├── Makefile                         ← build, test, generate targets
│
├── cmd/
│   └── prsm/                        ← the single binary entry point
│       └── main.go                  ← mounts all subcommands; thin, ~30 lines
│
├── model/                           ← Layer 2: exported resource types
│   ├── pr.go                        ← PullRequest, PRState, ReviewDecision, etc.
│   ├── load.go                      ← LoadResult[T], LoadState
│   ├── provider.go                  ← ProviderKind, ProviderInstance
│   └── common.go                    ← Author, Label, Repository, etc.
│
├── adapter/                         ← Layer 1: exported adapter interface + implementations
│   ├── adapter.go                   ← ProviderAdapter interface
│   ├── github/
│   │   ├── github.go                ← GitHubAdapter struct + constructor
│   │   ├── normalize.go             ← raw API → PullRequest mapping
│   │   └── auth.go                  ← PAT, OAuth handling
│   ├── gitlab/
│   │   ├── gitlab.go
│   │   ├── normalize.go
│   │   └── auth.go
│   └── gitea/
│       ├── gitea.go                 ← covers Gitea, Forgejo, Codeberg
│       ├── normalize.go
│       └── auth.go
│
├── query/                           ← Layer 3: exported filter/sort/group logic
│   ├── filter.go                    ← PRFilterSpec, BaseFilterSpec
│   ├── predicate.go                 ← Predicate[T], And/Or combinators
│   ├── sort.go                      ← SortSpec, sort key constants
│   └── group.go                     ← GroupSpec, group key constants
│
├── event/                           ← Layer 4 (v1.1): exported event engine
│   ├── engine.go                    ← Engine[T], delta detection loop
│   ├── bus.go                       ← Bus[T], Subscribe/Publish/Close
│   ├── event.go                     ← Event[T], EventKind constants
│   └── stream.go                    ← EventStream[T] public library API
│
├── config/                          ← Shared config types and loader (all consumers)
│   ├── config.go                    ← Config, ProviderConfig, ViewConfig structs
│   ├── load.go                      ← LoadFile, validation, env var expansion
│   └── defaults.go                  ← default values, XDG path resolution
│
├── client/                          ← High-level library entry point (assembles layers)
│   └── client.go                    ← Client struct: New(config), PullRequests(), Events()
│
├── internal/
│   ├── tui/                         ← TUI consumer (Bubble Tea; not importable externally)
│   │   ├── app.go                   ← tea.Program wiring, top-level Model
│   │   ├── list/                    ← PR list panel (Model/Update/View)
│   │   ├── detail/                  ← PR detail panel
│   │   ├── filter/                  ← session filter bar
│   │   ├── views/                   ← view picker panel
│   │   ├── notifications/           ← event notification panel (v1.1)
│   │   ├── keys/                    ← keybinding definitions
│   │   └── styles/                  ← Lip Gloss style definitions
│   │
│   ├── poller/                      ← poll loop that drives all consumers
│   │   └── poller.go                ← Poller[T]: runs adapters on interval, fans out
│   │
│   ├── hook/                        ← shell hook runner (v1.1)
│   │   └── runner.go                ← executes hook commands on Event[T]
│   │
│   └── subcommand/                  ← subcommand implementations wired by cmd/prsm/main.go
│       ├── tui.go                   ← "prsm" / "prsm tui" — launches internal/tui
│       ├── serve.go                 ← "prsm serve" — future HTTP/gRPC/MCP server (stub)
│       └── version.go               ← "prsm version"
│
├── api/                             ← gRPC/HTTP API layer (added when server consumer arrives)
│   ├── proto/                       ← .proto source files
│   │   └── prsm/v1/
│   │       └── prsm.proto
│   └── gen/                         ← generated Go code (committed; do not hand-edit)
│       └── prsm/v1/
│           ├── prsm.pb.go
│           └── prsm_grpc.pb.go
│
└── docs/
    └── decisions/
        └── ...
```

### Rationale for each directory

**`model/`** — Exported. Contains only normalized types; zero dependencies on any provider client library, Bubble Tea, or config. A library consumer `import "github.com/lstellway/prsm/model"` gets `PullRequest`, `LoadResult[T]`, `ProviderInstance`, and nothing else. No network code, no rendering code, no heavy dependencies.

**`adapter/`** — Exported. The `ProviderAdapter` interface is the contract that all adapters satisfy. Each provider has its own subdirectory. Provider-specific SDK dependencies (`google/go-github`, `gitlab-org/api/client-go`, `go-gitea/go-sdk`) live only inside these packages; nothing outside `adapter/` imports them. A library consumer can import `adapter/github` to construct a GitHub adapter directly.

**`query/`** — Exported. `FilterSpec`, `Predicate[T]`, `SortSpec`, `GroupSpec`. Zero dependency on any adapter or consumer. A library consumer imports this to build their own filter pipelines over `model.PullRequest` values.

**`event/`** — Exported. The event engine and broadcast bus. The `EventStream[T]` type is the library consumer's subscription handle. Zero dependency on adapters or consumers.

**`config/`** — Exported. Shared across all consumers. The TUI loads it; a future HTTP server loads it; a library consumer building on top of prsm can also use it. Depends only on `github.com/BurntSushi/toml` and the standard library.

**`client/`** — Exported. A `Client` struct that assembles the exported layers into a ready-to-use object. This is the primary entry point for library consumers who do not want to wire up adapters, pollers, and event engines manually. `prsm.NewClient(cfg)` → `.PullRequests()` → `<-chan []model.PullRequest`. The `cmd/prsm/main.go` also uses this internally.

**`internal/tui/`** — Not importable externally. All Bubble Tea, Lip Gloss, and Bubbles imports live here. The TUI's `Model`, `Update`, and `View` functions, panel layout, keybindings, and style definitions all live here. The import boundary enforced by Go's compiler ensures Bubble Tea cannot leak into any exported package.

**`internal/poller/`** — Not importable externally. Runs provider adapters on a configurable interval, fans results to the query layer and event engine. The TUI interacts with it via Bubble Tea `tea.Cmd`; the future HTTP server would interact with it via a channel. Keeping it internal avoids committing to its API surface prematurely.

**`internal/hook/`** — Not importable externally (v1.1). Shell hook runner. Depends on `event/` but not on any consumer. Kept internal because hook execution is a daemon concern, not a library concern.

**`internal/subcommand/`** — Not importable externally. Thin wrappers that mount the subcommand tree. Each file imports the relevant internal package (`tui`, `hook`) and wires it up via Cobra. `main.go` imports only `internal/subcommand`.

**`api/proto/` and `api/gen/`** — Placeholder directories for when the gRPC/MCP server consumer arrives. Keeping them under `api/` makes the transport layer's location unambiguous and avoids polluting the module root with generated code.

---

## Package boundary rules

### What is exported (importable by external code)

| Package | Import path | External consumers |
|---|---|---|
| `model` | `github.com/lstellway/prsm/model` | All library consumers; also imported by `adapter/`, `query/`, `event/`, `client/` |
| `adapter` | `github.com/lstellway/prsm/adapter` | Library consumers who want the interface type |
| `adapter/github` | `github.com/lstellway/prsm/adapter/github` | Library consumers using GitHub specifically |
| `adapter/gitlab` | `github.com/lstellway/prsm/adapter/gitlab` | Library consumers using GitLab specifically |
| `adapter/gitea` | `github.com/lstellway/prsm/adapter/gitea` | Library consumers using Gitea/Forgejo specifically |
| `query` | `github.com/lstellway/prsm/query` | Library consumers building filter pipelines |
| `event` | `github.com/lstellway/prsm/event` | Library consumers subscribing to the event stream |
| `config` | `github.com/lstellway/prsm/config` | Library consumers loading prsm config |
| `client` | `github.com/lstellway/prsm/client` | Primary library consumer entry point |

### What is internal (enforced by Go compiler)

| Package | Reason |
|---|---|
| `internal/tui` | Bubble Tea dependency; TUI rendering is not a library concern |
| `internal/poller` | Polling strategy is an implementation detail; API not yet stable |
| `internal/hook` | Shell execution is a daemon concern; not appropriate for library API |
| `internal/subcommand` | CLI wiring; has no meaning outside the binary |

### Import direction rules (must be enforced in CI)

```
model         → (no prsm imports)
adapter/*     → model only
query         → model only
event         → model only
config        → (no prsm imports; stdlib + BurntSushi/toml only)
client        → model, adapter, query, event, config, internal/poller
internal/tui  → model, query, event, config, client, (Bubble Tea)
internal/poller → model, adapter, config
internal/hook → event, model
internal/subcommand → internal/tui, internal/poller, internal/hook, config, client
cmd/prsm      → internal/subcommand only
```

No package may import a package above it in this list. A `go-imports-check` or `depguard` linter rule enforces this in CI.

---

## Single binary vs. multiple binaries

**Recommendation: single binary with subcommands.**

The binary is `prsm`. Subcommands:
- `prsm` (default, no subcommand) or `prsm tui` — launches the TUI
- `prsm serve` — future HTTP/gRPC/MCP server (stub in v1, fully implemented in v1.x)
- `prsm version` — prints build info

**Rationale:**

1. Users install one binary and one binary only. Distribution via Homebrew, `go install`, or prebuilt binaries is simpler.
2. Subcommands share the same config loading path, authentication initialization, and `"me"` resolution logic (ADR-006) without needing an IPC layer between separate binaries.
3. The Go community has converged on single-binary subcommand trees (Cobra or stdlib `flag`). gh, atlas, ollama, flyctl, buf all do this.
4. `prsm tui` vs. just `prsm`: making `tui` the default subcommand (invoked when no subcommand is given) preserves the UX that users who only use the TUI never need to type a subcommand, while `prsm serve` clearly names the server mode. This matches the `lazygit` / `k9s` UX (bare invocation launches the TUI) without preventing future non-TUI modes.

**When to revisit multi-binary:** if the server mode becomes a long-running system daemon that needs separate process lifecycle management (systemd unit, Docker container). At that point, a `prsm-server` binary may be warranted. That decision belongs with the server consumer design, not now.

---

## Library consumer experience

A third party who wants to build on prsm's normalized resource layer — say, a dashboard that shows PR counts per repo — writes:

```go
import (
    "context"
    "fmt"

    "github.com/lstellway/prsm/client"
    "github.com/lstellway/prsm/config"
    "github.com/lstellway/prsm/event"
    "github.com/lstellway/prsm/model"
    "github.com/lstellway/prsm/query"
)

func main() {
    cfg, err := config.LoadFile("")  // "" = use default XDG path
    if err != nil {
        log.Fatal(err)
    }

    c, err := client.New(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    ctx := context.Background()

    // Subscribe to the event stream for specific transitions
    ch := c.Events().Subscribe(ctx,
        event.EventPRReviewRequested,
        event.EventPRApprovalGranted,
    )

    // Snapshot: current pull requests matching a filter
    spec := query.PRFilterSpec{
        Reviewer: "me",
        Draft:    boolPtr(false),
    }
    pred, err := spec.Compile(c.ResolvedIdentities())
    if err != nil {
        log.Fatal(err)
    }

    prs := c.PullRequests().Filter(pred)
    for _, pr := range prs {
        fmt.Printf("%s/%s #%d — %s\n",
            pr.Repo.Owner, pr.Repo.Name, pr.Number, pr.Title)
    }

    // React to events
    for ev := range ch {
        switch ev.Kind {
        case event.EventPRReviewRequested:
            fmt.Printf("Review requested: %s\n", ev.Current.Title)
        case event.EventPRApprovalGranted:
            fmt.Printf("Approved: %s\n", ev.Current.Title)
        }
    }
}
```

Observations:
- No Bubble Tea import anywhere. The consumer's `go.sum` will not contain Bubble Tea.
- `google/go-github` does appear (transitively via `adapter/github`), but only if the config includes a GitHub provider. A consumer targeting only GitLab would not need the GitHub SDK.
- The `client` package is the only entry point needed for most consumers. Direct adapter imports are available for consumers who want to construct adapters programmatically without a config file.
- The import paths are clean and self-describing: `prsm/model`, `prsm/query`, `prsm/event`, `prsm/client`.

### What the consumer does NOT need to import

- `internal/tui` — compiler-enforced
- `internal/poller` — compiler-enforced
- `charm.land/bubbletea/v2` — never appears in exported packages
- `charm.land/lipgloss/v2` — never appears in exported packages
- `charm.land/bubbles/v2` — never appears in exported packages

---

## gRPC / proto placement

When the gRPC or MCP server consumer is built, the recommended layout is:

```
api/
├── proto/
│   └── prsm/v1/
│       └── prsm.proto          ← service definition, message types
├── gen/
│   └── prsm/v1/
│       ├── prsm.pb.go          ← generated (committed)
│       └── prsm_grpc.pb.go     ← generated (committed)
└── buf.yaml                    ← buf configuration for proto linting/generation
```

**Rationale:**
- `api/proto/` is the source of truth for the wire protocol. It is human-authored.
- `api/gen/` is generated code. It is committed to the repository so consumers do not need the Protobuf toolchain to build prsm. `make generate` or `go generate` re-generates it.
- The `api/` prefix scopes both directories as transport-layer concerns, distinct from the resource model (`model/`) and business logic (`query/`, `event/`).
- Generated Go code does NOT import Bubble Tea or any consumer package. It only imports `google.golang.org/protobuf` and `google.golang.org/grpc`. The server handler code that uses the generated types lives in `internal/subcommand/serve.go` or a future `internal/server/` package.
- `buf.yaml` is placed next to `api/proto/` for `buf lint` and `buf generate` to work without additional path configuration.

**MCP server placement:** The MCP server handler is a different transport from gRPC but follows the same principle — it is a consumer. Handler code lives in `internal/server/mcp.go` (or `internal/subcommand/serve.go` if the server modes share a single subcommand). It imports `github.com/modelcontextprotocol/go-sdk/mcp` and the exported prsm packages. No proto files are needed for MCP (it uses JSON-RPC over stdio or HTTP natively).

---

## Open questions

1. **`client/` vs. root package for library entry point.** The pattern `prsm.NewClient(cfg)` (root package) is more idiomatic than `client.NewClient(cfg)` (redundant word). An alternative is to place the `Client` type in the root package (`package prsm`) — then the import is `import "github.com/lstellway/prsm"` and the call is `prsm.NewClient(cfg)`. This is what wishlist and vhs do (library code at the root). The downside: the root package would import all the provider SDK dependencies, making `go mod tidy` on the module pull in all provider clients even for consumers who only use one provider. The `client/` subdirectory avoids this but produces the slightly awkward `client.New(cfg)`. Decision needed before scaffolding.

2. **`adapter/` interface file location.** The `ProviderAdapter` interface can live in `adapter/adapter.go` (same package as the subdirectory parent) or in `model/` (since it's defined in terms of model types). Placing it in `adapter/` keeps the interface co-located with its implementations. Placing it in `model/` avoids an import cycle if `client/` needs to reference both the interface and the model. Recommend `adapter/adapter.go` — the interface depends on `model`, not the reverse.

3. **Poller API exposure timing.** `internal/poller` is kept internal in v1. When the HTTP/gRPC server consumer arrives, it may need to subscribe to the same poll cycle that the TUI uses. At that point, `poller` may need to become an exported package or the `client/` package needs to expose polling control. This is a v1.x decision — defer until the server consumer design is underway.

4. **`go.work` for local development.** If `api/gen/` introduces a separate module in the future (e.g., to allow separate versioning of the proto-generated types), a `go.work` file will be needed for local development. For now, with a single `go.mod`, no workspace file is needed.

5. **Config as exported vs. internal.** Exporting `config/` makes the TOML schema a public API commitment — once an external library consumer builds against `config.Config`, every field rename is a breaking change. An alternative is to keep `config/` internal and require library consumers to construct adapters and client options programmatically (no TOML dependency). This is more composable but removes the convenience of `client.New(cfgFilePath)`. Recommend exporting `config/` with explicit semver discipline: fields are additive; renames are major version changes.

6. **Package name collision: `model` vs. `prsm/model`.** The package name `model` is common and may collide in import blocks with other `model` packages in consumer code. An alternative is `resource` (aligned with ADR-000's "Resource Model" terminology). Consider `resource/` vs. `model/` before scaffolding — the package name is hard to rename after it's in the wild.
