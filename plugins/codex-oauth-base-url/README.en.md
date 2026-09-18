# codex-oauth-base-url

[中文](README.md) | English

`codex-oauth-base-url` is a CPA auth provider plugin that rewrites the upstream base URL of **Codex OAuth (ChatGPT subscription)** credentials, so a Codex account can be pointed at a self-hosted or third-party upstream without patching CPA.

## Problem solved

The Codex executor resolves its upstream from `auth.Attributes["base_url"]` and falls back to a hardcoded default when the attribute is absent:

```go
// internal/runtime/executor/codex_executor_auth.go
func codexCreds(a *cliproxyauth.Auth) (apiKey, baseURL string) {
	if a.Attributes != nil {
		apiKey = a.Attributes["api_key"]
		baseURL = a.Attributes["base_url"]
	}
	...
}
```

`codex-api-key` config entries set that attribute, so a custom endpoint already works for API-key credentials. OAuth auth files do not: the built-in file synthesizer builds the `Attributes` map from a fixed field list and never writes `base_url`, so the hardcoded fallback always wins.

Adding `"base-url"` to an auth JSON file does not help either. `NormalizeCredentialMetadata` only renames it to `Metadata["base_url"]`, and the Codex executor never reads that key.

This plugin closes that gap from outside the CPA source tree.

## Architecture

```text
Codex auth file (auths/codex-*.json)
    │
    ▼
CPA watcher / file synthesizer
    │
    ├─ asks every registered auth provider whose identifier matches the
    │  file's "type" field, i.e. identifier == "codex"
    │
    ▼
codex-oauth-base-url: auth.parse
    ├─ read plugins.configs.codex-oauth-base-url.base-url
    ├─ reproduce the attributes the built-in synthesizer derives from the file
    ├─ add base_url
    │
    ▼
CPA Auth entry with Attributes["base_url"] set
    │
    ▼
Codex executor -> strings.TrimSuffix(baseURL, "/") + "/responses"
    │
    ▼
configured upstream

Fallback path (plugin returns Handled: false)
    │
    ▼
built-in synthesizer, unchanged behaviour
```

## Runtime flow

1. CPA loads the dynamic library and calls `plugin.register`.
2. The plugin declares the `auth_provider` capability and `auth.identifier` returns `codex`.
3. CPA offers every auth file whose `type` is `codex` to this plugin before falling back to its built-in synthesizer.
4. On config change CPA calls `plugin.reconfigure` with `plugins.configs.codex-oauth-base-url`.
5. For each Codex OAuth auth file the plugin returns an auth record whose attributes carry `base_url`.

## Which requests are affected

Every Codex upstream URL that flows through `codexCreds` is redirected, including `/responses`, `/responses/compact`, both streaming transports, the WebSocket transport, and the OpenAI image endpoints.

Two Codex URLs do **not** go through `codexCreds` and stay pointed at `chatgpt.com`:

- `/v1/alpha/search` — the OAuth branch of that handler uses the hardcoded URL and only the `codex-api-key` branch honours `base_url`. Alpha Search is an API-key feature.
- Codex Live realtime calls (`internal/client/codex/live/live.go`).
- Model listing (`/v1/models`) — served from the local model registry with no upstream request, so `base-url` does not apply.

The full picture, CPA entry point to upstream target:

| CPA entry | Upstream target | Affected by `base-url` |
|-----------|-----------------|------------------------|
| `POST /v1/responses` (incl. streaming, WebSocket) | `{base-url}/responses` | yes |
| `/responses/compact` | `{base-url}/responses/compact` | yes |
| `/v1/images/generations`, `/v1/images/edits` (direct) | `{base-url}/images/generations`, `{base-url}/images/edits` | yes |
| `/v1/models` | local registry | no (no upstream call) |
| `/v1/alpha/search` (OAuth) | `chatgpt.com/backend-api/codex/alpha/search` | no |
| Codex Live realtime calls | `chatgpt.com/backend-api/codex/realtime/calls` | no |
| Login authorization (`/codex-auth-url`) | `auth.openai.com/oauth/authorize` | no |
| Token refresh | `auth.openai.com/oauth/token` | no |

The WebSocket transport derives its scheme from `base-url`: `https` becomes `wss`, `http` becomes `ws`.

## Lossless by construction

The plugin reproduces the attributes the built-in synthesizer derives from the same file and adds `base_url` on top, so enabling or removing it changes nothing except the upstream URL.

| Attribute | Source |
|-----------|--------|
| `base_url` | this plugin |
| `plan_type` | `plan_type` metadata, else the `chatgpt_plan_type` `id_token` claim |
| `priority` | `priority` metadata |
| `note` | `note` metadata |
| `auth_kind`, `path`, `source`, `source_backend` | CPA, unchanged |
| `header:*`, `weight`, `model_aliases`, `excluded_models`, `proxy_url` | CPA, unchanged |

The plugin returns `Handled: false` — leaving the built-in synthesizer in charge — when any of these hold, so it can never interfere with credentials it does not own:

- the file is not `type: codex`;
- the file is an API key entry with no `access_token`, `refresh_token`, or `id_token`;
- no base URL is configured and the file carries no `base-url` of its own.

## Configuration

The plugin declares a `ConfigFields` entry at registration, so the CPA Management Center renders it as a form field. Open **Plugin Management**, pick the plugin, and click **Edit Config**:

```text
Config codex-oauth-base-url
  Base settings
    Enabled                  [ on ]
    Priority                 [ 1 ]
  Config fields
    base-url
    [ https://your-upstream.example.com ]
    Upstream base URL applied to Codex OAuth credentials.
    Empty keeps the built-in default.
                              [ Cancel ]  [ Save ]
```

Saving writes `plugins.configs.codex-oauth-base-url` to `config.yaml` with comments preserved and reloads the config, so the new upstream takes effect without restarting CPA. The next request uses it.

When writing the config through the Management API, use `PATCH /v0/management/plugins/codex-oauth-base-url/config`. `PUT` **replaces** the whole config node for this plugin, so a payload without `enabled` turns the plugin off.

### Minimal configuration

```yaml
plugins:
  enabled: true
  configs:
    codex-oauth-base-url:
      enabled: true
      priority: 1
      base-url: "https://your-upstream.example.com"
```

### Field reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | controlled by CPA | CPA plugin enable switch. |
| `priority` | integer | controlled by CPA | CPA plugin load and routing order. |
| `base-url` | string | empty | Upstream base URL for Codex OAuth credentials. The plugin appends `/responses` and `/responses/compact`. Empty disables the rewrite. |

A per-account `"base-url"` inside an auth JSON file takes precedence over the configured value, so individual credentials can point at different upstreams.

### Validation

A mistyped URL is the one mistake the form cannot catch, because the Management Center has no URL field type. The plugin checks the value when it is configured and logs a warning with the reason, so the mistake shows up in the CPA log view instead of only as a failed request:

```text
level=warn msg="codex-oauth-base-url: configured base-url looks wrong"
  base_url="codex-relay.example.com" reason="missing an http:// or https:// scheme"
```

### Verifying it works

The only reliable evidence that `base-url` is in use is the **actual upstream URL**. Turn on request logging:

```yaml
request-log: true
```

Logs are written to `logs/` under the CPA working directory, falling back to `<auth-dir>/logs/` when that is not writable. After one Codex request:

```text
=== API REQUEST 1 ===
Timestamp: 2026-09-18T15:28:17.051509+08:00
Upstream URL: https://your-upstream.example.com/responses
HTTP Method: POST
```

Your own host means it works. If it still reads `https://chatgpt.com/backend-api/codex/responses`, the plugin did not handle this request — check `plugins.configs.codex-oauth-base-url.enabled`, and that the auth file has `"type": "codex"` with an `access_token` (API-key-only entries are not handled).

You can also point `base-url` at a local listener for an end-to-end check:

```bash
python3 -c "
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_POST(self):
        print('upstream got:', self.path, dict(self.headers).get('Authorization'))
        self.send_response(200); self.end_headers(); self.wfile.write(b'{}')
    def log_message(self, *a): pass
HTTPServer(('127.0.0.1', 9999), H).serve_forever()
"
```

With `base-url` set to `http://127.0.0.1:9999` it should print `upstream got: /responses Bearer <access_token>`.

## Installation

Requires CPA `v7.2.46+`. Schema v1 is retired; only the v2 entry points below are available.

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

Install:

```bash
curl -X POST http://localhost:8317/v0/management/plugin-store/codex-oauth-base-url/install \
  -H "Authorization: Bearer ***"
```

Enable:

```yaml
plugins:
  enabled: true
  configs:
    codex-oauth-base-url:
      enabled: true
      priority: 1
      base-url: "https://your-upstream.example.com"
```

Restart CPA after installing or upgrading. Already loaded dynamic libraries are not hot-swapped.

## Manual install

Place the dynamic library in the CPA plugin directory, for example:

```text
plugins/darwin/arm64/codex-oauth-base-url-v0.1.0.dylib
```

## Compatibility notes

### Credential refresh and login do not use base-url

`base-url` **only affects inference requests**. OAuth login and token refresh go to their own fixed endpoints and are untouched by this plugin:

| Flow | Target | Affected by base-url |
|------|--------|----------------------|
| Inference (`/responses`, ...) | your configured `base-url` | yes |
| Login authorization (`/codex-auth-url`) | `https://auth.openai.com/oauth/authorize` | no |
| Token refresh | `https://auth.openai.com/oauth/token` | no |

Both addresses are hardcoded constants in CPA, and the refresh function takes only a `refresh_token` with no URL argument. The plugin `auth.refresh` hook is never called either: CPA wraps the plugin refresh adapter around `openai-compatibility` executors only, while Codex uses its native executor.

**Checking it yourself**: point `base-url` at a local listener and trigger a refresh (`POST /v0/management/auth-files/refresh`). The listener sees nothing, and the CPA log shows:

```text
Token refresh attempt 1 failed: token refresh request failed: Post "https://auth.openai.com/oauth/token": EOF
```

**Practical impact**: a `refresh_token` is only valid against OpenAI. If your third-party upstream only serves inference, the `access_token` cannot be renewed once it expires and that account stops working — regardless of whether the base URL is configured correctly.

Refresh needs `auth.openai.com` to be reachable. When it is blocked, refresh retries three times and then fails (CPA retries automatically every 15 minutes by default). Use `proxy-url` to route refresh through a proxy: the per-account `proxy-url` wins, falling back to the global `proxy-url`.

If your upstream issues API keys, use `codex-api-key` with `base-url` instead: that path needs no refresh and has no such problem.

### Authentication headers are unchanged

The plugin only rewrites the URL. Codex OAuth requests keep their ChatGPT-style authentication: `Authorization: Bearer <access_token>`, `chatgpt-account-id`, and the forced `Originator`/`User-Agent` headers applied by the Codex executor (disable with `codex.disable-codex-cloaking: true`).

An upstream that expects `Bearer sk-...` will reject those requests. If the target is a relay that issues API keys, use `codex-api-key` with `base-url` instead; this plugin is unnecessary there.

### TLS fingerprint

Requests go through CPA's uTLS HTTP client, whose TLS fingerprint and SNI follow the target host. Verify it against the upstream before rolling out, together with the WebSocket transport, which reuses the same base URL.

### Auth provider identifier sharing

Declaring the `codex` identifier also claims `auth.login.start`, `auth.login.poll`, and `auth.refresh`. CPA routes Codex OAuth login to its built-in routes and does not wrap the Codex executor in the plugin refresh adapter, so those hooks are unreachable today. The plugin answers them with an explicit `unsupported_method` error rather than empty credentials, so a future CPA change fails loudly instead of silently breaking credential refresh.

## Build and verification

```bash
cd plugins/codex-oauth-base-url/go
go test ./...
go vet ./...
CGO_ENABLED=1 go build -buildmode=c-shared -o codex-oauth-base-url.dylib .
nm -gU codex-oauth-base-url.dylib | grep cliproxy
```

The plugin must be built with `CGO_ENABLED=1` because it exports the CPA C ABI through cgo. The release build injects the version with `-ldflags "-X main.pluginVersion=<version>"`; the literal in `main.go` is only the development default.

## Releasing

The version belongs to this plugin alone: the tag form is `<plugin-id>-v<version>`, and CI builds only this plugin.

1. Synchronize the version in both places:
   - `go/main.go` -> `var pluginVersion = "X.Y.Z"`
   - repository `plugins.json` -> this plugin's `"version"`
2. Commit and push to `main`.
3. Create and push the tag:

```bash
git tag -a codex-oauth-base-url-vX.Y.Z -m "codex-oauth-base-url X.Y.Z"
git push origin codex-oauth-base-url-vX.Y.Z
```

4. The workflow builds only this plugin, publishes the six platform zips, and refreshes `registry-v2.json` on main plus the `cdn` branch.

Version 0.1.0 was released under `codex-oauth-base-url-v0.1.0`, the first use of a plugin-scoped tag.

## License

MIT
