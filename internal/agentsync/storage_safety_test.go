package agentsync

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func newStorageTestHome(t *testing.T) (home, config string) {
	t.Helper()
	home = t.TempDir()
	config = filepath.Join(home, ".config", "agentsync")
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home,
		"APPDATA":               filepath.Join(home, "appdata"),
		"LOCALAPPDATA":          filepath.Join(home, "localappdata"),
		"CLAUDE_CONFIG_DIR":     "",
		"KIMI_CODE_HOME":        "",
		"QODER_CONFIG_DIR":      "",
		"XDG_CONFIG_HOME":       filepath.Join(home, ".config"),
		"AGENTSYNC_CONFIG_HOME": config,
		"PI_CODING_AGENT_DIR":   filepath.Join(home, ".pi", "agent"),
		"DSH_HOME":              filepath.Join(home, ".dsh"),
	} {
		t.Setenv(key, value)
	}
	writeStorageTestFile(t, filepath.Join(config, "AGENTS.md"), "# Canonical\n")
	t.Cleanup(func() {
		if err := endBackupSession(); err != nil {
			t.Errorf("close backup session: %v", err)
		}
	})
	return home, config
}

func writeStorageTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := ensureParent(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func storageTestSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := ensureParent(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS == "windows" && os.IsPermission(err) {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

func storageTestSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%v", info.Mode())
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += ":" + target
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += ":" + string(data)
		}
		out[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertStorageSnapshot(t *testing.T, root string, before map[string]string) {
	t.Helper()
	if after := storageTestSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("tree changed at %s\nbefore: %v\nafter: %v", root, before, after)
	}
}

func assertStorageFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func TestStorageMaterializationPreservesSiblingAndChainedRoot(t *testing.T) {
	for _, chained := range []bool{false, true} {
		t.Run(fmt.Sprintf("chained=%v", chained), func(t *testing.T) {
			home, config := newStorageTestHome(t)
			real := filepath.Join(home, "external", "real-demo")
			writeStorageTestFile(t, filepath.Join(real, "SKILL.md"), "# Demo\n")
			writeStorageTestFile(t, filepath.Join(real, "assets", "data.txt"), "asset\n")
			target := real
			if chained {
				target = filepath.Join(home, "external", "alias")
				storageTestSymlink(t, "real-demo", target)
			}
			link := filepath.Join(config, "skills", "demo")
			storageTestSymlink(t, target, link)
			sibling := link + ".agentsync-tmp"
			writeStorageTestFile(t, filepath.Join(sibling, "SKILL.md"), "# Independent skill\n")
			writeStorageTestFile(t, filepath.Join(sibling, "unique.txt"), "unique user data\n")
			writeStorageTestFile(t, filepath.Join(home, ".claude", "installed"), "runtime\n")
			before := storageTestSnapshot(t, home)
			if err := Run(Options{Check: true, SkipMCP: true}); err != nil {
				t.Fatal(err)
			}
			assertStorageSnapshot(t, home, before)
			if err := Run(Options{SkipMCP: true}); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(link)
			if err != nil || !info.IsDir() {
				t.Fatalf("materialized skill is not a real directory: info=%v err=%v", info, err)
			}
			assertStorageFile(t, filepath.Join(link, "assets", "data.txt"), "asset\n")
			assertStorageFile(t, filepath.Join(home, ".claude", "skills", "demo", "SKILL.md"), "# Demo\n")
			assertStorageFile(t, filepath.Join(sibling, "unique.txt"), "unique user data\n")
			entries, err := os.ReadDir(filepath.Dir(link))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".agentsync-stage-") {
					t.Fatalf("owned staging directory leaked: %s", entry.Name())
				}
			}
			stamp, err := latestBackupStamp()
			if err != nil {
				t.Fatal(err)
			}
			before = storageTestSnapshot(t, home)
			if err := Run(Options{Check: true, Rollback: stamp}); err != nil {
				t.Fatal(err)
			}
			assertStorageSnapshot(t, home, before)
			if err := Run(Options{Rollback: stamp}); err != nil {
				t.Fatal(err)
			}
			if !symlinkPointsTo(link, target) {
				t.Fatal("named rollback did not restore original skill symlink")
			}
			if err := Run(Options{Rollback: "latest"}); err != nil {
				t.Fatal(err)
			}
			info, err = os.Lstat(link)
			if err != nil || !info.IsDir() {
				t.Fatalf("rollback undo did not restore the materialized directory: %v, %v", info, err)
			}
			assertStorageFile(t, filepath.Join(sibling, "unique.txt"), "unique user data\n")
		})
	}
}

func TestStorageRunRefusesCanonicalRuntimeRootCycle(t *testing.T) {
	home, config := newStorageTestHome(t)
	runtimeRoot := filepath.Join(home, ".claude", "skills")
	writeStorageTestFile(t, filepath.Join(runtimeRoot, "demo", "SKILL.md"), "# Runtime skill\n")
	canonical := filepath.Join(config, "skills")
	storageTestSymlink(t, runtimeRoot, canonical)
	before := storageTestSnapshot(t, runtimeRoot)
	for _, check := range []bool{true, false} {
		if err := Run(Options{Check: check, SkipMCP: true}); err == nil || !strings.Contains(err.Error(), "overlapping skill roots") {
			t.Fatalf("check=%v: expected overlap error, got %v", check, err)
		}
		assertStorageSnapshot(t, runtimeRoot, before)
		if !symlinkPointsTo(canonical, runtimeRoot) {
			t.Fatal("canonical root alias changed")
		}
	}
}

func TestStorageSkillRootOverlapGuards(t *testing.T) {
	for _, layout := range []string{"same", "source-child", "target-child", "dotdot-name-child", "aliased-parent"} {
		for _, filtered := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/filtered=%v", layout, filtered), func(t *testing.T) {
				home, _ := newStorageTestHome(t)
				tree := filepath.Join(home, "tree")
				source, target := tree, tree
				switch layout {
				case "source-child", "aliased-parent":
					source = filepath.Join(tree, "canonical")
				case "target-child":
					target = filepath.Join(tree, "runtime")
				case "dotdot-name-child":
					source = filepath.Join(tree, "..notes")
				}
				writeStorageTestFile(t, filepath.Join(source, "demo", "SKILL.md"), "# Source\n")
				writeStorageTestFile(t, filepath.Join(target, ".system", "owned.txt"), "runtime-owned\n")
				if layout == "aliased-parent" {
					alias := filepath.Join(home, "alias")
					storageTestSymlink(t, tree, alias)
					source = filepath.Join(alias, "canonical")
				}
				before := storageTestSnapshot(t, home)
				for _, check := range []bool{true, false} {
					var err error
					if filtered {
						_, _, err = syncSkillRootFiltered(source, target, Options{Check: check}, []string{"demo"}, nil)
					} else {
						_, _, err = syncSkillRootAlias(source, target, Options{Check: check})
					}
					if err == nil || !strings.Contains(err.Error(), "overlapping skill roots") {
						t.Fatalf("check=%v: expected overlap rejection, got %v", check, err)
					}
					assertStorageSnapshot(t, home, before)
				}
			})
		}
	}
}

func TestStorageSkillRootSiblingAndFilterTransitions(t *testing.T) {
	home, _ := newStorageTestHome(t)
	source := filepath.Join(home, "skills")
	target := filepath.Join(home, "skills-other")
	for _, name := range []string{"keep", "drop"} {
		writeStorageTestFile(t, filepath.Join(source, name, "SKILL.md"), "# "+name+"\n")
	}
	writeStorageTestFile(t, filepath.Join(target, "notes.txt"), "original target\n")
	before := storageTestSnapshot(t, source)
	if _, _, err := syncSkillRootAlias(source, target, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := syncSkillRootFiltered(source, target, Options{}, []string{"keep"}, []string{"drop"}); err != nil {
		t.Fatal(err)
	}
	if !symlinkPointsTo(filepath.Join(target, "keep"), filepath.Join(source, "keep")) || pathExists(filepath.Join(target, "drop")) {
		t.Fatal("filtered view did not contain exactly the allowed skill")
	}
	if _, _, err := syncSkillRootAlias(source, target, Options{}); err != nil {
		t.Fatal(err)
	}
	if !symlinkPointsTo(target, source) {
		t.Fatal("removing filtering did not restore the whole-root alias")
	}
	assertStorageSnapshot(t, source, before)
}
