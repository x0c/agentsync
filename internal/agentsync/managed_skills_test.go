package agentsync

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func setupIsolatedSkillTestEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{
		"HOME":                  home,
		"USERPROFILE":           home,
		"XDG_CONFIG_HOME":       filepath.Join(home, ".config"),
		"APPDATA":               filepath.Join(home, ".appdata"),
		"LOCALAPPDATA":          filepath.Join(home, ".localappdata"),
		"AGENTSYNC_CONFIG_HOME": filepath.Join(home, ".config", "agentsync"),
		"CLAUDE_CONFIG_DIR":     "",
		"PI_CODING_AGENT_DIR":   filepath.Join(home, ".pi", "agent"),
	} {
		t.Setenv(k, v)
	}
	return home
}

func writeTestSkillDir(t *testing.T, parent, name, body string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "extra.md"), []byte("ref body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestManagedSkillStandaloneInstallStatusRemoveAndDedupe(t *testing.T) {
	home := setupIsolatedSkillTestEnv(t)

	// Claude + Cursor are installed and resolve to separate roots;
	// OpenCode + ~/.agents symlink to the same physical directory so they deduplicate to one copy.
	claudeHome := filepath.Join(home, ".claude")
	cursorHome := filepath.Join(home, ".cursor")
	opencodeHome := filepath.Join(home, ".config", "opencode")
	agentsHome := filepath.Join(home, ".agents")
	sharedPhysical := filepath.Join(home, "shared-skills")
	for _, d := range []string{claudeHome, cursorHome, opencodeHome, agentsHome, sharedPhysical} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(sharedPhysical, filepath.Join(opencodeHome, "skills")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sharedPhysical, filepath.Join(agentsHome, "skills")); err != nil {
		t.Fatal(err)
	}

	srcRoot := filepath.Join(t.TempDir(), "plugin-skills")
	writeTestSkillDir(t, srcRoot, "corral-workflow", "---\nname: corral-workflow\ndescription: wf\n---\nbody\n")

	// --check preview leaves all files and directories absent.
	preview, err := RunManagedSkillInstall(ManagedSkillOptions{
		Owner:  "corral-plugin:workflow",
		Source: srcRoot,
		Check:  true,
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if !preview.OK || preview.Mode != "standalone" {
		t.Fatalf("unexpected preview report: %+v", preview)
	}
	if pathExists(filepath.Join(claudeHome, "skills")) {
		t.Fatal("--check created claude skills directory")
	}
	manifestPath, err := managedSkillsManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	if pathExists(manifestPath) {
		t.Fatal("--check wrote manifest")
	}

	// Real install writes one copy per distinct physical root and leaves absent runtimes untouched.
	res, err := RunManagedSkillInstall(ManagedSkillOptions{
		Owner:  "corral-plugin:workflow",
		Source: srcRoot,
	})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if !res.OK || len(res.Conflicts) != 0 {
		t.Fatalf("unexpected install conflicts: %+v", res)
	}
	// 3 distinct physical roots: claude, cursor, and (agents + opencode)
	if len(res.Actions) != 3 {
		t.Fatalf("got %d actions, want 3 deduplicated roots: %+v", len(res.Actions), res.Actions)
	}
	if pathExists(filepath.Join(home, ".codex")) {
		t.Fatal("install created directory for absent codex runtime")
	}
	for _, dst := range []string{
		filepath.Join(claudeHome, "skills", "corral-workflow", "SKILL.md"),
		filepath.Join(cursorHome, "skills", "corral-workflow", "SKILL.md"),
		filepath.Join(sharedPhysical, "corral-workflow", "SKILL.md"),
	} {
		if !isRegularFile(dst) {
			t.Fatalf("missing installed skill file: %s", dst)
		}
	}

	// Idempotent rerun reports unchanged.
	again, err := RunManagedSkillInstall(ManagedSkillOptions{
		Owner:  "corral-plugin:workflow",
		Source: srcRoot,
	})
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	for _, act := range again.Actions {
		if act.Action != "unchanged" || act.State != "current" {
			t.Fatalf("expected unchanged/current, got %+v", act)
		}
	}

	// Status reports current across all 3 roots.
	st, err := RunManagedSkillStatus(ManagedSkillOptions{Owner: "corral-plugin:workflow"})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !st.OK || len(st.Actions) != 3 {
		t.Fatalf("unexpected status: %+v", st)
	}

	// Remove deletes tracked files and empty dirs without deleting user files added beside them.
	userNote := filepath.Join(claudeHome, "skills", "corral-workflow", "user-note.md")
	if err := os.WriteFile(userNote, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rm, err := RunManagedSkillRemove(ManagedSkillOptions{Owner: "corral-plugin:workflow"})
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if !rm.OK || len(rm.Actions) != 3 {
		t.Fatalf("unexpected remove: %+v", rm)
	}
	if !isRegularFile(userNote) {
		t.Fatal("remove deleted non-manifest user file")
	}
	if pathExists(filepath.Join(cursorHome, "skills", "corral-workflow")) {
		t.Fatal("empty skill directory was not cleaned up")
	}
}

func TestManagedSkillCanonicalFilteredModeAndPreservesOtherFiles(t *testing.T) {
	home := setupIsolatedSkillTestEnv(t)

	// Prepare canonical agentsync skills + claude symlink + filtered codex directory.
	canonicalSkills := filepath.Join(home, ".config", "agentsync", "skills")
	baseSkill := writeTestSkillDir(t, canonicalSkills, "base-skill", "---\nname: base-skill\ndescription: base\n---\n")
	writeTestSkillDir(t, canonicalSkills, "dropped-skill", "---\nname: dropped-skill\ndescription: drop\n---\n")

	claudeHome := filepath.Join(home, ".claude")
	codexHome := filepath.Join(home, ".codex")
	codexSkills := filepath.Join(codexHome, "skills")
	for _, d := range []string{claudeHome, codexSkills} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(canonicalSkills, filepath.Join(claudeHome, "skills")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(baseSkill, filepath.Join(codexSkills, "base-skill")); err != nil {
		t.Fatal(err)
	}
	// Preserve canonical AGENTS.md and mcp.json untouched.
	agentsFile := filepath.Join(home, ".config", "agentsync", "AGENTS.md")
	if err := os.WriteFile(agentsFile, []byte("owner rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policyFile := filepath.Join(home, ".config", "agentsync", "sync-policy.json")
	if err := os.WriteFile(policyFile, []byte(`{"version":1,"skills":{"default":"allow","targets":{"codex":{"deny":["dropped-skill"]}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	srcRoot := filepath.Join(t.TempDir(), "plugin")
	writeTestSkillDir(t, srcRoot, "corral-workflow", "---\nname: corral-workflow\ndescription: wf\n---\nv1\n")

	inst, err := RunManagedSkillInstall(ManagedSkillOptions{
		Owner:  "corral-plugin:workflow",
		Source: srcRoot,
	})
	if err != nil {
		t.Fatalf("canonical install failed: %v", err)
	}
	if !inst.OK || inst.Mode != "canonical" || len(inst.Actions) != 1 {
		t.Fatalf("unexpected canonical install report: %+v", inst)
	}
	// Present in canonical root (visible to claude) AND symlinked into filtered codex dir.
	canonWF := filepath.Join(canonicalSkills, "corral-workflow")
	if !isRegularFile(filepath.Join(canonWF, "SKILL.md")) {
		t.Fatal("missing canonical workflow SKILL.md")
	}
	if !linkPointsTo(filepath.Join(codexSkills, "corral-workflow"), canonWF) {
		t.Fatal("missing per-skill symlink in filtered codex skills directory")
	}

	// Remove deletes both canonical skill and codex symlink while preserving base-skill & AGENTS.md.
	rm, err := RunManagedSkillRemove(ManagedSkillOptions{Owner: "corral-plugin:workflow"})
	if err != nil {
		t.Fatalf("canonical remove failed: %v", err)
	}
	if !rm.OK {
		t.Fatalf("unexpected canonical remove conflicts: %+v", rm)
	}
	if pathExists(canonWF) {
		t.Fatal("canonical workflow skill was not removed")
	}
	if pathExists(filepath.Join(codexSkills, "corral-workflow")) {
		t.Fatal("filtered codex symlink was not removed")
	}
	if !linkPointsTo(filepath.Join(codexSkills, "base-skill"), baseSkill) {
		t.Fatal("unrelated codex skill symlink altered")
	}
	gotAgents, err := os.ReadFile(agentsFile)
	if err != nil || string(gotAgents) != "owner rules\n" {
		t.Fatalf("AGENTS.md modified: %q, %v", gotAgents, err)
	}

	// When sync-policy denies the plugin skill for codex, no codex symlink is created.
	if err := os.WriteFile(policyFile, []byte(`{"version":1,"skills":{"default":"allow","targets":{"codex":{"deny":["corral-workflow","dropped-skill"]}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	inst2, err := RunManagedSkillInstall(ManagedSkillOptions{
		Owner:  "corral-plugin:workflow",
		Source: srcRoot,
	})
	if err != nil || !inst2.OK {
		t.Fatalf("policy-denied canonical install failed: %+v, %v", inst2, err)
	}
	if !isRegularFile(filepath.Join(canonWF, "SKILL.md")) {
		t.Fatal("canonical skill missing when only codex denied")
	}
	if pathExists(filepath.Join(codexSkills, "corral-workflow")) {
		t.Fatal("codex deny policy ignored")
	}
}

func TestManagedSkillOwnershipConflictsDoNotOverwriteOrDelete(t *testing.T) {
	home := setupIsolatedSkillTestEnv(t)
	canonicalSkills := filepath.Join(home, ".config", "agentsync", "skills")
	claudeHome := filepath.Join(home, ".claude")
	if err := os.MkdirAll(canonicalSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonicalSkills, filepath.Join(claudeHome, "skills")); err != nil {
		t.Fatal(err)
	}

	// 1. Pre-existing unowned skill blocks install.
	userSkill := writeTestSkillDir(t, canonicalSkills, "corral-workflow", "custom user content\n")
	srcRoot := filepath.Join(t.TempDir(), "plugin")
	writeTestSkillDir(t, srcRoot, "corral-workflow", "plugin content\n")

	conflictRes, err := RunManagedSkillInstall(ManagedSkillOptions{
		Owner:  "corral-plugin:workflow",
		Source: srcRoot,
	})
	if err != nil {
		t.Fatalf("conflict install returned error: %v", err)
	}
	if conflictRes.OK || len(conflictRes.Conflicts) != 1 {
		t.Fatalf("expected conflict, got %+v", conflictRes)
	}
	got, _ := os.ReadFile(filepath.Join(userSkill, "SKILL.md"))
	if string(got) != "custom user content\n" {
		t.Fatal("unowned user skill was overwritten")
	}

	// 2. Clean install succeeded after removing unowned skill, then another owner is rejected,
	//    and user edit blocks removal.
	_ = os.RemoveAll(userSkill)
	okRes, err := RunManagedSkillInstall(ManagedSkillOptions{Owner: "corral-plugin:workflow", Source: srcRoot})
	if err != nil || !okRes.OK {
		t.Fatalf("clean install failed: %+v, %v", okRes, err)
	}
	otherOwner, err := RunManagedSkillInstall(ManagedSkillOptions{Owner: "other-tool:workflow", Source: srcRoot})
	if err != nil || otherOwner.OK || len(otherOwner.Conflicts) != 1 {
		t.Fatalf("expected owner mismatch conflict, got %+v, %v", otherOwner, err)
	}

	// Edit tracked file -> status reports modified, remove preserves file.
	if err := os.WriteFile(filepath.Join(userSkill, "SKILL.md"), []byte("user edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := RunManagedSkillStatus(ManagedSkillOptions{Owner: "corral-plugin:workflow"})
	if err != nil || st.OK || st.Actions[0].State != "modified" {
		t.Fatalf("expected modified status, got %+v, %v", st, err)
	}
	rm, err := RunManagedSkillRemove(ManagedSkillOptions{Owner: "corral-plugin:workflow"})
	if err != nil || rm.OK || rm.Actions[0].Action != "keep_modified" {
		t.Fatalf("expected keep_modified on remove, got %+v, %v", rm, err)
	}
	edited, _ := os.ReadFile(filepath.Join(userSkill, "SKILL.md"))
	if string(edited) != "user edited\n" {
		t.Fatal("modified managed skill was deleted on remove")
	}
	_ = reflect.TypeOf(rm)
}
