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

const cursorFrontmatter = `---
description: agentsync global instructions
alwaysApply: true
---
`

func readPayload(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(data)
	// Cursor 受管规则：YAML frontmatter 后紧跟 managed marker 时，剥掉 frontmatter 再比正文。
	if strings.HasPrefix(s, "---\n") {
		if body, ok := stripYAMLFrontmatter(s); ok && strings.HasPrefix(body, managedMarkerPrefix) {
			s = body
		}
	}
	if strings.HasPrefix(s, managedMarkerPrefix) {
		if idx := strings.Index(s, "-->\n"); idx >= 0 {
			return []byte(s[idx+4:]), nil
		}
	}
	return []byte(s), nil
}

func stripYAMLFrontmatter(s string) (string, bool) {
	if !strings.HasPrefix(s, "---\n") {
		return "", false
	}
	rest := s[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", false
	}
	return rest[idx+5:], true
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

// isManagedAgentsyncFile 判断目标是否为 agentsync 自己写出的受管副本
// （含 Cursor .mdc：frontmatter + managed marker）。这类文件是统一源的衍生品，
// 过期时应以统一源为准直接替换，禁止再把旧正文 append 回统一源。
func isManagedAgentsyncFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(data)
	if strings.HasPrefix(s, "---\n") {
		if body, ok := stripYAMLFrontmatter(s); ok {
			s = body
		}
	}
	return strings.HasPrefix(s, managedMarkerPrefix)
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

func writeCursorRule(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	var b strings.Builder
	b.WriteString(cursorFrontmatter)
	b.WriteString(fmt.Sprintf("<!-- managed-by: agentsync source=%q sha256=%s -->\n", source, hex.EncodeToString(sum[:])))
	b.Write(data)
	if err := ensureParent(target); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(b.String()), 0o644)
}

func createAlias(source, target, mode string) (string, error) {
	if err := ensureParent(target); err != nil {
		return "", err
	}
	// Cursor 全局规则需要 alwaysApply frontmatter，不能直接 symlink 裸 AGENTS.md。
	if mode == "cursor" {
		if err := writeCursorRule(source, target); err != nil {
			return "", err
		}
		return "cursor-rule", nil
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

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := ensureParent(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agentsync-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
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
