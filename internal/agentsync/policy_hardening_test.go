package agentsync

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPolicyNormalizesTargetNamesInBothSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	writeMCPFixture(t, path, `{"mcp":{"targets":{"ClAuDe":{"deny":["Secret"]}}},"skills":{"targets":{"CoDeX":{"allow":["Approved"]}}}}`)
	policy, err := loadSyncPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := policy.MCP.Targets["claude"]; !exists || len(policy.MCP.Targets) != 1 {
		t.Fatalf("MCP target not normalized: %#v", policy.MCP.Targets)
	}
	if _, exists := policy.Skills.Targets["codex"]; !exists || len(policy.Skills.Targets) != 1 {
		t.Fatalf("skill target not normalized: %#v", policy.Skills.Targets)
	}
	if nameAllowed("secret", policy.MCP, "CLAUDE") || !nameAllowed("approved", policy.Skills, "CODEX") || nameAllowed("other", policy.Skills, "codex") {
		t.Fatal("normalization changed allow/deny semantics")
	}
}

func TestCaseEquivalentPolicyTargetsFailBeforeMCPWrites(t *testing.T) {
	for _, section := range []string{"mcp", "skills"} {
		for _, targets := range []string{
			`"claude":{},"CLAUDE":{"deny":["secret"]}`,
			`"ClAuDe":{"deny":["secret"]},"CLAUDE":{}`,
		} {
			for _, check := range []bool{true, false} {
				cfg := hardeningMCPConfig(t, "cursor", "json", "file", `{"mcpServers":{}}`, `{"mcpServers":{"secret":{"command":"secret"}}}`)
				cfg.MCPTargets[0].Name = "claude"
				cfg.PolicyPath = filepath.Join(filepath.Dir(cfg.MCPSource), "policy.json")
				writeMCPFixture(t, cfg.PolicyPath, fmt.Sprintf(`{"%s":{"targets":{%s}}}`, section, targets))
				_, _, err := syncMCP(cfg, Options{Check: check})
				if err == nil || !strings.Contains(err.Error(), "duplicate case-insensitive target") || !strings.Contains(err.Error(), section) {
					t.Fatalf("%s duplicate accepted or wrong error: %v", section, err)
				}
				if string(readMCPFixture(t, cfg.MCPTargets[0].Path)) != `{"mcpServers":{}}` {
					t.Fatal("ambiguous policy changed target")
				}
			}
		}
	}
}
