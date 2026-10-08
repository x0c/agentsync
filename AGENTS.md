<!-- managed:inherited-agents:start -->
<!-- source: ~/Codes/AgentSync/AGENTS.md -->
# AgentSync

Synchronize AI coding-agent instructions, complete skill folders, and MCP configurations from canonical user-owned sources.

The public Go module and Git repository are maintained in the `cli/` component. In a standalone clone, that component is the repository root: run Git and Go commands from the directory containing `go.mod`.

## Documentation

- [Repository instructions](https://github.com/x0c/agentsync/blob/main/AGENTS.md): contribution rules, validation, and the project documentation index.
- [Public repository](https://github.com/x0c/agentsync): installation, usage, source, and releases.

<!-- managed:inherited-agents:end -->

# Repository instructions

agentsync is a standalone Go CLI that aligns global agent instructions, complete skill directories, and MCP configuration.

## Contribution rules

- Use English for project instructions, contributor documentation, code comments, logs, errors, test diagnostics, CLI output, and generated notices. Keep `README.zh-CN.md` as a complete Chinese translation and retain the language switch in `README.md`. User-owned instructions and skill content keep their original language; agentsync does not translate them.
- Keep contributor instructions usable from a standalone clone. Do not depend on a maintainer's home directory, private services, or external local documents. Do not publish credentials, personal absolute paths, private network addresses, or machine inventories.
- Use Go 1.25 or newer. Prefer the standard library; TOML (`github.com/pelletier/go-toml/v2`) and YAML (`gopkg.in/yaml.v3`) are the existing MCP parsing exceptions. JSONC handling remains in this repository.
- Keep command parsing in `main.go` and synchronization logic in `internal/agentsync/`. Preserve unrelated settings in mixed runtime configuration files.
- Preserve existing user content before replacing files or directories. Keep `--check` read-only. Gate instruction, skill, and MCP targets on installed-runtime detection; do not create directories for absent runtimes.
- Use isolated homes and configuration roots in tests. Never write test backups into a real user's configuration directory.
- Changes to paths, backup/merge behavior, skill/MCP synchronization, policy semantics, or command flags require matching updates to the relevant guides and both READMEs.
- Keep `CLAUDE.md` as the single line `@AGENTS.md`.
- `TASKBOARD.md` lists active work only. Edit your own entry, keep its scope current, and remove it when complete; preserve other contributors' work.

## Validation and release

From the repository root, run:

```sh
python3 scripts/check_repository.py
python3 -m unittest discover -s scripts -p "test_*.py"
go test -race ./...
go build ./...
go vet ./...
golangci-lint run ./...
goreleaser check
```

The Python 3.9+ check uses only the standard library. It checks repository language, portable documentation links, and public-path hygiene. Review full diagnostic output and investigate each distinct warning; a successful build alone is insufficient.

Exercise the changed behavior through the CLI with an isolated home before installing. Build a binary into a temporary directory, then verify preview -> apply -> recheck, content preservation, backups, and an absent runtime staying absent. Use `--repo --check` to validate repository mode without modifying the checkout. The [workflow guide](docs/AGENTSYNC_GUIDE.md) covers expected results and watcher verification.

For a local install, choose a directory on `PATH`, set `GOBIN` explicitly, and check the resolved executable before validating it. An enabled watcher must be restarted after its binary is replaced. Never assume a bare `go install .` replaces the executable the user actually runs.

Releases use `v*` tags and [GoReleaser configuration](.goreleaser.yml). The release workflow tests and builds the tagged commit on Linux, macOS, and Windows before publishing archives and updating `x0c/homebrew-tap`. Confirm release assets and the tap version; do not substitute an earlier commit's CI result. The workflow guide also documents a local release path. Release notes are English first with a complete Simplified Chinese translation.

## Document index

Read the documents relevant to the current task before changing behavior; consult adjacent guides when useful.

| Document | Contents |
|---|---|
| [README.md](README.md) / [README.zh-CN.md](README.zh-CN.md) | Installation, first use, feature boundaries, and public safety guarantees. |
| [Workflow guide](docs/AGENTSYNC_GUIDE.md) | Commands, report states, watch/service setup, isolated validation, Cursor troubleshooting, and releases. |
| [Synchronization knowledge base](docs/AGENTSYNC_KNOWLEDGE_BASE.md) | Ownership, synchronization mechanisms, preservation boundaries, implementation anchors, and known limitations. |
| [Instruction and skill paths](docs/agent_runtime_global_paths.md) | Implemented runtime targets, discovery differences, and confidence limits. |
| [MCP paths and formats](docs/agent_runtime_mcp_paths.md) | Runtime configuration locations, merge strategies, schema translation, and compatibility constraints. |
| [GitHub discovery guide](docs/GITHUB_DISCOVERY_GUIDE.md) | Public positioning, README/platform consistency, discovery references, and traffic measurement limits. |
