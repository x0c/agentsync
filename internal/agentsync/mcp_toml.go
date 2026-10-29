package agentsync

import (
	"bytes"
	"fmt"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

func applyCodexTOML(existing []byte, payload any) ([]byte, error) {
	var root map[string]any
	if err := toml.Unmarshal(existing, &root); err != nil {
		return nil, fmt.Errorf("parse codex toml: %w", err)
	}
	servers, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("codex mcp payload must be an object")
	}
	if err := preserveCodexServerOptions(root, servers); err != nil {
		return nil, err
	}
	rest, err := stripTOMLTables(existing, "mcp_servers")
	if err != nil {
		return nil, err
	}
	fragment, err := toml.Marshal(map[string]any{"mcp_servers": servers})
	if err != nil {
		return nil, fmt.Errorf("encode codex mcp_servers: %w", err)
	}
	out := appendNewline(rest)
	out = append(out, fragment...)
	if err := toml.Unmarshal(out, &root); err != nil {
		return nil, fmt.Errorf("validate rendered codex toml: %w", err)
	}
	return appendNewline(out), nil
}

// Connection fields are managed; all other same-name options stay runtime-local.
// Unlike Pi's option allowlist, this retains unknown future Codex safety options.
func preserveCodexServerOptions(root, servers map[string]any) error {
	raw, exists := root["mcp_servers"]
	old, ok := raw.(map[string]any)
	if exists && !ok {
		return fmt.Errorf("codex mcp_servers must be an object")
	}
	for name, value := range servers {
		entry, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("codex MCP server %q must be an object", name)
		}
		previous, _ := old[name].(map[string]any)
		for key, option := range previous {
			switch key {
			case "command", "args", "env", "cwd", "url", "http_headers":
				continue
			}
			entry[key] = option
		}
	}
	return nil
}

// The pinned parser supplies decoded key components and complete value ranges,
// so quoted dots, comments, multiline strings and arrays are not line-scanned.
// Only owned statements are removed; standalone and trailing comments survive.
func stripTOMLTables(data []byte, prefix string) ([]byte, error) {
	var parser unstable.Parser
	parser.Reset(data)
	var out bytes.Buffer
	copied := 0
	rootScope, managed := true, false
	for parser.NextExpression() {
		node := parser.Expression()
		keys := node.Key()
		if !keys.Next() {
			return nil, fmt.Errorf("TOML expression has no key")
		}
		first := keys.Node()
		start, end := int(node.Raw.Offset), int(node.Raw.Offset+node.Raw.Length)
		remove := managed || rootScope && string(first.Data) == prefix
		if node.Kind == unstable.Table || node.Kind == unstable.ArrayTable {
			rootScope, managed = false, string(first.Data) == prefix
			remove = managed
			start, end = tomlHeaderRange(data, node)
		}
		if remove {
			start, end = tomlStatementRange(data, start, end)
			out.Write(data[copied:start])
			copied = end
		}
	}
	if err := parser.Error(); err != nil {
		return nil, fmt.Errorf("parse codex toml boundaries: %w", err)
	}
	out.Write(data[copied:])
	return out.Bytes(), nil
}

func tomlHeaderRange(data []byte, node *unstable.Node) (int, int) {
	keys := node.Key()
	keys.Next()
	first := keys.Node()
	last := first
	for keys.Next() {
		last = keys.Node()
	}
	start := bytes.LastIndexByte(data[:int(first.Raw.Offset)], '\n') + 1
	end := int(last.Raw.Offset + last.Raw.Length)
	end += bytes.IndexByte(data[end:], ']') + 1
	if node.Kind == unstable.ArrayTable {
		end++
	}
	return start, end
}

func tomlStatementRange(data []byte, start, end int) (int, int) {
	start = bytes.LastIndexByte(data[:start], '\n') + 1
	lineEnd := len(data)
	if next := bytes.IndexByte(data[end:], '\n'); next >= 0 {
		lineEnd = end + next + 1
	}
	if len(bytes.TrimSpace(data[end:lineEnd])) == 0 {
		end = lineEnd
	}
	return start, end
}
