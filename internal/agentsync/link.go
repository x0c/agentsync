package agentsync

import (
	"fmt"
	"os"
	"path/filepath"
)

// symlinkCreateFunc creates a symbolic link. It is a variable so tests can
// simulate "symbolic link not permitted" on every OS without toggling Windows
// Developer Mode.
var symlinkCreateFunc = os.Symlink

// junctionCreateFunc creates an NTFS directory junction. Overridable in tests
// to simulate junction success or failure without a real Windows volume.
var junctionCreateFunc = createJunction

// isManagedLink reports whether fi describes a link agentsync manages: a
// symbolic link or, on Windows since Go 1.23, a directory junction reported
// as ModeIrregular. Use this everywhere instead of checking only
// os.ModeSymlink so junctions are recognized for alias, replacement, backup,
// rollback, watch and overlap decisions.
func isManagedLink(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}
	mode := fi.Mode()
	return mode&os.ModeSymlink != 0 || mode&os.ModeIrregular != 0
}

// linkPointsTo reports whether path is a symlink or junction whose target
// resolves to source. os.Readlink handles both reparse tags on Windows, so
// this single helper covers junctions as well as symlinks.
func linkPointsTo(path, source string) bool {
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if filepath.IsAbs(target) {
		return samePath(target, source)
	}
	return samePath(filepath.Join(filepath.Dir(path), target), source)
}

// symlinkPointsTo is kept for existing callers and tests; it covers junctions
// too because os.Readlink resolves junction reparse points on Windows.
func symlinkPointsTo(path, source string) bool {
	return linkPointsTo(path, source)
}

// isDirectorySource reports whether source exists and is a directory.
func isDirectorySource(source string) bool {
	info, err := os.Stat(source)
	return err == nil && info.IsDir()
}

// developerModeError is the shared contract returned when both symlink and
// junction creation fail for a directory on Windows.
func developerModeError(target string, symlinkErr, junctionErr error) error {
	return fmt.Errorf("cannot create skill directory alias for %s: symbolic link failed (%v) and junction failed (%v); enable Windows Developer Mode (Settings > Privacy & security > For developers, run 'start ms-settings:developers') or use an elevated prompt, then retry; junctions also require an NTFS volume", target, symlinkErr, junctionErr)
}

// removeLinkOrDir removes a managed link with os.Remove (never touching its
// target) or a real directory tree with os.RemoveAll.
func removeLinkOrDir(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if isManagedLink(fi) {
		return os.Remove(path)
	}
	if fi.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// linkKind returns the backup manifest kind for a managed link.
func linkKind(fi os.FileInfo) string {
	if fi.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	return "junction"
}
