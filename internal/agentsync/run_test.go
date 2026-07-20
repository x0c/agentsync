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

func TestUninstalledRuntimeIsSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	installed := filepath.Join(dir, "installed")
	if err := os.MkdirAll(installed, 0o755); err != nil {
		t.Fatal(err)
	}
	missingRuntime := filepath.Join(dir, "missing")
	cfg := Config{
		Source: filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(installed, "AGENTS.md"), Mode: "link", Detect: installed},
			{Path: filepath.Join(missingRuntime, "AGENTS.md"), Mode: "link", Detect: missingRuntime},
		},
		SkillSource: filepath.Join(dir, "agentsync", "skills"),
		SkillTargets: []SkillTarget{
			{Path: filepath.Join(installed, "skills"), Detect: installed},
			{Path: filepath.Join(missingRuntime, "skills"), Detect: missingRuntime},
		},
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	// 未安装的 runtime 目录不能被创建，也不能生成任何入口文件。
	if pathExists(missingRuntime) {
		t.Fatalf("未安装 runtime 的目录被创建了: %s", missingRuntime)
	}
	if !symlinkPointsTo(cfg.Targets[0].Path, cfg.Source) && !sameContent(cfg.Source, cfg.Targets[0].Path) {
		t.Fatalf("已安装 runtime 未被收敛; report=%+v", report)
	}
	skipped := 0
	for _, r := range append(report.Results, report.SkillResults...) {
		if r.Status == "skipped" {
			skipped++
		}
	}
	if skipped != 2 {
		t.Fatalf("应有 2 条 skipped 记录（规范入口+skill 入口），实际 %d; report=%+v", skipped, report)
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

func TestDefaultGlobalConfigIncludesGenericAgents(t *testing.T) {
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	if !hasPathSuffix(targetPaths(cfg.Targets), filepath.Join(".agents", "AGENTS.md")) {
		t.Fatalf("通用跨工具规范入口缺失: %+v", cfg.Targets)
	}
	if !hasPathSuffix(skillTargetPaths(cfg.SkillTargets), filepath.Join(".agents", "skills")) {
		t.Fatalf("通用跨工具 Skill 入口缺失: %+v", cfg.SkillTargets)
	}
}

func TestDefaultGlobalConfigIncludesJoyCode(t *testing.T) {
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	if !hasPathSuffix(targetPaths(cfg.Targets), filepath.Join(".joycode", "AGENTS.md")) {
		t.Fatalf("JoyCode 规范入口缺失: %+v", cfg.Targets)
	}
	if !hasPathSuffix(skillTargetPaths(cfg.SkillTargets), filepath.Join(".joycode", "skills")) {
		t.Fatalf("JoyCode Skill 入口缺失: %+v", cfg.SkillTargets)
	}
}

func TestDefaultGlobalConfigIncludesCursor(t *testing.T) {
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	var cursorTarget *Target
	for i := range cfg.Targets {
		if strings.HasSuffix(cfg.Targets[i].Path, filepath.Join(".cursor", "rules", "AGENTS.mdc")) {
			cursorTarget = &cfg.Targets[i]
			break
		}
	}
	if cursorTarget == nil {
		t.Fatalf("Cursor 规范入口缺失: %+v", cfg.Targets)
	}
	if cursorTarget.Mode != "cursor" {
		t.Fatalf("Cursor 规范入口 Mode 应为 cursor，实际 %q", cursorTarget.Mode)
	}
	if !strings.HasSuffix(cursorTarget.Detect, filepath.Join(".cursor")) {
		t.Fatalf("Cursor Detect 应为 ~/.cursor，实际 %q", cursorTarget.Detect)
	}
	if !hasPathSuffix(skillTargetPaths(cfg.SkillTargets), filepath.Join(".cursor", "skills")) {
		t.Fatalf("Cursor Skill 入口缺失: %+v", cfg.SkillTargets)
	}
}

func TestCursorRuleCreatesFrontmatterAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cursorHome := filepath.Join(dir, ".cursor")
	if err := os.MkdirAll(cursorHome, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source: filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(cursorHome, "rules", "AGENTS.mdc"), Mode: "cursor", Detect: cursorHome},
		},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Source, []byte("# hello\nkeep rules short\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	data, err := os.ReadFile(cfg.Targets[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.HasPrefix(got, "---\n") || !strings.Contains(got, "alwaysApply: true") {
		t.Fatalf("cursor rule missing frontmatter: %q; report=%+v", got, report)
	}
	if !sameContent(cfg.Source, cfg.Targets[0].Path) {
		t.Fatalf("cursor rule payload does not match source: %q", got)
	}
	if !strings.Contains(got, "keep rules short") {
		t.Fatalf("cursor rule missing source body: %q", got)
	}

	report, err = syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("second syncConfig() error = %v", err)
	}
	for _, result := range report.Results {
		if result.Path == cfg.Targets[0].Path && result.Status != "ok" {
			t.Fatalf("second run should be idempotent, got %+v", report.Results)
		}
	}
}

func TestStaleManagedCursorRuleDoesNotPolluteSource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cursorHome := filepath.Join(dir, ".cursor")
	if err := os.MkdirAll(cursorHome, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source: filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(cursorHome, "rules", "AGENTS.mdc"), Mode: "cursor", Detect: cursorHome},
		},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Source, []byte("# v1\nold body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("initial syncConfig() error = %v", err)
	}

	// 统一源已前进；Cursor 受管副本仍是旧正文。再同步不得把旧正文 append 回统一源。
	if err := os.WriteFile(cfg.Source, []byte("# v2\nnew body with extra section\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("resync after source edit error = %v", err)
	}
	source, err := os.ReadFile(cfg.Source)
	if err != nil {
		t.Fatal(err)
	}
	got := string(source)
	if strings.Contains(got, "agentsync:begin import") || strings.Contains(got, "Imported From") {
		t.Fatalf("stale cursor rule was imported into source: %q; report=%+v", got, report)
	}
	if strings.Contains(got, "old body") {
		t.Fatalf("source polluted with stale cursor body: %q; report=%+v", got, report)
	}
	if !strings.Contains(got, "new body with extra section") {
		t.Fatalf("source lost new content: %q", got)
	}
	if !sameContent(cfg.Source, cfg.Targets[0].Path) {
		t.Fatalf("cursor rule not refreshed from source; report=%+v", report)
	}
	for _, result := range report.Results {
		if result.Path == cfg.Targets[0].Path && result.Status != "replaced" {
			t.Fatalf("expected replaced stale cursor-rule, got %+v", result)
		}
	}
}

func TestStaleManagedCopyDoesNotPolluteSource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cfg := Config{
		Source:  filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{{Path: filepath.Join(dir, "copy", "AGENTS.md"), Mode: "link"}},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Source, []byte("v1 managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedCopy(cfg.Source, cfg.Targets[0].Path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Source, []byte("v2 source only\n"), 0o644); err != nil {
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
	got := string(source)
	if strings.Contains(got, "agentsync:begin import") || strings.Contains(got, "v1 managed") {
		t.Fatalf("stale managed copy polluted source: %q; report=%+v", got, report)
	}
	if got != "v2 source only\n" {
		t.Fatalf("source changed unexpectedly: %q", got)
	}
}

func TestCursorRuleReplacesBareSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cursorHome := filepath.Join(dir, ".cursor")
	cfg := Config{
		Source: filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(cursorHome, "rules", "AGENTS.mdc"), Mode: "cursor", Detect: cursorHome},
		},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Source, []byte("source body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Targets[0].Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cfg.Source, cfg.Targets[0].Path); err != nil {
		t.Fatal(err)
	}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	info, err := os.Lstat(cfg.Targets[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("cursor rule should not remain a symlink; report=%+v", report)
	}
	data, err := os.ReadFile(cfg.Targets[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "alwaysApply: true") {
		t.Fatalf("replaced cursor rule missing frontmatter: %q", data)
	}
}

func TestDefaultGlobalConfigGatesEveryTarget(t *testing.T) {
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	for _, tgt := range cfg.Targets {
		if tgt.Detect == "" {
			t.Fatalf("规范入口缺少 Detect 门控: %s", tgt.Path)
		}
	}
	for _, tgt := range cfg.SkillTargets {
		if tgt.Detect == "" {
			t.Fatalf("Skill 入口缺少 Detect 门控: %s", tgt.Path)
		}
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
