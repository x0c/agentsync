package agentsync

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// dsh（DeepSeek Harness）的 MCP 没有独立 mcp.json：一个 server 就是
// host 层 cordis.patch.yml 里一个 @deepseek-ai/dsh-mcp-client 插件实例。
// agentsync 只写自己 marker 包裹的块，块外字节级不动（patch 文件里常见
// !!js 自定义 tag，全文件 YAML 回写极易弄坏用户内容）。

const (
	dshMCPClientName = "@deepseek-ai/dsh-mcp-client"
	dshBlockBegin    = "# managed-by: agentsync start"
	dshBlockEnd      = "# managed-by: agentsync end"
)

var dshServerNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// serverName 行扫描只看字面标量（schema 要求 serverName 是普通名字），
// 不做 YAML 解析，因此块外的 !!js 等写法不会影响我们。
var dshServerNameLineRe = regexp.MustCompile(`(?m)^\s*serverName:\s*(\S+)\s*$`)

func dshHomeDir() string {
	if dir := os.Getenv("DSH_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".dsh")
	}
	return filepath.Join(home, ".dsh")
}

func dshPatchPath() string {
	return filepath.Join(dshHomeDir(), "cordis.patch.yml")
}

func renderDshPatchBlock(servers []mcpServer) ([]byte, error) {
	sorted := append([]mcpServer{}, servers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	entries := make([]any, 0, len(sorted))
	for _, srv := range sorted {
		entry, err := dshPatchEntry(srv)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	doc := []any{map[string]any{"insert": entries}}
	body, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("dsh patch block: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString(dshBlockBegin + "\n")
	buf.WriteString("# 以下 dsh MCP 条目由统一源 mcp.json 生成，块内每次同步都会覆盖；手写条目请写到块外。\n")
	buf.Write(body)
	if !bytes.HasSuffix(body, []byte("\n")) {
		buf.WriteString("\n")
	}
	buf.WriteString(dshBlockEnd + "\n")
	return buf.Bytes(), nil
}

func dshPatchEntry(srv mcpServer) (map[string]any, error) {
	if !dshServerNameRe.MatchString(srv.Name) {
		return nil, fmt.Errorf("dsh rejected %q: serverName 须匹配 [A-Za-z0-9_-]{1,32}（dsh-mcp-client 命名契约），请改名后再同步", srv.Name)
	}
	config := map[string]any{"serverName": srv.Name}
	if srv.isRemote() {
		if srv.inferredType() == "sse" {
			return nil, fmt.Errorf("dsh rejected %q: dsh-mcp-client 只支持 stdio / streamable-http，不支持 sse", srv.Name)
		}
		config["transport"] = "streamable-http"
		if srv.URL != "" {
			config["url"] = srv.URL
		}
		if len(srv.Headers) > 0 {
			config["headers"] = srv.Headers
		}
	} else {
		config["transport"] = "stdio"
		if srv.Command != "" {
			config["command"] = srv.Command
		}
		if len(srv.Args) > 0 {
			config["args"] = srv.Args
		}
		if len(srv.Env) > 0 {
			config["env"] = srv.Env
		}
		if srv.Cwd != "" {
			config["cwd"] = srv.Cwd
		}
	}
	return map[string]any{
		"id":     "mcp-" + srv.Name,
		"name":   dshMCPClientName,
		"config": config,
	}, nil
}

// dshManagedSpan 返回现有 managed 块的起止（行首到行尾，含换行）。
func dshManagedSpan(data []byte) (start, end int, ok bool) {
	lines := strings.SplitAfter(string(data), "\n")
	off := 0
	found := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == dshBlockBegin {
			found = i
			start = off
			break
		}
		off += len(line)
	}
	if found < 0 {
		return 0, 0, false
	}
	off = start
	for i := found; i < len(lines); i++ {
		off += len(lines[i])
		if strings.TrimSpace(lines[i]) == dshBlockEnd {
			return start, off, true
		}
	}
	return 0, 0, false
}

func spliceDshPatchBlock(existing, block []byte) []byte {
	if start, end, ok := dshManagedSpan(existing); ok {
		out := make([]byte, 0, len(existing)-(end-start)+len(block))
		out = append(out, existing[:start]...)
		out = append(out, block...)
		out = append(out, existing[end:]...)
		return out
	}
	if dshPatchIsEmpty(existing) {
		comments := dshLeadingComments(existing)
		out := make([]byte, 0, len(comments)+len(block))
		out = append(out, comments...)
		if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		out = append(out, block...)
		return out
	}
	out := make([]byte, 0, len(existing)+len(block)+1)
	out = append(out, bytes.TrimRight(existing, "\n")...)
	out = append(out, '\n')
	out = append(out, block...)
	return out
}

// dshPatchIsEmpty 判定 patch 文件是否等价于空（只有注释/空白/`[]`）。
func dshPatchIsEmpty(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || trim == "[]" {
			continue
		}
		return false
	}
	return true
}

func dshLeadingComments(data []byte) []byte {
	var out []byte
	for _, line := range strings.SplitAfter(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			out = append(out, line...)
			continue
		}
		break
	}
	return out
}

// extractDshServers 只解析我们自己 managed 块内的条目（我们生成的内容，
// 无 !!js），块外手写条目不参与并集导入，避免解析用户自定义 tag 失败。
func extractDshServers(data []byte) ([]mcpServer, error) {
	start, end, ok := dshManagedSpan(data)
	if !ok {
		return []mcpServer{}, nil
	}
	body := string(data[start:end])
	body = strings.ReplaceAll(body, dshBlockBegin, "")
	body = strings.ReplaceAll(body, dshBlockEnd, "")
	var doc []map[string]any
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return nil, err
	}
	var out []mcpServer
	for _, item := range doc {
		raw, _ := item["insert"].([]any)
		for _, e := range raw {
			entry, _ := e.(map[string]any)
			if entry == nil || fmt.Sprint(entry["name"]) != dshMCPClientName {
				continue
			}
			cfg, _ := entry["config"].(map[string]any)
			if cfg == nil {
				continue
			}
			srv := mcpServer{
				Name:    fmt.Sprint(cfg["serverName"]),
				Type:    fmt.Sprint(cfg["transport"]),
				Command: fmt.Sprint(orEmpty(cfg["command"])),
				URL:     fmt.Sprint(orEmpty(cfg["url"])),
				Cwd:     fmt.Sprint(orEmpty(cfg["cwd"])),
			}
			if srv.Type == "streamable-http" || srv.Type == "streamable_http" {
				srv.Type = "http"
			}
			if args, ok := cfg["args"].([]any); ok {
				for _, a := range args {
					srv.Args = append(srv.Args, fmt.Sprint(a))
				}
			}
			srv.Env = stringMapField(cfg, "env")
			srv.Headers = stringMapField(cfg, "headers")
			if srv.Name == "" || srv.Name == "<nil>" {
				continue
			}
			out = append(out, srv)
		}
	}
	if out == nil {
		return []mcpServer{}, nil
	}
	return out, nil
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// dshOutsideServerNames 扫描 managed 块之外手写的同名 server（行扫描，
// 不解析 YAML，因此块外 !!js 不影响我们）。
func dshOutsideServerNames(data []byte) []string {
	outside := string(data)
	if start, end, ok := dshManagedSpan(data); ok {
		outside = string(data[:start]) + string(data[end:])
	}
	seen := map[string]bool{}
	var names []string
	for _, m := range dshServerNameLineRe.FindAllStringSubmatch(outside, -1) {
		name := strings.Trim(m[1], `"'`)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// dshClientDepProfiles 返回 profiles 下声明了 dsh-mcp-client 依赖的 profile 名。
func dshClientDepProfiles() []string {
	entries, err := os.ReadDir(filepath.Join(dshHomeDir(), "profiles"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifest, err := os.ReadFile(filepath.Join(dshHomeDir(), "profiles", e.Name(), "package.json"))
		if err != nil {
			continue
		}
		var pkg struct {
			Dependencies map[string]string `json:"dependencies"`
		}
		if err := yaml.Unmarshal(manifest, &pkg); err != nil {
			continue
		}
		if _, ok := pkg.Dependencies[dshMCPClientName]; ok {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func dshProfileNames() []string {
	entries, err := os.ReadDir(filepath.Join(dshHomeDir(), "profiles"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func syncDshMCPTarget(target MCPTarget, servers []mcpServer, opts Options, dropped []string) (TargetResult, string, error) {
	result := TargetResult{Path: target.Path}
	if target.Detect != "" && !pathExists(target.Detect) {
		result.Status = "skipped"
		result.Detail = "runtime not installed"
		return result, "", nil
	}
	detail := func(base string) string { return formatFilteredDetail(base, dropped) }

	// Codex 捆绑本机 server（node_repl / computer-use 等）写进 dsh 会启动失败，照例过滤。
	servers = filterServersForDialect(servers, "dsh")

	depProfiles := dshClientDepProfiles()
	if len(depProfiles) == 0 {
		profiles := dshProfileNames()
		hint := "dsh plugin --profile web add " + dshMCPClientName
		if len(profiles) > 0 && profiles[0] != "web" {
			hint = "dsh plugin --profile " + profiles[0] + " add " + dshMCPClientName
		}
		result.Status = "blocked"
		result.Detail = detail("dsh-mcp-client 未装进任何 profile 依赖，写 patch 会导致 dsh 无法启动；先跑：" + hint)
		return result, "", nil
	}

	block, err := renderDshPatchBlock(servers)
	if err != nil {
		result.Status = "blocked"
		result.Detail = detail(err.Error())
		return result, "", nil
	}

	existing, perm, err := readExistingMCP(target.Path)
	if err != nil {
		return result, "", err
	}
	next := spliceDshPatchBlock(existing, block)

	var warnings []string
	for _, name := range dshOutsideServerNames(next) {
		for _, srv := range servers {
			if srv.Name == name {
				warnings = append(warnings, name)
			}
		}
	}
	warnSuffix := ""
	if len(warnings) > 0 {
		warnSuffix = "；注意块外手写了同名 server（" + strings.Join(warnings, ", ") + "），同 scope 后者加载失败，必要时把手写条目并入统一源后删掉"
	}

	equal := false
	if s, e, ok := dshManagedSpan(existing); ok {
		equal = string(existing[s:e]) == string(block)
	}
	if equal && pathExists(target.Path) {
		result.Status = "ok"
		result.Detail = detail("dsh cordis patch（managed 块已一致）" + warnSuffix)
		return result, "", nil
	}

	if opts.Check {
		if pathExists(target.Path) {
			result.Status = "replaceable"
			result.Detail = detail("would update dsh cordis patch managed 块" + warnSuffix)
			return result, "", nil
		}
		result.Status = "missing"
		result.Detail = detail("would create dsh cordis patch" + warnSuffix)
		return result, "", nil
	}

	var backup string
	if pathExists(target.Path) {
		backup, err = backupFile(target.Path)
		if err != nil {
			return result, "", err
		}
		result.Status = "replaced"
	} else {
		result.Status = "created"
	}
	writePerm := os.FileMode(0o600)
	if perm != 0 {
		writePerm = perm
	}
	if err := writeFileAtomic(target.Path, next, writePerm); err != nil {
		return result, backup, err
	}
	result.Detail = detail("dsh cordis patch managed 块" + warnSuffix)

	if profile, ok := dshValidatePatch(); !ok {
		restoreErr := writeFileAtomic(target.Path, existing, writePerm)
		if restoreErr != nil {
			return result, backup, fmt.Errorf("dsh patch 写入后 dsh 自检失败，且回滚失败（备份在 %s）：%v", backup, restoreErr)
		}
		result.Status = "blocked"
		result.Detail = detail("dsh 自检（dump-config) 失败，已按备份回滚；把报错发来再修")
		return result, backup, nil
	} else if profile != "" {
		result.Detail = detail("dsh cordis patch managed 块（已用 " + profile + " profile 自检通过）" + warnSuffix)
	}
	return result, backup, nil
}

// dshValidatePatch 用 dsh 自带的 --dump-config 做写后自检（只读，不启动服务）。
// 返回值是实际校验的 profile 名；空串表示跳过校验（本机无 dsh 可执行文件或无 profile）。
func dshValidatePatch() (string, bool) {
	bin, err := exec.LookPath("dsh")
	if err != nil {
		return "", true
	}
	profiles := dshProfileNames()
	if len(profiles) == 0 {
		return "", true
	}
	profile := "web"
	found := false
	for _, p := range profiles {
		if p == "web" {
			found = true
			break
		}
	}
	if !found {
		profile = profiles[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--profile", profile, "--dump-config")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return profile, true
}
