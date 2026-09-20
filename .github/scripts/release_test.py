"""Offline release tests: real temporary Git history, mocked GitHub calls."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("release", Path(__file__).with_name("release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        old = os.getcwd()
        self.addCleanup(os.chdir, old)
        os.chdir(self.tmp.name)
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.name", "Release test")
        self.git("config", "user.email", "test@example.com")
        Path("go.mod").write_text("module github.com/Harrison-Blair/fledge\n")
        self.commit()
        self.base = self.git("rev-parse", "HEAD")
        self.git("tag", "v0.0.3")
        self.commit()
        self.sha = self.git("rev-parse", "HEAD")
        self.releases = [{"tag_name": "v0.0.3", "draft": False, "target_commitish": self.base}]
        self.assets = {}
        self.calls = []
        self.fail_upload = False
        self.truncate_upload = False
        self.gh_patch = patch.object(release, "gh", self.gh)
        self.gh_patch.start()
        self.addCleanup(self.gh_patch.stop)

    def git(self, *args):
        return subprocess.check_output(["git", *args], stderr=subprocess.PIPE, text=True).strip()

    def commit(self):
        path = Path("changes")
        path.write_text(path.read_text() + "change\n" if path.exists() else "change\n")
        self.git("add", "go.mod", "changes")
        self.git("commit", "-qm", "change")

    def gh(self, *args):
        self.calls.append(args)
        if args[0] == "api":
            return json.dumps([[dict(item, assets=[{"name": name} for tag, name in self.assets if tag == item["tag_name"]]) for item in self.releases]])
        command, tag = args[1:3]
        if command == "create":
            self.releases.append({"tag_name": tag, "draft": True, "target_commitish": args[args.index("--target") + 1]})
        elif command == "upload":
            if self.fail_upload:
                self.fail_upload = False
                raise RuntimeError("simulated upload failure")
            for filename in args[3:]:
                p = Path(filename)
                if (tag, p.name) in self.assets:
                    raise RuntimeError("asset already exists")
                self.assets[(tag, p.name)] = b"broken" if self.truncate_upload else p.read_bytes()
        elif command == "download":
            destination = Path(args[args.index("--dir") + 1])
            for (asset_tag, name), data in self.assets.items():
                if asset_tag == tag:
                    (destination / name).write_bytes(data)
        elif command == "edit":
            self.assertIn("--draft=false", args)
            self.assertIn("--latest", args)
            for item in self.releases:
                if item["tag_name"] == tag:
                    item["draft"] = False
            self.git("tag", tag, self.sha)
        else:
            raise AssertionError(args)
        return ""

    def plan(self, bump="patch"):
        return release.plan(bump, self.sha, "refs/heads/main")

    def dist(self):
        dist = Path("dist")
        dist.mkdir(exist_ok=True)
        sums = []
        for arch in ("amd64", "arm64"):
            name = f"fledge_v0.0.4_linux_{arch}.tar.gz"
            data = arch.encode()
            (dist / name).write_bytes(data)
            sums.append(f"{hashlib.sha256(data).hexdigest()}  {name}\n")
        (dist / "checksums.txt").write_text("".join(sums))
        return dist

    def test_bumps(self):
        for bump, want in [("patch", "v0.0.4"), ("minor", "v0.1.0"), ("major", "v1.0.0")]:
            self.assertEqual(self.plan(bump)["tag"], want)
        self.assertEqual(self.plan()["previous"], "v0.0.3")

    def test_numeric_order_and_resets(self):
        self.git("tag", "v0.9.99", self.base)
        self.git("tag", "v0.10.7", self.base)
        self.git("tag", "v9.0.0-rc.1", self.base)
        self.assertEqual(self.plan()["tag"], "v0.10.8")
        self.assertEqual(self.plan("minor")["tag"], "v0.11.0")

    def test_invalid_requests(self):
        for bump, ref, sha in [("huge", "refs/heads/main", self.sha), ("patch", "refs/heads/dev", self.sha), ("patch", "refs/heads/main", self.base)]:
            with self.assertRaises(RuntimeError):
                release.plan(bump, sha, ref)

    def test_missing_tag_history(self):
        self.git("tag", "-d", "v0.0.3")
        with self.assertRaisesRegex(RuntimeError, "tag history"):
            self.plan()

    def test_v2_requires_module_migration(self):
        self.git("tag", "v1.0.0", self.base)
        with self.assertRaisesRegex(RuntimeError, "module path"):
            self.plan("major")

    def test_matching_module_major_is_allowed(self):
        for old, new in [(1, 2), (9, 10)]:
            self.git("tag", f"v{old}.0.0", self.base)
            Path("go.mod").write_text(f"module github.com/Harrison-Blair/fledge/v{new}\n")
            self.assertEqual(self.plan("major")["tag"], f"v{new}.0.0")

    def test_latest_must_be_ancestor(self):
        self.git("checkout", "-q", "--orphan", "unrelated")
        self.commit()
        self.git("tag", "v0.9.0")
        self.git("checkout", "-q", "main")
        with self.assertRaisesRegex(RuntimeError, "ancestor"):
            self.plan()

    def test_published_commit_is_noop(self):
        self.git("tag", "v0.0.4")
        self.releases.append({"tag_name": "v0.0.4", "draft": False, "target_commitish": self.sha})
        self.assertTrue(self.plan()["skip"])

    def test_draft_recovery(self):
        self.releases.append({"tag_name": "v0.1.0", "draft": True, "target_commitish": self.sha})
        self.assertEqual(self.plan("minor")["tag"], "v0.1.0")
        with self.assertRaisesRegex(RuntimeError, "bump"):
            self.plan("patch")

    def test_tag_without_release_is_resumed(self):
        self.git("tag", "v0.0.4")
        self.assertEqual(self.plan()["tag"], "v0.0.4")
        self.assertFalse(self.plan()["skip"])

    def test_conflicting_draft_target(self):
        self.releases.append({"tag_name": "v0.0.4", "draft": True, "target_commitish": self.base})
        with self.assertRaisesRegex(RuntimeError, "draft"):
            self.plan()

    def test_tag_and_draft_disagree(self):
        self.git("tag", "v0.0.4", self.base)
        self.releases.append({"tag_name": "v0.0.4", "draft": True, "target_commitish": self.sha})
        with self.assertRaises(RuntimeError):
            self.plan()

    def test_duplicate_releases(self):
        self.releases.append(dict(self.releases[0]))
        with self.assertRaisesRegex(RuntimeError, "Multiple"):
            self.plan()

    def test_publish_and_retry(self):
        request = self.plan()
        release.publish(request, self.dist())
        self.assertFalse(self.releases[-1]["draft"])
        self.assertEqual(set(name for tag, name in self.assets), set(p.name for p in Path("dist").iterdir()))
        before = len([c for c in self.calls if c[:2] == ("release", "upload")])
        release.publish(request, self.dist())
        self.assertEqual(before, len([c for c in self.calls if c[:2] == ("release", "upload")]))

    def test_failed_upload_resumes_same_version(self):
        request = self.plan()
        self.fail_upload = True
        with self.assertRaisesRegex(RuntimeError, "upload"):
            release.publish(request, self.dist())
        self.assertTrue(self.releases[-1]["draft"])
        self.assertEqual(self.plan()["tag"], request["tag"])
        release.publish(request, self.dist())
        self.assertFalse(self.releases[-1]["draft"])

    def test_existing_draft_assets_are_reused(self):
        request = self.plan()
        self.releases.append({"tag_name": request["tag"], "draft": True, "target_commitish": self.sha})
        dist = self.dist()
        for p in dist.iterdir():
            self.assets[(request["tag"], p.name)] = p.read_bytes()
        release.publish(request, dist)
        self.assertFalse(any(c[:2] == ("release", "upload") for c in self.calls))

    def test_partial_upload_only_uploads_missing_assets(self):
        request = self.plan()
        self.releases.append({"tag_name": request["tag"], "draft": True, "target_commitish": self.sha})
        dist = self.dist()
        self.assets[(request["tag"], "checksums.txt")] = (dist / "checksums.txt").read_bytes()
        release.publish(request, dist)
        uploads = [c for c in self.calls if c[:2] == ("release", "upload")]
        self.assertEqual(len(uploads), 1)
        self.assertEqual(len(uploads[0][3:]), 2)
        self.assertFalse(self.releases[-1]["draft"])

    def test_mismatched_existing_asset_is_not_overwritten(self):
        request = self.plan()
        self.releases.append({"tag_name": request["tag"], "draft": True, "target_commitish": self.sha})
        self.assets[(request["tag"], "checksums.txt")] = b"unexpected"
        with self.assertRaisesRegex(RuntimeError, "refusing to overwrite"):
            release.publish(request, self.dist())
        self.assertFalse(any(c[:2] == ("release", "upload") for c in self.calls))
        self.assertTrue(self.releases[-1]["draft"])

    def test_bad_assets_never_publish(self):
        request = self.plan()
        self.truncate_upload = True
        with self.assertRaisesRegex(RuntimeError, "asset"):
            release.publish(request, self.dist())
        self.assertTrue(self.releases[-1]["draft"])

    def test_incomplete_dist_never_creates_release(self):
        request = self.plan()
        dist = self.dist()
        (dist / "fledge_v0.0.4_linux_arm64.tar.gz").unlink()
        with self.assertRaises(RuntimeError):
            release.publish(request, dist)
        self.assertEqual(len(self.releases), 1)

    def test_intervening_release_aborts(self):
        request = self.plan()
        self.git("tag", "v0.1.0", self.base)
        with self.assertRaisesRegex(RuntimeError, "changed"):
            release.publish(request, self.dist())


if __name__ == "__main__":
    unittest.main()
