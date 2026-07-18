# agentsync 项目规范

agentsync 是一个 Go CLI，用于把多个 AI coding agent 的全局指令文件、项目级指令入口和 Skill 根目录收敛到统一源。项目核心价值是减少 Codex、Claude Code、OpenCode、Gemini、Qwen、Copilot、Kimi Code、Grok、Amp、Crush、Goose、Factory、iFlow、Kilo、Windsurf、Zed、CodeBuddy、Qoder、Junie、Kiro、JoyCode 等众多工具之间的规范漂移，并保留替换前内容，避免一键收敛造成信息丢失。

本仓是工具型项目，不按传统业务域拆文档；文档入口按用户工作流和命令能力组织。

## 项目定位

- 全局规范源：`~/.config/agentsync/AGENTS.md`
- 全局 Skill 源：`~/.config/agentsync/skills/`
- 全局工具入口：覆盖 Codex、OpenCode、Claude、Gemini、Qwen、Copilot、Kimi Code、Grok、Amp、Crush、Goose、Factory、iFlow、Kilo、Windsurf、Zed、CodeBuddy、Qoder、Junie、Kiro、JoyCode 各自的用户级规范文件，外加通用 `~/.agents/AGENTS.md`。完整清单与各自路径见 `internal/agentsync/paths.go` 的 `defaultGlobalConfig()`。
- 全局 Skill 入口：上述工具各自的用户级 skill 根目录（如 `~/.claude/skills`、`~/.codex/skills`、`~/.config/opencode/skills`、`~/.joycode/skills` 等），外加通用 `~/.agents/skills`。
- **按安装门控**：每个入口都带一个 `Detect` 标志目录（该工具的用户级主目录，如 `~/.codex`、`~/.joycode`）。标志目录不存在即视为该工具未安装，同步时报告 `skipped`，绝不为未安装的工具创建任何目录或入口文件。新增 runtime 时必须同时给出正确的 `Detect`。
- 项目级入口：仓库内 `AGENTS.md` 与 `CLAUDE.md`

## 技术栈与约束

- Go 1.25+，仅使用 Go 标准库。
- 单 binary CLI，入口为 `main.go`，核心实现位于 `internal/agentsync/`。
- CLI 输出、错误信息和新增注释按本地规范使用中文；已有英文用户输出属于当前公开接口，改动前需要同步评估 README 与测试。
- 默认优先创建 symlink；Windows 或不支持 symlink 的场景会退化为 hardlink 或受管副本。
- 修改默认路径、别名策略、备份策略、Skill 同步策略或命令参数时，必须同步更新本文档、`README.md`、`README.zh-CN.md` 和 `docs/` 对应文档。
- 测试必须使用隔离配置目录，避免向真实 `~/.config/agentsync/backups/` 写测试备份。

## 验证命令

文档改动只需执行文档导航检查。代码、配置、构建脚本或发布配置改动后执行：

```bash
go test ./...
go build ./...
GOBIN=~/.local/bin go install .   # 见下方说明,不能省略 GOBIN
agentsync --check
goreleaser check
```

- **覆盖本机二进制必须带 `GOBIN=~/.local/bin`**:本机 `agentsync` 实际在 `~/.local/bin/agentsync`(在 PATH 内);裸 `go install .` 会装到 `$GOPATH/bin`(`~/go/bin`,不在 PATH),导致改完代码后 `agentsync --check` 仍跑旧版、看不到新行为。排查“改了没生效”前先确认跑的是哪个二进制。
- `agentsync --check` 是只读检查;涉及真实用户目录的验证前,先确认不会覆盖未备份内容。
- 功能新增 / 缺陷修复验证通过后,按全局规范默认走完整发布流程:提交 → 打 `v*` tag → 推送 → tag 触发 GitHub Actions(GoReleaser)构建产物并自动更新 `x0c/homebrew-tap` 的 cask。用 `gh run list` / `gh release list` 确认远端 Release 成功、cask 版本已跟随。

## 文档导航

> 以下文档在涉及对应领域的开发、评审或排查时先读取。

- [docs/AGENTSYNC_GUIDE.md](docs/AGENTSYNC_GUIDE.md)：命令使用、检查模式、全局收敛、仓库收敛、批量收敛、草稿采纳、本地验证、发布入口
- [docs/AGENTSYNC_KNOWLEDGE_BASE.md](docs/AGENTSYNC_KNOWLEDGE_BASE.md)：规范文件收敛、Skill 根目录收敛、路径与别名策略、备份与合并、安全边界、AI 易错点
- [docs/agent_runtime_global_paths.md](docs/agent_runtime_global_paths.md)：新增/调整某个 agent runtime 的规范入口或 skill 目录、核对某工具的全局规则文件与 skill 目录官方路径时查阅（市面主流 runtime 全局路径调研，含置信度标注）
- [README.md](README.md)：面向公开用户的英文安装与用法说明
- [README.zh-CN.md](README.zh-CN.md)：面向公开用户的中文安装与用法说明

## 领域地图（doc-init）

<!-- 覆盖度复核基线：2026-06-30 · 源码指纹 扫描 20 文件 / Go 8 / 0 子模块 · 基线提交 577b8bb -->

| 领域 | 入口锚点 |
|------|---------|
| 命令调度与运行模式 | main.go；internal/agentsync/run.go |
| 规范文件收敛 | internal/agentsync/run.go；internal/agentsync/merge.go |
| Skill 根目录收敛 | internal/agentsync/skills.go |
| 路径、别名与备份策略 | internal/agentsync/paths.go；internal/agentsync/files.go |

## 待补充知识库（doc-init backlog）

- [待补充] 发布与安装链路 Guide —— 入口锚点：.github/workflows/release.yml；.goreleaser.yml；触发场景：调整 GitHub Release、GoReleaser、Homebrew cask、安装分发前。
- [待补充] 测试隔离与安全验证 Guide —— 入口锚点：internal/agentsync/run_test.go；触发场景：新增同步场景测试、调整真实用户目录保护、排查测试污染风险前。

## 改动注意事项

- 改 `--check`、`--repo`、`--all`、`--adopt`、`--force` 任一行为时，先读 [docs/AGENTSYNC_GUIDE.md](docs/AGENTSYNC_GUIDE.md)，再同步更新 README 的用法示例。
- 改统一源、目标入口、备份、合并、别名降级或 Skill 根目录替换时，先读 [docs/AGENTSYNC_KNOWLEDGE_BASE.md](docs/AGENTSYNC_KNOWLEDGE_BASE.md)。
- `CLAUDE.md` 必须保持单行 `@AGENTS.md`，不要在其中写项目规则。
- `.doc-init-*.json` 是 doc-init 扫描产物；若需要重新初始化可复用或重跑，但普通功能改动不依赖它们。
