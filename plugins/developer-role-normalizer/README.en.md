# developer-role-normalizer

[中文](README.md) | English

`developer-role-normalizer` is a CPA request normalizer plugin for OpenAI-compatible upstream providers that do not support the `developer` message role. Before CPA forwards a request upstream, the plugin rewrites matched `messages[*].role == "developer"` values to `system`.

The default policy is conservative: it only handles target formats `openai` / `codex` and model names containing `deepseek`.

## Problem solved

Some OpenAI-compatible services expose Chat Completions style APIs but do not accept the newer `developer` role. Some DeepSeek-compatible targets only accept:

```text
system / user / assistant / tool
```

A request containing:

```json
{"role":"developer","content":"Follow these rules..."}
```

may fail upstream with HTTP 400 or a similar validation error. This plugin performs a narrow compatibility rewrite only for configured targets, without affecting models that natively support `developer`.

## Architecture

```text
client request
    │
    ▼
CPA protocol translation / routing
    │
    ▼
request.normalize hook
    │
    ├─ read plugin config
    ├─ check target format: openai/codex by default
    ├─ check model name: contains deepseek by default
    ├─ check strategy: role_to_system
    │
    ▼
rewrite messages[*].role == "developer" to "system"
    │
    ▼
upstream OpenAI-compatible provider
```

## Runtime flow

1. CPA loads the dynamic library and calls `plugin.register`.
2. The plugin declares the `request_normalizer` capability.
3. On config changes, CPA calls `plugin.reconfigure` with `plugins.configs.developer-role-normalizer`.
4. For each normalization call, CPA passes target format, model name, and request body.
5. The plugin checks target format, model matching, enable state, and strategy.
6. If matched, only exact role values in the top-level `messages` array are rewritten.
7. If not matched, or if the body shape is unexpected, the original body is returned unchanged.

## Configuration

### Minimal configuration

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
```

The defaults are equivalent to:

```yaml
normalize_enabled: true
target_formats:
  - openai
  - codex
model_match:
  mode: contains
  include:
    - deepseek
  exclude: []
strategy: role_to_system
```

### Full configuration example

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
      normalize_enabled: true
      target_formats:
        - openai
        - codex
      model_match:
        mode: contains
        include:
          - deepseek
          - qwen
        exclude:
          - deepseek-official-compatible
      strategy: role_to_system
```

### Field reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | controlled by CPA | CPA plugin enable switch. |
| `normalize_enabled` | boolean | `true` | Plugin logic switch; `false` disables rewriting. |
| `target_formats` | array/string | `[openai, codex]` | Target protocol formats eligible for normalization. |
| `model_match.mode` | enum | `contains` | Supports `contains`, `exact`, `prefix`, and `suffix`. |
| `model_match.include` | array/string | `[deepseek]` | The model must match at least one include pattern; an empty array means include all. |
| `model_match.exclude` | array/string | `[]` | Exclude patterns; exclude has higher priority than include. |
| `strategy` | enum | `role_to_system` | Currently only `role_to_system` is supported. |

Array fields also accept comma-separated strings:

```yaml
target_formats: openai,codex
model_match:
  include: deepseek,qwen
  exclude: deepseek-official-compatible
```

## Behavior example

Input:

```json
{
  "model": "deepseek-chat",
  "messages": [
    {"role": "developer", "content": "You are concise."},
    {"role": "user", "content": "Hello"}
  ]
}
```

Output:

```json
{
  "model": "deepseek-chat",
  "messages": [
    {"role": "system", "content": "You are concise."},
    {"role": "user", "content": "Hello"}
  ]
}
```

If the model does not match, for example `gpt-4.1`, the request is returned unchanged.

## Installation

### Recommended: CPA v7.2.46+ with CDN v2 registry

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json"
```

### GitHub raw fallback

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

### Older CPA compatibility entry point

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

Install:

```bash
curl -X POST http://localhost:8317/v0/management/plugin-store/developer-role-normalizer/install \
  -H "Authorization: Bearer ***"
```

Enable:

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
```

## Build and verification

```bash
cd plugins/developer-role-normalizer/go
go test ./...
go vet ./...
CGO_ENABLED=1 go build -buildmode=c-shared -o developer-role-normalizer.dylib .
nm developer-role-normalizer.dylib | grep cliproxy_plugin_init
```

The plugin must be built with `CGO_ENABLED=1` because it exports the CPA C ABI through cgo.

## Manual install

Place the dynamic library in the CPA plugin directory, for example:

```text
plugins/linux/amd64/developer-role-normalizer-v0.3.8.so
```

Restart CPA after installing or upgrading; already loaded dynamic libraries are not hot-swapped.

## Compatibility notes

- By default, only `openai` / `codex` target formats and model names containing `deepseek` are normalized.
- Model matching is case-insensitive.
- The plugin uses the transform request `Model` first and falls back to `body.model`.
- The plugin does not merge message contents; it only replaces role fields in the top-level `messages` array.
- Requests without a valid top-level `messages` array are returned unchanged.

## License

MIT
