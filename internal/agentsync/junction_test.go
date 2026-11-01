package agentsync

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func saveLinkSeams(t *testing.T) {
	t.Helper()
	oldSymlink := symlinkCreateFunc
	oldJunction := junctionCreateFunc
	t.Cleanup(func() {
		symlinkCreateFunc = oldSymlink
		junctionCreateFunc = oldJunction
	})
}

func TestDirAliasFallbackOrderSymlinkThenJunction(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	saveLinkSeams(t)

	source := filepath.Join(dir, "canonical", "skills")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "runtime", "skills")

	symlinkCalled := false
	junctionCalled := false
	symlinkCreateFunc = func(_, _ string) error {
		symlinkCalled = true
		return errors.New("operation not permitted: symbolic link not permitted")
	}
	// Fake junction success. On Windows use a real junction (needs no symlink
	// privilege, so this also passes with Developer Mode off); elsewhere a
	// real symlink, which unprivileged CI users can always create.
	junctionCreateFunc = func(src, link string) error {
		junctionCalled = true
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			return createJunction(src, link)
		}
		return os.Symlink(src, link)
	}

	kind, err := createAlias(source, target, "link")
	if err != nil {
		t.Fatalf("createAlias with symlink denied should fall back to junction: %v", err)
	}
	if kind != "junction" {
		t.Fatalf("createAlias kind = %q, want %q", kind, "junction")
	}
	if !symlinkCalled || !junctionCalled {
		t.Fatalf("fallback order not observed: symlink=%v junction=%v", symlinkCalled, junctionCalled)
	}
	if !linkPointsTo(target, source) {
		t.Fatal("fallback link does not point to source")
	}
}

func TestDirAliasNeverFallsBackToStaticCopy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	saveLinkSeams(t)

	source := filepath.Join(dir, "canonical", "skills")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "runtime", "skills")

	symlinkCreateFunc = func(_, _ string) error {
		return errors.New("operation not permitted: symbolic link not permitted")
	}
	junctionCreateFunc = func(_, _ string) error {
		return errors.New("junction failed: reparse not supported on this volume")
	}

	_, err := createAlias(source, target, "link")
	if err == nil {
		t.Fatal("expected failure when both symlink and junction fail")
	}
	if pathExists(target) {
		t.Fatal("directory alias failure must not leave a static copy behind")
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(err.Error(), "Developer Mode") || !strings.Contains(err.Error(), "start ms-settings:developers") {
			t.Fatalf("Windows directory failure must name Developer Mode with start command, got: %v", err)
		}
	} else {
		if !strings.Contains(err.Error(), "cannot create directory alias") {
			t.Fatalf("non-Windows directory failure must be explicit, got: %v", err)
		}
	}
}

func TestDeveloperModeErrorContract(t *testing.T) {
	err := developerModeError("/target", errors.New("symlink denied"), errors.New("junction denied"))
	msg := err.Error()
	if !strings.Contains(msg, "Developer Mode") {
		t.Fatalf("Developer Mode error must name Developer Mode, got: %q", msg)
	}
	if !strings.Contains(msg, "start ms-settings:developers") {
		t.Fatalf("Developer Mode error must include start command, got: %q", msg)
	}
	if !strings.Contains(msg, "NTFS") {
		t.Fatalf("Developer Mode error should mention NTFS limits, got: %q", msg)
	}
}

func TestDirAliasCheckModeIsReadOnlyWhenLinksDenied(t *testing.T) {
	home, _ := newStorageTestHome(t)
	saveLinkSeams(t)
	symlinkCreateFunc = func(_, _ string) error {
		return errors.New("operation not permitted")
	}
	junctionCreateFunc = func(_, _ string) error {
		return errors.New("junction denied")
	}

	source := filepath.Join(home, "skills-src")
	target := filepath.Join(home, "skills-target")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := storageTestSnapshot(t, home)
	result, backup, err := syncSkillRootAlias(source, target, Options{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "missing" {
		t.Fatalf("check status = %q, want missing", result.Status)
	}
	if backup != "" {
		t.Fatalf("check must not back up: %q", backup)
	}
	assertStorageSnapshot(t, home, before)
}

func TestFileAliasStillFallsBackToCopy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	saveLinkSeams(t)

	source := filepath.Join(dir, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "runtime", "AGENTS.md")
	symlinkCreateFunc = func(_, _ string) error {
		return errors.New("operation not permitted")
	}

	kind, err := createAlias(source, target, "link")
	if err != nil {
		t.Fatal(err)
	}
	// File aliases keep the symlink, hardlink, managed-copy order: with only
	// the symlink denied, Windows falls back to a hardlink while other
	// systems fall back to a managed copy. Either way the result is a real
	// file, never a junction.
	if kind != "hardlink" && kind != "copy" {
		t.Fatalf("file alias should fall back to hardlink or copy, got %q", kind)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "canonical") {
		t.Fatalf("managed copy lost content: %q", data)
	}
}

func TestIsManagedLinkRecognizesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !isManagedLink(fi) {
		t.Fatal("isManagedLink must recognize symlinks")
	}
	fi, err = os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if isManagedLink(fi) {
		t.Fatal("isManagedLink must not recognize regular files")
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if isManagedLink(dirInfo) {
		t.Fatal("isManagedLink must not recognize real directories")
	}
}

func TestRemoveLinkNeverTouchesTargetContents(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(source, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	before := storageTestSnapshot(t, source)
	if err := removeLinkOrDir(link); err != nil {
		t.Fatal(err)
	}
	if pathExists(link) {
		t.Fatal("link was not removed")
	}
	assertStorageSnapshot(t, source, before)
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	before = storageTestSnapshot(t, source)
	if err := removeExistingForRestore(link); err != nil {
		t.Fatal(err)
	}
	assertStorageSnapshot(t, source, before)
}

func TestBackupAnyTreatsLinkAsLinkNotDir(t *testing.T) {
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
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	backup, err := backupAny(link)
	if err != nil {
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
	if len(manifest.Entries) != 1 || manifest.Entries[0].Kind != "symlink" {
		t.Fatalf("link backup kind = %+v, want symlink", manifest.Entries)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup payload missing: %v", err)
	}
	// Payload must be the link target string, not a copied directory.
	info, err := os.Lstat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("link backup payload must be a regular file, got %v", info.Mode())
	}
}
