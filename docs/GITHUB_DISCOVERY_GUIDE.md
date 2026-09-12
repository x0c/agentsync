# GitHub discovery and first-use guide

## Product intent

The user selected agentsync for GitHub discovery and README improvements on 2026-09-09, and for need-first GitHub search SEO on 2026-09-12. Present its practical value to people who copy Cursor rules into Claude Code by hand and want one source for skills and MCP: maintain global instructions, complete skill folders, and MCP configuration together. Prioritize qualified visitors and successful first use; do not promise stars or claim that SEO alone establishes adoption. Do not rename the repository.

## Evidence and positioning

The GitHub API snapshot collected on 2026-09-09 returned 4 views / 1 unique visitor, 22 clones / 18 unique cloners, no referrers, and 1 star belonging to the owner. The returned traffic buckets end on 2026-09-07; this is a rolling API window, not a complete calendar-month measurement. Clone counts do not establish human adoption. The About description omitted MCP and the topic list was empty. Installation followed the full path catalog in both READMEs.

Lead with a global, machine-local workflow and a standalone binary. Keep the full path catalog available after installation and first use. Explain that support varies by feature and that successfully writing a file does not guarantee every assistant loads it in every workspace. Preserve the Cursor caveat and backup semantics.

## 2026-09-12 need-first search (default Repositories Best Match)

Measure with `gh search repos` / `search/repositories` and **no** `--sort`. Page to ~300 when the first 100 miss. Immediate retest after editing About is not a ranking failure.

Frozen queries (need → query). No product-internal jargon in the query.

| Need | Query | Pool | `x0c/agentsync` | Top 5 (wording to learn) |
| --- | --- | ---: | --- | --- |
| Copying Cursor rules into Claude by hand | `sync claude cursor rules` | 77 | **#60** | `hcastro/cursor2claude`, `PanisHandsome/ai-rules-sync`, `yelmuratoff/agent_sync`, `dhruv-anand-aintech/agent-rules-sync`, `lbb00/ai-rules-sync` |
| Skills written for one agent missing in another | `sync claude code skills` | 779 | not in top 300 | `xingkongliang/skills-manager`, `runkids/skillshare`, `EtienneLescot/n8n-as-code` (off-topic), `caliber-ai-org/ai-setup`, `knoxgraeme/skillfish` |
| Same copy-paste, more oral | `copy cursor rules claude` | 15 | not in 15 | persona/rule-pack repos, not sync CLIs |
| MCP set up in one tool, missing in Claude | `sync mcp config claude` | 73 | **#68** | `caliber-ai-org/ai-setup`, `slash9494/ai-config-sync-manager`, `nicepkg/vsync`, `M-Pineapple/msty-admin-mcp` (off-topic), `spxrogers/agentsync` |
| Want skills available in Cursor | `share agent skills cursor` | 142 | not in 142 | `rohitg00/skillkit`, skill registries, `VintLin/skill-flow` |
| Keep Cursor and Claude Code aligned | `sync cursor claude code` | 315 | **#224** | `colbymchenry/codegraph` (off-topic), `xingkongliang/skills-manager`, `caliber-ai-org/ai-setup` |

Name-collision warning: `PanisHandsome/ai-rules-sync` titles its README `# agentsync`, ships npm `@panishandsome/agentsync`, and shows a live playground screenshot. `spxrogers/agentsync` is another Go CLI with homepage `agentsync.cc`. Learn their **structure**. Do **not** copy their product name, playground image, or npm download badges into this repo, and do not claim we convert project `.cursor/rules/*.mdc` into `CLAUDE.md` (that is `cursor2claude`).

### Wording adopted vs omitted

| Winner pattern | Adopt | Omit |
| --- | --- | --- |
| About first sentence is almost the query (`sync` + `Claude Code` + `Cursor` + `rules` / `skills` / `MCP`) | Yes — About and README H1 subtitle | Do not rename `x0c/agentsync` |
| `skills` and `cursorrules` topics (skills-manager / Caliber) | Yes — add `skills`, `claude`, `cursorrules`, `mcp-server` | Unrelated trending tags |
| Pain-first README line: stop copy-pasting Cursor → Claude | Yes | Their feature matrices, Node/npm install as if we were npm |
| Centered logo, demo GIF, playground screenshot, npm download / Trendshift badges | Type noted | No stolen playground PNG; no fake npm counts; CLI has no App Icon requirement |
| Fold long path catalog; install before the catalog | Keep (already in README) | — |

### About before / after this round

| Field | Before | After |
| --- | --- | --- |
| Description | Sync AI coding-agent rules, skills, and MCP configs across Claude Code, Codex, Cursor, Gemini CLI, and more. One source of truth, one CLI. | Sync Claude Code and Cursor rules, skills, and MCP configs from one source of truth. One CLI for Codex, Gemini CLI, and other AI coding agents. |
| Topics | `agent-skills`, `agents-md`, `ai-agents`, `ai-coding`, `claude-code`, `cli`, `codex`, `configuration-management`, `cursor`, `developer-tools`, `dotfiles`, `gemini-cli`, `golang`, `mcp` | `agent-skills`, `agents-md`, `ai-agents`, `claude`, `claude-code`, `cli`, `codex`, `cursor`, `cursorrules`, `developer-tools`, `gemini-cli`, `golang`, `mcp`, `mcp-server`, `skills` |

Pushing git does not update About or Topics. Re-run the same six queries after GitHub has reindexed (hours, not minutes). Ranking movement is not claimed in this session.

## Reference review

Shallow clones or GitHub first screens of the following repositories were read before editing the README / About:

| Reference | Adopt | Do not copy |
| --- | --- | --- |
| [Ruler](https://github.com/intellectronica/ruler) | Pain-first explanation, visible proof, safety FAQ | Animation, wording, project rule-generation behavior |
| [Rulesync](https://github.com/dyoshikawa/rulesync) | Early installation and minimal first-use flow, link deeper reference material | Its broader feature claims, package channels, or supported-tool matrix |
| [cursor2claude](https://github.com/hcastro/cursor2claude) | Description that is the user query; install immediately | Project `.mdc` → `CLAUDE.md` conversion claim |
| [PanisHandsome/ai-rules-sync](https://github.com/PanisHandsome/ai-rules-sync) | “One source of truth” + convert/sync language in About | README title `agentsync`, npm badges, playground screenshot, their npm command name |
| [vsync](https://github.com/nicepkg/vsync) | “Sync MCP servers, Skills … across Claude Code, Cursor” | Broader agents/commands product claims, centered marketing badges we cannot verify |
| [skills-manager](https://github.com/xingkongliang/skills-manager) / [skillshare](https://github.com/runkids/skillshare) | Put `skills` next to Claude Code and Cursor in one sentence | Desktop screenshots, Trendshift, skill-marketplace positioning |

[GitHub topic documentation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/classifying-your-repository-with-topics) supports relevant topics as a discovery surface. [Repository search documentation](https://docs.github.com/en/search-github/searching-on-github/searching-for-repositories) documents that **default** repository search scans name, description, and topics — not README. Neither source establishes a guaranteed ranking weight or star conversion rate.

## Maintenance and measurement

- Keep English and Chinese README structure, commands, platform limits, and safety statements aligned.
- Keep About in the user's search words: Claude Code, Cursor, rules, skills, MCP configs, one source of truth. Use relevant topics (`skills`, `cursorrules`, `mcp-server`, `claude`), not unrelated trending tags.
- Never retitle this README as a different product's `agentsync` playground, and never paste their screenshots or npm download badges.
- Use macOS Homebrew cask instructions only for macOS. Published archives cover macOS and Linux on amd64/arm64, and Windows on amd64; do not imply Windows ARM has a native release.
- Verify release asset names and checksums before changing download instructions. New installation mechanisms need actual platform verification.
- Recheck the frozen Best Match queries after index delay; also recheck views, unique visitors, referrers, and external stargazers after about 14 days. Keep overlapping windows separate; do not call views divided by total stars a conversion rate.
- Public announcements and directory submissions are a separate outreach task. Prepare them from verified capabilities; publishing messages requires user authorization.

## Verification scope

The v0.11.0 Linux amd64 release archive was downloaded and matched against its published SHA-256 checksum. Its binary completed preview, apply, and recheck in an isolated home with distinct Claude/Codex instructions; both instruction texts survived and both aliases resolved to the canonical source. English/Chinese command blocks match, relative README links resolve, and GitHub's Markdown API renders the tables and folded path reference. Existing macOS/Windows releases and the Homebrew cask were inspected; installation on those operating systems was not rerun in this documentation task.

The 2026-09-12 storefront pass verified: Best Match ranks above; GitHub About description and topics via API; README English/Chinese first screens and FAQ disambiguation. It did not re-run the isolated binary install, and it does not claim improved rank until a later reindex measurement.
