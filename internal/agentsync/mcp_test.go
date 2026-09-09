package agentsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestSyncMCPCheckDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "cursor")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{
			{
				Name:    "cursor",
				Path:    filepath.Join(detect, "mcp.json"),
				Detect:  detect,
				Dialect: "cursor",
				Format:  "json",
				Mode:    "file",
			},
		},
	}
	if _, err := syncConfig(cfg, Options{Check: true}); err != nil {
		t.Fatalf("syncConfig(check) error = %v", err)
	}
	if pathExists(cfg.MCPSource) || pathExists(cfg.MCPTargets[0].Path) {
		t.Fatalf("check mode wrote mcp files")
	}
}

func TestSyncMCPSkipsUninstalledRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "missing-runtime")
	target := filepath.Join(detect, "mcp.json")
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "cursor",
			Path:    target,
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if err := os.WriteFile(cfg.Source, []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	if pathExists(detect) || pathExists(target) {
		t.Fatalf("skipped runtime should not be created")
	}
	if !hasMCPStatus(report, target, "skipped") {
		t.Fatalf("expected skipped, got %+v", report.MCPResults)
	}
}

func TestSyncMCPJoyCodeDoesNotCreateDetect(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, ".joycode")
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "joycode",
			Path:    filepath.Join(detect, "joycode-mcp.json"),
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if err := os.WriteFile(cfg.Source, []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	if pathExists(detect) {
		t.Fatalf("joycode detect directory must not be created")
	}
}

func TestSyncMCPPiWritesSharedGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	piAgentDir := filepath.Join(dir, ".pi", "agent")
	if err := os.MkdirAll(piAgentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, ".config", "mcp", "mcp.json")
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "pi",
			Path:    target,
			Detect:  piAgentDir,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if err := os.WriteFile(cfg.MCPSource, []byte(`{
  "mcpServers": {
    "shared": {"type": "http", "url": "https://example/mcp", "headers": {"x-token": "t"}},
    "local": {"type": "stdio", "command": "npx", "args": ["-y", "some-mcp"]},
    "node_repl": {"type": "stdio", "command": "/tmp/node_repl", "env": {"NODE_REPL_FOO": "1"}}
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	servers, err := parseCanonicalFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !hasServer(servers, "shared") || !hasServer(servers, "local") {
		t.Fatalf("pi shared global config missing servers: %+v", servers)
	}
	if hasServer(servers, "node_repl") {
		t.Fatalf("pi must not receive Codex bundled servers: %+v", servers)
	}
}

func TestSyncMCPPiSkippedWithoutAgentDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, ".config", "mcp", "mcp.json")
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "pi",
			Path:    target,
			Detect:  filepath.Join(dir, ".pi", "agent"),
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if err := os.WriteFile(cfg.MCPSource, []byte(`{"mcpServers": {"shared": {"type": "http", "url": "https://example/mcp"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if pathExists(filepath.Dir(target)) {
		t.Fatalf("pi not installed: shared mcp dir must not be created")
	}
	if !hasMCPStatus(report, target, "skipped") {
		t.Fatalf("expected skipped, got %+v", report.MCPResults)
	}
}

func TestSyncMCPImportsUnionThenOverwrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cursorDir := filepath.Join(dir, "cursor")
	claudeDir := filepath.Join(dir, "claude")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{
  "mcpServers": {
    "shared": {"type": "stdio", "command": "from-cursor"},
    "only-cursor": {"type": "stdio", "command": "cursor-cmd"}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{
  "oauth": {"token": "keep-me"},
  "mcpServers": {
    "shared": {"type": "stdio", "command": "from-claude"},
    "only-claude": {"type": "stdio", "command": "claude-cmd"}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{
			{
				Name:    "cursor",
				Path:    filepath.Join(cursorDir, "mcp.json"),
				Detect:  cursorDir,
				Dialect: "cursor",
				Format:  "json",
				Mode:    "file",
			},
			{
				Name:    "claude",
				Path:    filepath.Join(dir, ".claude.json"),
				Detect:  claudeDir,
				Dialect: "claude",
				Format:  "json",
				Mode:    "key",
			},
		},
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("syncConfig() error = %v", err)
	}
	source, err := parseCanonicalFile(cfg.MCPSource)
	if err != nil {
		t.Fatal(err)
	}
	if commandOf(source, "shared") != "from-cursor" {
		t.Fatalf("first-wins import failed: %+v", source)
	}
	if commandOf(source, "only-claude") != "claude-cmd" {
		t.Fatalf("union import missed claude-only server: %+v", source)
	}
	cursorServers, err := parseCanonicalFile(filepath.Join(cursorDir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if commandOf(cursorServers, "only-claude") != "claude-cmd" {
		t.Fatalf("cursor was not overwritten with union: %+v", cursorServers)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(cfg.MCPSource)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("new mcp.json mode = %o, want 0600", info.Mode().Perm())
		}
	}
	claudeRaw, err := os.ReadFile(filepath.Join(dir, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeRaw), `"oauth"`) || !strings.Contains(string(claudeRaw), `"keep-me"`) {
		t.Fatalf("claude key-merge dropped other keys: %s", claudeRaw)
	}
	report, err = syncConfig(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.MCPResults {
		if result.Path == cfg.Source {
			continue
		}
		if result.Status != "ok" && result.Status != "skipped" {
			t.Fatalf("second run should be idempotent, got %+v", report.MCPResults)
		}
	}
}

func TestImportMCPUnionCollapsesCaseVariants(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, "cursor")
	codexDir := filepath.Join(dir, "codex")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`[mcp_servers.mobbin]
url = "https://example/mcp"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{
  "mcpServers": {
    "Mobbin": {"type": "http", "url": "https://example/mcp"}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	imported := importMCPUnion([]MCPTarget{
		{Name: "codex", Path: filepath.Join(codexDir, "config.toml"), Detect: codexDir, Dialect: "codex", Format: "toml", Mode: "key"},
		{Name: "cursor", Path: filepath.Join(cursorDir, "mcp.json"), Detect: cursorDir, Dialect: "cursor", Format: "json", Mode: "file"},
	})
	if len(imported) != 1 {
		t.Fatalf("case variants should collapse, got %+v", imported)
	}
	if imported[0].Name != "mobbin" {
		t.Fatalf("first-wins should keep lowercase name, got %q", imported[0].Name)
	}
}

func TestCodexBundledServersStayOnCodexOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	cursorDir := filepath.Join(dir, "cursor")
	codexDir := filepath.Join(dir, "codex")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{
  "mcpServers": {
    "shared": {"type": "http", "url": "https://example/mcp"},
    "node_repl": {"type": "stdio", "command": "/tmp/node_repl", "env": {"NODE_REPL_FOO": "1"}},
    "computer-use": {"type": "stdio", "command": "/tmp/SkyComputerUseClient", "cwd": "/tmp"}
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{
			{Name: "codex", Path: filepath.Join(codexDir, "config.toml"), Detect: codexDir, Dialect: "codex", Format: "toml", Mode: "key"},
			{Name: "cursor", Path: filepath.Join(cursorDir, "mcp.json"), Detect: cursorDir, Dialect: "cursor", Format: "json", Mode: "file"},
		},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	cursorServers, err := parseCanonicalFile(filepath.Join(cursorDir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasServer(cursorServers, "shared") {
		t.Fatalf("cursor missing shared: %+v", cursorServers)
	}
	if hasServer(cursorServers, "node_repl") || hasServer(cursorServers, "computer-use") {
		t.Fatalf("cursor should not receive Codex bundled servers: %+v", cursorServers)
	}
	raw, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	codexServers, err := extractCodexTOML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !hasServer(codexServers, "node_repl") || !hasServer(codexServers, "computer-use") {
		t.Fatalf("codex should keep bundled servers: %+v", codexServers)
	}
}

func TestExtractCodexNestedEnvIsNotSeparateServer(t *testing.T) {
	data := []byte(`[mcp_servers.node_repl]
command = "node"
args = ["repl.js"]

[mcp_servers.node_repl.env]
FOO = "bar"
BAZ = "qux"
`)
	servers, err := extractCodexTOML(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("nested env table should not become another server, got %+v", servers)
	}
	if servers[0].Name != "node_repl" {
		t.Fatalf("server name = %q", servers[0].Name)
	}
	if servers[0].Command != "node" {
		t.Fatalf("command = %q", servers[0].Command)
	}
	if servers[0].Env["FOO"] != "bar" || servers[0].Env["BAZ"] != "qux" {
		t.Fatalf("env not folded into parent: %+v", servers[0].Env)
	}
}

func TestSyncMCPClaudeKeyMergeKeepsOtherKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "claude")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(path, []byte(`{
  "numStartups": 9,
  "mcpServers": {
    "old": {"type": "stdio", "command": "old"}
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{
  "mcpServers": {
    "new": {"type": "http", "url": "https://example/mcp"}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "claude",
			Path:    path,
			Detect:  detect,
			Dialect: "claude",
			Format:  "json",
			Mode:    "key",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["numStartups"] != float64(9) {
		t.Fatalf("numStartups lost: %s", raw)
	}
	servers, _ := obj["mcpServers"].(map[string]any)
	if _, ok := servers["old"]; ok {
		t.Fatalf("old server should be replaced: %s", raw)
	}
	if _, ok := servers["new"]; !ok {
		t.Fatalf("new server missing: %s", raw)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("claude.json mode changed: %o", info.Mode().Perm())
	}
}

func TestSyncMCPOpenCodeCommandArrayAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "opencode")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(detect, "opencode.json")
	if err := os.WriteFile(path, []byte(`{"$schema":"https://opencode.ai/config.json","model":"x","mcp":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{
  "mcpServers": {
    "demo": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "demo-mcp"],
      "env": {"TOKEN": "secret"}
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "opencode",
			Path:    path,
			Detect:  detect,
			Dialect: "opencode",
			Format:  "json",
			Mode:    "key",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["model"] != "x" {
		t.Fatalf("opencode non-mcp key lost: %s", raw)
	}
	mcp, _ := obj["mcp"].(map[string]any)
	demo, _ := mcp["demo"].(map[string]any)
	cmd, _ := demo["command"].([]any)
	if len(cmd) != 3 || cmd[0] != "npx" || cmd[1] != "-y" || cmd[2] != "demo-mcp" {
		t.Fatalf("opencode command should be argv array: %s", raw)
	}
	if _, ok := demo["env"]; ok {
		t.Fatalf("opencode should use environment not env: %s", raw)
	}
	env, _ := demo["environment"].(map[string]any)
	if env["TOKEN"] != "secret" {
		t.Fatalf("opencode environment missing: %s", raw)
	}
}

func TestSyncMCPCodexTOMLKeepsOtherTables(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "codex")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(detect, "config.toml")
	if err := os.WriteFile(path, []byte(`model = "gpt-5"
model_reasoning_effort = "high"

[mcp_servers.old]
command = "old"

[projects]
foo = "bar"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{
  "mcpServers": {
    "demo": {"type": "http", "url": "https://example/mcp", "headers": {"Authorization": "Bearer x"}}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "codex",
			Path:    path,
			Detect:  detect,
			Dialect: "codex",
			Format:  "toml",
			Mode:    "key",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `model = "gpt-5"`) || !strings.Contains(text, "foo") {
		t.Fatalf("codex non-mcp tables lost: %s", text)
	}
	if strings.Contains(text, "mcp_servers.old") || strings.Contains(text, `command = "old"`) {
		t.Fatalf("old mcp server still present: %s", text)
	}
	var root map[string]any
	if err := toml.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	servers, _ := root["mcp_servers"].(map[string]any)
	demo, _ := servers["demo"].(map[string]any)
	if demo["url"] != "https://example/mcp" {
		t.Fatalf("codex url missing: %s", text)
	}
	if _, ok := demo["type"]; ok {
		t.Fatalf("codex should not write type: %s", text)
	}
	headers, _ := demo["http_headers"].(map[string]any)
	if headers["Authorization"] != "Bearer x" {
		t.Fatalf("codex http_headers missing: %s", text)
	}
}

func TestSyncMCPCrushRejectsShellSubst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "crush")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(detect, "crush.json")
	original := `{"model":"keep","mcp":{}}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{
  "mcpServers": {
    "bad": {"type": "stdio", "command": "echo", "args": ["$(whoami)"]}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "crush",
			Path:    path,
			Detect:  detect,
			Dialect: "crush",
			Format:  "json",
			Mode:    "key",
		}},
	}
	report, err := syncConfig(cfg, Options{})
	if err != nil {
		t.Fatalf("crush reject should not abort run: %v", err)
	}
	if !hasMCPStatus(report, path, "blocked") {
		t.Fatalf("expected blocked, got %+v", report.MCPResults)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("crush file should be unchanged: %s", got)
	}
}

func TestSyncMCPCodeBuddyLegacyKeyMerge(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	home := filepath.Join(dir, "home")
	detect := filepath.Join(home, ".codebuddy")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, ".codebuddy.json")
	if err := os.WriteFile(legacy, []byte(`{"theme":"dark","mcpServers":{"old":{"command":"old"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{"new":{"type":"stdio","command":"new"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "codebuddy",
			Path:    filepath.Join(detect, ".mcp.json"),
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	if pathExists(filepath.Join(detect, ".mcp.json")) {
		t.Fatalf("must not create .mcp.json when legacy file exists")
	}
	raw, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["theme"] != "dark" {
		t.Fatalf("legacy non-mcp key lost: %s", raw)
	}
	servers, _ := obj["mcpServers"].(map[string]any)
	if _, ok := servers["new"]; !ok {
		t.Fatalf("legacy mcpServers not updated: %s", raw)
	}
}

func TestSyncMCPGitignoreWhenConfigIsGitRepo(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	t.Setenv("AGENTSYNC_CONFIG_HOME", root)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	detect := filepath.Join(dir, "cursor")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(root, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "cursor",
			Path:    filepath.Join(detect, "mcp.json"),
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !gitignoreHasMCP(string(data)) {
		t.Fatalf("mcp.json not ignored: %s", data)
	}
	if !ignoreHasEntry(string(data), "backups/") || !ignoreHasEntry(string(data), "merge-drafts/") {
		t.Fatalf("secret dirs not ignored: %s", data)
	}
}

func TestSyncMCPStignoreWhenConfigIsSyncthingFolder(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	t.Setenv("AGENTSYNC_CONFIG_HOME", root)
	if err := os.MkdirAll(filepath.Join(root, ".stfolder"), 0o755); err != nil {
		t.Fatal(err)
	}
	detect := filepath.Join(dir, "cursor")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(root, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "cursor",
			Path:    filepath.Join(detect, "mcp.json"),
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".stignore"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, line := range []string{"mcp.json", "backups/", "merge-drafts/"} {
		if !ignoreHasEntry(got, line) {
			t.Fatalf("%s not in .stignore: %s", line, got)
		}
	}
}

func TestParseCanonicalRejectsEmptyAndMissingKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty file", input: "", wantErr: "empty file"},
		{name: "whitespace", input: "  \n", wantErr: "empty file"},
		{name: "missing key", input: "{}", wantErr: "missing mcpServers"},
		{name: "null servers", input: `{"mcpServers":null}`, wantErr: "must be an object"},
		{name: "array servers", input: `{"mcpServers":[]}`, wantErr: "must be an object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCanonical([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
	servers, err := parseCanonical([]byte(`{"mcpServers":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("empty object should parse as no servers, got %+v", servers)
	}
}

func TestSyncMCPEmptyFileDoesNotWipeTargets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "cursor")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(detect, "mcp.json")
	original := `{"mcpServers":{"keep-me":{"type":"stdio","command":"keep"}}}` + "\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "cursor",
			Path:    target,
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err == nil {
		t.Fatal("expected empty mcp.json to error")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("empty mcp.json must not wipe targets, got %s", got)
	}
}

func TestSyncMCPWatchRefusesEmptyWhenTargetsHaveServers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "cursor")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(detect, "mcp.json")
	original := `{
  "mcpServers": {
    "keep-me": {"type": "stdio", "command": "keep"}
  }
}
`
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "cursor",
			Path:    target,
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if _, err := syncConfig(cfg, Options{Watch: true}); err == nil {
		t.Fatal("expected refuse empty mcp.json while watching")
	} else if !strings.Contains(err.Error(), "refusing to apply empty mcp.json") {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("watch must not wipe target MCP, got %s", got)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	wiped, err := parseCanonicalFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(wiped) != 0 {
		t.Fatalf("manual sync should apply explicit empty mcpServers, got %+v", wiped)
	}
}

func TestSyncMCPSkipDoesNotOverwriteTargetExtras(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "config"))
	detect := filepath.Join(dir, "cursor")
	if err := os.MkdirAll(detect, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{
  "mcpServers": {
    "canonical": {"type": "stdio", "command": "canon"}
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(detect, "mcp.json")
	cfg := Config{
		Source:    filepath.Join(dir, "AGENTS.md"),
		MCPSource: filepath.Join(dir, "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name:    "cursor",
			Path:    target,
			Detect:  detect,
			Dialect: "cursor",
			Format:  "json",
			Mode:    "file",
		}},
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{
  "mcpServers": {
    "canonical": {"type": "stdio", "command": "canon"},
    "from-ui": {"type": "stdio", "command": "ui"}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfig(cfg, Options{SkipMCP: true}); err != nil {
		t.Fatal(err)
	}
	kept, err := parseCanonicalFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if commandOf(kept, "from-ui") != "ui" {
		t.Fatalf("SkipMCP must leave UI-added server, got %+v", kept)
	}
	if _, err := syncConfig(cfg, Options{}); err != nil {
		t.Fatal(err)
	}
	overwritten, err := parseCanonicalFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if commandOf(overwritten, "from-ui") != "" {
		t.Fatalf("full sync should drop UI-added server, got %+v", overwritten)
	}
}

func TestUpsertMCPNoticeIdempotent(t *testing.T) {
	body := "rules\n"
	once, changed := upsertMCPNotice(body)
	if !changed {
		t.Fatal("first inject should change")
	}
	twice, changed := upsertMCPNotice(once)
	if changed {
		t.Fatalf("second inject should be no-op: %q", twice)
	}
	if !strings.Contains(twice, mcpNoticeBegin) || !strings.Contains(twice, "mcp.json") {
		t.Fatalf("notice missing: %q", twice)
	}
}

func TestRenderMCPDialects(t *testing.T) {
	servers := []mcpServer{{
		Name:    "demo",
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "demo"},
		Env:     map[string]string{"A": "1"},
	}}
	tests := []struct {
		name    string
		dialect string
		want    string
		not     string
	}{
		{name: "cursor type http not streamable", dialect: "cursor", want: `"command":"npx"`},
		{name: "gemini uses env", dialect: "gemini", want: `"env"`},
		{name: "zed flat command", dialect: "zed", want: `"command":"npx"`},
		{name: "goose cmd envs", dialect: "goose", want: `"cmd"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := renderMCPPayload(tt.dialect, servers)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tt.want) {
				t.Fatalf("got %s, want substring %s", raw, tt.want)
			}
			if tt.not != "" && strings.Contains(string(raw), tt.not) {
				t.Fatalf("got %s, must not contain %s", raw, tt.not)
			}
		})
	}
	zed := renderZed(servers)
	entry, _ := zed["demo"].(map[string]any)
	if _, ok := entry["command"].(string); !ok {
		t.Fatalf("zed command must be flat string: %#v", entry["command"])
	}
	if _, ok := entry["source"]; ok {
		t.Fatalf("zed must not write source: %#v", entry)
	}
}

func parseCanonicalFile(path string) ([]mcpServer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseCanonical(data)
}

func commandOf(servers []mcpServer, name string) string {
	for _, srv := range servers {
		if srv.Name == name {
			return srv.Command
		}
	}
	return ""
}

func hasServer(servers []mcpServer, name string) bool {
	for _, srv := range servers {
		if srv.Name == name {
			return true
		}
	}
	return false
}

func hasMCPStatus(report RunReport, path, status string) bool {
	for _, result := range report.MCPResults {
		if result.Path == path && result.Status == status {
			return true
		}
	}
	return false
}
