#!/usr/bin/env python3
"""Check public repository text and documentation without third-party packages."""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit

HAN = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff\U00020000-\U0002ebef]")
PERSONAL_HOME = re.compile(r"/(?:Users|home)/[^/\s<>]+/")
PRIVATE_IP = re.compile(
    r"(?<![\d.])(?:10\.(?:\d{1,3}\.){2}\d{1,3}"
    r"|192\.168\.\d{1,3}\.\d{1,3}"
    r"|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3})(?![\d.])"
)
LINK = re.compile(r"!?\[[^\]\n]*\]\(([^\s)]+)(?:\s+\"[^\"]*\")?\)")
LANGUAGE_SWITCH = "**Languages:** English | [\u7b80\u4f53\u4e2d\u6587](README.zh-CN.md)"
TRANSLATIONS = {"README.zh-CN.md"}


def repository_files(root: Path) -> list[Path]:
    """Include tracked and new source files while respecting Git ignores."""
    result = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        cwd=root,
        check=True,
        stdout=subprocess.PIPE,
    )
    return sorted({root / name for name in result.stdout.decode("utf-8").split("\0") if name})


def check_file(path: Path, root: Path) -> list[str]:
    relative = path.relative_to(root).as_posix()
    try:
        content = path.read_bytes()
    except OSError as error:
        return [f"{relative}: cannot read file: {error}"]
    if b"\0" in content:
        return []
    try:
        text = content.decode("utf-8")
    except UnicodeDecodeError:
        return []  # Binary assets are outside the text check.

    problems = []
    for number, line in enumerate(text.splitlines(), 1):
        location = f"{relative}:{number}"
        allowed_switch = relative == "README.md" and number == 1 and line == LANGUAGE_SWITCH
        if relative not in TRANSLATIONS and not allowed_switch and HAN.search(line):
            problems.append(f"{location}: use English outside designated translations")
        if PERSONAL_HOME.search(line):
            problems.append(f"{location}: replace personal absolute home paths")
        if PRIVATE_IP.search(line):
            problems.append(f"{location}: replace private network addresses")
        if path.suffix.lower() != ".md":
            continue
        for match in LINK.finditer(line):
            target = unquote(match.group(1).strip("<>"))
            parsed = urlsplit(target)
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            if parsed.path.startswith(("~", "/")):
                problems.append(f"{location}: documentation link must be repository-relative: {target}")
                continue
            destination = (path.parent / parsed.path).resolve()
            if not destination.is_relative_to(root.resolve()):
                problems.append(f"{location}: documentation link leaves the repository: {target}")
            elif not destination.exists():
                problems.append(f"{location}: missing documentation target: {target}")
    return problems


def main() -> int:
    root = Path(__file__).resolve().parent.parent
    try:
        files = repository_files(root)
    except (OSError, subprocess.CalledProcessError) as error:
        print(f"Repository check requires Git and a checkout: {error}", file=sys.stderr)
        return 1
    problems = [problem for path in files for problem in check_file(path, root)]
    if problems:
        print("\n".join(problems), file=sys.stderr)
        return 1
    print(f"Repository text and documentation checks passed ({len(files)} files).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
