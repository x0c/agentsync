# agentsync

English | [简体中文](README.zh-CN.md)

`agentsync` is a small Go CLI that keeps AI coding-agent instructions and reusable skills in one canonical place.

It prevents drift between tools such as Codex, Claude Code, and OpenCode by converging their global instruction files and `SKILL.md` directories into shared sources under `~/.config/agentsync`.

## What It Manages

Canonical instruction file:

```text
~/.config/agentsync/AGENTS.md
```

Tool-specific instruction aliases:

```text
~/.codex/AGENTS.md
~/.config/opencode/AGENTS.md
~/.claude/CLAUDE.md
```

Canonical skill directory:

```text
~/.config/agentsync/skills/<skill-name>/SKILL.md
```

Tool-specific skill aliases:

```text
~/.claude/skills/<skill-name>
~/.codex/skills/<skill-name>
~/.config/opencode/skill/<skill-name>
```

Each skill is managed as a whole directory. A skill must contain `SKILL.md`; any scripts, templates, references, or assets next to it stay with that skill.

## Install

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

Converge global instructions and skills:

```bash
agentsync
```

Running `agentsync` repeatedly is intended to be idempotent. After the first convergence, later runs should report `ok` for managed aliases.

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
- Existing skill directories are copied into the canonical skill directory before aliases are created.
- Replaced files and directories are backed up under `~/.config/agentsync/backups/`.
- Hidden skill directories such as Codex `.system` internals are ignored.
- macOS and Linux use symlinks first.
- Windows tries symlinks first, then hardlinks, then a managed copy with a marker comment.

## Development

Project documentation starts at [docs/OVERVIEW.md](docs/OVERVIEW.md).

```bash
go test ./...
go build ./...
agentsync --check
```

## License

MIT
