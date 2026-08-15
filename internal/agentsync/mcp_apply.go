package agentsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
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

func applyCodexTOML(existing []byte, payload any) ([]byte, error) {
	rest := stripTOMLTables(existing, "mcp_servers")
	fragment, err := toml.Marshal(map[string]any{"mcp_servers": payload})
	if err != nil {
		return nil, fmt.Errorf("encode codex mcp_servers: %w", err)
	}
	rest = bytes.TrimSpace(rest)
	var b bytes.Buffer
	if len(rest) > 0 {
		b.Write(rest)
		if rest[len(rest)-1] != '\n' {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.Write(fragment)
	if b.Len() > 0 && b.Bytes()[b.Len()-1] != '\n' {
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

func stripTOMLTables(data []byte, prefix string) []byte {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			name := strings.TrimSpace(trim[1 : len(trim)-1])
			name = strings.Trim(name, "[]")
			skipping = name == prefix || strings.HasPrefix(name, prefix+".")
		}
		if skipping {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
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
