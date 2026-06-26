# CPA Manager Plus (CPA Plugin)

在 CPA 管理端提供 **一项侧栏菜单「CPA Manager Plus」**，页内 **5 个 Tab**（仪表盘 / 用量 / 监控 / 巡检 / 配置），通过插件 **`host.http.do`** 反向代理到 [CPA-Manager-Plus](https://github.com/seakee/CPA-Manager-Plus) Manager Server（默认 `http://127.0.0.1:18317`）。

## 架构

- 浏览器只访问 CPA 同源：`8317`
- 页面：`GET /v0/resource/plugins/cpa-manager-plus/app`（无需 management key）
- API：`POST /v0/management/cpa-manager-plus/proxy`（需 management key，与 CPA 管理 API 一致）
- 健康：`GET /v0/management/cpa-manager-plus/health`

代理请求体示例：

```json
{
  "method": "GET",
  "path": "/v0/management/dashboard/summary",
  "query": "today_start_ms=1710000000000"
}
```

`path` 为 Manager Server 上的路径（与 Plus `router.go` 一致），例如 `/usage-service/info`、`/v0/management/monitoring/analytics`。

## 配置（`plugins.configs.cpa-manager-plus`）

| 字段 | 说明 |
|------|------|
| `manager_base_url` | Manager Server 地址，默认 `http://127.0.0.1:18317` |
| `admin_key` | 可选；设置后插件用 Bearer 访问 Manager，不转发浏览器 Authorization |

## 构建

```bash
cd plugins/cpa-manager-plus
make build
```

将产物放入 CPA `plugins/` 目录，在配置中启用 `cpa-manager-plus`。

## 运行前提

1. CPA 已启用插件
2. Manager Server 已启动并监听 `manager_base_url`
3. 打开 CPA 管理 UI → 侧栏 **CPA Manager Plus**；Tab 内按钮会调用同源 proxy

## 与 CPA-Auth-Plugin 的关系

本仓库是 **独立 CPA 动态库插件**（Go c-shared）。若你使用 Desktop 上的 Auth 插件壳，可把同一代理思路迁到其 `http-server`；本实现按 CPA 官方 **Management API + Resource** 模型落地。