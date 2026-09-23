# CLIProxyAPI Plugins Store

[中文](README.md) | English

This is a third-party plugin store repository for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI), requiring CPA `v7.2.46+`. It publishes only the schema v2 direct-install registry (`registry-v2.json`) and mirrors that registry plus release assets to jsDelivr CDN. **Schema v1 is no longer supported as of 2026-09-18.** Every plugin keeps its own version line and is released with a plugin-scoped tag.

## Available plugins

| Plugin | Description |
|--------|-------------|
| [developer-role-normalizer](plugins/developer-role-normalizer/) | Converts the unsupported `developer` message role to `system` for selected OpenAI-compatible providers such as DeepSeek. |
| [cpa-manager-plus](plugins/cpa-manager-plus/) | In-process local Runtime providing Manager Plus dashboard, usage analytics, monitoring, account inspection, and Codex quota window keeper inside CPA. |
| [codex-oauth-base-url](plugins/codex-oauth-base-url/) | Rewrites the upstream base URL of Codex OAuth (ChatGPT subscription) credentials so a Codex account can target a self-hosted or third-party upstream without patching CPA. Inference requests only; login and token refresh still use the fixed `auth.openai.com` endpoints. |
| [cursor-oauth](plugins/cursor-oauth/) | Connects user-authorized Cursor subscription accounts to the OpenAI-compatible `/v1/chat/completions` API, with thinking stream passthrough (`reasoning_content`), system constraint injection, session checkpoint reuse, and official branding. |

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

### Retired: schema v1

The schema v1 entry point (`registry.json`) is gone: this repository neither publishes nor maintains v1 anymore, and the `registry.json` on the `cdn` branch is removed and purged at the next release.

A CPA build still on v1 must first upgrade to `v7.2.46+` and then use the v2 entry point above.

## CPA version recommendation

| CPA version | Supported | Notes |
|-------------|-----------|-------|
| `< v7.2.46` | No | Missing direct install or the follow-up plugin-store fixes. Schema v1 is retired, so upgrade CPA first. |
| `>= v7.2.46` | Yes | Use `registry-v2.json`, preferably the CDN URL. Includes direct install plus follow-up plugin-store fixes. |

Evidence: upstream CPA commit `1f16e87` is included from `v7.2.44`; follow-up plugin-store fixes `3ea7f18`, `8970873`, and `caf7052` are included from `v7.2.46`.

## Architecture

```text
CPA
 └─ plugin store registry
     └─ registry-v2.json    schema v2 direct install (the only published registry)

plugins.json               repository source manifest, used only to generate registry-v2.json

GitHub Actions
 ├─ discovers the plugins targeted by the pushed tag (one plugin per plugin-scoped tag)
 ├─ builds linux/darwin/windows × amd64/arm64 dynamic-library zips
 ├─ publishes GitHub Release
 ├─ refreshes registry-v2.json on main branch (GitHub Release URLs)
 └─ publishes cdn branch
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
  - `registry-v2.json`
  - `latest/checksums.txt`
  - `latest/*.zip`
  - `registry.json` (only to flush the retired v1 file out of the jsDelivr cache)

Manual purge example:

```text
https://purge.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

## Release process

Releases use plugin-scoped tags only: `<plugin-id>-v<version>`. Every plugin owns an independent version sequence, and numbers are never reused across plugins — even when a number is already taken by another plugin (for example `v0.1.0` belongs to developer-role-normalizer), a new plugin can still start at `0.1.0`.

The shared `v<version>` release train was retired on 2026-09-18: pushing such a tag is rejected by the workflow, so use `<plugin-id>-v<version>` instead. Do not put the plugin name after the version (such as `v0.3.8-cpa-manager-plus`) either — CI does not recognise that form.

The existing plugins (cpa-manager-plus, developer-role-normalizer) keep their current version numbers and their releases stay downloadable; each uses a plugin-scoped tag from its next version on.

1. Change plugin code.
2. Choose a new version, for example `0.3.9`.
3. Synchronize versions:
   - `plugins/<plugin-id>/go/main.go` -> `var pluginVersion = "0.3.9"`
   - `plugins.json` -> that plugin's `"version"`
   - `plugins/<plugin-id>/Makefile` -> `VERSION := 0.3.9` when present
4. Commit and push to `main`.
5. Create and push the tag:

```bash
git tag -a cpa-manager-plus-v0.3.9 -m "cpa-manager-plus 0.3.9"
git push origin cpa-manager-plus-v0.3.9
```

6. GitHub Actions automatically:
   - Builds only the single plugin that tag names, and checks its source version matches the tag.
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
manifest = json.load(open('plugins.json'))
print('plugins.json', [(p['id'], p['version']) for p in manifest['plugins']])
registry = json.load(open('registry-v2.json'))
print('registry-v2.json', registry['schema_version'], len(registry['plugins']))
PY
```

## Detailed docs

- [registry / CDN 分发策略](docs/registry-schema-strategy.md)
- [Registry / CDN distribution strategy](docs/registry-schema-strategy.en.md)
- [developer-role-normalizer](plugins/developer-role-normalizer/)
- [cpa-manager-plus](plugins/cpa-manager-plus/)
- [codex-oauth-base-url](plugins/codex-oauth-base-url/)

## License

MIT
