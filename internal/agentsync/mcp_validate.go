package agentsync

import (
	"fmt"
	"sort"
	"strings"
)

// Canonical input is authoritative, unlike best-effort runtime extraction. A
// partially parsed set must never turn an invalid entry into a server deletion.
func validatedCanonicalServers(raw map[string]any) ([]mcpServer, error) {
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]mcpServer, 0, len(names))
	for _, name := range names {
		entry, ok := raw[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("parse mcp source: server %q must be an object", name)
		}
		if err := validateCanonicalFields(entry); err != nil {
			return nil, fmt.Errorf("parse mcp source: server %q: %w", name, err)
		}
		server := serverFromEntry(name, entry, "cursor")
		if err := validateMCPConnection(server); err != nil {
			return nil, fmt.Errorf("parse mcp source: server %q: %w", name, err)
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func validateCanonicalFields(entry map[string]any) error {
	for _, key := range []string{"type", "transport", "command", "cmd", "cwd", "url", "serverUrl", "httpUrl", "uri"} {
		if value, exists := entry[key]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s must be a string", key)
			}
		}
	}
	if value, exists := entry["args"]; exists {
		args, ok := value.([]any)
		if !ok {
			return fmt.Errorf("args must be an array of strings")
		}
		for _, arg := range args {
			if _, ok := arg.(string); !ok {
				return fmt.Errorf("args must be an array of strings")
			}
		}
	}
	for _, key := range []string{"env", "environment", "envs", "headers", "http_headers"} {
		if value, exists := entry[key]; exists {
			if err := validateMCPStringMap(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMCPStringMap(key string, value any) error {
	values, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object of strings", key)
	}
	for _, item := range values {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%s must be an object of strings", key)
		}
	}
	return nil
}

func validateMCPConnection(server mcpServer) error {
	if strings.TrimSpace(server.Name) == "" {
		return fmt.Errorf("server name must not be empty")
	}
	command, url := strings.TrimSpace(server.Command), strings.TrimSpace(server.URL)
	if command != "" && url != "" {
		return fmt.Errorf("specify either command or url, not both")
	}
	switch server.inferredType() {
	case "stdio":
		if command == "" {
			return fmt.Errorf("stdio connection requires a nonempty command")
		}
	case "http", "sse":
		if url == "" {
			return fmt.Errorf("remote connection requires a nonempty url")
		}
	default:
		return fmt.Errorf("unsupported transport %q", server.Type)
	}
	return nil
}
