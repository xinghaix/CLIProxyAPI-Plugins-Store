# codex-oauth-base-url

中文 | [English](README.en.md)

`codex-oauth-base-url` 是一个 CPA 认证提供者插件，用于改写 **Codex OAuth（ChatGPT 订阅账号）** 凭据的上游 base URL，让 Codex 账号可以指向自建或第三方上游，而无需修改 CPA 源码。

## 解决的问题

Codex 执行器从 `auth.Attributes["base_url"]` 解析上游地址，该属性缺失时回落到硬编码默认值：

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

`codex-api-key` 配置项会写入该属性，所以 API Key 凭据本来就能自定义端点。但 OAuth 认证文件不行：内置的文件合成器只按固定字段列表构造 `Attributes`，从不写入 `base_url`，于是硬编码的回落值总是生效。

在认证 JSON 里手写 `"base-url"` 也没有用：`NormalizeCredentialMetadata` 只是把它改名为 `Metadata["base_url"]`，而 Codex 执行器根本不读这个键。

本插件在 CPA 源码树之外补上这个缺口。

## 架构

```text
Codex 认证文件 (auths/codex-*.json)
    │
    ▼
CPA watcher / 文件合成器
    │
    ├─ 询问所有 identifier 与文件 type 字段匹配的认证提供者插件
    │  即 identifier == "codex"
    │
    ▼
codex-oauth-base-url: auth.parse
    ├─ 读取 plugins.configs.codex-oauth-base-url.base-url
    ├─ 复现内置合成器从该文件推导出的全部属性
    ├─ 追加 base_url
    │
    ▼
带 Attributes["base_url"] 的 CPA Auth 条目
    │
    ▼
Codex 执行器 -> strings.TrimSuffix(baseURL, "/") + "/responses"
    │
    ▼
配置的上游

回落路径（插件返回 Handled: false）
    │
    ▼
内置合成器，行为完全不变
```

## 运行流程

1. CPA 加载动态库并调用 `plugin.register`。
2. 插件声明 `auth_provider` 能力，`auth.identifier` 返回 `codex`。
3. CPA 把每个 `type` 为 `codex` 的认证文件先交给本插件，未处理时才回落到内置合成器。
4. 配置变化时 CPA 用 `plugins.configs.codex-oauth-base-url` 调用 `plugin.reconfigure`。
5. 对每个 Codex OAuth 认证文件，插件返回带 `base_url` 属性的认证记录。

## 影响的请求范围

所有经过 `codexCreds` 的 Codex 上游地址都会被改写，包括 `/responses`、`/responses/compact`、两种流式传输、WebSocket 传输，以及 OpenAI 图像端点。

另有两个 Codex 地址**不**经过 `codexCreds`，仍然指向 `chatgpt.com`：

- `/v1/alpha/search` —— 该处理器的 OAuth 分支使用硬编码地址，只有 `codex-api-key` 分支才会尊重 `base_url`。Alpha Search 属于 API Key 功能。
- Codex Live 实时通话（`internal/client/codex/live/live.go`）。

模型列表（`/v1/models`）由本地模型注册表提供，不向上游发起请求，因此与 `base-url` 无关。

对应的两级上游地址：

| CPA 入口 | 上游目标 | 受 `base-url` 影响 |
|----------|----------|--------------------|
| `POST /v1/responses`（含流式、WebSocket） | `{base-url}/responses` | 是 |
| `/responses/compact` | `{base-url}/responses/compact` | 是 |
| `/v1/images/generations`、`/v1/images/edits`（直连） | `{base-url}/images/generations`、`{base-url}/images/edits` | 是 |
| `/v1/models` | 本地注册表 | 否（不出网） |
| `/v1/alpha/search`（OAuth） | `chatgpt.com/backend-api/codex/alpha/search` | 否 |
| Codex Live 实时通话 | `chatgpt.com/backend-api/codex/realtime/calls` | 否 |
| 登录授权（`/codex-auth-url`） | `auth.openai.com/oauth/authorize` | 否 |
| Token 刷新 | `auth.openai.com/oauth/token` | 否 |

WebSocket 传输会按 `base-url` 的 scheme 推导：`https` → `wss`，`http` → `ws`。

## 无损改写

插件复现内置合成器从同一文件推导出的属性，再在其上追加 `base_url`，因此启用或移除插件除了上游地址之外不改变任何东西。

| 属性 | 来源 |
|------|------|
| `base_url` | 本插件 |
| `plan_type` | `plan_type` 元数据，否则取 `id_token` 中的 `chatgpt_plan_type` 声明 |
| `priority` | `priority` 元数据 |
| `note` | `note` 元数据 |
| `auth_kind`、`path`、`source`、`source_backend` | CPA，保持不变 |
| `header:*`、`weight`、`model_aliases`、`excluded_models`、`proxy_url` | CPA，保持不变 |

出现以下任一情况时插件返回 `Handled: false`，交由内置合成器处理，因此它绝不会干扰不属于自己的凭据：

- 文件不是 `type: codex`；
- 文件是没有 `access_token`、`refresh_token`、`id_token` 的 API Key 条目；
- 既没有配置 base URL，文件自身也没有 `base-url`。

## 配置

插件在注册时声明了 `ConfigFields`，因此 CPA 管理面板会把它渲染成表单字段。打开**插件管理**，选中该插件，点击**编辑配置**：

```text
配置 codex-oauth-base-url
  基础设置
    启用                     [ 开 ]
    优先级                   [ 1 ]
  配置字段
    base-url
    [ https://your-upstream.example.com ]
    Upstream base URL applied to Codex OAuth credentials.
    Empty keeps the built-in default.
                              [ 取消 ]  [ 保存 ]
```

保存会把 `plugins.configs.codex-oauth-base-url` 写入 `config.yaml`（保留注释）并触发配置热重载，新上游无需重启 CPA 即可生效。下一个请求就会使用它。

通过管理 API 写配置时请用 `PATCH /v0/management/plugins/codex-oauth-base-url/config`。`PUT` 会**整体替换**该插件的配置节点，载荷里没有 `enabled` 时插件会被一并关掉。

### 最小配置

```yaml
plugins:
  enabled: true
  configs:
    codex-oauth-base-url:
      enabled: true
      priority: 1
      base-url: "https://your-upstream.example.com"
```

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | boolean | 由 CPA 控制 | CPA 插件启用开关。 |
| `priority` | integer | 由 CPA 控制 | CPA 插件加载与路由顺序。 |
| `base-url` | string | 空 | Codex OAuth 凭据的上游 base URL。插件会追加 `/responses` 与 `/responses/compact`。留空则关闭改写。 |

认证 JSON 文件内的单账号 `"base-url"` 优先于全局配置值，因此不同凭据可以指向不同上游。

### 校验

URL 写错是表单唯一拦不住的错误，因为管理面板没有 URL 字段类型。插件在配置时检查该值，并把原因写进日志，让错误出现在 CPA 日志页而不是仅仅表现为一次失败的请求：

```text
level=warn msg="codex-oauth-base-url: configured base-url looks wrong"
  base_url="codex-relay.example.com" reason="missing an http:// or https:// scheme"
```

### 验证生效

`base-url` 是否真的被用上，唯一可靠的证据是**实际上游 URL**。打开请求日志：

```yaml
request-log: true
```

日志写在 CPA 工作目录下的 `logs/`；该目录不可写时回落到 `<auth-dir>/logs/`。发起一次 Codex 请求后：

```text
=== API REQUEST 1 ===
Timestamp: 2026-09-18T15:28:17.051509+08:00
Upstream URL: https://your-upstream.example.com/responses
HTTP Method: POST
```

`Upstream URL` 是你的域名即为生效；若仍是 `https://chatgpt.com/backend-api/codex/responses`，说明插件没有接管这次请求 —— 检查 `plugins.configs.codex-oauth-base-url.enabled`，以及认证文件的 `"type"` 是否为 `codex` 且带 `access_token`（纯 API Key 条目不接管）。

也可以把 `base-url` 临时指向本机监听来做端到端确认：

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

`base-url` 设为 `http://127.0.0.1:9999` 后发请求，应打印 `upstream got: /responses Bearer <access_token>`。

## 安装

要求 CPA `v7.2.46+`。本仓库已停止支持 schema v1，只提供下面的 v2 入口。

### 推荐：CPA v7.2.46+ 配合 CDN v2 注册表

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json"
```

### GitHub raw 备用入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

安装：

```bash
curl -X POST http://localhost:8317/v0/management/plugin-store/codex-oauth-base-url/install \
  -H "Authorization: Bearer ***"
```

启用：

```yaml
plugins:
  enabled: true
  configs:
    codex-oauth-base-url:
      enabled: true
      priority: 1
      base-url: "https://your-upstream.example.com"
```

安装或升级后请重启 CPA，已加载的动态库不会被热替换。

## 手动安装

把动态库放入 CPA 插件目录，例如：

```text
plugins/darwin/arm64/codex-oauth-base-url-v0.1.0.dylib
```

## 兼容性说明

### 凭据刷新与登录不走 base-url

`base-url` **只影响推理请求**。OAuth 登录与 token 刷新发送到各自固定地址，不受本插件影响：

| 流程 | 目标地址 | 受 base-url 影响 |
|------|----------|------------------|
| 推理请求（`/responses` 等） | 你配置的 `base-url` | 是 |
| 登录授权（`/codex-auth-url`） | `https://auth.openai.com/oauth/authorize` | 否 |
| Token 刷新 | `https://auth.openai.com/oauth/token` | 否 |

这两处地址在 CPA 里是硬编码常量，刷新函数只接收 `refresh_token`，没有 URL 参数。插件的 `auth.refresh` 钩子也不会被调用：CPA 只在 `openai-compatibility` 执行器上包装插件刷新适配器，Codex 走原生执行器。

**如何自查**：把 `base-url` 指向本机监听，触发一次刷新（`POST /v0/management/auth-files/refresh`）—— 监听端收不到请求，而 CPA 日志会出现：

```text
Token refresh attempt 1 failed: token refresh request failed: Post "https://auth.openai.com/oauth/token": EOF
```

**实际影响**：`refresh_token` 只对 OpenAI 有效。若你的第三方上游仅提供推理能力，`access_token` 过期后将无法自动续期，该账号会失效 —— 这与 base URL 配置正确与否无关。

刷新需要 `auth.openai.com` 可达。该地址被阻断时刷新会重试 3 次后失败（默认每 15 分钟自动重试一次）。可用 `proxy-url` 让刷新走代理：按账号 `proxy-url` 优先，未设置时回落到全局 `proxy-url`。

若目标上游发放 API Key，请改用 `codex-api-key` 配合 `base-url`：那种方式不需要刷新，也就没有这个问题。

### 认证头保持不变

插件只改写 URL。Codex OAuth 请求仍携带 ChatGPT 风格认证：`Authorization: Bearer <access_token>`、`chatgpt-account-id`，以及 Codex 执行器强制添加的 `Originator`/`User-Agent`（可用 `codex.disable-codex-cloaking: true` 关闭）。

如果上游期望 `Bearer sk-...`，这些请求会被拒绝。若目标是发放 API Key 的中转站，请直接使用 `codex-api-key` 配合 `base-url`，那种场景不需要本插件。

### TLS 指纹

请求经过 CPA 的 uTLS HTTP 客户端，其 TLS 指纹与 SNI 跟随目标主机。上线前请连同复用同一 base URL 的 WebSocket 传输一起验证。

### 共用认证提供者标识

声明 `codex` 标识同时也占用了 `auth.login.start`、`auth.login.poll` 与 `auth.refresh`。CPA 把 Codex OAuth 登录走内置路由，也不会把 Codex 执行器包装进插件刷新适配器，因此这些钩子目前不可达。插件对它们返回明确的 `unsupported_method` 错误而不是空凭据，这样未来 CPA 若改变行为会显式失败，而不是静默破坏凭据刷新。

## 构建与验证

```bash
cd plugins/codex-oauth-base-url/go
go test ./...
go vet ./...
CGO_ENABLED=1 go build -buildmode=c-shared -o codex-oauth-base-url.dylib .
nm -gU codex-oauth-base-url.dylib | grep cliproxy
```

插件通过 cgo 导出 CPA C ABI，必须使用 `CGO_ENABLED=1` 构建。发布构建通过 `-ldflags "-X main.pluginVersion=<version>"` 注入版本号，`main.go` 中的字面量只是开发默认值。

## 发布

版本号只属于本插件，与其他插件互不影响：tag 形式是 `<plugin-id>-v<version>`，CI 只构建本插件。

1. 同步版本号（两处必须一致）：
   - `go/main.go` → `var pluginVersion = "X.Y.Z"`
   - 仓库根 `plugins.json` → 本插件的 `"version"`
2. commit 并 push 到 `main`。
3. 打 tag 并推送：

```bash
git tag -a codex-oauth-base-url-vX.Y.Z -m "codex-oauth-base-url X.Y.Z"
git push origin codex-oauth-base-url-vX.Y.Z
```

4. workflow 只构建本插件，发布 6 平台 zip，并刷新 `registry-v2.json`（main）与 `cdn` 分支。

当前版本 0.1.0 发布在 `codex-oauth-base-url-v0.1.0` 下，是插件级 tag 的第一个使用示例。

## 许可证

MIT
