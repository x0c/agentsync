# 市面主流 Agent Runtime「全局规则文件」与「全局 Skill 目录」位置大全

> 调研日期：2026-07-18 · 信息以各工具**官方文档**为最高优先级，社区来源均已标注置信度
> 符号约定：`~` = 用户主目录（macOS/Linux 为 `$HOME`；Windows 为 `%USERPROFILE%`，即 `C:\Users\<用户名>`）；`<ws>` = 项目/工作区根目录
> 用户级 MCP 配置落点、写入策略与跨工具 schema 转换见 [agent_runtime_mcp_paths.md](agent_runtime_mcp_paths.md)（2026-08-15；由 `agentsync` 全局模式同步 `~/.config/agentsync/mcp.json`）。

## 一、总览速查表（按阵营分组）

### 1. 头部 CLI Agent（Claude / OpenAI / Google / 月之暗面 / 阿里）

| 工具 | 用户级（全局）规则/记忆文件 | 项目级规则文件 | 全局 Skill 目录 | 全局其他目录（commands/agents/plugins 等） |
|---|---|---|---|---|
| **Claude Code** | `~/.claude/CLAUDE.md`；用户规则目录 `~/.claude/rules/*.md`；自动记忆 `~/.claude/projects/<项目>/memory/MEMORY.md`；企业托管 macOS `/Library/Application Support/ClaudeCode/CLAUDE.md`、Linux `/etc/claude-code/CLAUDE.md`、Win `C:\Program Files\ClaudeCode\CLAUDE.md` | `./CLAUDE.md` 或 `./.claude/CLAUDE.md`（从 cwd 向上遍历拼接）+ `./CLAUDE.local.md`；`.claude/rules/*.md`；支持 `@path` 导入（最深 4 层）；**不原生读 AGENTS.md**，需 `@AGENTS.md` 导入 | `~/.claude/skills/<名>/SKILL.md` | 命令 `~/.claude/commands/*.md`（已并入 skills 体系）；子代理 `~/.claude/agents/*.md`；子代理记忆 `~/.claude/agent-memory/<名>/`；插件缓存 `~/.claude/plugins/`；设置 `~/.claude/settings.json`、`~/.claude.json` |
| **OpenAI Codex（CLI/IDE/cloud）** | `~/.codex/AGENTS.md`（`AGENTS.override.md` 优先；`CODEX_HOME` 可改根目录） | 项目根 `AGENTS.md` → 子目录逐级 `AGENTS.override.md`→`AGENTS.md`，自根向 cwd 合并（32 KiB 上限） | **`~/.agents/skills/`**（注意不是 `~/.codex/skills/`，后者为旧实验路径）；管理员级 `/etc/codex/skills` | 斜杠命令 `~/.codex/prompts/*.md`（已废弃，推荐 skills）；子代理 `~/.codex/agents/*.toml`；审批规则 `~/.codex/rules/*.rules`；配置 `~/.codex/config.toml` |
| **GitHub Copilot CLI** | `~/.copilot/copilot-instructions.md`；`~/.copilot/instructions/*.instructions.md`（`COPILOT_HOME` 可覆盖） | `.github/copilot-instructions.md` → `.github/instructions/*.instructions.md` → 根 `AGENTS.md`（另读根 `CLAUDE.md`/`GEMINI.md`） | `~/.copilot/skills/`（也接受 `~/.agents/skills/`） | 子代理 `~/.copilot/agents/*.agent.md`；hooks `~/.copilot/hooks/`；插件 `~/.copilot/installed-plugins/`；MCP `~/.copilot/mcp-config.json`；设置 `~/.copilot/settings.json` |
| **Gemini CLI** | `~/.gemini/GEMINI.md`；用户设置 `~/.gemini/settings.json` | `GEMINI.md`（工作区+父目录，JIT 子树加载；`context.fileName` 可改名） | 官方未单列全局 skills 目录（skills 经 `gemini skills install`，关联 extensions 体系） | 命令 `~/.gemini/commands/`（TOML）；扩展 `~/.gemini/extensions/<名>/gemini-extension.json` |
| **Kimi Code CLI（月之暗面）** | `~/.kimi-code/AGENTS.md`（即 `$KIMI_CODE_HOME/AGENTS.md`；旧版 kimi-cli 为 `~/.kimi/AGENTS.md`，两代并存） | 项目根 `AGENTS.md`（旧版另识别 `.kimi/AGENTS.md`、`agents.md`）；优先级：项目 > 全局 > `--system-prompt` | `~/.kimi-code/skills/`；跨工具通用 `~/.agents/skills/`；旧版回退链 `~/.config/agents/skills/`→`~/.agents/skills/`→`~/.kimi/skills/`→`~/.claude/skills/`→`~/.codex/skills/` | 配置 `~/.kimi-code/config.toml`；MCP `~/.kimi-code/mcp.json`；插件 `~/.kimi-code/plugins/`；自定义 agent 走 `--agent-file x.yaml`（无目录） |
| **Qwen Code（阿里开源）** | `~/.qwen/QWEN.md`（`QWEN_HOME` 可迁移）；自动记忆 `~/.qwen/projects/<项目>/memory/` | `QWEN.md`（从 cwd 向上至 git 根/主目录，多文件拼接；支持 `@path` 导入） | `~/.qwen/skills/<名>/SKILL.md` | 命令 `~/.qwen/commands/`；子代理 `~/.qwen/agents/`；扩展 `~/.qwen/extensions/` |

### 2. 开源 / 独立 CLI Agent

| 工具 | 用户级（全局）规则/记忆文件 | 项目级规则文件 | 全局 Skill 目录 | 全局其他目录 |
|---|---|---|---|---|
| **OpenCode** | `~/.config/opencode/AGENTS.md`（三平台相同，含 Windows）；回退 `~/.claude/CLAUDE.md`；全局配置 `~/.config/opencode/opencode.json` | `AGENTS.md`（cwd 向上遍历；`CLAUDE.md` 回退，每类首个匹配生效） | `~/.config/opencode/skills/<名>/SKILL.md`（兼容 `~/.claude/skills/`、`~/.agents/skills/`） | `~/.config/opencode/commands/`、`agents/`、`plugins/`（均为复数目录名） |
| **Amp（Sourcegraph）** | `~/.config/amp/AGENTS.md` 与 `~/.config/AGENTS.md`（均自动加载）；系统级 Linux `/etc/ampcode/AGENTS.md`、macOS `/Library/Application Support/ampcode/AGENTS.md`、Win `%ProgramData%\ampcode\AGENTS.md` | `AGENTS.md`（工作区+父目录至 $HOME；回退 `AGENT.md`/`CLAUDE.md`） | `~/.config/amp/skills/` | 插件 `~/.config/amp/plugins/*.ts`；设置 `~/.config/amp/settings.json` |
| **Crush（Charm）** | `~/.config/crush/CRUSH.md` + `~/.config/AGENTS.md`（双文件均加载；`global_context_paths` 可改） | `AGENTS.md`（`initialize_as` 可改名 CRUSH.md） | 按序：`$CRUSH_SKILLS_DIR`→`~/.config/agents/skills/`→`~/.config/crush/skills/`→`~/.agents/skills/`→`~/.claude/skills/`；Win 另有 `%LOCALAPPDATA%\{agents,crush}\skills\` | 全局配置 `~/.config/crush/crush.json` |
| **Goose（Block）** | `~/.config/goose/.goosehints`（默认也读 `~/.config/goose/AGENTS.md`）；配置 `~/.config/goose/config.yaml`（Win `%APPDATA%\Block\goose\config\`） | `.goosehints` / `AGENTS.md`（git 内嵌套合并，本地优先） | 未找到官方文档说明 | 提示模板 `~/.config/goose/prompts/`；配方 `~/.config/goose/recipes/` |
| **Aider** | **无自动加载的全局规则 md**；`~/.aider.conf.yml` 里写 `read: [CONVENTIONS.md...]` 可挂全局约定文件 | git 根 `.aider.conf.yml`、`.aiderignore`；CONVENTIONS.md 需 `--read` 显式挂载 | 无 | 无 |
| **Grok CLI（社区版 superagent-ai/grok-cli；xAI 无官方 CLI）** | 无全局规则 md；用户设置 `~/.grok/user-settings.json`（含 subAgents/hooks） | `AGENTS.md`（git 根→cwd 合并；`AGENTS.override.md` 覆盖）；项目设置 `.grok/settings.json` | `~/.agents/skills/` | 无（子代理定义在 user-settings.json 内） |
| **Factory AI Droid** | `~/.factory/AGENTS.md` | `./AGENTS.md`（cwd→父目录→子目录→个人级；也识别 CLAUDE.md） | `~/.factory/skills/<名>/SKILL.md`（兼容导入 `~/.claude/skills/`） | 子代理 `~/.factory/droids/<名>.md`；命令 `~/.factory/commands/`（legacy）；设置 `~/.factory/settings.json`、`~/.factory/mcp.json` |
| **Pi（earendil-works/pi）** | `~/.pi/agent/AGENTS.md`（`PI_CODING_AGENT_DIR` 可改根；`AGENTS.override.md` 优先） | `AGENTS.md` / `CLAUDE.md`（cwd + 向上父目录拼接） | `~/.pi/agent/skills/` 与 `~/.agents/skills/`（两者都原生扫描，同名取先发现者） | 设置 `~/.pi/agent/settings.json`；自定义模型 `~/.pi/agent/models.json`；OAuth/API key `~/.pi/agent/auth.json`；会话 `~/.pi/agent/sessions/`；扩展 `~/.pi/agent/extensions/`；包 `~/.pi/agent/npm/`；无内置 MCP（走 pi-mcp-adapter 扩展读 `~/.config/mcp/mcp.json` 等共享路径） |
| **iFlow CLI（心流）** | `~/.iflow/IFLOW.md`；全局配置 `~/.iflow/settings.json` | `IFLOW.md` 三层：全局 > 项目 > 目录级（从 cwd 向上搜索合并） | `~/.iflow/skills/` | 命令 `~/.iflow/commands/`（TOML/MD） |
| **OpenHands（原 OpenDevin）** | 无单一全局规则文件；用户状态目录 `~/.openhands/`（`agent_settings.json`） | 仓库根 `AGENTS.md`（推荐；亦支持 GEMINI.md/CLAUDE.md）；legacy `.openhands/microagents/repo.md`（已弃用） | 官方两处不一致：`~/.openhands/skills/` 或 `~/.agents/skills/`（项目级一致为 `.agents/skills/`） | — |
| **OpenClaw** | workspace 引导文件组，默认 `~/.openclaw/workspace/`：`AGENTS.md`（规则）、`SOUL.md`（人格）、`USER.md`（用户画像）、`IDENTITY.md`、`TOOLS.md`、`HEARTBEAT.md`；长期记忆 `MEMORY.md` + `memory/YYYY-MM-DD.md` | 无项目级概念（多 agent 用多 workspace 隔离，如 `~/.openclaw/workspace-alice/`） | `~/.openclaw/skills/`（项目级为 workspace 内 `skills/`） | 配置 `~/.openclaw/openclaw.json` |
| **Kilo Code** | `~/.config/kilo/AGENTS.md`（兼容 `~/.claude/CLAUDE.md`）；配置 `~/.config/kilo/kilo.jsonc` | `AGENTS.md`（首选）/`CLAUDE.md`/`CONTEXT.md`（findUp）；`.kilo/rules/`、`.kilo/rules-{mode}/`；legacy `.kilocoderules` | — | — |
| **Aider Desk** | 无自身全局规则 md（走 aider 层 `~/.aider.conf.yml`） | `AGENTS.md`（Agent 模式自动入系统提示） | `~/.aider-desk/skills/` | 子代理 `~/.aider-desk/agents/{名}/config.json`；命令项目级 `.aider-desk/commands/*.md` |
| **Plandex** | 无规则 md（状态目录 `~/.plandex-home/`） | 无（状态目录 `.plandex/`） | 无 | 无 |
| **mods（Charm）** | `mods.yml`：Linux `~/.config/mods/`、macOS `~/Library/Application Support/mods/`、Win `%LOCALAPPDATA%\mods\`；roles（system prompt）内联其中 | 无 | 无 | 无 |
| **aichat** | `config.yaml`：Linux `~/.config/aichat/`、macOS `~/Library/Application Support/aichat/`、Win `%APPDATA%\aichat\` | 无 | 无 | 角色 `<配置目录>/aichat/roles/<名>.md`；代理 `<配置目录>/aichat/agents/<名>/config.yaml` |
| **llm（Simon Willison）** | 用户目录：macOS `~/Library/Application Support/io.datasette.llm/`、Linux `~/.config/io.datasette.llm/`、Win `%APPDATA%\io.datasette.llm\` | 无 | 无 | 模板 `<用户目录>/templates/*.yaml` |
| **小米 MiMo Code（fork OpenCode）** | 未找到可靠资料；全局配置 `~/.config/mimocode/mimocode.json` | 项目记忆三层 `MEMORY.md` + `checkpoint.md` + `tasks/*/progress.md` | 未找到可靠资料 | 项目配置 `.mimocode/mimocode.json` |

### 3. IDE / 编辑器系 Agent

| 工具 | 用户级（全局）规则/记忆文件 | 项目级规则文件 | 全局 Skill 目录 | 全局其他目录 |
|---|---|---|---|---|
| **Cursor（IDE）** | Settings → Rules 的 User Rules 走云端/内部 DB（非公开单文件，跨项目最稳）；**文件型全局规则** `~/.cursor/rules/*.mdc`（Cursor 员工 2026-01 确认路径存在；agentsync 写 `AGENTS.mdc` + `alwaysApply: true`）。**注意**：Settings 列出 ≠ Agent 已注入——Agents Window 以 `$HOME` 为 workspace、或未打开具体项目时，file-backed 规则常不进系统提示（论坛 2026-04 员工确认类 bug / expected）；Skills 发现路径不同，可单独生效。官方 docs 仍把「全局偏好」主推为 Customize → User Rules 纯文本 | `.cursor/rules/`（.md/.mdc + frontmatter）；官方支持 `AGENTS.md`；旧 `.cursorrules` 向后兼容 | `~/.cursor/skills/`、`~/.agents/skills/`（兼容 `~/.claude/skills/`、`~/.codex/skills/`） | 子代理 `~/.cursor/agents/`；命令 `~/.cursor/commands/`（已并入 skills）；本地插件 `~/.cursor/plugins/local/` |
| **Cursor CLI（cursor-agent）** | 无官方全局规则文件；全局配置 `~/.cursor/cli-config.json`（Win `%USERPROFILE%\.cursor\cli-config.json`；`CURSOR_CONFIG_DIR` 覆盖） | 与编辑器同一套：`.cursor/rules/`、`AGENTS.md` | 共享 `~/.cursor/skills/` 体系 | 项目配置 `<项目>/.cursor/cli.json` |
| **Windsurf（Codeium / Devin Desktop）** | `~/.codeium/windsurf/memories/global_rules.md`（6000 字符上限，Always-On）；自动记忆 `~/.codeium/windsurf/memories/` | `.devin/rules/*.md`（首选）→ `.windsurf/rules/*.md`（回退）；旧 `.windsurfrules` 仍读；`AGENTS.md`（根=always-on，子目录=auto-glob） | `~/.codeium/windsurf/skills/<名>/SKILL.md`（兼容 `~/.agents/skills/`、`.agents/skills/`） | 全局工作流 `~/.codeium/windsurf/global_workflows/`；企业系统级 `/etc/devin/rules/`（Linux）等 |
| **Cline** | 全局规则目录 `~/Documents/Cline/Rules`（Win `Documents\Cline\Rules`；找不到时试 `~/Cline/Rules`）；另读 `~/.agents/AGENTS.md` | `.clinerules/`（支持 `paths:` 条件）；自动检测 `.cursorrules`、`.windsurfrules`、`AGENTS.md` | `~/.cline/skills/`（Win `C:\Users\<用户名>\.cline\skills\`） | 全局工作流 `~/Documents/Cline/Workflows`；CLI 全局 MCP `~/.cline/mcp.json` |
| **Roo Code** | `~/.roo/rules/` 与 `~/.roo/rules-{模式}/`（Win `%USERPROFILE%\.roo\rules\`） | `.roo/rules/`（首选）→ 回退 `.roorules`；`.roo/rules-{模式}/` → `.roorules-{模式}`；根目录 `AGENTS.md`/`AGENT.md` | 未找到官方文档说明 | 全局自定义模式 `custom_modes.yaml`（扩展存储） |
| **Continue.dev** | `~/.continue/config.yaml`；`~/.continue/` 下的 `rules/`、`prompts/`、`agents/`、`models/`、`mcpServers/` 全局生效 | `.continue/rules/*.md`（`globs/alwaysApply` frontmatter）；`.continue/prompts/`（`invokable: true`） | 无独立 skills 目录（rules/prompts/agents 体系） | 见左 |
| **Zed** | `~/.config/zed/AGENTS.md`（Win `%APPDATA%\Zed\AGENTS.md`；v1.4.0 起 Rules Library 并入 Skills） | 按序取首个匹配：`.rules`→`.cursorrules`→`.windsurfrules`→`.clinerules`→`.github/copilot-instructions.md`→`AGENT.md`→`AGENTS.md`→`CLAUDE.md`→`GEMINI.md` | `~/.agents/skills/`（项目 `<worktree>/.agents/skills/`） | — |
| **Augment Code（含 Auggie CLI）** | `~/.augment/rules/`（恒 always_apply）；`~/.augment/user-guidelines.md`（VS Code） | `.augment/rules/`；旧 `.augment-guidelines`；CLI 优先级 `--rules`→`CLAUDE.md`→`AGENTS.md`→`.augment-guidelines`→`.augment/rules/`→`~/.augment/rules/` | `~/.augment/skills/`（最高优先级；兼容 `~/.claude/skills/`、`~/.agents/skills/`） | 命令 `~/.augment/commands/`（兼容 `~/.claude/commands/`、`~/.agents/commands/`） |
| **VS Code GitHub Copilot（编辑器内置）** | `~/.copilot/instructions/`、`~/.claude/rules/`、`~/.claude/CLAUDE.md`（均兼容读取）；用户 prompts 存 `<用户数据目录>/User/prompts/`（Win `%APPDATA%\Code\User\prompts`、macOS `~/Library/Application Support/Code/User/prompts`、Linux `~/.config/Code/User/prompts`） | `.github/copilot-instructions.md`；`AGENTS.md`（嵌套为实验特性）；`CLAUDE.md`；`.github/instructions/*.instructions.md` | `~/.copilot/skills/`、`~/.claude/skills/`、`~/.agents/skills/` | 用户级代理 `~/.copilot/agents/`；项目 `.github/agents/`、`.github/prompts/` |
| **Void（开源，已暂停开发）** | 未找到官方文档说明 | 项目根 `.voidrules` | 无 | 无 |
| **PearAI** | 无独立机制（聊天为 Continue fork，继承 `~/.continue/` 体系）；Memory 为 Mem0 云端 | 继承 `.continue/rules/*.md`（第三方称 `.pearairules`，未证实） | 未找到官方文档说明 | — |
| **Zencoder** | 「Instructions for AI」UI 管理（路径未公开） | `.zencoder/rules/*.md` + `repo.md` | — | — |

### 4. 大厂自研 Agent（腾讯 / 阿里 / 字节 / 百度 / 京东 / 华为 / AWS / Google / JetBrains）

| 工具 | 用户级（全局）规则/记忆文件 | 项目级规则文件 | 全局 Skill 目录 | 全局其他目录 |
|---|---|---|---|---|
| **腾讯 CodeBuddy（CLI）** | `~/.codebuddy/CODEBUDDY.md`（用户记忆）；`~/.codebuddy/rules/*.md`（用户规则，递归）；自动记忆 `~/.codebuddy/memories/global/` 与 `~/.codebuddy/memories/{项目id}/` | `./CODEBUDDY.md` 或 `./.codebuddy/CODEBUDDY.md`（向上递归；缺省回退 AGENTS.md）；`./CODEBUDDY.local.md`；`.codebuddy/rules/*.md` | `~/.codebuddy/skills/` | `~/.codebuddy/agents/`、`~/.codebuddy/commands/`（项目级）；`~/.codebuddy/settings.json`、`~/.codebuddy/.mcp.json` |
| **腾讯 CodeBuddy（IDE）** | 用户规则全局生效，UI 管理（官方未给磁盘路径） | `.codebuddy/rules/<规则名>/RULE.mdc`；项目根 `CODEBUDDY.md` | 面板管理 | — |
| **阿里 Qoder（IDE + CLI）** | `~/.qoder/AGENTS.md`（用户级记忆）；`~/.qoder/rules/**/*.md`（用户级规则）；自动记忆 `~/.qoder/memory/`、`~/.qoder/projects/<项目>/memory/` | `AGENTS.md`、`AGENTS.local.md`、`.qoder/rules/**/*.md`（向上查找；冲突时 rules 优先） | `~/.qoder/skills/` ↔ 项目 `.qoder/skills/`（兼容 `.agents/skills/`） | 子代理 `~/.qoder/agents/` ↔ `.qoder/agents/`；命令项目级 `.qoder/commands/` |
| **阿里通义灵码（Lingma）** | 官方未公布全局规则路径（社区称 `~/.lingma/global.md`，未证实） | `.lingma/rules/*.md`（仅当前工程） | 第三方称 `~/.lingma/skills/` ↔ `.lingma/skills/`（非官方来源） | 企业版自定义指令控制台云端管理 |
| **QoderWork CN（阿里云桌面产品）** | 未找到可靠资料 | 未找到可靠资料 | `~/.qoderwork/skills/`（阿里云官方帮助文档） | — |
| **字节 Trae（国际版/国内版）** | 个人规则文件名 `user_rules.md`（官方论坛确认唯一识别名；UI 创建）；社区证据指向 `~/.trae/`（中等置信）；应用数据目录 `%APPDATA%\Trae CN` 等 | `.trae/rules/project_rules.md`（支持嵌套子目录）；项目根 `AGENTS.md`/`CLAUDE.md` 也识别 | `~/.trae/skills/`（Win `%USERPROFILE%\.trae\skills\`）↔ 项目 `.trae/skills/`（兼容 `.agents/skills/`） | 项目 MCP `.trae/mcp.json` |
| **百度 Comate（文心快码）** | v4.2.0+ 支持全局 Rules（跨 workspace），但官方未公布磁盘路径（个人知识库面板管理） | `.comate/rules/*.mdr`（兼容读取 `.cursor/rules/*.mdc`） | 官方未公布 | — |
| **京东 JoyCode** | 用户规则 UI 管理（官方未给磁盘路径）；长期记忆在「设置-记忆」 | `.joycode/rules/`（mdc 格式）；自定义智能体 `.joycode/agents/` | 官方未公布；第三方指向 `~/.joycode/skills/` ↔ `.joycode/skills/`（未证实）；**无官方独立 CLI** | 项目级 `.joycode/commands/`、`.joycode/mcp-configs/`（第三方佐证） |
| **华为码道 CodeArts（原 Snap）** | 支持企业/团队/个人三级 Rules/Skills 管理，路径未公开 | 未找到可靠资料 | 路径未公开 | — |
| **JetBrains Junie** | `~/.junie/AGENTS.md`（Win `%USERPROFILE%\.junie\AGENTS.md`） | `.junie/AGENTS.md` → 根 `AGENTS.md` → 旧 `.junie/guidelines.md` / `.junie/guidelines/` | 项目级 `.junie/skills/`（用户级 skills 未单列） | `~/.junie/mcp.json`、`~/.junie/allowlist.json` |
| **JetBrains AI Assistant** | 全局规则存 IDE 配置目录（社区路径：Linux `~/.config/JetBrains/<IDE>/ai-assistant/rules/` 等，非官方） | `.aiassistant/rules/*.md` | — | — |
| **Google Antigravity** | `~/.gemini/GEMINI.md` | `.agents/rules/*.md`（2.0 默认；兼容 `.agent/rules`） | `~/.gemini/config/skills/<名>/` ↔ 项目 `.agents/skills/` | 工作流官方未写路径（社区称 `~/.gemini/antigravity/global_workflows/`） |
| **Google Jules** | 无（云端异步 agent，按仓库网页端 setup script） | 仓库根 `AGENTS.md` | 无 | 无 |
| **Amazon Q Developer CLI（已归档，后继为 Kiro CLI）** | 无全局规则目录；Prompt 库 `~/.aws/amazonq/prompts/*.md` | `.amazonq/rules/**/*.md`；CLI 默认 context 自动读 `AmazonQ.md`、`AGENTS.md`、`README.md` | — | 自定义代理 `~/.aws/amazonq/cli-agents/*.json`；MCP `~/.aws/amazonq/mcp.json` |
| **AWS Kiro（IDE + CLI）** | `~/.kiro/steering/*.md`（全局 steering，可放全局 AGENTS.md） | `.kiro/steering/*.md`（always/fileMatch/manual）；根目录 `AGENTS.md` | `~/.kiro/skills/<名>/SKILL.md` ↔ 项目 `.kiro/skills/` | 代理 `~/.kiro/agents/`；Powers `~/.kiro/powers/installed/<名>/`；MCP `~/.kiro/settings/mcp.json`；hooks 官方仅项目级 `.kiro/hooks/` |
| **智谱 CodeGeeX / AutoGLM** | 无 markdown 规则文件机制（设置面板/云端） | — | — | — |

### 5. 云端 / Web 系 Agent（无本地文件机制为主）

| 工具 | 用户级（全局）规则/记忆 | 项目级规则 | 全局 Skill 机制 |
|---|---|---|---|
| **ChatGPT（web/桌面 agent）** | 无文件；Settings → Personalization → Custom Instructions（1500 字符，账户云端存储） | — | 经 plugins 分发（桌面 Work mode/Codex 走 `~/.codex/` 约定） |
| **Codex Cloud（chatgpt.com/codex）** | 无用户级文件（设置页 Custom instructions 输入框） | 仓库内 `AGENTS.md` | — |
| **Claude Desktop** | 无文件（应用内偏好，账户云端）；MCP 配置 macOS `~/Library/Application Support/Claude/claude_desktop_config.json`、Win `%APPDATA%\Claude\claude_desktop_config.json`（无 Linux 官方版） | — | 账户云端同步（不读 `~/.claude/skills/`） |
| **Warp 终端（2.0 / Oz）** | 无本地文件；Warp Drive 云端 Personal → Rules | `AGENTS.md`（首选，`WARP.md` 兼容别名） | 本地扫描：`~/.agents/skills/`（推荐）+ `~/.warp/skills/`、`~/.claude/skills/`、`~/.codex/skills/`、`~/.cursor/skills/`、`~/.gemini/skills/`、`~/.copilot/skills/`、`~/.factory/skills/`、`~/.github/skills/`、`~/.opencode/skills/` |
| **Devin（Cognition）** | 云端 Knowledge（Settings & Library，支持 `!macro`、pin repo） | 自动扫描仓库 `AGENTS.md`、`CLAUDE.md`、`.cursorrules`、`.windsurf`、`.rules`、`.mdc` | 无本地目录 |
| **Replit Agent** | 云端 Workspace Settings → Customization（Custom Instructions + 云端 Skills） | 项目根 `replit.md` | 云端（workspace 级，非本地文件） |
| **Bolt.new** | 云端 Settings → Knowledge（Account knowledge） | Project Knowledge + 项目内 `agents.md`；旧 v1 为 `.bolt/prompt`（已弃用） | 无 |
| **v0（Vercel）** | 云端账户 Instructions | Project → Knowledge（无仓库文件机制） | 无 |
| **Tabnine** | 云端 Admin Console → Context Engine → Coaching Guidelines（组织级，可 CSV 导入） | 未找到官方文档说明（第三方称 `.tabnine/guidelines/`，未证实） | 无本地目录 |

---

## 二、关键结论与使用建议

### 1. 两大事实标准已经收敛
- **AGENTS.md**：已成为 Linux Foundation 旗下 Agentic AI Foundation 的开放标准（60,000+ 项目采用），Codex、OpenCode、Gemini CLI（可配置）、Copilot、Cursor、Zed、Kiro、Jules、Devin、Qoder、Kimi 等几乎全部读取；**唯独 Claude Code 不原生读 AGENTS.md**（官方建议在 CLAUDE.md 里写 `@AGENTS.md` 导入）。
- **`~/.agents/skills/`**：2026 年已成跨工具通用全局技能目录，Codex、Copilot、Cursor、Zed、Kimi Code CLI、OpenCode、Crush、Grok CLI、Warp、Windsurf、OpenHands 等均扫描它。想"一处维护、处处生效"就放这里；各厂商私有目录（`~/.claude/skills/`、`~/.qwen/skills/` 等）用于专属技能。

### 2. 路径设计的两大流派
- **XDG 派**：`~/.config/<工具>/`（OpenCode、Amp、Crush、Kilo、Zed、aichat 等，符合 Linux 惯例）。
- **点目录派**：`~/.<工具>/`（Claude、Codex、Gemini、Qwen、Kiro、Factory、CodeBuddy、Qoder、Trae、iFlow 等，CLI 工具主流）。
- Windows 上绝大多数 CLI 沿用同一写法（`~` 展开为 `%USERPROFILE%`），只有 Claude Desktop、VS Code、aichat、mods 等少数走 `%APPDATA%` / `%LOCALAPPDATA%`。

### 3. 加载优先级通用规律
几乎全部工具遵循：**项目级（越靠近 cwd 越优先）> 用户级全局 > 系统/企业级默认**；规则目录普遍支持 frontmatter（`alwaysApply` / `globs` / `paths` / `description`）做条件触发；自动记忆（auto memory）正成为标配（Claude Code、Qwen、CodeBuddy、Qoder、Windsurf 均有 `memory/` 目录机制）。

### 4. 主要冲突与存疑点（引用时注意）
| 事项 | 说明 |
|---|---|
| Kimi 双版本 | 旧 kimi-cli 用 `~/.kimi/`；新 Kimi Code CLI 用 `~/.kimi-code/`（`$KIMI_CODE_HOME`），帮助中心旧页面与正式文档并存 |
| Codex skills 目录 | 旧实验文档写 `~/.codex/skills/`，现行官方文档为 `~/.agents/skills/` |
| Copilot agents 同名优先级 | 官方两份文档互相矛盾（项目级 vs 用户级优先），建议实测 |
| OpenHands 全局 skills | 官方两处不一致：`~/.openhands/skills/` vs `~/.agents/skills/` |
| Trae / 灵码 / Comate / JoyCode 全局规则磁盘路径 | 官方均未公开，表中社区路径已逐一标注置信度 |
| JoyCode MCP | 用户级已钉死为 `~/.joycode/joycode-mcp.json`（扩展源码）；规则文件磁盘路径仍未公开。见 [agent_runtime_mcp_paths.md](agent_runtime_mcp_paths.md) |
| joycode-cli | 不存在官方独立 CLI，JoyCode 仅 IDE + VS Code 插件形态 |
| Grok CLI | xAI 无官方 CLI，表中为主流社区实现 superagent-ai/grok-cli |
| Claude Code `~/.claude/commands/` | 命令已并入 skills，新版文档未单列用户级命令目录（旧文档曾明确，高置信仍存在） |
| Cursor `~/.cursor/rules/` 注入 | 路径与 Settings 展示已确认；Agent 自动注入在 Agents Window / home workspace 下不可靠（Settings 可见、提示词无正文）。另有 `alwaysApply: true` 被降成 requestable 的客户端 bug（社区称 3.2 修复）。验证：新开 Agent 问能否复述规则正文，勿仅看 Settings。可靠 workaround：打开具体项目 workspace；或把关键段贴进 Settings → User Rules；或聊天 `@` 该 `.mdc` |

## 三、调研方法与来源
- 方法：6 路并行调研（Anthropic/OpenAI 系、开源 CLI、IDE 系、大厂自研、新兴独立、中文社区交叉验证），累计 200+ 次检索，全部关键路径以官方文档原文核实（code.claude.com、developers.openai.com、docs.github.com、opencode.ai、cursor.com、docs.windsurf.com、kiro.dev、codebuddy.ai、docs.qoder.com、kimi.com/code/docs、joycode.jd.com 等）。
- 各工具逐条来源 URL 见调研底稿：`/mnt/agents/output/research/agent_runtime_wideA.md` ~ `wideF.md`。
