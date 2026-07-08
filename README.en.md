# CLIProxyAPI Plugins Store

[中文](README.md) | English

This is a third-party plugin store repository for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). It maintains both a schema v1 registry for older CPA builds and a schema v2 direct-install registry for current CPA builds, and mirrors registries plus release assets to jsDelivr CDN.

## Available plugins

| Plugin | Description |
|--------|-------------|
| [developer-role-normalizer](plugins/developer-role-normalizer/) | Converts the unsupported `developer` message role to `system` for selected OpenAI-compatible providers such as DeepSeek. |
| [cpa-manager-plus](plugins/cpa-manager-plus/) | Provides Manager Plus style dashboard, usage analytics, monitoring, account inspection, and settings pages inside CPA, proxied to Manager Server. |

## Recommended registry entry points

### CPA v7.2.46+: CDN + schema v2 recommended

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json"
```

This is the primary entry point:

- The registry is served through jsDelivr CDN.
- `registry-v2.json` uses `install.type = direct`.
- Each plugin is pinned to its own version and platform artifact URLs, no longer depending on GitHub `releases/latest`.
- The v2 registry on the `cdn` branch points plugin zip URLs to jsDelivr CDN assets.

### CPA v7.2.46+: GitHub raw fallback

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

This is also schema v2 direct install, but artifact URLs point to GitHub Releases. Use it when debugging CDN issues.

### Older CPA builds: schema v1 compatibility

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

Schema v1 remains available for older CPA builds. Note that v1 uses GitHub `releases/latest`, which is awkward for a multi-plugin repository.

## CPA version recommendation

| CPA version | Recommended registry | Notes |
|-------------|----------------------|-------|
| `< v7.2.44` | `registry.json` | These builds predate schema v2 direct install. |
| `v7.2.44` - `v7.2.45` | `registry.json`, or test `registry-v2.json` carefully | The first direct-install implementation exists, but later download/auth/error handling fixes are missing. |
| `>= v7.2.46` | `registry-v2.json`, preferably the CDN URL | Recommended path. Includes direct install plus follow-up plugin-store fixes. |

Evidence: upstream CPA commit `1f16e87` is included from `v7.2.44`; follow-up plugin-store fixes `3ea7f18`, `8970873`, and `caf7052` are included from `v7.2.46`.

## Architecture

```text
CPA
 └─ plugin store registry
     ├─ registry.json       schema v1, old CPA compatibility, GitHub Release latest model
     └─ registry-v2.json    schema v2, recommended for new CPA, direct install model

GitHub Actions
 ├─ discovers plugins whose source version matches the pushed tag
 ├─ builds linux/darwin/windows × amd64/arm64 dynamic-library zips
 ├─ publishes GitHub Release
 ├─ refreshes registry-v2.json on main branch (GitHub Release URLs)
 └─ publishes cdn branch
     ├─ registry.json
     ├─ registry-v2.json (jsDelivr artifact URLs)
     ├─ latest/
     └─ vX.Y.Z/

jsDelivr
 └─ https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/...
```

## CDN URL rules

GitHub raw file URL:

```text
https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}
```

Equivalent jsDelivr URL:

```text
https://cdn.jsdelivr.net/gh/{owner}/{repo}@{branch}/{path}
```

For this repository, do not use `@main` as the primary CDN entry point. Use the generated `cdn` branch instead:

```text
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/v0.3.8/cpa-manager-plus_0.3.8_linux_amd64.zip
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/latest/checksums.txt
```

The `cdn` branch contains only distribution artifacts and generated registries, so it is stable for jsDelivr caching.

## jsDelivr cache and purge

- Versioned paths such as `@cdn/v0.3.8/...` should be treated as immutable and production-safe.
- Mutable paths such as `@cdn/latest/...` and `@cdn/registry-v2.json` may have CDN propagation delay.
- The workflow automatically purges these mutable paths:
  - `registry.json`
  - `registry-v2.json`
  - `latest/checksums.txt`
  - `latest/*.zip`

Manual purge example:

```text
https://purge.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

## Release process

Releases use standard semver tags such as `v0.3.8`. Do not use plugin-suffixed tags such as `v0.3.8-cpa-manager-plus`.

1. Change plugin code.
2. Choose a new version, for example `0.3.9`.
3. Synchronize versions:
   - `plugins/<plugin-id>/go/main.go` -> `var pluginVersion = "0.3.9"`
   - `registry.json` -> that plugin's `"version"`
   - `plugins/<plugin-id>/Makefile` -> `VERSION := 0.3.9` when present
4. Commit and push to `main`.
5. Create and push the tag:

```bash
git tag -a v0.3.9 -m "v0.3.9"
git push origin v0.3.9
```

6. GitHub Actions automatically:
   - Builds only plugins whose source version equals the tag version.
   - Creates six platform zips per matching plugin.
   - Publishes GitHub Release.
   - Generates `registry-v2.json` on main.
   - Publishes/refreshes the `cdn` branch.
   - Purges mutable jsDelivr paths.

## Artifact contract

Every released plugin version must include six platforms:

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

The zip root must contain the dynamic library:

```text
{plugin-id}-v{version}.{so|dylib|dll}
```

Example:

```text
cpa-manager-plus_0.3.8_linux_amd64.zip
└── cpa-manager-plus-v0.3.8.so
```

## Local verification

```bash
python3 -m py_compile scripts/generate-registry-v2.py
scripts/generate-registry-v2.py --check
python3 - <<'PY'
import json
for f in ['registry.json', 'registry-v2.json']:
    j = json.load(open(f))
    print(f, j['schema_version'], len(j['plugins']))
PY
```

## Detailed docs

- [registry / CDN 分发策略](docs/registry-schema-strategy.md)
- [Registry / CDN distribution strategy](docs/registry-schema-strategy.en.md)
- [developer-role-normalizer](plugins/developer-role-normalizer/)
- [cpa-manager-plus](plugins/cpa-manager-plus/)

## License

MIT
