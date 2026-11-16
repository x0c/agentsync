package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// overlayFixture creates an isolated canonical source and overlay directory and
// returns them with the configuration that wires them together.
func overlayFixture(t *testing.T) (string, string, string) {
	t.Helper()
	home := hardeningHome(t)
	source := filepath.Join(home, "config", "AGENTS.md")
	overlayDir := filepath.Join(home, "config", "instructions")
	hardeningWrite(t, source, "BASE_RULE\n")
	return home, source, overlayDir
}

func overlayTarget(t *testing.T, name, path string) Target {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return Target{Name: name, Path: path, Mode: "link", Detect: filepath.Dir(path)}
}

func overlayResult(t *testing.T, names []TargetResult, path string) TargetResult {
	t.Helper()
	for _, result := range names {
		if result.Path == path {
			return result
		}
	}
	t.Fatalf("no report entry for %s in %+v", path, names)
	return TargetResult{}
}

func TestInstructionOverlayAppliesToOneRuntime(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	codexPath := filepath.Join(home, ".codex", "AGENTS.md")
	overlayWrite := filepath.Join(overlayDir, "pi.md")
	hardeningWrite(t, overlayWrite, "PI_ONLY_RULE: never call the banned tool.\n")
	cfg := Config{
		Source: source,
		Targets: []Target{
			overlayTarget(t, "pi", piPath),
			overlayTarget(t, "codex", codexPath),
		},
		InstructionOverlayDir: overlayDir,
	}

	if _, err := syncConfig(cfg, Options{Check: true}); err != nil {
		t.Fatalf("preview error = %v", err)
	}
	if pathExists(piPath) {
		t.Fatal("--check wrote the Pi target")
	}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	piResult := overlayResult(t, report.Results, piPath)
	if piResult.Status != "linked" || piResult.Detail != "managed copy with the pi overlay" {
		t.Fatalf("unexpected Pi result %+v", piResult)
	}
	piBody := hardeningRead(t, piPath)
	for _, want := range []string{"BASE_RULE", "PI_ONLY_RULE", `overlay="pi"`, `<!-- agentsync:begin overlay target="pi" -->`, "<!-- agentsync:end overlay -->"} {
		if !strings.Contains(piBody, want) {
			t.Fatalf("Pi copy missing %q: %q", want, piBody)
		}
	}
	if strings.HasPrefix(piBody, "---\n") {
		t.Fatal("plain Markdown target must not receive YAML frontmatter")
	}
	if !symlinkPointsTo(codexPath, source) {
		t.Fatal("a runtime without an overlay must stay a symlink")
	}
	if strings.Contains(hardeningRead(t, codexPath), "PI_ONLY_RULE") {
		t.Fatal("overlay leaked into an unrelated runtime")
	}
	if got := hardeningRead(t, source); got != "BASE_RULE\n" {
		t.Fatalf("overlay modified the canonical source: %q", got)
	}
	if managedCopyOverlay(piPath) != "pi" {
		t.Fatalf("overlay marker not recorded on %s", piPath)
	}

	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	for _, result := range recheck.Results {
		if result.Status != "ok" {
			t.Fatalf("first apply did not converge: %+v", result)
		}
	}
}

func TestInstructionOverlayRefreshNeverImportsGeneratedBody(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE_V1\n")
	cfg := Config{
		Source:                source,
		Targets:               []Target{overlayTarget(t, "pi", piPath)},
		InstructionOverlayDir: overlayDir,
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("initial sync error = %v", err)
	}

	hardeningWrite(t, source, "BASE_RULE_V2\n")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE_V2\n")
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("refresh error = %v", err)
	}
	if got := overlayResult(t, report.Results, piPath).Status; got != "replaced" {
		t.Fatalf("expected replaced on fragment change, got %q (%+v)", got, report.Results)
	}
	body := hardeningRead(t, piPath)
	for _, want := range []string{"BASE_RULE_V2", "PI_ONLY_RULE_V2"} {
		if !strings.Contains(body, want) {
			t.Fatalf("refreshed copy missing %q: %q", want, body)
		}
	}
	if strings.Contains(body, "PI_ONLY_RULE_V1") {
		t.Fatalf("stale fragment survived: %q", body)
	}
	if got := hardeningRead(t, source); got != "BASE_RULE_V2\n" {
		t.Fatalf("generated copy leaked into the source: %q", got)
	}
	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	if got := overlayResult(t, recheck.Results, piPath).Status; got != "ok" {
		t.Fatalf("refresh did not converge: %q (%+v)", got, recheck.Results)
	}
}

func TestInstructionOverlayRemovalRestoresSymlink(t *testing.T) {
	for _, blank := range []bool{false, true} {
		name := "delete"
		if blank {
			name = "blank"
		}
		t.Run(name, func(t *testing.T) {
			home, source, overlayDir := overlayFixture(t)
			piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
			overlayPath := filepath.Join(overlayDir, "pi.md")
			hardeningWrite(t, overlayPath, "PI_ONLY_RULE\n")
			cfg := Config{
				Source:                source,
				Targets:               []Target{overlayTarget(t, "pi", piPath)},
				InstructionOverlayDir: overlayDir,
			}
			if _, err := syncConfig(cfg, Options{}); err != nil {
				t.Fatalf("initial sync error = %v", err)
			}
			if blank {
				hardeningWrite(t, overlayPath, "  \n\n")
			} else if err := os.Remove(overlayPath); err != nil {
				t.Fatal(err)
			}

			report, err := syncConfig(cfg, Options{})
			if err != nil {
				t.Fatalf("removal sync error = %v", err)
			}
			result := overlayResult(t, report.Results, piPath)
			if result.Status != "replaced" || !strings.Contains(result.Detail, "restored") {
				t.Fatalf("unexpected removal result %+v", result)
			}
			if !symlinkPointsTo(piPath, source) {
				t.Fatal("target did not return to a symlink once the overlay was gone")
			}
			if got := hardeningRead(t, piPath); got != "BASE_RULE\n" {
				t.Fatalf("restored symlink content = %q", got)
			}
			if len(report.Backups) == 0 {
				t.Fatal("removal must back up the previous managed copy")
			}
			recheck, err := syncConfig(cfg, Options{Check: true})
			if err != nil {
				t.Fatalf("recheck error = %v", err)
			}
			if got := overlayResult(t, recheck.Results, piPath).Status; got != "ok" {
				t.Fatalf("removal did not converge: %q (%+v)", got, recheck.Results)
			}
		})
	}
}

func TestInstructionOverlayReplacesExistingSymlink(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	target := overlayTarget(t, "pi", piPath)
	if err := os.Symlink(source, piPath); err != nil {
		skipOnLinkPrivilegeDenied(t, err)
	}
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{Source: source, Targets: []Target{target}, InstructionOverlayDir: overlayDir}

	check, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}
	if got := overlayResult(t, check.Results, piPath).Status; got != "wrong-link" {
		t.Fatalf("expected wrong-link before conversion, got %q (%+v)", got, check.Results)
	}
	if !symlinkPointsTo(piPath, source) {
		t.Fatal("preview replaced the symlink")
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("conversion error = %v", err)
	}
	info, err := os.Lstat(piPath)
	if err != nil {
		t.Fatal(err)
	}
	if isManagedLink(info) {
		t.Fatal("overlay target must not stay a symlink")
	}
	if !strings.Contains(hardeningRead(t, piPath), "PI_ONLY_RULE") {
		t.Fatal("converted copy lost the runtime fragment")
	}
}

func TestInstructionOverlayPreservesUnmanagedTargetContent(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	target := overlayTarget(t, "pi", piPath)
	hardeningWrite(t, piPath, "HAND_WRITTEN_RULE\n")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{Source: source, Targets: []Target{target}, InstructionOverlayDir: overlayDir}

	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	body := hardeningRead(t, piPath)
	for _, want := range []string{"BASE_RULE", "HAND_WRITTEN_RULE", "PI_ONLY_RULE"} {
		if !strings.Contains(body, want) {
			t.Fatalf("managed copy missing %q: %q", want, body)
		}
	}
	if got := hardeningRead(t, source); !strings.Contains(got, "HAND_WRITTEN_RULE") {
		t.Fatalf("unique hand-written content was not imported: %q", got)
	}
}

func TestInstructionOverlayCursorKeepsFrontmatter(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	cursorPath := filepath.Join(home, ".cursor", "rules", "AGENTS.mdc")
	target := overlayTarget(t, "cursor", cursorPath)
	target.Mode = "cursor"
	hardeningWrite(t, filepath.Join(overlayDir, "cursor.md"), "CURSOR_ONLY_RULE\n")
	cfg := Config{Source: source, Targets: []Target{target}, InstructionOverlayDir: overlayDir}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	if got := overlayResult(t, report.Results, cursorPath).Detail; got != "cursor-rule" {
		t.Fatalf("unexpected cursor detail %q", got)
	}
	body := hardeningRead(t, cursorPath)
	for _, want := range []string{"alwaysApply: true", "BASE_RULE", "CURSOR_ONLY_RULE", `overlay="cursor"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("cursor rule missing %q: %q", want, body)
		}
	}
	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	if got := overlayResult(t, recheck.Results, cursorPath).Status; got != "ok" {
		t.Fatalf("cursor overlay did not converge: %q (%+v)", got, recheck.Results)
	}

	// A refresh must keep Cursor's frontmatter, not write a bare managed copy.
	hardeningWrite(t, source, "BASE_RULE_V2\n")
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("refresh error = %v", err)
	}
	refreshed := hardeningRead(t, cursorPath)
	if !strings.HasPrefix(refreshed, cursorFrontmatter) {
		t.Fatalf("refreshed cursor rule lost frontmatter: %q", refreshed)
	}
	for _, want := range []string{"BASE_RULE_V2", "CURSOR_ONLY_RULE"} {
		if !strings.Contains(refreshed, want) {
			t.Fatalf("refreshed cursor rule missing %q: %q", want, refreshed)
		}
	}

	// Removing the fragment rewrites the rule without an overlay marker.
	if err := os.Remove(filepath.Join(overlayDir, "cursor.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("removal error = %v", err)
	}
	if body := hardeningRead(t, cursorPath); strings.Contains(body, "CURSOR_ONLY_RULE") || managedCopyOverlay(cursorPath) != "" {
		t.Fatalf("cursor rule kept the removed fragment: %q", body)
	}
	if !strings.Contains(hardeningRead(t, cursorPath), "alwaysApply: true") {
		t.Fatal("cursor rule lost frontmatter after overlay removal")
	}
}

func TestInstructionOverlayAntigravityThresholds(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	configDir := filepath.Join(home, ".gemini", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	primary := filepath.Join(home, ".gemini", "AGENTS.md")
	overlayPath := filepath.Join(overlayDir, "antigravity.md")
	cfg := Config{
		Source: source,
		Targets: []Target{{
			Name: "antigravity", Path: primary, Mode: "antigravity", Detect: configDir,
		}},
		InstructionOverlayDir: overlayDir,
	}
	rulesDir := filepath.Join(configDir, "rules")

	// Small merged body: one managed file without frontmatter.
	hardeningWrite(t, overlayPath, "ANTIGRAVITY_ONLY_RULE\n")
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("small sync error = %v", err)
	}
	small := hardeningRead(t, primary)
	if strings.HasPrefix(small, "---\n") {
		t.Fatal("standalone ~/.gemini/AGENTS.md must not carry frontmatter")
	}
	for _, want := range []string{"BASE_RULE", "ANTIGRAVITY_ONLY_RULE", `overlay="antigravity"`} {
		if !strings.Contains(small, want) {
			t.Fatalf("small antigravity copy missing %q: %q", want, small)
		}
	}
	if info, err := os.Lstat(primary); err != nil || !info.Mode().IsRegular() {
		t.Fatal("antigravity overlay must not use a symlink")
	}
	if parts, err := listAntigravityPartFiles(rulesDir); err != nil || len(parts) != 0 {
		t.Fatalf("unexpected split parts for a small source: %v %v", parts, err)
	}

	// Fragments do not have to fit: crossing the threshold splits the merge.
	large := buildLargeAntigravitySource(21000)
	hardeningWrite(t, overlayPath, strings.Repeat("ANTIGRAVITY_OVERLAY_LINE\n", 200))
	if err := os.WriteFile(source, []byte(large), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("split sync error = %v", err)
	}
	parts, err := listAntigravityPartFiles(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) == 0 {
		t.Fatal("expected antigravity split rules once the merged body exceeds 22 KB")
	}
	joined := hardeningRead(t, primary)
	for _, part := range parts {
		joined += hardeningRead(t, part)
	}
	if !strings.Contains(joined, "ANTIGRAVITY_OVERLAY_LINE") {
		t.Fatal("split rules lost the runtime fragment")
	}
	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	if got := antigravityStatusOf(recheck.Results, primary); got != "ok" {
		t.Fatalf("split with overlay did not converge: %q (%+v)", got, recheck.Results)
	}

	// Shrinking back below the threshold prunes parts and keeps the fragment.
	hardeningWrite(t, source, "SMALL_AGAIN\n")
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("shrink error = %v", err)
	}
	if parts, err := listAntigravityPartFiles(rulesDir); err != nil || len(parts) != 0 {
		t.Fatalf("split parts were not pruned after shrinking: %v %v", parts, err)
	}
	if body := hardeningRead(t, primary); !strings.Contains(body, "ANTIGRAVITY_OVERLAY_LINE") || !strings.Contains(body, "SMALL_AGAIN") {
		t.Fatalf("shrunk antigravity copy lost content: %q", body)
	}

	// Dropping the fragment restores the symlink.
	if err := os.Remove(overlayPath); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("overlay removal error = %v", err)
	}
	if !symlinkPointsTo(primary, source) {
		t.Fatal("antigravity target did not return to a symlink")
	}
}

func TestInstructionOverlayWindsurfLimitWarning(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	windsurfPath := filepath.Join(home, ".codeium", "windsurf", "memories", "global_rules.md")
	if err := os.MkdirAll(filepath.Join(home, ".codeium", "windsurf"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := Target{Name: "windsurf", Path: windsurfPath, Mode: "link", Detect: filepath.Join(home, ".codeium", "windsurf")}
	hardeningWrite(t, filepath.Join(overlayDir, "windsurf.md"), strings.Repeat("W", 6100))
	cfg := Config{Source: source, Targets: []Target{target}, InstructionOverlayDir: overlayDir}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	var warned bool
	for _, result := range report.Results {
		if result.Status == "warning" && result.Path == windsurfPath && strings.Contains(result.Detail, "6000") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected a Windsurf limit warning, got %+v", report.Results)
	}
	if !strings.Contains(hardeningRead(t, windsurfPath), "WWWW") {
		t.Fatal("windsurf copy missing the overlay body")
	}

	// A short fragment stays silent.
	hardeningWrite(t, filepath.Join(overlayDir, "windsurf.md"), "SHORT_RULE\n")
	quiet, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("short sync error = %v", err)
	}
	for _, result := range quiet.Results {
		if result.Status == "warning" {
			t.Fatalf("unexpected warning for a short body: %+v", result)
		}
	}
}

func TestInstructionOverlayNamesAndValidation(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	cfg := Config{
		Source:                source,
		Targets:               []Target{overlayTarget(t, "pi", piPath)},
		InstructionOverlayDir: overlayDir,
	}

	hardeningWrite(t, filepath.Join(overlayDir, "typo.md"), "UNKNOWN_RUNTIME_RULE\n")
	hardeningWrite(t, filepath.Join(overlayDir, "notes.txt"), "ignored\n")
	hardeningWrite(t, filepath.Join(overlayDir, ".DS_Store"), "ignored\n")
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("unknown runtime must warn, not fail: %v", err)
	}
	var warned bool
	for _, result := range report.Results {
		if result.Status == "warning" && result.Path == filepath.Join(overlayDir, "typo.md") {
			if !strings.Contains(result.Detail, "unknown runtime") || !strings.Contains(result.Detail, "pi") {
				t.Fatalf("unhelpful warning detail: %q", result.Detail)
			}
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected an unknown runtime warning, got %+v", report.Results)
	}
	if !symlinkPointsTo(piPath, source) || strings.Contains(hardeningRead(t, piPath), "UNKNOWN_RUNTIME_RULE") {
		t.Fatal("an unknown fragment must not change a runtime without an overlay")
	}

	// Case-insensitive duplicates are rejected before any target is written.
	// A case-insensitive filesystem merges the two names into one file, so the
	// conflict itself is covered by duplicateOverlayKey below.
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_RULE\n")
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("single fragment must sync: %v", err)
	}
	if got := hardeningRead(t, piPath); !strings.Contains(got, "PI_RULE") {
		t.Fatalf("Pi fragment missing: %q", got)
	}
}

func TestInstructionOverlayDuplicateKeysRejected(t *testing.T) {
	files := []instructionOverlayFile{
		{key: "pi", path: "/config/instructions/pi.md"},
		{key: "codex", path: "/config/instructions/codex.md"},
		{key: "pi", path: "/config/instructions/PI.md"},
	}
	first, second, duplicate := duplicateOverlayKey(files)
	if !duplicate {
		t.Fatal("case-insensitive duplicate keys must be detected")
	}
	if first.path != "/config/instructions/pi.md" || second.path != "/config/instructions/PI.md" {
		t.Fatalf("duplicate report names the wrong files: %v %v", first, second)
	}
	if _, _, duplicate := duplicateOverlayKey(files[:2]); duplicate {
		t.Fatal("distinct runtimes must not conflict")
	}
}

func TestInstructionOverlayCheckIsReadOnly(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	overlayPath := filepath.Join(overlayDir, "pi.md")
	hardeningWrite(t, overlayPath, "PI_ONLY_RULE\n")
	cfg := Config{
		Source:                source,
		Targets:               []Target{overlayTarget(t, "pi", piPath)},
		InstructionOverlayDir: overlayDir,
	}
	report, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}
	if got := overlayResult(t, report.Results, piPath).Status; got != "missing" {
		t.Fatalf("expected missing in preview, got %q", got)
	}
	if pathExists(piPath) {
		t.Fatal("preview created the managed copy")
	}
	if got := hardeningRead(t, overlayPath); got != "PI_ONLY_RULE\n" {
		t.Fatalf("preview changed the fragment: %q", got)
	}
}

func TestInstructionOverlayOnlyInGlobalMode(t *testing.T) {
	home, _, overlayDir := overlayFixture(t)
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "GLOBAL_ONLY_RULE\n")
	if dir := repoConfig(home).InstructionOverlayDir; dir != "" {
		t.Fatalf("repository and batch modes must not read global overlays: %q", dir)
	}
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	if cfg.InstructionOverlayDir == "" {
		t.Fatal("global mode must expose the overlay directory")
	}
}

func TestInstructionOverlayWatchSnapshot(t *testing.T) {
	home := hardeningHome(t)
	source := filepath.Join(home, "config", "AGENTS.md")
	overlayDir := filepath.Join(home, "config", "instructions")
	hardeningWrite(t, source, "BASE_RULE\n")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_RULE_V1\n")
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	cfg.Source = source
	cfg.InstructionOverlayDir = overlayDir
	for i := range cfg.Targets {
		cfg.Targets[i].Detect = ""
		cfg.Targets[i].Path = filepath.Join(home, "targets", cfg.Targets[i].Name+".md")
	}
	if _, err := syncConfig(cfg, Options{Check: true}); err != nil {
		t.Fatalf("preview error = %v", err)
	}
	before, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatalf("snapshot error = %v", err)
	}
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_RULE_V2\n")
	after, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatalf("snapshot error = %v", err)
	}
	if before.equal(after) {
		t.Fatal("watch did not notice a fragment change")
	}
	if !watchSkipMCP(before, after) {
		t.Fatal("a fragment-only change must not rewrite MCP configuration")
	}
}

func TestInstructionOverlayDoesNotSeedNewSource(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{
		Source:                source,
		Targets:               []Target{overlayTarget(t, "pi", piPath)},
		InstructionOverlayDir: overlayDir,
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("initial sync error = %v", err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("source recreation error = %v", err)
	}
	if got := hardeningRead(t, source); strings.Contains(got, "PI_ONLY_RULE") {
		t.Fatalf("generated overlay body seeded the new source: %q", got)
	}
	if got := hardeningRead(t, piPath); !strings.Contains(got, "PI_ONLY_RULE") {
		t.Fatalf("recreated source did not reapply the fragment: %q", got)
	}
}
