package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollbackRestoresLatestStamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))

	target := filepath.Join(dir, "home", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	backup, err := backupFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("expected backup path")
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(target, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(Options{Rollback: "latest"}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "original\n" {
		t.Fatalf("restored content = %q, want original", got)
	}
}

func TestRollbackCheckDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))

	target := filepath.Join(dir, "home", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	if _, err := backupFile(target); err != nil {
		t.Fatal(err)
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(Options{Check: true, Rollback: "latest"}); err != nil {
		t.Fatalf("rollback check: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "changed\n" {
		t.Fatalf("check mode changed file: %q", got)
	}
}

func TestRollbackRestoresSymlinkAndDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))

	source := filepath.Join(dir, "source", "AGENTS.md")
	link := filepath.Join(dir, "runtime", "AGENTS.md")
	skills := filepath.Join(dir, "runtime", "skills")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skills, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skills, "demo", "SKILL.md"), []byte("skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	if _, err := backupFile(link); err != nil {
		t.Fatal(err)
	}
	if _, err := backupAny(skills); err != nil {
		t.Fatal(err)
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(skills); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Run(Options{Rollback: "latest"}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if !symlinkPointsTo(link, source) {
		t.Fatalf("symlink was not restored")
	}
	data, err := os.ReadFile(filepath.Join(skills, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "skill\n" {
		t.Fatalf("skill dir restore = %q", got)
	}
}

func TestSyncSessionWritesSingleStampManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))

	cfg := Config{
		Source: filepath.Join(dir, "source", "AGENTS.md"),
		Targets: []Target{
			{Path: filepath.Join(dir, "a", "AGENTS.md"), Mode: "link"},
			{Path: filepath.Join(dir, "b", "AGENTS.md"), Mode: "link"},
		},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Source, []byte("canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range cfg.Targets {
		if err := os.MkdirAll(filepath.Dir(target.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target.Path, []byte("old-"+filepath.Base(filepath.Dir(target.Path))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := beginBackupSession(); err != nil {
		t.Fatal(err)
	}
	report, err := syncConfig(cfg, Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}
	if len(report.Backups) == 0 {
		t.Fatalf("expected backups, got report=%+v", report)
	}

	stamps, err := listBackupStamps()
	if err != nil {
		t.Fatal(err)
	}
	if len(stamps) != 1 {
		t.Fatalf("want 1 stamp for one sync session, got %v", stamps)
	}
	stampDir, err := stampDirFor(stamps[0])
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := loadBackupManifest(stampDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) < 2 {
		t.Fatalf("manifest entries = %d, want at least 2: %+v", len(manifest.Entries), manifest.Entries)
	}
}

func TestRollbackNamedStamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))

	target := filepath.Join(dir, "home", "file.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := backupDir()
	if err != nil {
		t.Fatal(err)
	}

	writeStamp := func(stamp, content string) {
		t.Helper()
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		stampDir := filepath.Join(root, stamp)
		name := sanitizePath(target)
		if err := os.MkdirAll(stampDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stampDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest := backupManifest{
			Version: 1,
			Stamp:   stamp,
			Entries: []backupManifestEntry{{
				Original: target,
				Name:     name,
				Kind:     "file",
			}},
		}
		if err := writeBackupManifestFile(stampDir, manifest); err != nil {
			t.Fatal(err)
		}
	}

	writeStamp("20260101-010101", "v1\n")
	writeStamp("20260102-020202", "v2\n")
	if err := os.WriteFile(target, []byte("v3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(Options{Rollback: "20260101-010101"}); err != nil {
		t.Fatalf("rollback named: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "v1\n" {
		t.Fatalf("named stamp restore = %q, want v1", got)
	}
}

func TestRollbackRejectsConflictingFlags(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	err := Run(Options{Rollback: "latest", Force: true})
	if err == nil || !strings.Contains(err.Error(), "--rollback cannot be combined") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestRollbackMissingManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	root, err := backupDir()
	if err != nil {
		t.Fatal(err)
	}
	stampDir := filepath.Join(root, "20260103-030303")
	if err := os.MkdirAll(stampDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stampDir, "orphan.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Run(Options{Rollback: "20260103-030303"})
	if err == nil || !strings.Contains(err.Error(), "no manifest.json") {
		t.Fatalf("expected missing manifest error, got %v", err)
	}
}
