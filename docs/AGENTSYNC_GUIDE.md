# Workflow guide

This guide covers commands, report states, service setup, validation, and releases. See the [synchronization knowledge base](AGENTSYNC_KNOWLEDGE_BASE.md) for preservation rules and implementation details.

## Commands

| Goal | Command | Writes files? |
|---|---|---|
| Preview global synchronization | `agentsync --check` | No |
| Apply global synchronization | `agentsync` | Yes |
| Keep canonical changes applied | `agentsync --watch` | On changes |
| Align the current repository's instruction entry | `agentsync --repo` | Yes |
| Align repositories under a directory | `agentsync --all ~/projects` | Yes |
| Adopt a reviewed merge draft | `agentsync --adopt <draft>` | Yes |
| Restore the latest backup | `agentsync --rollback latest` | Yes |
| Replace conflicting entries after backup | `agentsync --force` | Yes |

`main.go` maps flags into `Options`; `Run()` selects rollback, batch, repository, global, or watch execution. Synchronization reports instruction, skill, and MCP results separately.

## Global synchronization

| Canonical source | Purpose |
|---|---|
| `~/.config/agentsync/AGENTS.md` | User-owned global instructions |
| `~/.config/agentsync/skills/` | Complete skill directories |
| `~/.config/agentsync/mcp.json` | Machine-local MCP server definitions; may contain tokens |
| `~/.config/agentsync/sync-policy.json` | Optional per-runtime allow/deny policy; contains no tokens |

The target list comes from `defaultGlobalConfig()` in `internal/agentsync/paths.go`. Each target is gated by its runtime home directory (`Detect`). An absent directory produces `skipped` without creating files. The [instruction and skill path guide](agent_runtime_global_paths.md) lists implemented targets.

The first apply may report `created`, `merged`, `replaced`, or `linked`. Repeating it should converge installed targets to `ok`. Existing unique instructions are preserved before replacement. Managed copies, including Cursor rules, are derived from the source and are refreshed directly rather than imported back into it.

Global mode translates canonical MCP servers into installed runtimes' formats and injects an English reminder to edit only the canonical configuration. Repository and batch modes do not synchronize global instructions, skills, or MCP. Pi selects native or adapter output as described in the [MCP guide](agent_runtime_mcp_paths.md#pi-earendil-workspi).

For example, exclude a server from Codex while keeping it available to other runtimes:

```json
{
  "version": 1,
  "mcp": {
    "default": "allow",
    "targets": {
      "codex": { "deny": ["example-server"] }
    }
  }
}
```

Save the policy beside the canonical sources, then apply or let the watcher handle it. The report identifies filtered servers. Filtering never removes content from the canonical source.

## Check mode and report states

`--check` does not create sources, backups, directories, aliases, skills, ignore files, or MCP configuration. A `would ...` detail describes a future apply, not an action already performed.

| State | Meaning | Next step |
|---|---|---|
| `skipped` | Runtime is not installed | No action needed |
| `ok` | Target matches the canonical source or filtered view | No action needed |
| `warning` | Policy contains an unknown target name | Review the policy |
| `missing` | Source or target is absent | Apply to create it |
| `mergeable` | Target contains unique content | Apply, then review the preserved content |
| `replaceable` | Target can be backed up and replaced | Apply |
| `wrong-link` | Link points to the wrong source | Apply; use `--force` only for intentional replacement |
| `broken-link` | Link destination is absent | Apply to repair it |
| `blocked` | Target or configuration cannot be handled safely | Inspect the reported cause |

## Watch mode and services

`--watch` is optional; a plain `agentsync` remains a one-shot command. The watcher polls canonical instructions, MCP configuration, skills, policy, runtime detection, and Pi output selection. It does not monitor runtime output files or import edits made through a runtime's UI.

The default polling interval is two seconds. Changes must remain stable for 1.5 seconds; newly detected runtimes receive about five additional seconds for installation to finish. Skill fingerprints ignore Syncthing conflict files, `.DS_Store`, and `.stversions`, but include hidden skill content such as `.system`. Directory modification times are excluded so ignored files cannot trigger synchronization.

Instruction/skill-only changes with an unchanged policy skip MCP writes. MCP, policy, Pi selection, and runtime installation changes can trigger MCP synchronization. Failed runs do not advance the successful fingerprint and are retried; errors go to stderr while the watcher continues.

An empty, malformed, or missing-`mcpServers` canonical file is rejected without clearing installed configurations. Watch mode also rejects an explicitly empty server map while installed runtimes still contain servers. An intentional clear requires a one-shot apply.

Watch cannot combine with `--check`, `--repo`, `--all`, `--adopt`, `--rollback`, or `--force`. Service setup uses the templates in `contrib/`; there is no `service install` command.

On Linux, after installing the binary at the template's configured path:

```sh
mkdir -p ~/.config/systemd/user
cp contrib/systemd/agentsync.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now agentsync.service
```

The template defaults to `%h/.local/bin/agentsync --watch`. On macOS, copy `contrib/launchd/top.x0c.agentsync.plist`, set its executable path to the installed binary, and load it using launchd. After replacing an active watcher's binary, restart the service and verify it uses the replacement.

## Repository and batch modes

`--repo` discovers the current Git root and manages only:

```text
CLAUDE.md -> AGENTS.md
```

It uses a relative link so moving the repository does not break the entry. `--all <directory>` scans Git repositories recursively, skipping large dependency/build directories such as `node_modules`, `.venv`, `target`, and `build`. It aggregates results under `Repositories`; a repository error stops the run. Batch mode applies repository behavior, not a global sync inside every checkout.

## Rollback

Every real write session groups replaced files, directories, and links under a unique timestamp in `~/.config/agentsync/backups/`. Its `manifest.json` records original paths and types.

```sh
agentsync --check --rollback latest
agentsync --rollback latest
agentsync --rollback 20260916-120412
```

Rollback backs up current entries before restoring. It does not automatically resynchronize afterward, which would immediately replace the restored state. A stamp contains only entries replaced in that session, not a full machine snapshot; `latest` selects the newest stamp. Older backups without a manifest require manual restoration.

Rollback cannot combine with `--watch`, `--repo`, `--all`, `--adopt`, or `--force`.

## Merge drafts

Unique user-written content can be appended to the canonical source. Generated copies marked `managed-by: agentsync` are refreshed directly; importing a stale generated copy would duplicate obsolete instructions.

To replace the canonical instructions with manually prepared content, write and review a Markdown file, then run:

```sh
agentsync --adopt <draft-path>
```

Adoption expands and validates the path, backs up the current source, writes the reviewed draft, then synchronizes targets. `--adopt --check` checks availability without replacing the source.

## Cursor rules visible in Settings but absent from the prompt

A Cursor target reporting `linked` or `ok` proves filesystem synchronization. Settings listing `~/.cursor/rules/AGENTS.mdc` does not prove that a particular Agent session loaded its body. Historical reports describe missing injection in Agents Window or home-directory workspaces, and skills can still load through a separate discovery path.

Open a concrete project workspace and start a fresh session. Ask it to quote a distinctive heading from the canonical instructions. If necessary, use Cursor's User Rules text setting or explicitly reference the `.mdc` file in chat. Verify the session's actual content rather than relying on the Settings list. See the [path guide](agent_runtime_global_paths.md#loading-and-confidence-limits) for scope and sources; these are compatibility observations, not a promise about every Cursor version.

## Development validation

Run the commands in [repository instructions](../AGENTS.md#validation-and-release). Tests must use temporary configuration roots. Review complete output, including warnings.

Build into a temporary directory and exercise the real CLI with a temporary home containing two installed-runtime directories and distinct instruction files. Verify:

1. Preview leaves all files unchanged and creates no backups.
2. Apply preserves both instruction texts and backs up replaced entries.
3. Recheck reports installed instruction targets as `ok`.
4. An absent runtime remains absent; reports and generated notices are English.
5. Repository preview reports only the checkout's instruction entry, without global skills or MCP.

Use temporary `HOME`, `XDG_CONFIG_HOME`, and any runtime-specific root overrides; never point a write-mode smoke test at real user configuration. The synchronization knowledge base records additional MCP and skill edge cases.

For installation, choose a directory on `PATH` and set `GOBIN` explicitly. Check `command -v agentsync` and the resolved binary; a Homebrew installation can precede a source installation on `PATH`. No HTTP service or database is required.

## Releases

Choose the next unused version, commit the validated workspace, tag that commit, and push the branch and tag. The tag workflow reuses CI for the same commit on Linux, macOS, and Windows before running GoReleaser. Archives cover macOS/Linux amd64 and arm64, and Windows amd64; Windows arm64 is excluded in `.goreleaser.yml`.

The release token needs repository write permission. `HOMEBREW_TAP_GITHUB_TOKEN` needs write access to `x0c/homebrew-tap`. Confirm the published archive list, checksums, latest release, and cask version. Release notes use English first followed by a complete Simplified Chinese translation.

A local release path is also available from the committed, tagged checkout:

```sh
goreleaser check
goreleaser release --clean --release-notes /path/to/reviewed-notes.md
```

Use authenticated release/tap tokens from the environment without logging them. Run this only after the same tagged commit has passed the required platform validation and when the automated publisher is not publishing the same tag. This is the same packaging configuration used by CI; do not bypass failed validation or publish from a dirty tree.

Long remote CI/publication waits may run in a traceable background process. Keep the run URL and completion log, and report pending outcomes accurately. A successful upload alone does not establish installability: validate the downloaded archive and installed command through preview, apply, and recheck.

### Retained Homebrew compatibility warning

Homebrew 7 reports `Calling postflight is deprecated` for the generated cask. The origin is GoReleaser's `homebrew_casks.hooks.post.install` wrapper, not agentsync's executable. The hook removes quarantine from the downloaded macOS binary. The v0.16.1 release passed archive checksum verification, Homebrew upgrade, installed-binary verification, and isolated preview -> apply -> recheck despite this warning.

Retain this existing hook while its replacement is assessed; removing it can change whether an unsigned download runs. [Homebrew's cookbook](https://docs.brew.sh/Cask-Cookbook) temporarily supports legacy flight blocks in third-party taps. [GoReleaser's upstream issue](https://github.com/goreleaser/goreleaser/issues/6870) tracks its generated wrapper. Remaining risk: a future Homebrew release may remove the compatibility path. Follow up by migrating the release source to declarative steps, checking the supported Homebrew version range, Linux exclusion, sandboxed quarantine removal, and a real downloaded-binary install. A wrapper rename alone is insufficient, and editing the generated tap is overwritten by the next release.
