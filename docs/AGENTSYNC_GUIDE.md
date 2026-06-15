# agentsync 使用指南

## 安装

推荐使用 Homebrew：

```bash
brew install --cask x0c/tap/agentsync
```

也可以使用 Go 安装最新版本：

```bash
go install github.com/x0c/agentsync@latest
```

在项目根目录执行（本机 binary 安装在 `~/.local/bin/`）：

```bash
go build -o ~/.local/bin/agentsync .
```

> `go install .` 会安装到 `$(go env GOPATH)/bin/`，若该路径不在 PATH 中，改用上方 `go build` 直接指定输出路径。

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
- 将 Codex、Claude Code、OpenCode 的用户 skill 根目录整体替换成指向 `~/.config/agentsync/skills/` 的软链接
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
go install .
agentsync --check
```

功能性改动完成后，验证通过必须重新安装本地 CLI。需要正式发布时，继续按下方发布流程创建并推送 `v*` tag。

## 发布

推送 `v*` tag 后，GitHub Actions 会调用 GoReleaser 构建 GitHub Release 资产，并更新 Homebrew tap 中的 cask：

```bash
git tag v0.1.0
git push origin v0.1.0
```

发布 Homebrew cask 前，需要准备：

- `x0c/homebrew-tap` 仓库。
- `HOMEBREW_TAP_GITHUB_TOKEN` 仓库 secret，token 需要有推送 `x0c/homebrew-tap` 的权限。

发布完成后验证：

```bash
brew update
brew install --cask x0c/tap/agentsync
agentsync --check
```
