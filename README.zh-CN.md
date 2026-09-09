**语言：** [English](README.md) | 简体中文

# agentsync

**换 AI 编程工具，不用重配规则、技能和 MCP。**

agentsync 将 Claude Code、Codex、Cursor、Gemini CLI 等工具的全局指令、Skill 和 MCP 服务器配置统一管理。只维护一份源文件，用一条命令同步到已安装的工具。

[![CI](https://github.com/x0c/agentsync/actions/workflows/ci.yml/badge.svg)](https://github.com/x0c/agentsync/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/x0c/agentsync)](https://github.com/x0c/agentsync/releases/latest) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 为什么用 agentsync？

你在 Claude Code 里改了编码规则，切到 Codex 又要复制一次；新增一个 Skill 或 MCP 服务器，还要逐个配置其他工具。agentsync 把这些重复操作集中起来。

| 你维护一次 | agentsync 负责 |
| --- | --- |
| `AGENTS.md` 全局指令 | 连接或生成各工具的规范入口 |
| `skills/` 完整技能目录 | 共享技能正文、脚本与参考资料 |
| `mcp.json` 服务器配置 | 转换为各工具需要的配置格式 |

单个可执行文件，无需 Node.js 或 Python。替换前保留备份；未安装的工具会跳过。各工具支持的能力不同，详见下方路径清单。

如果它让你少维护了几份配置，欢迎点个 Star，方便下次找到。

## 安装

### macOS：Homebrew

```bash
brew install --cask x0c/tap/agentsync
```

### Linux / Windows / macOS：Go

已安装 Go 1.25 或更新版本时，三个平台都可执行：

```sh
go install github.com/x0c/agentsync@latest
```

请将 Go 的安装目录（通常为 `~/go/bin`，Windows 为 `%USERPROFILE%\go\bin`）加入 PATH，重新打开终端。

### 无需 Go：下载可执行文件

从 [最新 Release](https://github.com/x0c/agentsync/releases/latest) 下载并解压对应文件，将 `agentsync`（Windows 为 `agentsync.exe`）放入 PATH 中的目录。同一页面提供 `checksums.txt` 校验文件。

| 系统 | 已发布的架构 |
| --- | --- |
| macOS | Apple Silicon（arm64）、Intel（amd64） |
| Linux | arm64、amd64 |
| Windows | amd64（x64） |

## 首次使用

```bash
agentsync --check  # 先预览，不修改文件
agentsync          # 合并已有内容、备份并同步
agentsync --check  # 检查已安装工具的入口
```

此后只编辑 `~/.config/agentsync/` 中的源文件。规范和 Skill 优先通过链接共享；MCP 配置需要再次运行同步，或使用下方监听模式。

下面是 Linux 上已发布版本的真实输出节选，临时用户目录缩写为 `~`：

```text
$ agentsync --check
  mergeable    ~/.codex/AGENTS.md (would merge unique content and replace with alias)
  mergeable    ~/.claude/CLAUDE.md (would merge unique content and replace with alias)
$ agentsync
  merged       ~/.codex/AGENTS.md (content already present; symlink)
  merged       ~/.claude/CLAUDE.md (content already present; symlink)
$ agentsync --check
  ok           ~/.codex/AGENTS.md (symlink)
  ok           ~/.claude/CLAUDE.md (symlink)
```

## 持续同步

不想每次手跑的话，开监听：

```bash
agentsync --watch
```

它会轮询统一源 `AGENTS.md`、`mcp.json`、`skills/`，以及各工具主目录是否出现。改统一源或新装了一个 agent，就会自动把副本写回去。MCP 配置不能软链接，所以靠这个保持同步。`--watch` 不能和 `--check`、`--repo`、`--all`、`--adopt` 一起用。

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
- MCP 服务器会先导入 `~/.config/agentsync/mcp.json`（同名时已安装工具按清单顺序先到先得），再按各工具 schema 覆盖已安装入口。`--repo` 与 `--all` 不同步 MCP。
- 替换前的文件和目录会备份到 `~/.config/agentsync/backups/`。
- Codex `.system` 这类隐藏内部 Skill 目录会先保留到统一 Skill 根目录，再替换工具侧 Skill 根目录。
- macOS 和 Linux 优先使用软链接。
- Windows 优先尝试软链接，再尝试硬链接，最后退化为带标记的托管副本。

## 只处理已安装的工具

每个受支持的工具都用它自己的主目录（如 `~/.codex`、`~/.gemini`、`~/.joycode`）做安装判断。如果该目录不存在，agentsync 视为该工具**未安装**，报告为 `skipped`，绝不会为你没用的工具创建目录或入口文件。装了新工具后跑一次 `agentsync`（或开着 `--watch`）就会收敛。

## 管理对象

<details>
<summary>展开规范、Skill 和 MCP 路径清单</summary>


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

每个 Skill 在统一 Skill 根目录下按完整目录管理。目录内必须包含 `SKILL.md`，旁边的脚本、模板、参考资料和资源文件会一起保留。由于工具侧 Skill 根目录整体指向统一根目录，新增、删除或重命名统一源中的 Skill 会立即反映到所有工具侧。

统一 MCP 配置：

```text
~/.config/agentsync/mcp.json
```

全局模式（不是 `--repo` / `--all`）会把该文件按各已安装工具的 schema 写进用户级 MCP 入口。`~/.claude.json`、`~/.codex/config.toml` 这类混杂热文件只改 MCP 那个 key；`~/.cursor/mcp.json` 这类独立 MCP 文件整段覆盖。不同步 iFlow，也不为 `~/.agents` 造 MCP 入口。请只改统一源——agentsync 会在 `~/.config/agentsync/AGENTS.md` 里注入提醒。若配置目录本身是 git 仓库，会把 `mcp.json` 写入 `.gitignore`（里面常有 token）。


</details>

## 常见问题

**会修改我的项目吗？** 默认只处理用户级配置。当前项目的 `AGENTS.md` / `CLAUDE.md` 需要单独运行仓库模式。

**只是建软链接吗？** 规范与技能会尽量用链接；MCP 则按工具格式转换，并保留混合配置文件中不相关的设置。完整的替换与备份规则见上方安全策略。

**会跨电脑同步吗？** agentsync 负责本机工具之间的配置统一，不是云同步服务。MCP 文件可能含令牌和本机路径，应保留在本机。

**适合团队规则生成吗？** agentsync 主要面向多助手用户的全局配置，仓库模式只管理 `CLAUDE.md` 入口。需要复杂的项目级规则生成时，可同时了解 [Ruler](https://github.com/intellectronica/ruler) 和 [Rulesync](https://github.com/dyoshikawa/rulesync)。

**发现问题或缺少工具支持？** [提交 Issue](https://github.com/x0c/agentsync/issues)，附上工具名称、系统和脱敏后的检查结果；请勿上传含令牌的 MCP 配置。支持路径与行为说明见 [使用指南](docs/AGENTSYNC_GUIDE.md)。

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
