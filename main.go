package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

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
  agentsync skill <install|remove|status> [flags]  manage externally owned skills

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
	if len(args) > 0 && args[0] == "skill" {
		return runSkillCLI(args[1:], stderr, stdout)
	}
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

const skillUsage = `Manage externally owned skill packages across installed AI coding tools.

Usage:
  agentsync skill install <source> --owner <id> [--skill <name>] [--check] [--json]
  agentsync skill remove --owner <id> [--skill <name>] [--check] [--json]
  agentsync skill status [--owner <id>] [--skill <name>] [--json]

Flags may appear before or after the positional source path.
`

type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiStringFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// partitionSkillArgs moves known flags ahead of positional arguments so both
// `agentsync skill install /dir --owner x` and `--owner x /dir` parse uniformly.
func partitionSkillArgs(args []string, valueFlags map[string]bool) ([]string, []string) {
	var flagsOut, posOut []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			posOut = append(posOut, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flagsOut = append(flagsOut, arg)
			name := strings.TrimLeft(arg, "-")
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				continue
			}
			if valueFlags[name] && i+1 < len(args) {
				i++
				flagsOut = append(flagsOut, args[i])
			}
			continue
		}
		posOut = append(posOut, arg)
	}
	return append(flagsOut, posOut...), posOut
}

func emitSkillReport(stdout io.Writer, report agentsync.ManagedSkillReport, jsonMode bool) int {
	if jsonMode {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1
		}
	} else {
		_, _ = fmt.Fprintf(stdout, "skill %s (%s, %d actions, %d conflicts)\n",
			report.Action, report.Mode, len(report.Actions), len(report.Conflicts))
		for _, act := range report.Actions {
			_, _ = fmt.Fprintf(stdout, "  %-13s %-8s %s\n", act.Action, act.State, act.Path)
		}
	}
	if !report.OK {
		return 1
	}
	return 0
}

func runSkillCLI(args []string, stderr, stdout io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stderr, skillUsage)
		return 0
	}
	sub := args[0]
	if sub != "install" && sub != "remove" && sub != "status" {
		return cliDiagnostic(stderr, 2, "unknown skill subcommand %q (expected install, remove, or status)", sub)
	}

	rest, _ := partitionSkillArgs(args[1:], map[string]bool{
		"owner":  true,
		"source": true,
		"skill":  true,
	})
	var opts agentsync.ManagedSkillOptions
	var skillFlags multiStringFlag
	fs := flag.NewFlagSet("agentsync skill "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.Owner, "owner", "", "ownership identifier (for example corral-plugin:workflow)")
	fs.StringVar(&opts.Source, "source", "", "source directory containing one skill or child skill folders")
	fs.Var(&skillFlags, "skill", "limit operation to a named skill (repeatable)")
	if sub == "install" || sub == "remove" {
		fs.BoolVar(&opts.Check, "check", false, "preview without modifying files")
	}
	fs.BoolVar(&opts.JSON, "json", false, "emit structured JSON report to stdout")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, skillUsage)
	}

	if err := fs.Parse(rest); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	pos := fs.Args()
	switch sub {
	case "install":
		if opts.Source == "" {
			if len(pos) != 1 {
				return cliDiagnostic(stderr, 2, "skill install expects exactly one source directory")
			}
			opts.Source = pos[0]
		} else if len(pos) != 0 {
			return cliDiagnostic(stderr, 2, "unexpected positional arguments %q with --source", pos)
		}
		opts.Names = append([]string(nil), skillFlags...)
		report, err := agentsync.RunManagedSkillInstall(opts)
		if err != nil {
			return cliDiagnostic(stderr, 1, "%s", err)
		}
		return emitSkillReport(stdout, report, opts.JSON)

	case "remove":
		if opts.Source != "" {
			return cliDiagnostic(stderr, 2, "--source is only valid with skill install")
		}
		opts.Names = append(append([]string(nil), skillFlags...), pos...)
		report, err := agentsync.RunManagedSkillRemove(opts)
		if err != nil {
			return cliDiagnostic(stderr, 1, "%s", err)
		}
		return emitSkillReport(stdout, report, opts.JSON)

	default: // status
		if opts.Source != "" {
			return cliDiagnostic(stderr, 2, "--source is only valid with skill install")
		}
		opts.Names = append(append([]string(nil), skillFlags...), pos...)
		report, err := agentsync.RunManagedSkillStatus(opts)
		if err != nil {
			return cliDiagnostic(stderr, 1, "%s", err)
		}
		return emitSkillReport(stdout, report, opts.JSON)
	}
}
