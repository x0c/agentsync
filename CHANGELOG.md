# Changelog

Release notes are bilingual: English first, then complete Simplified Chinese.
Development adds entries under `## Unreleased`; at release time they become the
next version's section. A `Release: minor` or `Release: major` line requests a
larger version step; `Release: hold <reason>` keeps the changes unreleased until
the named acceptance is done.

## 1.1.0

### English

- Fix Windows skill-directory sync without symbolic-link permission (Developer Mode off, not elevated): skill folders now link through an NTFS directory junction instead of failing, and the CLI reports `junction`. Directories never fall back to a static copy; when a junction is impossible too (for example on a non-NTFS volume), agentsync stops with Developer Mode guidance. Backup, rollback, replacement and overlap checks recognize junctions and never touch their targets.
- Add `agentsync --version`.
- Add one-line installers: `install.ps1` for Windows (amd64/arm64, checksum-verified `agentsync_windows_<arch>.zip`) and `install.sh` for macOS/Linux (amd64/arm64, checksum-verified tarballs), with a manual installer verification workflow. Release archives now include Windows arm64.
- Add Windows watcher templates (`contrib/windows/install-watcher.ps1`, `uninstall-watcher.ps1`): a per-user scheduled task runs `agentsync --watch` at sign-in with no console window; the uninstaller removes exactly what install created.
- Document the full supported-tool list, per-tool skill/MCP filtering, `.gitignore`/`.stignore` gating for `mcp.json`, and Windows symlink-then-junction skill linking.

### 简体中文

- 修复 Windows 在没有符号链接权限时（未开开发者模式、非管理员）无法同步 Skill 目录的问题：Skill 文件夹改用 NTFS 目录联接（junction）链接，不再报错，命令输出显示 `junction`。目录绝不退回为静态副本；连联接也无法创建时（例如非 NTFS 卷），agentsync 会停止并提示开启开发者模式。备份、回滚、替换与重叠检查都能识别联接，且绝不改动其指向的内容。
- 新增 `agentsync --version`。
- 新增一键安装脚本：Windows 用 `install.ps1`（amd64/arm64，校验 `agentsync_windows_<arch>.zip` 的 SHA-256），macOS/Linux 用 `install.sh`（amd64/arm64，校验对应 tarball），并配有手动安装验证工作流。Release 归档新增 Windows arm64。
- 新增 Windows 后台任务模板（`contrib/windows/install-watcher.ps1`、`uninstall-watcher.ps1`）：以按用户的计划任务在登录时运行 `agentsync --watch`，无控制台窗口；卸载脚本只移除安装时创建的内容。
- 补齐文档：完整的支持工具列表、按工具的 Skill/MCP 过滤、`mcp.json` 的 `.gitignore`/`.stignore` 写入条件，以及 Windows 下先符号链接后目录联接（junction）的 Skill 链接方式。

## 1.0.0

### English

- First stable release, with no functional changes since 0.16.5. The 0.16.x series resolved all 17 confirmed review defects and migrated the Homebrew Cask hook to the supported declarative format.

### 简体中文

- 首个稳定版本，自 0.16.5 起无功能变化。0.16.x 系列已修复审查确认的全部 17 项缺陷，并将 Homebrew Cask 钩子迁移至受支持的声明式写法。

## 0.16.5

### English

- Migrate the Homebrew Cask post-install hook to GoReleaser's declarative `install_steps` DSL, following the `homebrew_casks.hooks.post.install` deprecation. The quarantine-bit removal behavior on macOS is unchanged.

### 简体中文

- 将 Homebrew Cask 安装后钩子迁移至 GoReleaser 声明式 `install_steps` 写法，跟进 `homebrew_casks.hooks.post.install` 弃用说明；macOS 去隔离位行为不变。

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
