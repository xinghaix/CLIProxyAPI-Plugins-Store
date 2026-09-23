---
title: Codex OAuth 额度窗口保活插件实施方案
status: 可实施设计，尚未编码
plugin_id: codex-window-keeper
host: CLIProxyAPI v7.3.9 及以上
---

# Codex OAuth 额度窗口保活插件实施方案

## 1. 目标

新增独立插件 codex-window-keeper。它只处理 Codex OAuth 凭证，全局默认关闭。打开后，插件按每个账号自己的 usage 响应记录实际存在的窗口，而不是按 Free、Plus、Pro 写死窗口组合。当这次响应里被选中的窗口全部不再阻断请求时，尽快用配置好的模型和思考级别发送一条完整消息。

这样做是为了避免额度恢复后一直空等，直到使用者下次手动开口才开始下一个周期。账号之间的周期不同，状态只进本插件的 SQLite，不进 cpa-manager-plus 的库。

发送失败按错误类别处理。额度仍耗尽不是传输失败，不能指数退避地连续打模型。网络错误和 5xx 才指数退避。401、403、模型不可用、凭证被宿主冷却，都停止这一轮并给出明确状态。

## 2. 尚未证实的前提

官方定价页只说明存在 five-hour period，并且 weekly limits may also apply；reset 时间以 dashboard 和 /status 为准。它没有说窗口从下一条成功消息开始。依据见 docs/research/codex-oauth-window-activation-sources.md。

因此成功分成两级：

1. 发送成功：指定账号的流式响应出现 response.completed，并且输出文本非空。
2. 起算成功：发送前后各读一次 /wham/usage。该窗口新的 reset_at 约等于发送完成时间加上 limit_window_seconds，并且相对发送前的 reset_at 向前移动了一个周期。

第 2 级失败时，把该窗口记为 fixed_schedule，以后只监视、不再发信。继续发只会消耗已经恢复的额度，不能让下一个周期提前到来。首次安装只建立基线，不立即发信。

/wham/rate-limit-reset-credits 是另一类可消耗的重置券。本插件只读 usage，永不兑换重置券。

官方建议无人值守自动化默认使用 API key。ChatGPT OAuth 只适合操作者明确要用该订阅账号，并且运行在自己的可信 CPA 上。插件默认关闭，不绕过限额，不把请求改派到其他账号，也不隐藏用量。操作者需要自行确认自己的套餐允许这种使用。

## 3. 插件边界

cpa-manager-plus 已经能解析 Codex 窗口，但它的巡检还会采样、处置和自动封禁。保活发信会真实消耗额度，失败域也不同。放进同一进程会让一次实验性发信影响监控库和计费库。

仓库约定插件之间没有 Go 依赖。新插件复制窗口分类规则和测试向量，不 import cpa-manager-plus。

宿主下限是已核对的 CLIProxyAPI v7.3.9。更早宿主如果没有 AuthID 锁定语义，插件只允许保存配置，发信保持不可用。不能退化为不锁定账号的普通路由。

## 4. 操作者可见的行为

- 全局设置对所有 Codex OAuth 账号生效。账号行可以覆盖模型、思考级别、提示词、窗口选择方式，以及单独停用。
- 每个可发送世代最多成功发一条消息。同一账号这次实际返回的多个窗口同时解除时，这一条同时覆盖它们，不按窗口条数连发。
- 任一被选中的窗口仍显示 limit_reached，且结束时间在未来，就不发。先等到最晚的结束时间，再读 usage。
- 套餐只用于展示和形状对照。Free 常见只有月窗口，Plus 与 Team 常见是 5 小时、周、月三个窗口，Pro 及更高常见是周和月；响应里多出来或少掉的窗口以响应为准。
- 手动立即发信也走同一状态机和账号锁，不是旁路。
- 凭证 ID 是稳定身份。auth_index 变化但 chatgpt_account_id 相同的记录要合并，不能重置周期。
- 关闭插件、plugin.quiesce 或进程退出会取消正在读的流，不会把半截响应记成成功。

管理面沿用双路径。下表写绝对路径；相对路径去掉 /v0/management 前缀后也要匹配。

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | /codex-window-keeper/health | 进程、数据库、调度器和宿主回调是否可用 |
| GET, PUT | /v0/management/codex-window-keeper/settings | 全局设置 |
| PUT | /v0/management/codex-window-keeper/connection | CPA Base URL 与 management key |
| GET | /v0/management/codex-window-keeper/accounts | 账号、窗口和下一次动作 |
| PUT | /v0/management/codex-window-keeper/accounts/{auth_id} | 单账号覆盖 |
| POST | /v0/management/codex-window-keeper/accounts/{auth_id}/probe | 只读 usage |
| POST | /v0/management/codex-window-keeper/accounts/{auth_id}/activate | 手动走一轮发信判断 |
| GET | /v0/management/codex-window-keeper/attempts | 最近尝试，默认 100 条 |

前端用 vite-plugin-singlefile 打成一个 index.html。资源路由是 /v0/resource/plugins/codex-window-keeper/app。页面包含总开关、全局模型与思考级别、连接状态、账号表、套餐、每个窗口的开始和结束时间、观察结果与套餐提示是否一致、假设结论和最近一次错误。文案先做 en 与 zh-CN，键结构预留 zh-TW 和 ru。临时结构原型在 docs/design/codex-window-keeper-ui.prototype.html，用 ?variant=A|B|C 比较账号台账、下一次动作和例外收件箱，不进入正式页面。

## 5. 模块

外部缝只留在插件注册、管理 API 和 Keeper。测试不直接调用宿主。

### 5.1 WindowClock

纯函数，无 IO。输入是上一份持久化窗口、这次 usage 快照和 now。输出是每个窗口的相位，以及下一次值得醒来的时间。

相位：

- baseline：还没有见过这个窗口耗尽，只记录快照。
- blocked：limit_reached 为真，或用量达到阻断阈值，且 reset_at 在未来。
- due：曾经 blocked，现在 now 大于等于 reset_at 加 skew。此时仍不发信，只表示应该重新探测。
- clear：最新探测显示所有被监视窗口都不阻断。若其中至少一个来自本世代的 blocked，则产生一个 Generation。
- anchored：发信后判定窗口确实被这条消息向前拨动。
- fixed：发信后判定窗口不是由消息起算。
- inconclusive：前后快照无法判断。保留证据，下一周期再试一次；同一个世代不连续试。

GenerationKey 是账号 ID，加上每个参与窗口的 limit_id、slot、周期桶和发信前结束时间。数据库对成功尝试做唯一约束。套餐变化或窗口消失不会沿用旧世代。

### 5.2 ActivationPolicy

纯函数。输入是世代、最近一次尝试、错误类别和设置。输出只有 probe、send、backoff、pause_account、stop_window 五种决定，外加 not_before。

退避规则：

- 可重试类别：网络错误、408、409、425、500、502、503、504、流在 response.completed 前断开、空 incomplete。
- delay = min(max_delay, base * 2^(attempt-1))，再乘 0.8 到 1.2 的随机系数。默认 base 为 2 秒，max_delay 为 5 分钟，每个世代最多 5 次模型调用。
- 429 或 usage 仍显示阻断：不消耗上面的 5 次，下一次只做 usage 探测。探测间隔使用 poll_interval。连续多次仍阻断时，下一次探测放到快照里的 reset_at；没有 reset_at 时最多 15 分钟。
- 401 或 403：pause_account，原因 reauth。
- 400 且错误体是模型不存在，或思考级别不被该模型接受：pause_account，原因 config，不对所有账号盲重试。
- 宿主返回 auth_not_found，或账号 disabled、unavailable，或 next_retry_after 未到：本轮不发，等宿主冷却结束再探测。

### 5.3 UsageProbe

只发 GET https://chatgpt.com/backend-api/wham/usage。请求经 CPA api-call，body 为 JSON 对象，字段是 auth_index、method、url 和 header。header 里的 Authorization 使用 Bearer $TOKEN$，另加 Content-Type application/json 和配置的 User-Agent。chatgpt_account_id 存在时加 Chatgpt-Account-Id。token 不进入插件日志。

官方 Codex 客户端把同一份状态映射成 RateLimitWindow：used_percent、由 limit_window_seconds 换算的 window_minutes，以及来自 reset_at 的 resets_at。来源是 openai/codex 的 protocol.rs 中 RateLimitWindow，和 backend-client client.rs 的 map_rate_limit_window。已核对的 RateLimitWindowSnapshot 只有 used_percent、limit_window_seconds、reset_after_seconds、reset_at，没有单独的开始时间字段。

因此每个窗口落地为：

- 结束时间优先用 reset_at 或 resets_at。没有结束时间但有 reset_after_seconds 时，用探测时刻加上该秒数，并标记为 probe_relative。
- 周期优先用 limit_window_seconds；否则用 window_minutes 乘 60。
- 开始时间优先用响应里的 start_at、started_at、window_start 或 window_started_at。这些名字在已核对的官方结构里没有出现；一旦真实响应带了其中任何一个，就记录首次见到的字段并优先使用。否则在结束时间和周期都存在时，开始时间等于结束时间减去周期，并标记为 derived。
- 同时接受 primary_window / primary 和 secondary_window / secondary 两种拼写。一个 rate_limit 对象里的两个槽位都保留，不再把“次窗口不是 5 小时就是周或月”折叠成只会留下两个类别的结果。

周期桶按秒数动态划分，容差 2%：18000 秒是 five_hour，604800 秒是 weekly，28 到 31 天是 monthly。落在桶外的窗口保留为 custom，并带上原始秒数，不丢弃。

采集范围：

1. 主 rate_limit 的全部窗口始终参与。
2. code_review_rate_limit 默认只展示，不参与发信判断。它是另一块额度。
3. additional_rate_limits 默认使用 fill_gaps。某个额外限额的周期桶是主限额还没有的，就纳入发信判断，用来接住 Plus 把月窗口放在额外对象里的情况。限额名称或 normal_model_slug 明确指向另一个模型时，不纳入当前模型。设置可以改成 all 或 none。
4. window_kinds 为空表示自动使用上面选出的全部窗口。非空时只保留列出的周期桶；响应里没有的桶产生缺失警告，但不能凭套餐补出一个不存在的窗口。

plan_type 来自 usage 响应，缺失时才回退到凭证声明。它只生成对照，不决定窗口：

| 对照分组 | 常见窗口 | 计划值 |
| --- | --- | --- |
| free | monthly | free、guest、free_workspace |
| plus_team | five_hour、weekly、monthly | plus、team、edu_plus，以及 team-like 的 self_serve_business_prolite |
| pro_and_above | weekly、monthly | pro、prolite、business、enterprise 全家、edu_pro、ent26 |
| observe_only | 不假设 | go、edu、usage-based 计划、unknown 和其他未列出的值 |

观察到的周期桶和对照不一致时，账号记 shape_mismatch，界面显示多出或缺少哪些桶。发信仍只看实际选中的窗口。cpa-manager-plus 现有分类器每个 rate_limit 最多留一个 5 小时和一个次窗口，不能直接复用；新解析器要单独用三窗口样本做测试。

### 5.4 MessageSender

发信走 host.model.execute_stream，不走 api-call。api-call 不会做 Codex 执行器必须做的 stream=true、instructions 归一和身份头处理，也不能证明响应被算作一次完整 Codex 消息。

回调字段：

- entry_protocol 为 openai-response。
- exit_protocol 设计锁定为 codex。实现时用 v7.3.9 translator 常量做一次测试确认确切字符串；若常量不同，以测试得到的常量为准。
- model 默认 gpt-5.4。这个 ID 只因为本仓库费率笔记已经核验过它，操作者应改成自己套餐里实际有额度的模型。
- stream 为 true。
- forced_provider 为 codex。
- auth_id 为 HostAuthFileEntry.ID，不是 auth_index。
- body 是 OpenAI Responses JSON：store 为 false；input 是一条 user 消息，内容类型 input_text，默认文本 Reply with exactly OK.；reasoning.effort 默认 low。

思考级别允许 minimal、low、medium、high、xhigh。宿主若拒绝某个值，账号进入 config 暂停。默认不设置 service_tier。可选值只接受本仓库已经出现过的 default、standard、flex。不把 fast 或 priority 放进默认路径，避免保活消息消耗更快的额度池。

成功条件要同时满足：

- 流正常结束，没有 stream error。
- 事件中有 response.completed。
- response.incomplete、response.failed 和 error 都算失败。
- 完成事件或 output_text.delta 中有非空文本。
- 插件读完后调用 host.model.stream_close。

请求和响应原文不入库。尝试记录只留状态码、错误类别、截断到 120 字的输出、response id、token 计数和耗时。

### 5.5 Keeper

Keeper 是唯一编排者。Catalog、Prober、Sender 和 Store 都由构造函数注入。

Catalog.ListCodexOAuth 返回账号引用。Prober.Probe 返回 usage 快照。Sender.Send 返回发信结果。Store 提供 Load、Save、Claim、Release 和 RecordAttempt。Claim 使用 SQLite 立即事务抢租约，租约默认 2 分钟，长于单次请求超时。

一轮账号处理：

1. Claim。抢不到就跳过。
2. 探测 usage。探测失败只记录并按传输退避，不发信。
3. WindowClock 给出决定。非 send 则保存状态并释放租约。
4. send 前在同一个事务里把尝试写成 started，并让租约覆盖整个请求超时。进程若在这里崩溃，重启后续接这一条 started，先探测，不直接再发。
5. 发信。成功后立刻再探测。
6. 比较每个参与窗口发信前后的开始和结束时间。新的开始时间落在发送完成时刻附近，且新的结束时间约等于开始时间加周期，记 anchored。结束时间没变，或发信前开始时间已经早于本次发送，记 fixed。只有 probe_relative 结束时间、无法比较时记 inconclusive。
7. 释放租约。取消也要释放。进程退出后靠租约到期回收。

调度循环只有一个 goroutine，任务队列不放在内存里。每一轮先从同一个 SQLite 读出到期账号，处理完立刻写回下一次 not_before。循环只记住最近一个 timer。到期时间是所有账号 not_before 里最早的一个，并且不超过 poll_interval，这样新加的账号不用等到旧窗口结束才被看见。设置变更通过容量为 1 的 wake channel 打断，模式与 inspection_settings.go 第 344 行的 scheduleInspections 相同。全局同时发信数默认 1，最大 4。探测并发默认 2，最大 4。进程退出前先停收新任务，等已持有的租约写完结果或标成可恢复，再关闭这一个数据库。

## 6. 配置

宿主 YAML 只放启动所需项。运行中修改以 SQLite 里的设置为准，避免热加载把界面改动覆盖回去。

~~~yaml
data_dir: data/codex-window-keeper
enabled: false
poll_interval_seconds: 20
activation_skew_seconds: 3
request_timeout_seconds: 90
max_concurrent_sends: 1
max_concurrent_probes: 2
max_attempts: 5
retry_base_seconds: 2
retry_max_seconds: 300
window_mode: auto
window_kinds: []
include_code_review: false
include_additional: fill_gaps
model: gpt-5.4
reasoning_effort: low
service_tier: ""
prompt: Reply with exactly OK.
user_agent: codex_cli_rs/0.76.0
~~~

校验边界：

- poll_interval_seconds：5 到 600。
- activation_skew_seconds：0 到 120。
- request_timeout_seconds：15 到 300。
- max_attempts：1 到 8。
- prompt：1 到 500 个字符，不能为空。
- window_mode 只允许 auto。显式 window_kinds 可为空；非空时每个值必须是 five_hour、weekly、monthly 或 custom。
- include_additional 只允许 fill_gaps、all、none。
- management key 的密文写在同一个 SQLite 的 settings 表，用插件数据目录里的一个非数据库主密钥做 AES-GCM。响应里只返回是否已配置，不回显。不因为密钥再开一个数据库。

账号覆盖使用指针或显式 inherit。enabled 为 false 只停这一张凭证。宿主把凭证禁用后，插件不再探测或发信，但保留历史。

## 7. 数据库

插件只有一个 SQLite：{data_dir}/keeper.sqlite。设置、账号、窗口、尝试、租约和迁移都在这个文件里。不按账号分库，不为日志、界面缓存或密钥再打开第二个数据库。路径来自 data_dir，启动时打开已有文件并迁移；文件不存在才创建。禁止写成 keeper-2.sqlite 或带时间戳的新库。

连接参数使用 busy_timeout(5000)、foreign_keys(1) 和 _txlock=immediate。打开时设置 WAL 与 synchronous=FULL。进程内只保留一个写连接，调度循环和管理请求共用它。迁移表沿用 schema_migrations。

重启后续接，而不是重头扫描后重发：

1. 打开原来的 keeper.sqlite，先跑未完成迁移。
2. 把本进程之前留下的租约全部释放。租约只防同一时刻重入，不代表任务完成。
3. 找到 status 为 started 且没有结束时间的尝试。不把它们标成成功，也不立即再发。对应账号的 not_before 改成现在，相位保持原样。
4. 读取每个账号已经写下的 not_before、窗口结束时间和见过耗尽的标记。到期的马上进入探测；没到期的继续等原来的时间。
5. 探测确认窗口仍阻断，就只更新结束时间。确认已经解除且这一世代没有成功记录，才发信。成功世代的唯一索引挡住重启后的第二次发送。
6. 管理密钥密文、总开关和账号覆盖都从这一个库读回。CPA 重启后不需要重新在界面保存，任务才会继续。

未到期任务在重启后不得被提前发出。已经成功的世代不得因为进程重启再发一条。

accounts 保存 auth_id、auth_index、chatgpt_account_id、显示名、邮箱、plan_type、对照分组、shape_mismatch、宿主 disabled 与 unavailable、覆盖配置、租约所有者和 lease_until_ms、账号暂停原因。

windows 以 account_id、limit_id、slot、period_seconds 唯一。limit_id 区分 codex、code_review 和 additional。slot 是 primary、secondary 或额外序号。字段包括周期桶、starts_at_ms、ends_at_ms、时间来源 explicit、derived 或 probe_relative、used_percent、limit_reached、是否参与发信、相位、最近一次阻断的结束时间、发信前后快照 JSON、假设结论和证据摘要。本次响应不再出现的旧窗口标 absent，停止参与发信，但不删除历史。新出现的窗口从 baseline 开始，必须先见过阻断再解除，才允许触发第一次发信。

attempts 保存世代键、开始和结束时间、结果、HTTP 状态、错误类别、第几次、not_before_ms、截断输出和用量计数。成功世代有部分唯一索引。

settings 是单行 JSON。不保存上游 token，不保存完整请求体。

## 8. 插件骨架

~~~text
plugins/codex-window-keeper/
  assets/logo.svg
  go/main.go
  go/go.mod
  go/internal/config/
  go/internal/clock/
  go/internal/policy/
  go/internal/usage/
  go/internal/send/
  go/internal/probe/
  go/internal/store/
  go/internal/keeper/
  go/internal/api/
  web/
~~~

go.mod 模块路径是 github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go。Go 1.27.1，依赖 github.com/router-for-me/CLIProxyAPI/v7 v7.3.9、modernc.org/sqlite 和 gopkg.in/yaml.v3，不使用 replace。

注册约定：

- metadata.Name 使用 Codex Window Keeper。
- pluginVersion 初始 0.1.0，由 CI ldflags 注入。
- schema version 保持 1。
- capability 只需 management_api。不注册 usage 钩子，也不拦截别人的响应。
- 方法处理 plugin.register、plugin.reconfigure、management.register、management.handle 和 plugin.quiesce。
- 宿主回调只封装 host.auth.list、host.auth.get、host.http.do、host.model.execute_stream、host.model.stream_read、host.model.stream_close 和 host.log。

host.auth.get 只提取账号 ID、套餐和账号标识。读取后立刻丢弃原始 JSON，日志组件再做一次 token 字段过滤。

plugins.json 增加一条 codex-window-keeper，版本 0.1.0，标签 codex、oauth、quota。发布标签必须是 codex-window-keeper-v0.1.0。不手改生成的 registry-v2.json。

## 9. 测试

纯函数测试不启动宿主：

- 同一 rate_limit 中 primary 为周、secondary 为月时，两个窗口都保留。
- Plus 样本含主限额的 5 小时和周，以及 additional 中主限额没有的月窗口；fill_gaps 会把月窗口纳入，all 和 none 的结果不同。
- 额外限额明确属于另一个模型时，不阻断当前模型。
- Free 只有月窗口、Pro 只有周和月时，不补造 5 小时窗口；与对照分组不同只记 shape_mismatch。
- reset_at 加 limit_window_seconds 得到 derived 开始时间；响应自带 start_at 时使用 explicit，并且不再减周期。
- 只有 reset_after_seconds 时得到 probe_relative 结束时间。
- 只有基线时不发信。新出现的窗口在被见过阻断之前不发信。
- 一个窗口阻断时不发信，醒来时间是结束时间加 skew。
- 5 小时已恢复但月窗口仍阻断时不发信。
- 多个窗口同一次恢复只产生一个世代。
- 同一世代成功后再次轮询不发信。
- 新开始时间接近完成时刻，且新结束时间约等于开始时间加周期时记 anchored。
- 结束时间不变，或发信前窗口已经开始时记 fixed，后续 send 决定消失。
- 429 不增加模型尝试次数。
- 500 的第二次等待约是第一次的两倍，且不超过上限。
- 超过 max_attempts 后不再调用模型。
- 过期的 started 尝试不会在没有新探测的情况下重发。

适配器测试使用假 HTTP 和假模型流。要覆盖：锁定的 auth_id 被原样传递；流只有 delta 没有 completed 时失败并关闭流；取消上下文时不写成功。

SQLite 测试覆盖迁移、租约互斥、成功世代唯一约束和 busy 后重试。另开一个重启测试：写入一个未到期窗口、一个已到期窗口和一条 started 尝试，关闭连接后再打开同一个文件。未到期窗口不能发信，已到期窗口只探测，started 尝试不会直接变成第二次发送。测试里全程只能出现一个数据库文件。

合并门槛是 go test ./... 与 go test -race ./...。前端只给设置校验和账号状态渲染做最小组件测试。

## 10. 实施顺序

1. 先写 WindowClock、ActivationPolicy 和 usage 解析的失败测试，再写实现。
2. 加 SQLite 迁移、租约和尝试唯一约束。
3. 接宿主目录、api-call 探测和流式发信。这一阶段仍默认 enabled 为 false。
4. 加管理 API、健康检查和单文件页面。
5. 用一张真实测试账号做一次受控实验：等到自然耗尽，记录发信前后 usage。结果写回研究笔记。未得到 anchored 之前，不把插件描述改成已经能提前打开下一个周期。
6. 实验通过后，再登记 plugins.json 并按作用域标签发布 0.1.0。

## 11. 明确不做

- 不在窗口已经打开时周期性地发保活消息。
- 不自动调用 reset credit。
- 不修改、禁用或删除凭证。
- 不在锁定失败时换一个账号试。
- 不把这次生成写入 cpa-manager-plus 的计费库。宿主自己的 usage 钩子会看到这条真实请求，这是期望行为。
- 不把 fast 设成默认，不猜测未核验模型的价格。
- 不按 Free、Plus、Pro 写死要等待哪些窗口，也不在响应缺少某个窗口时补造它。
