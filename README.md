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

Store releases are **one semver tag → one GitHub Release** containing **all** plugins under `plugins/*/go`. CPA picks assets from `releases/latest` on the repo URL in `registry.json`.

### Release checklist (order matters)

1. **Change the plugin** (if this release includes code or metadata fixes).
2. **Choose the new semver** (e.g. `0.1.4`). It must be **greater than any existing `v*` tag** on this repo.
3. **Bump version everywhere** for plugins you are shipping (same string, no `v` prefix):
   - `registry.json` → each affected plugin's `"version"`
   - `plugins/<plugin-id>/go/main.go` → `var pluginVersion = "…"`
   - `plugins/<plugin-id>/Makefile` → `VERSION := …` (if the plugin has a Makefile)
4. **Commit and push to `main`** so the tag points at sources that already declare that version.
5. **Create and push the tag manually** (tag push triggers CI; do not rely on CI to invent the version):

   ```bash
   git tag -a v0.1.4 -m "v0.1.4"
   git push origin v0.1.4
   ```

6. **Watch the workflow** [Build and Release Plugins](https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/actions) until `discover`, all `build` matrix jobs, and `release` succeed. The release job uploads every `*.zip` and `checksums.txt`.

### What CI does (after the tag)

- Parses `VERSION` from the tag (`v0.1.4` → `0.1.4`). Tags must look like `v0.1.0` — **no** per-plugin suffix tags (e.g. `v0.1.0-cpa-manager-plus`).
- Builds each plugin for linux/darwin/windows × amd64/arm64.
- Names artifacts `<plugin-id>_<version>_<goos>_<goarch>.zip` with `<plugin-id>-v<version>.{so,dylib,dll}` inside.
- Sets link-time `-X main.pluginVersion=<version>` so CPA sees the same version in plugin metadata as in the zip name (must match what you set in source in step 3).

### Adding a new plugin (first time)

1. Add `plugins/<plugin-id>/` with `go/go.mod` and source.
2. Add a `registry.json` entry (`repository` = this store repo).
3. Follow the **Release checklist** above for the first tag that ships it.

### Zip / checksums examples

```
developer-role-normalizer_0.1.4_linux_amd64.zip
└── developer-role-normalizer-v0.1.4.so
```

```
<sha256>  developer-role-normalizer_0.1.4_linux_amd64.zip
```

The registry `repository` field must be `https://github.com/{owner}/{repo}` so CPA can call the GitHub Releases API.

## License

MIT
