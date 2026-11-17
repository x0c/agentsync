package agentsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Run(opts Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.Watch {
		return runWatch(opts)
	}
	return runOnce(opts)
}

func runOnce(opts Options) (runErr error) {
	if opts.Rollback != "" {
		return runRollback(opts)
	}
	if opts.All != "" {
		return runAll(opts)
	}
	var cfg Config
	var err error
	if opts.Repo {
		root, err := findRepoRoot(".")
		if err != nil {
			return err
		}
		cfg = repoConfig(root)
	} else {
		cfg, err = defaultGlobalConfig()
		if err != nil {
			return err
		}
	}
	if !opts.Check {
		if _, err := beginBackupSession(); err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, endBackupSession()) }()
	}
	if opts.Adopt != "" {
		backups, err := adoptDraft(cfg.Source, opts.Adopt, opts.Check)
		if err != nil {
			return err
		}
		report := RunReport{Source: cfg.Source, Backups: backups}
		if !opts.Check {
			r, err := syncConfig(cfg, opts)
			if err != nil {
				return err
			}
			report.Results = r.Results
			report.SkillResults = r.SkillResults
			report.MCPResults = r.MCPResults
			report.MergeDraft = r.MergeDraft
			report.Backups = append(report.Backups, r.Backups...)
		}
		printReport(report, opts)
		return reportError(report)
	}
	report, err := syncConfig(cfg, opts)
	if err != nil {
		return err
	}
	printReport(report, opts)
	return reportError(report)
}

func reportError(report RunReport) error {
	var failures []error
	for _, results := range [][]TargetResult{report.Results, report.SkillResults, report.MCPResults} {
		for _, result := range results {
			if result.Status == "blocked" {
				failures = append(failures, fmt.Errorf("blocked %s: %s", result.Path, result.Detail))
			}
		}
	}
	return errors.Join(failures...)
}

func syncConfig(cfg Config, opts Options) (RunReport, error) {
	report := RunReport{Source: cfg.Source}

	// Runtime fragments are validated before any write so a duplicate or
	// unreadable file cannot leave a half-updated source. Rendering still happens
	// per target, after imports, against the final source.
	overlays, overlayWarnings, err := loadInstructionOverlays(cfg.InstructionOverlayDir, cfg.Targets)
	if err != nil {
		return report, err
	}
	report.Results = append(report.Results, overlayWarnings...)

	if !pathExists(cfg.Source) {
		var existing []string
		for _, t := range cfg.Targets {
			// Generated copies are derived, never a source of instruction text.
			if p := readableContentPath(t.Path); p != "" && !isManagedAgentsyncFile(p) {
				existing = append(existing, p)
			}
		}
		if !opts.Check {
			if err := createSourceFromFiles(cfg.Source, existing); err != nil {
				return report, err
			}
		}
		status := "created"
		detail := "canonical source"
		if opts.Check {
			status = "missing"
			if len(existing) > 0 {
				detail = "would create canonical source from existing target files"
			} else {
				detail = "would create canonical source"
			}
		} else if len(existing) > 0 {
			detail = "canonical source created from existing target files"
		}
		report.Results = append(report.Results, TargetResult{Path: cfg.Source, Status: status, Detail: detail})
	}

	if cfg.MCPSource != "" && len(cfg.MCPTargets) > 0 {
		notice, err := syncMCPNotice(cfg.Source, opts)
		if err != nil {
			return report, err
		}
		if notice != nil {
			report.MCPResults = append(report.MCPResults, *notice)
		}
	}

	// Complete imports before generating copies, regardless of target ordering.
	// Note: same-home pairs (e.g. ~/.gemini/GEMINI.md + ~/.gemini/AGENTS.md) with
	// differing pre-existing content are both appended to the source on first
	// import, so overlapping sections may duplicate once until deduped in source.
	if !opts.Check && !opts.Force {
		for _, target := range cfg.Targets {
			if _, err := importInstructionTarget(cfg.Source, target); err != nil {
				return report, err
			}
		}
	}
	// Runtime fragments extend the final source, so they are applied per target
	// below, after imports, against the final source.
	for _, target := range cfg.Targets {
		// A missing source is reported as its own result; never fail the preview
		// because a fragment had nothing to extend.
		render := instructionRender{}
		if pathExists(cfg.Source) {
			render = newInstructionRender(target, overlays)
		}
		if target.Mode == "antigravity" {
			result, backups, err := syncAntigravityTarget(cfg.Source, target, render, opts)
			if err != nil {
				return report, err
			}
			report.Results = append(report.Results, result)
			report.Backups = append(report.Backups, backups...)
			continue
		}
		result, backup, err := syncTarget(cfg.Source, target, render, opts)
		if err != nil {
			return report, err
		}
		report.Results = append(report.Results, result)
		if backup != "" {
			report.Backups = append(report.Backups, backup)
		}
		if warning := windsurfLimitWarning(cfg.Source, target, render, result.Status); warning != nil {
			report.Results = append(report.Results, *warning)
		}
	}
	skillResults, skillBackups, err := syncSkills(cfg, opts)
	if err != nil {
		return report, err
	}
	report.SkillResults = append(report.SkillResults, skillResults...)
	report.Backups = append(report.Backups, skillBackups...)
	if !opts.SkipMCP {
		mcpResults, mcpBackups, err := syncMCP(cfg, opts)
		if err != nil {
			return report, err
		}
		report.MCPResults = append(report.MCPResults, mcpResults...)
		report.Backups = append(report.Backups, mcpBackups...)
	}
	return report, nil
}

func readableContentPath(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if info.Mode().IsRegular() {
		return path
	}
	if !isManagedLink(info) {
		return ""
	}
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	if isRegularFile(target) {
		return target
	}
	return ""
}

func importInstructionTarget(source string, target Target) (bool, error) {
	if target.Detect != "" && !pathExists(target.Detect) {
		return false, nil
	}
	origin := readableContentPath(target.Path)
	if origin == "" || sameContent(source, origin) || isManagedAgentsyncFile(origin) {
		return false, nil
	}
	return appendImportedContent(source, origin)
}

func syncTarget(source string, target Target, render instructionRender, opts Options) (TargetResult, string, error) {
	result := TargetResult{Path: target.Path}
	// Skip absent runtimes without creating directories or files.
	if target.Detect != "" && !pathExists(target.Detect) {
		result.Status = "skipped"
		result.Detail = "runtime not installed"
		return result, "", nil
	}
	if !pathExists(target.Path) {
		if opts.Check {
			result.Status = "missing"
			result.Detail = "would create alias"
			if render.active() {
				result.Detail = fmt.Sprintf("would create a managed copy with the %s overlay", render.Runtime)
			}
			return result, "", nil
		}
		kind, err := createAlias(source, target.Path, target.Mode, render)
		if err != nil {
			return result, "", err
		}
		result.Status = "linked"
		result.Detail = aliasDetail(kind, render)
		return result, "", nil
	}

	info, err := os.Lstat(target.Path)
	if err != nil {
		return result, "", err
	}
	if isManagedLink(info) {
		// Cursor mode and runtime overlays require a managed file, not a bare
		// source symlink, so the link is replaced after its content is imported.
		if target.Mode == "cursor" || render.active() {
			if opts.Check {
				result.Status = "wrong-link"
				result.Detail = "would preserve unmanaged content and replace symlink with " + derivedKind(target, render)
				return result, "", nil
			}
			if !opts.Force {
				if _, err := importInstructionTarget(source, target); err != nil {
					return result, "", err
				}
			}
			backup, err := backupFile(target.Path)
			if err != nil {
				return result, "", err
			}
			if err := removePath(target.Path); err != nil {
				return result, "", err
			}
			kind, err := createAlias(source, target.Path, target.Mode, render)
			if err != nil {
				return result, "", err
			}
			result.Status = "replaced"
			result.Detail = aliasDetail(kind, render)
			return result, backup, nil
		}
		if linkPointsTo(target.Path, source) {
			result.Status = "ok"
			result.Detail = "symlink"
			return result, "", nil
		}
		linkTarget, readErr := os.Readlink(target.Path)
		if readErr == nil && !filepath.IsAbs(linkTarget) && !pathExists(filepath.Join(filepath.Dir(target.Path), linkTarget)) {
			if opts.Check {
				result.Status = "broken-link"
				result.Detail = "would repair"
				return result, "", nil
			}
			backup, err := backupFile(target.Path)
			if err != nil {
				return result, "", err
			}
			if err := removePath(target.Path); err != nil {
				return result, "", err
			}
			kind, err := createAlias(source, target.Path, target.Mode, instructionRender{})
			if err != nil {
				return result, "", err
			}
			result.Status = "repaired"
			result.Detail = kind
			return result, backup, nil
		}
		if opts.Force {
			if opts.Check {
				result.Status = "wrong-link"
				result.Detail = "would replace because --force was set"
				return result, "", nil
			}
			backup, err := backupFile(target.Path)
			if err != nil {
				return result, "", err
			}
			if err := removePath(target.Path); err != nil {
				return result, "", err
			}
			kind, err := createAlias(source, target.Path, target.Mode, instructionRender{})
			if err != nil {
				return result, "", err
			}
			result.Status = "replaced"
			result.Detail = kind
			return result, backup, nil
		}
		if opts.Check {
			result.Status = "wrong-link"
			if p := readableContentPath(target.Path); p != "" {
				result.Detail = "would merge linked content, back up, and replace"
			} else {
				result.Detail = "would back up and replace"
			}
			return result, "", nil
		}
		if _, err := importInstructionTarget(source, target); err != nil {
			return result, "", err
		}
		backup, err := backupFile(target.Path)
		if err != nil {
			return result, "", err
		}
		if err := removePath(target.Path); err != nil {
			return result, "", err
		}
		kind, err := createAlias(source, target.Path, target.Mode, instructionRender{})
		if err != nil {
			return result, "", err
		}
		result.Status = "replaced"
		result.Detail = kind
		return result, backup, nil
	}
	if !info.Mode().IsRegular() {
		result.Status = "blocked"
		result.Detail = "not a regular file"
		return result, "", nil
	}
	// A runtime overlay always materializes as a managed copy, and its removal
	// restores the plain alias so the target follows the source again.
	if render.active() {
		return syncOverlayCopy(source, target, render, opts, result)
	}
	if managedCopyOverlay(target.Path) != "" {
		return syncOverlayRemoval(source, target, opts, result)
	}
	plainCopyCurrent := false
	if isManagedAgentsyncFile(target.Path) {
		if payload, readErr := os.ReadFile(source); readErr == nil {
			plainCopyCurrent = managedCopyMatches(target.Path, payload, target.Mode, "")
		}
	}
	if sameContent(source, target.Path) && (target.Mode != "cursor" || isManagedAgentsyncFile(target.Path)) && (!isManagedAgentsyncFile(target.Path) || plainCopyCurrent) {
		if !opts.Force {
			result.Status = "ok"
			if target.Mode == "cursor" {
				result.Detail = "cursor-rule"
			} else {
				result.Detail = "content matches source"
			}
			return result, "", nil
		}
		if opts.Check {
			result.Status = "replaceable"
			result.Detail = "would replace because --force was set"
			return result, "", nil
		}
		backup, err := backupFile(target.Path)
		if err != nil {
			return result, "", err
		}
		if err := removePath(target.Path); err != nil {
			return result, "", err
		}
		kind, err := createAlias(source, target.Path, target.Mode, instructionRender{})
		if err != nil {
			return result, "", err
		}
		result.Status = "replaced"
		result.Detail = kind
		return result, backup, nil
	}
	// Replace managed copies and forced targets from the canonical source without importing them.
	// Importing a stale managed body would duplicate old instructions in the source.
	if opts.Force || isManagedAgentsyncFile(target.Path) {
		if opts.Check {
			result.Status = "replaceable"
			switch {
			case opts.Force:
				result.Detail = "would replace because --force was set"
			case target.Mode == "cursor":
				result.Detail = "would replace stale managed cursor-rule from source"
			default:
				result.Detail = "would replace stale managed copy from source"
			}
			return result, "", nil
		}
		backup, err := backupFile(target.Path)
		if err != nil {
			return result, "", err
		}
		if err := removePath(target.Path); err != nil {
			return result, "", err
		}
		kind, err := createAlias(source, target.Path, target.Mode, instructionRender{})
		if err != nil {
			return result, "", err
		}
		result.Status = "replaced"
		result.Detail = kind
		return result, backup, nil
	}
	if opts.Check {
		result.Status = "mergeable"
		result.Detail = "would merge unique content and replace with alias"
		return result, "", nil
	}
	merged, err := appendImportedContent(source, target.Path)
	if err != nil {
		return result, "", err
	}
	backup, err := backupFile(target.Path)
	if err != nil {
		return result, "", err
	}
	if err := removePath(target.Path); err != nil {
		return result, "", err
	}
	kind, err := createAlias(source, target.Path, target.Mode, instructionRender{})
	if err != nil {
		return result, "", err
	}
	result.Status = "merged"
	if merged {
		result.Detail = "content appended; " + kind
	} else {
		result.Detail = "content already present; " + kind
	}
	return result, backup, nil
}

// derivedKind names the generated file that replaces a bare symlink.
func derivedKind(target Target, render instructionRender) string {
	if target.Mode == "cursor" {
		return "cursor-rule"
	}
	return fmt.Sprintf("a managed copy plus the %s overlay", render.Runtime)
}

// aliasDetail labels a created alias or managed copy in the report.
func aliasDetail(kind string, render instructionRender) string {
	if !render.active() {
		return kind
	}
	if kind == "cursor-rule" {
		return fmt.Sprintf("cursor-rule with the %s overlay", render.Runtime)
	}
	return fmt.Sprintf("managed copy with the %s overlay", render.Runtime)
}

// syncOverlayCopy writes source plus the runtime fragment as a managed copy,
// preserving unique unmanaged content first. A stale copy is refreshed without
// importing its body, so old fragments never leak back into the source.
func syncOverlayCopy(source string, target Target, render instructionRender, opts Options, result TargetResult) (TargetResult, string, error) {
	payload, err := render.body(source)
	if err != nil {
		return result, "", err
	}
	detail := fmt.Sprintf("managed copy with the %s overlay", render.Runtime)
	if target.Mode == "cursor" {
		detail = fmt.Sprintf("cursor-rule with the %s overlay", render.Runtime)
	}
	merged := false
	if !opts.Force {
		if isManagedAgentsyncFile(target.Path) {
			if managedCopyMatches(target.Path, payload, target.Mode, render.Runtime) {
				result.Status = "ok"
				result.Detail = detail
				return result, "", nil
			}
		} else {
			// A hand-written file may hold instructions the source does not have yet.
			if opts.Check {
				if sameContent(source, target.Path) {
					result.Status = "replaceable"
					result.Detail = "would replace with " + detail
				} else {
					result.Status = "mergeable"
					result.Detail = "would merge unique content and write " + detail
				}
				return result, "", nil
			}
			appended, err := appendImportedContent(source, target.Path)
			if err != nil {
				return result, "", err
			}
			merged = appended
			if payload, err = render.body(source); err != nil {
				return result, "", err
			}
		}
	}
	if opts.Check {
		result.Status = "replaceable"
		if isManagedAgentsyncFile(target.Path) {
			result.Detail = "would refresh " + detail + " from the canonical source"
		} else {
			result.Detail = "would replace with " + detail
		}
		return result, "", nil
	}
	backup, err := backupFile(target.Path)
	if err != nil {
		return result, "", err
	}
	// Cursor only loads .mdc rules that carry alwaysApply frontmatter.
	// The write is atomic, so a failed write keeps the previous copy.
	if target.Mode == "cursor" {
		err = writeCursorRuleBytes(source, target.Path, payload, render.Runtime)
	} else {
		err = writeManagedCopyBytes(source, target.Path, payload, render.Runtime)
	}
	if err != nil {
		return result, "", err
	}
	result.Status = "replaced"
	result.Detail = detail
	if merged {
		result.Status = "merged"
		result.Detail = "content appended; " + detail
	}
	return result, backup, nil
}

// syncOverlayRemoval restores the plain alias recorded by an overlay copy once
// the runtime fragment is gone, so later source edits apply without a rewrite.
func syncOverlayRemoval(source string, target Target, opts Options, result TargetResult) (TargetResult, string, error) {
	if opts.Check {
		result.Status = "replaceable"
		result.Detail = "would restore alias now that the runtime overlay is gone"
		return result, "", nil
	}
	backup, err := backupFile(target.Path)
	if err != nil {
		return result, "", err
	}
	if err := removePath(target.Path); err != nil {
		return result, "", err
	}
	kind, err := createAlias(source, target.Path, target.Mode, instructionRender{})
	if err != nil {
		return result, "", err
	}
	result.Status = "replaced"
	result.Detail = "restored " + kind + " now that the runtime overlay is gone"
	return result, backup, nil
}

func defaultSourceContent() string {
	return `# AGENTS.md

## Working Agreements

- Keep this file as the single source of truth for AI coding agent instructions.
- Tool-specific instruction files should link to this file when possible.
`
}

func printReport(report RunReport, opts Options) {
	mode := "apply"
	if opts.Check {
		mode = "check"
	}
	fmt.Printf("agentsync %s\n\n", mode)
	fmt.Printf("Source:\n  %s\n\n", report.Source)
	if report.Repositories > 0 || opts.All != "" {
		fmt.Printf("Repositories:\n  %d\n\n", report.Repositories)
	}
	if len(report.Results) > 0 {
		fmt.Println("Results:")
		for _, r := range report.Results {
			if r.Detail != "" {
				fmt.Printf("  %-12s %s (%s)\n", r.Status, r.Path, r.Detail)
			} else {
				fmt.Printf("  %-12s %s\n", r.Status, r.Path)
			}
		}
		fmt.Println()
	}
	if len(report.SkillResults) > 0 {
		fmt.Println("Skills:")
		for _, r := range report.SkillResults {
			if r.Detail != "" {
				fmt.Printf("  %-12s %s (%s)\n", r.Status, r.Path, r.Detail)
			} else {
				fmt.Printf("  %-12s %s\n", r.Status, r.Path)
			}
		}
		fmt.Println()
	}
	if len(report.MCPResults) > 0 {
		fmt.Println("MCP:")
		for _, r := range report.MCPResults {
			if r.Detail != "" {
				fmt.Printf("  %-12s %s (%s)\n", r.Status, r.Path, r.Detail)
			} else {
				fmt.Printf("  %-12s %s\n", r.Status, r.Path)
			}
		}
		fmt.Println()
	}
	if len(report.Backups) > 0 {
		fmt.Println("Backups:")
		for _, b := range report.Backups {
			fmt.Printf("  %s\n", b)
		}
		fmt.Println()
	}
	if report.MergeDraft != "" {
		fmt.Println("Merge draft:")
		fmt.Printf("  %s\n\n", report.MergeDraft)
		fmt.Println("Next:")
		fmt.Printf("  Review it, then run: agentsync --adopt %s\n\n", shellQuote(report.MergeDraft))
	}
}

func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if pathExists(filepath.Join(dir, ".git")) {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", fmt.Errorf("not inside a git repository")
		}
		dir = next
	}
}

func runAll(opts Options) error {
	root, err := expandPath(opts.All)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve scan root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect scan root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("scan root is not a directory: %s", root)
	}
	var repos []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("scan %s: %w", path, err)
		}
		name := d.Name()
		if name == ".git" {
			repos = append(repos, filepath.Dir(path))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && (name == "node_modules" || name == ".venv" || name == "target" || name == "build") {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	combined := RunReport{Source: "multiple repositories", Repositories: len(repos)}
	for _, repo := range repos {
		if !opts.Check {
			if _, err := beginBackupSession(); err != nil {
				return err
			}
		}
		cfg := repoConfig(repo)
		report, err := syncConfig(cfg, opts)
		if !opts.Check {
			err = errors.Join(err, endBackupSession())
		}
		if err != nil {
			return err
		}
		if err := reportError(report); err != nil {
			printReport(report, opts)
			return err
		}
		for _, r := range report.Results {
			if !strings.HasPrefix(r.Path, repo) {
				r.Path = filepath.Join(repo, r.Path)
			}
			combined.Results = append(combined.Results, r)
		}
		combined.Backups = append(combined.Backups, report.Backups...)
		if report.MergeDraft != "" {
			combined.Results = append(combined.Results, TargetResult{Path: repo, Status: "conflict", Detail: "merge draft: " + report.MergeDraft})
		}
	}
	printReport(combined, opts)
	return nil
}
