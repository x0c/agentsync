package agentsync

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func defaultGlobalConfig() (Config, error) {
	source, err := expandPath("~/.config/agentsync/AGENTS.md")
	if err != nil {
		return Config{}, err
	}
	skillSource, err := expandPath("~/.config/agentsync/skills")
	if err != nil {
		return Config{}, err
	}
	// 每个 runtime 一条：Detect 是该工具的用户级主目录，只有它已存在（即用户装了该工具）
	// 才会为其创建规范入口 / skill 根目录别名；未安装的工具一律跳过，不留任何文件。
	targets := []Target{
		// 头部 CLI Agent
		{Path: "~/.codex/AGENTS.md", Mode: "link", Detect: "~/.codex"},
		{Path: "~/.config/opencode/AGENTS.md", Mode: "link", Detect: "~/.config/opencode"},
		{Path: "~/.claude/CLAUDE.md", Mode: "claude", Detect: "~/.claude"},
		{Path: "~/.gemini/GEMINI.md", Mode: "link", Detect: "~/.gemini"},
		{Path: "~/.qwen/QWEN.md", Mode: "link", Detect: "~/.qwen"},
		{Path: "~/.copilot/copilot-instructions.md", Mode: "link", Detect: "~/.copilot"},
		{Path: "~/.kimi-code/AGENTS.md", Mode: "link", Detect: "~/.kimi-code"},
		// 开源 / 独立 CLI Agent
		{Path: "~/.grok/AGENTS.md", Mode: "link", Detect: "~/.grok"},
		{Path: "~/.config/amp/AGENTS.md", Mode: "link", Detect: "~/.config/amp"},
		{Path: "~/.config/crush/CRUSH.md", Mode: "link", Detect: "~/.config/crush"},
		{Path: "~/.config/goose/AGENTS.md", Mode: "link", Detect: "~/.config/goose"},
		{Path: "~/.factory/AGENTS.md", Mode: "link", Detect: "~/.factory"},
		{Path: "~/.iflow/IFLOW.md", Mode: "link", Detect: "~/.iflow"},
		{Path: "~/.config/kilo/AGENTS.md", Mode: "link", Detect: "~/.config/kilo"},
		// IDE / 编辑器系 Agent
		{Path: "~/.cursor/rules/AGENTS.mdc", Mode: "cursor", Detect: "~/.cursor"},
		{Path: "~/.codeium/windsurf/memories/global_rules.md", Mode: "link", Detect: "~/.codeium/windsurf"},
		{Path: "~/.config/zed/AGENTS.md", Mode: "link", Detect: "~/.config/zed"},
		// 大厂自研 Agent
		{Path: "~/.codebuddy/CODEBUDDY.md", Mode: "link", Detect: "~/.codebuddy"},
		{Path: "~/.qoder/AGENTS.md", Mode: "link", Detect: "~/.qoder"},
		{Path: "~/.junie/AGENTS.md", Mode: "link", Detect: "~/.junie"},
		{Path: "~/.kiro/steering/AGENTS.md", Mode: "link", Detect: "~/.kiro"},
		{Path: "~/.joycode/AGENTS.md", Mode: "link", Detect: "~/.joycode"},
		// 通用跨工具入口（约定俗成的 ~/.agents，仅在用户已建立时才收敛）
		{Path: "~/.agents/AGENTS.md", Mode: "link", Detect: "~/.agents"},
	}
	skillTargets := []SkillTarget{
		// 头部 CLI Agent
		{Path: "~/.claude/skills", Detect: "~/.claude"},
		{Path: "~/.codex/skills", Detect: "~/.codex"},
		{Path: "~/.config/opencode/skills", Detect: "~/.config/opencode"},
		{Path: "~/.qwen/skills", Detect: "~/.qwen"},
		{Path: "~/.copilot/skills", Detect: "~/.copilot"},
		{Path: "~/.kimi-code/skills", Detect: "~/.kimi-code"},
		// 开源 / 独立 CLI Agent
		{Path: "~/.grok/skills", Detect: "~/.grok"},
		{Path: "~/.config/amp/skills", Detect: "~/.config/amp"},
		{Path: "~/.config/crush/skills", Detect: "~/.config/crush"},
		{Path: "~/.factory/skills", Detect: "~/.factory"},
		{Path: "~/.iflow/skills", Detect: "~/.iflow"},
		{Path: "~/.aider-desk/skills", Detect: "~/.aider-desk"},
		// IDE / 编辑器系 Agent
		{Path: "~/.cursor/skills", Detect: "~/.cursor"},
		{Path: "~/.codeium/windsurf/skills", Detect: "~/.codeium/windsurf"},
		// 大厂自研 Agent
		{Path: "~/.codebuddy/skills", Detect: "~/.codebuddy"},
		{Path: "~/.qoder/skills", Detect: "~/.qoder"},
		{Path: "~/.kiro/skills", Detect: "~/.kiro"},
		{Path: "~/.joycode/skills", Detect: "~/.joycode"},
		// 通用跨工具入口
		{Path: "~/.agents/skills", Detect: "~/.agents"},
	}
	for i := range targets {
		p, err := expandPath(targets[i].Path)
		if err != nil {
			return Config{}, err
		}
		targets[i].Path = p
		d, err := expandPath(targets[i].Detect)
		if err != nil {
			return Config{}, err
		}
		targets[i].Detect = d
	}
	for i := range skillTargets {
		p, err := expandPath(skillTargets[i].Path)
		if err != nil {
			return Config{}, err
		}
		skillTargets[i].Path = p
		d, err := expandPath(skillTargets[i].Detect)
		if err != nil {
			return Config{}, err
		}
		skillTargets[i].Detect = d
	}
	mcpSource, err := expandPath("~/.config/agentsync/mcp.json")
	if err != nil {
		return Config{}, err
	}
	mcpTargets := []MCPTarget{
		{Name: "codex", Path: "~/.codex/config.toml", Detect: "~/.codex", Dialect: "codex", Format: "toml", Mode: "key"},
		{Name: "claude", Path: "~/.claude.json", Detect: "~/.claude", Dialect: "claude", Format: "json", Mode: "key"},
		{Name: "opencode", Path: "~/.config/opencode/opencode.json", Detect: "~/.config/opencode", Dialect: "opencode", Format: "json", Mode: "key"},
		{Name: "gemini", Path: "~/.gemini/settings.json", Detect: "~/.gemini", Dialect: "gemini", Format: "json", Mode: "key"},
		{Name: "qwen", Path: "~/.qwen/settings.json", Detect: "~/.qwen", Dialect: "gemini", Format: "json", Mode: "key"},
		{Name: "copilot", Path: "~/.copilot/mcp-config.json", Detect: "~/.copilot", Dialect: "cursor", Format: "json", Mode: "file"},
		{Name: "kimi-code", Path: "~/.kimi-code/mcp.json", Detect: "~/.kimi-code", Dialect: "cursor", Format: "json", Mode: "file"},
		{Name: "grok", Path: "~/.grok/user-settings.json", Detect: "~/.grok", Dialect: "grok", Format: "json", Mode: "key"},
		{Name: "amp", Path: "~/.config/amp/settings.json", Detect: "~/.config/amp", Dialect: "amp", Format: "json", Mode: "key"},
		{Name: "crush", Path: "~/.config/crush/crush.json", Detect: "~/.config/crush", Dialect: "crush", Format: "json", Mode: "key"},
		{Name: "goose", Path: "~/.config/goose/config.yaml", Detect: "~/.config/goose", Dialect: "goose", Format: "yaml", Mode: "key"},
		{Name: "factory", Path: "~/.factory/mcp.json", Detect: "~/.factory", Dialect: "cursor", Format: "json", Mode: "file"},
		{Name: "kilo", Path: "~/.config/kilo/kilo.jsonc", Detect: "~/.config/kilo", Dialect: "opencode", Format: "jsonc", Mode: "key"},
		{Name: "cursor", Path: "~/.cursor/mcp.json", Detect: "~/.cursor", Dialect: "cursor", Format: "json", Mode: "file"},
		{Name: "windsurf", Path: "~/.codeium/windsurf/mcp_config.json", Detect: "~/.codeium/windsurf", Dialect: "windsurf", Format: "json", Mode: "file"},
		{Name: "zed", Path: "~/.config/zed/settings.json", Detect: "~/.config/zed", Dialect: "zed", Format: "json", Mode: "key"},
		{Name: "codebuddy", Path: "~/.codebuddy/.mcp.json", Detect: "~/.codebuddy", Dialect: "cursor", Format: "json", Mode: "file"},
		{Name: "qoder", Path: "~/.qoder/settings.json", Detect: "~/.qoder", Dialect: "claude", Format: "json", Mode: "key"},
		{Name: "junie", Path: "~/.junie/mcp/mcp.json", Detect: "~/.junie", Dialect: "cursor", Format: "json", Mode: "file"},
		{Name: "kiro", Path: "~/.kiro/settings/mcp.json", Detect: "~/.kiro", Dialect: "cursor", Format: "json", Mode: "file"},
		{Name: "joycode", Path: "~/.joycode/joycode-mcp.json", Detect: "~/.joycode", Dialect: "cursor", Format: "json", Mode: "file"},
	}
	for i := range mcpTargets {
		p, err := expandPath(mcpTargets[i].Path)
		if err != nil {
			return Config{}, err
		}
		mcpTargets[i].Path = p
		d, err := expandPath(mcpTargets[i].Detect)
		if err != nil {
			return Config{}, err
		}
		mcpTargets[i].Detect = d
	}
	return Config{
		Source:       source,
		Targets:      targets,
		SkillSource:  skillSource,
		SkillTargets: skillTargets,
		MCPSource:    mcpSource,
		MCPTargets:   mcpTargets,
	}, nil
}

func repoConfig(root string) Config {
	return Config{
		Source: filepath.Join(root, "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(root, "CLAUDE.md"), Mode: "relative-link"},
		},
	}
}

func expandPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return filepath.Abs(path)
}

func configRoot() (string, error) {
	if override := os.Getenv("AGENTSYNC_CONFIG_HOME"); override != "" {
		return filepath.Abs(override)
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, "agentsync"), nil
		}
	}
	p, err := expandPath("~/.config/agentsync")
	if err != nil {
		return "", err
	}
	return p, nil
}

func mergeDraftDir() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "merge-drafts"), nil
}

func backupDir() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "backups"), nil
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
