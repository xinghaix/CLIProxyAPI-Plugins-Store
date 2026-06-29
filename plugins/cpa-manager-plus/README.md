# CPA Manager Plus (CPA Plugin)

在 CPA 管理端提供 **一项侧栏菜单「CPA Manager Plus」**，页内以 **Tabs** 承载用量管理相关能力，并通过插件服务端反向代理到 [CPA-Manager-Plus](https://github.com/seakee/CPA-Manager-Plus) Manager Server（默认 `http://127.0.0.1:18317`）。

## 当前架构决策

本插件 **不代理、不 iframe、不整页嵌入 Plus 的 `management.html`**。

原因：Plus 是独立管理面产品，整页代理会把另一个控制台塞进 CPA 插件页，导致视觉、路由、登录态、布局与 CPA Management Center 割裂。

正确边界：

1. **前端归插件所有**
   - 插件资源页只提供 CPA 风格的单页应用。
   - 只迁移/重写当前需要的 Plus 功能 Tab：概览、用量分析、请求监控、账号巡检、配置。
   - 页面样式使用 CPA Management Center 的 CSS 变量，例如 `--bg-secondary`、`--bg-primary`、`--text-primary`、`--border-color`、`--primary-color`，避免独立暗色主题。

2. **后端仍依赖 Plus Manager Server**
   - SQLite、collector、历史用量、Codex 服务端巡检等长生命周期能力仍由 Manager Server 提供。
   - 插件不在当前阶段内联这些后台模块。

3. **插件只反向代理 API**
   - 浏览器只访问 CPA 同源地址：`8317`。
   - 插件服务端通过 `host.http.do` 请求 Manager Server：默认 `18317`。
   - 代理只允许本插件 Tab 需要的 Manager API 路径，不作为通用 HTTP tunnel。

## 前端构建架构

采用 **`web/` + Vite + Vue SFC** 的工程化构建，而不是在 `index.html` 里直接加载 `vue.global.prod.js`：

```text
plugins/cpa-manager-plus/
├── web/                       # Vue 源码工程，必须进入 git
│   ├── package.json
│   ├── package-lock.json
│   ├── vite.config.js
│   └── src/
│       ├── App.vue
│       ├── main.js
│       ├── styles.css
│       ├── components/
│       └── utils/
└── go/
    ├── embed.go
    ├── main.go
    └── web-dist/              # Vite 构建产物，git ignore，由 make/CI 生成
        ├── index.html
        └── assets/
            ├── app.js
            └── app.css
```

构建命令：

```bash
cd plugins/cpa-manager-plus
make build
```

`make build` 会先执行：

```bash
cd web && npm ci && npm run build
```

Vite 将产物写入：

```text
go/web-dist/index.html
go/web-dist/assets/app.js
go/web-dist/assets/app.css
```

随后 Go 通过 `embed.FS` 打包这些 **编译后的产物**。不要把 runtime Vue CDN/global build 页面直接交给 Go embed。

## Go 资源嵌入

- `embed.go` 使用 `//go:embed web-dist/* web-dist/assets/*`。
- Resource route：`GET /v0/resource/plugins/cpa-manager-plus/app` 返回 `web-dist/index.html`。
- 静态资产：
  - `/v0/resource/plugins/cpa-manager-plus/assets/app.js` → `web-dist/assets/app.js`
  - `/v0/resource/plugins/cpa-manager-plus/assets/app.css` → `web-dist/assets/app.css`
- 只允许一层 `assets/*.js|*.css`，拒绝 `..`、嵌套路径和非 JS/CSS 文件。

## URL 结构

- 页面：`GET /v0/resource/plugins/cpa-manager-plus/app`（资源路由，无 CPA management middleware）
- 健康：`GET /v0/management/cpa-manager-plus/health`（CPA 管理鉴权）
- API 网关：`POST /v0/management/cpa-manager-plus/proxy`（CPA 管理鉴权）

代理请求体：

```json
{
  "method": "GET",
  "path": "/v0/management/dashboard/summary",
  "query": "today_start_ms=1710000000000"
}
```

`path` 是 Manager Server 上的路径。插件会做路径/方法白名单校验后再转发。

## 允许代理的 Manager API

当前前端 Tabs 使用以下 Manager Server 接口：

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

单个 CPA 插件页面，内部 Tabs：

| Tab | Vue loader | Manager Server endpoint |
|---|---|---|
| 仪表盘 | `loadDashboard` | `GET /v0/management/dashboard/summary` |
| 用量分析 | `loadUsage` | `GET /v0/management/usage` |
| 请求监控 | `loadMonitoring` | `GET /v0/management/monitoring/analytics` |
| 账号巡检 | `loadInspection` | `GET /v0/management/codex-inspection/runs` |
| 配置 | `loadConfig` | `GET /usage-service/config` |
| 健康 | `checkHealth` | `GET /v0/management/cpa-manager-plus/health` |

当前仍是业务 v1 / 数据链路型界面，保留 cards/table/raw JSON 展示，后续可逐 Tab 深迁移 Plus 原功能。

## 前端设计原则

插件页面应像 CPA Management Center 的一部分，而不是 Plus 的外部网页：

- 单侧栏菜单，页内 Tabs。
- 使用 CPA theme tokens：`--bg-secondary`、`--bg-primary`、`--bg-tertiary`、`--text-primary`、`--text-secondary`、`--border-color`、`--primary-color`、`--glass-bg`。
- 不引入 Plus 整站 Header、Sidebar、Router、Login。
- 数据展示从 Manager API 转换为 CPA 风格卡片、表格、状态条。
- 缺少 CPA 管理密钥时，在页面内提示并允许本会话临时输入；不要要求用户把 CPA secret-key 写入 YAML。

## 配置（`plugins.configs.cpa-manager-plus`）

| 字段 | 说明 |
|------|------|
| `manager_base_url` | Manager Server 地址，默认 `http://127.0.0.1:18317` |
| `management_key` | **可选**。Plus Manager 的 admin Bearer，仅插件服务端 `host.http.do` 访问 18317 时使用。本机无鉴权可省略。旧字段 `admin_key` 仍可读，等价于 `management_key` |

**CPA 管理密钥**（`remote-management.secret-key`）**不要**写入插件配置。浏览器调用 `/v0/management/cpa-manager-plus/*` 时，需要由 CPA 管理台登录态或本页临时输入提供；插件进程无法从 CPA 配置读取 secret-key（SDK 无 host 回调）。

**Plus `management_key` 与 CPA secret-key 是两个不同密钥**：

- CPA secret-key：浏览器访问 CPA `/v0/management/*` 使用。
- Plus `management_key`：插件服务端访问 Manager Server `18317` 使用。

## CI / Release 构建

仓库 workflow 在 Go build 前检查插件目录是否存在 `web/package.json`：

- 有则先 `npm ci && npm run build`。
- 无则跳过前端构建。

这能保证 tag release 里的动态库嵌入的是最新编译前端，而不是旧的/缺失的 `go/web-dist`。

## 验证标准

必须通过：

```bash
cd plugins/cpa-manager-plus
make build
cd go
go test ./...
go vet ./...
```

并检查：

- `go/web-dist/index.html` 引用 `./assets/app.js` 与 `./assets/app.css`。
- 动态库 strings 中包含 `./assets/app.js`、`./assets/app.css`、业务 API marker。
- 导出符号包含 `cliproxy_plugin_init`、`cliproxyPluginCall`、`cliproxyPluginFree`。
- zip 中只包含对应平台动态库。

## 运行前提

1. CPA 已启用插件。
2. Manager Server 已启动并监听 `manager_base_url`。
3. CPA 管理台已登录，或在插件页临时输入 CPA management key。
4. 如 Manager Server 启用了 admin key，在插件配置中设置 `management_key`。

## 非本阶段完成项

- 不完整复刻 Plus 原 React 页所有复杂交互。
- 不内联 Manager Server 后台。
- 不做真实 CPA + Manager Server 浏览器联调，除非后续明确启动两个服务验证。

## 与 CPA-Auth-Plugin 的关系

本仓库是 **独立 CPA 动态库插件**（Go c-shared）。若使用 Desktop 上的 Auth 插件壳，可复用同一 API 代理与 Tab UI 思路；本实现按 CPA 官方 **Management API + Resource** 模型落地。
