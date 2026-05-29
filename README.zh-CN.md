# agentsync

[English](README.md) | 简体中文

`agentsync` 是一个小型 Go CLI，用来把 AI coding agent 的全局指令和可复用 Skill 统一收敛到一个源目录。

它解决 Codex、Claude Code、OpenCode 各自维护 `AGENTS.md`、`CLAUDE.md` 和 `SKILL.md` 目录导致内容漂移的问题。

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
```

统一 Skill 目录：

```text
~/.config/agentsync/skills/<skill-name>/SKILL.md
```

工具侧 Skill 入口：

```text
~/.claude/skills/<skill-name>
~/.codex/skills/<skill-name>
~/.config/opencode/skill/<skill-name>
```

每个 Skill 按完整目录管理。目录内必须包含 `SKILL.md`，旁边的脚本、模板、参考资料和资源文件会一起保留。

## 安装

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
- 已有 Skill 目录会先复制到统一 Skill 目录，再创建软链接。
- 替换前的文件和目录会备份到 `~/.config/agentsync/backups/`。
- Codex `.system` 这类隐藏内部 Skill 目录不会导入或替换。
- macOS 和 Linux 优先使用软链接。
- Windows 优先尝试软链接，再尝试硬链接，最后退化为带标记的托管副本。

## 开发

项目文档入口：[docs/OVERVIEW.md](docs/OVERVIEW.md)。

```bash
go test ./...
go build ./...
agentsync --check
```

## 许可证

MIT
