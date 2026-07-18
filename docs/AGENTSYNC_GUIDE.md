# agentsync 命令工作流 Guide

## 文档定位

本文覆盖 agentsync 的命令入口、用户工作流、验证方式和发布入口。它回答“执行哪个命令会发生什么、改完如何确认行为正确”。收敛机制内部细节、路径策略、备份策略和 Skill 根目录替换规则见 [AGENTSYNC_KNOWLEDGE_BASE.md](AGENTSYNC_KNOWLEDGE_BASE.md)。

## 核心命令速查

| 场景 | 命令 | 是否写文件 | 主要输出 | 实现入口 |
|---|---|---:|---|---|
| 预览全局收敛 | `agentsync --check` | 否 | `Results`、`Skills`、可能的 `Merge draft` | `main.go`；`internal/agentsync/run.go` |
| 执行全局收敛 | `agentsync` | 是 | 统一源、目标别名、备份、Skill 结果 | `internal/agentsync/run.go`；`internal/agentsync/skills.go` |
| 收敛当前仓库 | `agentsync --repo` | 是 | 当前仓库 `CLAUDE.md` 指向 `AGENTS.md` | `internal/agentsync/paths.go` |
| 批量收敛仓库 | `agentsync --all ~/Codes` | 是 | 扫描到的仓库数量与每个仓库结果 | `internal/agentsync/run.go` |
| 采纳合并草稿 | `agentsync --adopt <draft>` | 是 | 备份原统一源并用草稿替换 | `internal/agentsync/merge.go` |
| 强制替换冲突入口 | `agentsync --force` | 是 | 备份后替换错误链接或不同内容文件 | `internal/agentsync/run.go` |

## 命令调度流程

```mermaid
flowchart TD
    A[main.go 解析 flag] --> B[agentsync.Run]
    B -->|--all| C[runAll 扫描 Git 仓库]
    B -->|--repo| D[repoConfig 使用仓库 AGENTS.md]
    B -->|默认| E[defaultGlobalConfig 使用用户级统一源]
    D --> F[syncConfig]
    E --> F
    C --> F
    B -->|--adopt| G[adoptDraft]
    G --> F
    F --> H[syncTarget 处理规范文件入口]
    F --> I[syncSkills 处理 Skill 根目录]
    H --> J[printReport]
    I --> J
```

命令入口只负责参数到 `Options` 的映射。实际工作都汇入 `Run()`：先判断是否批量仓库模式，再根据全局/仓库模式生成配置，最后执行 `syncConfig()` 并打印报告。

## 全局收敛工作流

全局模式的统一源与目标来自 `defaultGlobalConfig()`：

| 类型 | 路径 | 说明 |
|---|---|---|
| 规范统一源 | `~/.config/agentsync/AGENTS.md` | 所有工具共享的指令文件 |
| Skill 统一源 | `~/.config/agentsync/skills` | 每个子目录是一个完整 skill |

规范/Skill 入口覆盖 Codex、OpenCode、Claude、Gemini、Qwen、Copilot、Kimi Code、Grok、Amp、Crush、Goose、Factory、iFlow、Kilo、Windsurf、Zed、CodeBuddy、Qoder、Junie、Kiro、JoyCode 及通用 `~/.agents`，完整清单以 `defaultGlobalConfig()` 为准。各入口大致形如：

```text
规范入口：  ~/.codex/AGENTS.md、~/.claude/CLAUDE.md、~/.gemini/GEMINI.md、
            ~/.qwen/QWEN.md、~/.joycode/AGENTS.md …（指向规范统一源）
Skill 入口：~/.codex/skills、~/.config/opencode/skills、~/.joycode/skills …
            （整体指向 Skill 统一源）
```

**按安装门控**：每个入口只在对应工具已安装（其用户级主目录如 `~/.codex`、`~/.joycode` 存在）时才收敛；未安装的工具报告为 `skipped`，不创建任何文件。因此在同一台机器上 `Results` / `Skills` 里出现的入口取决于你实际装了哪些工具。

第一次运行可能会出现 `created`、`merged`、`replaced`、`linked` 等状态，未装的工具显示 `skipped`。第二次运行已安装工具应收敛到 `ok`，这是幂等性判断的主要用户信号。

## 检查模式

`--check` 只做状态判定，不创建统一源、不写备份、不替换入口、不复制 skill。报告中的状态含义：

| 状态 | 含义 | 下一步 |
|---|---|---|
| `skipped` | 该工具未安装（`Detect` 主目录不存在） | 无需处理；装了该工具再跑一次即可收敛 |
| `ok` | 入口已指向统一源 | 无需处理 |
| `missing` | 统一源或目标入口缺失 | 直接运行 `agentsync` 创建 |
| `mergeable` | 目标文件有独特内容，可合并进统一源 | 运行 `agentsync`，必要时检查合并结果 |
| `replaceable` | 文件或目录可被备份后替换 | 运行 `agentsync` |
| `wrong-link` | 已是链接但指向不对 | 运行 `agentsync`；冲突强时可加 `--force` |
| `broken-link` | 链接目标已失效 | 运行 `agentsync` 修复 |
| `blocked` | 目标不是可处理的普通文件或目录 | 人工判断后再处理 |

**AI 易错点**：不要把 `--check` 报告里的 “would ...” 当作已修复。它只说明下一次真实运行会做什么。

## 仓库模式

`agentsync --repo` 用当前 Git 仓库根目录的 `AGENTS.md` 作为项目级源，只管理一个目标：

```text
CLAUDE.md -> AGENTS.md
```

仓库模式通过 `findRepoRoot()` 找 `.git`，再由 `repoConfig()` 构造配置。目标使用相对链接模式，便于仓库移动目录后链接仍成立。

仓库模式不处理全局 Skill 目录，也不处理用户级全局规范入口。

## 批量仓库模式

`agentsync --all <目录>` 会递归扫描目录下的 Git 仓库，并对每个仓库执行仓库模式。扫描时会跳过 `node_modules`、`.venv`、`target`、`build` 等常见大型构建目录。

批量模式的结果聚合到一个报告里，`Repositories` 表示扫描到的仓库数量。某个仓库出现错误会中断本轮执行；这适合个人工作区批量规范 `CLAUDE.md` 指针。

**AI 易错点**：`--all` 的语义是“批量仓库级指针收敛”，不是全局规范和 Skill 同步；不要把它写成对每个仓库执行全局模式。

## 合并草稿与采纳

当现有入口文件和统一源存在冲突时，agentsync 会尽量将独特内容追加到统一源。对于需要人工整理的场景，`createMergeDraft()` 会在合并草稿目录生成 Markdown，报告中出现 `Merge draft` 和下一步命令：

```bash
agentsync --adopt <draft-path>
```

采纳草稿时，`adoptDraft()` 会：

1. 展开草稿路径。
2. 确认草稿存在。
3. 非检查模式下备份当前统一源。
4. 将草稿内容写入统一源。
5. 继续执行一次同步，让目标入口重新收敛。

`--adopt --check` 只检查草稿路径是否可用，不替换统一源。

## 本地开发验证

代码改动后至少执行：

```bash
go test ./...
go build ./...
```

修改 CLI 行为后执行：

```bash
go install .
agentsync --check
```

改发布配置后执行：

```bash
goreleaser check
```

文档-only 改动执行：

```bash
python3 /home/vibecoder/.config/agentsync/skills/doc-init/scripts/doc_nav_lint.py --root .
```

本仓没有常驻服务、HTTP 端口或本地数据库依赖，不生成 `OPERATIONS_GUIDE.md`。运行验证主要是 CLI 冒烟和测试。

## 发布入口

正式发布由 tag 触发：

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions 的 release 工作流使用 GoReleaser，读取 `.goreleaser.yml`，构建 Linux、Darwin、Windows 的 amd64/arm64 产物，并更新 `x0c/homebrew-tap` 的 cask。

发布前必须确认：

- `HOMEBREW_TAP_GITHUB_TOKEN` 已配置且有推送 `x0c/homebrew-tap` 的权限。
- `.goreleaser.yml` 的 `homebrew_casks` 仍指向正确 tap。
- tag 版本与 README 安装说明一致。

发布链路本次未深写为独立文档，已登记在根 `AGENTS.md` 的 doc-init backlog。

## 覆盖度与待补充项

- 代码推断覆盖：命令入口、参数分发、全局/仓库/批量/草稿工作流均已从代码和 README 校准。
- 多源证据补强：读取了 README、中文 README、CI、Release workflow、GoReleaser 配置和既有 docs。
- Git 弱信号：历史只有 5 个提交，热点主要集中在 README、docs、release workflow 和 Skill 同步，作为优先级参考，不单独沉淀成当前规则。
- Q&A 补充：缺少用户经验输入；真实团队常用命令、失败处理习惯和发布前人工检查口径仍待补充。
- 待补充：发布与安装链路、测试隔离与安全验证可在后续 doc-update/doc-init 续写中拆为独立 Guide。

<!-- 该文档由 doc-init 更新于 2026-06-30；定位：AI 修改 agentsync 命令工作流前的快速参考文档 -->
