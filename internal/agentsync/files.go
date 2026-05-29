package agentsync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const managedMarkerPrefix = "<!-- managed-by: agentsync "

func ensureParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func readPayload(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(string(data), managedMarkerPrefix) {
		if idx := strings.Index(string(data), "-->\n"); idx >= 0 {
			return data[idx+4:], nil
		}
	}
	return data, nil
}

func sameContent(a, b string) bool {
	aa, err := readPayload(a)
	if err != nil {
		return false
	}
	bb, err := readPayload(b)
	if err != nil {
		return false
	}
	return string(aa) == string(bb)
}

func writeManagedCopy(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	body := fmt.Sprintf("<!-- managed-by: agentsync source=%q sha256=%s -->\n", source, hex.EncodeToString(sum[:]))
	body += string(data)
	if err := ensureParent(target); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(body), 0o644)
}

func createAlias(source, target, mode string) (string, error) {
	if err := ensureParent(target); err != nil {
		return "", err
	}
	linkTarget := source
	if mode == "relative-link" {
		rel, err := filepath.Rel(filepath.Dir(target), source)
		if err == nil {
			linkTarget = rel
		}
	}
	if err := os.Symlink(linkTarget, target); err == nil {
		return "symlink", nil
	}
	if runtime.GOOS == "windows" {
		if err := os.Link(source, target); err == nil {
			return "hardlink", nil
		}
	}
	if err := writeManagedCopy(source, target); err != nil {
		return "", err
	}
	return "copy", nil
}

func removePath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("refusing to remove directory %s", path)
	}
	return os.Remove(path)
}

func backupFile(path string) (string, error) {
	if !pathExists(path) {
		return "", nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("refusing to back up directory %s", path)
	}
	dir, err := backupDir()
	if err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	name := sanitizePath(path)
	dst := filepath.Join(dir, stamp, name)
	if err := ensureParent(dst); err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(dst+".symlink", []byte(target+"\n"), 0o644); err != nil {
			return "", err
		}
		return dst + ".symlink", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return "", err
	}
	return dst, nil
}

func backupAny(path string) (string, error) {
	if !pathExists(path) {
		return "", nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return backupFile(path)
	}
	dir, err := backupDir()
	if err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	dst := filepath.Join(dir, stamp, sanitizePath(path))
	if err := copyDir(path, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := ensureParent(target); err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	if err := ensureParent(dst); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func dirDigest(path string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write([]byte(info.Mode().String()))
		h.Write([]byte{0})
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			h.Write([]byte(target))
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sanitizePath(path string) string {
	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, string(filepath.VolumeName(path)))
	path = strings.TrimLeft(path, string(filepath.Separator))
	replacer := strings.NewReplacer("/", "__", "\\", "__", ":", "_")
	return replacer.Replace(path)
}

func symlinkPointsTo(path, source string) bool {
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if filepath.IsAbs(target) {
		return samePath(target, source)
	}
	return samePath(filepath.Join(filepath.Dir(path), target), source)
}
