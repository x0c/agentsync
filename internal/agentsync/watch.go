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
	"syscall"
	"time"
)

var (
	watchPollInterval = 2 * time.Second
	watchDebounce     = 400 * time.Millisecond
)

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
	apply := func(reason string) {
		fmt.Fprintf(os.Stderr, "agentsync watch: %s\n", reason)
		if err := runOnce(opts); err != nil {
			fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
		}
	}
	apply("initial sync")
	last, err := watchFingerprint(cfg)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "agentsync watch: stopped")
			return nil
		case <-ticker.C:
			fp, err := watchFingerprint(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
				continue
			}
			if fp == last {
				continue
			}
			select {
			case <-ctx.Done():
				fmt.Fprintln(os.Stderr, "agentsync watch: stopped")
				return nil
			case <-time.After(watchDebounce):
			}
			apply("canonical source or installed runtime changed")
			last, err = watchFingerprint(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "agentsync watch: %v\n", err)
			}
		}
	}
}

func watchFingerprint(cfg Config) (string, error) {
	h := sha256.New()
	if err := addPathFingerprint(h, cfg.Source); err != nil {
		return "", err
	}
	if err := addPathFingerprint(h, cfg.MCPSource); err != nil {
		return "", err
	}
	if cfg.SkillSource != "" {
		if pathExists(cfg.SkillSource) {
			digest, err := dirDigest(cfg.SkillSource)
			if err != nil {
				return "", err
			}
			h.Write([]byte(digest))
		} else {
			h.Write([]byte("skills-missing"))
		}
	}
	for _, detect := range watchDetectDirs(cfg) {
		h.Write([]byte(detect))
		h.Write([]byte{0})
		if pathExists(detect) {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
