# Changelog

Release notes are bilingual: English first, then complete Simplified Chinese.
Development adds entries under `## Unreleased`; at release time they become the
next version's section. A `Release: minor` or `Release: major` line requests a
larger version step; `Release: hold <reason>` keeps the changes unreleased until
the named acceptance is done.

## Unreleased

## 0.16.4

### English

- Fix all 17 confirmed review defects, including destructive skill staging, backup-name collisions, unrelated TOML configuration deletion, and malformed MCP entries being treated as server removals.
- Preserve Codex server-local restrictions and authentication, unmanaged Cursor rules, Gemini/Qwen SSE transport, and quoted dotted server names. Reject Crush shell command substitution and ambiguous policy targets.
- Reject unexpected arguments before writes, report blocked targets and batch-scan failures correctly, reconcile watcher inputs, and make skill materialization and latest-backup rollback safe and reversible.
- Add 18 isolated real-CLI safety scenarios and expanded regression coverage. Require Go 1.25.8 or newer and recommend the patched Go 1.26.10 toolchain. Previously overwritten backup contents cannot be recovered; ambiguous legacy manifests are rejected instead of restoring incorrect data.

### 简体中文

- 修复本次审查确认的全部 17 项缺陷，包括 Skill 临时目录误删、备份重名覆盖、非 MCP 的 TOML 配置被删除，以及无效 MCP 条目被误当作删除服务器。
- 保留 Codex 服务器的本地限制与认证、用户自写的 Cursor 规则、Gemini/Qwen 的 SSE 传输方式，以及名称中带点的服务器。拒绝 Crush shell 命令替换和有歧义的策略目标。
- 写入前拒绝多余参数，正确报告被阻止的目标与批量扫描错误，修复 watcher 同步收敛，并确保 Skill 实体化与最近备份回滚安全可逆。
- 新增 18 组隔离的真实 CLI 安全验收场景并扩充回归测试。最低要求 Go 1.25.8，推荐已修补的 Go 1.26.10 工具链。旧版本已覆盖的备份内容无法恢复；有歧义的旧清单会被拒绝，而不会恢复错误数据。
