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

## Publishing a release

Store releases use **standard semver tags** (e.g. `v1.2.0`). One tag = one release, and only plugins whose hardcoded `var pluginVersion` matches the tag version are built. CPA picks assets from `releases/latest` on the repo URL in `registry.json`.

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


### Adding a new plugin (first time)

1. Add `plugins/<plugin-id>/` with `go/go.mod` and source.
2. Add a `registry.json` entry (`repository` = this store repo).
3. Follow the **Release checklist** above with a standard semver tag.

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
