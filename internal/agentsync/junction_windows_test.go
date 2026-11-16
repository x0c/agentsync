//go:build windows

package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsJunctionIsManagedLink(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := createJunction(source, link); err != nil {
		t.Fatalf("createJunction: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !isManagedLink(fi) {
		t.Fatalf("junction Lstat mode %v not recognized as managed link", fi.Mode())
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("junction should report ModeIrregular, got %v", fi.Mode())
	}
	if fi.Mode()&os.ModeIrregular == 0 {
		t.Fatalf("junction should report ModeIrregular, got %v", fi.Mode())
	}
	if !linkPointsTo(link, source) {
		t.Fatal("linkPointsTo did not resolve junction target")
	}
	stat, err := os.Stat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !stat.IsDir() {
		t.Fatal("junction Stat should be a directory")
	}
	// Edits through the junction reach the canonical source.
	if err := os.WriteFile(filepath.Join(link, "demo", "SKILL.md"), []byte("# Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(source, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Edited\n" {
		t.Fatalf("junction write-through failed: %q", data)
	}
}

func TestWindowsSkillRootFallsBackToJunctionWhenSymlinkDenied(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	oldSymlink := symlinkCreateFunc
	t.Cleanup(func() { symlinkCreateFunc = oldSymlink })
	symlinkCreateFunc = func(_, _ string) error {
		return os.ErrPermission
	}

	source := filepath.Join(dir, "canonical", "skills")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "runtime", "skills")

	kind, err := createAlias(source, target, "link", instructionRender{})
	if err != nil {
		t.Fatalf("junction fallback failed: %v", err)
	}
	if kind != "junction" {
		t.Fatalf("kind = %q, want junction", kind)
	}
	fi, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !isManagedLink(fi) || fi.Mode()&os.ModeIrregular == 0 {
		t.Fatalf("fallback did not create a junction: %v", fi.Mode())
	}
	// The junction exposes canonical skills and stays in sync.
	if _, err := os.Stat(filepath.Join(target, "demo", "SKILL.md")); err != nil {
		t.Fatalf("junction does not expose skill: %v", err)
	}
	result, _, err := syncSkillRootAlias(source, target, Options{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" {
		t.Fatalf("junction alias should report ok, got %+v", result)
	}
	if !strings.Contains(result.Detail, "junction") {
		t.Fatalf("ok detail should mention junction, got %+v", result)
	}
}

func TestWindowsJunctionRemovalNeverTouchesTarget(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := createJunction(source, link); err != nil {
		t.Fatal(err)
	}
	before := storageTestSnapshot(t, source)
	if err := removeLinkOrDir(link); err != nil {
		t.Fatal(err)
	}
	if pathExists(link) {
		t.Fatal("junction was not removed")
	}
	assertStorageSnapshot(t, source, before)

	if err := createJunction(source, link); err != nil {
		t.Fatal(err)
	}
	before = storageTestSnapshot(t, source)
	if err := removeExistingForRestore(link); err != nil {
		t.Fatal(err)
	}
	assertStorageSnapshot(t, source, before)
}

func TestWindowsJunctionBackupAndRollbackFaithful(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := createJunction(source, link); err != nil {
		t.Fatal(err)
	}

	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	if _, err := backupAny(link); err != nil {
		t.Fatal(err)
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}
	stamp, err := latestBackupStamp()
	if err != nil {
		t.Fatal(err)
	}
	stampDir, err := stampDirFor(stamp)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := loadBackupManifest(stampDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].Kind != "junction" {
		t.Fatalf("junction backup kind = %+v, want junction", manifest.Entries)
	}

	// Replace the junction with a real directory, then roll back.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(link, "replacement.txt"), []byte("replacement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(Options{Rollback: stamp}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !isManagedLink(fi) {
		t.Fatalf("rollback did not restore junction: %v", fi.Mode())
	}
	if !linkPointsTo(link, source) {
		t.Fatal("restored junction points to the wrong target")
	}
	data, err := os.ReadFile(filepath.Join(source, "demo", "SKILL.md"))
	if err != nil || string(data) != "# Demo\n" {
		t.Fatalf("junction target contents changed: %q %v", data, err)
	}
}

func TestWindowsJunctionOverlapGuardSeesThrough(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	canonical := filepath.Join(tree, "canonical")
	if err := os.MkdirAll(filepath.Join(canonical, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := createJunction(tree, alias); err != nil {
		t.Fatal(err)
	}
	// The source reached through a junction is inside the target tree, mirroring
	// the symlink "aliased-parent" overlap layout: it must be rejected.
	aliasedSource := filepath.Join(alias, "canonical")
	before := storageTestSnapshot(t, dir)
	if _, _, err := syncSkillRootAlias(aliasedSource, tree, Options{Check: true}); err == nil || !strings.Contains(err.Error(), "overlapping skill roots") {
		t.Fatalf("expected overlap rejection through junction, got %v", err)
	}
	assertStorageSnapshot(t, dir, before)
	// A correct junction alias itself must be accepted as non-overlapping.
	correctTarget := filepath.Join(dir, "correct")
	if err := os.MkdirAll(correctTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	junctionTarget := filepath.Join(correctTarget, "skills")
	if err := createJunction(canonical, junctionTarget); err != nil {
		t.Fatal(err)
	}
	if err := validateSkillRootOverlap(canonical, junctionTarget); err != nil {
		t.Fatalf("correct junction alias rejected: %v", err)
	}
}

func TestWindowsJunctionReplacementKeepsTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	source := filepath.Join(dir, "canonical", "skills")
	other := filepath.Join(dir, "other", "skills")
	for _, root := range []string{source, other} {
		if err := os.MkdirAll(filepath.Join(root, "demo"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "demo", "SKILL.md"), []byte("# "+filepath.Base(filepath.Dir(root))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(dir, "runtime", "skills")
	if err := createJunction(other, target); err != nil {
		t.Fatal(err)
	}
	beforeOther := storageTestSnapshot(t, other)
	beforeSource := storageTestSnapshot(t, source)
	result, _, err := syncSkillRootAlias(source, target, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "replaced" {
		t.Fatalf("wrong junction should be replaced, got %+v", result)
	}
	assertStorageSnapshot(t, other, beforeOther)
	assertStorageSnapshot(t, source, beforeSource)
	if !linkPointsTo(target, source) {
		t.Fatal("replacement did not point at the new source")
	}
}
