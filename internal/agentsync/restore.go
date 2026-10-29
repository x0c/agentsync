package agentsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runRollback(opts Options) (runErr error) {
	if opts.Watch || opts.Repo || opts.All != "" || opts.Adopt != "" || opts.Force {
		return fmt.Errorf("--rollback cannot be combined with --watch, --repo, --all, --adopt, or --force")
	}
	stamp, err := resolveBackupStamp(opts.Rollback)
	if err != nil {
		return err
	}
	stampDir, err := stampDirFor(stamp)
	if err != nil {
		return err
	}
	manifest, err := loadBackupManifest(stampDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup stamp %s has no manifest.json (created before rollback support); restore manually from %s", stamp, stampDir)
		}
		return err
	}
	if len(manifest.Entries) == 0 {
		return fmt.Errorf("backup stamp %s has an empty manifest", stamp)
	}
	if err := validateRestoreManifest(manifest); err != nil {
		return err
	}

	report := RunReport{Source: "rollback " + stamp}
	backupsRoot, err := backupDir()
	if err != nil {
		return err
	}
	// Validate all entries before a malformed later entry can cause a partial restore.
	for _, entry := range manifest.Entries {
		abs, _, err := validateRestoreEntry(stampDir, backupsRoot, entry)
		if err != nil {
			return err
		}
		if err := validateRestoreDestination(abs); err != nil {
			return err
		}
	}

	if !opts.Check {
		if _, err := beginBackupSession(); err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, endBackupSession()) }()
	}

	for _, entry := range manifest.Entries {
		result, backup, err := restoreManifestEntry(stampDir, backupsRoot, entry, opts.Check)
		if err != nil {
			return err
		}
		report.Results = append(report.Results, result)
		if backup != "" {
			report.Backups = append(report.Backups, backup)
		}
	}
	printReport(report, opts)
	if !opts.Check {
		fmt.Println("Next:")
		fmt.Println("  Review restored files. Run agentsync again only if you want to re-apply the canonical source.")
		fmt.Println()
	}
	return nil
}

func restoreManifestEntry(stampDir, backupsRoot string, entry backupManifestEntry, check bool) (TargetResult, string, error) {
	result := TargetResult{Path: entry.Original}
	abs, stored, err := validateRestoreEntry(stampDir, backupsRoot, entry)
	if err != nil {
		return result, "", err
	}
	if err := validateRestoreDestination(abs); err != nil {
		return result, "", err
	}

	if check {
		result.Status = "restore"
		result.Detail = fmt.Sprintf("would restore %s from stamp", entry.Kind)
		return result, "", nil
	}

	var backup string
	if pathExists(abs) {
		backup, err = backupAny(abs)
		if err != nil {
			return result, "", err
		}
	}

	if err := removeExistingForRestore(abs); err != nil {
		return result, backup, err
	}
	if err := ensureParent(abs); err != nil {
		return result, backup, err
	}

	switch entry.Kind {
	case "symlink":
		data, err := os.ReadFile(stored)
		if err != nil {
			return result, backup, err
		}
		target := strings.TrimSpace(string(data))
		if err := os.Symlink(target, abs); err != nil {
			return result, backup, err
		}
	case "dir":
		if err := copyDir(stored, abs); err != nil {
			return result, backup, err
		}
	case "file":
		info, err := os.Stat(stored)
		if err != nil {
			return result, backup, err
		}
		if err := copyFile(stored, abs, info.Mode().Perm()); err != nil {
			return result, backup, err
		}
	}
	result.Status = "restored"
	result.Detail = entry.Kind
	return result, backup, nil
}

func validateRestoreManifest(manifest backupManifest) error {
	if manifest.Version < 0 || manifest.Version > 1 {
		return fmt.Errorf("unsupported backup manifest version %d", manifest.Version)
	}
	names, originals := map[string]string{}, map[string]bool{}
	for _, entry := range manifest.Entries {
		if original, exists := names[entry.Name]; exists && original != entry.Original {
			return fmt.Errorf("backup payload %q is shared by multiple original paths; restore manually from an intact backup", entry.Name)
		}
		original := filepath.Clean(entry.Original)
		if originals[original] {
			return fmt.Errorf("duplicate original path in backup manifest: %s", original)
		}
		names[entry.Name], originals[original] = entry.Original, true
	}
	return nil
}

func validateRestoreEntry(stampDir, backupsRoot string, entry backupManifestEntry) (abs, stored string, err error) {
	if entry.Original == "" || entry.Name == "" || entry.Kind == "" {
		return "", "", fmt.Errorf("invalid manifest entry: %+v", entry)
	}
	if !filepath.IsAbs(entry.Original) {
		return "", "", fmt.Errorf("refusing to restore non-absolute path %s", entry.Original)
	}
	abs = filepath.Clean(entry.Original)
	if samePath(abs, backupsRoot) || isUnderPath(abs, backupsRoot) {
		return "", "", fmt.Errorf("refusing to restore path inside backups/: %s", abs)
	}
	stored, err = validateBackupPayload(stampDir, entry)
	return abs, stored, err
}

func validateBackupPayload(stampDir string, entry backupManifestEntry) (string, error) {
	if strings.Contains(entry.Name, "..") || filepath.IsAbs(entry.Name) {
		return "", fmt.Errorf("invalid backup entry name %q", entry.Name)
	}
	stored := filepath.Join(stampDir, entry.Name)
	info, err := os.Lstat(stored)
	if err != nil {
		return "", fmt.Errorf("backup payload unavailable: %s: %w", stored, err)
	}
	switch entry.Kind {
	case "file", "symlink":
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("backup payload is not a regular file: %s", stored)
		}
		data, err := os.ReadFile(stored)
		if err != nil {
			return "", err
		}
		if entry.Kind == "symlink" && strings.TrimSpace(string(data)) == "" {
			return "", fmt.Errorf("empty symlink target in %s", stored)
		}
	case "dir":
		if !info.IsDir() {
			return "", fmt.Errorf("backup payload is not a directory: %s", stored)
		}
		if err := filepath.WalkDir(stored, func(_ string, _ os.DirEntry, err error) error { return err }); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown backup kind %q for %s", entry.Kind, entry.Original)
	}
	return stored, nil
}

func validateRestoreDestination(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("unsupported current entry type at %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("restore parent is not a directory: %s", parent)
			}
			return nil
		}
		if !os.IsNotExist(err) || filepath.Dir(parent) == parent {
			return err
		}
	}
}

func removeExistingForRestore(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func isUnderPath(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
