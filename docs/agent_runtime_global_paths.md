# Instruction and skill paths

This table describes agentsync's implemented targets in `defaultGlobalConfig()` in `internal/agentsync/paths.go`. It is a filesystem compatibility reference, not a claim that every version of every runtime loads these files in every surface. The original survey was recorded in July 2026; implementation changes require checking current upstream documentation and the installed runtime.

`~` means the user's home directory (`HOME` on Unix-like systems, `USERPROFILE` on Windows). Each target is gated on its runtime home directory; missing runtimes are skipped without creating files. MCP output paths and Windows-specific MCP locations are documented separately in the [MCP guide](agent_runtime_mcp_paths.md).

## Implemented targets

A dash means agentsync does not create a dedicated target for that feature. A runtime may still discover shared skills itself. Paths are defaults; Pi and dsh have explicit root handling described below.

| Runtime | Global instruction target | Dedicated skill target |
|---|---|---|
| Claude Code | `~/.claude/CLAUDE.md` | `~/.claude/skills/` |
| Codex | `~/.codex/AGENTS.md` | `~/.codex/skills/` |
| OpenCode | `~/.config/opencode/AGENTS.md` | `~/.config/opencode/skills/` |
| Gemini CLI | `~/.gemini/GEMINI.md` | - |
| Qwen Code | `~/.qwen/QWEN.md` | `~/.qwen/skills/` |
| GitHub Copilot CLI | `~/.copilot/copilot-instructions.md` | `~/.copilot/skills/` |
| Kimi Code | `~/.kimi-code/AGENTS.md` | `~/.kimi-code/skills/` |
| Grok CLI (superagent-ai) | `~/.grok/AGENTS.md` | `~/.grok/skills/` |
| Amp | `~/.config/amp/AGENTS.md` | `~/.config/amp/skills/` |
| Crush | `~/.config/crush/CRUSH.md` | `~/.config/crush/skills/` |
| Goose | `~/.config/goose/AGENTS.md` | - |
| Factory Droid | `~/.factory/AGENTS.md` | `~/.factory/skills/` |
| iFlow | `~/.iflow/IFLOW.md` | `~/.iflow/skills/` |
| Kilo | `~/.config/kilo/AGENTS.md` | - |
| Pi | `<Pi agent dir>/AGENTS.md` | Shared `~/.agents/skills/` entry |
| dsh (DeepSeek Harness) | `~/.dsh/AGENTS.md` | `~/.dsh/skills/` |
| Cursor | `~/.cursor/rules/AGENTS.mdc` | `~/.cursor/skills/` |
| Windsurf | `~/.codeium/windsurf/memories/global_rules.md` | `~/.codeium/windsurf/skills/` |
| Zed | `~/.config/zed/AGENTS.md` | Shared `~/.agents/skills/` entry |
| CodeBuddy | `~/.codebuddy/CODEBUDDY.md` | `~/.codebuddy/skills/` |
| Qoder | `~/.qoder/AGENTS.md` | `~/.qoder/skills/` |
| Junie | `~/.junie/AGENTS.md` | - |
| Kiro | `~/.kiro/steering/AGENTS.md` | `~/.kiro/skills/` |
| JoyCode | `~/.joycode/AGENTS.md` | `~/.joycode/skills/` |
| Aider Desk | - | `~/.aider-desk/skills/` |
| Shared entry | `~/.agents/AGENTS.md` | `~/.agents/skills/` |

`PI_CODING_AGENT_DIR` overrides Pi's agent directory; the default is `~/.pi/agent`. Pi needs no additional skill alias because it can read the shared entry. `DSH_HOME` controls dsh MCP patch output; the instruction and skill entries above are the defaults currently defined in the target list. Do not infer that every runtime's configuration-root override applies to all agentsync features.

## Loading and confidence limits

- **Project and global scope differ.** agentsync repository mode manages `CLAUDE.md -> AGENTS.md`; it does not convert every vendor's project rule format or generate team rules.
- **Claude instructions.** Current [official documentation](https://code.claude.com/docs/en/memory#agentsmd) includes native project `AGENTS.md` discovery, subject to instruction-file selection and existing Claude files. The explicit `@AGENTS.md` import remains supported. Keep this repository's one-line entry for compatibility rather than duplicating instructions. The earlier blanket claim that Claude never reads `AGENTS.md` natively is obsolete.
- **Codex skill discovery.** Both the Codex root and shared `~/.agents/skills/` can be discovered. Older survey wording calling the Codex root merely obsolete was incomplete. Per-target filtering is not a global disable; see the [known filtering limitations](AGENTSYNC_KNOWLEDGE_BASE.md#known-filtering-limitations).
- **Cursor injection.** agentsync writes a managed `.mdc` with `alwaysApply: true`, not a bare symlink. Historical reports describe Settings listing global file rules while an Agent session in Agents Window or a home-directory workspace omits their body. Skills use a separate path and can still load. Open a concrete project workspace and verify a fresh session can quote a distinctive rule; User Rules text or an explicit chat reference can serve as a workaround. A historical report of requestable-only handling being fixed in Cursor 3.2 is not a guarantee about every installed client.
- **Kimi generations.** Legacy `kimi-cli` used `~/.kimi/`; this implementation targets Kimi Code under `~/.kimi-code/`. Do not create both generations' entries as a precaution.
- **JoyCode confidence.** Public rule-path documentation was incomplete in the original survey. The MCP path has extension-source evidence; that does not establish the same confidence for instruction or skill loading. Do not confuse a maintained compatibility target with a verified injection contract.
- **Grok identity.** The Grok target refers to the community `superagent-ai/grok-cli` implementation. Do not imply that the path is an official xAI CLI standard.
- **Unsupported survey entries.** Broader historical research included tools with cloud-only rules, unpublished paths, or no matching agentsync target. Their presence in an ecosystem survey is not feature support. Add a runtime only with a validated path, installation gate, tests, and aligned public documentation.

Runtime discovery, precedence, enterprise controls, trust, and prompt-size limits belong to the runtime. Do not generalize one client's ordering or memory behavior to all others. Filesystem synchronization does not establish prompt injection.

## Upstream references

Use the appropriate runtime's current documentation when changing a contract. Preserve version-specific evidence when it explains a compatibility constraint; inaccessible research scratch files are not contributor documentation.

- [Claude Code memory and instructions](https://code.claude.com/docs/en/memory)
- [Codex AGENTS.md guide](https://developers.openai.com/codex/guides/agents-md) and [skills](https://developers.openai.com/codex/skills)
- [OpenCode rules](https://opencode.ai/docs/rules/) and [skills](https://opencode.ai/docs/skills/)
- [Cursor rules](https://cursor.com/docs/context/rules)
- [Windsurf rules](https://docs.windsurf.com/windsurf/cascade/memories)
- [Pi coding-agent documentation](https://github.com/earendil-works/pi/tree/main/packages/coding-agent/docs)
- [Grok CLI implementation](https://github.com/superagent-ai/grok-cli)

The [MCP guide](agent_runtime_mcp_paths.md#sources) retains configuration-specific upstream references and source evidence for paths that differ from instruction locations.
