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

Store releases are **one plugin per tag**. A tag builds and uploads assets only for the plugin encoded in the tag suffix. CPA picks assets from `releases/latest` on the repo URL in `registry.json`.

### Tag format

```text
v<version>-<plugin-id>
```

Example:

```text
v1.2.0-cpa-manager-plus
```

This builds only:

```text
plugins/cpa-manager-plus/go
```

### Release checklist (order matters)

1. **Change the plugin** (if this release includes code or metadata fixes).
2. **Choose the plugin version** (e.g. `1.2.0`).
3. **Bump version everywhere for that plugin only** (same string, no `v` prefix):
   - `registry.json` → that plugin's `"version"`
   - `plugins/<plugin-id>/go/main.go` → `var pluginVersion = "…"`
   - `plugins/<plugin-id>/Makefile` → `VERSION := …` (if the plugin has a Makefile)
4. **Commit and push to `main`** so the tag points at sources that already declare that version.
5. **Create and push the plugin tag manually** (tag push triggers CI; do not rely on CI to invent the version):

   ```bash
   git tag -a v1.2.0-cpa-manager-plus -m "cpa-manager-plus v1.2.0"
   git push origin v1.2.0-cpa-manager-plus
   ```

6. **Watch the workflow** [Build and Release Plugin](https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/actions) until `discover`, the six-platform `build` matrix, and `release` succeed. The release job uploads that plugin's `*.zip` files and `checksums.txt`.

### What CI does (after the tag)

- Parses `VERSION` and `PLUGIN_ID` from the tag (`v1.2.0-cpa-manager-plus` → `VERSION=1.2.0`, `PLUGIN_ID=cpa-manager-plus`).
- Builds only that plugin for linux/darwin/windows × amd64/arm64.
- If the plugin has `web/package.json`, runs `npm ci && npm run build` before Go build so Go embeds compiled frontend assets.
- Names artifacts `<plugin-id>_<version>_<goos>_<goarch>.zip` with `<plugin-id>-v<version>.{so,dylib,dll}` inside.
- Sets link-time `-X main.pluginVersion=<version>` so CPA sees the same version in plugin metadata as in the zip name.

### Adding a new plugin (first time)

1. Add `plugins/<plugin-id>/` with `go/go.mod` and source.
2. Add a `registry.json` entry (`repository` = this store repo).
3. Follow the **Release checklist** above with a plugin-specific tag.

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
