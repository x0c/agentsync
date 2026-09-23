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
			t.Errorf("block 缺少 %q\n%s", want, text)
		}
	}
	// 按名排序保证确定性：github 必须在 web 之前。
	if strings.Index(text, "mcp-github") > strings.Index(text, "mcp-web") {
		t.Errorf("block 未按 server 名排序\n%s", text)
	}
}

func TestRenderDshPatchBlockRejects(t *testing.T) {
	if _, err := renderDshPatchBlock([]mcpServer{{Name: "bad name!"}}); err == nil || !strings.Contains(err.Error(), "dsh rejected") {
		t.Errorf("非法 serverName 应被 dsh rejected，实际 %v", err)
	}
	if _, err := renderDshPatchBlock([]mcpServer{{Name: "sse-srv", Type: "sse", URL: "http://x/y"}}); err == nil || !strings.Contains(err.Error(), "dsh rejected") {
		t.Errorf("sse 应被 dsh rejected，实际 %v", err)
	}
}

func TestSpliceDshPatchBlock(t *testing.T) {
	block, err := renderDshPatchBlock(dshTestServers())
	if err != nil {
		t.Fatalf("render error = %v", err)
	}

	// 空 patch（含官方注释头 + []）：注释保留，[] 被块替代。
	empty := "# Your patch layer for this dsh profile\n[]\n"
	next := spliceDshPatchBlock([]byte(empty), block)
	if !strings.Contains(string(next), "# Your patch layer") {
		t.Errorf("空文件注释头丢失：\n%s", next)
	}
	if strings.Contains(string(next), "[]") {
		t.Errorf("空文件的 [] 应被替代：\n%s", next)
	}

	// 已有块：原地替换，块外不动。
	handwritten := "- insert:\n    - id: mine\n      name: some-plugin\n"
	withBlock := spliceDshPatchBlock([]byte(handwritten), block)
	if !strings.Contains(string(withBlock), "id: mine") {
		t.Fatalf("手写条目丢失：\n%s", withBlock)
	}
	other, err := renderDshPatchBlock([]mcpServer{{Name: "solo", Command: "x"}})
	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	replaced := spliceDshPatchBlock(withBlock, other)
	if strings.Contains(string(replaced), "mcp-github") || !strings.Contains(string(replaced), "id: mcp-solo") {
		t.Errorf("块未被整体替换：\n%s", replaced)
	}
	if !strings.Contains(string(replaced), "id: mine") {
		t.Errorf("替换时手写条目丢失：\n%s", replaced)
	}
}

func TestExtractDshServersRoundTrip(t *testing.T) {
	servers := dshTestServers()
	block, err := renderDshPatchBlock(servers)
	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	// 块外塞一个 !!js 行：extract 只读块内，不应受影响。
	doc := append(block, []byte("- insert:\n    - id: hand\n      name: '@deepseek-ai/dsh-mcp-client'\n      config:\n        serverName: hand\n        transport: stdio\n        command: x\n        env:\n          K: !!js process.env.K\n")...)
	got, err := extractDshServers(doc)
	if err != nil {
		t.Fatalf("extract error = %v", err)
	}
	if !sameMCPServers(got, servers) {
		t.Errorf("round trip 不一致：got %+v want %+v", got, servers)
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
		t.Errorf("无依赖应 blocked，实际 %+v", result)
	}
	if backup != "" {
		t.Errorf("blocked 不应备份，实际 %q", backup)
	}
	if pathExists(target.Path) {
		t.Errorf("blocked 不应写文件")
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
		t.Fatalf("首次应 created，实际 %+v", result)
	}
	// temp DSH_HOME 下无真实 profiles 以外的校验负担：validation 因 profiles 存在会尝试
	// 找 dsh 二进制；有则跑 web profile 的 dump-config（读的是 temp home），无则跳过。
	// 无论哪条路，文件必须已落盘且可被 extract。
	data, err := os.ReadFile(target.Path)
	if err != nil {
		t.Fatalf("read = %v", err)
	}
	got, err := extractDshServers(data)
	if err != nil {
		t.Fatalf("extract = %v", err)
	}
	if !sameMCPServers(got, dshTestServers()) {
		t.Errorf("落盘内容不一致：got %+v", got)
	}

	again, _, err := syncDshMCPTarget(target, dshTestServers(), Options{}, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if again.Status != "ok" && again.Status != "blocked" {
		t.Errorf("重复运行应 ok（或 dsh 自检 blocked），实际 %+v", again)
	}

	check, _, err := syncDshMCPTarget(target, []mcpServer{{Name: "new", Command: "x"}}, Options{Check: true}, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if check.Status != "replaceable" && check.Status != "blocked" {
		t.Errorf("check 下内容变化应 replaceable，实际 %+v", check)
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
		t.Fatalf("应 created，实际 %+v", result)
	}
	data, err := os.ReadFile(target.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "node_repl") {
		t.Errorf("Codex 捆绑 server 不应写进 dsh patch")
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
				t.Errorf("dsh MCP 入口 Mode/Dialect 错误：%+v", m)
			}
			if filepath.Dir(m.Path) != dir {
				t.Errorf("dsh MCP 路径应跟随 DSH_HOME=%s，实际 %s", dir, m.Path)
			}
		}
	}
	if !found {
		t.Errorf("dsh MCP 入口缺失")
	}
}
