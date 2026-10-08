#!/usr/bin/env python3
"""Record the README's one-command demo from a real, isolated CLI run.

Requires Go, Python 3.9+, VHS, ttyd, and ffmpeg. No real agent configuration
is read or changed. The capture displays an explicitly labeled output excerpt.
"""

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[1]


def seed(home):
    """Represent two installed tools with distinct rules, skills, and MCP."""
    for tool, skill in (("claude", "review"), ("codex", "tests")):
        folder = home / f".{tool}/skills/{skill}"
        folder.mkdir(parents=True)
        (folder / "SKILL.md").write_text(
            f"---\nname: {skill}\ndescription: Demo {skill} skill\n---\n"
            f"Run the {skill} workflow.\n", encoding="utf-8"
        )
    (home / ".claude/CLAUDE.md").write_text(
        "# Review\nKeep changes focused.\n", encoding="utf-8"
    )
    (home / ".codex/AGENTS.md").write_text(
        "# Tests\nRun relevant checks.\n", encoding="utf-8"
    )
    (home / ".claude.json").write_text(json.dumps({
        "theme": "dark",
        "mcpServers": {"docs": {"command": "demo-docs-server"}},
    }) + "\n", encoding="utf-8")


def isolated_env(home):
    env = os.environ.copy()
    env.update({
        "HOME": str(home),
        "XDG_CONFIG_HOME": str(home / ".config"),
        "PI_CODING_AGENT_DIR": str(home / ".pi/agent"),
        "DSH_HOME": str(home / ".dsh"),
    })
    return env


def verify(home):
    """Check the behavior shown in the capture before displaying successes."""
    source = home / ".config/agentsync"
    rules = (source / "AGENTS.md").read_text(encoding="utf-8")
    for original in ("Keep changes focused.", "Run relevant checks."):
        if original not in rules:
            raise RuntimeError("Original instructions were not preserved")
    for path in (".claude/CLAUDE.md", ".codex/AGENTS.md"):
        if (home / path).resolve() != (source / "AGENTS.md").resolve():
            raise RuntimeError(f"Instruction entry is not linked: {path}")
    for tool in ("claude", "codex"):
        if (home / f".{tool}/skills").resolve() != (source / "skills").resolve():
            raise RuntimeError(f"Skill root is not linked: {tool}")
    for skill in ("review", "tests"):
        if not (source / f"skills/{skill}/SKILL.md").is_file():
            raise RuntimeError(f"Original skill is missing: {skill}")
    if not list((source / "backups").glob("*/manifest.json")):
        raise RuntimeError("Replacement backups are missing")
    canonical = json.loads((source / "mcp.json").read_text(encoding="utf-8"))
    claude = json.loads((home / ".claude.json").read_text(encoding="utf-8"))
    if "docs" not in canonical["mcpServers"] or claude["theme"] != "dark":
        raise RuntimeError("MCP import or unrelated setting preservation failed")
    codex = (home / ".codex/config.toml").read_text(encoding="utf-8")
    if "demo-docs-server" not in codex:
        raise RuntimeError("Codex did not receive the imported MCP server")
    if (home / ".gemini").exists():
        raise RuntimeError("An absent tool was created")


def emit(work):
    """Run the binary, retain its full report, and print selected verbatim rows."""
    home = work / "home"
    result = subprocess.run(
        [str(work / "agentsync-real")], cwd=home, env=isolated_env(home),
        capture_output=True, text=True, check=True,
    )
    (work / "full-output.txt").write_text(result.stdout, encoding="utf-8")
    verify(home)
    # Omit source/import details, absent tools, and long backup paths. Preserve
    # section names and installed-tool result rows exactly; shorten only HOME.
    keep = re.compile(
        r"^  (?:merged|replaced|created|ok)\s+"
        r"~/(?:\.claude/CLAUDE\.md|\.codex/AGENTS\.md|"
        r"\.claude/skills|\.codex/skills|\.claude\.json|\.codex/config\.toml) "
    )
    excerpt = []
    for line in result.stdout.replace(str(home), "~").splitlines():
        if line in ("agentsync apply", "Results:", "Skills:", "MCP:"):
            if excerpt:
                excerpt.append("")
            excerpt.append(line)
        elif keep.match(line):
            excerpt.append(line)
    text = "\n".join(excerpt) + "\n"
    (work / "excerpt.txt").write_text(text, encoding="utf-8")
    print(text, end="", flush=True)


def record(output, evidence):
    for executable in ("go", "vhs", "ttyd", "ffmpeg"):
        if not shutil.which(executable):
            raise RuntimeError(f"Required capture tool is missing: {executable}")
    output = output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="agentsync-capture-") as directory:
        work = Path(directory)
        home = work / "home"
        seed(home)
        subprocess.run(
            ["go", "build", "-o", str(work / "agentsync-real"), "."],
            cwd=ROOT, check=True,
        )
        # VHS runs this capture-only entry point; it invokes the actual binary
        # in the isolated home and never supplies synthetic result messages.
        entry = work / "agentsync"
        entry.write_text(
            f"#!{sys.executable}\nimport runpy, sys\n"
            f"sys.argv = [{str(Path(__file__).resolve())!r}, '--emit', {str(work)!r}]\n"
            f"runpy.run_path({str(Path(__file__).resolve())!r}, run_name='__main__')\n",
            encoding="utf-8",
        )
        entry.chmod(0o755)
        tape = (ROOT / "docs/images/demo.tape").read_text(encoding="utf-8")
        tape = tape.replace("@OUTPUT@", json.dumps(str(output)))
        tape = tape.replace("@PATH@", json.dumps(str(work) + os.pathsep + os.environ["PATH"]))
        (work / "demo.tape").write_text(tape, encoding="utf-8")
        subprocess.run(["vhs", str(work / "demo.tape")], cwd=work, check=True)
        verify(home)
        if evidence:
            evidence.mkdir(parents=True, exist_ok=True)
            for name in ("full-output.txt", "excerpt.txt"):
                shutil.copy2(work / name, evidence / name)
        print(f"Recorded and verified: {output}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "docs/images/demo.gif")
    parser.add_argument("--evidence", type=Path, help="Optional directory for the full report")
    parser.add_argument("--emit", type=Path, help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.emit:
        emit(args.emit)
    else:
        record(args.output, args.evidence)


if __name__ == "__main__":
    main()
