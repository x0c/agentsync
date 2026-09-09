package agentsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

var (
	watchPollInterval = 2 * time.Second
	watchDebounce     = 1500 * time.Millisecond
	watchDetectSettle = 5 * time.Second
)

type watchSnapshot struct {
	Agents string
	MCP    string
	Skills string
	Detect string
}

func (a watchSnapshot) equal(b watchSnapshot) bool {
	return a == b
}

func watchSkipMCP(prev, next watchSnapshot) bool {
	return prev.MCP == next.MCP && prev.Detect == next.Detect
}

func runWatch(opts Options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return watchLoop(ctx, opts)
}

func watchLoop(ctx context.Context, opts Options) error {
	cfg, err := defaultGlobalConfig()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "agentsync watch: polling %s every %s\n", filepath.Dir(cfg.Source), watchPollInterval)
	last := watchSnapshot{}
	if err := watchApply(ctx, cfg, opts, last, "initial sync"); err != nil {
		fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
	} else if snap, err := watchSnapshotOf(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
	} else {
		last = snap
	}
	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "agentsync watch: stopped")
			return nil
		case <-ticker.C:
			next, err := watchSnapshotOf(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
				continue
			}
			if next.equal(last) {
				continue
			}
			stable, err := waitUntilStable(ctx, cfg, next, watchDebounce)
			if err != nil {
				if ctx.Err() != nil {
					fmt.Fprintln(os.Stderr, "agentsync watch: stopped")
					return nil
				}
				fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
				continue
			}
			if !stable {
				continue
			}
			next, err = watchSnapshotOf(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
				continue
			}
			if next.equal(last) {
				continue
			}
			if next.Detect != last.Detect {
				stable, err = waitUntilStable(ctx, cfg, next, watchDetectSettle)
				if err != nil {
					if ctx.Err() != nil {
						fmt.Fprintln(os.Stderr, "agentsync watch: stopped")
						return nil
					}
					fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
					continue
				}
				if !stable {
					continue
				}
				next, err = watchSnapshotOf(cfg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
					continue
				}
				if next.equal(last) {
					continue
				}
			}
			reason := "canonical source or installed runtime changed"
			if watchSkipMCP(last, next) {
				reason = "rules or skills changed"
			}
			if err := watchApply(ctx, cfg, opts, last, reason); err != nil {
				fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
				continue
			}
			snap, err := watchSnapshotOf(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
				continue
			}
			last = snap
		}
	}
}

func watchApply(ctx context.Context, cfg Config, opts Options, prev watchSnapshot, reason string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	next, err := watchSnapshotOf(cfg)
	if err != nil {
		return err
	}
	applyOpts := opts
	applyOpts.SkipMCP = watchSkipMCP(prev, next)
	fmt.Fprintf(os.Stderr, "agentsync watch: %s\n", reason)
	return runOnce(applyOpts)
}

func waitUntilStable(ctx context.Context, cfg Config, want watchSnapshot, d time.Duration) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(d):
	}
	got, err := watchSnapshotOf(cfg)
	if err != nil {
		return false, err
	}
	return got.equal(want), nil
}

func watchSnapshotOf(cfg Config) (watchSnapshot, error) {
	var snap watchSnapshot
	var err error
	snap.Agents, err = pathFingerprint(cfg.Source)
	if err != nil {
		return watchSnapshot{}, err
	}
	snap.MCP, err = pathFingerprint(cfg.MCPSource)
	if err != nil {
		return watchSnapshot{}, err
	}
	snap.Skills, err = skillsFingerprint(cfg.SkillSource)
	if err != nil {
		return watchSnapshot{}, err
	}
	snap.Detect = detectFingerprint(cfg)
	return snap, nil
}

func pathFingerprint(path string) (string, error) {
	h := sha256.New()
	if err := addPathFingerprint(h, path); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func skillsFingerprint(root string) (string, error) {
	if root == "" {
		return "", nil
	}
	if !pathExists(root) {
		return "skills-missing", nil
	}
	return watchTreeFingerprint(root)
}

func detectFingerprint(cfg Config) string {
	h := sha256.New()
	for _, detect := range watchDetectDirs(cfg) {
		h.Write([]byte(detect))
		h.Write([]byte{0})
		if pathExists(detect) {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func watchDetectDirs(cfg Config) []string {
	seen := map[string]struct{}{}
	detects := make([]string, 0)
	add := func(dir string) {
		if dir == "" {
			return
		}
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		detects = append(detects, dir)
	}
	for _, t := range cfg.Targets {
		add(t.Detect)
	}
	for _, t := range cfg.SkillTargets {
		add(t.Detect)
	}
	for _, t := range cfg.MCPTargets {
		add(t.Detect)
	}
	sort.Strings(detects)
	return detects
}

func addPathFingerprint(h hash.Hash, path string) error {
	if path == "" {
		h.Write([]byte{0})
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			h.Write([]byte("missing:"))
			h.Write([]byte(path))
			return nil
		}
		return err
	}
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write(data)
	return nil
}

func watchTreeFingerprint(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if p != root && watchShouldSkipName(name) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// 只指纹文件。目录 mtime 会因被跳过的冲突文件而变化，不能算进哈希。
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %d %d %d\n", rel, info.Mode(), info.ModTime().UnixNano(), info.Size())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func watchShouldSkipName(name string) bool {
	if strings.Contains(name, ".sync-conflict") || strings.HasPrefix(name, ".syncthing.") {
		return true
	}
	switch name {
	case ".DS_Store", ".git", ".stfolder", ".stversions", ".gitignore", ".stignore":
		return true
	default:
		// 保留 .system 等隐藏 skill 目录，它们是统一源的真实内容。
		return false
	}
}
