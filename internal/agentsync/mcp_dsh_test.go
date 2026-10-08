package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dshTestServers() []mcpServer {
	return []mcpServer{
		{Name: "web", Type: "http", URL: "http://localhost:3000/mcp", Headers: map[string]string{"Authorization": "Bearer abc"}},
		{Name: "github", Type: "stdio", Command: "npx", Args: []string{"-y", "server-github"}, Env: map[string]string{"GITHUB_TOKEN": "t:k\"en"}},
	}
}

func TestRenderDshPatchBlock(t *testing.T) {
	block, err := renderDshPatchBlock(dshTestServers())
	if err != nil {
		t.Fatalf("renderDshPatchBlock error = %v", err)
	}
	text := string(block)
	for _, want := range []string{
		dshBlockBegin, dshBlockEnd,
		"- insert:",
		"id: mcp-github",
		"id: mcp-web",
		"name: '@deepseek-ai/dsh-mcp-client'",
		"transport: stdio",
		"transport: streamable-http",
		"serverName: github",
		"url: http://localhost:3000/mcp",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("block is missing %q\n%s", want, text)
		}
	}
	// Sort by name for deterministic output: github must precede web.
	if strings.Index(text, "mcp-github") > strings.Index(text, "mcp-web") {
		t.Errorf("block is not sorted by server name\n%s", text)
	}
}

func TestRenderDshPatchBlockRejects(t *testing.T) {
	if _, err := renderDshPatchBlock([]mcpServer{{Name: "bad name!"}}); err == nil || !strings.Contains(err.Error(), "dsh rejected") {
		t.Errorf("invalid serverName should be rejected by dsh, got %v", err)
	}
	if _, err := renderDshPatchBlock([]mcpServer{{Name: "sse-srv", Type: "sse", URL: "http://x/y"}}); err == nil || !strings.Contains(err.Error(), "dsh rejected") {
		t.Errorf("SSE should be rejected by dsh, got %v", err)
	}
}

func TestSpliceDshPatchBlock(t *testing.T) {
	block, err := renderDshPatchBlock(dshTestServers())
	if err != nil {
		t.Fatalf("render error = %v", err)
	}

	// For an empty patch, preserve header comments and replace [] with the managed block.
	empty := "# Your patch layer for this dsh profile\n[]\n"
	next := spliceDshPatchBlock([]byte(empty), block)
	if !strings.Contains(string(next), "# Your patch layer") {
		t.Errorf("empty patch lost its header comments:\n%s", next)
	}
	if strings.Contains(string(next), "[]") {
		t.Errorf("empty patch should have its [] replaced:\n%s", next)
	}

	// Replace an existing block in place; preserve everything outside it.
	handwritten := "- insert:\n    - id: mine\n      name: some-plugin\n"
	withBlock := spliceDshPatchBlock([]byte(handwritten), block)
	if !strings.Contains(string(withBlock), "id: mine") {
		t.Fatalf("manual entry was lost:\n%s", withBlock)
	}
	other, err := renderDshPatchBlock([]mcpServer{{Name: "solo", Command: "x"}})
	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	replaced := spliceDshPatchBlock(withBlock, other)
	if strings.Contains(string(replaced), "mcp-github") || !strings.Contains(string(replaced), "id: mcp-solo") {
		t.Errorf("managed block was not replaced in full:\n%s", replaced)
	}
	if !strings.Contains(string(replaced), "id: mine") {
		t.Errorf("replacement lost the manual entry:\n%s", replaced)
	}
}

func TestExtractDshServersRoundTrip(t *testing.T) {
	servers := dshTestServers()
	block, err := renderDshPatchBlock(servers)
	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	// A !!js line outside the block must not affect managed-block extraction.
	doc := append(block, []byte("- insert:\n    - id: hand\n      name: '@deepseek-ai/dsh-mcp-client'\n      config:\n        serverName: hand\n        transport: stdio\n        command: x\n        env:\n          K: !!js process.env.K\n")...)
	got, err := extractDshServers(doc)
	if err != nil {
		t.Fatalf("extract error = %v", err)
	}
	if !sameMCPServers(got, servers) {
		t.Errorf("round trip mismatch: got %+v want %+v", got, servers)
	}
}

func setupDshHome(t *testing.T, withDep bool) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DSH_HOME", dir)
	if withDep {
		prof := filepath.Join(dir, "profiles", "web")
		if err := os.MkdirAll(prof, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"name":"dsh-profile-web","private":true,"dependencies":{"@deepseek-ai/dsh-mcp-client":"^0.1.0"}}`
		if err := os.WriteFile(filepath.Join(prof, "package.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSyncDshMCPTargetBlockedWithoutDep(t *testing.T) {
	setupDshHome(t, false)
	target := MCPTarget{Name: "dsh", Path: dshPatchPath(), Detect: dshHomeDir(), Dialect: "dsh", Format: "yaml", Mode: "patch"}
	result, backup, err := syncDshMCPTarget(target, dshTestServers(), Options{}, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if result.Status != "blocked" {
		t.Errorf("missing dependency should be blocked, got %+v", result)
	}
	if backup != "" {
		t.Errorf("blocked result should not create a backup, got %q", backup)
	}
	if pathExists(target.Path) {
		t.Errorf("blocked result should not write a file")
	}
}

func TestSyncDshMCPTargetCreateAndOK(t *testing.T) {
	setupDshHome(t, true)
	target := MCPTarget{Name: "dsh", Path: dshPatchPath(), Detect: dshHomeDir(), Dialect: "dsh", Format: "yaml", Mode: "patch"}

	result, _, err := syncDshMCPTarget(target, dshTestServers(), Options{}, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if result.Status != "created" {
		t.Fatalf("first sync should report created, got %+v", result)
	}
	// With profiles in the temporary DSH_HOME, validation attempts to locate dsh.
	// If available, dump the web profile from that temporary home; otherwise skip validation.
	// Either way, the written file must be extractable.
	data, err := os.ReadFile(target.Path)
	if err != nil {
		t.Fatalf("read = %v", err)
	}
	got, err := extractDshServers(data)
	if err != nil {
		t.Fatalf("extract = %v", err)
	}
	if !sameMCPServers(got, dshTestServers()) {
		t.Errorf("written content mismatch: got %+v", got)
	}

	again, _, err := syncDshMCPTarget(target, dshTestServers(), Options{}, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if again.Status != "ok" && again.Status != "blocked" {
		t.Errorf("repeated sync should report ok (or blocked by dsh validation), got %+v", again)
	}

	check, _, err := syncDshMCPTarget(target, []mcpServer{{Name: "new", Command: "x"}}, Options{Check: true}, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if check.Status != "replaceable" && check.Status != "blocked" {
		t.Errorf("changed content in check mode should report replaceable, got %+v", check)
	}
}

func TestSyncDshMCPTargetDropsCodexBundled(t *testing.T) {
	setupDshHome(t, true)
	target := MCPTarget{Name: "dsh", Path: dshPatchPath(), Detect: dshHomeDir(), Dialect: "dsh", Format: "yaml", Mode: "patch"}
	servers := []mcpServer{
		{Name: "ok-srv", Command: "npx", Args: []string{"-y", "x"}},
		{Name: "node_repl", Command: "node_repl"},
	}
	result, _, err := syncDshMCPTarget(target, servers, Options{}, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if result.Status != "created" {
		t.Fatalf("expected created, got %+v", result)
	}
	data, err := os.ReadFile(target.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "node_repl") {
		t.Errorf("Codex bundled servers must not be written into the dsh patch")
	}
	got, err := extractDshServers(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "ok-srv" {
		t.Errorf("got %+v", got)
	}
}

func TestDefaultGlobalConfigIncludesDshMCP(t *testing.T) {
	dir := setupDshHome(t, true)
	cfg, err := defaultGlobalConfig()
	if err != nil {
		t.Fatalf("defaultGlobalConfig() error = %v", err)
	}
	found := false
	for _, m := range cfg.MCPTargets {
		if m.Name == "dsh" {
			found = true
			if m.Mode != "patch" || m.Dialect != "dsh" {
				t.Errorf("unexpected dsh MCP target Mode/Dialect:%+v", m)
			}
			if filepath.Dir(m.Path) != dir {
				t.Errorf("dsh MCP path should follow DSH_HOME=%s, got %s", dir, m.Path)
			}
		}
	}
	if !found {
		t.Errorf("missing dsh MCP target")
	}
}
