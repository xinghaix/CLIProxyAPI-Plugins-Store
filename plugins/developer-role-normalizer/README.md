# developer-role-normalizer

`developer-role-normalizer` is a CPA request normalizer plugin for OpenAI-compatible model providers that do not support the `developer` message role. It rewrites matched request payloads from `developer` role messages to `system` role messages before CPA sends the request upstream.

The default configuration follows **scheme B**: normalization is enabled, but it only applies to OpenAI-compatible target formats (`openai`, `codex`) and models whose identifier contains `deepseek`.

## Problem

Some OpenAI-compatible providers expose Chat Completions-style APIs but do not accept the newer `developer` role. For example, DeepSeek-compatible targets commonly support roles such as `system`, `user`, `assistant`, and `tool`, but may reject requests containing:

```json
{"role":"developer","content":"Follow these rules..."}
```

When CPA translates or forwards a request to those providers without role normalization, the upstream provider can return HTTP 400 or an equivalent validation error. This plugin provides a narrow compatibility layer for those targets without changing requests for models that already support `developer`.

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
    ├─ load plugin-owned config
    ├─ check target format: openai/codex by default
    ├─ check target model: contains deepseek by default
    ├─ check rewrite strategy: role_to_system
    │
    ▼
rewrite messages[*].role == "developer" to "system"
    │
    ▼
upstream OpenAI-compatible provider
```

### Runtime flow

1. CPA loads the dynamic plugin and calls `plugin.register`.
2. The plugin declares the `request_normalizer` capability.
3. CPA calls `plugin.reconfigure` with `plugins.configs.developer-role-normalizer` YAML whenever configuration changes.
4. For each request normalization call, the plugin receives a `RequestTransformRequest` containing:
   - `ToFormat`: target protocol format.
   - `Model`: selected target model when available.
   - `Body`: provider-bound JSON payload.
5. The plugin applies all gates before rewriting:
   - `normalize_enabled` must be true.
   - `ToFormat` must match `target_formats`.
   - `Model` or `body.model` must match `model_match.include` and must not match `model_match.exclude`.
   - `strategy` must be `role_to_system`.
6. If matched, the plugin scans the JSON `messages` array and replaces every exact message role value `"developer"` with `"system"`.
7. If no gate matches or no developer messages exist, the original payload is returned unchanged.

### Rewrite strategy

Current supported strategy:

| Strategy | Behavior |
| --- | --- |
| `role_to_system` | Converts each `messages[*].role == "developer"` value to `"system"`. Message order and content are preserved. |

The strategy field is intentionally explicit so future versions can add alternatives such as merging developer content into an existing system message if a provider requires stricter system-message structure.

## Configuration

### Recommended configuration

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
        exclude: []
      strategy: role_to_system
```

### Minimal DeepSeek-only configuration

Because scheme B is the default, this is enough after the plugin is installed:

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
```

With defaults, the plugin normalizes only when:

```text
normalize_enabled = true
ToFormat          = openai or codex
model             contains deepseek, case-insensitive
strategy          = role_to_system
```

### Configuration reference

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `enabled` | boolean | controlled by CPA | Host-managed plugin enable switch. For backward compatibility, when this value is passed to the plugin config, `false` also disables normalization and `true` leaves normalization enabled. Prefer `normalize_enabled` for plugin logic. |
| `normalize_enabled` | boolean | `true` | Plugin-owned logic switch. Set to `false` to keep the plugin loaded but return all request bodies unchanged. |
| `target_formats` | array or comma-separated string | `[openai, codex]` | Target protocol formats eligible for normalization. Matching is exact and case-insensitive after trimming. |
| `model_match.mode` | enum string | `contains` | Model matching mode. Supported values: `contains`, `exact`, `prefix`, `suffix`. Unknown values fall back to `contains`. |
| `model_match.include` | array or comma-separated string | `[deepseek]` | Include patterns. The model must match at least one include pattern. If explicitly set to an empty list, all models are included unless excluded. |
| `model_match.exclude` | array or comma-separated string | `[]` | Exclude patterns. Exclude has higher priority than include. |
| `strategy` | enum string | `role_to_system` | Rewrite behavior. Currently only `role_to_system` is supported. |

### Array and string forms

For easier management UI input, list fields accept both YAML arrays and comma-separated strings.

Array form:

```yaml
target_formats:
  - openai
  - codex
model_match:
  include:
    - deepseek
    - qwen
  exclude:
    - qwen-official-compatible
```

Comma-separated form:

```yaml
target_formats: openai,codex
model_match:
  include: deepseek,qwen
  exclude: qwen-official-compatible
```

### Matching examples

#### Only DeepSeek models, default behavior

```yaml
model_match:
  mode: contains
  include:
    - deepseek
  exclude: []
```

Matches:

- `deepseek-chat`
- `deepseek-reasoner`
- `provider/deepseek-v3`
- `deepseek/deepseek-r1`

Does not match:

- `gpt-4.1`
- `claude-sonnet-4-6`
- `qwen-plus`

#### Multiple incompatible OpenAI-compatible providers

```yaml
model_match:
  mode: contains
  include:
    - deepseek
    - qwen
    - kimi
  exclude: []
```

#### Exclude a known compatible model family

```yaml
model_match:
  mode: contains
  include:
    - deepseek
  exclude:
    - deepseek-official-compatible
```

A model named `deepseek-official-compatible-v1` will be skipped even though it contains `deepseek`.

#### Prefix matching

```yaml
model_match:
  mode: prefix
  include:
    - deepseek-
```

Matches `deepseek-chat`, but not `provider/deepseek-chat`.

#### Apply to every OpenAI-compatible target model

Use this only if every upstream behind the target formats rejects `developer` roles:

```yaml
model_match:
  mode: contains
  include: []
  exclude: []
```

## Behavior examples

### Matched request

Input body:

```json
{
  "model": "deepseek-chat",
  "messages": [
    {"role": "developer", "content": "You are concise."},
    {"role": "user", "content": "Hello"}
  ]
}
```

Output body:

```json
{
  "model": "deepseek-chat",
  "messages": [
    {"role": "system", "content": "You are concise."},
    {"role": "user", "content": "Hello"}
  ]
}
```

### Unmatched model

With default config, this request is returned unchanged because the model does not contain `deepseek`:

```json
{
  "model": "gpt-4.1",
  "messages": [
    {"role": "developer", "content": "You are concise."},
    {"role": "user", "content": "Hello"}
  ]
}
```

## Management UI metadata

The plugin registration exposes these `ConfigFields` for CPA management clients:

- `normalize_enabled` (`boolean`)
- `target_formats` (`array`)
- `model_match` (`object`)
- `strategy` (`enum`, currently `role_to_system`)

These fields describe plugin-owned configuration under:

```yaml
plugins:
  configs:
    developer-role-normalizer:
      # plugin-owned fields here
```

## Build

```bash
cd plugins/developer-role-normalizer/go
CGO_ENABLED=1 go build -buildmode=c-shared -o developer-role-normalizer.so .
```

The plugin must be compiled with `CGO_ENABLED=1` because it exports the CPA C ABI with cgo.

## Test

```bash
cd plugins/developer-role-normalizer/go
go test ./...
```

## Install via Plugin Store

Register this store in your CPA config:

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

Then install via the Management API:

```bash
curl -X POST http://localhost:8317/v0/management/plugin-store/developer-role-normalizer/install \
  -H "Authorization: Bearer <management-key>"
```

After installation, enable the plugin:

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
```

## Manual install

Build the dynamic library for your target platform and place it in the CPA plugin directory using the versioned artifact naming convention used by your deployment, for example:

```text
plugins/linux/amd64/developer-role-normalizer-v0.3.0.so
```

Then enable it in CPA config:

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
```

## Compatibility notes

- The default is intentionally narrow: only `openai`/`codex` target formats and models containing `deepseek` are normalized.
- Matching is case-insensitive.
- The plugin uses `RequestTransformRequest.Model` first and falls back to `body.model` when the request transform model field is empty.
- The plugin does not parse or merge message contents; it only replaces exact `developer` role values inside the top-level `messages` array.
- Requests without a valid top-level `messages` array are returned unchanged.

## License

MIT
