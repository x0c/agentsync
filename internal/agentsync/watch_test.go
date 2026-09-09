package agentsync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchCannotCombineWithOtherModes(t *testing.T) {
	tests := []Options{
		{Watch: true, Check: true},
		{Watch: true, Repo: true},
		{Watch: true, All: "/tmp"},
		{Watch: true, Adopt: "draft.md"},
		{Watch: true, Force: true},
	}
	for _, opts := range tests {
		t.Run(watchComboName(opts), func(t *testing.T) {
			err := Run(opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "--watch cannot be combined") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWatchSnapshotChangesWithMCPSource(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
	}
	if err := os.WriteFile(cfg.Source, []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.MCPSource, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if before.MCP == after.MCP {
		t.Fatal("MCP fingerprint should change when mcp.json appears")
	}
	if before.Agents != after.Agents {
		t.Fatal("Agents fingerprint should stay put when only mcp.json appears")
	}
}

func TestWatchSnapshotChangesWhenRuntimeAppears(t *testing.T) {
	dir := t.TempDir()
	detect := filepath.Join(dir, ".codex")
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		Targets:   []Target{{Path: filepath.Join(detect, "AGENTS.md"), Detect: detect}},
	}
	if err := os.WriteFile(cfg.Source, []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	after, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if before.Detect == after.Detect {
		t.Fatal("Detect fingerprint should change when a runtime detect dir appears")
	}
	if watchSkipMCP(before, after) {
		t.Fatal("new Detect dir must still sync MCP")
	}
}

func TestWatchSkipMCPWhenOnlyAgentsChange(t *testing.T) {
	prev := watchSnapshot{Agents: "a", MCP: "m", Skills: "s", Detect: "d"}
	next := watchSnapshot{Agents: "a2", MCP: "m", Skills: "s2", Detect: "d"}
	if !watchSkipMCP(prev, next) {
		t.Fatal("rules/skills-only change should skip MCP")
	}
	next.MCP = "m2"
	if watchSkipMCP(prev, next) {
		t.Fatal("mcp.json change must sync MCP")
	}
}

func TestWaitUntilStableTrailing(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Source: filepath.Join(dir, "AGENTS.md")}
	if err := os.WriteFile(cfg.Source, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stable, err := waitUntilStable(ctx, cfg, want, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !stable {
		t.Fatal("unchanged file should be stable")
	}
	if err := os.WriteFile(cfg.Source, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stable, err = waitUntilStable(ctx, cfg, want, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if stable {
		t.Fatal("changed file should not count as trailing-stable")
	}
}

func TestWatchTreeFingerprintSkipsSyncConflict(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo", "SKILL.md"), []byte("skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := watchTreeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo", "SKILL.md.sync-conflict-123"), []byte("noise\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := watchTreeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("sync-conflict files should not change skills fingerprint")
	}
}

func TestWatchTreeFingerprintIncludesHiddenSkills(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".system", "openai-docs")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "SKILL.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := watchTreeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "SKILL.md"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := watchTreeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("hidden .system skill changes must change fingerprint")
	}
}

func TestWatchTreeFingerprintSkipsDSStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := watchTreeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := watchTreeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal(".DS_Store should not change skills fingerprint")
	}
}

func watchComboName(opts Options) string {
	switch {
	case opts.Check:
		return "check"
	case opts.Repo:
		return "repo"
	case opts.All != "":
		return "all"
	case opts.Adopt != "":
		return "adopt"
	case opts.Force:
		return "force"
	default:
		return "watch"
	}
}
