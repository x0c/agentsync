"""Regression checks for public text policy and documentation portability."""

import tempfile
import unittest
from pathlib import Path

from check_repository import LANGUAGE_SWITCH, check_file


class RepositoryCheckTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)

    def check(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        return check_file(path, self.root)

    def test_translation_exception_does_not_allow_chinese_in_code(self):
        chinese = "\u4e2d\u6587"
        self.assertEqual(self.check("README.zh-CN.md", chinese), [])
        self.assertIn("use English", self.check("main.go", chinese)[0])

    def test_language_switch_is_allowed_only_on_first_line(self):
        self.check("README.zh-CN.md", "Translation")
        self.assertEqual(self.check("README.md", LANGUAGE_SWITCH + "\n# agentsync\n"), [])
        self.assertIn("use English", self.check("README.md", "# agentsync\n" + LANGUAGE_SWITCH)[0])

    def test_translation_still_checks_private_paths_and_addresses(self):
        private_path = "/" + "Users" + "/example/project/"
        private_ip = ".".join(["192", "168", "1", "2"])
        problems = self.check("README.zh-CN.md", private_path + "\n" + private_ip)
        self.assertEqual(len(problems), 2)

    def test_links_resolve_from_document_and_stay_inside_checkout(self):
        self.check("AGENTS.md", "# Instructions")
        self.assertEqual(self.check("docs/guide.md", "[Root](../AGENTS.md)"), [])
        self.assertIn("missing", self.check("docs/guide.md", "[Missing](missing.md)")[0])
        self.assertIn("leaves", self.check("docs/guide.md", "[Outside](../../private.md)")[0])
        self.assertIn("repository-relative", self.check("docs/guide.md", "[Home](~/private.md)")[0])
        self.assertEqual(self.check("docs/guide.md", "[Upstream](https://example.com/docs)"), [])


if __name__ == "__main__":
    unittest.main()
