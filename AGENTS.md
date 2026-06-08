@~/.config/agentsync/AGENTS.md

# agentsync 项目规范

本项目遵循全局 Agent 规范；本文件补充 agentsync 项目的技术栈、命令和文档入口。

## 项目定位

agentsync 是一个 Go CLI，用于把多个 AI coding agent 的全局指令文件和 Agent Skill 目录收敛到统一源：

- 全局规范源：`~/.config/agentsync/AGENTS.md`
- 全局 Skill 源：`~/.config/agentsync/skills/`
- 工具入口：Codex、Claude Code、OpenCode 的指令文件和 skill 目录软链接

## 技术栈

- Go 1.25+
- 仅使用 Go 标准库
- 单 binary CLI，安装命令为 `brew install --cask x0c/tap/agentsync` 或 `go install .`
- tag 发布使用 GoReleaser，并更新 `x0c/homebrew-tap`

## 常用命令

```bash
go test ./...
go build ./...
go install .
agentsync --check
agentsync
goreleaser check
```

## 文档导航

- [docs/AGENTSYNC_KNOWLEDGE_BASE.md](docs/AGENTSYNC_KNOWLEDGE_BASE.md) —— 核心概念、目录模型、同步对象与安全边界
- [docs/AGENTSYNC_GUIDE.md](docs/AGENTSYNC_GUIDE.md) —— 安装、检查、收敛与验证命令
- [README.md](README.md) —— 面向用户的简明说明

## 改动注意事项

- 修改默认路径、软链接策略、备份策略或 skill 同步策略时，同步更新 `README.md`、[docs/AGENTSYNC_KNOWLEDGE_BASE.md](docs/AGENTSYNC_KNOWLEDGE_BASE.md) 和 [docs/AGENTSYNC_GUIDE.md](docs/AGENTSYNC_GUIDE.md)。
- 改动同步逻辑后必须跑 `go test ./...`，涉及真实机器状态时再跑 `agentsync --check`。
- 功能性改动完成并验证通过后，必须执行 `go install .` 重新安装本地 CLI；需要正式发布时按 [docs/AGENTSYNC_GUIDE.md](docs/AGENTSYNC_GUIDE.md) 的发布流程创建并推送 `v*` tag。
- 测试必须使用隔离配置目录，避免向真实 `~/.config/agentsync/backups/` 写测试备份。
