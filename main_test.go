package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func cliTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home,
		"XDG_CONFIG_HOME":       filepath.Join(home, ".config"),
		"APPDATA":               filepath.Join(home, ".appdata"),
		"LOCALAPPDATA":          filepath.Join(home, ".localappdata"),
		"CLAUDE_CONFIG_DIR":     "",
		"KIMI_CODE_HOME":        "",
		"QODER_CONFIG_DIR":      "",
		"AGENTSYNC_CONFIG_HOME": filepath.Join(home, ".config", "agentsync"),
		"PI_CODING_AGENT_DIR":   filepath.Join(home, ".pi", "agent"),
		"DSH_HOME":              filepath.Join(home, ".dsh"),
	} {
		t.Setenv(key, value)
	}
	return home
}

func cliTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			out[path] = "directory"
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			out[path] = "symlink:" + target
			return err
		}
		data, err := os.ReadFile(path)
		out[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCLIRejectsUnexpectedArgumentsWithoutWrites(t *testing.T) {
	for _, args := range [][]string{
		{"status", "--check"}, {"--check", "status"}, {"version"},
		{"--", "--check"}, {"--watch", "status"}, {"--unknown"},
		{"--all"}, {"--all", "somewhere", "--repo"},
		{"--all", "somewhere", "--adopt", "draft.md"},
		{"--rollback", "latest", "--force"}, {"--watch", "--check"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := cliTestHome(t)
			before := cliTree(t, home)
			var stderr bytes.Buffer
			if code := runCLI(args, &stderr); code != 2 {
				t.Fatalf("exit = %d, want usage exit 2; stderr=%s", code, &stderr)
			}
			if stderr.Len() == 0 {
				t.Fatal("usage failure had no diagnostic")
			}
			if after := cliTree(t, home); !reflect.DeepEqual(before, after) {
				t.Fatalf("invalid invocation wrote files: before=%v after=%v", before, after)
			}
		})
	}
}

func TestCLIHelpDoesNotWrite(t *testing.T) {
	home := cliTestHome(t)
	before := cliTree(t, home)
	var stderr bytes.Buffer
	if code := runCLI([]string{"--help"}, &stderr); code != 0 {
		t.Fatalf("help exit = %d: %s", code, &stderr)
	}
	if !strings.Contains(stderr.String(), "only inspect") {
		t.Fatalf("help missing flag description: %s", &stderr)
	}
	if !reflect.DeepEqual(before, cliTree(t, home)) {
		t.Fatal("help wrote files")
	}
}

func TestCLIPreviewApplyAndRecheck(t *testing.T) {
	home := cliTestHome(t)
	before := cliTree(t, home)
	var stderr bytes.Buffer
	if code := runCLI([]string{"--check"}, &stderr); code != 0 {
		t.Fatalf("preview exit = %d: %s", code, &stderr)
	}
	if !reflect.DeepEqual(before, cliTree(t, home)) {
		t.Fatal("preview wrote files")
	}
	if code := runCLI(nil, &stderr); code != 0 {
		t.Fatalf("apply exit = %d: %s", code, &stderr)
	}
	canonical := filepath.Join(home, ".config", "agentsync", "AGENTS.md")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("apply did not create source: %v", err)
	}
	before = cliTree(t, home)
	if code := runCLI([]string{"--check"}, &stderr); code != 0 {
		t.Fatalf("recheck exit = %d: %s", code, &stderr)
	}
	if !reflect.DeepEqual(before, cliTree(t, home)) {
		t.Fatal("recheck wrote files")
	}
}

func TestCLIRuntimeFailureReturnsNonzero(t *testing.T) {
	home := cliTestHome(t)
	var stderr bytes.Buffer
	if code := runCLI([]string{"--all", filepath.Join(home, "missing"), "--check"}, &stderr); code != 1 {
		t.Fatalf("missing root exit = %d, want 1: %s", code, &stderr)
	}
	if !strings.Contains(stderr.String(), "scan root") {
		t.Fatalf("missing root diagnostic: %s", &stderr)
	}
}
