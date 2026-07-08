# CPA Manager Plus（CPA 插件）

中文 | [English](README.en.md)

`cpa-manager-plus` 在 CPA 管理端提供一个侧栏菜单「CPA Manager Plus」。页面内用 Tabs 承载仪表盘、用量分析、请求监控、账号巡检、配置与健康检查，并通过插件服务端反向代理到 [CPA-Manager-Plus](https://github.com/seakee/CPA-Manager-Plus) Manager Server，默认地址为 `http://127.0.0.1:18317`。

## 当前架构决策

本插件不代理、不 iframe、不整页嵌入 Plus 的 `management.html`。

原因：Plus 是独立管理面产品，整页代理会把另一个控制台塞进 CPA 插件页，导致视觉、路由、登录态、布局与 CPA Management Center 割裂。

正确边界：

1. 前端归插件所有
   - 插件资源页只提供 CPA 风格的单页应用。
   - 当前迁移/重写需要的功能 Tab：概览、用量分析、请求监控、账号巡检、配置、健康。
   - 页面样式使用 CPA Management Center CSS 变量，例如 `--bg-secondary`、`--bg-primary`、`--text-primary`、`--border-color`、`--primary-color`。

2. 后端仍依赖 Manager Server
   - SQLite、collector、历史用量、Codex 服务端巡检等长生命周期能力仍由 Manager Server 提供。
   - 插件不内联这些后台模块。

3. 插件只反向代理必要 API
   - 浏览器只访问 CPA 同源地址：`8317`。
   - 插件服务端通过 `host.http.do` 请求 Manager Server：默认 `18317`。
   - 代理只允许本插件 Tab 需要的 Manager API 路径，不作为通用 HTTP tunnel。

## 架构

```text
CPA Management Center
    │
    ▼
/v0/resource/plugins/cpa-manager-plus/app
    │
    ▼
Vue single-page plugin UI
    │
    ▼
/v0/management/cpa-manager-plus/proxy
    │
    ▼
CPA plugin server handler
    │ host.http.do
    ▼
CPA-Manager-Plus Manager Server
    default: http://127.0.0.1:18317
```

## 前端构建架构

采用 `web/` + Vite + Vue SFC 的工程化构建，而不是在 `index.html` 里直接加载 runtime Vue CDN/global build：

```text
plugins/cpa-manager-plus/
├── web/
│   ├── package.json
│   ├── package-lock.json
│   ├── vite.config.js
│   └── src/
└── go/
    ├── embed.go
    ├── main.go
    └── web-dist/      # Vite 构建产物，git ignore，由 make/CI 生成
```

构建命令：

```bash
cd plugins/cpa-manager-plus
make build
```

`make build` 会执行：

```bash
cd web && npm ci && npm run build
cd go && go mod tidy
cd go && CGO_ENABLED=1 go build -buildmode=c-shared -o ../cpa-manager-plus-v<version>.dylib .
```

Go 通过 `embed.FS` 打包编译后的 `go/web-dist`。不要把未构建的前端源码或 runtime CDN 页面直接交给 Go embed。

## URL 结构

| 功能 | 路径 | 说明 |
|------|------|------|
| 插件页面 | `GET /v0/resource/plugins/cpa-manager-plus/app` | Resource route，无 CPA management middleware |
| 健康检查 | `GET /v0/management/cpa-manager-plus/health` | CPA 管理鉴权 |
| API 网关 | `POST /v0/management/cpa-manager-plus/proxy` | CPA 管理鉴权，代理到 Manager Server |

代理请求体：

```json
{
  "method": "GET",
  "path": "/v0/management/dashboard/summary",
  "query": "today_start_ms=1710000000000"
}
```

`path` 是 Manager Server 上的路径。插件会做路径和方法白名单校验。

## 当前允许代理的 Manager API

- `/health`
- `/usage-service/info`
- `/usage-service/config`
- `/usage-service/account-processing-policy`
- `/usage-service/quota-cooldowns`
- `/v0/management/dashboard/summary`
- `/v0/management/usage`
- `/v0/management/model-prices`
- `/v0/management/api-key-aliases`
- `/v0/management/monitoring/header-snapshots`
- `/v0/management/monitoring/analytics`
- `/v0/management/codex-inspection/run`
- `/v0/management/codex-inspection/runs`
- `/v0/management/codex-inspection/runs/{id}`
- `/v0/management/codex-inspection/runs/{id}/actions`

## UI 结构

| Tab | Vue loader | Manager Server endpoint |
|-----|------------|-------------------------|
| 仪表盘 | `loadDashboard` | `GET /v0/management/dashboard/summary` |
| 用量分析 | `loadUsage` | `GET /v0/management/usage` |
| 请求监控 | `loadMonitoring` | `GET /v0/management/monitoring/analytics` |
| 账号巡检 | `loadInspection` | `GET /v0/management/codex-inspection/runs` |
| 配置 | `loadConfig` | `GET /usage-service/config` |
| 健康 | `checkHealth` | `GET /v0/management/cpa-manager-plus/health` |

## 配置：`plugins.configs.cpa-manager-plus`

| 字段 | 说明 |
|------|------|
| `manager_base_url` | Manager Server 地址，默认 `http://127.0.0.1:18317` |
| `management_key` | 可选。Plus Manager 的 admin Bearer，仅插件服务端访问 `18317` 时使用。本机无鉴权可省略。旧字段 `admin_key` 仍可读，等价于 `management_key` |

CPA 管理密钥（`remote-management.secret-key`）不要写入插件配置。浏览器调用 `/v0/management/cpa-manager-plus/*` 时，需要 CPA 管理台登录态或本页临时输入；插件进程无法从 CPA 配置读取 secret-key。

`management_key` 与 CPA secret-key 是两个不同密钥：

- CPA secret-key：浏览器访问 CPA `/v0/management/*` 使用。
- Plus `management_key`：插件服务端访问 Manager Server `18317` 使用。

## 安装

### 推荐：CPA v7.2.46+ 使用 CDN v2 registry

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

### 老 CPA 兼容入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

安装：

```bash
curl -X POST http://localhost:8317/v0/management/plugin-store/cpa-manager-plus/install \
  -H "Authorization: Bearer ***"
```

启用并配置 Manager Server：

```yaml
plugins:
  enabled: true
  configs:
    cpa-manager-plus:
      enabled: true
      priority: 10
      manager_base_url: "http://127.0.0.1:18317"
      # management_key: "optional-plus-manager-admin-key"
```

安装或升级后重启 CPA 进程，已加载的动态库不会热替换。

## 运行前提

1. CPA 已启用插件。
2. Manager Server 已启动并监听 `manager_base_url`。
3. CPA 管理台已登录，或在插件页临时输入 CPA management key。
4. 如果 Manager Server 启用了 admin key，在插件配置中设置 `management_key`。

## 验证标准

```bash
cd plugins/cpa-manager-plus
make build
cd go
go test ./...
go vet ./...
nm ../cpa-manager-plus-v0.3.8.dylib | grep cliproxy_plugin_init
```

还应检查：

- `go/web-dist/index.html` 引用 `./assets/app.js` 与 `./assets/app.css`。
- 动态库 strings 中包含 `./assets/app.js`、`./assets/app.css` 和业务 API marker。
- 导出符号包含 `cliproxy_plugin_init`、`cliproxyPluginCall`、`cliproxyPluginFree`。
- zip 中只包含对应平台动态库。

## 非目标

- 不完整复刻 Plus 原 React 页全部复杂交互。
- 不内联 Manager Server 后台。
- 不提供通用 HTTP tunnel。

## License

MIT
