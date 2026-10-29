package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchBlockedTargetIsFailureNotAcknowledged(t *testing.T) {
	home := hardeningHome(t)
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	hardeningWrite(t, cfg.Source, "BASE_RULE\n")
	hardeningWrite(t, cfg.MCPSource, `{"mcpServers":{}}`)
	blocked := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Run(Options{Check: true}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("blocked target was successful: %v", err)
	}
	if err := Run(Options{}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("blocked apply was successful: %v", err)
	}
	if info, err := os.Stat(blocked); err != nil || !info.IsDir() {
		t.Fatalf("blocked directory changed: %v %v", info, err)
	}
}

func TestAdoptPreviewApplyAndRollback(t *testing.T) {
	home := hardeningHome(t)
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	original := "# Original\nORIGINAL_RULE\n"
	hardeningWrite(t, cfg.Source, original)
	if _, err := syncMCPNotice(cfg.Source, Options{}); err != nil {
		t.Fatal(err)
	}
	original = hardeningRead(t, cfg.Source)
	draft := filepath.Join(home, "reviewed.md")
	hardeningWrite(t, draft, "# Replacement\nADOPTED_RULE\n")
	before := storageTestSnapshot(t, home)
	if err := Run(Options{Adopt: draft, Check: true}); err != nil {
		t.Fatal(err)
	}
	assertStorageSnapshot(t, home, before)
	if err := Run(Options{Adopt: draft}); err != nil {
		t.Fatal(err)
	}
	if got := hardeningRead(t, cfg.Source); !strings.Contains(got, "ADOPTED_RULE") || strings.Contains(got, "ORIGINAL_RULE") {
		t.Fatalf("draft not adopted: %q", got)
	}
	if err := Run(Options{Rollback: "latest"}); err != nil {
		t.Fatal(err)
	}
	if got := hardeningRead(t, cfg.Source); got != original {
		t.Fatalf("adopt rollback lost source: %q", got)
	}
}

func TestAdoptRejectsUnreadableDraftBeforeBackup(t *testing.T) {
	home := hardeningHome(t)
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	hardeningWrite(t, cfg.Source, "ORIGINAL_RULE\n")
	directory := filepath.Join(home, "not-a-draft")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	before := storageTestSnapshot(t, home)
	for _, check := range []bool{false, true} {
		if _, err := adoptDraft(cfg.Source, directory, check); err == nil {
			t.Fatalf("check=%v accepted a directory as draft", check)
		}
		assertStorageSnapshot(t, home, before)
	}
}

func TestLegacyCollisionManifestRejectedBeforeRestoring(t *testing.T) {
	home := hardeningHome(t)
	root, err := backupDir()
	if err != nil {
		t.Fatal(err)
	}
	stamp := "20000101-000001"
	dir := filepath.Join(root, stamp)
	a, b := filepath.Join(home, "a"), filepath.Join(home, "b")
	hardeningWrite(t, a, "current A")
	hardeningWrite(t, b, "current B")
	hardeningWrite(t, filepath.Join(dir, "same-payload"), "overwritten backup")
	manifest := backupManifest{Version: 1, Entries: []backupManifestEntry{
		{Original: a, Name: "same-payload", Kind: "file"},
		{Original: b, Name: "same-payload", Kind: "file"},
	}}
	if err := writeBackupManifestFile(dir, manifest); err != nil {
		t.Fatal(err)
	}
	before := storageTestSnapshot(t, home)
	for _, check := range []bool{false, true} {
		if err := Run(Options{Rollback: stamp, Check: check}); err == nil || !strings.Contains(err.Error(), "shared by multiple") {
			t.Fatalf("check=%v accepted corrupted backup: %v", check, err)
		}
		assertStorageSnapshot(t, home, before)
	}
	if _, err := latestBackupStamp(); err == nil {
		t.Fatal("latest selected a corrupted collision manifest")
	}
}
