package agentsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPiMCPAutoSwitchKeepsPrivateFilesAndPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	agentDir := filepath.Join(dir, "custom-pi")
	t.Setenv("PI_CODING_AGENT_DIR", agentDir)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	native := filepath.Join(agentDir, "mcp.json")
	private := filepath.Join(agentDir, "mcp-adapter.json")
	nativeText := `{"autoEnableCodemode":false,"mcpServers":{"local":{"command":"old","exposure":"deferred","timeout":42,"enabled":false,"oauth":{"clientId":"test"}}}}`
	privateText := `{"settings":{"idleTimeout":123},"imports":["cursor"]}`
	write(native, nativeText)
	write(private, privateText)
	write(filepath.Join(agentDir, "settings.json"), `{"packages":["npm:pi-mcp-adapter@3.0.0"]}`)
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	var piTarget MCPTarget
	for _, target := range cfg.MCPTargets {
		if target.Name == "pi" {
			piTarget = target
		}
	}
	if piTarget.Detect != agentDir {
		t.Fatalf("custom agent dir not used: %+v", piTarget)
	}
	cfg = Config{MCPSource: filepath.Join(dir, "source.json"), PolicyPath: filepath.Join(dir, "policy.json"), MCPTargets: []MCPTarget{piTarget}}
	write(cfg.MCPSource, `{"mcpServers":{"local":{"command":"new","args":["--stdio"]},"remote":{"url":"https://example.com/mcp"},"denied":{"command":"unused"},"node_repl":{"command":"node_repl","env":{"NODE_REPL_TEST":"1"}}}}`)
	write(cfg.PolicyPath, `{"version":1,"mcp":{"targets":{"pi":{"deny":["denied"]}}}}`)
	before, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := syncMCP(cfg, Options{Check: true}); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(dir, ".config", "mcp", "mcp.json")
	if pathExists(shared) {
		t.Fatal("check wrote adapter config")
	}
	if _, _, err := syncMCP(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(native); string(data) != nativeText {
		t.Fatal("adapter sync changed native config")
	}
	servers, err := parseCanonicalFile(shared)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || hasServer(servers, "denied") || hasServer(servers, "node_repl") {
		t.Fatalf("policy/bundled filtering failed: %+v", servers)
	}
	write(filepath.Join(agentDir, "settings.json"), `{"packages":[]}`)
	after, err := watchSnapshotOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if before.equal(after) || watchSkipMCP(before, after) {
		t.Fatal("watch missed adapter removal")
	}
	sharedBefore, _ := os.ReadFile(shared)
	if _, _, err := syncMCP(cfg, Options{Check: true}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(native); string(data) != nativeText {
		t.Fatal("native check wrote config")
	}
	if _, _, err := syncMCP(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(native)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["autoEnableCodemode"] != false {
		t.Fatal("lost native top-level setting")
	}
	entries := doc["mcpServers"].(map[string]any)
	local := entries["local"].(map[string]any)
	if local["command"] != "new" || local["exposure"] != "deferred" || local["timeout"] != float64(42) || local["enabled"] != false || local["oauth"] == nil || len(entries) != 2 {
		t.Fatalf("bad native config: %s", data)
	}
	if got, _ := os.ReadFile(private); string(got) != privateText {
		t.Fatal("sync modified adapter settings")
	}
	if got, _ := os.ReadFile(shared); string(got) != string(sharedBefore) {
		t.Fatal("native sync modified shared adapter config")
	}
	results, _, err := syncMCP(cfg, Options{})
	if err != nil || len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("not idempotent: %+v %v", results, err)
	}
}

func TestPiAdapterSelection(t *testing.T) {
	for _, tc := range []struct {
		name, settings string
		want           bool
	}{
		{"native", `{}`, false},
		{"pinned", `{"packages":["npm:pi-mcp-adapter@3.3.0"]}`, true},
		{"fork", `{"packages":["npm:@piarium/pi-mcp-adapter"]}`, true},
		{"selected", `{"packages":[{"source":"npm:pi-mcp-adapter","extensions":["index.ts"]}]}`, true},
		{"disabled", `{"packages":[{"source":"npm:pi-mcp-adapter","extensions":[]}]}`, false},
		{"direct", `{"extensions":["/example/pi-mcp-adapter/index.ts"]}`, true},
		{"excluded", `{"extensions":["-/example/pi-mcp-adapter/index.ts"]}`, false},
		{"local", `{"packages":["./renamed"]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "renamed"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "renamed", "package.json"), []byte(`{"name":"pi-mcp-adapter"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(tc.settings), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := piUsesAdapter(dir)
			if err != nil || got != tc.want {
				t.Fatalf("got %v %v want %v", got, err, tc.want)
			}
		})
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"packages":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := piUsesAdapter(dir); err == nil {
		t.Fatal("invalid settings must not silently switch MCP backends")
	}
}

func TestPiNativeRejectsUnsupportedServers(t *testing.T) {
	for _, server := range []mcpServer{{Name: "legacy", Type: "sse", URL: "https://example.com/sse"}, {Name: "bad.name", Command: "test"}} {
		if _, err := renderMCPPayload("pi", []mcpServer{server}); err == nil {
			t.Fatalf("accepted unsupported native server %+v", server)
		}
		if _, err := renderMCPPayload("cursor", []mcpServer{server}); err != nil {
			t.Fatalf("broke adapter compatibility: %v", err)
		}
	}
}

func TestPiMCPExplicitMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"packages":["npm:pi-mcp-adapter"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{MCPTargets: []MCPTarget{{Name: "pi", Detect: dir, Dialect: "pi-auto"}}}
	native, err := resolvePiMCPConfig(cfg, "native")
	if err != nil || native.MCPTargets[0].Path != filepath.Join(dir, "mcp.json") {
		t.Fatalf("native override failed: %+v %v", native, err)
	}
	if err := os.Remove(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	adapter, err := resolvePiMCPConfig(cfg, "adapter")
	if err != nil || adapter.MCPTargets[0].Path != filepath.Join(dir, ".config", "mcp", "mcp.json") {
		t.Fatalf("adapter override failed: %+v %v", adapter, err)
	}
	policy := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policy, []byte(`{"piMCP":"typo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSyncPolicy(policy); err == nil {
		t.Fatal("invalid backend selection must fail")
	}
}
