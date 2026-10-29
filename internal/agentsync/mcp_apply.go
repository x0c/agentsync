package agentsync

import (
	"bytes"
	"encoding/json"

	"gopkg.in/yaml.v3"
)

func applyMCPToFile(target MCPTarget, existing []byte, servers []mcpServer) ([]byte, error) {
	payload, err := renderMCPPayload(target.Dialect, servers)
	if err != nil {
		return nil, err
	}
	switch target.Format {
	case "toml":
		return applyCodexTOML(existing, payload)
	case "yaml":
		return applyGooseYAML(existing, payload)
	default:
		return applyMCPJSON(target, existing, payload)
	}
}

func applyMCPJSON(target MCPTarget, existing []byte, payload any) ([]byte, error) {
	if target.Dialect == "pi" {
		if err := preservePiServerOptions(existing, payload); err != nil {
			return nil, err
		}
	}
	if target.Mode == "file" {
		wrapper := map[string]any{"mcpServers": payload}
		out, err := json.MarshalIndent(wrapper, "", "  ")
		if err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		return out, nil
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	doc := existing
	if target.Format == "jsonc" {
		doc = stripJSONC(doc)
	}
	key := jsonKeyForDialect(target.Dialect)
	if target.Dialect == "grok" {
		raw, err = json.MarshalIndent(map[string]any{"servers": payload}, "", "  ")
		if err != nil {
			return nil, err
		}
		key = "mcp"
	}
	out, err := setTopLevelJSONKey(doc, key, raw)
	if err != nil {
		return nil, err
	}
	if target.Name == "opencode" && !bytes.Contains(out, []byte(`"$schema"`)) {
		schema, err := json.Marshal("https://opencode.ai/config.json")
		if err != nil {
			return nil, err
		}
		out, err = setTopLevelJSONKey(out, "$schema", schema)
		if err != nil {
			return nil, err
		}
	}
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

func jsonKeyForDialect(dialect string) string {
	switch dialect {
	case "opencode", "crush":
		return "mcp"
	case "amp":
		return "amp.mcpServers"
	case "zed":
		return "context_servers"
	default:
		return "mcpServers"
	}
}

func applyGooseYAML(existing []byte, payload any) ([]byte, error) {
	kept, err := gooseNonMCPExtensions(existing)
	if err != nil {
		return nil, err
	}
	mcp, _ := payload.(map[string]any)
	ext := map[string]any{}
	for k, v := range kept {
		ext[k] = v
	}
	for k, v := range mcp {
		ext[k] = v
	}
	var root map[string]any
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := yaml.Unmarshal(existing, &root); err != nil {
			return nil, err
		}
	}
	if root == nil {
		root = map[string]any{}
	}
	root["extensions"] = ext
	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, err
	}
	return out, nil
}
