# CLIProxyAPI Plugins Store

Custom plugin store registry for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI).

## Structure

```
.
├── registry.json                          # Plugin store registry (consumed by CPA)
├── README.md                              # This file
└── plugins/
    ├── cpa-manager-plus/                 # CPA Manager Plus plugin mirror
    │   ├── README.md
    │   ├── Makefile
    │   ├── embed.go
    │   ├── go.mod
    │   ├── go.sum
    │   ├── main.go
    │   └── web/
    │       └── index.html
    └── developer-role-normalizer/         # One subdirectory per plugin
        ├── README.md                      # Plugin documentation
        └── go/                            # Plugin source code
            ├── go.mod
            ├── go.sum
            └── main.go
```

## Available Plugins

| Plugin | Description |
|--------|-------------|
| [developer-role-normalizer](plugins/developer-role-normalizer/) | Converts `developer` message roles to `system` for OpenAI-compatible providers that don't recognize the `developer` role. |
| [cpa-manager-plus](plugins/cpa-manager-plus/) | Embeds CPA Manager Plus inside CPA and proxies panel calls to a Manager Server. |

## Using This Store

### 1. Register the store in CPA config

Add this registry URL to your `config.yaml`:

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

`registry.json` stays on `schema_version: 1` for maximum compatibility with older CPA builds.
For CPA v7.2.46 and newer, use the schema v2 direct-install registry instead:

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

The built-in official store is always included; this adds a third-party source alongside it.

### 2. Browse and install via Management API

```bash
# List available plugins from all stores
curl http://localhost:8317/v0/management/plugin-store \
  -H "Authorization: Bearer ***"

# Install a specific plugin
curl -X POST http://localhost:8317/v0/management/plugin-store/cpa-manager-plus/install \
  -H "Authorization: Bearer ***"
```

### 3. Verify installation

```bash
curl http://localhost:8317/v0/management/plugins \
  -H "Authorization: Bearer ***"
```

Check that `registered: true` and `effective_enabled: true` for the installed plugin.

## Registry schemas

This repository publishes two registry entry points:

| File | Schema | Install model | Use case |
|------|--------|---------------|----------|
| `registry.json` | v1 | GitHub Release `latest` | Maximum compatibility with older CPA builds, especially `< v7.2.44`. |
| `registry-v2.json` | v2 | Direct install artifacts | Recommended for CPA `>= v7.2.46`, independent plugin versions, and fewer `releases/latest` failure modes. |

Detailed local strategy notes: [`docs/registry-schema-strategy.md`](docs/registry-schema-strategy.md).

### Schema v1 compatibility registry

`registry.json` keeps the legacy GitHub Release model. CPA resolves each plugin by calling GitHub `releases/latest` on the plugin's `repository`, then downloads `{plugin-id}_{release-version}_{goos}_{goarch}.zip` and `checksums.txt` from that latest release.

Because all plugins currently share this store repository, v1 has a hard limitation: the latest release must contain the assets for every plugin that users may install from this repository. If a release only includes one changed plugin, v1 installs for unchanged plugins can fail because their zip is absent from `releases/latest`.

### Schema v2 direct registry

Use this registry for CPA v7.2.46+. Tags v7.2.44-v7.2.45 contain the first direct-install implementation, but v7.2.46 includes the follow-up plugin-store asset download/auth/error-handling fixes, so that is the practical minimum recommendation.

`registry-v2.json` uses schema v2 direct install entries:

```json
{
  "install": {
    "type": "direct",
    "artifacts": [
      {
        "goos": "linux",
        "goarch": "amd64",
        "url": "https://github.com/.../releases/download/v0.3.7/cpa-manager-plus_0.3.7_linux_amd64.zip",
        "sha256": "...",
        "size": 5364606
      }
    ]
  }
}
```

CPA selects the artifact matching its runtime `GOOS/GOARCH`, downloads the pinned URL directly, and verifies the inline `sha256`. This avoids the v1 shared-`latest` problem and lets each plugin keep its own version in one shared store.

`registry-v2.json` is generated, not hand-maintained:

```bash
scripts/generate-registry-v2.py
```

The release workflow runs this script after a successful release and commits `registry-v2.json` back to `main` when artifact URLs/checksums change.

## Publishing a release

Store releases use **standard semver tags** (e.g. `v1.2.0`). One tag = one release, and only plugins whose hardcoded `var pluginVersion` matches the tag version are built. Schema v2 pins each plugin to its own versioned assets after release; schema v1 users still depend on `releases/latest`.

### Tag format

```text
v<version>
```

Example:

```text
v1.2.0
```

This builds every plugin whose `go/main.go` declares `var pluginVersion = "1.2.0"`. If no plugin matches, CI fails with a list of available plugin versions.

### Release checklist (order matters)

1. **Change the plugin** (if this release includes code or metadata fixes).
2. **Choose the new version** (e.g. `1.2.0`).
3. **Bump version everywhere** for the plugin(s) you are releasing (same string, no `v` prefix):
   - `plugins/<plugin-id>/go/main.go` → `var pluginVersion = "…"`
   - `registry.json` → that plugin's `"version"`
   - `plugins/<plugin-id>/Makefile` → `VERSION := …` (if the plugin has a Makefile)
4. **Commit and push to `main`** so the tag points at sources that already declare that version.
5. **Create and push the tag manually** (tag push triggers CI):

   ```bash
   git tag -a v1.2.0 -m "v1.2.0"
   git push origin v1.2.0
   ```

6. **Watch the workflow** [Build and Release Matching Plugins](https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/actions) until `discover`, all `build` matrix jobs, and `release` succeed.

### What CI does (after the tag)

- Parses `VERSION` from the tag (`v1.2.0` → `1.2.0`).
- Scans every `plugins/*/go/main.go` for `var pluginVersion` and builds only plugins whose version matches the tag.
- Each matching plugin is built for linux/darwin/windows × amd64/arm64 (6 platform zips each).
- If a plugin has `web/package.json`, runs `npm ci && npm run build` before Go build.
- Names artifacts `<plugin-id>_<version>_<goos>_<goarch>.zip` with `<plugin-id>-v<version>.{so,dylib,dll}` inside.
- Merges all checksums into one `checksums.txt` and uploads with the release.
- Regenerates `registry-v2.json` from `registry.json` plus GitHub release assets and commits it back to `main` if changed.

### Schema v2 maintenance rules

- Do not edit `registry-v2.json` by hand. Edit `registry.json` and plugin source versions, publish the release, then run or let CI run `scripts/generate-registry-v2.py`.
- Every plugin listed in `registry.json` must have a release matching its own `version` field, with all six standard platform zips.
- A plugin can stay on an older version while another plugin advances. The v2 registry will keep the unchanged plugin pinned to its older artifact URLs.
- For v1 compatibility, be aware that CPA still uses the repository latest release. If supporting old CPA builds is required, either keep latest releases complete for all plugins or direct old users to a release that contains the plugin they need.


### Adding a new plugin (first time)

1. Add `plugins/<plugin-id>/` with `go/go.mod` and source.
2. Add a `registry.json` entry (`repository` = this store repo for v1 compatibility; `version` = the plugin's own current version).
3. Publish a standard semver tag matching that plugin version.
4. Confirm `registry-v2.json` has a direct `install.artifacts` block for the new plugin after CI refreshes it.

### Zip / checksums examples

```
cpa-manager-plus_1.2.0_linux_amd64.zip
└── cpa-manager-plus-v1.2.0.so
```

```
<sha256>  cpa-manager-plus_1.2.0_linux_amd64.zip
```

The registry `repository` field must be `https://github.com/{owner}/{repo}` so CPA can call the GitHub Releases API.

## License

MIT
