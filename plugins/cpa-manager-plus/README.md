# CPA Manager Plus（CPA 插件）

中文 | [English](README.en.md)

`cpa-manager-plus` 在 CPA 管理端提供一个侧栏菜单「CPA Manager Plus」。页面内用 Tabs 承载仪表盘、用量分析、请求监控、账号巡检、配置与健康检查。

从 **0.4.0** 起，插件在 CPA 同进程内运行 **本地 Runtime**（SQLite + worker），**不再依赖外部 Manager Server / `:18317` 反向代理**。

## 当前架构决策

本插件不代理、不 iframe、不整页嵌入旧 Plus 的 `management.html`，也不再把外部 Manager 当作运行前提。

正确边界：

1. 前端归插件所有
   - 插件资源页只提供 CPA 风格的单页应用。
   - 功能 Tab：概览、用量分析、请求监控、认证异常、账号巡检、配置、健康。
   - 页面样式使用 CPA Management Center CSS 变量。

2. 后端能力内嵌在插件本地 Runtime
   - SQLite、用量接入、rollup、Codex 巡检、认证异常等由插件同进程实现。
   - 默认数据目录为开发友好的 CWD 相对路径；生产请显式配置 `data_dir`。

3. 浏览器只访问 CPA 同源 management API
   - 浏览器 → `POST /v0/management/cpa-manager-plus/api`（payload 形状与旧 proxy 相同）
   - 浏览器 → `GET /v0/management/cpa-manager-plus/health`
   - 插件在本地 router 分发到服务层；不再提供通用 HTTP tunnel。

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
/v0/management/cpa-manager-plus/api|health
    │  management.handle
    ▼
Plugin Local Runtime (in-process)
    ├── API facade（保持旧 path 契约）
    ├── Services / Workers
    └── SQLite（本地 data_dir）
```

## 前端构建架构

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
    ├── internal/      # 本地 Runtime
    └── web-dist/      # Vite 构建产物，git ignore，由 make/CI 生成
```

构建：

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

## URL 结构

| 功能 | 路径 | 说明 |
|------|------|------|
| 插件页面 | `GET /v0/resource/plugins/cpa-manager-plus/app` | Resource route，无 CPA management middleware |
| 健康检查 | `GET /v0/management/cpa-manager-plus/health` | CPA 管理鉴权；返回本地 Runtime 状态 |
| API 网关 | `POST /v0/management/cpa-manager-plus/api` | CPA 管理鉴权；本地分发（payload 兼容旧 proxy） |

API 请求体（内部仍由前端 `proxyCall` 发送）：

```json
{
  "method": "GET",
  "path": "/v0/management/dashboard/summary",
  "query": "today_start_ms=1710000000000"
}
```

`path` 是本地 Runtime 兼容的业务路径；插件会做路径和方法白名单校验。

## 主要本地 API（白名单）

- `/health`、`/status`
- `/usage-service/config`
- `/v0/management/dashboard/summary`
- `/v0/management/model-prices`
- `/v0/management/monitoring/analytics`
- `/v0/management/codex-inspection/run|runs|...`
- `/v0/management/account-action-candidates` 及 `.../enable|ignore|resolve|auth-file`

## UI 结构

| Tab | 主要 endpoint |
|-----|---------------|
| 仪表盘 | `GET /v0/management/dashboard/summary` |
| 请求监控 / 用量 | `POST /v0/management/monitoring/analytics` |
| 模型单价 | `GET/PUT /v0/management/model-prices` |
| 认证异常 | `GET/POST/DELETE .../account-action-candidates...` |
| 账号巡检 | `GET/POST .../codex-inspection/...` |
| 配置 | `GET/PUT /usage-service/config` |
| 健康 | `GET /v0/management/cpa-manager-plus/health` |

## 配置：`plugins.configs.cpa-manager-plus`

| 字段 | 说明 |
|------|------|
| `enabled` | 是否启用插件 |
| `data_dir` | 本地 SQLite / data.key 目录；生产强烈建议绝对路径 |
| `db_filename` | 可选，默认 `manager.sqlite` |
| `ingest_mode` | 可选：`usage_plugin`（默认）/ `queue` / `dual` |
| `egress_proxy_url` | 可选，仅用于外部价格同步等出站请求 |

已废弃（读取时忽略）：`manager_base_url`、`management_key`（旧 Plus admin）、面向外部 Manager 的 `proxy_url`。

浏览器访问 `/v0/management/cpa-manager-plus/*` 仍需要 CPA `remote-management.secret-key`（管理台登录或配置 Tab 临时输入）。插件内 SQLite 还可保存「插件访问 CPA management/auth」的连接密钥，与浏览器密钥用途不同。

示例：

```yaml
plugins:
  enabled: true
  configs:
    cpa-manager-plus:
      enabled: true
      priority: 10
      data_dir: "/var/lib/cliproxyapi/plugins/cpa-manager-plus"
      # ingest_mode: "usage_plugin"
```

## 安装

### 推荐：CPA v7.2.46+ 使用 CDN v2 registry

> **CPA Manager Plus 运行要求 CPA v7.2.103+。** 此处的 v7.2.46+ 仅表示支持通过 registry-v2 安装；较早 CPA 无法加载本插件。

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

安装或升级后重启 CPA 进程，已加载的动态库不会热替换。

## 运行前提

1. CPA 已启用插件。
2. **不需要**外部 Manager Server / `:18317`。
3. CPA 管理台已登录，或在插件页临时输入 CPA management key。
4. 生产环境建议显式配置 `data_dir`。
5. 本地模式独占本机 CPA 用量流；勿与旧外部 Manager 同时消费同一 `usage-queue`。

## 验证标准

```bash
cd plugins/cpa-manager-plus
make build
cd web && npm test
cd ../go
go test ./...
go vet ./...
nm ../cpa-manager-plus-v0.4.0.dylib | grep cliproxy_plugin_init
```

还应检查：

- `go/web-dist/index.html` 引用 `./assets/app.js` 与 `./assets/app.css`。
- 导出符号包含 `cliproxy_plugin_init`、`cliproxyPluginCall`、`cliproxyPluginFree`。
- 字符串扫描无 `127.0.0.1:18317` 硬依赖。
- zip 中只包含对应平台动态库。

## 非目标

- 不完整复刻旧 Plus React 页全部复杂交互。
- 不提供通用 HTTP tunnel。
- 不把插件做成独立网络服务。
- 不自动导入旧外部 Manager 历史库。

## License

MIT
