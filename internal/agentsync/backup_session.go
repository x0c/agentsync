package agentsync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const backupManifestName = "manifest.json"

var stampDirPattern = regexp.MustCompile(`^\d{8}-\d{6}(-\d{2})?$`)

type backupManifest struct {
	Version int                   `json:"version"`
	Stamp   string                `json:"stamp"`
	Entries []backupManifestEntry `json:"entries"`
}

type backupManifestEntry struct {
	Original string `json:"original"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // file | symlink | dir
}

type backupSession struct {
	stamp   string
	dir     string
	entries []backupManifestEntry
}

var (
	backupSessionMu sync.Mutex
	activeBackup    *backupSession
)

func createUniqueStampDir(root string) (stamp, dir string, err error) {
	base := time.Now().Format("20060102-150405")
	for n := 0; n < 100; n++ {
		stamp = base
		if n > 0 {
			stamp = fmt.Sprintf("%s-%02d", base, n)
		}
		dir = filepath.Join(root, stamp)
		err = os.Mkdir(dir, 0o755)
		if err == nil {
			return stamp, dir, nil
		}
		if !os.IsExist(err) {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("could not allocate unique backup stamp under %s", root)
}

func beginBackupSession() (string, error) {
	backupSessionMu.Lock()
	defer backupSessionMu.Unlock()
	if activeBackup != nil {
		return "", fmt.Errorf("backup session already active")
	}
	root, err := backupDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	stamp, dir, err := createUniqueStampDir(root)
	if err != nil {
		return "", err
	}
	activeBackup = &backupSession{stamp: stamp, dir: dir}
	return stamp, nil
}

func endBackupSession() error {
	backupSessionMu.Lock()
	defer backupSessionMu.Unlock()
	if activeBackup == nil {
		return nil
	}
	session := activeBackup
	activeBackup = nil
	if len(session.entries) == 0 {
		// Remove only an empty directory we allocated, never unrecorded/manual payloads.
		return os.Remove(session.dir)
	}
	return writeBackupManifestLocked(session)
}

// backupStampDir returns the directory that new backups should write into.
// When a sync session is active, every backup in that run shares one stamp.
func backupStampDir() (string, error) {
	backupSessionMu.Lock()
	defer backupSessionMu.Unlock()
	if activeBackup != nil {
		return activeBackup.dir, nil
	}
	root, err := backupDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	_, dir, err := createUniqueStampDir(root)
	return dir, err
}

// backupPayloadName preserves the first snapshot of each absolute original per session.
func backupPayloadName(stampDir, original, kind string) (name string, existing bool, err error) {
	abs, err := filepath.Abs(original)
	if err != nil {
		return "", false, err
	}
	name = fmt.Sprintf("entry-%x.%s", sha256.Sum256([]byte(abs)), kind)

	backupSessionMu.Lock()
	defer backupSessionMu.Unlock()
	var entries []backupManifestEntry
	if activeBackup != nil && samePath(activeBackup.dir, stampDir) {
		entries = activeBackup.entries
	} else {
		manifest, err := loadBackupManifest(stampDir)
		if err != nil && !os.IsNotExist(err) {
			return "", false, err
		}
		entries = manifest.Entries
	}
	for _, entry := range entries {
		if samePath(entry.Original, abs) {
			if _, err := validateBackupPayload(stampDir, entry); err != nil {
				return "", false, err
			}
			return entry.Name, true, nil
		}
		if entry.Name == name {
			return "", false, fmt.Errorf("backup payload %q already belongs to %s", name, entry.Original)
		}
	}
	return name, false, nil
}

func validateBackupEntryOwnership(entries []backupManifestEntry, entry backupManifestEntry) error {
	for _, existing := range entries {
		if samePath(existing.Original, entry.Original) {
			if existing.Name != entry.Name || existing.Kind != entry.Kind {
				return fmt.Errorf("path already backed up in this session: %s", entry.Original)
			}
		} else if existing.Name == entry.Name {
			return fmt.Errorf("backup payload %q already belongs to %s", entry.Name, existing.Original)
		}
	}
	return nil
}

func recordBackupEntryAt(stampDir, original, name, kind string) error {
	abs, err := filepath.Abs(original)
	if err != nil {
		return err
	}
	entry := backupManifestEntry{Original: abs, Name: name, Kind: kind}

	backupSessionMu.Lock()
	defer backupSessionMu.Unlock()
	if activeBackup != nil && samePath(activeBackup.dir, stampDir) {
		if err := validateBackupEntryOwnership(activeBackup.entries, entry); err != nil {
			return err
		}
		next := *activeBackup
		next.entries = upsertBackupEntry(append([]backupManifestEntry(nil), activeBackup.entries...), entry)
		if err := writeBackupManifestLocked(&next); err != nil {
			return err
		}
		activeBackup.entries = next.entries
		return nil
	}
	return mergeBackupManifestFile(stampDir, entry)
}

func writeBackupManifestLocked(session *backupSession) error {
	manifest := backupManifest{
		Version: 1,
		Stamp:   session.stamp,
		Entries: append([]backupManifestEntry(nil), session.entries...),
	}
	return writeBackupManifestFile(session.dir, manifest)
}

func mergeBackupManifestFile(stampDir string, entry backupManifestEntry) error {
	manifest, err := loadBackupManifest(stampDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		manifest = backupManifest{Version: 1, Stamp: filepath.Base(stampDir)}
	}
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	if manifest.Stamp == "" {
		manifest.Stamp = filepath.Base(stampDir)
	}
	if err := validateBackupEntryOwnership(manifest.Entries, entry); err != nil {
		return err
	}
	manifest.Entries = upsertBackupEntry(manifest.Entries, entry)
	return writeBackupManifestFile(stampDir, manifest)
}

func writeBackupManifestFile(stampDir string, manifest backupManifest) error {
	if err := os.MkdirAll(stampDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(filepath.Join(stampDir, backupManifestName), data, 0o644)
}

func loadBackupManifest(stampDir string) (backupManifest, error) {
	data, err := os.ReadFile(filepath.Join(stampDir, backupManifestName))
	if err != nil {
		return backupManifest{}, err
	}
	var manifest backupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return backupManifest{}, err
	}
	if manifest.Stamp == "" {
		manifest.Stamp = filepath.Base(stampDir)
	}
	return manifest, nil
}

func upsertBackupEntry(entries []backupManifestEntry, entry backupManifestEntry) []backupManifestEntry {
	for _, existing := range entries {
		if samePath(existing.Original, entry.Original) {
			return entries
		}
	}
	return append(entries, entry)
}

func listBackupStamps() ([]string, error) {
	root, err := backupDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var stamps []string
	for _, entry := range entries {
		if !entry.IsDir() || !stampDirPattern.MatchString(entry.Name()) {
			continue
		}
		stamps = append(stamps, entry.Name())
	}
	sort.Strings(stamps)
	return stamps, nil
}

func latestBackupStamp() (string, error) {
	stamps, err := listBackupStamps()
	if err != nil {
		return "", err
	}
	root, err := backupDir()
	if err != nil {
		return "", err
	}
	for i := len(stamps) - 1; i >= 0; i-- {
		stampDir := filepath.Join(root, stamps[i])
		manifest, err := loadBackupManifest(stampDir)
		if err != nil || len(manifest.Entries) == 0 || validateRestoreManifest(manifest) != nil {
			continue
		}
		valid := true
		for _, entry := range manifest.Entries {
			if _, _, err := validateRestoreEntry(stampDir, root, entry); err != nil {
				valid = false
				break
			}
		}
		if valid {
			return stamps[i], nil
		}
	}
	return "", fmt.Errorf("no valid nonempty backup manifests found under backups/; older backups may require manual restoration")
}

func resolveBackupStamp(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "latest" {
		return latestBackupStamp()
	}
	if stampDirPattern.MatchString(spec) {
		root, err := backupDir()
		if err != nil {
			return "", err
		}
		dir := filepath.Join(root, spec)
		info, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("backup stamp %s not found: %w", spec, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("backup stamp %s is not a directory", spec)
		}
		return spec, nil
	}
	abs, err := filepath.Abs(spec)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("backup stamp %s not found: %w", spec, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("backup stamp %s is not a directory", spec)
	}
	name := filepath.Base(abs)
	if !stampDirPattern.MatchString(name) {
		return "", fmt.Errorf("invalid backup stamp %q (expected YYYYMMDD-HHMMSS or latest)", spec)
	}
	root, err := backupDir()
	if err != nil {
		return "", err
	}
	if !samePath(filepath.Dir(abs), root) {
		return "", fmt.Errorf("backup stamp path must be under backups/")
	}
	return name, nil
}

func stampDirFor(stamp string) (string, error) {
	root, err := backupDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, stamp), nil
}
