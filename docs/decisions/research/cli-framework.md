# CLI Framework Evaluation

_Evaluated: 2026-06-13. Frameworks: Cobra v1.10.2, urfave/cli v3 (v3.9.1), kong v1.15.0, stdlib flag + manual dispatch._

---

## Key Constraints

1. **Internal-only**: The CLI framework must not appear in any exported package. It is confined to `internal/subcommand/` and `cmd/prsm/`. Library consumers of `github.com/lstellway/prsm/model` or `github.com/lstellway/prsm/adapter/github` must not transitively pull in the framework.
2. **Three subcommands**: `tui` (default), `serve` (stub), `version`. Bare `prsm` delegates to `tui`.
3. **Bubble Tea v2 TUI**: The primary subcommand takes over the terminal. The framework must hand off stdin/stdout cleanly without interference.
4. **Hard to change later**: This decision is made before STE-54 scaffold; changing it post-v1 would require touching `cmd/prsm/` and `internal/subcommand/` only — but framework idioms propagate through those layers.
5. **Distributed as library**: Import-graph cleanliness matters; consumers must not inherit CLI concerns.
6. **Future extensibility**: `serve` (HTTP/gRPC/MCP) will grow. The framework needs to handle flag-rich subcommands without pain.

---

## Comparison Table

| Dimension | Cobra v1.10.2 | urfave/cli v3.9.1 | kong v1.15.0 | stdlib flag |
|---|---|---|---|---|
| **Stars** | 44,100 | 24,100 | 3,100 | n/a (stdlib) |
| **Last release** | Dec 3, 2025 | Jun 10, 2026 | Apr 1, 2026 | Go 1.26.4 (current) |
| **Open issues** | 245 | 52 | 31 | n/a |
| **Active maintainers** | ~3 core + 327 contributors; maintainer bandwidth is stretched | community-run volunteers; v2 deprecated, v3 actively developed | 1 primary (alecthomas); small but focused | Google / Go team |
| **Go min version** | 1.15 | 1.22 | 1.20 | 1.0 (FlagSet since 1.0) |
| **License** | Apache 2.0 | MIT | MIT | BSD (part of Go stdlib) |
| **Direct deps** | 4 (pflag, mousetrap, go-md2man, go.yaml.in/yaml) | 1 (testify, test-only; zero runtime deps) | 2 (alecthomas/assert + repr, both dev/test only) | 0 |
| **Binary size impact** | ~3.4 KB stripped (gschauer benchmark) | ~3.7 KB stripped | ~3.4 KB stripped | ~0 KB (already in binary) |
| **Shell completions** | Built-in: bash, zsh, fish, PowerShell | Built-in: bash, zsh, fish, PowerShell | External: kongplete (third-party) | None; must hand-write |
| **TUI compatible** | Yes — Cobra does not touch stdin/stdout; Bubble Tea integrates cleanly | Yes — no stdin interference; context-based dispatch is clean | Yes — struct-based; no stdin concern | Yes — trivially |
| **Flag groups** | Yes: RequiredTogether, OneRequired, MutuallyExclusive (v1.5+) | Yes: mutually exclusive, required flags, categories | Yes: via struct tags (`required`, `xor`, `and`) | No; must implement manually |
| **Persistent flags** | Yes (inherited down command tree) | Yes (`PersistentFlagsFunc`, inherited) | Yes (embed parent struct) | No; must thread manually |
| **Import graph risk** | Low (confined to `cmd/` and `internal/`) | Low (zero runtime deps; even cleaner) | Low (minimal deps) | Zero |
| **Breaking change history** | Stable since v1.0 (2015); v1.9→v1.10: pflag rename (minor, patched in v1.10.1) | Major: v3 removed `cli.App`, `cli.Context`, `flag.FlagSet` dep; requires migration from v2; v3 itself has been stable | One breaking change at v1.0.0 (#436); otherwise stable | Stable since Go 1.0 |

---

## Framework Profiles

### Cobra

**Module**: `github.com/spf13/cobra` | **License**: Apache 2.0 | **Go min**: 1.15

**Adoption**: 44,100 GitHub stars, 195,884 imported packages, 53,600+ dependent repositories. Users include Kubernetes (`kubectl`), GitHub CLI (`gh`), Hugo, Helm, etcd, Istio, Packer, Terraform CLI wrapper tools, and hundreds of thousands of others. It is the de facto standard Go CLI framework.

**Maintenance**: 3 named core maintainers (Steve Francia, Eric Paris, Marc Khouzam), 327 total contributors. Steve Francia has publicly noted the maintenance burden is high (245 open issues, 118 open PRs as of his post). Despite that, releases have shipped consistently: v1.10.2 (Dec 2025), pre-releases into Apr 2026.

**API surface**: Imperative, object-based. Commands are `*cobra.Command` structs with `Use`, `Short`, `Long`, and `Run`/`RunE` fields. Subcommands attach via `AddCommand`. Flags live on the command's `FlagSet`, accessed via `.Flags()` or `.PersistentFlags()`.

```go
// cmd/prsm/main.go
var rootCmd = &cobra.Command{
    Use:   "prsm",
    Short: "PR review inbox",
    RunE: func(cmd *cobra.Command, args []string) error {
        return tuiCmd.RunE(cmd, args) // delegate to tui
    },
}

var tuiCmd = &cobra.Command{
    Use:   "tui",
    Short: "Launch the TUI",
    RunE: func(cmd *cobra.Command, args []string) error {
        return tui.Run()
    },
}

var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start the MCP/HTTP server (stub)",
    RunE: func(cmd *cobra.Command, args []string) error {
        return serve.Run()
    },
}

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println(build.Version)
    },
}

func init() {
    rootCmd.AddCommand(tuiCmd, serveCmd, versionCmd)
}
```

**Binary size**: ~3.4 KB stripped (gschauer/go-cli-comparison benchmark). Four direct deps, all lightweight. After v1.10.2, removed the deprecated `gopkg.in/yaml.v3` in favour of `go.yaml.in/yaml/v3`.

**TUI compatibility**: Cobra parses flags and runs `RunE`; it has no opinion on what happens inside `RunE`. Bubble Tea is instantiated inside the run function and takes over the terminal cleanly. The recommended pattern (confirmed by the Bubble Tea community) is to use Cobra's pflag to capture arguments, then pass them to `tea.NewProgram(model)`. No known compatibility issues.

**Shell completions**: Built-in generation for bash, zsh, fish, PowerShell. Single method call per command tree.

**Breaking changes**: API has been stable since v1.0 (2015). The most disruptive change in recent history was a pflag rename in v1.10.0 (patched in v1.10.1). No major migrations required.

---

### urfave/cli v3

**Module**: `github.com/urfave/cli/v3` | **License**: MIT | **Go min**: 1.22

**Adoption**: 24,100 stars, 2,915 imported packages on pkg.go.dev (v3 specifically; v2 has 23,065 — the ecosystem is still migrating). v2 is deprecated; all new development is on v3.

**Maintenance**: Community-run by volunteers. 52 open issues (low — sign of active triage). 169 total releases. Latest: v3.9.1 (Jun 10, 2026), showing active cadence.

**API surface**: v3 unified `cli.App` and `cli.Command` into a single `*cli.Command`. All handlers take `(context.Context, *cli.Command) error`. No external runtime dependencies — zero-dep at runtime (testify is test-only).

```go
// cmd/prsm/main.go
app := &cli.Command{
    Name:  "prsm",
    Usage: "PR review inbox",
    Action: func(ctx context.Context, cmd *cli.Command) error {
        return tui.Run(ctx)
    },
    Commands: []*cli.Command{
        {
            Name:   "tui",
            Usage:  "Launch the TUI",
            Action: func(ctx context.Context, cmd *cli.Command) error { return tui.Run(ctx) },
        },
        {
            Name:   "serve",
            Usage:  "Start the MCP/HTTP server (stub)",
            Action: func(ctx context.Context, cmd *cli.Command) error { return serve.Run(ctx) },
        },
        {
            Name:  "version",
            Usage: "Print version",
            Action: func(ctx context.Context, cmd *cli.Command) error {
                fmt.Println(build.Version)
                return nil
            },
        },
    },
}
app.Run(context.Background(), os.Args)
```

**Binary size**: ~3.7 KB stripped. Slightly larger than Cobra in benchmarks, but the zero-runtime-dep characteristic means no transitive bloat.

**TUI compatibility**: The `context.Context`-based dispatch is clean. No terminal manipulation by the framework itself.

**Shell completions**: Built-in for bash, zsh, fish, PowerShell. Dynamic completion via `ShellComplete` field on each command.

**Breaking changes**: v3 is a significant break from v2: `cli.App` removed, `cli.Context` removed, `flag.FlagSet` dependency removed, `IntFlag` uses `int` not `int64`. v3 itself has been stable since stabilisation. Migration from v2 is non-trivial for large codebases. Since prsm would start on v3, this is not a concern.

**Go 1.22 minimum**: This is the highest floor of any framework evaluated. prsm is a new project likely targeting Go 1.22+, so this is acceptable — but worth noting.

---

### kong

**Module**: `github.com/alecthomas/kong` | **License**: MIT | **Go min**: 1.20

**Adoption**: 3,100 stars, 3,079 known importers on pkg.go.dev. Primarily used by smaller tooling projects; no dominant household-name CLI users found. Smaller but focused community.

**Maintenance**: Single primary maintainer (Alec Thomas). 31 open issues (very low). Reached v1.0.0 recently, indicating considered API stability. Latest: v1.15.0 (Apr 1, 2026). Active: releases in Dec 2024, Jan 2025, Apr 2026.

**API surface**: Struct-tag declarative. The entire CLI grammar is a Go struct; kong reflects over it to build the command tree and flag set. Leaf commands implement `Run(...) error`.

```go
// internal/subcommand/cli.go
type CLI struct {
    TUI     TUICmd     `cmd:"" help:"Launch the TUI" default:"withargs"`
    Serve   ServeCmd   `cmd:"" help:"Start the MCP/HTTP server (stub)"`
    Version VersionCmd `cmd:"" help:"Print version"`
}

type TUICmd struct{}
func (t *TUICmd) Run() error { return tui.Run() }

type ServeCmd struct{}
func (s *ServeCmd) Run() error { return serve.Run() }

type VersionCmd struct{}
func (v *VersionCmd) Run() error { fmt.Println(build.Version); return nil }

// cmd/prsm/main.go
func main() {
    var cli CLI
    ctx := kong.Parse(&cli)
    err := ctx.Run()
    ctx.FatalIfErrorf(err)
}
```

**Binary size**: ~3.4 KB stripped — comparable to Cobra.

**TUI compatibility**: Kong has no stdin/terminal concerns; it reflects over structs, then calls `Run()`. Bubble Tea integration is identical to Cobra: instantiate `tea.NewProgram` inside `Run()`.

**Shell completions**: Not built-in. Requires the third-party `kongplete` package (`github.com/willabides/kongplete`) or carapace-spec-kong integration. This is an extra dependency and integration step.

**Breaking changes**: One breaking change at v1.0.0 (#436, noted as affecting few users). Otherwise stable across history.

**Tradeoff**: The struct-tag model is elegant for large, flag-rich CLIs where the grammar maps naturally to nested structs. For prsm's three lean subcommands it adds structural overhead (a top-level `CLI` struct, per-command structs) without proportional benefit. The missing built-in shell completion is a notable gap.

---

### stdlib flag + manual dispatch

**Module**: `flag` (Go standard library) | **License**: BSD | **Go min**: 1.0

**Adoption**: Used by the Go toolchain itself (`go build`, `go run`, etc.) as the most prominent production example. Not commonly used for multi-subcommand CLIs in the broader Go ecosystem; major production CLIs (kubectl, docker, hugo, gh) use Cobra.

**Maintenance**: The Go team (Google). Zero maintenance burden on adopters. Stable since Go 1.0.

**API surface**: `flag.FlagSet` per subcommand. Manual dispatch via `switch os.Args[1]`. No help text generation, no shell completions, no persistent flags, no flag groups. All of these must be hand-rolled.

```go
// cmd/prsm/main.go
func main() {
    if len(os.Args) < 2 {
        runTUI()
        return
    }
    switch os.Args[1] {
    case "tui":
        runTUI()
    case "serve":
        serveFS := flag.NewFlagSet("serve", flag.ExitOnError)
        // serve-specific flags here
        serveFS.Parse(os.Args[2:])
        runServe()
    case "version":
        fmt.Println(build.Version)
    default:
        fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
        os.Exit(1)
    }
}
```

**Binary size**: Zero CLI framework overhead — the flag package is already linked by any Go binary that uses it.

**TUI compatibility**: Trivially compatible. The dispatch function calls into Bubble Tea directly.

**Shell completions**: None. Must write completion scripts manually for each shell.

**Breaking changes**: None. The `flag` package API has been stable since Go 1.0.

**Tradeoffs vs frameworks**:
- Pro: Zero external dependencies, total control, no framework inversion-of-control, smallest possible binary.
- Con: No persistent flags (must pass global state manually), no flag groups, no auto-generated help (must write `Usage` functions), no shell completions, no aliases, no intelligent suggestions. For three simple subcommands with flat flags, this is manageable — but `serve` is expected to grow with `--host`, `--port`, `--mcp-transport`, auth flags, etc. Each new flag requires manual wiring.
- The Go toolchain is often cited as a stdlib-flag success story, but it has a dedicated team and a narrow, well-defined command grammar. The `serve` subcommand's growth trajectory makes stdlib increasingly expensive over time.

---

## prsm-Specific Analysis

### internal/subcommand isolation

All four options satisfy the architecture constraint equally well. The CLI framework is confined to `cmd/prsm/` and `internal/subcommand/`. Since `model/`, `adapter/`, `config/`, and `query/` do not import from `internal/subcommand/` or `cmd/`, no library consumer transitively gets the CLI framework regardless of which one is chosen. This constraint is enforced by Go's `internal/` package boundary, not by framework choice.

### Bubble Tea v2 compatibility

All four options are compatible. Bubble Tea v2 takes over the terminal inside its `tea.NewProgram(model).Run()` call. The CLI framework's job ends when it dispatches to the `tui.Run()` function. None of the frameworks evaluated touch stdin, stdout, or raw mode in a way that conflicts with Bubble Tea. The community-documented pattern (capture flags via framework, then instantiate Bubble Tea model) works identically across all four.

### The 3-subcommand structure

prsm's current command grammar is deliberately minimal: three subcommands, shallow flag depth, a default-to-TUI behaviour. This is in Cobra/urfave's wheelhouse. It is also trivially implementable in stdlib. Kong adds struct-wrapping overhead that only pays off at larger scale.

The default-to-TUI behaviour (bare `prsm` → launch TUI) is handled differently per framework:
- **Cobra**: Set `RunE` on the root command to call into `tui.Run()`.
- **urfave/cli v3**: Set `Action` on the root command.
- **kong**: Tag the `TUI` struct field with `default:"withargs"`.
- **stdlib**: Check `len(os.Args) < 2` before dispatch.

All are straightforward.

### Future `serve` subcommand growth

`serve` is a stub today but will grow into an HTTP/gRPC/MCP server with flags for host, port, transport, TLS, auth, etc. This is where framework choice has the most impact:
- **Cobra** and **urfave/cli v3** both support persistent flags, flag groups, and mutually exclusive flags — useful for `--tls-cert`/`--tls-key` combos, mutually exclusive transport flags, etc.
- **kong** handles this elegantly via struct embedding — a `ServeCmd` struct grows naturally.
- **stdlib** requires manual wiring for every new flag; no persistent flags without explicit threading.

### Library consumer implications

All options confine the framework to `cmd/` and `internal/subcommand/`. However, Cobra pulls in `pflag`, `mousetrap`, and `go.yaml.in/yaml` — four total direct dependencies — whereas urfave/cli v3 has zero runtime dependencies and kong has two (test-only in practice). For prsm-as-library, the binary overhead is irrelevant (library consumers don't build the CLI binary); but for the CLI binary distribution, smaller dep graphs are easier to audit and supply-chain-scan.

---

## Recommendation

**Use Cobra.**

Cobra is already stubbed in, has the broadest adoption in the Go ecosystem (44K stars, 195K+ importers, kubectl/gh/Hugo/Helm), ships with built-in shell completions, has a stable 10-year API history with no major migrations required, integrates cleanly with Bubble Tea, and satisfies all prsm-specific constraints. The marginal advantages of urfave/cli v3 (zero runtime deps, context-first API) and kong (struct-tag elegance) do not outweigh Cobra's ecosystem depth, documentation density, and the fact that it is already present in the codebase.

### Top 2 tradeoffs

1. **Maintainer bandwidth**: Steve Francia has publicly acknowledged the maintenance backlog is heavy (245 open issues, 118 open PRs). Cobra is so widely used that critical bugs get fixed — but response time for edge cases or minor issues can be slow. This is a known open-source sustainability concern, not a blocking issue for prsm.
2. **pflag dependency**: Cobra's flag layer is `pflag` rather than the stdlib `flag` package. This is the right call for Cobra's feature set (POSIX short+long flags, flag groups), but it means one extra dependency that library consumers of the CLI binary inherit. Since the framework is internal-only, this does not affect prsm's exported packages.

---

## Open Questions

1. **Go version floor**: urfave/cli v3 requires Go 1.22. If prsm targets Go 1.20 or earlier, urfave/cli v3 is ruled out. If the project targets Go 1.22+ (reasonable for a new project in 2026), urfave/cli v3 becomes more competitive and its zero-runtime-dep profile may tip the balance.
2. **Shell completion importance**: If shell completion is a v1 deliverable (not just planned), Cobra and urfave/cli v3 ship it without extra deps; kong requires kongplete. This gap matters if completions are needed at launch.
3. **API aesthetics**: The urfave/cli v3 `context.Context`-first signature (`func(context.Context, *cli.Command) error`) is more idiomatic for modern Go and integrates naturally with any future context-propagation to the HTTP/MCP server. If the team prefers that style, urfave/cli v3 is a defensible alternative — especially given its zero runtime deps.
4. **Maintainer bus factor**: kong's single-maintainer model is a risk if prsm is a long-lived project. Cobra's diverse contributor base (327 contributors, 3 named core maintainers) is more resilient despite the known bandwidth strain.

---

_Sources consulted: github.com/spf13/cobra, github.com/urfave/cli, github.com/alecthomas/kong, pkg.go.dev, cli.urfave.org, libraries.io, gschauer/go-cli-comparison, charmbracelet/bubbletea discussion #940, spf13.com/p/the-maintainers-dilemma._
