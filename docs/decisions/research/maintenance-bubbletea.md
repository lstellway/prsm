# Bubble Tea v2 — Maintenance Continuity Research

Research conducted: 2026-06-13

---

## Commit & Release Activity

**Repository:** https://github.com/charmbracelet/bubbletea
**Stars as of research date:** ~43,100
**Total commits on main:** 1,867+
**Total releases:** 78 across all versions (v0.x through v2.x)

### v2.x Release Timeline

v2 development began publicly with release candidates in late 2024:

| Version | Date |
|---|---|
| v2.0.0-rc.1 | November 4, 2024 |
| v2.0.0-rc.2 | November 17, 2024 |
| v2.0.0 (GA) | February 24, 2025 |
| v2.0.1 | March 2, 2025 |
| v2.0.2 | March 9, 2025 |
| v2.0.3 | April 13, 2025 |
| v2.0.4 | April 13, 2025 |
| v2.0.5 | April 13, 2025 |
| v2.0.6 | April 16, 2025 |
| v2.0.7 | June 1, 2025 (latest as of research date) |

That is **10 v2.x releases in ~7 months** from first RC to the latest patch. The cadence through spring 2025 was rapid — three patch releases on a single day (April 13) suggest active triage of post-GA issues.

v2.0.7 specifically addressed "a few solid improvements around stability and correctness," including race condition fixes and panic prevention — signals of serious production hardening, not just feature work.

### Gap Analysis

No significant gaps in activity were identified during the v2 development window. The two-month gap between the last RC (November 2024) and GA (February 2025) appears to have been used for final stabilization, not abandonment. CI workflows showed commits from contributors like `bashbunni` and `andreynering` as recently as April 2025.

**v1 status:** v1 is now frozen. The last v1 maintenance was v1.3.10 (published September 17, 2025 on pkg.go.dev — likely a backport or tooling update rather than feature work). There have been no v1 releases after GA of v2.

---

## Maintainer Health

### Core Team

Based on GitHub commit and review activity across 2024–2025, the core maintainers are all Charm employees or closely associated:

| GitHub Handle | Role |
|---|---|
| `meowgorithm` (Christian Rocha) | Co-founder/CEO; highest commit volume on core libraries |
| `aymanbagabas` (Ayman Bagabas) | Code owner on bubbletea and bubbles; most active on v2 renderer work and colorprofile |
| `bashbunni` | Code owner; merged commits to main as recently as December 2024 |
| `maaslalani` | Code owner on bubbles and huh |
| `caarlos0` (Carlos Becker) | Member; active reviewer across Charm repos |

Seven core developers were credited in the v2.0.0 release notes. This is **not a one-person project** — it has a bench of 5–7 active maintainers, all employed by or closely associated with Charm.

### Company Structure

- **Charmbracelet, Inc.** founded 2019, New York City
- Co-founders: **Toby Padilla** and **Christian Rocha** (CEO)
- **~8 full-time employees** as of available data
- Funded: $3M seed (April 2021) + $6M Series A (November 2023, led by Alphabet/Google's Gradient Ventures) = **$10M raised total**
- 14 investors including Alphabet, Gradient Ventures, Firestreak Ventures, Niche Capital

The Google Gradient Ventures investment in late 2023 is a meaningful signal — Gradient focuses specifically on AI and developer tools, and they do not typically fund projects without a plausible path to revenue.

---

## Corporate Backing & Business Model

### Revenue Strategy

Charm's current business model is primarily **open-source / enterprise funnel**:

1. **Open-source libraries** (Bubble Tea, Lip Gloss, Bubbles, Glamour, Wish, etc.) — free, MIT licensed — used as top-of-funnel
2. **Enterprise contact** — their site lists `vt52@charm.land` for enterprise inquiries, with no public pricing; the pitch is their tools are deployed "in over 25,000 applications" including enterprise names
3. **Crush** — their AI terminal coding agent (https://github.com/charmbracelet/crush), released 2025. 25,300 stars. Licensed under FSL-1.1-MIT (Functional Source License) which converts to MIT after 4 years. This is their most obvious commercial play — it connects to paid LLM APIs (Anthropic, OpenAI, etc.) and positions Charm in the AI dev tools space where monetization is more tractable.

**The FSL license on Crush is notable**: Functional Source License is a commercial-first license that prevents commercial competition for the license term, then converts to MIT. This signals Charm is actively pursuing a revenue model, not purely a donations/sponsorship play.

The Charm Cloud service was a prior commercial attempt (hosted key-value/SSH infrastructure), but was **shut down November 29, 2024** and the `charmbracelet/charm` library was **archived March 6, 2025**. This is discussed more in the Ecosystem section.

### Key Risk Signal: Charm Cloud Shutdown

Charm shut down their Charm Cloud service (a hosted SSH/key-value infrastructure product) in late 2024. The repositories (`charmbracelet/charm`, `charmbracelet/skate`) were archived with the note "if you love this project and would like to maintain a fork, please do." This is evidence that Charm **does** deprecate and shut things down when they don't pan out commercially — it is not indefinite open-source stewardship.

However, Bubble Tea is qualitatively different from Charm Cloud: it is the **core framework** that all of Charm's other products (including Crush) are built on. Abandoning Bubble Tea would be self-defeating in a way that abandoning a hosted service was not.

---

## Issue Responsiveness

### Current State

- **Open issues:** 104 as of research date
- **Open PRs:** 75

### Oldest Open Issues (Backlog Age)

The oldest open issues date to **2021**:
- Issue #79: "Node-based rendering" — opened April 5, 2021 (enhancement)
- Issue #162: "Allow both native text selection and mouse wheel scrolling" — opened November 25, 2021 (enhancement)
- Issue #163: "Displaying images" — opened November 29, 2021 (enhancement)
- Issue #169: "Drag and drop" — opened December 12, 2021 (enhancement)

These are all **feature requests / complex enhancements**, not bug reports. The presence of 4-year-old open enhancement issues is consistent with a project that deliberately scopes and triages. It does not indicate neglect — image display and drag-and-drop in a terminal framework are legitimately hard problems.

A misfiled issue (#1360, opened March 2025) was quickly closed, suggesting active moderation.

### Response Quality

The maintainers have engaged substantively in discussions around v2 design, including a notable reversal during the alpha where they changed `Init()` back to its original signature after community pushback — then acknowledged that "experimenting with alpha code means such breaking changes are to be expected." This shows responsive, if sometimes iterative, leadership.

Specific quotes from maintainers on migration complexity: "We don't take API changes lightly and strive to make the upgrade process as simple as possible."

104 open issues against a project of this scale (43k+ stars, 11k+ dependents) is not alarming. For reference, lazygit has ~700+ open issues.

---

## Dependency Health

### Bubble Tea v2's go.mod Dependencies

Direct dependencies in the v2 (main) branch:

| Dependency | Notes |
|---|---|
| `github.com/charmbracelet/colorprofile` | Charm-owned; actively maintained by aymanbagabas |
| `github.com/charmbracelet/ultraviolet` | Charm-owned; new renderer library underpinning the Cursed Renderer |
| `github.com/charmbracelet/x/ansi` | Charm-owned; experimental/extended packages |
| `github.com/charmbracelet/x/exp/golden` | Charm-owned |
| `github.com/charmbracelet/x/term` | Charm-owned; their own x/term replacing golang.org/x/term |
| `github.com/lucasb-eyer/go-colorful` | Third-party; widely used color library in Go ecosystem |
| `github.com/muesli/cancelreader` | Third-party; small, stable, low-churn |
| `golang.org/x/sys` | Go standard extended library; extremely stable |

**Key observation:** Charm has been deliberately **internalizing their dependency stack**. Rather than depending on `golang.org/x/term`, they now use `github.com/charmbracelet/x/term`. Similarly, `colorprofile`, `ultraviolet`, and `x/ansi` are all Charm-maintained packages. This reduces the risk of third-party abandonment cascading into Bubble Tea, but it also concentrates risk in Charm's own organization.

The only non-Charm, non-stdlib-extended dependencies are `go-colorful` (very stable, 1k stars, used across many Go projects) and `cancelreader` (small, stable). Neither is a meaningful abandonment risk.

**The Bubbles v1 branch** (the component library used alongside Bubble Tea) also depends on `github.com/muesli/termenv` v0.16.0 — a third-party terminal environment detection library that is actively maintained.

### The charm.land Vanity Domain Risk

**This is the single most concrete dependency risk for Bubble Tea v2.** The import path changed from `github.com/charmbracelet/bubbletea` to `charm.land/bubbletea/v2`. The `charm.land` domain is now **critical infrastructure for Go module resolution**.

If `charm.land` goes offline or is misconfigured:
- `go get charm.land/bubbletea/v2` fails globally for all new installs
- CI/CD pipelines doing fresh module downloads break
- New versions cannot be fetched

Mitigations that exist:
- Go module proxy (`proxy.golang.org`) caches already-fetched versions, so existing projects with already-resolved modules are partially protected
- The underlying code is still mirrored on GitHub at `github.com/charmbracelet/bubbletea`
- Go's `GONOSUMCHECK` and direct mode can work around a vanity domain outage for sophisticated users

Mitigations that do not exist:
- No stated fallback if `charm.land` expires or goes down
- No indication Charm has committed to keeping `charm.land` running for any specific period

This risk is not unique to Charm — many Go projects use vanity domains (e.g., `gopkg.in`, `k8s.io`, `sigs.k8s.io`). But it is a **new risk introduced in v2** that did not exist when the library was at `github.com/charmbracelet/bubbletea`.

Minimum Go version required for v2: **Go 1.25.0** (per go.mod).

---

## Ecosystem Stickiness & Fork Risk

### Dependent Scale

- **GitHub repository dependents:** 20,676 repositories, 19,638 packages (approximate, per GitHub's network dependents page)
- **pkg.go.dev known importers:** 11,682 packages
- **Self-reported:** "over 25,000 applications" on Charm's own site

These numbers are large enough that the ecosystem creates meaningful inertia. A fork or replacement would need to be adopted by a significant portion of these dependents to remain viable.

### Confirmed Corporate/Enterprise Adoption

The pkg.go.dev page and Charm's own documentation confirm the following organizations use Bubble Tea in open-source tools:
- **Microsoft Azure** — `aztify` (TUI for Azure resource migration)
- **AWS** — `eks-node-viewer` (TUI for Kubernetes node visualization; Azure later forked it as `aks-node-viewer`)
- **CockroachDB** — internal tooling
- **NVIDIA** — referenced on the Charm site
- **MinIO** — S3-compatible storage tooling
- **Ubuntu/Canonical** — referenced on the Charm site
- **Crush (Charmbracelet itself)** — 25,300-star AI coding agent built on Bubble Tea/Lip Gloss

GitHub internally reviewed `charmbracelet/bubbletea` as a Go module dependency (via the `github/gh-aw` internal review process), suggesting GitHub's own developer tooling uses it.

The presence of AWS and Azure maintaining open-source tools built on Bubble Tea is a significant stickiness signal. These organizations do not migrate TUI frameworks lightly — there is real organizational cost to switching.

### Notable Ecosystem Projects

From the `bubbletea` topic on GitHub: `gh-dash` (GitHub PR dashboard, highly popular), `superfile` (file manager), `chezmoi` (dotfile manager), `Tetrigo` (Tetris), and many CLI tools and shells. The `bubbletea`-tagged ecosystem is broad and diverse.

### Fork Risk Assessment

If Charm shut down tomorrow:
1. The code is MIT-licensed — forking is explicitly permitted and legally clean
2. The core maintainer team (5–7 people who deeply understand the codebase) is small enough to reconstitute in a community fork
3. The API is well-understood; the community knows how to extend it
4. There is precedent: when Charm Cloud shut down, they explicitly invited community forks

However:
- The `charm.land` vanity domain would become a critical problem immediately — all module paths would need to be rewritten or a redirect maintained
- The Bubbles component library, Lip Gloss, and Wish would need parallel community governance
- 20k+ dependent repositories would face migration work

The Ollama project (a major, heavily-used LLM serving tool) uses Bubble Tea in its TUI and opened an issue tracking migration to v2 — a signal that large projects are actively tracking and adopting the new version, creating community investment.

---

## v1 → v2 Migration Signal

### What Changed

v2 is a **substantive architectural overhaul**, not a cosmetic bump. The key changes:

1. **Import path migration to vanity domain**: `github.com/charmbracelet/bubbletea` → `charm.land/bubbletea/v2` (and same for Lip Gloss, Bubbles)
2. **`View()` return type changed**: `string` → `tea.View` struct (the single biggest code-touch change)
3. **Key event types refactored**: `tea.KeyMsg` → `tea.KeyPressMsg`/`tea.KeyReleaseMsg`; field names changed (`msg.Type` → `msg.Code`, `msg.Runes` → `msg.Text`, `msg.Alt` → `msg.Mod`)
4. **Mouse event types split**: `tea.MouseMsg` interface replacing the old struct
5. **Terminal feature flags moved**: `tea.WithAltScreen()`, `tea.EnterAltScreen` etc. replaced by fields on the View struct
6. **All three companion libraries must be upgraded together**: Bubble Tea v2 + Bubbles v2 + Lip Gloss v2

### Community Reception

The v2.0.0 release generated **94 reactions** on GitHub, predominantly positive (64 "hooray" responses). The Hacker News thread and Lobste.rs thread were also generally positive, with specific enthusiasm for keyboard event handling improvements.

One vocal dissenter: "I won't be upgrading to v2, I tried migrating a while ago but it has too many breaking changes/issues currently." This individual preferred v1 feature backports. This is a minority position — the overall tone was supportive.

### Migration Experience from the Field

**Ollama** (one of the most-downloaded open-source LLM tools, with millions of users) opened issue #15692 to track their v1→v2 migration. The stated motivation was a concrete bug fix: v2 removes a package-level `init()` that called `lipgloss.HasDarkBackground()`, which caused a **5-second OSC 11 timeout** on every invocation of `ollama ls` in a PTY. The fix is real and pragmatic.

The Ollama migration identified as the key breaking changes: import path rewriting, `lipgloss.AdaptiveColor` → `compat.AdaptiveColor` shim, and `tea.KeyMsg{…}` literal field name updates. These are mechanical changes but require touching many files.

Charm provided a comprehensive upgrade guide (https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md) with before/after code examples for every breaking change. No automated migration tooling was provided (no codemods or sed scripts in the guide itself), though the community suggested LLM-assisted migration.

### What This Signals About Charm's Compatibility Philosophy

Charm's approach to v2 reveals a pattern:
- They are willing to make **significant breaking changes** when justified by architectural improvements (the Cursed Renderer and declarative View model are genuine technical improvements, not aesthetic reshuffling)
- They provide **comprehensive documentation** for migrations but no automation
- They made **one notable reversal** during the alpha (the `Init()` signature was changed then changed back) in response to community feedback, showing they listen but also that the alpha period was genuinely unstable
- **v1 is now frozen**: no stated timeline, no backport policy. For a project starting fresh on v2, this is not a concern; for a project migrating from v1, it means there is no safe "hold" position

The vanity domain change is the most controversial decision: moving from `github.com` to `charm.land` as the canonical import path introduces real infrastructure dependency on Charm's domain management. This is a one-way door — there is no way to offer `github.com` import paths for v2 without a redirect or module proxy.

---

## Summary Assessment

Bubble Tea v2 presents a **high confidence** case for long-term maintenance, with one specific risk requiring attention.

**The positives are substantial.** Charm is a funded company ($10M raised, most recently from Google Gradient Ventures in late 2023) with 8 employees and a team of 5–7 active maintainers who all know the codebase deeply. Bubble Tea v2 shipped GA in February 2025 and has been actively patched — 10 releases in 7 months — with stability and correctness fixes continuing through mid-2025. The ecosystem is enormous (20k+ dependent repositories, confirmed use by AWS, Microsoft Azure, CockroachDB, and others), creating institutional stickiness that makes abandonment economically painful. Charm's newest flagship product, the AI coding agent Crush (25k stars), is built directly on Bubble Tea, meaning Charm's own commercial ambitions are now dependent on maintaining the framework — a strong self-interest alignment. The dependency tree has been deliberately internalized (Charm owns most of their transitive dependencies), reducing third-party abandonment cascade risk.

**The one concrete risk** is the `charm.land` vanity domain introduced in v2. All module paths now depend on Charm controlling this domain in perpetuity. If Charm ceased operations, `charm.land` could go dark, breaking fresh module installs globally. Go's module proxy caches mitigate this for existing deployments but not for new installs or CI environments. This risk is mitigated — but not eliminated — by the Go proxy cache, the MIT license making forks trivial, and the GitHub mirror of the source code. A team adopting Bubble Tea v2 should monitor this risk and consider using `GOPROXY` settings that cache aggressively.

**The secondary risk** is Charm's small team size (~8 people) and evidence that they do shut things down (Charm Cloud was deprecated in November 2024 and archived in March 2025). Bubble Tea is qualitatively different from Charm Cloud in that it is Charm's own development platform — abandoning it would cripple their own products — but the organization's demonstrated willingness to sunset products is worth noting.

**Overall confidence rating: High**, with the `charm.land` domain dependency flagged as a concrete risk to document in any ADR. For a project like prsm that is starting fresh on v2 (no migration cost), the framework is a sound foundation for a long-lived product. The more interesting question is not whether Bubble Tea will be maintained, but whether Charm's commercial bets (Crush, enterprise) prove out — a company with a healthy revenue path is far less likely to sunset its core open-source infrastructure.

---

## Sources

- https://github.com/charmbracelet/bubbletea
- https://github.com/charmbracelet/bubbletea/releases
- https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.0
- https://github.com/charmbracelet/bubbletea/discussions/1374
- https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md
- https://github.com/charmbracelet/bubbletea/blob/main/go.mod
- https://github.com/charmbracelet/bubbletea/network/dependents
- https://pkg.go.dev/github.com/charmbracelet/bubbletea
- https://pkg.go.dev/charm.land/bubbletea/v2
- https://charm.land
- https://github.com/charmbracelet/crush
- https://github.com/charmbracelet/charm (archived)
- https://github.com/aymanbagabas
- https://github.com/meowgorithm
- https://github.com/ollama/ollama/issues/15692
- https://lobste.rs/s/1to8sq/charm_v2_major_releases_for_bubble_tea_lip
- https://news.ycombinator.com/item?id=47138688
- https://pitchbook.com/profiles/company/455143-42 (Charmbracelet funding data)
- https://startupintros.com/orgs/withcharm
- https://deepwiki.com/charmbracelet/colorprofile
- https://deepwiki.com/charmbracelet/bubbletea/1.2-installation-and-dependencies
