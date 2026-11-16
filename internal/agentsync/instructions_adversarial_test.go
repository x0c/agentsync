package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// overlayAdversarialBeginBackup starts a shared backup session so every backup
// in one apply lands in the same stamp and can be inspected through its
// manifest. The cleanup keeps a failed assertion from leaving a session open.
func overlayAdversarialBeginBackup(t *testing.T) string {
	t.Helper()
	stamp, err := beginBackupSession()
	if err != nil {
		t.Fatalf("beginBackupSession() error = %v", err)
	}
	t.Cleanup(func() { _ = endBackupSession() })
	return stamp
}

func overlayAdversarialEndBackup(t *testing.T) {
	t.Helper()
	if err := endBackupSession(); err != nil {
		t.Fatalf("endBackupSession() error = %v", err)
	}
}

func overlayAdversarialManifest(t *testing.T, stamp string) backupManifest {
	t.Helper()
	stampDir, err := stampDirFor(stamp)
	if err != nil {
		t.Fatalf("stampDirFor(%q) error = %v", stamp, err)
	}
	manifest, err := loadBackupManifest(stampDir)
	if err != nil {
		t.Fatalf("loadBackupManifest(%q) error = %v", stampDir, err)
	}
	return manifest
}

func overlayAdversarialEntry(t *testing.T, manifest backupManifest, original string) backupManifestEntry {
	t.Helper()
	abs, err := filepath.Abs(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Entries {
		if samePath(entry.Original, abs) {
			return entry
		}
	}
	t.Fatalf("manifest has no entry for %s: %+v", abs, manifest.Entries)
	return backupManifestEntry{}
}

func overlayAdversarialStamps(t *testing.T) string {
	t.Helper()
	stamps, err := listBackupStamps()
	if err != nil {
		t.Fatalf("listBackupStamps() error = %v", err)
	}
	return strings.Join(stamps, ",")
}

// TestOverlayAdversarialForceReplacesUnmanagedTarget verifies that --force
// treats an unmanaged target as disposable: its content is backed up but never
// imported into the canonical source, and the result converges.
func TestOverlayAdversarialForceReplacesUnmanagedTarget(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	hardeningWrite(t, piPath, "HAND_WRITTEN_ONLY\n")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{
		Source:                source,
		Targets:               []Target{overlayTarget(t, "pi", piPath)},
		InstructionOverlayDir: overlayDir,
	}

	report, err := syncConfig(cfg, Options{Force: true})
	if err != nil {
		t.Fatalf("forced sync error = %v", err)
	}
	result := overlayResult(t, report.Results, piPath)
	if result.Status != "replaced" || !strings.Contains(result.Detail, "pi overlay") {
		t.Fatalf("unexpected forced result %+v", result)
	}
	if len(report.Backups) != 1 {
		t.Fatalf("forced replacement must back up the unmanaged file: %+v", report.Backups)
	}
	if got := hardeningRead(t, report.Backups[0]); got != "HAND_WRITTEN_ONLY\n" {
		t.Fatalf("backup lost the unmanaged content: %q", got)
	}
	if got := hardeningRead(t, source); strings.Contains(got, "HAND_WRITTEN_ONLY") {
		t.Fatalf("--force imported the unmanaged file into the source: %q", got)
	}
	body := hardeningRead(t, piPath)
	if strings.Contains(body, "HAND_WRITTEN_ONLY") {
		t.Fatalf("forced managed copy kept the unmanaged body: %q", body)
	}
	for _, want := range []string{"BASE_RULE", "PI_ONLY_RULE", `overlay="pi"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("forced managed copy missing %q: %q", want, body)
		}
	}

	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	if got := overlayResult(t, recheck.Results, piPath).Status; got != "ok" {
		t.Fatalf("forced apply did not converge: %q (%+v)", got, recheck.Results)
	}
}

// TestOverlayAdversarialCheckBetweenApplies verifies that a preview after
// convergence is read-only and agrees with the following apply, and that
// repeated applies are idempotent without allocating new backups.
func TestOverlayAdversarialCheckBetweenApplies(t *testing.T) {
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

	converged := hardeningRead(t, piPath)
	stampsAfterCreate := overlayAdversarialStamps(t)
	preview, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}
	if got := overlayResult(t, preview.Results, piPath).Status; got != "ok" {
		t.Fatalf("converged preview = %q, want ok (%+v)", got, preview.Results)
	}
	if got := hardeningRead(t, piPath); got != converged {
		t.Fatal("preview changed the managed copy")
	}
	again, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("repeat apply error = %v", err)
	}
	if got := overlayResult(t, again.Results, piPath).Status; got != "ok" {
		t.Fatalf("repeat apply = %q, want ok (%+v)", got, again.Results)
	}
	if len(again.Backups) != 0 {
		t.Fatalf("repeat apply created backups: %+v", again.Backups)
	}
	if got := overlayAdversarialStamps(t); got != stampsAfterCreate {
		t.Fatalf("repeat apply allocated a backup stamp: %q -> %q", stampsAfterCreate, got)
	}

	// A source-only edit is a pending overlay refresh: the preview must report
	// it, leave the old body in place, and the next apply must replace it once.
	hardeningWrite(t, source, "BASE_RULE_V2\n")
	pending := hardeningRead(t, piPath)
	pendingPreview, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("pending preview error = %v", err)
	}
	if got := overlayResult(t, pendingPreview.Results, piPath).Status; got != "replaceable" {
		t.Fatalf("pending preview = %q, want replaceable (%+v)", got, pendingPreview.Results)
	}
	if got := hardeningRead(t, piPath); got != pending {
		t.Fatal("pending preview rewrote the managed copy")
	}
	if got := overlayAdversarialStamps(t); got != stampsAfterCreate {
		t.Fatalf("pending preview allocated a backup stamp: %q -> %q", stampsAfterCreate, got)
	}
	refreshed, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("refresh apply error = %v", err)
	}
	if got := overlayResult(t, refreshed.Results, piPath).Status; got != "replaced" {
		t.Fatalf("refresh apply = %q, want replaced (%+v)", got, refreshed.Results)
	}
	if !strings.Contains(hardeningRead(t, piPath), "BASE_RULE_V2") {
		t.Fatal("refresh did not pick up the source-only edit")
	}
	stampsAfterRefresh := overlayAdversarialStamps(t)
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("post-refresh apply error = %v", err)
	}
	if got := overlayAdversarialStamps(t); got != stampsAfterRefresh {
		t.Fatalf("idempotent apply allocated a backup stamp: %q -> %q", stampsAfterRefresh, got)
	}
}

// TestOverlayAdversarialRollbackRestoresPreviousState verifies that rolling
// back an overlay apply restores the exact prior target type and content, and
// that the next preview reports the pending overlay conversion instead of ok.
func TestOverlayAdversarialRollbackRestoresPreviousState(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		home, source, overlayDir := overlayFixture(t)
		piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
		target := overlayTarget(t, "pi", piPath)
		if err := os.Symlink(source, piPath); err != nil {
			skipOnLinkPrivilegeDenied(t, err)
		}
		hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
		cfg := Config{Source: source, Targets: []Target{target}, InstructionOverlayDir: overlayDir}
		if _, err := syncConfig(cfg, Options{}); err != nil {
			t.Fatalf("overlay apply error = %v", err)
		}
		if got := managedCopyOverlay(piPath); got != "pi" {
			t.Fatalf("apply did not create a Pi managed copy: %q", got)
		}

		if err := runRollback(Options{Rollback: "latest"}); err != nil {
			t.Fatalf("rollback latest error = %v", err)
		}
		if !symlinkPointsTo(piPath, source) {
			t.Fatal("rollback did not restore the previous symlink")
		}
		if got := hardeningRead(t, piPath); got != "BASE_RULE\n" {
			t.Fatalf("restored symlink content = %q", got)
		}

		recheck, err := syncConfig(cfg, Options{Check: true})
		if err != nil {
			t.Fatalf("recheck error = %v", err)
		}
		if got := overlayResult(t, recheck.Results, piPath).Status; got != "wrong-link" {
			t.Fatalf("post-rollback preview = %q, want wrong-link (%+v)", got, recheck.Results)
		}
	})

	t.Run("file", func(t *testing.T) {
		home, source, overlayDir := overlayFixture(t)
		piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
		hardeningWrite(t, piPath, "ORIGINAL_ONLY_RULE\n")
		hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
		cfg := Config{
			Source:                source,
			Targets:               []Target{overlayTarget(t, "pi", piPath)},
			InstructionOverlayDir: overlayDir,
		}
		if _, err := syncConfig(cfg, Options{}); err != nil {
			t.Fatalf("overlay apply error = %v", err)
		}
		if got := managedCopyOverlay(piPath); got != "pi" {
			t.Fatalf("apply did not create a Pi managed copy: %q", got)
		}

		if err := runRollback(Options{Rollback: "latest"}); err != nil {
			t.Fatalf("rollback latest error = %v", err)
		}
		if info, err := os.Lstat(piPath); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("rollback did not restore the previous regular file: %v", err)
		}
		if got := hardeningRead(t, piPath); got != "ORIGINAL_ONLY_RULE\n" {
			t.Fatalf("restored file content = %q", got)
		}

		recheck, err := syncConfig(cfg, Options{Check: true})
		if err != nil {
			t.Fatalf("recheck error = %v", err)
		}
		if got := overlayResult(t, recheck.Results, piPath).Status; got != "mergeable" {
			t.Fatalf("post-rollback preview = %q, want mergeable (%+v)", got, recheck.Results)
		}
	})
}

// TestOverlayAdversarialExternalSymlinkImport verifies that an overlay target
// linked to an unrelated file imports its unique content, backs the link up as
// a link, and then materializes the merged managed copy.
func TestOverlayAdversarialExternalSymlinkImport(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	external := filepath.Join(home, "elsewhere", "external-rules.md")
	hardeningWrite(t, external, "EXTERNAL_ONLY_RULE\n")
	target := overlayTarget(t, "pi", piPath)
	if err := os.Symlink(external, piPath); err != nil {
		skipOnLinkPrivilegeDenied(t, err)
	}
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{Source: source, Targets: []Target{target}, InstructionOverlayDir: overlayDir}

	stamp := overlayAdversarialBeginBackup(t)
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		overlayAdversarialEndBackup(t)
		t.Fatalf("sync error = %v", err)
	}
	overlayAdversarialEndBackup(t)

	if got := overlayResult(t, report.Results, piPath).Status; got != "replaced" {
		t.Fatalf("external link conversion = %q (%+v)", got, report.Results)
	}
	manifest := overlayAdversarialManifest(t, stamp)
	entry := overlayAdversarialEntry(t, manifest, piPath)
	if entry.Kind != "symlink" {
		t.Fatalf("external link was backed up as %q, want symlink", entry.Kind)
	}
	stampDir, err := stampDirFor(stamp)
	if err != nil {
		t.Fatal(err)
	}
	saved := hardeningRead(t, filepath.Join(stampDir, entry.Name))
	if strings.TrimSpace(saved) != external {
		t.Fatalf("link backup target = %q, want %q", strings.TrimSpace(saved), external)
	}

	if got := hardeningRead(t, source); !strings.Contains(got, "EXTERNAL_ONLY_RULE") || strings.Contains(got, "PI_ONLY_RULE") {
		t.Fatalf("import/overlay separation failed: source = %q", got)
	}
	info, err := os.Lstat(piPath)
	if err != nil {
		t.Fatal(err)
	}
	if isManagedLink(info) || !info.Mode().IsRegular() {
		t.Fatal("external link did not become a managed regular copy")
	}
	body := hardeningRead(t, piPath)
	for _, want := range []string{"BASE_RULE", "EXTERNAL_ONLY_RULE", "PI_ONLY_RULE", `overlay="pi"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("managed copy missing %q: %q", want, body)
		}
	}
}

// TestOverlayAdversarialAbsentRuntimeSkipped verifies that a fragment for an
// uninstalled runtime stays a warning-free skip with no filesystem effects.
func TestOverlayAdversarialAbsentRuntimeSkipped(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	detect := filepath.Join(home, "not-installed")
	piPath := filepath.Join(detect, "AGENTS.md")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{
		Source: source,
		Targets: []Target{{
			Name: "pi", Path: piPath, Mode: "link", Detect: detect,
		}},
		InstructionOverlayDir: overlayDir,
	}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("sync error = %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("unexpected extra results for an absent runtime: %+v", report.Results)
	}
	result := overlayResult(t, report.Results, piPath)
	if result.Status != "skipped" {
		t.Fatalf("absent runtime = %q, want skipped (%+v)", result.Status, report.Results)
	}
	for _, candidate := range report.Results {
		if candidate.Status == "warning" {
			t.Fatalf("absent runtime produced warning noise: %+v", candidate)
		}
	}
	if pathExists(piPath) || pathExists(detect) {
		t.Fatal("absent runtime must not receive files or directories")
	}
	if len(report.Backups) != 0 {
		t.Fatalf("skipped runtime created backups: %+v", report.Backups)
	}
}

// TestOverlayAdversarialDirectoryTargetBlocked verifies that a directory at the
// target path is reported as blocked and is never modified or deleted.
func TestOverlayAdversarialDirectoryTargetBlocked(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	piPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	target := overlayTarget(t, "pi", piPath)
	if err := os.Mkdir(piPath, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(piPath, "keep.txt")
	hardeningWrite(t, keep, "KEEP_ME\n")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{Source: source, Targets: []Target{target}, InstructionOverlayDir: overlayDir}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("sync error = %v", err)
	}
	result := overlayResult(t, report.Results, piPath)
	if result.Status != "blocked" {
		t.Fatalf("directory target = %q, want blocked (%+v)", result.Status, report.Results)
	}
	info, err := os.Lstat(piPath)
	if err != nil || !info.IsDir() {
		t.Fatalf("blocked target directory was removed: %v", err)
	}
	if got := hardeningRead(t, keep); got != "KEEP_ME\n" {
		t.Fatalf("blocked target contents changed: %q", got)
	}
	if got := hardeningRead(t, source); got != "BASE_RULE\n" {
		t.Fatalf("blocked target modified the source: %q", got)
	}
	if len(report.Backups) != 0 {
		t.Fatalf("blocked target created backups: %+v", report.Backups)
	}
}

// TestOverlayAdversarialHandEditedCopyReplaced verifies that edits to a managed
// overlay copy are overwritten, not imported, and stay recoverable in backup.
func TestOverlayAdversarialHandEditedCopyReplaced(t *testing.T) {
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
	managed := hardeningRead(t, piPath)
	hardeningWrite(t, piPath, managed+"TAMPERED_RULE\n")

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("repair sync error = %v", err)
	}
	result := overlayResult(t, report.Results, piPath)
	if result.Status != "replaced" {
		t.Fatalf("hand-edited managed copy = %q, want replaced (%+v)", result.Status, report.Results)
	}
	if len(report.Backups) != 1 {
		t.Fatalf("edited copy was not backed up: %+v", report.Backups)
	}
	if got := hardeningRead(t, report.Backups[0]); !strings.Contains(got, "TAMPERED_RULE") || !strings.Contains(got, managed) {
		t.Fatalf("backup does not preserve the edited copy: %q", got)
	}
	if got := hardeningRead(t, piPath); strings.Contains(got, "TAMPERED_RULE") {
		t.Fatalf("edited body survived the repair: %q", got)
	}
	if got := hardeningRead(t, source); strings.Contains(got, "TAMPERED_RULE") {
		t.Fatalf("edited body was imported into the source: %q", got)
	}
	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	if got := overlayResult(t, recheck.Results, piPath).Status; got != "ok" {
		t.Fatalf("repair did not converge: %q (%+v)", got, recheck.Results)
	}
}

// TestOverlayAdversarialSharedRuntimeNameTargets verifies that two targets with
// one runtime name both receive the fragment and the run converges.
func TestOverlayAdversarialSharedRuntimeNameTargets(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	first := filepath.Join(home, "pi-a", "AGENTS.md")
	second := filepath.Join(home, "pi-b", "AGENTS.md")
	hardeningWrite(t, filepath.Join(overlayDir, "pi.md"), "PI_ONLY_RULE\n")
	cfg := Config{
		Source: source,
		Targets: []Target{
			overlayTarget(t, "pi", first),
			overlayTarget(t, "pi", second),
		},
		InstructionOverlayDir: overlayDir,
	}

	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("sync error = %v", err)
	}
	for _, path := range []string{first, second} {
		if got := overlayResult(t, report.Results, path).Status; got != "linked" {
			t.Fatalf("%s = %q, want linked (%+v)", path, got, report.Results)
		}
		if got := managedCopyOverlay(path); got != "pi" {
			t.Fatalf("%s did not record the pi overlay: %q", path, got)
		}
		if !strings.Contains(hardeningRead(t, path), "PI_ONLY_RULE") {
			t.Fatalf("%s is missing the shared fragment", path)
		}
	}
	if hardeningRead(t, first) != hardeningRead(t, second) {
		t.Fatal("targets sharing a runtime name received different bodies")
	}
	recheck, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("recheck error = %v", err)
	}
	for _, path := range []string{first, second} {
		if got := overlayResult(t, recheck.Results, path).Status; got != "ok" {
			t.Fatalf("%s did not converge: %q (%+v)", path, got, recheck.Results)
		}
	}
}

// TestOverlayAdversarialAntigravityRemovalPrunesParts verifies that removing an
// Antigravity overlay prunes leftover split rules with backups, and that the
// preview reports the prune without touching anything.
func TestOverlayAdversarialAntigravityRemovalPrunesParts(t *testing.T) {
	home, source, overlayDir := overlayFixture(t)
	configDir := filepath.Join(home, ".gemini", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	primary := filepath.Join(home, ".gemini", "AGENTS.md")
	overlayPath := filepath.Join(overlayDir, "antigravity.md")
	hardeningWrite(t, overlayPath, "ANTIGRAVITY_ONLY_RULE\n")
	cfg := Config{
		Source: source,
		Targets: []Target{{
			Name: "antigravity", Path: primary, Mode: "antigravity", Detect: configDir,
		}},
		InstructionOverlayDir: overlayDir,
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatalf("overlay sync error = %v", err)
	}
	if got := managedCopyOverlay(primary); got != "antigravity" {
		t.Fatalf("overlay sync did not create the Antigravity copy: %q", got)
	}

	rulesDir := antigravityRulesDir(primary)
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parts := []string{
		filepath.Join(rulesDir, "agentsync-part-01.md"),
		filepath.Join(rulesDir, "agentsync-part-02.md"),
	}
	for i, part := range parts {
		hardeningWrite(t, part, "STALE_PART_"+string(rune('A'+i))+"\n")
	}
	managed := hardeningRead(t, primary)
	if err := os.Remove(overlayPath); err != nil {
		t.Fatal(err)
	}

	stampsBefore := overlayAdversarialStamps(t)
	preview, err := syncConfig(cfg, Options{Check: true})
	if err != nil {
		t.Fatalf("removal preview error = %v", err)
	}
	result := overlayResult(t, preview.Results, primary)
	if result.Status != "replaceable" || !strings.Contains(result.Detail, "prune") {
		t.Fatalf("removal preview = %+v, want a pending prune", result)
	}
	if got := hardeningRead(t, primary); got != managed {
		t.Fatal("removal preview rewrote the Antigravity target")
	}
	for _, part := range parts {
		if !pathExists(part) {
			t.Fatalf("removal preview deleted %s", part)
		}
	}
	if got := overlayAdversarialStamps(t); got != stampsBefore {
		t.Fatalf("removal preview allocated a backup stamp: %q -> %q", stampsBefore, got)
	}

	stamp := overlayAdversarialBeginBackup(t)
	if _, err := syncConfig(cfg, Options{}); err != nil {
		overlayAdversarialEndBackup(t)
		t.Fatalf("removal sync error = %v", err)
	}
	overlayAdversarialEndBackup(t)
	if !overlayAliasRestored(primary, source) {
		t.Fatal("overlay removal did not restore the Antigravity symlink")
	}
	if remaining, err := listAntigravityPartFiles(rulesDir); err != nil || len(remaining) != 0 {
		t.Fatalf("leftover split rules survived removal: %v %v", remaining, err)
	}
	manifest := overlayAdversarialManifest(t, stamp)
	stampDir, err := stampDirFor(stamp)
	if err != nil {
		t.Fatal(err)
	}
	for i, part := range parts {
		entry := overlayAdversarialEntry(t, manifest, part)
		if entry.Kind != "file" {
			t.Fatalf("part %s was backed up as %q, want file", part, entry.Kind)
		}
		if got := hardeningRead(t, filepath.Join(stampDir, entry.Name)); got != "STALE_PART_"+string(rune('A'+i))+"\n" {
			t.Fatalf("backup for %s lost its content: %q", part, got)
		}
	}
	if entry := overlayAdversarialEntry(t, manifest, primary); entry.Kind != "file" {
		t.Fatalf("removed Antigravity copy was not backed up as a file: %+v", entry)
	}
}

// TestOverlayAdversarialWatchAddRemove verifies that the watcher notices a
// fragment appearing and disappearing and never schedules MCP rewrites for
// those fragment-only changes.
func TestOverlayAdversarialWatchAddRemove(t *testing.T) {
	home := hardeningHome(t)
	source := filepath.Join(home, "config", "AGENTS.md")
	overlayDir := filepath.Join(home, "config", "instructions")
	hardeningWrite(t, source, "BASE_RULE\n")
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

	missing, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatalf("snapshot without overlays error = %v", err)
	}
	fragment := filepath.Join(overlayDir, "pi.md")
	hardeningWrite(t, fragment, "PI_RULE\n")
	added, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatalf("snapshot after adding a fragment error = %v", err)
	}
	if missing.equal(added) {
		t.Fatal("watch did not notice a new fragment")
	}
	if !watchSkipMCP(missing, added) {
		t.Fatal("adding a fragment must not rewrite MCP configuration")
	}

	if err := os.Remove(fragment); err != nil {
		t.Fatal(err)
	}
	removed, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatalf("snapshot after removing a fragment error = %v", err)
	}
	if added.equal(removed) {
		t.Fatal("watch did not notice a removed fragment")
	}
	if !watchSkipMCP(added, removed) {
		t.Fatal("removing a fragment must not rewrite MCP configuration")
	}
}
