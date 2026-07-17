package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncCreatesSourceAndAliases(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source: filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(dir, "codex", "AGENTS.md"), Mode: "link"},
			{Path: filepath.Join(dir, "claude", "CLAUDE.md"), Mode: "link"},
		},
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	if !pathExists(cfg.Source) {
		t.Fatalf("source was not created")
	}
	for _, target := range cfg.Targets {
		if !symlinkPointsTo(target.Path, cfg.Source) && !sameContent(cfg.Source, target.Path) {
			t.Fatalf("target %s does not point to or match source; report=%+v", target.Path, report)
		}
	}
}

func TestCheckDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source:  filepath.Join(dir, "AGENTS.md"),
		Targets: []Target{{Path: filepath.Join(dir, "CLAUDE.md"), Mode: "link"}},
	}
	if _, err := syncConfig(cfg, Options{Check: true}); err != nil {
		t.Fatalf("syncConfig(check) error = %v", err)
	}
	if pathExists(cfg.Source) || pathExists(cfg.Targets[0].Path) {
		t.Fatalf("check mode wrote files")
	}
}

func TestConflictMergesAndLinks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source:  filepath.Join(dir, "AGENTS.md"),
		Targets: []Target{{Path: filepath.Join(dir, "CLAUDE.md"), Mode: "link"}},
	}
	if err := os.WriteFile(cfg.Source, []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Targets[0].Path, []byte("target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	if report.MergeDraft != "" {
		t.Fatalf("merge draft should not be created: %+v", report)
	}
	source, err := os.ReadFile(cfg.Source)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(source); !containsAll(got, "source", "target", "agentsync:begin import") {
		t.Fatalf("source did not contain merged content: %q", got)
	}
	if !symlinkPointsTo(cfg.Targets[0].Path, cfg.Source) && !sameContent(cfg.Source, cfg.Targets[0].Path) {
		t.Fatalf("target was not linked to source: %+v", report)
	}
	report, err = syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("second syncConfig() error = %v", err)
	}
	for _, result := range report.Results {
		if result.Status != "ok" {
			t.Fatalf("second run should be idempotent, got %+v", report.Results)
		}
	}
}

func TestMissingSourceCreatedFromExistingTargets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source: filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(dir, "codex", "AGENTS.md"), Mode: "link"},
			{Path: filepath.Join(dir, "claude", "CLAUDE.md"), Mode: "link"},
		},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Targets[0].Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Targets[1].Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Targets[0].Path, []byte("codex rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Targets[1].Path, []byte("claude rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	source, err := os.ReadFile(cfg.Source)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(source); !containsAll(got, "codex rules", "claude rules") {
		t.Fatalf("source did not include existing files: %q", got)
	}
	for _, target := range cfg.Targets {
		if !symlinkPointsTo(target.Path, cfg.Source) && !sameContent(cfg.Source, target.Path) {
			t.Fatalf("target %s was not linked; report=%+v", target.Path, report)
		}
	}
}

func TestSkillSyncImportsAndLinksRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source:      filepath.Join(dir, "AGENTS.md"),
		Targets:     []Target{{Path: filepath.Join(dir, "codex", "AGENTS.md"), Mode: "link"}},
		SkillSource: filepath.Join(dir, "agentsync", "skills"),
		SkillTargets: []SkillTarget{
			{Path: filepath.Join(dir, "claude", "skills")},
			{Path: filepath.Join(dir, "codex", "skills")},
		},
	}
	if err := os.MkdirAll(filepath.Join(cfg.SkillTargets[0].Path, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.SkillTargets[0].Path, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	canonicalSkill := filepath.Join(cfg.SkillSource, "demo", "SKILL.md")
	if !pathExists(canonicalSkill) {
		t.Fatalf("canonical skill was not created; report=%+v", report)
	}
	for _, target := range cfg.SkillTargets {
		if !symlinkPointsTo(target.Path, cfg.SkillSource) {
			t.Fatalf("skill target root was not linked: %s; report=%+v", target.Path, report)
		}
	}
	if !pathExists(filepath.Join(cfg.SkillTargets[1].Path, "demo", "SKILL.md")) {
		t.Fatalf("linked target root does not expose canonical skill; report=%+v", report)
	}

	report, err = syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("second syncConfig() error = %v", err)
	}
	for _, result := range report.SkillResults {
		if result.Status != "ok" {
			t.Fatalf("second run should be idempotent, got %+v", report.SkillResults)
		}
	}
}

func TestSkillRootLinkReflectsDeletedCanonicalSkill(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source:      filepath.Join(dir, "AGENTS.md"),
		Targets:     []Target{{Path: filepath.Join(dir, "codex", "AGENTS.md"), Mode: "link"}},
		SkillSource: filepath.Join(dir, "agentsync", "skills"),
		SkillTargets: []SkillTarget{
			{Path: filepath.Join(dir, "codex", "skills")},
		},
	}
	canonicalSkill := filepath.Join(cfg.SkillSource, "demo")
	if err := os.MkdirAll(canonicalSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonicalSkill, "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	targetSkill := filepath.Join(cfg.SkillTargets[0].Path, "demo")
	if !pathExists(filepath.Join(targetSkill, "SKILL.md")) {
		t.Fatalf("target skill should exist through the skill root symlink")
	}
	if err := os.RemoveAll(canonicalSkill); err != nil {
		t.Fatal(err)
	}
	if pathExists(targetSkill) {
		t.Fatalf("target skill should disappear when canonical skill is deleted")
	}
}

func TestSkillSyncPreservesHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source:      filepath.Join(dir, "AGENTS.md"),
		Targets:     []Target{{Path: filepath.Join(dir, "codex", "AGENTS.md"), Mode: "link"}},
		SkillSource: filepath.Join(dir, "agentsync", "skills"),
		SkillTargets: []SkillTarget{
			{Path: filepath.Join(dir, "codex", "skills")},
		},
	}
	hidden := filepath.Join(cfg.SkillTargets[0].Path, ".system", "internal")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "SKILL.md"), []byte("# Internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	if !pathExists(filepath.Join(cfg.SkillSource, ".system", "internal", "SKILL.md")) {
		t.Fatalf("hidden system skills should be preserved before replacing the root")
	}
	if !symlinkPointsTo(cfg.SkillTargets[0].Path, cfg.SkillSource) {
		t.Fatalf("skill target root was not linked")
	}
}

func TestSkillSyncMaterializesCanonicalSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source:      filepath.Join(dir, "AGENTS.md"),
		Targets:     []Target{{Path: filepath.Join(dir, "codex", "AGENTS.md"), Mode: "link"}},
		SkillSource: filepath.Join(dir, "agentsync", "skills"),
		SkillTargets: []SkillTarget{
			{Path: filepath.Join(dir, "claude", "skills")},
		},
	}
	realSkill := filepath.Join(dir, "external", "demo")
	if err := os.MkdirAll(realSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realSkill, "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalLink := filepath.Join(cfg.SkillSource, "demo")
	if err := os.MkdirAll(filepath.Dir(canonicalLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realSkill, canonicalLink); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	info, err := os.Lstat(canonicalLink)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("canonical skill should be a real directory, still symlink")
	}
	if !pathExists(filepath.Join(canonicalLink, "SKILL.md")) {
		t.Fatalf("materialized canonical skill lost SKILL.md")
	}
}

func TestDefaultGlobalConfigIncludesKimiCode(t *testing.T) {
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	if !hasPathSuffix(targetPaths(cfg.Targets), filepath.Join(".kimi-code", "AGENTS.md")) {
		t.Fatalf("Kimi Code 规范入口缺失: %+v", cfg.Targets)
	}
	if !hasPathSuffix(skillTargetPaths(cfg.SkillTargets), filepath.Join(".kimi-code", "skills")) {
		t.Fatalf("Kimi Code Skill 入口缺失: %+v", cfg.SkillTargets)
	}
}

func targetPaths(targets []Target) []string {
	paths := make([]string, 0, len(targets))
	for _, t := range targets {
		paths = append(paths, t.Path)
	}
	return paths
}

func skillTargetPaths(targets []SkillTarget) []string {
	paths := make([]string, 0, len(targets))
	for _, t := range targets {
		paths = append(paths, t.Path)
	}
	return paths
}

func hasPathSuffix(paths []string, suffix string) bool {
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
