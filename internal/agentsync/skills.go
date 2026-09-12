package agentsync

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func syncSkills(cfg Config, opts Options) ([]TargetResult, []string, error) {
	if cfg.SkillSource == "" || len(cfg.SkillTargets) == 0 {
		return nil, nil, nil
	}

	var results []TargetResult
	var backups []string

	policy, err := loadSyncPolicy(cfg.PolicyPath)
	if err != nil {
		return results, backups, err
	}
	results = append(results, policyWarningResults(cfg.PolicyPath, policy, nil, cfg.SkillTargets)...)

	existing := map[string]string{}
	for _, target := range cfg.SkillTargets {
		found, err := discoverSkills(target.Path, true)
		if err != nil {
			return results, backups, err
		}
		for name, path := range found {
			if _, ok := existing[name]; !ok {
				existing[name] = path
			}
		}
	}

	if !pathExists(cfg.SkillSource) {
		if opts.Check {
			results = append(results, TargetResult{Path: cfg.SkillSource, Status: "missing", Detail: "would create skill source"})
		} else if err := os.MkdirAll(cfg.SkillSource, 0o755); err != nil {
			return results, backups, err
		}
	}
	preserved, preserveBackups, err := preserveHiddenSkillEntries(cfg.SkillSource, cfg.SkillTargets, opts)
	if err != nil {
		return results, backups, err
	}
	results = append(results, preserved...)
	backups = append(backups, preserveBackups...)
	materialized, materialBackups, err := materializeCanonicalSkillLinks(cfg.SkillSource, opts)
	if err != nil {
		return results, backups, err
	}
	results = append(results, materialized...)
	backups = append(backups, materialBackups...)

	names := sortedSkillNames(existing)
	for _, name := range names {
		srcPath := filepath.Join(cfg.SkillSource, name)
		if pathExists(srcPath) {
			continue
		}
		if opts.Check {
			results = append(results, TargetResult{Path: srcPath, Status: "missing", Detail: "would import skill from " + existing[name]})
			continue
		}
		if err := copyDir(existing[name], srcPath); err != nil {
			return results, backups, err
		}
		results = append(results, TargetResult{Path: srcPath, Status: "created", Detail: "skill imported from " + existing[name]})
	}

	canonical, err := discoverSkills(cfg.SkillSource, true)
	if err != nil {
		return results, backups, err
	}
	allNames := sortedSkillNames(canonical)

	for _, target := range cfg.SkillTargets {
		kept, dropped := filterNamesForPolicy(allNames, policy.Skills, target.Name)
		result, backup, err := syncSkillRoot(cfg.SkillSource, target, opts, kept, dropped, policyFiltersTarget(allNames, policy.Skills, target.Name))
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

func discoverSkills(root string, resolveSymlinks bool) (map[string]string, error) {
	skills := map[string]string{}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return skills, nil
		}
		return skills, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(root, name)
		if skillPath := skillDirPath(path, resolveSymlinks); skillPath != "" {
			skills[name] = skillPath
		}
	}
	return skills, nil
}

func preserveHiddenSkillEntries(source string, targets []SkillTarget, opts Options) ([]TargetResult, []string, error) {
	var results []TargetResult
	var backups []string
	for _, target := range targets {
		entries, err := os.ReadDir(target.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return results, backups, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, ".") {
				continue
			}
			srcPath := filepath.Join(target.Path, name)
			dstPath := filepath.Join(source, name)
			if pathExists(dstPath) {
				continue
			}
			info, err := os.Lstat(srcPath)
			if err != nil {
				return results, backups, err
			}
			if !info.IsDir() {
				continue
			}
			if opts.Check {
				results = append(results, TargetResult{Path: dstPath, Status: "missing", Detail: "would preserve hidden skill entry from " + srcPath})
				continue
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return results, backups, err
			}
			results = append(results, TargetResult{Path: dstPath, Status: "created", Detail: "hidden skill entry preserved from " + srcPath})
		}
	}
	return results, backups, nil
}

func skillDirPath(path string, resolveSymlinks bool) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		if resolveSymlinks {
			path = target
		}
	}
	info, err = os.Stat(path)
	if err == nil && info.IsDir() && isRegularFile(filepath.Join(path, "SKILL.md")) {
		return path
	}
	return ""
}

func materializeCanonicalSkillLinks(root string, opts Options) ([]TargetResult, []string, error) {
	var results []TargetResult
	var backups []string
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return results, backups, nil
		}
		return results, backups, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			return results, backups, err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		realPath := skillDirPath(path, true)
		if realPath == "" {
			continue
		}
		if opts.Check {
			results = append(results, TargetResult{Path: path, Status: "replaceable", Detail: "would materialize canonical skill symlink"})
			continue
		}
		backup, err := backupAny(path)
		if err != nil {
			return results, backups, err
		}
		temp := path + ".agentsync-tmp"
		_ = os.RemoveAll(temp)
		if err := copyDir(realPath, temp); err != nil {
			return results, backups, err
		}
		if err := os.Remove(path); err != nil {
			_ = os.RemoveAll(temp)
			return results, backups, err
		}
		if err := os.Rename(temp, path); err != nil {
			_ = os.RemoveAll(temp)
			return results, backups, err
		}
		results = append(results, TargetResult{Path: path, Status: "materialized", Detail: "canonical skill is now a real directory"})
		if backup != "" {
			backups = append(backups, backup)
		}
	}
	return results, backups, nil
}

func sortedSkillNames(skills map[string]string) []string {
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func syncSkillRoot(source string, targetSpec SkillTarget, opts Options, kept, dropped []string, filtered bool) (TargetResult, string, error) {
	target := targetSpec.Path
	result := TargetResult{Path: target}
	// runtime 未安装时跳过，不创建其 skill 根目录别名。
	if targetSpec.Detect != "" && !pathExists(targetSpec.Detect) {
		result.Status = "skipped"
		result.Detail = "runtime not installed"
		return result, "", nil
	}
	if filtered {
		return syncSkillRootFiltered(source, target, opts, kept, dropped)
	}
	return syncSkillRootAlias(source, target, opts)
}

func syncSkillRootAlias(source, target string, opts Options) (TargetResult, string, error) {
	result := TargetResult{Path: target}
	if !pathExists(target) {
		if opts.Check {
			result.Status = "missing"
			result.Detail = "would create skill directory alias"
			return result, "", nil
		}
		kind, err := createAlias(source, target, "link")
		if err != nil {
			return result, "", err
		}
		result.Status = "linked"
		result.Detail = kind
		return result, "", nil
	}

	info, err := os.Lstat(target)
	if err != nil {
		return result, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if symlinkPointsTo(target, source) {
			result.Status = "ok"
			result.Detail = "skill directory symlink"
			return result, "", nil
		}
		if opts.Check {
			result.Status = "wrong-link"
			result.Detail = "would back up and replace skill directory alias"
			return result, "", nil
		}
		backup, err := backupAny(target)
		if err != nil {
			return result, "", err
		}
		if err := os.Remove(target); err != nil {
			return result, "", err
		}
		kind, err := createAlias(source, target, "link")
		if err != nil {
			return result, "", err
		}
		result.Status = "replaced"
		result.Detail = kind
		return result, backup, nil
	}
	if !info.IsDir() {
		result.Status = "blocked"
		result.Detail = "skill target is not a directory"
		return result, "", nil
	}
	if opts.Check {
		result.Status = "replaceable"
		result.Detail = "would back up and replace skill directory with alias"
		return result, "", nil
	}
	backup, err := backupAny(target)
	if err != nil {
		return result, "", err
	}
	if err := os.RemoveAll(target); err != nil {
		return result, "", err
	}
	kind, err := createAlias(source, target, "link")
	if err != nil {
		return result, "", err
	}
	result.Status = "replaced"
	result.Detail = kind
	return result, backup, nil
}

func syncSkillRootFiltered(source, target string, opts Options, kept, dropped []string) (TargetResult, string, error) {
	result := TargetResult{Path: target}
	detailBase := "filtered skill directory"
	if matched, err := filteredSkillRootMatches(source, target, kept); err != nil {
		return result, "", err
	} else if matched {
		result.Status = "ok"
		result.Detail = formatFilteredDetail(detailBase, dropped)
		return result, "", nil
	}

	if opts.Check {
		if pathExists(target) {
			result.Status = "replaceable"
			result.Detail = formatFilteredDetail("would materialize filtered skill directory", dropped)
			return result, "", nil
		}
		result.Status = "missing"
		result.Detail = formatFilteredDetail("would create filtered skill directory", dropped)
		return result, "", nil
	}

	var backup string
	var err error
	if pathExists(target) {
		backup, err = backupAny(target)
		if err != nil {
			return result, "", err
		}
		info, err := os.Lstat(target)
		if err != nil {
			return result, backup, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(target); err != nil {
				return result, backup, err
			}
		} else {
			if err := os.RemoveAll(target); err != nil {
				return result, backup, err
			}
		}
		result.Status = "replaced"
	} else {
		result.Status = "created"
	}
	if err := materializeFilteredSkillRoot(source, target, kept); err != nil {
		return result, backup, err
	}
	result.Detail = formatFilteredDetail(detailBase, dropped)
	return result, backup, nil
}

func filteredSkillRootMatches(source, target string, kept []string) (bool, error) {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	if !info.IsDir() {
		return false, nil
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return false, err
	}
	want := map[string]struct{}{}
	for _, name := range kept {
		want[name] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		seen[name] = struct{}{}
		if _, ok := want[name]; !ok {
			return false, nil
		}
		link := filepath.Join(target, name)
		if !symlinkPointsTo(link, filepath.Join(source, name)) {
			return false, nil
		}
	}
	for name := range want {
		if _, ok := seen[name]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func materializeFilteredSkillRoot(source, target string, kept []string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	// Preserve hidden entries from the canonical source (e.g. Codex .system).
	srcEntries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range srcEntries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".") {
			continue
		}
		srcPath := filepath.Join(source, name)
		dstPath := filepath.Join(target, name)
		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		if pathExists(dstPath) {
			continue
		}
		if err := copyDir(srcPath, dstPath); err != nil {
			return err
		}
	}
	for _, name := range kept {
		srcPath := filepath.Join(source, name)
		dstPath := filepath.Join(target, name)
		if err := os.Symlink(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}
