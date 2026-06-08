# agentsync 知识库

## 核心目标

agentsync 用一个命令把多个 AI coding agent 的指令文件和 Agent Skill 目录收敛到统一源，避免 Codex、Claude Code、OpenCode 各自维护一套内容。

## 全局规范源

默认全局规范源是：

```text
~/.config/agentsync/AGENTS.md
```

这些工具入口应指向同一个源文件：

```text
~/.codex/AGENTS.md
~/.config/opencode/AGENTS.md
~/.claude/CLAUDE.md
```

## Skill 源目录

默认 Skill 源目录是：

```text
~/.config/agentsync/skills/
```

每个 skill 是统一 Skill 根目录下的一个目录，目录内必须包含 `SKILL.md`。脚本、模板、示例、参考文件等与 `SKILL.md` 放在同一 skill 目录下。

这些工具 skill 目录会指向统一源：

```text
~/.claude/skills -> ~/.config/agentsync/skills
~/.codex/skills -> ~/.config/agentsync/skills
~/.config/opencode/skill -> ~/.config/agentsync/skills
```

工具侧 Skill 根目录整体软链接到统一源，因此统一源中新增、删除或重命名 skill 后，所有工具侧会实时看到同一结果，不需要逐个 skill 清理旧引用。

## 收敛策略

- `agentsync` 是一键幂等命令。
- 第一次运行会创建统一源文件和统一 skill 目录。
- 已有工具侧文件有独特内容时，内容会并入统一源，再把工具侧文件替换成软链接。
- 已有工具侧 skill 目录会复制到统一 skill 源，再把工具侧 Skill 根目录整体替换成软链接。
- 第二次运行应只输出 `ok`，不重复追加内容。

## 安全边界

- `--check` 只读，不写文件。
- 替换文件或目录前会写入 `~/.config/agentsync/backups/`。
- Codex 的隐藏系统 skill 目录，例如 `.system`，会在工具侧 Skill 根目录替换前先保留到统一 Skill 根目录。
- 单测必须设置 `AGENTSYNC_CONFIG_HOME`，避免测试备份污染真实用户目录。
