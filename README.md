# agentsync

English | [简体中文](README.zh-CN.md)

`agentsync` is a small Go CLI that keeps AI coding-agent instructions, reusable skills, and MCP server configs in one canonical place.

It prevents drift across many AI coding agents — Codex, Claude Code, OpenCode, Gemini CLI, Qwen Code, Copilot CLI, Kimi Code, Grok, Amp, Crush, Goose, Factory Droid, iFlow, Kilo, Cursor, Windsurf, Zed, CodeBuddy, Qoder, Junie, Kiro, JoyCode, and more — by converging their global instruction files and `SKILL.md` directories into shared sources under `~/.config/agentsync`.

## Only Touches Installed Runtimes

Each supported runtime is gated on its own home directory (for example `~/.codex`, `~/.gemini`, `~/.joycode`). If that directory does not exist, agentsync treats the runtime as **not installed** and reports it as `skipped` — it never creates directories or alias files for tools you do not use. Install a new agent, run `agentsync` again, and it converges on the next pass.

## What It Manages

Canonical instruction file:

```text
~/.config/agentsync/AGENTS.md
```

Tool-specific instruction aliases (created only when the runtime is installed):

```text
~/.codex/AGENTS.md
~/.config/opencode/AGENTS.md
~/.claude/CLAUDE.md
~/.gemini/GEMINI.md
~/.qwen/QWEN.md
~/.copilot/copilot-instructions.md
~/.kimi-code/AGENTS.md
~/.grok/AGENTS.md
~/.config/amp/AGENTS.md
~/.config/crush/CRUSH.md
~/.config/goose/AGENTS.md
~/.factory/AGENTS.md
~/.iflow/IFLOW.md
~/.config/kilo/AGENTS.md
~/.cursor/rules/AGENTS.mdc
~/.codeium/windsurf/memories/global_rules.md
~/.config/zed/AGENTS.md
~/.codebuddy/CODEBUDDY.md
~/.qoder/AGENTS.md
~/.junie/AGENTS.md
~/.kiro/steering/AGENTS.md
~/.joycode/AGENTS.md
~/.agents/AGENTS.md
```

Cursor's entry is a managed `.mdc` rule (`alwaysApply: true` frontmatter + source body), not a bare symlink — Cursor ignores plain rule files without frontmatter. If Settings lists the rule but the Agent cannot quote its body, Cursor often skips file-backed `~/.cursor/rules` when the workspace is `$HOME` / Agents Window has no project open; open a real project workspace, or paste critical text into Settings → User Rules (see `docs/AGENTSYNC_GUIDE.md`).

Canonical skill directory:

```text
~/.config/agentsync/skills/<skill-name>/SKILL.md
```

Tool-specific skill aliases (created only when the runtime is installed), each pointing at `~/.config/agentsync/skills`:

```text
~/.claude/skills          ~/.config/amp/skills
~/.codex/skills           ~/.config/crush/skills
~/.config/opencode/skills ~/.factory/skills
~/.qwen/skills            ~/.iflow/skills
~/.copilot/skills         ~/.aider-desk/skills
~/.kimi-code/skills       ~/.cursor/skills
~/.grok/skills            ~/.codeium/windsurf/skills
~/.codebuddy/skills       ~/.qoder/skills
~/.kiro/skills            ~/.joycode/skills
~/.agents/skills
```

Each skill is managed as a whole directory under the canonical skill root. A skill must contain `SKILL.md`; any scripts, templates, references, or assets next to it stay with that skill. Because tool-specific skill roots point at the canonical root, adding, deleting, or renaming a canonical skill is reflected by every tool immediately.

Canonical MCP config:

```text
~/.config/agentsync/mcp.json
```

On global `agentsync` (not `--repo` / `--all`), that file is translated into each installed runtime's user-level MCP config. Mixed files such as `~/.claude.json` and `~/.codex/config.toml` are key-merged so OAuth and other settings stay put; dedicated MCP files such as `~/.cursor/mcp.json` are replaced as a whole. iFlow is skipped. `~/.agents` has no MCP entry. Codex bundled local servers (`node_repl`, `computer-use`) stay on Codex only. Edit the canonical file only — agentsync injects a reminder into `~/.config/agentsync/AGENTS.md`. `mcp.json` is machine-local (tokens, host paths) and is added to `.gitignore` / `.stignore` when the config directory is a git repo or Syncthing folder. Do not sync it across machines.

## Install

With Homebrew:

```bash
brew install --cask x0c/tap/agentsync
```

Or with Go:

```bash
go install github.com/x0c/agentsync@latest
```

Or from a checkout:

```bash
go install .
```

## Usage

Preview changes without writing anything:

```bash
agentsync --check
```

Converge global instructions, skills, and MCP configs:

```bash
agentsync
```

Running `agentsync` repeatedly is intended to be idempotent. After the first convergence, later runs should report `ok` for managed aliases.

Keep it applied without running the command by hand:

```bash
agentsync --watch
```

This polls the canonical `AGENTS.md`, `mcp.json`, `skills/`, and whether each runtime's home directory exists. Edit the canonical files (or install a new agent) and the copies are rewritten for you. Changing only `AGENTS.md` or `skills/` does not rewrite MCP configs. MCP configs cannot be symlinks, so this is how those stay in sync. `--watch` cannot be combined with `--check`, `--repo`, `--all`, `--adopt`, or `--force`.

A systemd user unit lives in `contrib/systemd/agentsync.service`. On Linux:

```bash
mkdir -p ~/.config/systemd/user
cp contrib/systemd/agentsync.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now agentsync.service
```

On macOS, copy `contrib/launchd/top.x0c.agentsync.plist`, point `ProgramArguments` at `$(which agentsync)`, then `launchctl load` it.

## Repository Mode

Inside a Git repository:

```bash
agentsync --repo
```

This uses the repository `AGENTS.md` as the source and manages:

```text
CLAUDE.md -> AGENTS.md
```

Batch process repositories under a directory:

```bash
agentsync --all ~/Codes
```

## Safety

- `--check` is read-only.
- Existing unique instruction content is appended to the canonical source before aliases are created.
- Existing skill directories are copied into the canonical skill directory before tool-specific skill roots are replaced with aliases.
- MCP servers are imported once into `~/.config/agentsync/mcp.json` (first installed runtime wins on case-insensitive name clashes), then overwritten onto installed tools using each tool's schema. Codex bundled local servers are not copied to other tools. `--repo` and `--all` do not sync MCP.
- Replaced files and directories are backed up under `~/.config/agentsync/backups/`.
- Hidden skill directories such as Codex `.system` internals are preserved in the canonical skill root before tool-specific skill roots are replaced.
- macOS and Linux use symlinks first.
- Windows tries symlinks first, then hardlinks, then a managed copy with a marker comment.

## Development

Project documentation is indexed in [AGENTS.md](AGENTS.md).

```bash
go test ./...
go build ./...
agentsync --check
```

Tagged releases are built by GoReleaser. To publish the Homebrew cask, create the `x0c/homebrew-tap` repository and add a `HOMEBREW_TAP_GITHUB_TOKEN` secret with permission to push to that tap.

## License

MIT
