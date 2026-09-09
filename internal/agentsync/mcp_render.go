package agentsync

import (
	"fmt"
	"strings"
)

func renderMCPPayload(dialect string, servers []mcpServer) (any, error) {
	servers = filterServersForDialect(servers, dialect)
	switch dialect {
	case "cursor", "claude":
		return marshalCanonicalMap(normalizeCursor(servers)), nil
	case "windsurf":
		return renderWindsurf(servers), nil
	case "opencode":
		return renderOpenCode(servers), nil
	case "gemini":
		return renderGemini(servers), nil
	case "amp":
		return renderAmp(servers), nil
	case "crush":
		payload, err := renderCrush(servers)
		return payload, err
	case "grok":
		return renderGrok(servers), nil
	case "zed":
		return renderZed(servers), nil
	case "codex":
		return renderCodex(servers), nil
	case "goose":
		return renderGoose(servers), nil
	default:
		return nil, fmt.Errorf("unknown mcp dialect %q", dialect)
	}
}

func normalizeCursor(servers []mcpServer) []mcpServer {
	out := make([]mcpServer, 0, len(servers))
	for _, srv := range servers {
		srv.Type = srv.inferredType()
		if srv.Type == "streamable-http" || srv.Type == "streamable_http" || srv.Type == "remote" {
			srv.Type = "http"
		}
		if srv.Type == "local" {
			srv.Type = "stdio"
		}
		out = append(out, srv)
	}
	return out
}

func renderWindsurf(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		entry := map[string]any{}
		if srv.isRemote() {
			if srv.URL != "" {
				entry["serverUrl"] = srv.URL
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
		} else {
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
		}
		out[srv.Name] = entry
	}
	return out
}

func renderOpenCode(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		entry := map[string]any{"enabled": true}
		if srv.isRemote() {
			entry["type"] = "remote"
			if srv.URL != "" {
				entry["url"] = srv.URL
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
		} else {
			entry["type"] = "local"
			cmd := []string{}
			if srv.Command != "" {
				cmd = append(cmd, srv.Command)
			}
			cmd = append(cmd, srv.Args...)
			entry["command"] = cmd
			if len(srv.Env) > 0 {
				entry["environment"] = srv.Env
			}
			if srv.Cwd != "" {
				entry["cwd"] = srv.Cwd
			}
		}
		out[srv.Name] = entry
	}
	return out
}

func renderGemini(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		entry := map[string]any{}
		t := srv.inferredType()
		if t == "sse" {
			if srv.URL != "" {
				entry["url"] = srv.URL
			}
		} else if srv.isRemote() {
			if srv.URL != "" {
				entry["httpUrl"] = srv.URL
			}
		} else {
			if srv.Command != "" {
				entry["command"] = srv.Command
			}
			if len(srv.Args) > 0 {
				entry["args"] = srv.Args
			}
			if srv.Cwd != "" {
				entry["cwd"] = srv.Cwd
			}
		}
		if len(srv.Env) > 0 {
			entry["env"] = srv.Env
		}
		if len(srv.Headers) > 0 {
			entry["headers"] = srv.Headers
		}
		out[srv.Name] = entry
	}
	return out
}

func renderAmp(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		entry := map[string]any{}
		if srv.isRemote() {
			if srv.URL != "" {
				entry["url"] = srv.URL
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
		} else {
			if srv.Command != "" {
				entry["command"] = srv.Command
			}
			if len(srv.Args) > 0 {
				entry["args"] = srv.Args
			}
			if len(srv.Env) > 0 {
				entry["env"] = srv.Env
			}
		}
		out[srv.Name] = entry
	}
	return out
}

func renderCrush(servers []mcpServer) (map[string]any, error) {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		if mcpHasShellSubst(srv) {
			return nil, fmt.Errorf("crush rejected server %q: value contains $(...)", srv.Name)
		}
		entry := map[string]any{}
		t := srv.inferredType()
		if t == "sse" {
			entry["type"] = "sse"
		} else if srv.isRemote() {
			entry["type"] = "http"
		} else {
			entry["type"] = "stdio"
		}
		if srv.isRemote() {
			if srv.URL != "" {
				entry["url"] = crushExpand(srv.URL)
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = crushExpandMap(srv.Headers)
			}
		} else {
			if srv.Command != "" {
				entry["command"] = crushExpand(srv.Command)
			}
			if len(srv.Args) > 0 {
				entry["args"] = crushExpandSlice(srv.Args)
			}
			if len(srv.Env) > 0 {
				entry["env"] = crushExpandMap(srv.Env)
			}
			if srv.Cwd != "" {
				entry["cwd"] = crushExpand(srv.Cwd)
			}
		}
		out[srv.Name] = entry
	}
	return out, nil
}

func renderGrok(servers []mcpServer) []any {
	out := make([]any, 0, len(servers))
	for _, srv := range sortedMCPServers(servers) {
		t := srv.inferredType()
		transport := "stdio"
		if t == "sse" {
			transport = "sse"
		} else if srv.isRemote() {
			transport = "http"
		}
		entry := map[string]any{
			"id":        srv.Name,
			"label":     srv.Name,
			"enabled":   true,
			"transport": transport,
		}
		if srv.isRemote() {
			if srv.URL != "" {
				entry["url"] = srv.URL
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
		} else {
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
		}
		out = append(out, entry)
	}
	return out
}

func renderZed(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		entry := map[string]any{}
		if srv.isRemote() {
			if srv.URL != "" {
				entry["url"] = srv.URL
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
		} else {
			if srv.Command != "" {
				entry["command"] = srv.Command
			}
			if srv.Args != nil {
				entry["args"] = srv.Args
			} else {
				entry["args"] = []string{}
			}
			if len(srv.Env) > 0 {
				entry["env"] = srv.Env
			}
		}
		out[srv.Name] = entry
	}
	return out
}

func renderCodex(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		entry := map[string]any{}
		if srv.isRemote() {
			if srv.URL != "" {
				entry["url"] = srv.URL
			}
			if len(srv.Headers) > 0 {
				entry["http_headers"] = srv.Headers
			}
		} else {
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
		}
		out[srv.Name] = entry
	}
	return out
}

func renderGoose(servers []mcpServer) map[string]any {
	out := map[string]any{}
	for _, srv := range sortedMCPServers(servers) {
		t := "stdio"
		if srv.inferredType() == "sse" {
			t = "sse"
		} else if srv.isRemote() {
			t = "streamable_http"
		}
		entry := map[string]any{
			"name":    srv.Name,
			"enabled": true,
			"type":    t,
		}
		if srv.isRemote() {
			if srv.URL != "" {
				entry["uri"] = srv.URL
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
		} else {
			if srv.Command != "" {
				entry["cmd"] = srv.Command
			}
			if len(srv.Args) > 0 {
				entry["args"] = srv.Args
			}
			if len(srv.Env) > 0 {
				entry["envs"] = srv.Env
			}
		}
		out[srv.Name] = entry
	}
	return out
}

func mcpHasShellSubst(srv mcpServer) bool {
	if containsShellSubst(srv.Command) || containsShellSubst(srv.URL) || containsShellSubst(srv.Cwd) {
		return true
	}
	for _, a := range srv.Args {
		if containsShellSubst(a) {
			return true
		}
	}
	for _, v := range srv.Env {
		if containsShellSubst(v) {
			return true
		}
	}
	for _, v := range srv.Headers {
		if containsShellSubst(v) {
			return true
		}
	}
	return false
}

func crushExpand(s string) string {
	s = strings.ReplaceAll(s, "${env:", "${")
	return s
}

func crushExpandSlice(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = crushExpand(v)
	}
	return out
}

func crushExpandMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = crushExpand(v)
	}
	return out
}
