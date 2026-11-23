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
			var stderr, stdout bytes.Buffer
			if code := runCLI(args, &stderr, &stdout); code != 2 {
				t.Fatalf("exit = %d, want usage exit 2; stderr=%s", code, &stderr)
			}
			if stderr.Len() == 0 {
				t.Fatal("usage failure had no diagnostic")
			}
			if stdout.Len() != 0 {
				t.Fatalf("usage failure wrote to stdout: %q", stdout.String())
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
	var stderr, stdout bytes.Buffer
	if code := runCLI([]string{"--help"}, &stderr, &stdout); code != 0 {
		t.Fatalf("help exit = %d: %s", code, &stderr)
	}
	if !strings.Contains(stderr.String(), "only inspect") {
		t.Fatalf("help missing flag description: %s", &stderr)
	}
	if !strings.Contains(stderr.String(), "print the agentsync version") {
		t.Fatalf("help missing version flag description: %s", &stderr)
	}
	// The per-tool fragment convention is the one capability an installed binary
	// must explain by itself: its absence sent an agent into the source tree.
	for _, want := range []string{
		"instructions/<runtime>.md",
		"instructions/pi.md",
		`agentsync:begin overlay target="pi"`,
		"appends a managed MCP",
		"#per-tool-instruction-fragments",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("help missing %q: %s", want, &stderr)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("help wrote to stdout: %q", stdout.String())
	}
	if !reflect.DeepEqual(before, cliTree(t, home)) {
		t.Fatal("help wrote files")
	}
}

func TestCLIVersionPrintsNameAndVersion(t *testing.T) {
	home := cliTestHome(t)
	before := cliTree(t, home)
	var stderr, stdout bytes.Buffer
	if code := runCLI([]string{"--version"}, &stderr, &stdout); code != 0 {
		t.Fatalf("version exit = %d: %s", code, &stderr)
	}
	want := "agentsync " + resolvedVersion() + "\n"
	if stdout.String() != want {
		t.Fatalf("version output = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("version wrote to stderr: %q", stderr.String())
	}
	if !reflect.DeepEqual(before, cliTree(t, home)) {
		t.Fatal("version wrote files")
	}
}

func TestCLIVersionWinsOverOtherArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--version", "--check"},
		{"--check", "--version"},
		{"--version", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := cliTestHome(t)
			before := cliTree(t, home)
			var stderr, stdout bytes.Buffer
			if code := runCLI(args, &stderr, &stdout); code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%s", code, &stderr)
			}
			want := "agentsync " + resolvedVersion() + "\n"
			if stdout.String() != want {
				t.Fatalf("output = %q, want %q", stdout.String(), want)
			}
			if !reflect.DeepEqual(before, cliTree(t, home)) {
				t.Fatal("version invocation wrote files")
			}
		})
	}
}

func TestDisplayVersionFallback(t *testing.T) {
	for _, tc := range []struct {
		ldflags, build, want string
	}{
		{"1.2.3", "v9.9.9", "1.2.3"},
		{"", "v1.2.3", "v1.2.3"},
		{"", "", "dev"},
		{"", "(devel)", "dev"},
	} {
		if got := displayVersion(tc.ldflags, tc.build); got != tc.want {
			t.Errorf("displayVersion(%q, %q) = %q, want %q", tc.ldflags, tc.build, got, tc.want)
		}
	}
}

func TestCLIPreviewApplyAndRecheck(t *testing.T) {
	home := cliTestHome(t)
	before := cliTree(t, home)
	var stderr, stdout bytes.Buffer
	if code := runCLI([]string{"--check"}, &stderr, &stdout); code != 0 {
		t.Fatalf("preview exit = %d: %s", code, &stderr)
	}
	if !reflect.DeepEqual(before, cliTree(t, home)) {
		t.Fatal("preview wrote files")
	}
	if code := runCLI(nil, &stderr, &stdout); code != 0 {
		t.Fatalf("apply exit = %d: %s", code, &stderr)
	}
	canonical := filepath.Join(home, ".config", "agentsync", "AGENTS.md")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("apply did not create source: %v", err)
	}
	before = cliTree(t, home)
	if code := runCLI([]string{"--check"}, &stderr, &stdout); code != 0 {
		t.Fatalf("recheck exit = %d: %s", code, &stderr)
	}
	if !reflect.DeepEqual(before, cliTree(t, home)) {
		t.Fatal("recheck wrote files")
	}
}

func TestCLIRuntimeFailureReturnsNonzero(t *testing.T) {
	home := cliTestHome(t)
	var stderr, stdout bytes.Buffer
	if code := runCLI([]string{"--all", filepath.Join(home, "missing"), "--check"}, &stderr, &stdout); code != 1 {
		t.Fatalf("missing root exit = %d, want 1: %s", code, &stderr)
	}
	if !strings.Contains(stderr.String(), "scan root") {
		t.Fatalf("missing root diagnostic: %s", &stderr)
	}
}
