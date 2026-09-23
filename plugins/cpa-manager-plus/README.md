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

## 事件流：上游响应模型

事件模型弹框区分请求模型、实际并计费模型和上游响应模型。仅当上游响应模型非空且与计费模型不同（去除首尾空白后精确比较）时显示第三行；即使请求模型等于计费模型，此时也会出现弹框入口。旧事件或三者相同不会增加多余提示，已有请求映射仍按原规则展示。上游响应模型不参与价格查找、费用计算、模型筛选或统计分组。

### 双链路来源与合并

- **宿主上报（首选）**：官方 CPA 在 commit [`ac3849e5d981e85dd3f713aae0691d23d7b3a56c`](https://github.com/router-for-me/CLIProxyAPI/commit/ac3849e5d981e85dd3f713aae0691d23d7b3a56c) 为 `UsageRecord` 增加 `ResponseModel`，并由 usage adapter 传给插件。插件优先使用官方字段；旧宿主或字段为空时，兼容 `responseModel` / `response_model` 扩展，再回退到插件观察。
- **插件观察**：只读注册 `response_before_translator`、`response_interceptor`、`response_stream_interceptor`。先读取翻译前原始模型，再用保留的响应 ID 连接到下游钩子的原生请求 ID 响应头，最后与同一凭据的 usage 精确关联。不读取 Usage queue、不抓取请求日志、不改写响应。
- 宿主有值时优先采用；仅观察有可靠值时补充；两路相同标记“双路确认”；不同或关联有歧义时显示采集冲突。冲突本身不等同于模型降级，也不单独触发红色模型差异高亮。
- API 返回 `response_model`（选定值）、`host_response_model`、`observed_response_model`、`response_model_source`（空／`host`／`observer`／`confirmed`）和 `response_model_conflict`。宿主原值与观察证据独立保存；晚到观察只补充监控信息，不重复入账。

### 当前覆盖与保守边界

- 主动观察仅覆盖已核实的 **Antigravity/Gemini → OpenAI Chat、OpenAI Responses、Claude、Gemini** HTTP 非流式和完整 SSE 事件路径；排除宿主合成模型的 Imagen，暂不接入 WebSocket 观察。宿主显式上报链路不受此协议白名单限制。
- 必须同时具有真实响应 `responseId`、可核对的原始请求指纹、钩子中的 `selected_auth_id`，以及同一凭据在 usage 和响应钩子共同携带的原生单请求标识头（依次识别 `X-Request-ID`、`Request-ID`、`X-Goog-Request-ID`）。请求指纹只是附加范围，绝不单独用于匹配；响应体 ID 也不被假定等于响应头 ID。
- 上游未提供标识、ID 被转换后无法对应、缺少模型、多值/冲突标识或同一标识对应多条 usage（含分拆计费）时，不给用量事件填入不确定的观察模型。原生 ID 是否存在、是否保证单请求唯一仍取决于供应商；不承诺所有真实流量都能采到。
- 关联缓存有容量限制，每类缓存最多 4096 条；活跃流刷新关联，闲置关联窗口为 10 分钟，每个响应最多保留 16 个关联键。仅保留哈希、模型和必要状态，不持久化请求/响应正文或凭据原文。
- 只解析完整且受限的 JSON/SSE：单次正文不超过 1 MiB、最多 64 个事件、嵌套深度 32；不拼接跨回调的碎片 SSE。已关联范围发生解析缺口时撤销该范围的观察可信状态。
- ABI 信封超过 8 MiB、信封损坏、1024 条观察队列溢出或观察写入失败时，立即隐藏派生观察结果，后台持久化隔离，并停用观察直到插件运行时重启；宿主显式字段、业务响应与计费照常。健康接口 `response_observer` 提供 enabled、disabled_until_restart、callbacks、persisted、queue_depth、dropped、last_error。
- SQLite 自动迁移；观察和 usage 可任意先后到达。冲突隔离持久化，查询时动态判断，避免迟到冲突仍被显示为可信模型。无 usage 引用的观察保留最多 7 天／10000 条；有引用的证据和隔离状态不随缓存过期丢失。

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
- `/v0/management/auto-ban/settings|rules|accounts|...`

## 本地凭证健康巡检

从 **0.5.0** 起，账号巡检在插件本地 Runtime 中执行真实 provider 探测，而不是只汇总认证文件状态。

- 支持 **Codex**、**xAI** 与 **Codex + xAI** 组合；按 provider 独立抽样。
- 通过 CPA 管理 `api-call` 与 `auth_index` 代理请求；认证 token 不会写入插件 SQLite、巡检日志或前端响应。
- Codex 使用 usage 探测；xAI 使用 billing 探测，并可选启用 inference 健康探测。
- 支持定时、手动启动、运行中取消、结果日志、额度阈值与受控自动处置。
- 自动恢复只会启用由巡检自动禁用且已记录归属的凭证；认证失效、限流、协议变化或缺少认证元数据均保守地要求人工复核。

真实巡检需要在「账号处置授权」中配置 CPA 管理地址和密钥，且目标 CPA 必须支持 `/v0/management/api-call`。建议先以“不自动执行”模式确认 provider 响应与结果分类，再开启自动处置。

## Auto-Ban（按状态码账号处置）

Auto-Ban 默认关闭。启用后，插件会使用已落库的 usage 失败事件和巡检结果，按 provider、状态码/错误类别、连续或窗口累计命中次数匹配规则，并写入独立的账号状态与追加式操作历史。

- **Codex OAuth** 默认将 `429` 禁用并冷却后自动启用：优先采用 `X-Ratelimit-Reset` 或 `Retry-After`，缺失时使用可配置的默认 5 小时；`401` 默认禁用。自动删除必须由单独规则显式选择并设置每日上限。
- **xAI OAuth** 对额度耗尽可禁用；`429`、`401/402/403` 默认只观测，避免与 CPA conductor 内置冷却重复处置。规则可调整，但会显示宿主冷却提示。
- **自定义 provider** 会记录状态码、阈值和审计历史；只有能映射为 CPA auth-file 的凭证可以自动禁用、启用或删除。纯 API key/provider 配置会降级为人工复核，不会伪造“软封禁”。
- 「认证异常」Tab 提供规则编辑、账号状态、冷却倒计时、历史详情和手动解禁/禁用/删除/暂停/恢复入口。人工暂停优先于自动恢复。

所有 token 和完整认证头都不会写入 Auto-Ban 状态或历史。请先在测试规则或演练模式确认命中结果，再开启真实动作。

## UI 结构

| Tab | 主要 endpoint |
|-----|---------------|
| 仪表盘 | `GET /v0/management/dashboard/summary` |
| 请求监控 / 用量 | `POST /v0/management/monitoring/analytics` |
| 模型单价 | `GET/PUT /v0/management/model-prices`、`GET /v0/management/model-prices/source-lookup` |
| 认证异常 | `GET/POST/DELETE .../account-action-candidates...` |
| 账号巡检 | `GET/POST .../codex-inspection/...` |
| 额度窗口 | `GET/PUT /v0/management/window-keeper/...` |
| 配置 | `GET/PUT /usage-service/config` |
| 健康 | `GET /v0/management/cpa-manager-plus/health` |


## Codex 额度窗口保活（Window Keeper）

插件内置 Codex OAuth 额度窗口动态跟踪与恢复发信能力：

- 自动识别每个账号 ChatGPT 上游实际返回的主窗口、额外限额（按 fill_gaps 补齐缺失周期）与 Code Review 窗口。
- 仅当门控窗口曾耗尽并重新恢复可用时，通过 CPA 模型执行回调（锁定 forced_provider=codex 与精确 auth_id）发送一条短消息，避免额度在恢复后一直空转等待。
- 完整流式校验：必须消费完整 SSE 事件且收到 response.completed 且含非空文本才记为成功；发信后复测判定窗口是否锚定或属于固定周期，固定窗口不重复发信。
- 数据与调度直接集成在本地同一 SQLite（WAL 事务），支持断电与重启接续；401/403/400 自动安全暂停账号，支持在界面一键恢复。
- 提供独立「额度窗口」Tab，支持中/英/繁/俄四语切换、动态额度胶囊展示、全局策略抽屉与单账号覆盖。

## 价格同步来源

模型价格同步按以下优先级处理：

1. `models.dev:xai`
2. `models.dev:openrouter`
3. `models.dev:other`
4. `litellm`
5. `openrouter`

高优先级来源提供的字段不会被低优先级来源覆盖；低优先级来源只会补齐缺失字段。精确匹配自动写入，模糊匹配只进入候选确认。`models.dev` 为社区维护目录，具体 provider 价格不代表实际账单。

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

要求 CPA `v7.2.46+`。本仓库已停止支持 schema v1，只提供下面的 v2 入口。

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

## 发布

版本号只属于本插件，与其他插件互不影响：tag 形式是 `<plugin-id>-v<version>`，CI 只构建本插件。

1. 同步版本号（三处必须一致）：
   - `go/main.go` → `var pluginVersion = "X.Y.Z"`
   - 仓库根 `plugins.json` → 本插件的 `"version"`
   - `Makefile` → `VERSION := X.Y.Z`（如存在）
2. commit 并 push 到 `main`。
3. 打 tag 并推送：

```bash
git tag -a cpa-manager-plus-vX.Y.Z -m "cpa-manager-plus X.Y.Z"
git push origin cpa-manager-plus-vX.Y.Z
```

4. workflow 只构建本插件，发布 6 平台 zip，并刷新 `registry-v2.json`（main）与 `cdn` 分支。

历史版本（0.5.28 及更早）发布在全局 `v<version>` tag 下，安装入口保持不变；从下一个版本起使用插件级 tag。
## 非目标

- 不完整复刻旧 Plus React 页全部复杂交互。
- 不提供通用 HTTP tunnel。
- 不把插件做成独立网络服务。
- 不自动导入旧外部 Manager 历史库。

## License

MIT
