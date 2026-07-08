# Registry schema strategy

This store intentionally publishes two registry entry points:

- `registry.json` — schema v1 compatibility registry
- `registry-v2.json` — schema v2 direct-install registry

The long-term default for maintained deployments should be schema v2 direct install. Schema v1 remains available because older CPA builds only understand the GitHub Release `latest` install model.

## CPA version recommendation

Use this rule for users:

| CPA version | Recommended registry | Reason |
|-------------|----------------------|--------|
| `< v7.2.44` | `registry.json` | These builds predate registry schema v2 direct install support. |
| `v7.2.44` - `v7.2.45` | `registry.json`, or test `registry-v2.json` carefully | Direct install support exists, but later plugin-store download/auth fixes are not included. |
| `>= v7.2.46` | `registry-v2.json` | Recommended. Includes direct install support plus follow-up asset download/auth/error-handling fixes. |

Evidence from upstream CPA history:

- `1f16e87` (`feat(pluginstore): introduce support for direct install type and version management`) is contained from tag `v7.2.44` onward.
- Follow-up plugin store fixes (`3ea7f18`, `8970873`, `caf7052`) are contained from tag `v7.2.46` onward.

So the user-facing recommendation should be: **CPA v7.2.46+ should use `registry-v2.json`; older CPA should use `registry.json`.**

## User-facing registry URLs

### Recommended for CPA v7.2.46+

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

### Compatibility fallback for older CPA

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

## Why v2 is the sustainable model

Schema v1 uses the legacy GitHub Release `latest` model:

1. CPA reads the plugin entry from `registry.json`.
2. CPA calls GitHub `GET /repos/{owner}/{repo}/releases/latest` using the plugin `repository` field.
3. CPA parses the release tag as the plugin version.
4. CPA downloads `{plugin-id}_{version}_{goos}_{goarch}.zip` and `checksums.txt` from that single latest release.

That breaks down for a multi-plugin store where plugins have independent versions. If the latest release contains only plugin A, v1 installation for plugin B can fail because plugin B's zip is absent from latest. To keep v1 perfectly compatible, every latest release would need to rebuild and upload all plugin zips, including unchanged plugins.

Schema v2 direct install solves that:

1. Each plugin entry declares its own `version`.
2. Each plugin entry declares direct artifacts per `goos/goarch`.
3. Each artifact has a stable URL, sha256, and size.
4. CPA downloads the exact artifact URL and verifies inline sha256.

This means plugin A can advance to `1.2.0` while plugin B stays pinned to `1.1.0`, even inside the same store repository.

## Registry roles

### `registry.json`

Purpose:

- Compatibility for old CPA builds.
- Minimal metadata plus GitHub `repository` field.
- Must remain schema v1 unless we deliberately drop old CPA support.

Constraints:

- Shared repository means shared `releases/latest`.
- Best-effort compatibility only when latest release contains the requested plugin's zip.
- For full v1 compatibility, publish latest releases containing all listed plugins.

### `registry-v2.json`

Purpose:

- Primary registry for current/new CPA deployments.
- Uses `install.type = direct`.
- Pins each plugin to its own artifact URLs.
- Avoids the shared latest-release coupling.
- Reduces GitHub API/latest lookup failure modes.

Do not hand-edit this file. Generate it from `registry.json` and release assets:

```bash
scripts/generate-registry-v2.py
```

Check mode:

```bash
scripts/generate-registry-v2.py --check
```

The release workflow runs this script after publishing assets and commits `registry-v2.json` back to `main` if changed.

## Required artifact contract

Every plugin version listed in `registry.json` must have all six standard platform zips in its matching release tag:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

Release asset name:

```text
{plugin-id}_{version}_{goos}_{goarch}.zip
```

Zip root must contain exactly the target dynamic library, named either:

```text
{plugin-id}.{ext}
```

or:

```text
{plugin-id}-v{version}.{ext}
```

Examples:

```text
cpa-manager-plus_0.3.7_linux_amd64.zip
└── cpa-manager-plus-v0.3.7.so
```

```text
developer-role-normalizer_0.3.0_darwin_arm64.zip
└── developer-role-normalizer-v0.3.0.dylib
```

## Release workflow policy

The current sustainable policy is:

1. Keep each plugin's source version independent.
2. Build only plugins whose `var pluginVersion` matches the pushed tag.
3. Keep `registry.json` as the compatibility source of plugin metadata and current plugin version.
4. Generate `registry-v2.json` after releases so each plugin is pinned to its own latest declared version.
5. Recommend v2 to users on CPA v7.2.46+.

This gives us independent plugin distribution without forcing unchanged plugins to be rebuilt for every release.

## Adding a new plugin

1. Add plugin source under `plugins/<plugin-id>/go/`.
2. Add a `registry.json` entry with `version`, `repository`, and metadata.
3. Publish a standard semver tag matching that plugin version, for example `v0.1.0`.
4. Wait for CI to build platform zips and publish release assets.
5. Let CI regenerate `registry-v2.json`, or run `scripts/generate-registry-v2.py` manually.
6. Verify `registry-v2.json` contains a direct `install.artifacts` block for the new plugin.

## Troubleshooting install failures

If users see plugin installation 502 errors:

1. If CPA is `>= v7.2.46`, switch to `registry-v2.json` first.
2. If CPA is older than `v7.2.44`, upgrade CPA or stay on `registry.json`.
3. If using schema v1, check whether the current GitHub latest release contains the requested plugin zip.
4. If using schema v2, check whether the artifact URL is reachable from the CPA runtime environment, especially inside Docker.
5. For Docker deployments, persist the plugins directory with a volume mount so installed libraries survive container recreation.
