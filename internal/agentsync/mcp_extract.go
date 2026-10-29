package agentsync

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func extractMCPServers(dialect, format string, data []byte) ([]mcpServer, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return []mcpServer{}, nil
	}
	switch format {
	case "toml":
		return extractCodexTOML(data)
	case "yaml":
		return extractGooseYAML(data)
	default:
		obj, err := unmarshalJSONObject(data)
		if err != nil {
			return nil, err
		}
		return extractMCPServersFromJSON(dialect, obj), nil
	}
}

func extractMCPServersFromJSON(dialect string, obj map[string]any) []mcpServer {
	switch dialect {
	case "opencode":
		raw, _ := obj["mcp"].(map[string]any)
		return serversFromMap(raw, "opencode")
	case "amp":
		raw, _ := obj["amp.mcpServers"].(map[string]any)
		return serversFromMap(raw, "amp")
	case "crush":
		raw, _ := obj["mcp"].(map[string]any)
		return serversFromMap(raw, "crush")
	case "zed":
		raw, _ := obj["context_servers"].(map[string]any)
		return extractZed(raw)
	case "grok":
		return extractGrok(obj)
	case "windsurf":
		raw, _ := obj["mcpServers"].(map[string]any)
		return serversFromMap(raw, "windsurf")
	default:
		raw, _ := obj["mcpServers"].(map[string]any)
		return serversFromMap(raw, dialect)
	}
}

func extractZed(raw map[string]any) []mcpServer {
	if raw == nil {
		return []mcpServer{}
	}
	servers := serversFromMap(raw, "zed")
	for i, srv := range servers {
		entry, _ := raw[srv.Name].(map[string]any)
		if entry == nil {
			continue
		}
		if nested, ok := entry["command"].(map[string]any); ok {
			servers[i].Command = stringField(nested, "path", "command")
			if servers[i].Args == nil {
				servers[i].Args = stringSliceField(nested, "args")
			}
			if len(servers[i].Env) == 0 {
				servers[i].Env = stringMapField(nested, "env")
			}
		}
	}
	return servers
}

func extractGrok(obj map[string]any) []mcpServer {
	mcp, _ := obj["mcp"].(map[string]any)
	if mcp == nil {
		return []mcpServer{}
	}
	list, ok := mcp["servers"].([]any)
	if !ok {
		return []mcpServer{}
	}
	servers := make([]mcpServer, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := stringField(entry, "id", "label", "name")
		if name == "" {
			continue
		}
		servers = append(servers, serverFromEntry(name, entry, "grok"))
	}
	return servers
}

func extractCodexTOML(data []byte) ([]mcpServer, error) {
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse codex toml: %w", err)
	}
	raw, _ := root["mcp_servers"].(map[string]any)
	return serversFromMap(raw, "codex"), nil
}

func extractGooseYAML(data []byte) ([]mcpServer, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse goose yaml: %w", err)
	}
	raw, _ := root["extensions"].(map[string]any)
	if raw == nil {
		return []mcpServer{}, nil
	}
	filtered := map[string]any{}
	for name, v := range raw {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if !isGooseMCPExtension(entry) {
			continue
		}
		filtered[name] = entry
	}
	return serversFromMap(filtered, "goose"), nil
}

func isGooseMCPExtension(entry map[string]any) bool {
	t, _ := entry["type"].(string)
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "stdio", "streamable_http", "sse", "http":
		return true
	case "builtin", "platform", "frontend", "inline_python":
		return false
	case "":
		return entry["cmd"] != nil || entry["command"] != nil || entry["uri"] != nil || entry["url"] != nil
	default:
		return false
	}
}

func gooseNonMCPExtensions(data []byte) (map[string]any, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	raw, _ := root["extensions"].(map[string]any)
	kept := map[string]any{}
	for name, v := range raw {
		entry, ok := v.(map[string]any)
		if !ok {
			kept[name] = v
			continue
		}
		if isGooseMCPExtension(entry) {
			continue
		}
		kept[name] = v
	}
	return kept, nil
}
