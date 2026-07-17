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
	targets := []Target{
		{Path: "~/.codex/AGENTS.md", Mode: "link"},
		{Path: "~/.config/opencode/AGENTS.md", Mode: "link"},
		{Path: "~/.claude/CLAUDE.md", Mode: "claude"},
		{Path: "~/.grok/AGENTS.md", Mode: "link"},
		{Path: "~/.kimi-code/AGENTS.md", Mode: "link"},
	}
	skillTargets := []SkillTarget{
		{Path: "~/.claude/skills"},
		{Path: "~/.codex/skills"},
		{Path: "~/.config/opencode/skill"},
		{Path: "~/.grok/skills"},
		{Path: "~/.kimi-code/skills"},
	}
	for i := range targets {
		p, err := expandPath(targets[i].Path)
		if err != nil {
			return Config{}, err
		}
		targets[i].Path = p
	}
	for i := range skillTargets {
		p, err := expandPath(skillTargets[i].Path)
		if err != nil {
			return Config{}, err
		}
		skillTargets[i].Path = p
	}
	return Config{Source: source, Targets: targets, SkillSource: skillSource, SkillTargets: skillTargets}, nil
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
