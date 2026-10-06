"""Formatting check tests: real temporary Git checkout, real gofmt."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("gofmt-check.sh")
FORMATTED = "package p\n"
UNFORMATTED = "package  p\n"


class GofmtCheckTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Format test")
        self.git("config", "user.email", "test@example.com")
        self.write(".gitignore", ".fledge/\n")
        self.write("ok.go", FORMATTED)
        self.git("add", "-A")
        self.git("commit", "-q", "-m", "base")

    def git(self, *args):
        subprocess.run(["git", *args], cwd=self.root, check=True, capture_output=True)

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def check(self):
        return subprocess.run(["bash", str(SCRIPT)], cwd=self.root, capture_output=True, text=True)

    def test_formatted_checkout_passes(self):
        result = self.check()
        self.assertEqual((result.returncode, result.stdout, result.stderr), (0, "", ""))

    def test_reports_unformatted_tracked_and_new_files(self):
        self.write("tracked.go", UNFORMATTED)
        self.git("add", "tracked.go")
        self.write("new dir/nëw file.go", UNFORMATTED)
        result = self.check()
        self.assertEqual(result.returncode, 1)
        self.assertEqual(sorted(result.stderr.splitlines()), ["new dir/nëw file.go", "tracked.go"])

    def test_skips_deleted_and_ignored_files(self):
        self.write("gone.go", FORMATTED)
        self.git("add", "gone.go")
        self.git("commit", "-q", "-m", "gone")
        os.remove(self.root / "gone.go")
        self.write(".fledge/worktrees/x/bad.go", UNFORMATTED)
        result = self.check()
        self.assertEqual((result.returncode, result.stderr), (0, ""))

    def test_fails_on_gofmt_error(self):
        self.write("broken.go", "package\n")
        result = self.check()
        self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
