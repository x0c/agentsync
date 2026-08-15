package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/x0c/agentsync/internal/agentsync"
)

func main() {
	opts := agentsync.Options{}

	flag.BoolVar(&opts.Check, "check", false, "only inspect; do not modify files")
	flag.BoolVar(&opts.Repo, "repo", false, "manage AGENTS.md aliases in the current repository")
	flag.StringVar(&opts.All, "all", "", "scan and manage every repository under this directory")
	flag.StringVar(&opts.Adopt, "adopt", "", "adopt a reviewed merge draft as the canonical source")
	flag.BoolVar(&opts.Force, "force", false, "replace conflicting files after backing them up")
	flag.BoolVar(&opts.Watch, "watch", false, "keep running and sync when the canonical source or an installed runtime changes")
	flag.Parse()

	if err := agentsync.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "agentsync:", err)
		os.Exit(1)
	}
}
