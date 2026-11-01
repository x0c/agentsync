package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/x0c/agentsync/internal/agentsync"
)

// version is the released version, injected at build time:
//
//	-ldflags "-X main.version={{.Version}}"
//
// Plain `go build` checkouts leave it empty; those fall back to the module
// version recorded in the build info (`go install module@version`), else "dev".
var version = ""

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stderr, os.Stdout))
}

func runCLI(args []string, stderr, stdout io.Writer) int {
	opts := agentsync.Options{}
	flags := flag.NewFlagSet("agentsync", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var showVersion bool
	flags.BoolVar(&showVersion, "version", false, "print the agentsync version and exit")
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
	// --version wins over every other flag and positional argument: it only
	// prints the version and never starts a synchronization.
	if showVersion {
		if _, err := fmt.Fprintf(stdout, "agentsync %s\n", resolvedVersion()); err != nil {
			return 1
		}
		return 0
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

// displayVersion selects the version string: the ldflags value wins, then the
// module version from the build info, then "dev". Go records "(devel)" when
// the main module has no version stamp; that carries no version information,
// so it maps to "dev" as well.
func displayVersion(ldflagsVersion, buildVersion string) string {
	if ldflagsVersion != "" {
		return ldflagsVersion
	}
	if buildVersion != "" && buildVersion != "(devel)" {
		return buildVersion
	}
	return "dev"
}

func resolvedVersion() string {
	buildVersion := ""
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		buildVersion = info.Main.Version
	}
	return displayVersion(version, buildVersion)
}
