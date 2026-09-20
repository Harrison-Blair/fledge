"""Select and publish releases. Git tags own versions; GitHub drafts track retries.

Requires only Python's standard library, Git, and the authenticated GitHub CLI.
All GitHub mutations are confined to publish(), after CI and archive creation.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def run(*args):
    result = subprocess.run(args, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or f"{args[0]} failed")
    return result.stdout.strip()


def gh(*args):
    return run("gh", *args)


def version(tag):
    match = re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag)
    return tuple(map(int, match.groups())) if match else None


def releases():
    pages = json.loads(gh("api", "--paginate", "--slurp", "repos/{owner}/{repo}/releases?per_page=100"))
    result = {}
    for page in pages:
        for item in page:
            tag = item["tag_name"]
            if version(tag) is None:
                continue
            if tag in result:
                raise RuntimeError(f"Multiple GitHub releases use {tag}; resolve the conflict manually")
            result[tag] = item
    return result


def plan(bump, sha, ref):
    if ref != "refs/heads/main":
        raise RuntimeError("Releases must be requested from main")
    if bump not in ("patch", "minor", "major"):
        raise RuntimeError("bump must be patch, minor, or major")
    if run("git", "rev-parse", "HEAD") != sha:
        raise RuntimeError("Checkout does not match the requested commit")
    tags = {tag: run("git", "rev-parse", f"{tag}^{{commit}}")
            for tag in run("git", "tag", "--list").splitlines() if version(tag)}
    if not tags:
        raise RuntimeError("No stable tag history found; fetch the existing release tags")
    existing = releases()
    at_target = [tag for tag, commit in tags.items() if commit == sha]
    if len(at_target) > 1:
        raise RuntimeError("Multiple stable tags mark this commit; resolve the conflict manually")
    request = {"sha": sha, "bump": bump, "skip": False, "previous": ""}
    if at_target and at_target[0] in existing and not existing[at_target[0]]["draft"]:
        return dict(request, tag=at_target[0], skip=True)

    drafts = [item for item in existing.values() if item["draft"]]
    if len(drafts) > 1 or any(item["target_commitish"] != sha for item in drafts):
        raise RuntimeError("An unfinished draft targets another commit; finish or remove that draft before requesting a new release")
    candidate = drafts[0]["tag_name"] if drafts else (at_target[0] if at_target else None)
    if candidate in tags and tags[candidate] != sha:
        raise RuntimeError(f"{candidate} already marks a different commit; tags are never moved")
    if candidate and at_target and at_target != [candidate]:
        raise RuntimeError("Draft and existing tag disagree")
    previous_tags = [tag for tag in tags if tag != candidate]
    if not previous_tags:
        raise RuntimeError("Previous release tag history is missing")
    previous = max(previous_tags, key=version)
    if subprocess.run(["git", "merge-base", "--is-ancestor", tags[previous], sha], capture_output=True).returncode:
        raise RuntimeError(f"Latest stable tag {previous} is not an ancestor of the requested commit")
    parts = list(version(previous))
    index = {"major": 0, "minor": 1, "patch": 2}[bump]
    parts[index] += 1
    parts[index + 1:] = [0] * (2 - index)
    tag = "v" + ".".join(map(str, parts))
    if candidate and candidate != tag:
        raise RuntimeError(f"Existing unfinished release {candidate} does not match the requested {bump} bump ({tag})")
    module = re.search(r"^module\s+(\S+)", Path("go.mod").read_text(), re.MULTILINE).group(1)
    suffix = re.search(r"/v([2-9]|[1-9][0-9]+)$", module)
    module_major = int(suffix.group(1)) if suffix else None
    if (parts[0] >= 2 and module_major != parts[0]) or (module_major and parts[0] != module_major):
        raise RuntimeError(f"Release {tag} requires a matching Go module path (for v2+, migrate go.mod and imports to /v{parts[0]} first)")
    if tag in existing and not existing[tag]["draft"]:
        raise RuntimeError(f"{tag} is already published")
    return dict(request, tag=tag, previous=previous)


def check_dist(dist, tag):
    names = [f"fledge_{tag}_linux_{arch}.tar.gz" for arch in ("amd64", "arm64")]
    expected = set(names + ["checksums.txt"])
    if set(p.name for p in dist.iterdir()) != expected:
        raise RuntimeError("Release assets must contain both Linux archives and checksums.txt only")
    lines = (dist / "checksums.txt").read_text().splitlines()
    want = [f"{hashlib.sha256((dist / name).read_bytes()).hexdigest()}  {name}" for name in names]
    if sorted(lines) != sorted(want):
        raise RuntimeError("Release asset checksums do not match")
    return sorted(expected)


def verify_assets(tag, dist, names, complete):
    with tempfile.TemporaryDirectory() as tmp:
        gh("release", "download", tag, "--dir", tmp)
        found = set(p.name for p in Path(tmp).iterdir())
        if found - set(names):
            raise RuntimeError("Draft contains unexpected release assets")
        for name in found:
            if (Path(tmp) / name).read_bytes() != (dist / name).read_bytes():
                raise RuntimeError(f"Draft asset {name} differs from this build; refusing to overwrite it")
        if complete and found != set(names):
            raise RuntimeError("Uploaded release assets are incomplete")
        return found


def publish(request, dist):
    # The workflow refreshes remote tags immediately before this recheck.
    current = plan(request["bump"], request["sha"], "refs/heads/main")
    if current["tag"] != request["tag"]:
        raise RuntimeError("Release state changed during CI; request a new release")
    if current["skip"]:
        return
    if current["previous"] != request["previous"]:
        raise RuntimeError("Previous release changed during CI")
    tag = request["tag"]
    dist = Path(dist)
    names = check_dist(dist, tag)
    existing = releases().get(tag)
    if existing and not existing["draft"]:
        raise RuntimeError(f"{tag} was published during this run; refusing to change it")
    if not existing:
        gh("release", "create", tag, "--target", request["sha"], "--title", tag,
           "--generate-notes", "--notes-start-tag", request["previous"], "--draft")
        found = set()
    else:
        # The API includes asset names, avoiding gh download's no-assets error.
        found = verify_assets(tag, dist, names, False) if existing.get("assets") else set()
    missing = [str(dist / name) for name in names if name not in found]
    if missing:
        gh("release", "upload", tag, *missing)
    verify_assets(tag, dist, names, True)
    gh("release", "edit", tag, "--draft=false", "--latest")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    preflight = commands.add_parser("plan")
    preflight.add_argument("--bump", required=True, choices=("patch", "minor", "major"))
    publish_cmd = commands.add_parser("publish")
    publish_cmd.add_argument("--request", required=True)
    publish_cmd.add_argument("--dist", required=True, type=Path)
    args = parser.parse_args()
    try:
        if args.command == "plan":
            print(json.dumps(plan(args.bump, os.environ["GITHUB_SHA"], os.environ["GITHUB_REF"])))
        else:
            request = json.loads(args.request)
            if os.environ["GITHUB_REF"] != "refs/heads/main" or request["sha"] != os.environ["GITHUB_SHA"]:
                raise RuntimeError("Publication must use the requested main commit")
            publish(request, args.dist)
            print(f"Published {request['tag']}")
    except (RuntimeError, OSError, ValueError) as exc:
        parser.exit(1, f"release: {exc}\n")


if __name__ == "__main__":
    main()
