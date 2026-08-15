package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchCannotCombineWithOtherModes(t *testing.T) {
	tests := []Options{
		{Watch: true, Check: true},
		{Watch: true, Repo: true},
		{Watch: true, All: "/tmp"},
		{Watch: true, Adopt: "draft.md"},
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

func TestWatchFingerprintChangesWithMCPSource(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
	}
	if err := os.WriteFile(cfg.Source, []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := watchFingerprint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.MCPSource, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := watchFingerprint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("fingerprint should change when mcp.json appears")
	}
}

func TestWatchFingerprintChangesWhenRuntimeAppears(t *testing.T) {
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
	before, err := watchFingerprint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	after, err := watchFingerprint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("fingerprint should change when a runtime detect dir appears")
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
	default:
		return "watch"
	}
}
