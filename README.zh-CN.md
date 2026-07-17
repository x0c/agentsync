# agentsync

[English](README.md) | 简体中文

`agentsync` 是一个小型 Go CLI，用来把 AI coding agent 的全局指令和可复用 Skill 统一收敛到一个源目录。

它解决 Codex、Claude Code、OpenCode、Grok、Kimi Code 各自维护 `AGENTS.md`、`CLAUDE.md` 和 `SKILL.md` 目录导致内容漂移的问题。

## 管理对象

统一指令文件：

```text
~/.config/agentsync/AGENTS.md
```

工具侧指令入口：

```text
~/.codex/AGENTS.md
~/.config/opencode/AGENTS.md
~/.claude/CLAUDE.md
~/.grok/AGENTS.md
~/.kimi-code/AGENTS.md
~/.agents/AGENTS.md
```

统一 Skill 目录：

```text
~/.config/agentsync/skills/<skill-name>/SKILL.md
```

工具侧 Skill 入口：

```text
~/.claude/skills -> ~/.config/agentsync/skills
~/.codex/skills -> ~/.config/agentsync/skills
~/.config/opencode/skill -> ~/.config/agentsync/skills
~/.grok/skills -> ~/.config/agentsync/skills
~/.kimi-code/skills -> ~/.config/agentsync/skills
~/.agents/skills -> ~/.config/agentsync/skills
```

每个 Skill 在统一 Skill 根目录下按完整目录管理。目录内必须包含 `SKILL.md`，旁边的脚本、模板、参考资料和资源文件会一起保留。由于工具侧 Skill 根目录整体指向统一根目录，新增、删除或重命名统一源中的 Skill 会立即反映到所有工具侧。

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

一键收敛全局指令和 Skill：

```bash
agentsync
```

`agentsync` 设计为幂等命令。第一次完成收敛后，后续重复运行应看到托管入口都是 `ok`。

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
