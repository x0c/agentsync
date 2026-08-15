package agentsync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	mcpNoticeBegin = "<!-- agentsync:begin mcp -->"
	mcpNoticeEnd   = "<!-- agentsync:end mcp -->"
)

func syncMCP(cfg Config, opts Options) ([]TargetResult, []string, error) {
	if cfg.MCPSource == "" || len(cfg.MCPTargets) == 0 {
		return nil, nil, nil
	}

	var results []TargetResult
	var backups []string

	servers, sourceResult, err := ensureMCPSource(cfg, opts)
	if err != nil {
		return results, backups, err
	}
	if sourceResult != nil {
		results = append(results, *sourceResult)
	}
	if opts.Check && !pathExists(cfg.MCPSource) {
		for _, target := range cfg.MCPTargets {
			result, _, err := syncMCPTarget(target, nil, opts)
			if err != nil {
				return results, backups, err
			}
			results = append(results, result)
		}
		return results, backups, nil
	}

	ignoreResult, err := ensureMCPGitignore(filepath.Dir(cfg.MCPSource), opts)
	if err != nil {
		return results, backups, err
	}
	if ignoreResult != nil {
		results = append(results, *ignoreResult)
	}

	for _, target := range cfg.MCPTargets {
		result, backup, err := syncMCPTarget(target, servers, opts)
		if err != nil {
			return results, backups, err
		}
		results = append(results, result)
		if backup != "" {
			backups = append(backups, backup)
		}
	}
	return results, backups, nil
}

func ensureMCPSource(cfg Config, opts Options) ([]mcpServer, *TargetResult, error) {
	if pathExists(cfg.MCPSource) {
		data, err := os.ReadFile(cfg.MCPSource)
		if err != nil {
			return nil, nil, err
		}
		servers, err := parseCanonical(data)
		if err != nil {
			return nil, nil, err
		}
		return servers, nil, nil
	}

	imported := importMCPUnion(cfg.MCPTargets)
	result := TargetResult{Path: cfg.MCPSource, Status: "created", Detail: "canonical mcp source"}
	if opts.Check {
		result.Status = "missing"
		if len(imported) > 0 {
			result.Detail = "would create canonical mcp source from existing target files"
		} else {
			result.Detail = "would create canonical mcp source"
		}
		return imported, &result, nil
	}
	data, err := marshalCanonical(imported)
	if err != nil {
		return nil, nil, err
	}
	if err := writeFileAtomic(cfg.MCPSource, appendNewline(data), 0o644); err != nil {
		return nil, nil, err
	}
	if len(imported) > 0 {
		result.Detail = "canonical mcp source created from existing target files"
	}
	return imported, &result, nil
}

func importMCPUnion(targets []MCPTarget) []mcpServer {
	seen := map[string]mcpServer{}
	names := make([]string, 0)
	for _, target := range targets {
		target = resolveMCPTarget(target)
		if target.Detect != "" && !pathExists(target.Detect) {
			continue
		}
		servers, err := loadTargetServers(target)
		if err != nil {
			continue
		}
		for _, srv := range servers {
			if srv.Name == "" {
				continue
			}
			if _, ok := seen[srv.Name]; ok {
				continue
			}
			seen[srv.Name] = srv
			names = append(names, srv.Name)
		}
	}
	out := make([]mcpServer, 0, len(names))
	for _, name := range names {
		out = append(out, seen[name])
	}
	return out
}

func syncMCPTarget(target MCPTarget, servers []mcpServer, opts Options) (TargetResult, string, error) {
	target = resolveMCPTarget(target)
	result := TargetResult{Path: target.Path}
	if target.Detect != "" && !pathExists(target.Detect) {
		result.Status = "skipped"
		result.Detail = "runtime not installed"
		return result, "", nil
	}
	if servers == nil && opts.Check {
		if pathExists(target.Path) {
			result.Status = "replaceable"
			result.Detail = "would sync mcp config after creating canonical source"
			return result, "", nil
		}
		result.Status = "missing"
		result.Detail = "would create mcp config"
		return result, "", nil
	}

	existing, existingMode, err := readExistingMCP(target.Path)
	if err != nil {
		return result, "", err
	}
	next, err := applyMCPToFile(target, existing, servers)
	if err != nil {
		if isCrushRejected(err) {
			result.Status = "blocked"
			result.Detail = err.Error()
			return result, "", nil
		}
		if target.Mode == "key" && len(existing) > 0 {
			return result, "", fmt.Errorf("apply mcp to %s: %w", target.Path, err)
		}
		return result, "", fmt.Errorf("apply mcp to %s: %w", target.Path, err)
	}

	existingServers, extractErr := extractMCPServers(target.Dialect, target.Format, existing)
	if extractErr != nil && target.Mode == "key" && len(existing) > 0 {
		return result, "", fmt.Errorf("parse existing mcp config %s: %w", target.Path, extractErr)
	}
	nextServers, err := extractMCPServers(target.Dialect, target.Format, next)
	if err != nil {
		return result, "", fmt.Errorf("parse rendered mcp config %s: %w", target.Path, err)
	}

	jsoncOverlay, jsoncPath, err := openCodeJSONCOverlay(target)
	if err != nil {
		return result, "", err
	}
	matched := extractErr == nil && sameMCPServers(existingServers, nextServers) && !jsoncOverlay
	if matched && pathExists(target.Path) {
		result.Status = "ok"
		result.Detail = "mcp config"
		return result, "", nil
	}

	if opts.Check {
		if pathExists(target.Path) {
			result.Status = "replaceable"
			result.Detail = "would update mcp config"
			if jsoncOverlay {
				result.Detail = "would update mcp config and clear opencode.jsonc mcp overlay"
			}
			return result, "", nil
		}
		result.Status = "missing"
		result.Detail = "would create mcp config"
		return result, "", nil
	}

	var backup string
	if pathExists(target.Path) {
		backup, err = backupFile(target.Path)
		if err != nil {
			return result, "", err
		}
		result.Status = "replaced"
		result.Detail = "mcp config"
	} else {
		result.Status = "created"
		result.Detail = "mcp config"
	}
	perm := os.FileMode(0o644)
	if existingMode != 0 {
		perm = existingMode
	}
	if err := writeFileAtomic(target.Path, next, perm); err != nil {
		return result, backup, err
	}
	if jsoncOverlay {
		jsoncBackup, err := clearOpenCodeJSONCMCP(jsoncPath, opts)
		if err != nil {
			return result, backup, err
		}
		if jsoncBackup != "" {
			if backup == "" {
				backup = jsoncBackup
			}
		}
		result.Detail = "mcp config; cleared opencode.jsonc mcp overlay"
	}
	return result, backup, nil
}

func isCrushRejected(err error) bool {
	return err != nil && strings.Contains(err.Error(), "crush rejected")
}

func readExistingMCP(path string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	if info.IsDir() {
		return nil, 0, fmt.Errorf("mcp target is a directory: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	return data, info.Mode().Perm(), nil
}

func loadTargetServers(target MCPTarget) ([]mcpServer, error) {
	if target.Name == "opencode" {
		return loadOpenCodeServers(filepath.Dir(target.Path))
	}
	data, _, err := readExistingMCP(target.Path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []mcpServer{}, nil
	}
	return extractMCPServers(target.Dialect, target.Format, data)
}

func loadOpenCodeServers(dir string) ([]mcpServer, error) {
	jsonPath := filepath.Join(dir, "opencode.json")
	jsoncPath := filepath.Join(dir, "opencode.jsonc")
	merged := map[string]mcpServer{}
	names := make([]string, 0)
	for _, item := range []struct {
		path   string
		format string
	}{
		{jsonPath, "json"},
		{jsoncPath, "jsonc"},
	} {
		data, _, err := readExistingMCP(item.path)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		servers, err := extractMCPServers("opencode", item.format, data)
		if err != nil {
			return nil, err
		}
		for _, srv := range servers {
			if _, ok := merged[srv.Name]; !ok {
				names = append(names, srv.Name)
			}
			merged[srv.Name] = srv
		}
	}
	out := make([]mcpServer, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out, nil
}

func resolveMCPTarget(target MCPTarget) MCPTarget {
	switch target.Name {
	case "claude":
		if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
			target.Path = filepath.Join(dir, ".claude.json")
			target.Detect = dir
		}
	case "kimi-code":
		if dir := os.Getenv("KIMI_CODE_HOME"); dir != "" {
			target.Path = filepath.Join(dir, "mcp.json")
			target.Detect = dir
		}
	case "qoder":
		if dir := os.Getenv("QODER_CONFIG_DIR"); dir != "" {
			target.Path = filepath.Join(dir, "settings.json")
			target.Detect = dir
		}
	case "opencode":
		target = resolveOpenCodeTarget(target)
	case "kilo":
		target = resolveKiloTarget(target)
	case "codebuddy":
		target = resolveCodeBuddyTarget(target)
	}
	return target
}

func resolveOpenCodeTarget(target MCPTarget) MCPTarget {
	dir := target.Detect
	if dir == "" {
		dir = filepath.Dir(target.Path)
	}
	jsonPath := filepath.Join(dir, "opencode.json")
	jsoncPath := filepath.Join(dir, "opencode.jsonc")
	switch {
	case pathExists(jsonPath):
		target.Path = jsonPath
		target.Format = "json"
	case pathExists(jsoncPath):
		target.Path = jsoncPath
		target.Format = "jsonc"
	default:
		target.Path = jsonPath
		target.Format = "json"
	}
	return target
}

func resolveKiloTarget(target MCPTarget) MCPTarget {
	dir := target.Detect
	if dir == "" {
		dir = filepath.Dir(target.Path)
	}
	jsoncPath := filepath.Join(dir, "kilo.jsonc")
	jsonPath := filepath.Join(dir, "kilo.json")
	switch {
	case pathExists(jsoncPath):
		target.Path = jsoncPath
		target.Format = "jsonc"
	case pathExists(jsonPath):
		target.Path = jsonPath
		target.Format = "json"
	default:
		target.Path = jsoncPath
		target.Format = "jsonc"
	}
	return target
}

func resolveCodeBuddyTarget(target MCPTarget) MCPTarget {
	home := target.Detect
	if home == "" {
		home = filepath.Dir(target.Path)
	}
	dot := filepath.Join(home, ".mcp.json")
	plain := filepath.Join(home, "mcp.json")
	legacy := filepath.Join(filepath.Dir(home), ".codebuddy.json")
	switch {
	case pathExists(dot):
		target.Path = dot
		target.Mode = "file"
	case pathExists(plain):
		target.Path = plain
		target.Mode = "file"
	case pathExists(legacy):
		target.Path = legacy
		target.Mode = "key"
	default:
		target.Path = dot
		target.Mode = "file"
	}
	return target
}

func openCodeJSONCOverlay(target MCPTarget) (bool, string, error) {
	if target.Name != "opencode" || !strings.HasSuffix(target.Path, "opencode.json") {
		return false, "", nil
	}
	jsoncPath := filepath.Join(filepath.Dir(target.Path), "opencode.jsonc")
	data, _, err := readExistingMCP(jsoncPath)
	if err != nil {
		return false, jsoncPath, err
	}
	if len(data) == 0 {
		return false, jsoncPath, nil
	}
	servers, err := extractMCPServers("opencode", "jsonc", data)
	if err != nil {
		return false, jsoncPath, err
	}
	return len(servers) > 0, jsoncPath, nil
}

func clearOpenCodeJSONCMCP(path string, opts Options) (string, error) {
	if opts.Check || !pathExists(path) {
		return "", nil
	}
	existing, perm, err := readExistingMCP(path)
	if err != nil {
		return "", err
	}
	empty, err := jsonMarshalObject(map[string]any{})
	if err != nil {
		return "", err
	}
	next, err := setTopLevelJSONKey(stripJSONC(existing), "mcp", empty)
	if err != nil {
		return "", err
	}
	if len(next) > 0 && next[len(next)-1] != '\n' {
		next = append(next, '\n')
	}
	backup, err := backupFile(path)
	if err != nil {
		return "", err
	}
	if perm == 0 {
		perm = 0o644
	}
	if err := writeFileAtomic(path, next, perm); err != nil {
		return backup, err
	}
	return backup, nil
}

func jsonMarshalObject(v any) ([]byte, error) {
	return json.Marshal(v)
}

func appendNewline(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return append(data, '\n')
	}
	return data
}

func mcpNoticeText() string {
	return mcpNoticeBegin + "\n" +
		"## MCP 配置\n\n" +
		"MCP 服务器的唯一源是 `~/.config/agentsync/mcp.json`（与本文件、`skills/` 平级）。" +
		"新增、修改或删除 MCP 时只改那份 JSON，不要改各工具自己的 MCP 配置" +
		"（例如 `~/.claude.json`、`~/.cursor/mcp.json`、`~/.codex/config.toml`）。" +
		"运行 `agentsync` 后，已安装工具的用户级 MCP 入口会按各自 schema 被覆盖为统一源中的服务器集合。\n" +
		mcpNoticeEnd
}

func syncMCPNotice(source string, opts Options) (*TargetResult, error) {
	if source == "" || !pathExists(source) {
		return nil, nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	next, changed := upsertMCPNotice(string(data))
	if !changed {
		return nil, nil
	}
	result := TargetResult{Path: source, Status: "replaced", Detail: "injected mcp notice"}
	if opts.Check {
		result.Status = "replaceable"
		result.Detail = "would inject mcp notice"
		return &result, nil
	}
	info, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(source, []byte(next), info.Mode().Perm()); err != nil {
		return nil, err
	}
	return &result, nil
}

func upsertMCPNotice(body string) (string, bool) {
	notice := mcpNoticeText()
	begin := strings.Index(body, mcpNoticeBegin)
	end := strings.Index(body, mcpNoticeEnd)
	if begin >= 0 && end > begin {
		end += len(mcpNoticeEnd)
		current := body[begin:end]
		if current == notice {
			return body, false
		}
		return body[:begin] + notice + body[end:], true
	}
	trimmed := strings.TrimRight(body, "\n")
	if trimmed == "" {
		return notice + "\n", true
	}
	return trimmed + "\n\n" + notice + "\n", true
}

func ensureMCPGitignore(root string, opts Options) (*TargetResult, error) {
	if root == "" || !pathExists(filepath.Join(root, ".git")) {
		return nil, nil
	}
	path := filepath.Join(root, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if gitignoreHasMCP(existing) {
		return nil, nil
	}
	result := TargetResult{Path: path, Status: "created", Detail: "ignore mcp.json"}
	if opts.Check {
		result.Status = "missing"
		result.Detail = "would ignore mcp.json"
		return &result, nil
	}
	next := existing
	if next != "" && !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	next += "mcp.json\n"
	perm := os.FileMode(0o644)
	if info, err := os.Lstat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := writeFileAtomic(path, []byte(next), perm); err != nil {
		return nil, err
	}
	return &result, nil
}

func gitignoreHasMCP(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		switch trim {
		case "mcp.json", "/mcp.json", "./mcp.json":
			return true
		}
	}
	return false
}
