package agentsync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Run(opts Options) error {
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
		return nil
	}
	report, err := syncConfig(cfg, opts)
	if err != nil {
		return err
	}
	printReport(report, opts)
	return nil
}

func syncConfig(cfg Config, opts Options) (RunReport, error) {
	report := RunReport{Source: cfg.Source}

	if !pathExists(cfg.Source) {
		var existing []string
		for _, t := range cfg.Targets {
			if p := readableContentPath(t.Path); p != "" {
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

	for _, target := range cfg.Targets {
		result, backup, err := syncTarget(cfg.Source, target, opts)
		if err != nil {
			return report, err
		}
		report.Results = append(report.Results, result)
		if backup != "" {
			report.Backups = append(report.Backups, backup)
		}
	}
	skillResults, skillBackups, err := syncSkills(cfg, opts)
	if err != nil {
		return report, err
	}
	report.SkillResults = append(report.SkillResults, skillResults...)
	report.Backups = append(report.Backups, skillBackups...)
	mcpResults, mcpBackups, err := syncMCP(cfg, opts)
	if err != nil {
		return report, err
	}
	report.MCPResults = append(report.MCPResults, mcpResults...)
	report.Backups = append(report.Backups, mcpBackups...)
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
	if info.Mode()&os.ModeSymlink == 0 {
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

func syncTarget(source string, target Target, opts Options) (TargetResult, string, error) {
	result := TargetResult{Path: target.Path}
	// runtime 未安装（标志目录不存在）时直接跳过，绝不为其创建目录或文件。
	if target.Detect != "" && !pathExists(target.Detect) {
		result.Status = "skipped"
		result.Detail = "runtime not installed"
		return result, "", nil
	}
	if !pathExists(target.Path) {
		if opts.Check {
			result.Status = "missing"
			result.Detail = "would create alias"
			return result, "", nil
		}
		kind, err := createAlias(source, target.Path, target.Mode)
		if err != nil {
			return result, "", err
		}
		result.Status = "linked"
		result.Detail = kind
		return result, "", nil
	}

	info, err := os.Lstat(target.Path)
	if err != nil {
		return result, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// cursor 模式必须是带 frontmatter 的受管文件，裸 symlink 到统一源不算就绪。
		if target.Mode == "cursor" {
			if opts.Check {
				result.Status = "wrong-link"
				result.Detail = "would replace symlink with cursor-rule"
				return result, "", nil
			}
			backup, err := backupFile(target.Path)
			if err != nil {
				return result, "", err
			}
			if err := removePath(target.Path); err != nil {
				return result, "", err
			}
			kind, err := createAlias(source, target.Path, target.Mode)
			if err != nil {
				return result, "", err
			}
			result.Status = "replaced"
			result.Detail = kind
			return result, backup, nil
		}
		if symlinkPointsTo(target.Path, source) {
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
			kind, err := createAlias(source, target.Path, target.Mode)
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
			kind, err := createAlias(source, target.Path, target.Mode)
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
		if p := readableContentPath(target.Path); p != "" {
			if _, err := appendImportedContent(source, p); err != nil {
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
		kind, err := createAlias(source, target.Path, target.Mode)
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
	if sameContent(source, target.Path) {
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
		kind, err := createAlias(source, target.Path, target.Mode)
		if err != nil {
			return result, "", err
		}
		result.Status = "replaced"
		result.Detail = kind
		return result, backup, nil
	}
	// 受管副本（Cursor .mdc / writeManagedCopy）与 --force：统一源为准，直接替换，不回写。
	// 否则「改统一源 → 再跑 agentsync」会把过期受管正文当独特内容 append 进统一源，造成整篇重复。
	if opts.Force || target.Mode == "cursor" || isManagedAgentsyncFile(target.Path) {
		if opts.Check {
			result.Status = "replaceable"
			switch {
			case opts.Force:
				result.Detail = "would replace because --force was set"
			case target.Mode == "cursor":
				result.Detail = "would replace stale cursor-rule from source"
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
		kind, err := createAlias(source, target.Path, target.Mode)
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
	kind, err := createAlias(source, target.Path, target.Mode)
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
	if report.Repositories > 0 {
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
	var repos []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
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
		cfg := repoConfig(repo)
		report, err := syncConfig(cfg, opts)
		if err != nil {
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
