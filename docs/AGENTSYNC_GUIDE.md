# agentsync 使用指南

## 安装

在项目根目录执行：

```bash
go install .
```

安装后命令位于 Go 用户 bin 目录，当前机器是：

```text
$(go env GOPATH)/bin/agentsync
```

## 检查

只检查状态，不修改文件：

```bash
agentsync --check
```

所有规范文件和 skill 目录收敛后，应看到 `Results` 和 `Skills` 都是 `ok`。

## 一键收敛

执行：

```bash
agentsync
```

该命令会：

- 创建或更新 `~/.config/agentsync/AGENTS.md`
- 创建或更新 `~/.config/agentsync/skills/`
- 将 Codex、Claude Code、OpenCode 的规范文件替换成软链接
- 将 Codex、Claude Code、OpenCode 的用户 skill 目录替换成软链接
- 写入替换前备份

## 项目级收敛

在 Git 仓库内执行：

```bash
agentsync --repo
```

该命令使用当前仓库的 `AGENTS.md` 作为项目级源文件，并管理：

```text
CLAUDE.md -> AGENTS.md
```

## 批量仓库收敛

扫描目录下所有 Git 仓库：

```bash
agentsync --all ~/Codes
```

## 验证

代码改动后至少执行：

```bash
go test ./...
go build ./...
agentsync --check
```
