---
title: Codex OAuth 额度窗口与保活发信的第一手依据
status: 调查笔记，不是实现
date: 2026-09-23
---

# Codex OAuth 额度窗口与保活发信的第一手依据

本文只记录编写 docs/design/codex-window-keeper.md 时核对过的事实。官方页面没有证明“新窗口要等第一次成功请求才开始计时”。

## 已证实

1. ChatGPT / Codex 文档把本地消息估算写成 per five-hour period，并写明 Weekly limits may also apply。当前余量和 reset 时间要看 usage dashboard，或活动 CLI 会话里的 /status。文档没有写明 5 小时窗口是固定时钟，还是从下一次成功请求起算。页面里 first Codex message 只出现在推荐奖励的 banked rate-limit reset，不能当成窗口起算规则。来源：https://learn.chatgpt.com/docs/pricing.md （2026-09-23 抓取）。

2. 官方把无人值守自动化的默认认证写成 API key。ChatGPT 登录只在确实要以某个 Codex 账号运行，并且 runner 可信、私有时使用。示例命令是 codex exec --json，提示为 Reply with the single word OK。来源：https://learn.chatgpt.com/docs/auth.md 与 https://learn.chatgpt.com/docs/auth/ci-cd-auth.md 。这不能推出一次保活消息违法，也不能推出它一定被订阅条款允许。help.openai.com 的条款页本次返回 403，没有读到原文。

3. CPA v7.3.9 的插件回调可以把模型请求锁到一张凭证上，锁不住就失败，不会改派到别的账号：
   - HostModelExecutionRequest 的 ForcedProvider 与 AuthID：模块缓存 sdk/pluginapi/types.go 约 610–632 行。
   - ExecuteModel / ExecuteModelStream 在 AuthID 非空时调用 WithPinnedAuthID：sdk/api/handlers/model_execution.go 约 104–143 行。
   - ForcedProvider 直接成为唯一 provider：sdk/api/handlers/handlers_routing.go 约 128–144 行。
   - pinned auth 不在该 provider 或当前不可调度时返回 auth_not_found，不会挑下一张凭证：sdk/cliproxy/auth/scheduler.go 约 380–394 行。
   - 回调测试确认这两个字段会传到执行器：internal/pluginhost/host_callbacks_test.go 的 TestHostModelExecutePropagatesForcedProviderAndAuthID。

4. Codex 执行器即使调用方要非流式结果，也会把上游 body 改成 stream=true，打到 {baseURL}/responses。只有读到 response.completed 才成功；流在完成前断开是 incomplete，空的 incomplete 也是错误。来源：模块缓存 internal/runtime/executor/codex_executor_execute.go 约 59–210 行，以及 codex_executor_terminal.go 的 incomplete 文案。因此保活消息必须走流式回调并读到结束，不能只看 HTTP 200。

5. 管理端 POST /v0/management/api-call 可以用 auth_index 加请求头里的 $TOKEN$ 代表指定凭证访问绝对 URL。宿主会替换 token，失败时返回 400，不会静默改用别的账号。它是通用 HTTP 代理，不经过 Codex 执行器的请求整形。来源：internal/api/handlers/management/api_tools.go 约 49–90 行。本仓库巡检已经用它读取 https://chatgpt.com/backend-api/wham/usage ：plugins/cpa-manager-plus/go/internal/app/inspection_engine.go 第 21 行，以及 inspection_quota.go 第 1043 行。

6. 本仓库已有窗口分类，可直接复用规则，但不能 import cpa-manager-plus：
   - limit_window_seconds 等于 18000 视为 5 小时。
   - 604800 视为周限额。
   - 28 到 31 天的秒数视为月限额。
   - 来源：inspection_quota.go 第 485 行，测试在 inspection_quota_test.go 第 10 行。
   - /wham/rate-limit-reset-credits 读的是可主动消耗的 reset credit，不是普通 5 小时或周窗口的自然重置。保活插件不得调用会消耗这类 credit 的接口。

7. 插件运行时约定已经存在：C ABI cliproxyPluginCall、schema version 1、相对和绝对两条管理路由、SQLite WAL、设置变更用有缓冲的 wake channel 打断调度循环。参照 cpa-manager-plus/go/main.go 第 164 行与 inspection_settings.go 第 344 行。

## 未证实，必须用实验闸门

- 成功请求会把下一次 reset_at 改成大约是现在加上窗口长度。这是用户的目标假设，不是上面任一来源中的规则。
- GET /wham/usage 本身不消耗生成额度。现有巡检反复调用它，但没有官方句子保证它不计费。实现上仍只在额度探测阶段调用它，不把它当成保活消息。
- 一次请求同时拨动 5 小时和周窗口，还是只拨动当前耗尽的那一个。实验要分别记录。
- 429 的失败请求会不会提前打开窗口。策略按不会处理：429 永不记成功。
- 宿主 host.auth.get 会返回整份凭证 JSON，其中可能有 access token。它可以用来读 chatgpt_account_id，但不能把 token 写入日志、数据库或管理响应。额度查询仍走 api-call 的 $TOKEN$，让宿主负责刷新。

## 实验记录格式

每个账号、每个窗口在第一次保活后保存：

- 发信前 reset_at、used_percent、limit_reached、limit_window_seconds。
- 发信完成时刻。
- 发信后同一组字段。
- 结论：usage_anchored、fixed_schedule 或 inconclusive。

只有 usage_anchored 才继续自动发信。fixed_schedule 停止发信，并在界面说明窗口不由首条消息起算，继续发只会消耗额度。

## 订阅和窗口字段

官方 Codex 客户端的计划枚举包括 free、go、plus、pro、prolite、team、business、enterprise 变体、edu 变体和 unknown。来源：https://cdn.jsdelivr.net/gh/openai/codex@main/codex-rs/protocol/src/account.rs ，2026-09-23 抓取。它没有规定每个计划有哪些限额窗口。

同一客户端的 RateLimitWindow 只有 used_percent、window_minutes 和 resets_at。HTTP 状态里的 RateLimitWindowSnapshot 在 client.rs 测试中只出现 used_percent、limit_window_seconds、reset_after_seconds、reset_at。map_rate_limit_window 用 limit_window_seconds 换算分钟，用 reset_at 作为结束时间。一个 rate_limit 对象只有 primary_window 和 secondary_window 两个槽；第三种周期如果存在，只能出现在另一个 rate_limit 或 additional_rate_limits 里。来源：https://cdn.jsdelivr.net/gh/openai/codex@main/codex-rs/protocol/src/protocol.rs 的 RateLimitWindow，以及 https://cdn.jsdelivr.net/gh/openai/codex@main/codex-rs/backend-client/src/client.rs 的 map_rate_limit_window 和 usage_payload_maps_primary_and_additional_rate_limits。

因此方案把结束时间当作接口直接给出的值，把开始时间默认计算为结束时间减去窗口长度。操作者给出的常见组合只作对照：Free 常见月窗口，Plus 与 Team 常见 5 小时加周加月，Pro 及更高常见周加月。对照和响应不一致时记录差异，不以对照补造或删除窗口。
