package agentsync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func buildLargeAntigravitySource(targetBytes int) string {
	var b strings.Builder
	b.WriteString("# Global Agent Specification\n\n")
	for sec := 1; b.Len() < targetBytes; sec++ {
		fmt.Fprintf(&b, "## Section %02d — Detailed Runtime Guidance\n\n", sec)
		fmt.Fprintf(&b, "- Rule %02d.A: Keep canonical instructions in one file and link or split safely across runtimes.\n", sec)
		fmt.Fprintf(&b, "- Rule %02d.B: Avoid truncating instructions mid-fence or mid-UTF-8 rune; preserve all policy sections.\n\n", sec)
		b.WriteString("```sh\n# Fenced block inside section should remain balanced\necho \"section=")
		fmt.Fprintf(&b, "%02d", sec)
		b.WriteString("\"\n```\n\n")
	}
	return b.String()
}

func TestAntigravitySmallSourceUsesLinkAndPrunesLeftovers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(home, ".config", "agentsync"))

	configDir := filepath.Join(home, ".gemini", "config")
	rulesDir := filepath.Join(configDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(rulesDir, "agentsync-part-02.md")
	if err := os.WriteFile(leftover, []byte("---\ntrigger: always_on\n---\nstale part\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(rulesDir, "user-custom.md")
	if err := os.WriteFile(unrelated, []byte("---\ntrigger: always_on\n---\ncustom user rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(home, ".config", "agentsync", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("# Small Canonical\nShort instructions.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	primary := filepath.Join(home, ".gemini", "AGENTS.md")
	cfg := Config{
		Source: source,
		Targets: []Target{
			{Path: primary, Mode: "antigravity", Detect: configDir},
		},
	}

	// Check reports replaceable without mutating leftover part.
	checkReport, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("syncConfig(--check) error = %v", err)
	}
	if got := antigravityStatusOf(checkReport.Results, primary); got != "replaceable" {
		t.Fatalf("expected replaceable before prune, got %q (%+v)", got, checkReport.Results)
	}
	if !pathExists(leftover) {
		t.Fatal("--check must not remove leftover part file")
	}

	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = endBackupSession() }()

	applyReport, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	if got := antigravityStatusOf(applyReport.Results, primary); got != "replaced" && got != "linked" {
		t.Fatalf("unexpected status after small sync: %q", got)
	}
	if !symlinkPointsTo(primary, source) {
		t.Fatalf("expected %s to link to %s", primary, source)
	}
	if pathExists(leftover) {
		t.Fatalf("expected leftover part %s to be pruned", leftover)
	}
	if !pathExists(unrelated) {
		t.Fatalf("must never remove unrelated user rule %s", unrelated)
	}
	if len(applyReport.Backups) == 0 {
		t.Fatal("expected backup for pruned leftover part")
	}

	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	if got := antigravityStatusOf(recheck.Results, primary); got != "ok" {
		t.Fatalf("expected ok after small convergence, got %q (%+v)", got, recheck.Results)
	}
}

func TestAntigravityLargeSourceSplitsIdempotentlyAndShrinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(home, ".config", "agentsync"))

	configDir := filepath.Join(home, ".gemini", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(home, ".config", "agentsync", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}

	// Start with 50 KB source -> 3 parts, convert from existing symlink to split files.
	large := buildLargeAntigravitySource(50000)
	if err := os.WriteFile(source, []byte(large), 0o644); err != nil {
		t.Fatal(err)
	}
	primary := filepath.Join(home, ".gemini", "AGENTS.md")
	if err := os.Symlink(source, primary); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Source: source,
		Targets: []Target{
			{Path: primary, Mode: "antigravity", Detect: configDir},
		},
	}

	// --check detects symlink conversion without creating rules/.
	checkBefore, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("checkBefore error = %v", err)
	}
	if got := antigravityStatusOf(checkBefore.Results, primary); got != "replaceable" {
		t.Fatalf("expected replaceable before split, got %q", got)
	}
	rulesDir := filepath.Join(configDir, "rules")
	if pathExists(rulesDir) {
		t.Fatal("--check must not create rules dir")
	}

	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	applyLarge, err := syncConfig(cfg, Options{})
	if err != nil {
		_ = endBackupSession()
		t.Fatalf("applyLarge error = %v", err)
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}
	if got := antigravityStatusOf(applyLarge.Results, primary); got != "replaced" {
		t.Fatalf("expected replaced when converting symlink -> split, got %q", got)
	}

	parts, err := listAntigravityPartFiles(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) < 2 {
		t.Fatalf("expected >=2 rule part files for 50 KB source, got %d (%v)", len(parts), parts)
	}
	primaryInfo, err := os.Lstat(primary)
	if err != nil || !primaryInfo.Mode().IsRegular() {
		t.Fatalf("primary %s must be a regular managed file, not symlink", primary)
	}
	primaryBytes, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(primaryBytes), "---\n") {
		t.Fatal("primary ~/.gemini/AGENTS.md must stay plain Markdown without YAML frontmatter")
	}
	if len(primaryBytes) > antigravityMaxChunkBytes || len(primaryBytes) >= 24000 {
		t.Fatalf("primary size %d exceeds limit", len(primaryBytes))
	}
	for _, part := range parts {
		partBytes, err := os.ReadFile(part)
		if err != nil {
			t.Fatal(err)
		}
		if len(partBytes) > antigravityMaxChunkBytes || len(partBytes) >= 24000 {
			t.Fatalf("part %s size %d exceeds limit", part, len(partBytes))
		}
		if !strings.HasPrefix(string(partBytes), "---\ntrigger: always_on\n") {
			t.Fatalf("part %s missing trigger: always_on frontmatter", part)
		}
		if !isManagedAgentsyncFile(part) {
			t.Fatalf("part %s not recognized by isManagedAgentsyncFile", part)
		}
	}

	// Second check and apply converge cleanly with zero re-import growth.
	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	if got := antigravityStatusOf(recheck.Results, primary); got != "ok" {
		t.Fatalf("expected ok on converged split, got %q (%+v)", got, recheck.Results)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("idempotent apply error = %v", err)
	}
	srcAfter, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(srcAfter) != large {
		t.Fatal("split files must never be re-imported into canonical source")
	}

	// Shrink from 50 KB -> 30 KB: extra trailing parts removed and backed up.
	medium := buildLargeAntigravitySource(30000)
	if err := os.WriteFile(source, []byte(medium), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	applyMedium, err := syncConfig(cfg, Options{})
	if err != nil {
		_ = endBackupSession()
		t.Fatalf("applyMedium error = %v", err)
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}
	if got := antigravityStatusOf(applyMedium.Results, primary); got != "replaced" {
		t.Fatalf("expected replaced after shrink, got %q", got)
	}
	partsMedium, err := listAntigravityPartFiles(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(partsMedium) != 1 {
		t.Fatalf("expected 1 rule part for 30 KB source, got %d (%v)", len(partsMedium), partsMedium)
	}

	// Rollback restores previous 50 KB split primary file safely.
	if err := runRollback(Options{Rollback: "latest"}); err != nil {
		t.Fatalf("runRollback(latest) error = %v", err)
	}
	restoredPrimary, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	if string(restoredPrimary) != string(primaryBytes) {
		t.Fatal("rollback latest did not restore previous split primary file")
	}

	// Shrink further from current state down to 4 KB: restores symlink & removes all parts.
	small := "# Small Again\nCompact body.\n"
	if err := os.WriteFile(source, []byte(small), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	applySmall, err := syncConfig(cfg, Options{})
	if err != nil {
		_ = endBackupSession()
		t.Fatalf("applySmall error = %v", err)
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}
	if got := antigravityStatusOf(applySmall.Results, primary); got != "replaced" {
		t.Fatalf("expected replaced when shrinking back to link, got %q", got)
	}
	if !symlinkPointsTo(primary, source) {
		t.Fatal("primary should revert to symlink once source <= 22 KB")
	}
	partsAfterSmall, err := listAntigravityPartFiles(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(partsAfterSmall) != 0 {
		t.Fatalf("expected all split parts removed, got %v", partsAfterSmall)
	}
}

func TestAntigravitySplitUTF8WithoutSpaces(t *testing.T) {
	sample := strings.Repeat("\u6c49", 10000)
	chunks := splitAntigravityRules("", []byte(sample), antigravityMaxChunkBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks for 30 KB CJK text, got %d", len(chunks))
	}
	var joined strings.Builder
	for i, c := range chunks {
		if !utf8.ValidString(c) {
			t.Fatalf("chunk %d invalid UTF-8", i)
		}
		if len(c) == 0 || len(c) >= antigravityMaxChunkBytes {
			t.Fatalf("chunk %d invalid size %d", i, len(c))
		}
		joined.WriteString(c)
	}
	if joined.String() != sample {
		t.Fatal("UTF-8 chunking altered content")
	}
}

func TestGeminiCLISkippedOnAntigravityOnlyHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	geminiHome := filepath.Join(home, ".gemini")
	if err := os.MkdirAll(filepath.Join(geminiHome, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(home, ".config", "agentsync", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("# Canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing agentsync symlink + mcpServers-only settings.json from older agentsync
	// must NOT self-trigger Gemini CLI detection.
	if err := os.Symlink(source, filepath.Join(geminiHome, "GEMINI.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(geminiHome, "settings.json"), []byte(`{"mcpServers":{"demo":{"command":"echo"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	if got := detectForInstruction(cfg.Targets, filepath.Join(".gemini", "GEMINI.md")); pathExists(got) {
		t.Fatalf("expected Antigravity-only host to skip GEMINI.md, but Detect %q exists", got)
	}
	if got := detectForMCP(cfg.MCPTargets, "gemini"); pathExists(got) {
		t.Fatalf("expected Antigravity-only host to skip gemini MCP, but Detect %q exists", got)
	}

	// Installing a real Gemini CLI marker re-enables Gemini CLI targets.
	if err := os.WriteFile(filepath.Join(geminiHome, "oauth_creds.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg2, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	if got := detectForInstruction(cfg2.Targets, filepath.Join(".gemini", "GEMINI.md")); !pathExists(got) {
		t.Fatalf("expected oauth_creds.json to enable GEMINI.md, Detect %q missing", got)
	}
	if got := detectForMCP(cfg2.MCPTargets, "gemini"); !pathExists(got) {
		t.Fatalf("expected oauth_creds.json to enable gemini MCP, Detect %q missing", got)
	}
}

func detectForInstruction(targets []Target, suffix string) string {
	for _, tgt := range targets {
		if strings.HasSuffix(tgt.Path, suffix) {
			return tgt.Detect
		}
	}
	return ""
}

func detectForMCP(targets []MCPTarget, name string) string {
	for _, tgt := range targets {
		if tgt.Name == name {
			return tgt.Detect
		}
	}
	return ""
}

func antigravityStatusOf(results []TargetResult, path string) string {
	for _, r := range results {
		if r.Path == path {
			return r.Status
		}
	}
	return ""
}
