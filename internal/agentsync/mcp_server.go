package agentsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type mcpServer struct {
	Name    string
	Type    string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
	Cwd     string
}

func (s mcpServer) inferredType() string {
	if s.Type != "" {
		switch s.Type {
		case "stdio", "local":
			return "stdio"
		case "http", "remote", "streamable-http", "streamable_http":
			return "http"
		case "sse":
			return "sse"
		default:
			return s.Type
		}
	}
	if s.URL != "" {
		return "http"
	}
	return "stdio"
}

func (s mcpServer) isRemote() bool {
	t := s.inferredType()
	return t == "http" || t == "sse" || t == "remote"
}

func marshalCanonical(servers []mcpServer) ([]byte, error) {
	obj := map[string]any{
		"mcpServers": marshalCanonicalMap(servers),
	}
	return json.MarshalIndent(obj, "", "  ")
}

func marshalCanonicalMap(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		out[srv.Name] = canonicalEntry(srv)
	}
	return out
}

func canonicalEntry(srv mcpServer) map[string]any {
	entry := map[string]any{}
	t := srv.inferredType()
	if t != "" {
		if t == "local" {
			t = "stdio"
		}
		if t == "remote" || t == "streamable-http" || t == "streamable_http" {
			t = "http"
		}
		entry["type"] = t
	}
	if srv.isRemote() {
		if srv.URL != "" {
			entry["url"] = srv.URL
		}
		if len(srv.Headers) > 0 {
			entry["headers"] = srv.Headers
		}
		return entry
	}
	if srv.Command != "" {
		entry["command"] = srv.Command
	}
	if len(srv.Args) > 0 {
		entry["args"] = srv.Args
	}
	if len(srv.Env) > 0 {
		entry["env"] = srv.Env
	}
	if srv.Cwd != "" {
		entry["cwd"] = srv.Cwd
	}
	return entry
}

func parseCanonical(data []byte) ([]mcpServer, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("parse mcp source: empty file")
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse mcp source: %w", err)
	}
	raw, ok := root["mcpServers"]
	if !ok {
		return nil, fmt.Errorf("parse mcp source: missing mcpServers")
	}
	servers, _ := raw.(map[string]any)
	if servers == nil {
		return nil, fmt.Errorf("parse mcp source: mcpServers must be an object")
	}
	return serversFromMap(servers, "cursor"), nil
}

func serversFromMap(raw map[string]any, dialect string) []mcpServer {
	if raw == nil {
		return []mcpServer{}
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]mcpServer, 0, len(names))
	for _, name := range names {
		entry, ok := raw[name].(map[string]any)
		if !ok {
			continue
		}
		servers = append(servers, serverFromEntry(name, entry, dialect))
	}
	return servers
}

func serverFromEntry(name string, entry map[string]any, dialect string) mcpServer {
	srv := mcpServer{
		Name:    name,
		Type:    stringField(entry, "type", "transport"),
		Command: stringField(entry, "command", "cmd"),
		Args:    stringSliceField(entry, "args"),
		Env:     stringMapField(entry, "env", "environment", "envs"),
		URL:     stringField(entry, "url", "serverUrl", "httpUrl", "uri"),
		Headers: stringMapField(entry, "headers", "http_headers"),
		Cwd:     stringField(entry, "cwd"),
	}
	if dialect == "opencode" || dialect == "kilo" {
		if cmd, ok := entry["command"].([]any); ok && len(cmd) > 0 {
			srv.Command = fmt.Sprint(cmd[0])
			if len(cmd) > 1 {
				srv.Args = make([]string, 0, len(cmd)-1)
				for _, a := range cmd[1:] {
					srv.Args = append(srv.Args, fmt.Sprint(a))
				}
			}
		}
	}
	if srv.Type == "" {
		srv.Type = srv.inferredType()
	}
	return srv
}

func stringField(entry map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := entry[key]; ok {
			switch t := v.(type) {
			case string:
				return t
			}
		}
	}
	return ""
}

func stringSliceField(entry map[string]any, key string) []string {
	v, ok := entry[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return append([]string{}, t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, fmt.Sprint(item))
		}
		return out
	}
	return nil
}

func stringMapField(entry map[string]any, keys ...string) map[string]string {
	for _, key := range keys {
		v, ok := entry[key]
		if !ok {
			continue
		}
		out := map[string]string{}
		switch t := v.(type) {
		case map[string]string:
			for k, val := range t {
				out[k] = val
			}
			return out
		case map[string]any:
			for k, val := range t {
				out[k] = fmt.Sprint(val)
			}
			return out
		}
	}
	return nil
}

func sortedMCPServers(servers []mcpServer) []mcpServer {
	out := append([]mcpServer{}, servers...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sameMCPServers(a, b []mcpServer) bool {
	a = sortedMCPServers(a)
	b = sortedMCPServers(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		if a[i].inferredType() != b[i].inferredType() {
			return false
		}
		if a[i].Command != b[i].Command || a[i].Cwd != b[i].Cwd {
			return false
		}
		if a[i].URL != b[i].URL {
			return false
		}
		if !equalStringSlice(a[i].Args, b[i].Args) {
			return false
		}
		if !equalStringMap(a[i].Env, b[i].Env) {
			return false
		}
		if !equalStringMap(a[i].Headers, b[i].Headers) {
			return false
		}
	}
	return true
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func containsShellSubst(s string) bool {
	return strings.Contains(s, "$(")
}

// isCodexBundledMCP 识别 Codex 桌面/CLI 捆绑的本机服务器。
// 这些条目只对 Codex 有意义，写进其他工具会启动失败。
func isCodexBundledMCP(srv mcpServer) bool {
	base := filepath.Base(srv.Command)
	switch base {
	case "SkyComputerUseClient", "node_repl":
		return true
	}
	if strings.Contains(srv.Command, "Codex.app") || strings.Contains(srv.Cwd, "Codex.app") {
		return true
	}
	for k := range srv.Env {
		if strings.HasPrefix(k, "NODE_REPL_") || strings.HasPrefix(k, "BROWSER_USE_") {
			return true
		}
	}
	return false
}

func filterServersForDialect(servers []mcpServer, dialect string) []mcpServer {
	if dialect == "codex" {
		return servers
	}
	out := make([]mcpServer, 0, len(servers))
	for _, srv := range servers {
		if isCodexBundledMCP(srv) {
			continue
		}
		out = append(out, srv)
	}
	return out
}
