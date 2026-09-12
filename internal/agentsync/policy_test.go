package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNameAllowedDenyFirst(t *testing.T) {
	pol := ResourcePolicy{
		Default: "allow",
		Targets: map[string]TargetPolicy{
			"codex": {Allow: []string{"tavily", "memory"}, Deny: []string{"tavily"}},
		},
	}
	if nameAllowed("tavily", pol, "codex") {
		t.Fatal("deny must win over allow")
	}
	if !nameAllowed("memory", pol, "codex") {
		t.Fatal("memory should remain allowed")
	}
	if !nameAllowed("other", pol, "cursor") {
		t.Fatal("unset target with default allow should keep names")
	}
}

func TestNameAllowedAllowlistAndDefaultDeny(t *testing.T) {
	pol := ResourcePolicy{
		Default: "deny",
		Targets: map[string]TargetPolicy{
			"claude": {Allow: []string{"context7"}},
		},
	}
	if nameAllowed("tavily", pol, "claude") {
		t.Fatal("allowlist must drop unlisted names")
	}
	if !nameAllowed("context7", pol, "claude") {
		t.Fatal("listed name must pass")
	}
	if nameAllowed("context7", pol, "codex") {
		t.Fatal("default deny with no target entry must drop all")
	}
}

func TestFilterServersForPolicyCaseInsensitive(t *testing.T) {
	servers := []mcpServer{{Name: "Tavily"}, {Name: "memory"}}
	pol := ResourcePolicy{
		Default: "allow",
		Targets: map[string]TargetPolicy{
			"Codex": {Deny: []string{"tavily"}},
		},
	}
	kept, dropped := filterServersForPolicy(servers, pol, "codex")
	if len(kept) != 1 || kept[0].Name != "memory" {
		t.Fatalf("kept=%v", kept)
	}
	if len(dropped) != 1 || !strings.EqualFold(dropped[0], "Tavily") {
		t.Fatalf("dropped=%v", dropped)
	}
}

func TestLoadSyncPolicyMissingIsEmpty(t *testing.T) {
	pol, err := loadSyncPolicy(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if pol.Version != 0 || len(pol.MCP.Targets) != 0 {
		t.Fatalf("unexpected policy: %+v", pol)
	}
}

func TestLoadSyncPolicyRejectsBadDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sync-policy.json")
	if err := os.WriteFile(path, []byte(`{"mcp":{"default":"maybe"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSyncPolicy(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestUnknownPolicyTargets(t *testing.T) {
	pol := ResourcePolicy{Targets: map[string]TargetPolicy{"codex": {}, "nope": {}}}
	unknown := unknownPolicyTargets(pol, []string{"codex", "cursor"})
	if len(unknown) != 1 || unknown[0] != "nope" {
		t.Fatalf("unknown=%v", unknown)
	}
}
