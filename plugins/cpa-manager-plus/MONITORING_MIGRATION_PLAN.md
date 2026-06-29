# 请求监控迁移计划

目标：把 CPA-Manager-Plus 的「请求监控」从当前 JSON PoC 迁移为插件内可用的 Vue 功能页，保留 CPA 插件约束：单文件内联前端、所有 Plus API 通过 `POST /v0/management/cpa-manager-plus/proxy` 代理。

## 范围

P0（本次实现）
- 时间范围：今天 / 7 天 / 14 天 / 30 天 / 全部 / 自定义。
- 筛选：账号、Provider、模型、API Key、项目、请求类型、状态、最低延迟、缓存状态、Trace ID、全文搜索。
- 数据源：`POST /v0/management/monitoring/analytics`，include summary/timeline/hourly/model/channel/account/api-key/failure/task/recent/events/filter_options。
- 摘要卡片：请求数、成功率、失败数、Token、费用、平均延迟、RPM/TPM、近似任务成功率。
- 数据页签：实时事件、账号、API Key、模型、渠道、失败来源、任务桶、最近失败、时间线。
- 事件详情抽屉/面板：展示响应 metadata、错误、quota、trace、token、延迟、路径。
- 自动刷新：关闭/5s/15s/30s/60s。
- CSV 导出：导出当前事件列表。

P1（后续）
- 和原 Plus 完全一致的图表视觉、异常点/热力图、账号卡片复杂展开、Header snapshot 关联、导入/导出用量数据。
- 与 AccountActionCandidates / Codex Inspection 的跨页面跳转。

## API

主请求：
```json
POST /v0/management/monitoring/analytics
{
  "from_ms": 1710000000000,
  "to_ms": 1710003600000,
  "time_zone": "Asia/Shanghai",
  "search_query": "...",
  "filters": {
    "models": ["..."],
    "providers": ["..."],
    "api_key_hashes": ["..."],
    "project_ids": ["..."],
    "request_types": ["..."],
    "failed_only": true,
    "include_failed": false,
    "min_latency_ms": 3000,
    "cache_status": "hit",
    "header_trace_ids": ["..."]
  },
  "include": {
    "summary": true,
    "summary_comparison": true,
    "timeline": true,
    "hourly_distribution": true,
    "model_share": true,
    "channel_share": true,
    "model_stats": true,
    "failure_sources": true,
    "account_stats": true,
    "api_key_stats": true,
    "filter_options": true,
    "task_buckets": true,
    "recent_failures": 20,
    "events_page": {"limit": 200},
    "granularity": "hour"
  }
}
```

## 验收

- 构建产物仍为单文件 `go/web-dist/index.html`，无 `/assets/*` 请求。
- `make build`、`go test ./...`、`go vet ./...` 通过。
- 请求监控页不再展示原始 JSON 为主界面，具备可操作筛选、数据页签、详情面板、导出。
- 后端白名单允许 `POST /v0/management/monitoring/analytics`。
