package agentsync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func piAgentDir() string {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		if expanded, err := expandPath(dir); err == nil {
			return expanded
		}
	}
	dir, _ := expandPath("~/.pi/agent")
	return dir
}

// Resolve at each sync, so a running watcher follows package changes too.
func resolvePiMCPConfig(cfg Config, mode string) (Config, error) {
	cfg.MCPTargets = append([]MCPTarget(nil), cfg.MCPTargets...)
	for i, target := range cfg.MCPTargets {
		if target.Dialect != "pi-auto" {
			continue
		}
		adapter := mode == "adapter"
		var err error
		if mode == "" || mode == "auto" {
			adapter, err = piUsesAdapter(target.Detect)
			if err != nil {
				return cfg, err
			}
		}
		if adapter {
			target.Path, err = expandPath("~/.config/mcp/mcp.json")
			target.Dialect, target.Mode = "cursor", "file"
		} else {
			target.Path = filepath.Join(target.Detect, "mcp.json")
			target.Dialect, target.Mode = "pi", "key"
		}
		if err != nil {
			return cfg, err
		}
		cfg.MCPTargets[i] = target
	}
	return cfg, nil
}

func piUsesAdapter(agentDir string) (bool, error) {
	path := filepath.Join(agentDir, "settings.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return piHasAdapterExtension(agentDir), nil
	}
	if err != nil {
		return false, err
	}
	var settings struct {
		Packages   []json.RawMessage `json:"packages"`
		Extensions []string          `json:"extensions"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("read Pi package selection %s: %w", path, err)
	}
	for _, pkg := range settings.Packages {
		var source string
		if json.Unmarshal(pkg, &source) != nil {
			var selection struct {
				Source     string    `json:"source"`
				Extensions *[]string `json:"extensions"`
			}
			if err := json.Unmarshal(pkg, &selection); err != nil {
				return false, fmt.Errorf("invalid Pi package selection in %s: %w", path, err)
			}
			if selection.Extensions != nil && len(*selection.Extensions) == 0 {
				continue
			}
			source = selection.Source
		}
		if piAdapterSource(source, agentDir) {
			return true, nil
		}
	}
	for _, extension := range settings.Extensions {
		if !strings.HasPrefix(extension, "-") && !strings.HasPrefix(extension, "!") && piAdapterSource(strings.TrimPrefix(extension, "+"), agentDir) {
			return true, nil
		}
	}
	return piHasAdapterExtension(agentDir), nil
}

func piAdapterSource(source, agentDir string) bool {
	if strings.Contains(source, "pi-mcp-adapter") {
		return true
	}
	// Local packages may have arbitrary directory names; inspect their manifest.
	if strings.HasPrefix(source, "npm:") || strings.HasPrefix(source, "git:") || source == "" {
		return false
	}
	path := source
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") {
		path = filepath.Join(agentDir, path)
	}
	path, err := expandPath(path)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return false
	}
	var manifest struct {
		Name string `json:"name"`
	}
	return json.Unmarshal(data, &manifest) == nil && strings.Contains(manifest.Name, "pi-mcp-adapter")
}

func piHasAdapterExtension(agentDir string) bool {
	entries, _ := os.ReadDir(filepath.Join(agentDir, "extensions"))
	for _, entry := range entries {
		if piAdapterSource(entry.Name(), filepath.Join(agentDir, "extensions")) {
			return true
		}
	}
	return false
}

func renderPi(servers []mcpServer) (map[string]any, error) {
	for _, server := range servers {
		if server.inferredType() != "stdio" && server.inferredType() != "http" {
			return nil, fmt.Errorf("native Pi MCP server %q uses unsupported transport %q; use stdio or streamable HTTP, or an adapter", server.Name, server.inferredType())
		}
		if server.Name == "" || strings.IndexFunc(server.Name, func(r rune) bool {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
				return false
			default:
				return true
			}
		}) >= 0 {
			return nil, fmt.Errorf("native Pi MCP server %q has an invalid name; use letters, digits, underscores or hyphens", server.Name)
		}
	}
	return marshalCanonicalMap(normalizeCursor(servers)), nil
}

func preservePiServerOptions(existing []byte, payload any) error {
	if len(existing) == 0 {
		return nil
	}
	var doc struct {
		Servers map[string]map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(existing, &doc); err != nil {
		return err
	}
	for name, value := range payload.(map[string]any) {
		entry := value.(map[string]any)
		for _, key := range []string{"exposure", "toolExposure", "oauth", "timeout", "enabled"} {
			if option, ok := doc.Servers[name][key]; ok {
				entry[key] = option
			}
		}
	}
	return nil
}
