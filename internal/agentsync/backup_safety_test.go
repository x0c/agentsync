package agentsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageRunBackupCollisionAndNamedRollback(t *testing.T) {
	home, _ := newStorageTestHome(t)
	paths := []string{
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(home, ".config__opencode", "AGENTS.md"),
	}
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Dir(paths[1]))
	for i, path := range paths {
		writeStorageTestFile(t, path, fmt.Sprintf("original-%d\n", i))
	}
	before := storageTestSnapshot(t, home)
	if err := Run(Options{Check: true, Force: true, SkipMCP: true}); err != nil {
		t.Fatal(err)
	}
	assertStorageSnapshot(t, home, before)
	if err := Run(Options{Force: true, SkipMCP: true}); err != nil {
		t.Fatal(err)
	}
	stamp, err := latestBackupStamp()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := stampDirFor(stamp)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := loadBackupManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertDistinctStorageBackups(t, dir, manifest, map[string]string{paths[0]: "original-0\n", paths[1]: "original-1\n"})
	if err := Run(Options{Rollback: stamp}); err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		assertStorageFile(t, path, fmt.Sprintf("original-%d\n", i))
	}
	// Undo the rollback too: both prior runtime aliases must be recoverable.
	if err := Run(Options{Rollback: "latest"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := os.Readlink(path); err != nil {
			t.Fatalf("latest rollback did not restore runtime alias %s: %v", path, err)
		}
	}
}

func assertDistinctStorageBackups(t *testing.T, dir string, manifest backupManifest, contents map[string]string) {
	t.Helper()
	seen := map[string]string{}
	for _, entry := range manifest.Entries {
		if original, ok := seen[entry.Name]; ok && original != entry.Original {
			t.Fatalf("payload %s shared by %s and %s", entry.Name, original, entry.Original)
		}
		seen[entry.Name] = entry.Original
		if want, ok := contents[entry.Original]; ok {
			assertStorageFile(t, filepath.Join(dir, entry.Name), want)
			delete(contents, entry.Original)
		}
	}
	if len(contents) != 0 {
		t.Fatalf("missing backup originals: %v", contents)
	}
}

func TestStorageBackupPayloadTypeSuffixesDoNotCollide(t *testing.T) {
	home, _ := newStorageTestHome(t)
	root := filepath.Join(home, "originals")
	link := filepath.Join(root, "entry")
	storageTestSymlink(t, "missing-original-target", link)
	files := map[string]string{
		filepath.Join(root, "entry.symlink"): "ordinary symlink-suffix file\n",
		filepath.Join(root, "entry.dir"):     "ordinary dir-suffix file\n",
		filepath.Join(root, "a", "b"):        "nested path\n",
		filepath.Join(root, "a__b"):          "literal underscores\n",
	}
	for path, content := range files {
		writeStorageTestFile(t, path, content)
	}
	directory := filepath.Join(root, "entry.file")
	writeStorageTestFile(t, filepath.Join(directory, "data"), "directory payload\n")
	stamp, err := beginBackupSession()
	if err != nil {
		t.Fatal(err)
	}
	for path := range files {
		if _, err := backupAny(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{link, directory} {
		if _, err := backupAny(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := endBackupSession(); err != nil {
		t.Fatal(err)
	}
	dir, err := stampDirFor(stamp)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := loadBackupManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != len(files)+2 {
		t.Fatalf("wrong entry count: %+v", manifest.Entries)
	}
	assertDistinctStorageBackups(t, dir, manifest, files)
	for _, entry := range manifest.Entries {
		switch entry.Kind {
		case "symlink":
			assertStorageFile(t, filepath.Join(dir, entry.Name), "missing-original-target\n")
		case "dir":
			assertStorageFile(t, filepath.Join(dir, entry.Name, "data"), "directory payload\n")
		}
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := Run(Options{Rollback: stamp}); err != nil {
		t.Fatal(err)
	}
	assertStorageFile(t, filepath.Join(directory, "data"), "directory payload\n")
	if target, err := os.Readlink(link); err != nil || target != "missing-original-target" {
		t.Fatalf("symlink restore = %q, %v", target, err)
	}
	assertStorageFile(t, filepath.Join(root, "a", "b"), "nested path\n")
	assertStorageFile(t, filepath.Join(root, "a__b"), "literal underscores\n")
	assertStorageFile(t, filepath.Join(root, "entry.symlink"), "ordinary symlink-suffix file\n")
}

func TestStorageBackupRetainsFirstSnapshotAcrossTypeChanges(t *testing.T) {
	for _, initial := range []string{"file", "symlink", "dir"} {
		t.Run(initial, func(t *testing.T) {
			home, _ := newStorageTestHome(t)
			path := filepath.Join(home, "original")
			makeEntry := func(kind, value string) {
				t.Helper()
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "file":
					writeStorageTestFile(t, path, value)
				case "symlink":
					storageTestSymlink(t, value, path)
				case "dir":
					writeStorageTestFile(t, filepath.Join(path, "data"), value)
				}
			}
			makeEntry(initial, "first")
			stamp, err := beginBackupSession()
			if err != nil {
				t.Fatal(err)
			}
			first, err := backupAny(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{initial, "file", "symlink", "dir"} {
				makeEntry(kind, "later")
				got, err := backupAny(path)
				if err != nil || got != first {
					t.Fatalf("repeated backup = %s, %v; want first payload %s", got, err, first)
				}
			}
			if err := endBackupSession(); err != nil {
				t.Fatal(err)
			}
			dir, err := stampDirFor(stamp)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := loadBackupManifest(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(manifest.Entries) != 1 || manifest.Entries[0].Kind != initial {
				t.Fatalf("initial snapshot replaced in manifest: %+v", manifest)
			}
			if err := Run(Options{Rollback: stamp}); err != nil {
				t.Fatal(err)
			}
			switch initial {
			case "file":
				assertStorageFile(t, path, "first")
			case "symlink":
				if target, err := os.Readlink(path); err != nil || target != "first" {
					t.Fatalf("original symlink target = %q, %v", target, err)
				}
			case "dir":
				assertStorageFile(t, filepath.Join(path, "data"), "first")
			}
		})
	}
}

func TestStorageBackupRefusesOccupiedPayload(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "dir"} {
		t.Run(kind, func(t *testing.T) {
			home, _ := newStorageTestHome(t)
			original := filepath.Join(home, "original")
			switch kind {
			case "file":
				writeStorageTestFile(t, original, "original data")
			case "symlink":
				storageTestSymlink(t, "original-target", original)
			case "dir":
				writeStorageTestFile(t, filepath.Join(original, "data"), "original data")
			}
			stamp, err := beginBackupSession()
			if err != nil {
				t.Fatal(err)
			}
			dir, err := stampDirFor(stamp)
			if err != nil {
				t.Fatal(err)
			}
			name, _, err := backupPayloadName(dir, original, kind)
			if err != nil {
				t.Fatal(err)
			}
			occupied := filepath.Join(dir, name)
			writeStorageTestFile(t, occupied, "unrecorded user payload")
			if _, err := backupAny(original); !errors.Is(err, os.ErrExist) {
				t.Fatalf("expected exclusive-create failure, got %v", err)
			}
			assertStorageFile(t, occupied, "unrecorded user payload")
			// Register another original owning the name; identity checks must also reject it.
			if err := recordBackupEntryAt(dir, filepath.Join(home, "other"), name, "file"); err != nil {
				t.Fatal(err)
			}
			if _, err := backupAny(original); err == nil {
				t.Fatal("accepted payload already owned by another original")
			}
			assertStorageFile(t, occupied, "unrecorded user payload")
		})
	}
}
