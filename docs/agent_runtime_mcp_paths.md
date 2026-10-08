# MCP paths and formats

This guide describes agentsync's implemented user-level MCP targets and conversion rules. The initial path review was recorded in August 2026, with Pi compatibility added in September. Version-specific observations are compatibility evidence, not assertions that upstream formats never change. Consult current official sources before changing a contract. Instruction and skill targets are in the [path guide](agent_runtime_global_paths.md).

Implementation starts at `syncMCP()` in `internal/agentsync/mcp.go`; `defaultGlobalConfig()` owns target definitions. `~` denotes the user's home directory. Missing runtime detection directories produce `skipped` without creating files.

## Canonical source and policy

`~/.config/agentsync/mcp.json` owns the complete server set. Global sync injects an English reminder into canonical `AGENTS.md` to edit that source rather than runtime outputs.

```json
{
  "mcpServers": {
    "example-local": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "example-mcp"],
      "env": {}
    },
    "example-remote": {
      "type": "http",
      "url": "https://example.com/mcp",
      "headers": {}
    }
  }
}
```

A missing canonical file is initialized from installed runtimes using case-insensitive server names; the first definition wins. Optional `sync-policy.json` filters output only. Deny wins, then a nonempty target allowlist, then the section default (`allow` when omitted). Names are case-insensitive; unknown runtime names warn. Policy does not filter import, individual MCP tools, enterprise allowlists, or runtime approval state.

```json
{
  "version": 1,
  "mcp": {
    "default": "allow",
    "targets": {
      "codex": { "deny": ["example-remote"] },
      "claude": { "allow": ["example-local"] }
    }
  },
  "skills": {
    "default": "allow",
    "targets": {
      "codex": { "deny": ["example-skill"] }
    }
  }
}
```

Skill policy follows the same evaluation, materializing a filtered directory of per-skill links when needed; otherwise the whole skill root aliases the canonical root. See the [knowledge base](AGENTSYNC_KNOWLEDGE_BASE.md#known-filtering-limitations) for shared-root discovery limits.

Mixed configuration files must preserve unrelated keys; never replace or symlink the entire file. New canonical files use `0600`, and existing target permissions are preserved. MCP data can contain plaintext secrets; exclude canonical MCP, backups, and merge drafts from Git/Syncthing. Per-runtime trust and OAuth remain user-owned. Cross-runtime timeout fields are dropped rather than implicitly converting seconds and milliseconds, except for preserving existing Pi-native options.

Codex bundled local servers stay available to Codex but are filtered from other runtimes. There is no MCP target for iFlow or the shared `~/.agents` entry. TOML/YAML parsing libraries are the documented exceptions to the standard-library preference; JSONC handling is local.

## Targets

`file` replaces a dedicated MCP file; `key` updates only the MCP key in a mixed configuration; `patch` replaces only a marked section. Runtime homes are the installation gates. Root overrides and existing-file detection can alter the default targets below.

| Runtime | Default output | Mode | Shape |
|---|---|---|---|
| Codex | `~/.codex/config.toml` | key | TOML `mcp_servers` |
| Claude Code | `~/.claude.json` | key | JSON `mcpServers` |
| OpenCode | `~/.config/opencode/opencode.json` or existing JSONC | key | Flat `mcp.<name>` |
| Gemini CLI | `~/.gemini/settings.json` | key | `mcpServers` |
| Qwen Code | `~/.qwen/settings.json` | key | Gemini dialect |
| Copilot CLI | `~/.copilot/mcp-config.json` | file | `mcpServers` |
| Kimi Code | `~/.kimi-code/mcp.json` | file | `mcpServers` |
| Grok CLI | `~/.grok/user-settings.json` | key | `mcp.servers` array |
| Amp | `~/.config/amp/settings.json` | key | Literal dotted key `amp.mcpServers` |
| Crush | `~/.config/crush/crush.json` | key | `mcp`, inverse `disabled` switch |
| Goose | `~/.config/goose/config.yaml` | key | `extensions` map |
| Factory Droid | `~/.factory/mcp.json` | file | `mcpServers` |
| Kilo | `~/.config/kilo/kilo.jsonc` or existing JSON | key | Flat OpenCode-compatible `mcp` |
| Cursor | `~/.cursor/mcp.json` | file | `mcpServers` |
| Windsurf | `~/.codeium/windsurf/mcp_config.json` | file | `mcpServers`, remote `serverUrl` |
| Zed | `~/.config/zed/settings.json` | key | `context_servers` |
| CodeBuddy | Existing path selected below | file/key | `mcpServers` |
| Qoder | `~/.qoder/settings.json` | key | `mcpServers` |
| Junie | `~/.junie/mcp/mcp.json` | file | `mcpServers` |
| Kiro | `~/.kiro/settings/mcp.json` | file | `mcpServers` |
| JoyCode | `~/.joycode/joycode-mcp.json` | file | `mcpServers` |
| Pi native | `<Pi agent dir>/mcp.json` | key | `mcpServers` |
| Pi adapter | `~/.config/mcp/mcp.json` | file | `mcpServers` |
| dsh | `$DSH_HOME/cordis.patch.yml` | patch | Managed plugin insertion block |

Windows-specific MCP outputs include Amp `%APPDATA%\amp\settings.json`, Crush `%LOCALAPPDATA%\crush\crush.json`, Goose `%APPDATA%\Block\goose\config\config.yaml`, and Zed `%APPDATA%\Zed\settings.json`. Do not infer native installation success from Unix path tests alone.

## Runtime-specific boundaries

### Claude Code

Change only top-level `mcpServers` in `~/.claude.json`, or the corresponding file under `CLAUDE_CONFIG_DIR`. Do not read or write `~/.claude/.mcp.json` as a user-level source. Preserve OAuth, project trust, statistics, project-local MCP entries, manually disabled servers, and enterprise controls. Same-directory atomic saves can replace symlinks, so whole-file linking is inappropriate.

### OpenCode and Kilo

The implemented dialect uses flat `mcp.<name>`, `type: local|remote`, `command: [executable, ...args]`, and `environment`. The original source review used OpenCode 1.18.18. Do not silently replace this format with a different version's nested `mcp.servers` layout or switch polarity.

Existing `opencode.json` takes write precedence; otherwise use existing JSONC, then create JSON. Historical loading merged both files, with JSONC overriding same-name entries. A nonempty secondary configuration can therefore affect effective behavior; report it rather than writing competing copies. Kilo uses the flat compatible dialect in this implementation.

### CodeBuddy, Grok, and JoyCode

CodeBuddy reads the first available same-scope file: `~/.codebuddy/.mcp.json`, then legacy `mcp.json`, then mixed `~/.codebuddy.json`. Write the existing winner. For the mixed legacy file, merge only `mcpServers`; creating a higher-priority file would silently hide its servers. If none exists, create `.mcp.json` only when installed.

Grok refers to `superagent-ai/grok-cli`. Use the `mcp.servers` array in `~/.grok/user-settings.json`, retaining API keys and other state. The older README's `.grok/settings.json`/`mcpServers` description disagreed with the reviewed implementation.

JoyCode's user path came from `getUserConfigPath()` in extension version 3.8.67. Its project `.joycode/mcp.json` is outside this sync scope. The extension migrates legacy `~/.joycoder/joycoder-mcp.json` only before `.joycode` exists; never create an empty home for an absent runtime. A historical tutorial's misspelled legacy path is not authoritative.

### dsh (DeepSeek Harness)

Each server is a host-level `@deepseek-ai/dsh-mcp-client` plugin instance in `$DSH_HOME/cordis.patch.yml` (default root `~/.dsh`), applying across profiles. Entries use `id: mcp-<name>` and `config.serverName/transport/command/args/env/cwd/url/headers`. Names must match `[A-Za-z0-9_-]{1,32}`. Stdio and streamable HTTP are supported; invalid names or SSE are blocked.

Only replace the sorted `- insert:` block between `# managed-by: agentsync start/end`. Preserve every byte outside it: custom YAML tags such as `!!js` make full-file reserialization unsafe. Check mode compares the generated block text only.

Require `dsh-mcp-client` in at least one profile's package dependencies before writing. If absent, report `blocked` with the profile installation command. Mixed profile dependency coverage is reported. A same-name manually configured host server blocks the write; a profile-level name collision warns because host configuration loads later.

After writing, validate with `dsh --profile <web-or-first-profile> --dump-config` when the executable and profiles exist. Failure restores the backup and reports `blocked`. This validates composition, not npm resolution; the dependency gate handles package declarations. Explicitly report skipped validation when unavailable.

New patch files use `0600`; preserve existing permissions. Users must exclude their dsh patch from any Git/Syncthing configuration separately because agentsync's secret-ignore handling covers only its own canonical root. Do not read or write community-plugin OAuth/status files, workspace-scoped MCP stores, or UI-specific private schemas.

### Pi (earendil-works/pi)

Keep stable policy key `pi`. In `auto` mode, an enabled `pi-mcp-adapter` package or extension selects shared `~/.config/mcp/mcp.json`; otherwise use native `<Pi agent dir>/mcp.json`. `piMCP` can explicitly select `native` or `adapter`; invalid modes fail before writes. Respect `PI_CODING_AGENT_DIR`, skip absent agent directories, and watch package-selection changes. Do not install/remove packages or rename configurations during sync.

The September 2026 review found built-in MCP in Pi 0.99.0, including `pi mcp` and `/mcp`, global native `mcp.json`, trusted project `.pi/mcp.json`, stdio/streamable HTTP, and deferred or codemode tools. Legacy SSE is outside that native contract. Native writes preserve top-level settings and existing per-server options while removing servers no longer in the canonical set.

Adapter 3.0.0 moved private overrides/imports/settings to `mcp-adapter.json` while retaining the shared MCP source. Project adapter settings can override globals. An adapter filename-migration warning does not authorize renaming a Pi-native MCP file: separate adapter settings carefully and avoid connecting identical servers twice. Native OAuth requires its own sign-in; agentsync does not migrate adapter credentials. Older Pi installations can still require an extension; do not infer deployment popularity from package downloads.

Versioned sources: [Pi changelog](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/CHANGELOG.md), [native guide](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/mcp.md), [adapter v3 migration](https://github.com/nicobailon/pi-mcp-adapter/releases/tag/v3.0.0), [adapter configuration](https://github.com/nicobailon/pi-mcp-adapter/blob/main/config.ts), and [alternative extension](https://github.com/irahardianto/pi-mcp-extension).

Isolated tests cover selection, explicit modes, policy, preview, native-option preservation, custom roots, and watch changes. Historical Pi 0.99.1 acceptance exercised two read-only native tools. This does not prove every host's network permission, OAuth, or remote connectivity. Skills continue through the shared entry.

### Other path constraints

- Kimi Code uses `mcp.json` under its configured home, not TOML timeout defaults or legacy `~/.kimi/mcp.json`. The reviewed 0.31.x CLI had no `kimi mcp` command; verify newer versions before relying on that historical limitation.
- Junie uses `~/.junie/mcp/mcp.json`, not the older one-level path.
- Qoder uses the `mcpServers` key in `settings.json`; do not invent `~/.qoder/mcp.json`.
- Amp uses the literal dotted user-level key; a project `.amp/settings.json` has a separate approval scope.
- Goose uses an `extensions` map, not a list or an extra `config:` wrapper from internal Rust structures.
- iFlow has no implemented MCP target. Any future compatibility work must separately validate its effective settings path.

## Conversion reference

| Dialect | Stdio / HTTP type | Command / environment | Remote / headers | Switch |
|---|---|---|---|---|
| Claude, Cursor, Copilot, Factory, Kimi, Kiro, JoyCode, Pi | `stdio` or omitted / `http` | `command`, `args`, `env` | `url`, `headers` | Runtime-specific switches are not generally copied |
| Windsurf | Omit type | `command`, `args`, `env` | `serverUrl` (import accepts `url` too) | Omit |
| Codex | Omit type | `command`, `args`, `env` | `url`, `http_headers` | `enabled` |
| OpenCode / Kilo | `local` / `remote` | argv array `command`, `environment` | `url`, `headers` | `enabled` |
| Crush | `stdio` / `http` | `command`, `args`, `env` | `url`, `headers` | Inverse `disabled` |
| Goose | `stdio` / `streamable_http` | `cmd`, `args`, `envs` | `uri`, `headers` | `enabled`, plus `name` |
| Grok | `transport: stdio` / HTTP or SSE | `command`, `args`, `env` | `url` | Array-entry `enabled` |
| Zed | Omit type | Flat `command` string, `args`, `env` | `url`, `headers` | Optional `enabled` |
| Amp | Omit type | `command`, `args`, `env` | `url`, `headers` | Separate permission channel |
| Gemini / Qwen | Stdio optional / HTTP-specific key | `command`, `args`, `env` | HTTP `httpUrl`, SSE `url` | Separate enablement channel |

Use `http`, not `streamable-http`, in the implemented Cursor dialect. Never copy a boolean without its polarity: `enabled` and Crush's `disabled` are opposites. Timeout units differ and are not converted implicitly. Environment placeholder syntaxes differ too; unrecognized forms can become literal credentials. Crush converts `${env:X}` to `$X` and rejects `$(...)`, which could execute commands during loading.

## Verification and sources

Validate the effective top-level container, not merely valid JSON/TOML/YAML. Zed requires `context_servers` with a flat command; nested `command: {path,args,env}` is outside this implementation. Cursor filesystem convergence does not prove approval or project trust; prefer a noninteractive tool-list probe when available. Do not scan bundled plugin directories as user MCP sources.

Check mode must remain read-only. Test mixed-file preservation, malformed/empty-source rejection, installation gates, semantic idempotency, bundled-server isolation, policy filtering, and dsh block preservation. Runtime-specific authentication and connectivity need separate acceptance.

### Sources

- [Claude MCP documentation](https://code.claude.com/docs/en/mcp)
- [OpenCode MCP documentation](https://opencode.ai/docs/mcp-servers/); original source review: v1.18.18 `packages/opencode/src/config/config.ts` and `cli/cmd/mcp.ts`
- [Kimi Code MCP](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/mcp.html); reviewed package 0.31.1 `resolveMcpJsonPaths`
- [Grok settings source](https://github.com/superagent-ai/grok-cli/blob/main/src/utils/settings.ts)
- [Junie MCP](https://junie.jetbrains.com/docs/junie-cli-mcp-configuration.html)
- [CodeBuddy MCP](https://www.codebuddy.ai/docs/cli/mcp)
- [Qoder MCP](https://docs.qoder.com/cli/mcp-reference)
- [Windsurf MCP](https://docs.windsurf.com/windsurf/cascade/mcp)
- [Zed MCP](https://zed.dev/docs/ai/mcp)
- [Goose configuration](https://github.com/block/goose/blob/main/documentation/docs/guides/config-files.md)
- [Amp MCP](https://ampcode.com/manual/mcp.md)
- [Kilo MCP](https://kilo.ai/docs/automate/mcp/using-in-kilo-code)
- [Crush source](https://github.com/charmbracelet/crush): `internal/shell/expand.go`, `internal/config/config.go`
- [Cursor CLI MCP](https://cursor.com/docs/cli/mcp)
- [Codex MCP](https://developers.openai.com/codex/mcp)
- JoyCode: extension 3.8.67 `getUserConfigPath()`; public tutorials did not establish the user-level path at the original review date.
