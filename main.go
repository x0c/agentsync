package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/x0c/agentsync/internal/agentsync"
)

// usageSynopsis documents the canonical source layout, including the per-tool
// fragment convention. An installed binary is often the only thing a user or an
// agent has to read, so the overlay feature and its file name must be stated
// here rather than only in the repository guide.
const usageSynopsis = `agentsync keeps every installed AI coding tool on one canonical instruction source.

Usage:
  agentsync            sync once
  agentsync --check    preview without writing
  agentsync --watch    keep running and resync on change

Canonical source: ~/.config/agentsync (AGENTSYNC_CONFIG_HOME overrides; %APPDATA%\agentsync on Windows)
  AGENTS.md                  rules shared by every tool
  instructions/<runtime>.md  extra rules for one tool only, for example instructions/pi.md
                             The fragment is appended under a marked
                             <!-- agentsync:begin overlay target="pi" --> block and that tool
                             receives a managed copy instead of a symlink; tools without a
                             fragment stay linked. A missing or blank fragment means no overlay.
  skills/                    skill folders synced to every installed tool
  mcp.json                   MCP servers translated into each tool's format
  sync-policy.json           per-tool exclusions
  backups/                   restore points for --rollback

The first sync also creates mcp.json, skills/ and backups/, and appends a managed MCP
notice block to AGENTS.md.

Runtime keys are the target names in the guide, for example pi, codex, claude, opencode,
cursor, antigravity, windsurf, kimi-code, agents.

Flags:
`

// usageDocs points to the full reference, including per-tool fragments, after
// the flag list.
const usageDocs = `
Docs: https://github.com/x0c/agentsync/blob/main/docs/AGENTSYNC_GUIDE.md#per-tool-instruction-fragments
`

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
	flags.Usage = func() {
		out := flags.Output()
		_, _ = fmt.Fprint(out, usageSynopsis)
		flags.PrintDefaults()
		_, _ = fmt.Fprint(out, usageDocs)
	}
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
