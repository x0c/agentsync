package agentsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func writeMCPFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readMCPFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func hardeningMCPConfig(t *testing.T, dialect, format, mode, original, source string) Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("AGENTSYNC_CONFIG_HOME", filepath.Join(dir, "canonical"))
	path := filepath.Join(dir, "runtime", "config."+format)
	writeMCPFixture(t, path, original)
	cfg := Config{
		MCPSource: filepath.Join(dir, "canonical", "mcp.json"),
		MCPTargets: []MCPTarget{{
			Name: "fixture", Path: path, Detect: filepath.Dir(path),
			Dialect: dialect, Format: format, Mode: mode,
		}},
	}
	writeMCPFixture(t, cfg.MCPSource, source)
	return cfg
}

func runHardeningMCP(t *testing.T, cfg Config, opts Options) []TargetResult {
	t.Helper()
	results, _, err := syncMCP(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

func TestCodexRewritesPreserveLocalOptions(t *testing.T) {
	original := `# Root comment
model = "keep-model"
[mcp_servers.db]
url = "https://old.example/mcp"
http_headers = { Old = "stale" }
enabled = false
enabled_tools = ["read"]
disabled_tools = ["delete"]
bearer_token_env_var = "DB_TOKEN"
env_http_headers = { Authorization = "DB_HEADER" }
env_vars = ["HOST_TOKEN"]
startup_timeout_sec = 19
tool_timeout_sec = 45
required = true
future_safety_option = { policy = "deny" }
[mcp_servers.db.oauth]
client_id = "keep-client"
[mcp_servers.db.tools.delete]
approval_mode = "deny"
[mcp_servers.removed]
command = "gone"
enabled = false
[projects."/work"] # trusted project
trust_level = "trusted"
`
	source := `{"mcpServers":{"db":{"url":"https://new.example/mcp","headers":{"New":"fresh"}},"added":{"command":"new"}}}`
	cfg := hardeningMCPConfig(t, "codex", "toml", "key", original, source)
	path := cfg.MCPTargets[0].Path
	runHardeningMCP(t, cfg, Options{Check: true})
	if got := readMCPFixture(t, path); string(got) != original {
		t.Fatalf("check changed TOML: %s", got)
	}
	runHardeningMCP(t, cfg, Options{})
	data := readMCPFixture(t, path)
	root := parseTOMLFixture(t, data)
	servers := root["mcp_servers"].(map[string]any)
	db := servers["db"].(map[string]any)
	previous := parseTOMLFixture(t, []byte(original))["mcp_servers"].(map[string]any)["db"].(map[string]any)
	for key, value := range previous {
		if key != "url" && key != "http_headers" && !reflect.DeepEqual(db[key], value) {
			t.Errorf("lost local option %s: got %#v want %#v", key, db[key], value)
		}
	}
	if db["url"] != "https://new.example/mcp" || !reflect.DeepEqual(db["http_headers"], map[string]any{"New": "fresh"}) {
		t.Fatalf("connection fields not replaced: %#v", db)
	}
	if _, exists := servers["removed"]; exists {
		t.Fatal("removed canonical server survived")
	}
	if !bytes.Contains(data, []byte("[projects.\"/work\"] # trusted project\ntrust_level = \"trusted\"\n")) {
		t.Fatalf("unrelated table changed: %s", data)
	}
	results := runHardeningMCP(t, cfg, Options{})
	if len(results) != 1 || results[0].Status != "ok" || !bytes.Equal(data, readMCPFixture(t, path)) {
		t.Fatalf("not idempotent: %+v", results)
	}
}

func parseTOMLFixture(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCodexTransportRewriteClearsOnlyConnectionFields(t *testing.T) {
	for _, tc := range []struct {
		name, original, source string
		removed                []string
	}{
		{"to HTTP", "[mcp_servers.db]\ncommand='old'\nargs=['old']\nenv={ OLD='x' }\ncwd='/old'\nenabled=false\n", `{"mcpServers":{"db":{"url":"https://new.example/mcp"}}}`, []string{"command", "args", "env", "cwd"}},
		{"to stdio", "[mcp_servers.db]\nurl='https://old.example/mcp'\nhttp_headers={ OLD='x' }\nenabled=false\n", `{"mcpServers":{"db":{"command":"new"}}}`, []string{"url", "http_headers"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			servers, err := parseCanonical([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			target := MCPTarget{Dialect: "codex", Format: "toml", Mode: "key"}
			out, err := applyMCPToFile(target, []byte(tc.original), servers)
			if err != nil {
				t.Fatal(err)
			}
			db := parseTOMLFixture(t, out)["mcp_servers"].(map[string]any)["db"].(map[string]any)
			for _, key := range tc.removed {
				if _, exists := db[key]; exists {
					t.Errorf("stale connection field %s survived", key)
				}
			}
			if db["enabled"] != false {
				t.Fatal("disabled server was re-enabled")
			}
			again, err := applyMCPToFile(target, out, servers)
			if err != nil || !bytes.Equal(again, out) {
				t.Fatalf("TOML renderer not idempotent: %v\nfirst=%s\nnext=%s", err, out, again)
			}
		})
	}
}

func TestCodexTOMLBoundariesPreserveUnrelatedBytes(t *testing.T) {
	for _, tc := range []struct{ name, before, owned, after string }{
		{"commented headers", "# before\nmodel='keep'\n", "[mcp_servers.old] # MCP header\ncommand='old' # MCP command\n", "# Project comment\n[projects.'/work'] # trust\ntrust_level='trusted'\n"},
		{"quoted and spaced headers", "# keep leading whitespace\n\n", " [ 'mcp_servers' . 'old.name' ] # owned\ncommand='old'\n", " [ 'mcp_servers.literal' ] # unrelated\ncommand='keep'\n[other.mcp_servers]\ncommand='keep too'\n"},
		{"multiline values", "doc = '''\n[mcp_servers.not_a_table]\n[projects.fake] # literal\n'''\n", "[mcp_servers.old]\ncommand='old'\nargs=[\n '[not.a.table]',\n]\n", "[projects]\ntext=\"\"\"\n[mcp_servers.also_literal]\n\"\"\"\n"},
		{"root inline table", "model='keep'\n", "mcp_servers = { old = { command = 'old' } }\n", "[projects]\ntrust_level='trusted'\n"},
		{"root dotted keys", "model='keep'\n", "mcp_servers.'old.name'.command = 'old'\nmcp_servers.'old.name'.env.TOKEN='keep'\n", "[[unrelated]] # array table\nname='one'\n[[unrelated]]\nname='two'\n"},
		{"CRLF", "# before\r\nmodel='keep'\r\n", "[mcp_servers.old] # old\r\ncommand='old'\r\n", "# after\r\n[projects] # keep\r\ntrust_level='trusted'\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := tc.before + tc.owned + tc.after
			out, err := applyCodexTOML([]byte(original), map[string]any{"new": map[string]any{"command": "new"}})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(out, []byte(tc.before)) || !bytes.Contains(out, []byte(tc.after)) {
				t.Fatalf("unrelated bytes changed:\n%s", out)
			}
			for _, line := range strings.Split(tc.owned, "\n") {
				if index := strings.IndexByte(line, '#'); index >= 0 && !bytes.Contains(out, []byte(line[index:])) {
					t.Errorf("comment was lost: %q", line[index:])
				}
			}
			root := parseTOMLFixture(t, out)
			servers := root["mcp_servers"].(map[string]any)
			if len(servers) != 1 || servers["new"] == nil {
				t.Fatalf("wrong server set: %#v", servers)
			}
		})
	}
}

func TestInvalidExistingCodexTOMLIsNeverWritten(t *testing.T) {
	for _, original := range []string{"[mcp_servers.old]\ncommand='old'\n[projects\n", "model=1\nmodel=2\n", "mcp_servers = 'not an object'\n"} {
		for _, check := range []bool{false, true} {
			cfg := hardeningMCPConfig(t, "codex", "toml", "key", original, `{"mcpServers":{"new":{"command":"new"}}}`)
			if _, _, err := syncMCP(cfg, Options{Check: check}); err == nil {
				t.Fatalf("accepted invalid existing TOML: %q", original)
			}
			if got := readMCPFixture(t, cfg.MCPTargets[0].Path); string(got) != original {
				t.Fatalf("invalid TOML overwritten: %s", got)
			}
		}
	}
}

func invalidCanonicalFixtures(t *testing.T) map[string]string {
	t.Helper()
	fixtures := map[string]string{
		"null entry": `null`, "array entry": `[]`, "string entry": `"node"`,
		"boolean entry": `false`, "number entry": `42`, "empty entry": `{}`,
		"empty command": `{"command":" "}`, "empty URL": `{"url":" "}`,
		"missing remote URL": `{"type":"http"}`, "missing stdio command": `{"type":"stdio","url":"https://example.com/mcp"}`,
		"ambiguous connection": `{"command":"node","url":"https://example.com/mcp"}`,
		"unknown transport":    `{"type":"unknown","command":"node"}`,
	}
	for _, key := range []string{"type", "transport", "command", "cmd", "cwd", "url", "serverUrl", "httpUrl", "uri", "args", "env", "environment", "envs", "headers", "http_headers"} {
		for i, value := range []any{nil, 42, true, []any{1}, map[string]any{"KEY": 1}} {
			entry := map[string]any{"command": "node", key: value}
			data, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			fixtures[fmt.Sprintf("%s wrong type %d", key, i)] = string(data)
		}
	}
	fixtures["mixed argument types"] = `{"command":"node","args":["ok",null]}`
	fixtures["non-string environment"] = `{"command":"node","env":{"TOKEN":true}}`
	fixtures["non-string header"] = `{"url":"https://example.com","headers":{"Authorization":[]}}`
	return fixtures
}

func TestInvalidCanonicalEntriesFailBeforeMCPWrites(t *testing.T) {
	for name, entry := range invalidCanonicalFixtures(t) {
		t.Run(name, func(t *testing.T) {
			for _, opts := range []Options{{}, {Check: true}, {Watch: true}} {
				original := `{"mcpServers":{"keep":{"command":"keep"},"new":{"command":"new"}}}`
				source := `{"mcpServers":{"keep":` + entry + `,"new":{"command":"new"}}}`
				cfg := hardeningMCPConfig(t, "cursor", "json", "file", original, source)
				root := filepath.Dir(cfg.MCPSource)
				if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
					t.Fatal(err)
				}
				if _, _, err := syncMCP(cfg, opts); err == nil {
					t.Fatalf("accepted invalid entry: %s (options=%+v)", entry, opts)
				}
				if string(readMCPFixture(t, cfg.MCPTargets[0].Path)) != original || string(readMCPFixture(t, cfg.MCPSource)) != source {
					t.Fatal("invalid canonical input changed an MCP file")
				}
				if pathExists(filepath.Join(root, ".gitignore")) || pathExists(filepath.Join(root, "backups")) {
					t.Fatal("invalid input caused MCP side effects")
				}
			}
		})
	}
}

func TestCanonicalValidConnectionForms(t *testing.T) {
	for _, entry := range []string{
		`{"command":"node","args":[],"env":{},"cwd":"/work"}`,
		`{"type":"local","cmd":"node","environment":{"TOKEN":"value"}}`,
		`{"type":"streamable-http","serverUrl":"https://example.com/mcp","headers":{}}`,
		`{"transport":"streamable_http","uri":"https://example.com/mcp","http_headers":{"TOKEN":"value"}}`,
		`{"httpUrl":"https://example.com/mcp","enabled":false,"timeout":42,"oauth":{"clientId":"user-owned"}}`,
		`{"type":"sse","url":"https://example.com/sse"}`,
	} {
		if _, err := parseCanonical([]byte(`{"mcpServers":{"valid":` + entry + `}}`)); err != nil {
			t.Errorf("valid connection %s: %v", entry, err)
		}
	}
	if _, err := parseCanonical([]byte(`{"mcpServers":{" ":{"command":"node"}}}`)); err == nil {
		t.Fatal("accepted empty server name")
	}
}

func TestInitialMCPImportFailurePreservesEveryTarget(t *testing.T) {
	for _, invalid := range []string{
		`{"mcpServers":{"bad":null}}`, `{"mcpServers":{"bad":{}}}`,
		`{"mcpServers":{"bad":{"command":42}}}`, `{"mcpServers":`,
	} {
		for _, check := range []bool{false, true} {
			cfg := hardeningMCPConfig(t, "cursor", "json", "file", `{"mcpServers":{"good":{"command":"node"}}}`, `{}`)
			if err := os.Remove(cfg.MCPSource); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(filepath.Dir(cfg.MCPSource), "invalid-runtime.json")
			writeMCPFixture(t, other, invalid)
			cfg.MCPTargets = append(cfg.MCPTargets, MCPTarget{Name: "other", Path: other, Dialect: "cursor", Format: "json", Mode: "file"})
			before := readMCPFixture(t, cfg.MCPTargets[0].Path)
			if _, _, err := syncMCP(cfg, Options{Check: check}); err == nil {
				t.Fatalf("accepted incomplete import %s", invalid)
			}
			if pathExists(cfg.MCPSource) || !bytes.Equal(before, readMCPFixture(t, cfg.MCPTargets[0].Path)) || string(readMCPFixture(t, other)) != invalid {
				t.Fatal("incomplete import changed files")
			}
		}
	}
}

func TestGeminiTransportImportAndIdempotency(t *testing.T) {
	original := `{"theme":"keep","mcpServers":{"events":{"url":"https://example.com/sse"},"http":{"httpUrl":"https://example.com/mcp"}}}`
	cfg := hardeningMCPConfig(t, "gemini", "json", "key", original, `{}`)
	if err := os.Remove(cfg.MCPSource); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(cfg.MCPSource), "other.json")
	writeMCPFixture(t, other, `{"mcpServers":{"new":{"command":"new"}}}`)
	cfg.MCPTargets = append(cfg.MCPTargets, MCPTarget{Name: "other", Path: other, Dialect: "cursor", Format: "json", Mode: "file"})
	runHardeningMCP(t, cfg, Options{Check: true})
	if pathExists(cfg.MCPSource) || string(readMCPFixture(t, cfg.MCPTargets[0].Path)) != original {
		t.Fatal("import check wrote files")
	}
	runHardeningMCP(t, cfg, Options{})
	data := readMCPFixture(t, cfg.MCPTargets[0].Path)
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	servers := root["mcpServers"].(map[string]any)
	events := servers["events"].(map[string]any)
	http := servers["http"].(map[string]any)
	if events["url"] != "https://example.com/sse" || events["httpUrl"] != nil || http["httpUrl"] != "https://example.com/mcp" || root["theme"] != "keep" {
		t.Fatalf("transport or unrelated settings changed: %s", data)
	}
	canonical, err := parseCanonicalFile(cfg.MCPSource)
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range canonical {
		if server.Name == "events" && server.Type != "sse" {
			t.Fatalf("canonical lost SSE: %+v", server)
		}
	}
	for _, result := range runHardeningMCP(t, cfg, Options{}) {
		if result.Status != "ok" {
			t.Fatalf("not idempotent: %+v", result)
		}
	}
	if !bytes.Equal(data, readMCPFixture(t, cfg.MCPTargets[0].Path)) {
		t.Fatal("second sync rewrote Gemini")
	}
}

func TestGeminiAndQwenImportTransportPrecedence(t *testing.T) {
	for _, dialect := range []string{"gemini", "qwen"} {
		for _, tc := range []struct{ entry, transport, url string }{
			{`{"url":"https://example.com/sse"}`, "sse", "https://example.com/sse"},
			{`{"httpUrl":"https://example.com/mcp"}`, "http", "https://example.com/mcp"},
			{`{"httpUrl":"https://example.com/mcp","url":"https://example.com/sse"}`, "http", "https://example.com/mcp"},
			{`{"type":"http","url":"https://example.com/mcp"}`, "http", "https://example.com/mcp"},
			{`{"type":"sse","url":"https://example.com/sse"}`, "sse", "https://example.com/sse"},
		} {
			servers, err := extractMCPServers(dialect, "json", []byte(`{"mcpServers":{"demo":`+tc.entry+`}}`))
			if err != nil || len(servers) != 1 || servers[0].inferredType() != tc.transport || servers[0].URL != tc.url {
				t.Fatalf("%s %s: servers=%+v error=%v", dialect, tc.entry, servers, err)
			}
		}
	}
}

func TestCrushRejectsBothCommandSubstitutionFormsInAllExpandedFields(t *testing.T) {
	for _, substitution := range []string{"$(printf unsafe)", "`printf unsafe`"} {
		for _, field := range []string{"command", "args", "env", "cwd", "url", "headers"} {
			t.Run(field+substitution, func(t *testing.T) {
				entry := map[string]any{"command": "node"}
				switch field {
				case "args":
					entry[field] = []string{substitution}
				case "env":
					entry[field] = map[string]string{"TOKEN": substitution}
				case "url", "headers":
					entry = map[string]any{"url": "https://example.com/mcp"}
					if field == "url" {
						entry[field] = "https://example.com/" + substitution
					} else {
						entry[field] = map[string]string{"Authorization": substitution}
					}
				default:
					entry[field] = substitution
				}
				source, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"demo": entry}})
				if err != nil {
					t.Fatal(err)
				}
				original := `{"theme":"keep","mcp":{}}`
				cfg := hardeningMCPConfig(t, "crush", "json", "key", original, string(source))
				for _, check := range []bool{true, false} {
					results := runHardeningMCP(t, cfg, Options{Check: check})
					if len(results) != 1 || results[0].Status != "blocked" || string(readMCPFixture(t, cfg.MCPTargets[0].Path)) != original {
						t.Fatalf("substitution was not safely blocked: %+v", results)
					}
				}
			})
		}
	}
}

func TestCrushKeepsEnvironmentExpansionWithoutCommands(t *testing.T) {
	payload, err := renderCrush([]mcpServer{{Name: "demo", Command: "node", Args: []string{"${env:TOKEN}"}, Env: map[string]string{"X": "$TOKEN"}}})
	if err != nil {
		t.Fatal(err)
	}
	entry := payload["demo"].(map[string]any)
	if !reflect.DeepEqual(entry["args"], []string{"${TOKEN}"}) || !reflect.DeepEqual(entry["env"], map[string]string{"X": "$TOKEN"}) {
		t.Fatalf("ordinary environment expansion changed: %#v", entry)
	}
}

func TestQuotedDottedCodexNamesImportWithoutCollapsing(t *testing.T) {
	original := "[mcp_servers.\"foo.bar\"]\ncommand='dotted'\n[mcp_servers.\"foo.bar\".env]\nTOKEN='keep'\n[mcp_servers.foo]\ncommand='plain'\n"
	cfg := hardeningMCPConfig(t, "codex", "toml", "key", original, `{}`)
	if err := os.Remove(cfg.MCPSource); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(cfg.MCPSource), "cursor.json")
	writeMCPFixture(t, other, `{"mcpServers":{"added":{"command":"new"}}}`)
	cfg.MCPTargets = append(cfg.MCPTargets, MCPTarget{Name: "cursor", Path: other, Dialect: "cursor", Format: "json", Mode: "file"})
	runHardeningMCP(t, cfg, Options{Check: true})
	if pathExists(cfg.MCPSource) || string(readMCPFixture(t, cfg.MCPTargets[0].Path)) != original {
		t.Fatal("quoted-name import check changed files")
	}
	runHardeningMCP(t, cfg, Options{})
	canonical, err := parseCanonicalFile(cfg.MCPSource)
	if err != nil {
		t.Fatal(err)
	}
	if commandOf(canonical, "foo.bar") != "dotted" || commandOf(canonical, "foo") != "plain" || len(canonical) != 3 {
		t.Fatalf("quoted key was collapsed: %+v", canonical)
	}
	for _, server := range canonical {
		if server.Name == "foo.bar" && server.Env["TOKEN"] != "keep" {
			t.Fatalf("nested env lost: %+v", server)
		}
	}
	for _, result := range runHardeningMCP(t, cfg, Options{}) {
		if result.Status != "ok" {
			t.Fatalf("quoted names did not converge: %+v", result)
		}
	}
}
