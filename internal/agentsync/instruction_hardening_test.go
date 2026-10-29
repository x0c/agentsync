package agentsync

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func hardeningHome(t *testing.T) string {
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

func hardeningWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hardeningRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstructionImportsPrecedeDerivedCopies(t *testing.T) {
	for _, cursorFirst := range []bool{true, false} {
		name := "cursor-last"
		if cursorFirst {
			name = "cursor-first"
		}
		t.Run(name, func(t *testing.T) {
			home := hardeningHome(t)
			source := filepath.Join(home, "source", "AGENTS.md")
			cursor := filepath.Join(home, "cursor", "rules", "AGENTS.mdc")
			late := filepath.Join(home, "windsurf", "global_rules.md")
			hardeningWrite(t, source, "BASE_RULE\n")
			hardeningWrite(t, late, "LATE_UNIQUE_RULE\n")
			cfg := Config{Source: source, Targets: []Target{{Path: cursor, Mode: "cursor"}, {Path: late, Mode: "link"}}}
			if !cursorFirst {
				cfg.Targets[0], cfg.Targets[1] = cfg.Targets[1], cfg.Targets[0]
			}
			if _, err := syncConfig(cfg, Options{}); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{source, cursor, late} {
				if !containsAll(hardeningRead(t, path), "BASE_RULE", "LATE_UNIQUE_RULE") {
					t.Fatalf("first pass left stale instructions at %s", path)
				}
			}
			report, err := syncConfig(cfg, Options{Check: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, result := range report.Results {
				if result.Status != "ok" {
					t.Fatalf("first apply did not converge: %+v", result)
				}
			}
		})
	}
}

func TestUnmanagedCursorContentIsPreserved(t *testing.T) {
	for _, linked := range []bool{false, true} {
		name := "file"
		if linked {
			name = "symlink"
		}
		t.Run(name, func(t *testing.T) {
			home := hardeningHome(t)
			source := filepath.Join(home, "source", "AGENTS.md")
			target := filepath.Join(home, "cursor", "AGENTS.mdc")
			hardeningWrite(t, source, "BASE_RULE\n")
			original := "---\nalwaysApply: true\n---\nCURSOR_UNIQUE_RULE\n"
			hardeningWrite(t, target, original)
			if linked {
				external := filepath.Join(home, "external", "cursor.mdc")
				hardeningWrite(t, external, original)
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, target); err != nil {
					t.Fatal(err)
				}
			}
			cfg := Config{Source: source, Targets: []Target{{Path: target, Mode: "cursor"}}}
			if _, err := syncConfig(cfg, Options{Check: true}); err != nil {
				t.Fatal(err)
			}
			if hardeningRead(t, source) != "BASE_RULE\n" || hardeningRead(t, target) != original {
				t.Fatal("preview changed user content")
			}
			if _, err := syncConfig(cfg, Options{}); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{source, target} {
				if !containsAll(hardeningRead(t, path), "BASE_RULE", "CURSOR_UNIQUE_RULE") {
					t.Fatalf("unique content missing from %s", path)
				}
			}
			if !isManagedAgentsyncFile(target) || !strings.Contains(hardeningRead(t, target), "alwaysApply: true") {
				t.Fatal("Cursor target is not a managed rule")
			}
			report, err := syncConfig(cfg, Options{Check: true})
			if err != nil || report.Results[0].Status != "ok" {
				t.Fatalf("Cursor did not converge: report=%+v error=%v", report, err)
			}
		})
	}
}

func TestManagedCursorLinkDoesNotReimportStaleBody(t *testing.T) {
	home := hardeningHome(t)
	source := filepath.Join(home, "source", "AGENTS.md")
	oldSource := filepath.Join(home, "old", "AGENTS.md")
	external := filepath.Join(home, "external", "cursor.mdc")
	target := filepath.Join(home, "cursor.mdc")
	hardeningWrite(t, source, "CURRENT_RULE\n")
	hardeningWrite(t, oldSource, "OBSOLETE_RULE\n")
	if err := writeManagedCopy(oldSource, external); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, target); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Source: source, Targets: []Target{{Path: target, Mode: "cursor"}}}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := hardeningRead(t, source); got != "CURRENT_RULE\n" {
		t.Fatalf("stale managed body was imported: %q", got)
	}
}

func TestForceDoesNotImportUnmanagedCursorContent(t *testing.T) {
	home := hardeningHome(t)
	source, target := filepath.Join(home, "AGENTS.md"), filepath.Join(home, "cursor.mdc")
	hardeningWrite(t, source, "CANONICAL_ONLY\n")
	hardeningWrite(t, target, "REPLACED_RULE\n")
	if _, err := syncConfig(Config{Source: source, Targets: []Target{{Path: target, Mode: "cursor"}}}, Options{Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := hardeningRead(t, source); got != "CANONICAL_ONLY\n" {
		t.Fatalf("force imported replaced content: %q", got)
	}
}

func TestWatchApplyAcknowledgesOnlyObservedInputs(t *testing.T) {
	hardeningHome(t)
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	hardeningWrite(t, cfg.Source, "BASE_RULE\n")
	before, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := watchApply(context.Background(), cfg, Options{Watch: true}, watchSnapshot{}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !applied.equal(before) {
		t.Fatal("watch acknowledged inputs not observed before the run")
	}
	after, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if after.equal(applied) {
		t.Fatal("fixture did not exercise source initialization during synchronization")
	}
	applied, err = watchApply(context.Background(), cfg, Options{Watch: true}, applied, "reconcile imports")
	if err != nil {
		t.Fatal(err)
	}
	if !applied.equal(after) {
		t.Fatal("watch did not reconcile its own source changes")
	}
}

func TestWatchApplyCancellationAndInvalidSource(t *testing.T) {
	hardeningHome(t)
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := watchApply(ctx, cfg, Options{Watch: true}, watchSnapshot{}, "cancelled"); err != context.Canceled {
		t.Fatalf("cancelled apply error = %v", err)
	}
	if pathExists(cfg.Source) {
		t.Fatal("cancelled apply wrote a source")
	}
	hardeningWrite(t, cfg.Source, "BASE_RULE\n")
	hardeningWrite(t, cfg.MCPSource, "{invalid-json")
	applied, err := watchApply(context.Background(), cfg, Options{Watch: true}, watchSnapshot{}, "invalid source")
	if err == nil || applied != (watchSnapshot{}) {
		t.Fatalf("failed apply was acknowledged: snapshot=%+v error=%v", applied, err)
	}
	hardeningWrite(t, cfg.MCPSource, `{"mcpServers":{}}`)
	if _, err := watchApply(context.Background(), cfg, Options{Watch: true}, watchSnapshot{}, "repaired source"); err != nil {
		t.Fatal(err)
	}
}

func TestWatchLoopReconcilesImportsAndSourceEdits(t *testing.T) {
	home := hardeningHome(t)
	oldPoll, oldDebounce, oldSettle := watchPollInterval, watchDebounce, watchDetectSettle
	watchPollInterval, watchDebounce, watchDetectSettle = 5*time.Millisecond, 5*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { watchPollInterval, watchDebounce, watchDetectSettle = oldPoll, oldDebounce, oldSettle })
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	hardeningWrite(t, cfg.Source, "BASE_RULE\n")
	hardeningWrite(t, cfg.MCPSource, `{"mcpServers":{}}`)
	cursor := filepath.Join(home, ".cursor", "rules", "AGENTS.mdc")
	if err := os.MkdirAll(filepath.Dir(cursor), 0o755); err != nil {
		t.Fatal(err)
	}
	hardeningWrite(t, filepath.Join(home, ".codeium", "windsurf", "memories", "global_rules.md"), "LATE_UNIQUE_RULE\n")
	watchLog := filepath.Join(home, "watch.log")
	stderr, err := os.Create(watchLog)
	if err != nil {
		t.Fatal(err)
	}
	previousStderr := os.Stderr
	os.Stderr = stderr
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- watchLoop(ctx, Options{Watch: true}) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("watch loop error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("watch did not stop after cancellation")
		}
		os.Stderr = previousStderr
		if err := stderr.Close(); err != nil {
			t.Error(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(hardeningRead(t, watchLog)), "\n") {
			if strings.HasPrefix(line, "agentsync watch: polling ") || line == "agentsync watch: initial sync" || line == "agentsync watch: rules or skills changed" || line == "agentsync watch: canonical source or installed runtime changed" || line == "agentsync watch: stopped" {
				continue
			}
			t.Errorf("watch emitted an unexpected failure: %s", line)
		}
	})
	waitInstruction(t, cursor, "LATE_UNIQUE_RULE")
	hardeningWrite(t, cfg.Source, hardeningRead(t, cfg.Source)+"\nSECOND_VERSION_RULE\n")
	waitInstruction(t, cursor, "SECOND_VERSION_RULE")
}

func waitInstruction(t *testing.T, path, text string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), text) {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("watch did not expose %q at %s (last read error: %v)", text, path, err)
		case <-ticker.C:
		}
	}
}

func TestBatchRejectsInvalidScanRoots(t *testing.T) {
	home := hardeningHome(t)
	file := filepath.Join(home, "file")
	hardeningWrite(t, file, "not a directory")
	for _, root := range []string{filepath.Join(home, "missing"), file} {
		if err := Run(Options{All: root, Check: true}); err == nil {
			t.Fatalf("scan of %s unexpectedly succeeded", root)
		}
	}
}

func TestBatchReportsUnreadableSubtree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory mode bits do not model Windows ACLs")
	}
	home := hardeningHome(t)
	blocked := filepath.Join(home, "unreadable")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(blocked, 0o700); err != nil {
			t.Error(err)
		}
	})
	if _, err := os.ReadDir(blocked); err == nil {
		t.Skip("effective user can read a mode-000 directory")
	}
	if err := Run(Options{All: home, Check: true}); err == nil {
		t.Fatal("scan silently ignored an unreadable subtree")
	}
}

func TestBatchPreviewApplyRecheck(t *testing.T) {
	home := hardeningHome(t)
	root := filepath.Join(home, "repositories")
	for _, name := range []string{"one", "two"} {
		repo := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		hardeningWrite(t, filepath.Join(repo, "CLAUDE.md"), name+" unique rules\n")
	}
	if err := Run(Options{All: root, Check: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if pathExists(filepath.Join(root, name, "AGENTS.md")) {
			t.Fatal("batch preview created a source")
		}
	}
	if err := Run(Options{All: root}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		repo := filepath.Join(root, name)
		if got := hardeningRead(t, filepath.Join(repo, "AGENTS.md")); got != name+" unique rules\n" {
			t.Fatalf("repository %s lost rules: %q", name, got)
		}
		if !symlinkPointsTo(filepath.Join(repo, "CLAUDE.md"), filepath.Join(repo, "AGENTS.md")) && !sameContent(filepath.Join(repo, "CLAUDE.md"), filepath.Join(repo, "AGENTS.md")) {
			t.Fatalf("repository %s is not aligned", name)
		}
	}
	if err := Run(Options{All: root, Check: true}); err != nil {
		t.Fatal(err)
	}
	if pathExists(filepath.Join(home, ".codex")) {
		t.Fatal("batch mode created an absent global runtime")
	}
}
