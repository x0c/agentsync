package agentsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const managedSkillsVersion = 1

var (
	managedSkillNameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	managedSkillOwnerRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
)

// ManagedSkillOptions configures scoped install, remove, and status runs for
// externally owned skill directories (e.g. Corral plugin skills).
type ManagedSkillOptions struct {
	Owner  string
	Source string
	Names  []string
	Check  bool
	JSON   bool
}

// ManagedSkillAction describes the inspected or applied state of one skill
// inside one physical destination directory.
type ManagedSkillAction struct {
	Skill    string   `json:"skill"`
	Path     string   `json:"path"`
	Runtimes []string `json:"runtimes,omitempty"`
	Action   string   `json:"action"`
	State    string   `json:"state"`
	Detail   string   `json:"detail,omitempty"`
}

// ManagedSkillReport is the structured result emitted by `agentsync skill …`.
type ManagedSkillReport struct {
	OK        bool                 `json:"ok"`
	Action    string               `json:"action"`
	Mode      string               `json:"mode"`
	Owner     string               `json:"owner,omitempty"`
	Check     bool                 `json:"check"`
	Actions   []ManagedSkillAction `json:"actions"`
	Conflicts []ManagedSkillAction `json:"conflicts"`
}

type managedSkillsManifest struct {
	Version int                           `json:"version"`
	Entries map[string]managedSkillRecord `json:"entries"`
}

type managedSkillRecord struct {
	Owner     string            `json:"owner"`
	Skill     string            `json:"skill"`
	Files     map[string]string `json:"files"`
	UpdatedAt string            `json:"updated_at"`
}

type sourceSkillBundle struct {
	Name   string
	Dir    string
	Files  map[string][]byte
	Hashes map[string]string
}

type skillWriteRoot struct {
	PhysicalRoot string
	DisplayRoot  string
	Runtimes     []string
}

type skillPlacementMode struct {
	Mode            string
	CanonicalRoot   string
	DetectedTargets []SkillTarget
	WriteRoots      []skillWriteRoot
	FilteredTargets []SkillTarget
}

func validateManagedOwner(owner string, required bool) error {
	trimmed := strings.TrimSpace(owner)
	if trimmed == "" {
		if required {
			return fmt.Errorf("--owner is required")
		}
		return nil
	}
	if !managedSkillOwnerRe.MatchString(trimmed) {
		return fmt.Errorf("invalid --owner %q", owner)
	}
	return nil
}

func validateManagedSkillNames(names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if !managedSkillNameRe.MatchString(name) {
			return nil, fmt.Errorf("invalid skill name %q", raw)
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func hashSkillBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readSkillBundle(name, dir string) (sourceSkillBundle, error) {
	if !managedSkillNameRe.MatchString(name) {
		return sourceSkillBundle{}, fmt.Errorf("invalid skill directory name %q", name)
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return sourceSkillBundle{}, fmt.Errorf("resolve skill directory %s: %w", dir, err)
	}
	files := map[string][]byte{}
	hashes := map[string]string{}
	err = filepath.WalkDir(resolvedDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == resolvedDir {
			return nil
		}
		rel, err := filepath.Rel(resolvedDir, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		base := entry.Name()
		if base == ".DS_Store" || strings.HasPrefix(base, ".agentsync") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if isManagedLink(info) {
			return fmt.Errorf("symlinks and junctions are not supported inside source skill %s (%s)", name, relSlash)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported non-regular file in skill %s: %s", name, relSlash)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[relSlash] = data
		hashes[relSlash] = hashSkillBytes(data)
		return nil
	})
	if err != nil {
		return sourceSkillBundle{}, err
	}
	if _, ok := files["SKILL.md"]; !ok {
		return sourceSkillBundle{}, fmt.Errorf("skill %s is missing SKILL.md", name)
	}
	return sourceSkillBundle{
		Name:   name,
		Dir:    resolvedDir,
		Files:  files,
		Hashes: hashes,
	}, nil
}

func discoverSourceSkills(source string, filterNames []string) ([]sourceSkillBundle, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("skill source path is required")
	}
	absSource, err := expandPath(source)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absSource)
	if err != nil {
		return nil, fmt.Errorf("inspect skill source %s: %w", absSource, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill source is not a directory: %s", absSource)
	}

	want := map[string]struct{}{}
	for _, n := range filterNames {
		want[n] = struct{}{}
	}

	candidates := map[string]string{}
	if isRegularFile(filepath.Join(absSource, "SKILL.md")) {
		name := filepath.Base(absSource)
		candidates[name] = absSource
	} else {
		entries, err := os.ReadDir(absSource)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			child := filepath.Join(absSource, name)
			if isDirectorySource(child) && isRegularFile(filepath.Join(child, "SKILL.md")) {
				candidates[name] = child
			}
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no valid skills (containing SKILL.md) found in %s", absSource)
	}
	for name := range want {
		if _, ok := candidates[name]; !ok {
			return nil, fmt.Errorf("skill %q not found in %s", name, absSource)
		}
	}

	names := make([]string, 0, len(candidates))
	for name := range candidates {
		if len(want) > 0 {
			if _, ok := want[name]; !ok {
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)

	bundles := make([]sourceSkillBundle, 0, len(names))
	for _, name := range names {
		bundle, err := readSkillBundle(name, candidates[name])
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func managedSkillsManifestPath() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", "managed-skills.json"), nil
}

func managedSkillsLockPath() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", "managed-skills.lock"), nil
}

func acquireManagedSkillsLock() (func(), error) {
	lockPath, err := managedSkillsLockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		// Break stale lock files left by crashed writers after 60 seconds.
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > 60*time.Second {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for managed skills lock %s", lockPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func loadManagedSkillsManifest() (managedSkillsManifest, string, error) {
	path, err := managedSkillsManifestPath()
	if err != nil {
		return managedSkillsManifest{}, "", err
	}
	out := managedSkillsManifest{
		Version: managedSkillsVersion,
		Entries: map[string]managedSkillRecord{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, path, nil
		}
		return out, path, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return out, path, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, path, fmt.Errorf("parse managed skills manifest %s: %w", path, err)
	}
	if out.Entries == nil {
		out.Entries = map[string]managedSkillRecord{}
	}
	if out.Version == 0 {
		out.Version = managedSkillsVersion
	}
	return out, path, nil
}

func saveManagedSkillsManifest(path string, manifest managedSkillsManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	manifest.Version = managedSkillsVersion
	if manifest.Entries == nil {
		manifest.Entries = map[string]managedSkillRecord{}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data, 0o600)
}

func dirHasLinkInto(dir, canonicalRoot string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		child := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(child)
		if err != nil || !isManagedLink(info) {
			continue
		}
		if linkPointsTo(child, filepath.Join(canonicalRoot, entry.Name())) {
			return true
		}
	}
	return false
}

func detectSkillPlacementMode(cfg Config, policy SyncPolicy) (skillPlacementMode, error) {
	detected := make([]SkillTarget, 0, len(cfg.SkillTargets))
	for _, target := range cfg.SkillTargets {
		if target.Detect == "" || pathExists(target.Detect) {
			detected = append(detected, target)
		}
	}

	canonicalActive := false
	var canonicalRuntimes []string
	var filteredTargets []SkillTarget
	if cfg.SkillSource != "" && pathExists(cfg.SkillSource) {
		for _, target := range detected {
			if linkPointsTo(target.Path, cfg.SkillSource) {
				canonicalActive = true
				canonicalRuntimes = append(canonicalRuntimes, target.Name)
				continue
			}
			info, err := os.Lstat(target.Path)
			if err == nil && !isManagedLink(info) && info.IsDir() && dirHasLinkInto(target.Path, cfg.SkillSource) {
				canonicalActive = true
				canonicalRuntimes = append(canonicalRuntimes, target.Name)
				filteredTargets = append(filteredTargets, target)
			}
		}
		if !canonicalActive && len(policy.Skills.Targets) > 0 {
			for _, target := range detected {
				info, err := os.Lstat(target.Path)
				if err == nil && !isManagedLink(info) && info.IsDir() {
					if _, hasPol := policy.Skills.targetPolicy(target.Name); hasPol {
						canonicalActive = true
						canonicalRuntimes = append(canonicalRuntimes, target.Name)
						filteredTargets = append(filteredTargets, target)
					}
				}
			}
		}
	}

	if canonicalActive {
		resolvedCanonical, err := resolvedSkillPath(cfg.SkillSource)
		if err != nil {
			return skillPlacementMode{}, err
		}
		// Include any filtered targets not yet added if they are materialized directories under canonical sync.
		seenFiltered := map[string]struct{}{}
		for _, ft := range filteredTargets {
			seenFiltered[ft.Path] = struct{}{}
		}
		for _, target := range detected {
			if _, ok := seenFiltered[target.Path]; ok {
				continue
			}
			if _, hasPol := policy.Skills.targetPolicy(target.Name); hasPol {
				info, err := os.Lstat(target.Path)
				if err == nil && !isManagedLink(info) && info.IsDir() {
					filteredTargets = append(filteredTargets, target)
					canonicalRuntimes = append(canonicalRuntimes, target.Name)
				}
			}
		}
		sort.Strings(canonicalRuntimes)
		return skillPlacementMode{
			Mode:            "canonical",
			CanonicalRoot:   cfg.SkillSource,
			DetectedTargets: detected,
			WriteRoots: []skillWriteRoot{
				{
					PhysicalRoot: resolvedCanonical,
					DisplayRoot:  cfg.SkillSource,
					Runtimes:     canonicalRuntimes,
				},
			},
			FilteredTargets: filteredTargets,
		}, nil
	}

	groupsByRoot := map[string]*skillWriteRoot{}
	orderedKeys := []string{}
	for _, target := range detected {
		resolved, err := resolvedSkillPath(target.Path)
		if err != nil {
			return skillPlacementMode{}, err
		}
		group, exists := groupsByRoot[resolved]
		if !exists {
			group = &skillWriteRoot{
				PhysicalRoot: resolved,
				DisplayRoot:  target.Path,
			}
			groupsByRoot[resolved] = group
			orderedKeys = append(orderedKeys, resolved)
		}
		group.Runtimes = append(group.Runtimes, target.Name)
	}
	roots := make([]skillWriteRoot, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		g := groupsByRoot[key]
		sort.Strings(g.Runtimes)
		roots = append(roots, *g)
	}
	return skillPlacementMode{
		Mode:            "standalone",
		DetectedTargets: detected,
		WriteRoots:      roots,
	}, nil
}

func inspectManagedDestination(destDir, owner, skillName string, record *managedSkillRecord) (state string, detail string) {
	info, err := os.Lstat(destDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", ""
		}
		return "conflict", fmt.Sprintf("cannot inspect existing path: %v", err)
	}
	if isManagedLink(info) {
		return "conflict", "destination is a symlink or junction"
	}
	if !info.IsDir() {
		return "conflict", "destination exists and is not a directory"
	}
	if record == nil {
		return "conflict", "existing unowned skill directory left untouched"
	}
	if owner != "" && record.Owner != owner {
		return "conflict", fmt.Sprintf("owned by %q, not %q", record.Owner, owner)
	}
	if skillName != "" && record.Skill != skillName {
		return "conflict", fmt.Sprintf("recorded skill mismatch (%q vs %q)", record.Skill, skillName)
	}
	if len(record.Files) == 0 {
		return "conflict", "manifest entry has no tracked files"
	}
	for rel, expectedHash := range record.Files {
		filePath := filepath.Join(destDir, filepath.FromSlash(rel))
		fi, err := os.Lstat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				return "modified", fmt.Sprintf("tracked file missing: %s", rel)
			}
			return "conflict", fmt.Sprintf("cannot inspect tracked file %s: %v", rel, err)
		}
		if isManagedLink(fi) {
			return "conflict", fmt.Sprintf("tracked file replaced with link: %s", rel)
		}
		if !fi.Mode().IsRegular() {
			return "modified", fmt.Sprintf("tracked file is no longer regular: %s", rel)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "conflict", fmt.Sprintf("cannot read tracked file %s: %v", rel, err)
		}
		if hashSkillBytes(data) != expectedHash {
			return "modified", fmt.Sprintf("user-modified file: %s", rel)
		}
	}
	return "owned", ""
}

func fileMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func writeManagedSkillDir(destDir string, bundle sourceSkillBundle, oldRecord *managedSkillRecord) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	relNames := make([]string, 0, len(bundle.Files))
	for rel := range bundle.Files {
		relNames = append(relNames, rel)
	}
	sort.Strings(relNames)
	for _, rel := range relNames {
		dstPath := filepath.Join(destDir, filepath.FromSlash(rel))
		if err := writeFileAtomic(dstPath, bundle.Files[rel], 0o644); err != nil {
			return err
		}
	}
	if oldRecord != nil {
		for oldRel := range oldRecord.Files {
			if _, stillPresent := bundle.Files[oldRel]; stillPresent {
				continue
			}
			stalePath := filepath.Join(destDir, filepath.FromSlash(oldRel))
			_ = os.Remove(stalePath)
			pruneEmptyParents(filepath.Dir(stalePath), destDir)
		}
	}
	return nil
}

func removeManagedSkillFiles(destDir string, record managedSkillRecord) error {
	for rel := range record.Files {
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		pruneEmptyParents(filepath.Dir(target), destDir)
	}
	removeDirIfEmpty(destDir)
	return nil
}

func pruneEmptyParents(dir, stopAt string) {
	cleanStop := filepath.Clean(stopAt)
	for cur := filepath.Clean(dir); cur != cleanStop && isUnderPath(cur, cleanStop); cur = filepath.Dir(cur) {
		if !removeDirIfEmpty(cur) {
			break
		}
	}
}

func removeDirIfEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return false
	}
	return os.Remove(dir) == nil
}

func reconcileCanonicalFilteredTarget(canonicalRoot, skillName string, target SkillTarget, policy ResourcePolicy, installed, check bool) (string, error) {
	srcSkill := filepath.Join(canonicalRoot, skillName)
	dstLink := filepath.Join(target.Path, skillName)
	allowed := installed && nameAllowed(skillName, policy, target.Name)

	info, err := os.Lstat(dstLink)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	if allowed {
		if exists {
			if isManagedLink(info) && linkPointsTo(dstLink, srcSkill) {
				return "ok", nil
			}
			// Never overwrite an unowned directory/file inside a filtered skill root.
			return "conflict", nil
		}
		if check {
			return "would-link", nil
		}
		if err := symlinkCreateFunc(srcSkill, dstLink); err == nil {
			return "linked", nil
		} else if runtime.GOOS == "windows" {
			if jErr := junctionCreateFunc(srcSkill, dstLink); jErr == nil {
				return "linked", nil
			}
		} else {
			return "", err
		}
		return "linked", nil
	}

	if exists && isManagedLink(info) && linkPointsTo(dstLink, srcSkill) {
		if check {
			return "would-unlink", nil
		}
		if err := os.Remove(dstLink); err != nil {
			return "", err
		}
		return "unlinked", nil
	}
	return "", nil
}

// RunManagedSkillInstall installs or updates externally owned skills across
// installed agent runtimes (or the canonical store in canonical mode).
func RunManagedSkillInstall(opts ManagedSkillOptions) (ManagedSkillReport, error) {
	if err := validateManagedOwner(opts.Owner, true); err != nil {
		return ManagedSkillReport{}, err
	}
	names, err := validateManagedSkillNames(opts.Names)
	if err != nil {
		return ManagedSkillReport{}, err
	}
	bundles, err := discoverSourceSkills(opts.Source, names)
	if err != nil {
		return ManagedSkillReport{}, err
	}

	cfg, err := defaultGlobalConfig()
	if err != nil {
		return ManagedSkillReport{}, err
	}
	policy, err := loadSyncPolicy(cfg.PolicyPath)
	if err != nil {
		return ManagedSkillReport{}, err
	}
	placement, err := detectSkillPlacementMode(cfg, policy)
	if err != nil {
		return ManagedSkillReport{}, err
	}

	if !opts.Check {
		unlock, err := acquireManagedSkillsLock()
		if err != nil {
			return ManagedSkillReport{}, err
		}
		defer unlock()
	}

	manifest, manifestPath, err := loadManagedSkillsManifest()
	if err != nil {
		return ManagedSkillReport{}, err
	}

	report := ManagedSkillReport{
		OK:        true,
		Action:    "install",
		Mode:      placement.Mode,
		Owner:     strings.TrimSpace(opts.Owner),
		Check:     opts.Check,
		Actions:   []ManagedSkillAction{},
		Conflicts: []ManagedSkillAction{},
	}

	mutated := false
	nowStamp := time.Now().UTC().Format(time.RFC3339)

	for _, root := range placement.WriteRoots {
		for _, bundle := range bundles {
			destDir := filepath.Join(root.PhysicalRoot, bundle.Name)
			existingRec, hasRec := manifest.Entries[destDir]
			var recPtr *managedSkillRecord
			if hasRec {
				copyRec := existingRec
				recPtr = &copyRec
			}

			state, detail := inspectManagedDestination(destDir, report.Owner, bundle.Name, recPtr)
			var actionName string
			var outState string
			switch state {
			case "missing":
				actionName = "create"
				outState = "missing"
				if !opts.Check {
					if err := writeManagedSkillDir(destDir, bundle, nil); err != nil {
						return report, err
					}
					manifest.Entries[destDir] = managedSkillRecord{
						Owner:     report.Owner,
						Skill:     bundle.Name,
						Files:     bundle.Hashes,
						UpdatedAt: nowStamp,
					}
					mutated = true
					outState = "current"
				}
			case "owned":
				if fileMapsEqual(existingRec.Files, bundle.Hashes) {
					actionName = "unchanged"
					outState = "current"
				} else {
					actionName = "update"
					outState = "outdated"
					if !opts.Check {
						if err := writeManagedSkillDir(destDir, bundle, &existingRec); err != nil {
							return report, err
						}
						manifest.Entries[destDir] = managedSkillRecord{
							Owner:     report.Owner,
							Skill:     bundle.Name,
							Files:     bundle.Hashes,
							UpdatedAt: nowStamp,
						}
						mutated = true
						outState = "current"
					}
				}
			default:
				actionName = "conflict"
				outState = state
				report.OK = false
			}

			if placement.Mode == "canonical" && (actionName == "create" || actionName == "update" || actionName == "unchanged") {
				for _, ft := range placement.FilteredTargets {
					linkStatus, linkErr := reconcileCanonicalFilteredTarget(
						placement.CanonicalRoot,
						bundle.Name,
						ft,
						policy.Skills,
						true,
						opts.Check,
					)
					if linkErr != nil {
						return report, linkErr
					}
					if linkStatus == "conflict" {
						conflictEntry := ManagedSkillAction{
							Skill:    bundle.Name,
							Path:     filepath.Join(ft.Path, bundle.Name),
							Runtimes: []string{ft.Name},
							Action:   "conflict",
							State:    "conflict",
							Detail:   "existing unowned entry blocks filtered target symlink",
						}
						report.Conflicts = append(report.Conflicts, conflictEntry)
						report.OK = false
					}
				}
			}

			entry := ManagedSkillAction{
				Skill:    bundle.Name,
				Path:     destDir,
				Runtimes: append([]string(nil), root.Runtimes...),
				Action:   actionName,
				State:    outState,
				Detail:   detail,
			}
			report.Actions = append(report.Actions, entry)
			if actionName == "conflict" {
				report.Conflicts = append(report.Conflicts, entry)
			}
		}
	}

	if mutated && !opts.Check {
		if err := saveManagedSkillsManifest(manifestPath, manifest); err != nil {
			return report, err
		}
	}
	return report, nil
}

// RunManagedSkillRemove uninstalls unchanged owned skills previously recorded
// in managed-skills.json.
func RunManagedSkillRemove(opts ManagedSkillOptions) (ManagedSkillReport, error) {
	if err := validateManagedOwner(opts.Owner, true); err != nil {
		return ManagedSkillReport{}, err
	}
	names, err := validateManagedSkillNames(opts.Names)
	if err != nil {
		return ManagedSkillReport{}, err
	}
	nameFilter := map[string]struct{}{}
	for _, n := range names {
		nameFilter[n] = struct{}{}
	}

	cfg, err := defaultGlobalConfig()
	if err != nil {
		return ManagedSkillReport{}, err
	}
	policy, err := loadSyncPolicy(cfg.PolicyPath)
	if err != nil {
		return ManagedSkillReport{}, err
	}
	placement, err := detectSkillPlacementMode(cfg, policy)
	if err != nil {
		return ManagedSkillReport{}, err
	}

	if !opts.Check {
		unlock, err := acquireManagedSkillsLock()
		if err != nil {
			return ManagedSkillReport{}, err
		}
		defer unlock()
	}

	manifest, manifestPath, err := loadManagedSkillsManifest()
	if err != nil {
		return ManagedSkillReport{}, err
	}

	owner := strings.TrimSpace(opts.Owner)
	report := ManagedSkillReport{
		OK:        true,
		Action:    "remove",
		Mode:      placement.Mode,
		Owner:     owner,
		Check:     opts.Check,
		Actions:   []ManagedSkillAction{},
		Conflicts: []ManagedSkillAction{},
	}

	keys := make([]string, 0, len(manifest.Entries))
	for path, rec := range manifest.Entries {
		if rec.Owner != owner {
			continue
		}
		if len(nameFilter) > 0 {
			if _, ok := nameFilter[rec.Skill]; !ok {
				continue
			}
		}
		keys = append(keys, path)
	}
	sort.Strings(keys)

	runtimesByRoot := map[string][]string{}
	for _, root := range placement.WriteRoots {
		runtimesByRoot[root.PhysicalRoot] = root.Runtimes
	}

	mutated := false
	for _, destDir := range keys {
		rec := manifest.Entries[destDir]
		recCopy := rec
		state, detail := inspectManagedDestination(destDir, owner, rec.Skill, &recCopy)

		var actionName string
		switch state {
		case "missing":
			actionName = "forget"
			if !opts.Check {
				delete(manifest.Entries, destDir)
				mutated = true
			}
		case "owned":
			actionName = "remove"
			if !opts.Check {
				if err := removeManagedSkillFiles(destDir, rec); err != nil {
					return report, err
				}
				delete(manifest.Entries, destDir)
				mutated = true
			}
		default:
			actionName = "keep_modified"
			report.OK = false
		}

		if placement.Mode == "canonical" && (actionName == "remove" || actionName == "forget") {
			for _, ft := range placement.FilteredTargets {
				if _, err := reconcileCanonicalFilteredTarget(
					placement.CanonicalRoot,
					rec.Skill,
					ft,
					policy.Skills,
					false,
					opts.Check,
				); err != nil {
					return report, err
				}
			}
		}

		entry := ManagedSkillAction{
			Skill:    rec.Skill,
			Path:     destDir,
			Runtimes: append([]string(nil), runtimesByRoot[filepath.Dir(destDir)]...),
			Action:   actionName,
			State:    state,
			Detail:   detail,
		}
		report.Actions = append(report.Actions, entry)
		if actionName == "keep_modified" {
			report.Conflicts = append(report.Conflicts, entry)
		}
	}

	if mutated && !opts.Check {
		if err := saveManagedSkillsManifest(manifestPath, manifest); err != nil {
			return report, err
		}
	}
	return report, nil
}

// RunManagedSkillStatus inspects managed-skills.json and live disk hashes strictly
// read-only without creating directories or manifest files.
func RunManagedSkillStatus(opts ManagedSkillOptions) (ManagedSkillReport, error) {
	if err := validateManagedOwner(opts.Owner, false); err != nil {
		return ManagedSkillReport{}, err
	}
	names, err := validateManagedSkillNames(opts.Names)
	if err != nil {
		return ManagedSkillReport{}, err
	}
	nameFilter := map[string]struct{}{}
	for _, n := range names {
		nameFilter[n] = struct{}{}
	}

	cfg, err := defaultGlobalConfig()
	if err != nil {
		return ManagedSkillReport{}, err
	}
	policy, err := loadSyncPolicy(cfg.PolicyPath)
	if err != nil {
		return ManagedSkillReport{}, err
	}
	placement, err := detectSkillPlacementMode(cfg, policy)
	if err != nil {
		return ManagedSkillReport{}, err
	}

	manifest, _, err := loadManagedSkillsManifest()
	if err != nil {
		return ManagedSkillReport{}, err
	}

	owner := strings.TrimSpace(opts.Owner)
	report := ManagedSkillReport{
		OK:        true,
		Action:    "status",
		Mode:      placement.Mode,
		Owner:     owner,
		Check:     true,
		Actions:   []ManagedSkillAction{},
		Conflicts: []ManagedSkillAction{},
	}

	runtimesByRoot := map[string][]string{}
	for _, root := range placement.WriteRoots {
		runtimesByRoot[root.PhysicalRoot] = root.Runtimes
	}

	keys := make([]string, 0, len(manifest.Entries))
	for path, rec := range manifest.Entries {
		if owner != "" && rec.Owner != owner {
			continue
		}
		if len(nameFilter) > 0 {
			if _, ok := nameFilter[rec.Skill]; !ok {
				continue
			}
		}
		keys = append(keys, path)
	}
	sort.Strings(keys)

	for _, destDir := range keys {
		rec := manifest.Entries[destDir]
		recCopy := rec
		state, detail := inspectManagedDestination(destDir, rec.Owner, rec.Skill, &recCopy)
		outState := state
		if state == "owned" {
			outState = "current"
		} else {
			report.OK = false
		}
		entry := ManagedSkillAction{
			Skill:    rec.Skill,
			Path:     destDir,
			Runtimes: append([]string(nil), runtimesByRoot[filepath.Dir(destDir)]...),
			Action:   "status",
			State:    outState,
			Detail:   detail,
		}
		report.Actions = append(report.Actions, entry)
		if outState != "current" {
			report.Conflicts = append(report.Conflicts, entry)
		}
	}
	return report, nil
}
