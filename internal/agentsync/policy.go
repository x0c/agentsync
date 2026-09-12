package agentsync

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// SyncPolicy is the on-disk shape of ~/.config/agentsync/sync-policy.json.
// Missing file means allow-all (legacy behavior).
type SyncPolicy struct {
	Version int            `json:"version"`
	MCP     ResourcePolicy `json:"mcp"`
	Skills  ResourcePolicy `json:"skills"`
}

// ResourcePolicy controls which MCP server / skill names are written to a target.
type ResourcePolicy struct {
	Default string                  `json:"default"` // "allow" (default) or "deny"
	Targets map[string]TargetPolicy `json:"targets"`
}

// TargetPolicy is per-runtime allow/deny. Non-empty Allow switches that target to allowlist mode.
type TargetPolicy struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

func loadSyncPolicy(path string) (SyncPolicy, error) {
	if path == "" {
		return SyncPolicy{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SyncPolicy{}, nil
		}
		return SyncPolicy{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return SyncPolicy{}, fmt.Errorf("parse sync policy %s: empty file", path)
	}
	var pol SyncPolicy
	if err := json.Unmarshal(data, &pol); err != nil {
		return SyncPolicy{}, fmt.Errorf("parse sync policy %s: %w", path, err)
	}
	if err := validateResourcePolicy("mcp", pol.MCP); err != nil {
		return SyncPolicy{}, err
	}
	if err := validateResourcePolicy("skills", pol.Skills); err != nil {
		return SyncPolicy{}, err
	}
	return pol, nil
}

func validateResourcePolicy(kind string, pol ResourcePolicy) error {
	switch strings.ToLower(strings.TrimSpace(pol.Default)) {
	case "", "allow", "deny":
	default:
		return fmt.Errorf("sync policy %s.default must be allow or deny, got %q", kind, pol.Default)
	}
	return nil
}

func (p ResourcePolicy) defaultAllow() bool {
	switch strings.ToLower(strings.TrimSpace(p.Default)) {
	case "deny":
		return false
	default:
		return true
	}
}

func (p ResourcePolicy) targetPolicy(target string) (TargetPolicy, bool) {
	if len(p.Targets) == 0 || target == "" {
		return TargetPolicy{}, false
	}
	if tp, ok := p.Targets[target]; ok {
		return tp, true
	}
	lower := strings.ToLower(target)
	for name, tp := range p.Targets {
		if strings.ToLower(name) == lower {
			return tp, true
		}
	}
	return TargetPolicy{}, false
}

func nameInList(name string, list []string) bool {
	lower := strings.ToLower(name)
	for _, item := range list {
		if strings.ToLower(item) == lower {
			return true
		}
	}
	return false
}

// nameAllowed applies deny-first, then optional allowlist, then resource default.
func nameAllowed(name string, pol ResourcePolicy, target string) bool {
	tp, _ := pol.targetPolicy(target)
	if nameInList(name, tp.Deny) {
		return false
	}
	if len(tp.Allow) > 0 {
		return nameInList(name, tp.Allow)
	}
	return pol.defaultAllow()
}

func filterNamesForPolicy(names []string, pol ResourcePolicy, target string) (kept, dropped []string) {
	kept = make([]string, 0, len(names))
	dropped = make([]string, 0)
	for _, name := range names {
		if nameAllowed(name, pol, target) {
			kept = append(kept, name)
		} else {
			dropped = append(dropped, name)
		}
	}
	return kept, dropped
}

func filterServersForPolicy(servers []mcpServer, pol ResourcePolicy, target string) (kept []mcpServer, dropped []string) {
	kept = make([]mcpServer, 0, len(servers))
	dropped = make([]string, 0)
	for _, srv := range servers {
		if nameAllowed(srv.Name, pol, target) {
			kept = append(kept, srv)
		} else {
			dropped = append(dropped, srv.Name)
		}
	}
	return kept, dropped
}

// policyFiltersTarget is true when the kept set differs from the full name set.
func policyFiltersTarget(names []string, pol ResourcePolicy, target string) bool {
	_, dropped := filterNamesForPolicy(names, pol, target)
	return len(dropped) > 0
}

func unknownPolicyTargets(pol ResourcePolicy, known []string) []string {
	if len(pol.Targets) == 0 {
		return nil
	}
	knownLower := map[string]struct{}{}
	for _, name := range known {
		knownLower[strings.ToLower(name)] = struct{}{}
	}
	var unknown []string
	for name := range pol.Targets {
		if _, ok := knownLower[strings.ToLower(name)]; !ok {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	return unknown
}

func knownMCPTargetNames(targets []MCPTarget) []string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		if t.Name != "" {
			names = append(names, t.Name)
		}
	}
	return names
}

func knownSkillTargetNames(targets []SkillTarget) []string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		if t.Name != "" {
			names = append(names, t.Name)
		}
	}
	return names
}

func formatFilteredDetail(base string, dropped []string) string {
	if len(dropped) == 0 {
		return base
	}
	sort.Strings(dropped)
	msg := "filtered out: " + strings.Join(dropped, ", ")
	if base == "" {
		return msg
	}
	return base + "; " + msg
}

func policyWarningResults(path string, pol SyncPolicy, mcpTargets []MCPTarget, skillTargets []SkillTarget) []TargetResult {
	if path == "" {
		return nil
	}
	var results []TargetResult
	if mcpTargets != nil {
		if unknown := unknownPolicyTargets(pol.MCP, knownMCPTargetNames(mcpTargets)); len(unknown) > 0 {
			results = append(results, TargetResult{
				Path:   path,
				Status: "warning",
				Detail: "unknown mcp policy targets: " + strings.Join(unknown, ", "),
			})
		}
	}
	if skillTargets != nil {
		if unknown := unknownPolicyTargets(pol.Skills, knownSkillTargetNames(skillTargets)); len(unknown) > 0 {
			results = append(results, TargetResult{
				Path:   path,
				Status: "warning",
				Detail: "unknown skills policy targets: " + strings.Join(unknown, ", "),
			})
		}
	}
	return results
}
