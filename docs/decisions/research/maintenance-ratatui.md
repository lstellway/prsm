# Ratatui — Maintenance Continuity Research

Research date: 2026-06-13. Sources: GitHub repo, crates.io/lib.rs, ratatui.rs, blog posts, RustSec advisory.

---

## Commit & Release Activity

**Release history (verified dates):**

| Version | Date | Notes |
|---------|------|-------|
| 0.20.0 | 2023-03-19 | First Ratatui release (forked from tui-rs) |
| 0.20.1 | 2023-03-22 | Patch |
| 0.21.0 | ~2023-05-29 | Added termwiz backend, Calendar widget |
| 0.22.0 | 2023-07-17 | |
| 0.23.0 | 2023-08-28 | |
| 0.24.0 | 2023-10-23 | |
| 0.25.0 | 2023-12-18 | |
| 0.26.0 | 2024-02-02 | Flex layout, Material/Tailwind color palettes |
| 0.27.0 | 2024-06-24 | |
| 0.28.0 | 2024-08-07 | Crossterm 0.28 upgrade |
| 0.29.0 | 2024-10-21 | Sparkline improvements, crossterm multi-version feature flags |
| 0.30.0-beta.0 | 2025-10-31 | |
| 0.30.0 | 2025-12-26 | no_std support, workspace modularization — "biggest release yet" |
| 0.30.1 | 2026-06-05 | Block shadows, filled areas, buffer diff performance |

**Cadence summary:** From v0.20.0 (March 2023) through v0.29.0 (October 2024), releases came approximately every 6–10 weeks — roughly consistent quarterly-to-bimonthly cadence for major versions. The 0.29 → 0.30.0 gap (October 2024 → December 2025) was ~14 months — notably longer, explained by the scope of the 0.30.0 restructuring (workspace modularization, no_std, Rust 2024 edition). A 0.30.0-beta landed October 2025, indicating active work throughout that period.

**Total releases:** 131 total releases documented on GitHub (includes alpha/beta pre-releases; the automated Saturday alpha cadence accounts for many of these). Main crate has 2,250+ commits on the main branch.

**Weekly alpha releases:** The project runs an automated GitHub Actions workflow that publishes an alpha pre-release every Saturday, providing a continuous integration signal even between major version milestones.

**Recent activity:** Issues closed as recently as 2026-06-13 (day of this research). The v0.30.1 bug-fix release on 2026-06-05 responded to trait regressions (`Send`/`Sync` issues on `Block`) reported after 0.30.0, closed within days of filing.

---

## Maintainer Health & Governance

**Current active maintainers** (per `MAINTAINERS.md`, verified 2026-06-13):

- `joshka` (Josh McKinney) — most active committer in recent releases
- `orhun` (Orhun Parmaksız) — public face; JetBrains Rust Developer Advocate; creator of git-cliff, binsider, and many other Rust CLI tools
- `kdheepak` (Dheepak Krishnamurthy)
- `j-g00da`

**Past maintainers** (listed in MAINTAINERS.md as emeritus):

- `fdehau` (original tui-rs author, blessed the handoff)
- `mindoodoo`, `sayanarijit`, `EdJoPaTo`, `Valentin271`

**Employment status:** Orhun is employed full-time as Rust Developer Advocate at JetBrains (as of 2025). This is a paid position with a company that has direct interest in Rust ecosystem health. The other three current maintainers' employment is not publicly documented; they appear to be independent contributors.

**Funding:** Ratatui received ~$20,000+ from Radworks/Radicle via the Drips FOSS funding network. Additional income via GitHub Sponsors and Open Collective. Orhun's 2025 year-end post lists JetBrains, Terminal Trove, and 33 individual sponsors. Funds cover maintenance and promotional costs, not salaries. The OpenCollective is transparent.

**Governance model:** Informal. No written governance document beyond MAINTAINERS.md. Decision-making happens on GitHub issues/PRs and Discord. The maintainers list has rotated over time — emeritus members stepped down without the project stalling. No single point of failure observed in practice.

**Bus factor:** Moderate concern. With 4 named maintainers, the bus factor is not 1, but it is small. `joshka` and `orhun` appear to drive the majority of recent releases. However, the tui-rs → ratatui transition itself demonstrated that the community can re-organize around a successor even if key individuals depart.

**Community infrastructure:** Discord server, Matrix bridge, dedicated forum at forum.ratatui.rs. Enough infrastructure that conversations don't live only in one person's DMs.

---

## tui-rs Abandonment History

**What happened:**

- **2016**: Florian Dehau (`fdehau`) creates tui-rs.
- **August 14, 2022**: Florian opens a GitHub issue titled "Future of tui-rs," stating he cannot find time to continue development.
- **February 2, 2023**: Community creates a Discord server to discuss forking.
- **February 8, 2023**: Florian responds, proposes a plan for transferring ownership.
- **February 14, 2023**: Fork created as `tui-rs-revival`, then renamed to Ratatui.
- **February 18, 2023**: First Ratatui team meeting.
- **March 19, 2023**: Ratatui v0.20.0 published — first release under new stewardship.
- **August 7, 2023**: Original tui-rs repository archived. RustSec advisory RUSTSEC-2023-0049 issued, officially designating `tui` as unmaintained and recommending `ratatui`.
- **August 2023**: Ratatui recognized as the official successor.

**Timeline gap between abandonment signal and successor:** From the August 2022 "future of tui-rs" issue to Ratatui's first release in March 2023 was ~7 months. From that issue to the tui-rs archive was exactly 12 months.

**Key differences from a sudden abandonment:** Florian was responsive, cooperated on the handoff, and gave the community his blessing. This was an orderly succession, not a hostile fork or vacuum scenario.

**Does this history increase or decrease confidence?** It increases it, modestly. The community proved it can organize around a successor crate in under 6 months when motivated. The existence of a recognized fork path (and tooling like RustSec advisories to signal it) makes a future "Ratatui goes dark → community rallies" scenario more plausible. The tui-rs origin story is now part of Ratatui's brand identity; maintainers explicitly cite it as motivation.

---

## Issue Responsiveness

**Open issues:** 145 as of 2026-06-13.

**Oldest open issues:** Several from April 2023 (e.g., #128 "List: Add wrap for ListItem," opened 2023-04-04; #132 "Create a WidgetList," opened 2023-04-12). These are feature requests, not bugs, and some are marked "On Hold" pending design decisions.

**Recent closed issues:** Multiple issues filed in June 2026 related to `Send`/`Sync` trait regressions in v0.30.1 were closed within 1–3 days of filing (#2592, #2593, #2595, #2597 closed 2026-06-08 through 2026-06-13). This demonstrates that regression bugs are being caught and addressed quickly post-release.

**Assessment:** The backlog of ~145 open issues is not alarming given the project's breadth — many are enhancement requests. The responsiveness to regressions and breaking bugs is fast (days, not weeks). Stale feature requests from 2023 suggest some capacity constraints but not neglect.

**PR activity:** 57 open PRs as of 2026-06-13. Recent releases credit 9–15 contributors per release cycle, suggesting consistent external contribution that doesn't rely solely on the core maintainers.

---

## Dependency Health

**Primary backend dependency: crossterm**

- crates.io: `crossterm` has 73.7M total downloads, 15.3M recent downloads
- Latest version: 0.29.0
- Actively maintained; cross-platform (all UNIX + Windows 7+)
- Repository: github.com/crossterm-rs/crossterm

**Ratatui's crossterm version strategy:** Starting in v0.29.0, Ratatui introduced feature flags (`crossterm_0_28`, `crossterm_0_29`) to support multiple major crossterm versions simultaneously. Starting in v0.30.0, the crossterm backend was moved to a separate `ratatui-crossterm` crate. The project commits to supporting at least the two most recent crossterm versions, which buffers against crossterm version bumps causing breakage.

**Other backends:** `termion` (Unix-only) and `termwiz` (WezTerm's library) are now in standalone backend crates and are optional. Neither is a hard dependency.

**Core layout algorithm:** Uses Cassowary (constraint solver). This is a stable, well-understood algorithm implemented in a pure-Rust crate.

**`ratatui-core` split (v0.30.0):** The v0.30.0 workspace modularization creates a stable ABI surface for widget library authors — `ratatui-core` provides primitives without the full dependency tree. This reduces the blast radius of breaking changes for the wider ecosystem going forward.

**Risk of "another maintainer abandons it" scenario:** Low-to-moderate. If the current maintainers stepped down, the 4,200+ dependent crates create a strong economic incentive for the community to re-fork, just as tui-rs users motivated the Ratatui fork. The project has also proactively structured its code (workspace modularization, stable ratatui-core) to reduce the switching cost of such an event.

---

## Ecosystem Stickiness & Fork Risk

**Crate adoption (lib.rs, 2026-06-13):**

- **4,778 crates** use ratatui; **4,353 direct dependencies**
- **31.3M total downloads** on crates.io (per ratatui.rs homepage)
- **4.7M downloads/month** (lib.rs)
- Ranked #9 in the CLI category on lib.rs

**Notable production tools:**

| Tool | Use case | Notable backer |
|------|----------|---------------|
| gitui | Git TUI | |
| bottom (btm) | System monitor | |
| bpftop | eBPF program monitor | Netflix |
| Yazi | Terminal file manager | |
| codex | AI coding agent | OpenAI |
| dua-cli | Disk usage | |
| csvlens | CSV viewer | |
| taskwarrior-tui | Task management | |
| gpg-tui | GPG key manager | |
| binsider | Binary analyzer | orhun |
| television | Fuzzy finder | |

The ratatui.rs homepage lists Netflix, OpenAI, OVH Cloud, Oxide, AWS, Electronic Arts, Vercel, and Hugging Face as organizations trusting Ratatui. (These are self-reported; individual engineers at those companies use it, not necessarily official enterprise commitments.)

**`awesome-ratatui`:** A curated list with 32+ showcased apps, maintained under the ratatui GitHub org.

**Fork risk:** If the current maintainers were to disappear, the massive dependent-crate footprint (4,200+) and the established tui-rs → ratatui playbook make a community-led continuation highly probable. The RustSec advisory mechanism provides an official channel to signal abandonment and redirect users. The project's org-based GitHub ownership (github.com/ratatui) rather than a personal account also reduces single-maintainer lock-in — organization ownership is transferable.

**Competing alternatives:** None with comparable momentum in the Rust ecosystem. `crossterm` alone handles low-level terminal I/O; Ratatui sits above it. No serious challenger has materialized since 2023.

---

## Summary Assessment

Ratatui presents **high confidence** for long-term maintenance continuity, with one moderate caveat.

The evidence is strongly positive across most dimensions: the project has shipped 11+ major versions since February 2023 at a consistent cadence, has 4 active named maintainers (one of whom is employed at JetBrains with an institutional stake in the Rust ecosystem), a weekly automated alpha cadence, and a 4,200+ crate dependent ecosystem that creates powerful community incentive to sustain the project. Issue regressions are addressed within days. The 0.30.0 release (December 2025) — the most architecturally ambitious yet — demonstrates the project is still in an active growth phase, not maintenance mode. The tui-rs abandonment history is a net positive signal: it proves the Rust community can successfully execute an orderly succession, and that history is now baked into Ratatui's identity and governance intuitions.

The moderate caveat is bus factor: 4 named maintainers with `joshka` and `orhun` carrying the most visible load means the project is not immune to key-person risk. But the same was true of tui-rs, and the community response there took under 6 months. For a tool like prsm — which will depend on Ratatui but not modify its internals — the practical risk is low: even a multi-month disruption would not affect a shipped binary, and any successor crate would almost certainly maintain API compatibility by design (as Ratatui did with tui-rs). **Confidence rating: High.**

---

## Sources

- https://github.com/ratatui/ratatui
- https://github.com/ratatui/ratatui/blob/main/MAINTAINERS.md
- https://github.com/ratatui/ratatui/blob/main/RELEASE.md
- https://lib.rs/crates/ratatui
- https://crates.io/crates/ratatui
- https://ratatui.rs/
- https://ratatui.rs/highlights/
- https://ratatui.rs/showcase/apps/
- https://rustsec.org/advisories/RUSTSEC-2023-0049.html
- https://blog.orhun.dev/open-source-funding-with-ratatui/
- https://blog.orhun.dev/ratatui-0-23-0/
- https://blog.orhun.dev/2025-wrapped/
- https://terminaltrove.com/blog/terminal-trove-talks-with-orhun-parmaksiz-ratatui/
- https://github.com/orhun/tui-rs-revival
- https://deepwiki.com/ratatui/ratatui
- https://newreleases.io/project/github/ratatui/ratatui/release/ratatui-v0.30.0
- Release pages: v0.22.0, v0.23.0, v0.24.0, v0.25.0, v0.26.0, v0.27.0, v0.28.0, v0.29.0, v0.30.0, v0.30.1 on github.com/ratatui/ratatui/releases
