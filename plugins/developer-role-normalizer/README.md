# developer-role-normalizer

A request normalizer plugin that converts `developer` message roles to `system` for OpenAI-compatible providers.

## Problem

DeepSeek and some other OpenAI-compatible providers do not support the `developer` message role (only `system`/`user`/`assistant`/`tool`). When requests pass through the openai-to-openai translator, `developer`-role messages are forwarded unchanged, causing HTTP 400 errors from these providers.

## How It Works

The plugin declares the `request_normalizer` capability. On each request, it checks the target format (`ToFormat`). When the target is `openai` or `codex`, it scans the `messages` array in the JSON payload and replaces all `"role":"developer"` with `"role":"system"` using a single-pass byte copy — no per-message allocations.

## Configuration

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
      enabled: true   # plugin-owned field: set false to disable normalization
```

The host-managed `plugins.configs.developer-role-normalizer.enabled` controls whether the plugin is loaded. The plugin-owned `enabled` field provides an additional fine-grained switch for the normalization logic itself.

## Build

```bash
cd plugins/developer-role-normalizer/go
CGO_ENABLED=1 go build -buildmode=c-shared -o developer-role-normalizer.so .
```

The plugin must be compiled with `CGO_ENABLED=1` since it uses cgo for the C ABI export.

## Install via Plugin Store

Register this store in your CPA config:

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

Then install via Management API:

```bash
curl -X POST http://localhost:8317/v0/management/plugin-store/developer-role-normalizer/install \
  -H "Authorization: Bearer <manag...n```

## Manual Install

Build the dynamic library for your target platform and place it in the plugin directory:

```
plugins/linux/amd64/developer-role-normalizer-v0.1.0.so
```

## License

MIT
