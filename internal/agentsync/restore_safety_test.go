package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageNoopRunKeepsLatestRecoverableBackup(t *testing.T) {
	home, _ := newStorageTestHome(t)
	target := filepath.Join(home, ".claude", "CLAUDE.md")
	writeStorageTestFile(t, target, "original instruction\n")
	if err := Run(Options{Force: true, SkipMCP: true}); err != nil {
		t.Fatal(err)
	}
	first, err := latestBackupStamp()
	if err != nil {
		t.Fatal(err)
	}
	stamps, err := listBackupStamps()
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(Options{SkipMCP: true}); err != nil {
		t.Fatal(err)
	}
	after, err := listBackupStamps()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(stamps) {
		t.Fatalf("no-op retained an empty session: before=%v, after=%v", stamps, after)
	}
	latest, err := latestBackupStamp()
	if err != nil || latest != first {
		t.Fatalf("latest after no-op = %q, %v; want %q", latest, err, first)
	}
	before := storageTestSnapshot(t, home)
	if err := Run(Options{Check: true, Rollback: "latest"}); err != nil {
		t.Fatal(err)
	}
	assertStorageSnapshot(t, home, before)
	if err := Run(Options{Rollback: "latest"}); err != nil {
		t.Fatal(err)
	}
	assertStorageFile(t, target, "original instruction\n")
}

func TestStorageLatestSkipsInvalidAndManualStamps(t *testing.T) {
	home, config := newStorageTestHome(t)
	target := filepath.Join(home, "current.txt")
	writeStorageTestFile(t, target, "current\n")
	root := filepath.Join(config, "backups")
	validStamp := "20000101-000001"
	validDir := filepath.Join(root, validStamp)
	legacyName := sanitizePath(target)
	writeStorageTestFile(t, filepath.Join(validDir, legacyName), "legacy saved\n")
	manifest := backupManifest{Version: 1, Stamp: validStamp, Entries: []backupManifestEntry{{Original: target, Name: legacyName, Kind: "file"}}}
	if err := writeBackupManifestFile(validDir, manifest); err != nil {
		t.Fatal(err)
	}
	for stamp, body := range map[string]string{
		"99990101-000001": `{invalid json`,
		"99990101-000002": `{"version":1,"entries":[]}`,
		"99990101-000003": `{"version":1,"entries":[{"original":"relative.txt","name":"payload","kind":"file"}]}`,
	} {
		writeStorageTestFile(t, filepath.Join(root, stamp, backupManifestName), body)
	}
	manualStamp := "99990101-000004"
	manualPayload := filepath.Join(root, manualStamp, "old-manual-file")
	writeStorageTestFile(t, manualPayload, "manual backup must remain\n")
	missingDir := filepath.Join(root, "99990101-000005")
	if err := writeBackupManifestFile(missingDir, backupManifest{Version: 1, Entries: []backupManifestEntry{{Original: target, Name: "missing", Kind: "file"}}}); err != nil {
		t.Fatal(err)
	}
	if got, err := latestBackupStamp(); err != nil || got != validStamp {
		t.Fatalf("latest = %q, %v; want legacy nonempty stamp %s", got, err, validStamp)
	}
	before := storageTestSnapshot(t, home)
	if err := Run(Options{Check: true, Rollback: "latest"}); err != nil {
		t.Fatal(err)
	}
	assertStorageSnapshot(t, home, before)
	if err := Run(Options{Rollback: "latest"}); err != nil {
		t.Fatal(err)
	}
	assertStorageFile(t, target, "legacy saved\n")
	assertStorageFile(t, manualPayload, "manual backup must remain\n")
	if err := Run(Options{Rollback: manualStamp}); err == nil || !strings.Contains(err.Error(), "restore manually") {
		t.Fatalf("manual-only stamp lost its explicit compatibility error: %v", err)
	}
	assertStorageFile(t, manualPayload, "manual backup must remain\n")
	// Newly created recovery payloads coexist with legacy names and are selectable.
	latest, err := latestBackupStamp()
	if err != nil || latest == validStamp {
		t.Fatalf("new recovery session not selected: %q, %v", latest, err)
	}
	if err := Run(Options{Rollback: latest}); err != nil {
		t.Fatal(err)
	}
	assertStorageFile(t, target, "current\n")
}

func TestStorageRollbackPreflightsAllPayloadsWithoutWrites(t *testing.T) {
	for _, invalid := range []string{"empty-link", "wrong-type", "missing"} {
		t.Run(invalid, func(t *testing.T) {
			home, config := newStorageTestHome(t)
			first := filepath.Join(home, "first.txt")
			second := filepath.Join(home, "second.txt")
			writeStorageTestFile(t, first, "first current\n")
			writeStorageTestFile(t, second, "second current\n")
			stamp := "20000101-000001"
			dir := filepath.Join(config, "backups", stamp)
			writeStorageTestFile(t, filepath.Join(dir, "first"), "first saved\n")
			kind := "file"
			switch invalid {
			case "empty-link":
				kind = "symlink"
				writeStorageTestFile(t, filepath.Join(dir, "invalid"), "\n")
			case "wrong-type":
				writeStorageTestFile(t, filepath.Join(dir, "invalid", "child"), "not a file\n")
			}
			manifest := backupManifest{Version: 1, Entries: []backupManifestEntry{
				{Original: first, Name: "first", Kind: "file"},
				{Original: second, Name: "invalid", Kind: kind},
			}}
			if err := writeBackupManifestFile(dir, manifest); err != nil {
				t.Fatal(err)
			}
			before := storageTestSnapshot(t, home)
			for _, check := range []bool{true, false} {
				if err := Run(Options{Check: check, Rollback: stamp}); err == nil {
					t.Fatalf("check=%v accepted invalid payload", check)
				}
				assertStorageSnapshot(t, home, before)
			}
		})
	}
}

func TestStorageEmptySessionPreservesUnrecordedManualPayload(t *testing.T) {
	_, _ = newStorageTestHome(t)
	stamp, err := beginBackupSession()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := stampDirFor(stamp)
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(dir, "manual-file")
	writeStorageTestFile(t, payload, "not owned by the session\n")
	if err := endBackupSession(); err == nil {
		t.Fatal("expected refusal to remove a nonempty unrecorded session")
	}
	assertStorageFile(t, payload, "not owned by the session\n")
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
