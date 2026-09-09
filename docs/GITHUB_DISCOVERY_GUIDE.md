# GitHub discovery and first-use guide

## Product intent

The user selected agentsync for GitHub discovery and README improvements on 2026-09-09. Present its practical value to people who switch coding assistants: maintain global instructions, complete skill folders, and MCP configuration together. Prioritize qualified visitors and successful first use; do not promise stars or claim that SEO alone establishes adoption.

## Evidence and positioning

The GitHub API snapshot collected on 2026-09-09 returned 4 views / 1 unique visitor, 22 clones / 18 unique cloners, no referrers, and 1 star belonging to the owner. The returned traffic buckets end on 2026-09-07; this is a rolling API window, not a complete calendar-month measurement. Clone counts do not establish human adoption. The About description omitted MCP and the topic list was empty. Installation followed the full path catalog in both READMEs.

Lead with a global, machine-local workflow and a standalone binary. Keep the full path catalog available after installation and first use. Explain that support varies by feature and that successfully writing a file does not guarantee every assistant loads it in every workspace. Preserve the Cursor caveat and backup semantics.

## Reference review

Shallow clones of the following repositories were read before editing the README:

| Reference | Adopt | Do not copy |
| --- | --- | --- |
| [Ruler](https://github.com/intellectronica/ruler) | Pain-first explanation, visible proof, safety FAQ | Animation, wording, project rule-generation behavior |
| [Rulesync](https://github.com/dyoshikawa/rulesync) | Early installation and minimal first-use flow, link deeper reference material | Its broader feature claims, package channels, or supported-tool matrix |

[GitHub topic documentation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/classifying-your-repository-with-topics) supports relevant topics as a discovery surface. [Repository search documentation](https://docs.github.com/en/search-github/searching-on-github/searching-for-repositories) documents searching names, descriptions, topics, and READMEs. Neither source establishes a guaranteed ranking weight or star conversion rate.

## Maintenance and measurement

- Keep English and Chinese README structure, commands, platform limits, and safety statements aligned.
- Keep About specific: syncing AI coding-agent rules, skills, and MCP across named mainstream tools. Use relevant topics, not unrelated trending tags.
- Use macOS Homebrew cask instructions only for macOS. Published archives cover macOS and Linux on amd64/arm64, and Windows on amd64; do not imply Windows ARM has a native release.
- Verify release asset names and checksums before changing download instructions. New installation mechanisms need actual platform verification.
- Recheck views, unique visitors, referrers, and external stargazers after about 14 days. Keep overlapping windows separate; do not call views divided by total stars a conversion rate.
- Public announcements and directory submissions are a separate outreach task. Prepare them from verified capabilities; publishing messages requires user authorization.

## Verification scope

The v0.11.0 Linux amd64 release archive was downloaded and matched against its published SHA-256 checksum. Its binary completed preview, apply, and recheck in an isolated home with distinct Claude/Codex instructions; both instruction texts survived and both aliases resolved to the canonical source. English/Chinese command blocks match, relative README links resolve, and GitHub's Markdown API renders the tables and folded path reference. Existing macOS/Windows releases and the Homebrew cask were inspected; installation on those operating systems was not rerun in this documentation task.

Publish only the README/discovery documentation changes from this task. Pre-existing unverified source changes and their accompanying documentation remain in the working tree; do not tag a binary release from that state. The inherited root navigation contains an existing `../_standards/go.md` link that does not resolve from this checkout; it is outside this task's new navigation, which resolves correctly.
