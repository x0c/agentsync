# agentsync

[English](README.md) | 简体中文

`agentsync` 是一个小型 Go CLI，用来把 AI coding agent 的全局指令、可复用 Skill 和 MCP 服务器配置统一收敛到一个源目录。

它解决 Codex、Claude Code、OpenCode、Gemini CLI、Qwen Code、Copilot CLI、Kimi Code、Grok、Amp、Crush、Goose、Factory Droid、iFlow、Kilo、Pi、Cursor、Windsurf、Zed、CodeBuddy、Qoder、Junie、Kiro、JoyCode 等众多工具各自维护 `AGENTS.md`、`CLAUDE.md` 和 `SKILL.md` 目录导致内容漂移的问题。

## 只处理已安装的工具

每个受支持的工具都用它自己的主目录（如 `~/.codex`、`~/.gemini`、`~/.joycode`）做安装判断。如果该目录不存在，agentsync 视为该工具**未安装**，报告为 `skipped`，绝不会为你没用的工具创建目录或入口文件。装了新工具后跑一次 `agentsync`（或开着 `--watch`）就会收敛。

## 管理对象

统一指令文件：

```text
~/.config/agentsync/AGENTS.md
```

工具侧指令入口（仅在对应工具已安装时创建）：

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
~/.pi/agent/AGENTS.md
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

Cursor 入口是带 `alwaysApply: true` frontmatter 的受管 `.mdc`（不是裸 symlink）——Cursor 会忽略没有 frontmatter 的规则文件。若 Settings 已列出该规则但 Agent 会话复述不出正文，多半是 Cursor 在 home workspace / Agents Window 下未注入 file-backed 规则；请打开具体项目再试，或把关键段贴进 Settings → User Rules（详见 `docs/AGENTSYNC_GUIDE.md`）。

统一 Skill 目录：

```text
~/.config/agentsync/skills/<skill-name>/SKILL.md
```

工具侧 Skill 入口（仅在对应工具已安装时创建），均整体指向 `~/.config/agentsync/skills`：

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

每个 Skill 在统一 Skill 根目录下按完整目录管理。目录内必须包含 `SKILL.md`，旁边的脚本、模板、参考资料和资源文件会一起保留。由于工具侧 Skill 根目录整体指向统一根目录，新增、删除或重命名统一源中的 Skill 会立即反映到所有工具侧。Pi 原生读 `~/.agents/skills/`，因此不建专属 Skill 别名。

统一 MCP 配置：

```text
~/.config/agentsync/mcp.json
```

全局模式（不是 `--repo` / `--all`）会把该文件按各已安装工具的 schema 写进用户级 MCP 入口。`~/.claude.json`、`~/.codex/config.toml` 这类混杂热文件只改 MCP 那个 key；`~/.cursor/mcp.json` 这类独立 MCP 文件整段覆盖。不同步 iFlow，也不为 `~/.agents` 造 MCP 入口。Codex 捆绑的本机服务器（`node_repl`、`computer-use`）只留在 Codex。请只改统一源——agentsync 会在 `~/.config/agentsync/AGENTS.md` 里注入提醒。`mcp.json` 是本机文件（常有 token 和本机路径），配置目录若是 git 仓库或 Syncthing 文件夹，会写入 `.gitignore` / `.stignore`。不要默认同步到其他机器。

## 安装

使用 Homebrew：

```bash
brew install --cask x0c/tap/agentsync
```

或者使用 Go：

```bash
go install github.com/x0c/agentsync@latest
```

或者在源码目录内：

```bash
go install .
```

## 使用

只检查，不写文件：

```bash
agentsync --check
```

一键收敛全局指令、Skill 和 MCP 配置：

```bash
agentsync
```

`agentsync` 设计为幂等命令。第一次完成收敛后，后续重复运行应看到托管入口都是 `ok`。

不想每次手跑的话，开监听：

```bash
agentsync --watch
```

它会轮询统一源 `AGENTS.md`、`mcp.json`、`skills/`，以及各工具主目录是否出现。改统一源或新装了一个 agent，就会自动把副本写回去。只改规范或 Skill 时不会重写 MCP。MCP 配置不能软链接，所以靠这个保持同步。`--watch` 不能和 `--check`、`--repo`、`--all`、`--adopt`、`--force` 一起用。

Linux 可用 `contrib/systemd/agentsync.service`：

```bash
mkdir -p ~/.config/systemd/user
cp contrib/systemd/agentsync.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now agentsync.service
```

macOS 可拷 `contrib/launchd/top.x0c.agentsync.plist`，把 `ProgramArguments` 改成 `$(which agentsync)` 的路径，再 `launchctl load`。

## 仓库模式

在 Git 仓库内执行：

```bash
agentsync --repo
```

该模式使用仓库内 `AGENTS.md` 作为源文件，并管理：

```text
CLAUDE.md -> AGENTS.md
```

批量处理目录下所有 Git 仓库：

```bash
agentsync --all ~/Codes
```

## 安全策略

- `--check` 只读。
- 已有独特指令内容会先并入统一源文件，再创建软链接。
- 已有 Skill 目录会先复制到统一 Skill 目录，再将工具侧 Skill 根目录替换成软链接。
- MCP 服务器会先导入 `~/.config/agentsync/mcp.json`（同名时已安装工具按清单顺序先到先得，大小写不敏感），再按各工具 schema 覆盖已安装入口。Codex 捆绑的本机服务器不扩散到其他工具。`--repo` 与 `--all` 不同步 MCP。
- 替换前的文件和目录会备份到 `~/.config/agentsync/backups/`。
- Codex `.system` 这类隐藏内部 Skill 目录会先保留到统一 Skill 根目录，再替换工具侧 Skill 根目录。
- macOS 和 Linux 优先使用软链接。
- Windows 优先尝试软链接，再尝试硬链接，最后退化为带标记的托管副本。

## 开发

项目文档入口：[AGENTS.md](AGENTS.md)。

```bash
go test ./...
go build ./...
agentsync --check
```

tag 发布由 GoReleaser 构建。发布 Homebrew cask 前，需要创建 `x0c/homebrew-tap` 仓库，并配置可推送该 tap 的 `HOMEBREW_TAP_GITHUB_TOKEN` secret。

## 许可证

MIT
