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

## Publishing a Plugin

Each plugin lives under `plugins/<plugin-id>/`. To add a new plugin:

1. Create `plugins/<plugin-id>/` with the plugin source code.
2. Add an entry to `registry.json` with the plugin metadata.
3. Push tag **`v<version>`** (e.g. **`v0.1.0`**) on this repo. Workflow **Build and Release Plugins** builds **every** plugin under `plugins/*/go` for all platforms and attaches all zips plus **`checksums.txt`** to **one** GitHub Release on that tag.

   CPA install/update calls **`releases/latest`** on the `repository` URL in `registry.json` (this store repo). The release tag must normalize to a valid version (e.g. `v0.1.0` → `0.1.0`). Do **not** use suffix tags like `v0.1.0-plugin-name` — both plugins share the same latest release and are distinguished by asset names:

   - `<plugin-id>_<version>_<goos>_<goarch>.zip`
   - `checksums.txt`

The registry `repository` field must point to `https://github.com/{owner}/{repo}`. The CPA plugin store client fetches releases from that repository's GitHub Releases API.

### Zip format

```
developer-role-normalizer_0.1.0_linux_amd64.zip
└── developer-role-normalizer-v0.1.0.so      # dynamic library at zip root
```

### Checksums format

```
<sha256>  developer-role-normalizer_0.1.0_linux_amd64.zip
```

## License

MIT
