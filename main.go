package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/x0c/agentsync/internal/agentsync"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stderr))
}

func runCLI(args []string, stderr io.Writer) int {
	opts := agentsync.Options{}
	flags := flag.NewFlagSet("agentsync", flag.ContinueOnError)
	flags.SetOutput(stderr)

	flags.BoolVar(&opts.Check, "check", false, "only inspect; do not modify files")
	flags.BoolVar(&opts.Repo, "repo", false, "manage AGENTS.md aliases in the current repository")
	flags.StringVar(&opts.All, "all", "", "scan and manage every repository under this directory")
	flags.StringVar(&opts.Adopt, "adopt", "", "adopt a reviewed merge draft as the canonical source")
	flags.StringVar(&opts.Rollback, "rollback", "", "restore a backup stamp: latest or YYYYMMDD-HHMMSS")
	flags.BoolVar(&opts.Force, "force", false, "replace conflicting files after backing them up")
	flags.BoolVar(&opts.Watch, "watch", false, "keep running and sync when the canonical source or an installed runtime changes")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return cliDiagnostic(stderr, 2, "unexpected positional arguments %q; use flags only (see --help)", flags.Args())
	}
	if err := opts.Validate(); err != nil {
		return cliDiagnostic(stderr, 2, "%s", err)
	}
	if err := agentsync.Run(opts); err != nil {
		return cliDiagnostic(stderr, 1, "%s", err)
	}
	return 0
}

func cliDiagnostic(stderr io.Writer, code int, format string, args ...any) int {
	if _, err := fmt.Fprintf(stderr, "agentsync: "+format+"\n", args...); err != nil {
		return 1
	}
	return code
}
