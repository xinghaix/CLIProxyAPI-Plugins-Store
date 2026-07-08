#!/usr/bin/env python3
"""Generate schema_version=2 direct-install registry from registry.json.

The v1 registry keeps CPA's legacy GitHub release model. This script builds the
v2 registry that pins each plugin to its own versioned release assets so plugins
can be released independently in a shared store repository.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

DEFAULT_PLATFORMS: tuple[tuple[str, str], ...] = (
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
    ("windows", "arm64"),
)

COPY_PLUGIN_FIELDS = (
    "id",
    "name",
    "description",
    "author",
    "version",
    "repository",
    "logo",
    "homepage",
    "license",
    "tags",
    "auth_required",
)


def github_token() -> str:
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    if token:
        return token.strip()
    try:
        result = subprocess.run(
            ["gh", "auth", "token"],
            check=False,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            timeout=10,
        )
    except (OSError, subprocess.SubprocessError):
        return ""
    if result.returncode != 0:
        return ""
    return result.stdout.strip()


def github_headers(accept: str = "application/vnd.github+json") -> dict[str, str]:
    headers = {
        "Accept": accept,
        "User-Agent": "CLIProxyAPI-Plugins-Store-registry-v2-generator",
    }
    token = github_token()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    return headers


def fetch_bytes(url: str, accept: str = "application/octet-stream") -> bytes:
    req = urllib.request.Request(url, headers=github_headers(accept))
    try:
        with urllib.request.urlopen(req, timeout=60) as response:
            return response.read()
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", "replace")[:500]
        raise RuntimeError(f"GET {url} failed: HTTP {exc.code}: {body}") from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"GET {url} failed: {exc}") from exc


def fetch_json(url: str) -> dict[str, Any]:
    return json.loads(fetch_bytes(url, "application/vnd.github+json").decode("utf-8"))


def github_repo_parts(repository: str) -> tuple[str, str]:
    parsed = urllib.parse.urlparse(repository.strip())
    parts = [part for part in parsed.path.split("/") if part]
    if parsed.scheme != "https" or parsed.netloc != "github.com" or len(parts) != 2:
        raise ValueError(f"repository must be https://github.com/{{owner}}/{{repo}}: {repository!r}")
    return parts[0], parts[1]


def normalize_version(version: str) -> str:
    version = version.strip()
    if len(version) > 1 and version[0] in "vV":
        return version[1:]
    return version


def release_tag_for(version: str) -> str:
    version = normalize_version(version)
    return f"v{version}"


def release_by_tag(repository: str, version: str) -> dict[str, Any]:
    owner, repo = github_repo_parts(repository)
    tag = release_tag_for(version)
    url = f"https://api.github.com/repos/{urllib.parse.quote(owner)}/{urllib.parse.quote(repo)}/releases/tags/{urllib.parse.quote(tag)}"
    return fetch_json(url)


def parse_checksums(data: bytes) -> dict[str, str]:
    out: dict[str, str] = {}
    for raw_line in data.decode("utf-8", "replace").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        fields = line.split()
        if len(fields) < 2:
            continue
        name = fields[1].lstrip("*")
        out[name] = fields[0].lower()
    return out


def checksum_map_from_release(release: dict[str, Any]) -> dict[str, str]:
    for asset in release.get("assets", []):
        if asset.get("name") == "checksums.txt" and asset.get("browser_download_url"):
            return parse_checksums(fetch_bytes(asset["browser_download_url"], "text/plain"))
    return {}


def artifact_sha256(asset: dict[str, Any], checksums: dict[str, str]) -> str:
    digest = str(asset.get("digest") or "").strip().lower()
    if digest.startswith("sha256:"):
        return digest.removeprefix("sha256:")
    name = str(asset.get("name") or "")
    if name in checksums:
        return checksums[name]
    raise ValueError(f"missing sha256 for asset {name}")


def format_artifact_url(
    template: str,
    *,
    asset_name: str,
    release_tag: str,
    plugin_id: str,
    version: str,
    goos: str,
    goarch: str,
    repository: str,
) -> str:
    if not template:
        return ""
    return template.format(
        asset_name=asset_name,
        tag=release_tag,
        release_tag=release_tag,
        plugin_id=plugin_id,
        version=version,
        goos=goos,
        goarch=goarch,
        repository=repository,
    )


def direct_artifacts(
    plugin: dict[str, Any],
    required_platforms: set[tuple[str, str]],
    artifact_url_template: str = "",
) -> list[dict[str, Any]]:
    plugin_id = plugin["id"].strip()
    version = normalize_version(plugin["version"])
    repository = plugin.get("repository", "").strip()
    if not repository:
        raise ValueError(f"{plugin_id}: repository is required to discover release assets")

    release = release_by_tag(repository, version)
    checksums = checksum_map_from_release(release)
    pattern = re.compile(
        r"^" + re.escape(plugin_id) + r"_" + re.escape(version) + r"_([^_]+)_([^_]+)\.zip$"
    )

    artifacts: list[dict[str, Any]] = []
    for asset in release.get("assets", []):
        name = str(asset.get("name") or "")
        match = pattern.match(name)
        if not match:
            continue
        goos, goarch = match.groups()
        url = format_artifact_url(
            artifact_url_template,
            asset_name=name,
            release_tag=str(release.get("tag_name") or release_tag_for(version)),
            plugin_id=plugin_id,
            version=version,
            goos=goos,
            goarch=goarch,
            repository=repository,
        ) or str(asset.get("browser_download_url") or asset.get("url") or "").strip()
        if not url:
            raise ValueError(f"{plugin_id}: asset {name} missing download URL")
        artifacts.append(
            {
                "goos": goos,
                "goarch": goarch,
                "url": url,
                "sha256": artifact_sha256(asset, checksums),
                "size": int(asset.get("size") or 0),
            }
        )

    if not artifacts:
        raise ValueError(f"{plugin_id}: no release assets found for version {version}")

    found = {(item["goos"], item["goarch"]) for item in artifacts}
    missing = sorted(required_platforms - found)
    if missing:
        formatted = ", ".join(f"{goos}/{goarch}" for goos, goarch in missing)
        raise ValueError(f"{plugin_id}: missing required artifacts for {formatted}")

    order = {platform: index for index, platform in enumerate(DEFAULT_PLATFORMS)}
    artifacts.sort(key=lambda item: (order.get((item["goos"], item["goarch"]), 999), item["goos"], item["goarch"]))
    return artifacts


def convert_plugin(
    plugin: dict[str, Any],
    required_platforms: set[tuple[str, str]],
    artifact_url_template: str = "",
) -> dict[str, Any]:
    converted = {key: plugin[key] for key in COPY_PLUGIN_FIELDS if key in plugin}
    converted["version"] = normalize_version(str(plugin.get("version") or ""))
    if not converted["version"]:
        raise ValueError(f"{plugin.get('id', '<unknown>')}: version is required for schema v2 direct install")
    converted["install"] = {
        "type": "direct",
        "artifacts": direct_artifacts(plugin, required_platforms, artifact_url_template),
    }
    return converted


def generate(
    source: Path,
    required_platforms: set[tuple[str, str]],
    artifact_url_template: str = "",
) -> dict[str, Any]:
    registry = json.loads(source.read_text(encoding="utf-8"))
    return {
        "schema_version": 2,
        "plugins": [
            convert_plugin(plugin, required_platforms, artifact_url_template)
            for plugin in registry.get("plugins", [])
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate schema_version=2 direct-install registry.")
    parser.add_argument("--source", default="registry.json", help="source v1 registry path")
    parser.add_argument("--output", default="registry-v2.json", help="output v2 registry path")
    parser.add_argument(
        "--artifact-url-template",
        default="",
        help=(
            "optional artifact URL template; supports {asset_name}, {tag}, {release_tag}, "
            "{plugin_id}, {version}, {goos}, {goarch}, {repository}. If omitted, uses GitHub release URLs."
        ),
    )
    parser.add_argument(
        "--allow-missing-platforms",
        action="store_true",
        help="do not require the standard six GOOS/GOARCH artifacts",
    )
    parser.add_argument("--check", action="store_true", help="fail if output is not already up to date")
    args = parser.parse_args()

    required_platforms = set() if args.allow_missing_platforms else set(DEFAULT_PLATFORMS)
    source = Path(args.source)
    output = Path(args.output)
    generated = generate(source, required_platforms, args.artifact_url_template)
    content = json.dumps(generated, ensure_ascii=False, indent=2) + "\n"

    if args.check:
        existing = output.read_text(encoding="utf-8") if output.exists() else ""
        if existing != content:
            print(f"{output} is not up to date; run scripts/generate-registry-v2.py", file=sys.stderr)
            return 1
        print(f"{output} is up to date")
        return 0

    output.write_text(content, encoding="utf-8")
    print(f"wrote {output} with {len(generated['plugins'])} plugin(s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
