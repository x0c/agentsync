package agentsync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runRollback(opts Options) error {
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

	report := RunReport{Source: "rollback " + stamp}
	backupsRoot, err := backupDir()
	if err != nil {
		return err
	}

	if !opts.Check {
		if _, err := beginBackupSession(); err != nil {
			return err
		}
		defer func() {
			_ = endBackupSession()
		}()
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
	if entry.Original == "" || entry.Name == "" || entry.Kind == "" {
		return result, "", fmt.Errorf("invalid manifest entry: %+v", entry)
	}
	abs, err := filepath.Abs(entry.Original)
	if err != nil {
		return result, "", err
	}
	if !filepath.IsAbs(abs) {
		return result, "", fmt.Errorf("refusing to restore non-absolute path %s", entry.Original)
	}
	if samePath(abs, backupsRoot) || isUnderPath(abs, backupsRoot) {
		return result, "", fmt.Errorf("refusing to restore path inside backups/: %s", abs)
	}
	if strings.Contains(entry.Name, "..") || filepath.IsAbs(entry.Name) {
		return result, "", fmt.Errorf("invalid backup entry name %q", entry.Name)
	}
	stored := filepath.Join(stampDir, entry.Name)
	if !pathExists(stored) {
		return result, "", fmt.Errorf("backup payload missing: %s", stored)
	}

	switch entry.Kind {
	case "file", "symlink", "dir":
	default:
		return result, "", fmt.Errorf("unknown backup kind %q for %s", entry.Kind, abs)
	}

	if check {
		result.Status = "restore"
		result.Detail = fmt.Sprintf("would restore %s from stamp", entry.Kind)
		return result, "", nil
	}

	var backup string
	if pathExists(abs) {
		switch entry.Kind {
		case "dir":
			backup, err = backupAny(abs)
		default:
			backup, err = backupFile(abs)
		}
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
		if target == "" {
			return result, backup, fmt.Errorf("empty symlink target in %s", stored)
		}
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
	return rel != "." && !strings.HasPrefix(rel, "..")
}
