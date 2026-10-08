**语言：** [English](README.md) | 简体中文

# agentsync

**一条命令，统一管理 AI 工具的规则、Skills 和 MCP 配置。**

Claude Code、Codex、Cursor、Pi、OpenCode 等 20 多款工具，本机只维护一份配置。轻量，单个可执行文件。

[![CI](https://github.com/x0c/agentsync/actions/workflows/ci.yml/badge.svg)](https://github.com/x0c/agentsync/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/x0c/agentsync)](https://github.com/x0c/agentsync/releases/latest) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

![运行一次 agentsync，同步规则、Skills 和 MCP](docs/images/demo.gif)

*Claude Code 与 Codex 的真实运行演示；为便于阅读，仅展示部分输出。*

## 安装

**macOS — Homebrew：**

```sh
brew install --cask x0c/tap/agentsync
```

**macOS / Linux / Windows — Go 1.25+：**

```sh
go install github.com/x0c/agentsync@latest
```

如有需要，将 Go 的可执行文件目录加入 PATH（通常为 `~/go/bin`；Windows 为 `%USERPROFILE%\go\bin`）。

**没有 Go：** 下载并解压 [Release 中的可执行文件](https://github.com/x0c/agentsync/releases/latest)，放到 PATH 中即可。支持 macOS 和 Linux（arm64 / amd64），以及 Windows（amd64），附带校验文件。

## 使用

```sh
agentsync
```

就这一条。它会合并已有规则、收集完整的 Skill 文件夹，并导入 MCP 服务器配置，统一到一个公共目录，再为已安装的工具建立链接或写入对应格式的配置。替换前自动备份，未安装的工具自动跳过。

此后，只需维护 `~/.config/agentsync/` 中的这三项：

| 文件 | 只维护一份的内容 |
|---|---|
| `AGENTS.md` | 全局规则 |
| `skills/` | 完整的 Skill 文件夹，包含脚本与参考资料 |
| `mcp.json` | MCP 服务器配置 |

修改后再运行一次 `agentsync`，也可以使用下方的自动同步。

## 按需使用

| 需求 | 命令 |
|---|---|
| 只预览，不修改文件 | `agentsync --check` |
| 自动同步公共目录中的改动 | `agentsync --watch` |
| 还原最近一次备份 | `agentsync --rollback latest` |
| 将项目的 `CLAUDE.md` 链接到 `AGENTS.md` | `agentsync --repo` |

后台服务、批量同步仓库、按工具过滤与备份详情，见 [使用指南](docs/AGENTSYNC_GUIDE.md)。

## 几点说明

- 默认管理**本机的全局配置**；项目文件使用单独的仓库模式。
- 各工具支持的能力不同，详见 [规则与 Skill 支持](docs/agent_runtime_global_paths.md) 和 [MCP 支持](docs/agent_runtime_mcp_paths.md)。
- 规则与 Skill 尽量使用链接；Cursor 使用受管的 `.mdc` 规则，Windows 可退回托管副本。另见 [Cursor 规则加载限制](docs/AGENTSYNC_GUIDE.md#cursor-rules-visible-in-settings-but-absent-from-the-prompt)。
- 请修改公共目录中的源文件。自动同步只向各工具写入，不会导入工具设置界面的改动。`mcp.json` 可能含令牌与本机路径，请保留在本机。

遇到问题或希望支持新工具？[提交 Issue](https://github.com/x0c/agentsync/issues)，附上工具名称、系统和脱敏输出。贡献规范见 [AGENTS.md](AGENTS.md)。

## 许可证

[MIT](LICENSE)
