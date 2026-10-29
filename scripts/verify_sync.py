"""Verify sync safety through the actual CLI using disposable configuration roots.

Builds a temporary binary by default. Use --binary to verify an installed or
packaged executable. No real user configuration or network service is used.
"""

import argparse
import hashlib
import json
import os
import signal
import subprocess
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class Fixture:
    def __init__(self, root, binary, evidence, name):
        self.name = name
        self.home = root / name
        self.home.mkdir()
        self.config = self.home / ".config/agentsync"
        self.binary = binary
        self.evidence = evidence
        self.invocations = 0
        self.write(self.config / "AGENTS.md", "# Canonical\nBASE_RULE\n")
        self.canonical({})
        self.env = {key: os.environ[key] for key in
                    ("PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "TEMP", "TMP")
                    if key in os.environ}
        self.env.update({
            "HOME": str(self.home), "USERPROFILE": str(self.home),
            "XDG_CONFIG_HOME": str(self.home / ".config"),
            "APPDATA": str(self.home / "appdata"),
            "LOCALAPPDATA": str(self.home / "localappdata"),
            "AGENTSYNC_CONFIG_HOME": str(self.config),
            "PI_CODING_AGENT_DIR": str(self.home / ".pi/agent"),
            "DSH_HOME": str(self.home / ".dsh"),
        })

    @staticmethod
    def write(path, content):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def canonical(self, servers):
        self.write(self.config / "mcp.json", json.dumps({"mcpServers": servers}))

    def run(self, *args, expected=0):
        result = subprocess.run([str(self.binary), *args], cwd=self.home,
                                env=self.env, capture_output=True, text=True,
                                stdin=subprocess.DEVNULL, timeout=30, check=False)
        self.invocations += 1
        if self.evidence:
            path = self.evidence / f"{self.name}-{self.invocations}.log"
            path.write_text(result.stdout + result.stderr, encoding="utf-8")
        if expected is None:
            assert result.returncode != 0, f"{self.name}: unsafe call succeeded: {args}"
        else:
            assert result.returncode == expected, (
                f"{self.name}: {args}: exit {result.returncode}, expected {expected}\n"
                + result.stdout + result.stderr)
        return result

    def stamp(self):
        manifests = sorted((self.config / "backups").glob("*/manifest.json"))
        assert manifests, f"{self.name}: no backup manifest"
        return manifests[-1].parent.name


def read(path):
    return path.read_text(encoding="utf-8")


def snapshot(root):
    result = {}
    for path in root.rglob("*"):
        relative = str(path.relative_to(root))
        if path.is_symlink():
            result[relative] = "link:" + os.readlink(path)
        elif path.is_dir():
            result[relative] = "directory"
        else:
            result[relative] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


def skill(fixture, name="demo", chained=False):
    real = fixture.home / "external/real-demo"
    fixture.write(real / "SKILL.md", "# Demo\n")
    fixture.write(real / "assets/data.txt", "KEEP_ASSET\n")
    target = real
    if chained:
        target = fixture.home / "external/alias"
        target.symlink_to("real-demo", target_is_directory=True)
    link = fixture.config / "skills" / name
    link.parent.mkdir(exist_ok=True)
    link.symlink_to(target, target_is_directory=True)
    (fixture.home / ".claude").mkdir(exist_ok=True)
    return real, link, target


def case_staging(f):
    _, link, _ = skill(f)
    sibling = link.with_name(link.name + ".agentsync-tmp")
    f.write(sibling / "SKILL.md", "# Independent\n")
    f.write(sibling / "unique.txt", "USER_DATA\n")
    before = snapshot(f.home)
    f.run("--check")
    assert snapshot(f.home) == before
    f.run()
    assert read(sibling / "unique.txt") == "USER_DATA\n"
    assert link.is_dir() and not link.is_symlink()


def case_backups(f):
    paths = [f.home / ".config/opencode/AGENTS.md",
             f.home / ".config__opencode/AGENTS.md"]
    f.env["PI_CODING_AGENT_DIR"] = str(paths[1].parent)
    for index, path in enumerate(paths):
        f.write(path, f"ORIGINAL_{index}\n")
    f.run("--force")
    stamp = f.stamp()
    manifest = json.loads(read(f.config / "backups" / stamp / "manifest.json"))
    entries = manifest["entries"]
    assert len({entry["name"] for entry in entries}) == len(entries)
    f.run("--rollback", stamp)
    for index, path in enumerate(paths):
        assert read(path) == f"ORIGINAL_{index}\n"


def case_arguments(f):
    before = snapshot(f.home)
    for args in [("status", "--check"), ("--check", "status"),
                 ("--all", str(f.home), "--repo")]:
        f.run(*args, expected=2)
        assert snapshot(f.home) == before


def case_codex_options(f):
    path = f.home / ".codex/config.toml"
    f.write(path, '[mcp_servers.db]\nurl="https://example.com/mcp"\n'
            'enabled=false\nenabled_tools=["read"]\ndisabled_tools=["delete"]\n'
            'bearer_token_env_var="DB_TOKEN"\n')
    f.canonical({"db": {"url": "https://example.com/mcp"},
                 "new": {"command": "new"}})
    f.run()
    text = read(path)
    for field in ("enabled = false", "enabled_tools", "disabled_tools", "bearer_token_env_var"):
        assert field in text, f"lost Codex restriction: {field}"
    before = path.read_bytes()
    f.run()
    assert path.read_bytes() == before


def case_toml_comments(f):
    path = f.home / ".codex/config.toml"
    keep = '[projects."/work"] # project comment\ntrust_level="trusted"\n'
    f.write(path, '[mcp_servers.old]\ncommand="old"\n' + keep)
    f.canonical({"new": {"command": "new"}})
    f.run()
    assert keep in read(path)


def case_crush(f):
    path = f.home / ".config/crush/crush.json"
    original = '{"theme":"keep","mcp":{}}'
    f.write(path, original)
    for expression in ("`printf MUST_NOT_EXECUTE`", "$(printf MUST_NOT_EXECUTE)"):
        f.canonical({"demo": {"command": "node", "env": {"VALUE": expression}}})
        f.run(expected=None)
        assert read(path) == original


def case_invalid_source(f):
    path = f.home / ".cursor/mcp.json"
    original = json.dumps({"mcpServers": {"keep": {"command": "keep"}, "new": {"command": "new"}}})
    f.write(path, original)
    for invalid in (None, [], {}, {"command": "node", "args": [42]}):
        f.canonical({"keep": invalid, "new": {"command": "new"}})
        f.run(expected=None)
        assert read(path) == original


def case_cursor(f):
    path = f.home / ".cursor/rules/AGENTS.mdc"
    f.write(path, "---\nalwaysApply: true\n---\nUSER_CURSOR_RULE\n")
    f.run()
    assert "USER_CURSOR_RULE" in read(f.config / "AGENTS.md")
    assert "USER_CURSOR_RULE" in read(path) and "managed-by: agentsync" in read(path)


def case_rollback_directory(f):
    _, link, target = skill(f)
    f.run()
    stamp = f.stamp()
    before = snapshot(f.home)
    f.run("--check", "--rollback", stamp)
    assert snapshot(f.home) == before
    f.run("--rollback", stamp)
    assert link.is_symlink() and os.path.samefile(link, target)
    f.run("--rollback", "latest")
    assert link.is_dir() and not link.is_symlink()
    assert read(link / "assets/data.txt") == "KEEP_ASSET\n"


def case_latest(f):
    path = f.home / ".claude/CLAUDE.md"
    f.write(path, "ORIGINAL\n")
    f.run("--force")
    stamp = f.stamp()
    f.run()
    assert f.stamp() == stamp
    f.run("--rollback", "latest")
    assert read(path) == "ORIGINAL\n"


def case_overlap(f):
    runtime = f.home / ".claude/skills"
    f.write(runtime / "demo/SKILL.md", "ORIGINAL_SKILL\n")
    (f.config / "skills").symlink_to(runtime, target_is_directory=True)
    before = snapshot(runtime)
    f.run(expected=None)
    assert snapshot(runtime) == before and not runtime.is_symlink()


def case_chained(f):
    _, link, _ = skill(f, chained=True)
    f.run()
    assert link.is_dir() and not link.is_symlink()
    assert read(f.home / ".claude/skills/demo/assets/data.txt") == "KEEP_ASSET\n"


def case_watch(f):
    cursor = f.home / ".cursor/rules/AGENTS.mdc"
    cursor.parent.mkdir(parents=True)
    f.write(f.home / ".codeium/windsurf/memories/global_rules.md", "LATE_RULE\n")
    log_path = f.home / "watch.log"
    with log_path.open("w", encoding="utf-8") as log:
        flags = subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0
        process = subprocess.Popen([str(f.binary), "--watch"], cwd=f.home, env=f.env,
                                   stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                                   creationflags=flags)
        try:
            wait_for(cursor, "LATE_RULE", process)
            canonical = f.config / "AGENTS.md"
            f.write(canonical, read(canonical) + "\nEDITED_RULE\n")
            wait_for(cursor, "EDITED_RULE", process)
        finally:
            try:
                if os.name == "nt":
                    process.send_signal(signal.CTRL_BREAK_EVENT)
                else:
                    process.terminate()
                process.wait(timeout=10)
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait(timeout=10)
    assert process.returncode == 0, read(log_path)
    assert "unsafe overlapping" not in read(log_path)
    f.run("--check")


def wait_for(path, needle, process):
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        assert process.poll() is None, "watch terminated unexpectedly"
        try:
            if needle in read(path):
                return
        except FileNotFoundError:
            pass
        time.sleep(0.05)
    raise AssertionError(f"watch did not synchronize {needle}")


def case_sse(f):
    (f.config / "mcp.json").unlink()
    path = f.home / ".gemini/settings.json"
    f.write(path, '{"theme":"keep","mcpServers":{"events":{"url":"https://example.com/sse"}}}')
    f.write(f.home / ".cursor/mcp.json", '{"mcpServers":{"other":{"command":"other"}}}')
    f.run()
    doc = json.loads(read(path))
    assert doc["mcpServers"]["events"] == {"url": "https://example.com/sse"}
    assert doc["theme"] == "keep"


def case_policy(f):
    path = f.home / ".claude.json"
    f.write(f.home / ".claude/installed", "runtime")
    f.write(path, '{"mcpServers":{}}')
    f.canonical({"secret": {"command": "secret"}})
    for section in ("mcp", "skills"):
        f.write(f.config / "sync-policy.json", json.dumps({section: {"targets": {
            "claude": {}, "CLAUDE": {"deny": ["secret"]}}}}))
        f.run(expected=None)
        assert read(path) == '{"mcpServers":{}}'


def case_scan(f):
    f.run("--all", str(f.home / "missing"), "--check", expected=1)
    empty = f.home / "empty"
    empty.mkdir()
    result = f.run("--all", str(empty), "--check")
    assert "Repositories:\n  0" in result.stdout


def case_quoted(f):
    (f.config / "mcp.json").unlink()
    f.write(f.home / ".codex/config.toml", '[mcp_servers."foo.bar"]\ncommand="node"\n')
    f.write(f.home / ".cursor/mcp.json", '{"mcpServers":{"other":{"command":"other"}}}')
    f.run()
    servers = json.loads(read(f.config / "mcp.json"))["mcpServers"]
    assert servers["foo.bar"]["command"] == "node" and "foo" not in servers


def positive_smoke(f):
    for tool, name in (("claude", "CLAUDE.md"), ("codex", "AGENTS.md")):
        f.write(f.home / f".{tool}" / name, f"{tool.upper()}_RULE\n")
        f.write(f.home / f".{tool}/skills/{tool}/SKILL.md", f"# {tool}\n")
    before = snapshot(f.home)
    f.run("--check")
    assert before == snapshot(f.home), "preview wrote files"
    f.run()
    source = read(f.config / "AGENTS.md")
    assert "CLAUDE_RULE" in source and "CODEX_RULE" in source
    assert not (f.home / ".gemini").exists()
    result = f.run("--check")
    for state in ("replaceable ", "mergeable ", "wrong-link ", "missing ", "blocked "):
        assert state not in result.stdout, result.stdout


CASES = [case_staging, case_backups, case_arguments, case_codex_options,
         case_toml_comments, case_crush, case_invalid_source, case_cursor,
         case_rollback_directory, case_latest, case_overlap, case_chained,
         case_watch, case_sse, case_policy, case_scan, case_quoted, positive_smoke]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, help="Existing executable to verify")
    parser.add_argument("--evidence", type=Path, help="Retain complete CLI logs here")
    args = parser.parse_args()
    evidence = args.evidence.resolve() if args.evidence else None
    if evidence:
        evidence.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="agentsync-acceptance-") as directory:
        root = Path(directory)
        binary = args.binary.resolve() if args.binary else root / ("agentsync.exe" if os.name == "nt" else "agentsync")
        if not args.binary:
            subprocess.run(["go", "build", "-o", str(binary), "."], cwd=ROOT, check=True)
        for case in CASES:
            case(Fixture(root, binary, evidence, case.__name__))
            print(f"PASS {case.__name__}", flush=True)
        print(f"PASS {len(CASES)} isolated CLI scenarios; no real configuration changed")


if __name__ == "__main__":
    main()
