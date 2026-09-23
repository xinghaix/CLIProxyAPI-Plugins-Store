# CPA-Plus：Codex OAuth 计费精度调研与改动方案

## 结论摘要

落地口径已确定：OAuth 与 API-key 用量统一按 OpenAI API 官方美元费率估算，使用同一套输入/缓存/输出、上下文档位和 API Fast 规则；不按 AuthType 切换到 Codex Credits 费率。OAuth 侧应标为“API 价格折算/估算”，反映 token 使用的 API 等价值，不代表 ChatGPT 订阅实际扣款或套餐剩余额度。

调研确认的代码现状是价格模型只有五个平面单价，监控页与 Go 汇总对 Fast 的处理也不一致。官方 Codex OAuth 的 Credits 规则与 API 美元费率不同；按本次定下的产品口径，Credits 作为官方额度语义背景保留，但 CPA-Plus 成本估算统一采用 API 费率。该估算能按 token 类别、上下文档位和 Fast 更完整地反映用量等价值，但不能推导 ChatGPT 账号的实际套餐余额。

## 1. 官方计价模型：必须区分 API 与 Codex 登录

### 1.1 OpenAI API：每百万 token 单价、上下文档位和服务档位

用户提供的 [API 标准价格页](https://developers.openai.com/api/docs/pricing?latest-pricing=standard)将价格按每 1M tokens 展示，分别列出输入、缓存输入、缓存写入、输出，并在适用模型上区分 short-context 与 long-context 价格。它不是按账号月累计用量逐渐跨档的“月度阶梯”；这里的档位与单次请求上下文长度及 token 类别有关。表中 GPT-5.4 和 GPT-5.5 行标有 “<272K context length”，同时另列 long-context 单价；实现时应使用模型自己的上下文分档规则，不应把一个门槛套给所有模型。价格表对这两款模型的 Fast 行没有列出 long-context 价格，因此不能自行把 long-context 标准单价乘倍率补出来。

下面列出官网当前页面中的代表性费率（USD / 1M tokens；顺序为输入 / 缓存输入 / 缓存写入 / 输出）。这些是调研时的页面快照示例，不是可永久硬编码的常量。

| 模型 | API Standard：short | API Standard：long | API Fast：short | API Fast：long |
|---|---:|---:|---:|---:|
| GPT-5.4 | 2.50 / 0.25 / — / 15.00 | 5.00 / 0.50 / — / 22.50 | 5.00 / 0.50 / — / 30.00 | 官网表未列出 |
| GPT-5.5 | 5.00 / 0.50 / — / 30.00 | 10.00 / 1.00 / — / 45.00 | 12.50 / 1.25 / — / 75.00 | 官网表未列出 |
| GPT-5.6 Sol | 4.00 / 0.40 / 5.00 / 20.00 | 8.00 / 0.80 / 10.00 / 30.00 | 8.00 / 0.80 / 10.00 / 40.00 | 16.00 / 1.60 / 20.00 / 60.00 |

Fast mode 的官方 API 配置值是 service_tier=fast；priority 仍作为兼容值。重要的是应读取实际处理结果而非仅看请求意图：API Responses 对象的 service_tier 是实际使用的档位；GPT-5.6 及更早模型即使请求 fast，返回值仍为 priority；Fast 请求被降级至标准速度时会返回 default，并按标准价格收费。项目级设置也可能让未指定 service_tier 的请求默认走 Fast。缓存输入折扣在 Fast 下仍适用。以上规则与费率详见 [Fast mode API 指南](https://developers.openai.com/api/docs/guides/fast-mode)及[官方 Fast 价格表](https://developers.openai.com/api/docs/pricing?latest-pricing=fast)。

### 1.2 ChatGPT 登录的 Codex OAuth：Credits，不是 API 美元账单

OpenAI 的 [Codex Speed 文档](https://learn.chatgpt.com/codex/speed)明确把登录 ChatGPT 的 Fast mode 定义为 ChatGPT Credits 特性；使用 API key 的 Codex 才采用 API token pricing，且 ChatGPT Credits 倍率不适用。Codex 订阅的标准 token 费率使用 credits / 1M tokens，官网当前价格表的代表值为：

| 模型 | 输入 credits / 1M | 缓存输入 credits / 1M | 输出 credits / 1M |
|---|---:|---:|---:|
| GPT-5.6 Sol | 100 | 10 | 500 |
| GPT-5.5 | 125 | 12.50 | 750 |
| GPT-5.4 | 62.50 | 6.25 | 375 |

Codex Credits 的说明还指出没有单独的 cache-write 收费；API-key 使用则遵循 API 价格。Codex Fast 的 credit 倍率按模型族另行定义：GPT-5.6 / GPT-5.5 为标准 credits 的 2.5 倍，GPT-5.4 为 2 倍；较新的 GPT-6 系列也有自己的可用范围和倍率说明。Codex 定价页特别提醒，token credit 单价本身不能直接推导套餐内剩余/包含用量，套餐用量应以账号 usage dashboard 为准。以上参见[Codex Pricing](https://learn.chatgpt.com/codex/pricing)和[Codex Speed](https://learn.chatgpt.com/codex/speed)。

官方 Codex OAuth 的真实订阅额度仍以 Credits 衡量，且 GPT-5.6 Sol 的 Codex Fast 为标准 Credits 的 2.5 倍，而 API Fast 价格为 API Standard 的 2 倍。本方案选择统一采用 API 价格表，因此 OAuth 事件显示的是“API 等价成本估算”，不是订阅实际扣点/扣款；它能统一折算 token 使用成本，但账户套餐剩余额度仍需独立的官方 quota 数据，不能由 API 金额推导。

## 2. CPA-Plus 落地前实现与误差来源

> 本节是实施前代码调研快照；落地后的行为以第 6 节为准。

1. **价格数据结构是平面单价。** Go 的 Price 仅包含 prompt、completion、cache、cacheRead、cacheCreation 及来源/更新时间元数据，没有上下文档位、Fast 费率、官方 API 价格来源/计价说明或生效版本。[Price 定义](../../plugins/cpa-manager-plus/go/internal/store/analytics.go#L13-L23) 对应的价格管理 API 也只读写这一结构。[价格 API](../../plugins/cpa-manager-plus/go/internal/api/api.go#L78-L94)
2. **自动同步的来源无法表达官网完整规则。** 现有同步源为 models.dev、LiteLLM 与 OpenRouter；同步转换只映射 input/output/cache-read/cache-write 等单值字段，不能表示 OpenAI 官网的 short/long context × Standard/Fast 费率矩阵。[来源与优先级](../../plugins/cpa-manager-plus/go/internal/pricesync/sync.go#L16-L27) [同步流程](../../plugins/cpa-manager-plus/go/internal/pricesync/sync.go#L97-L126) [来源数据映射](../../plugins/cpa-manager-plus/go/internal/pricesync/sync.go#L241-L274)
3. **前端用静态倍率代替官方费率矩阵。** 监控页先按扁平单价拆分输入、缓存、输出，然后乘 service-tier 倍率；倍率表只显式识别 priority，GPT-5.5 为 2.5、GPT-5.4 为 2，其他 priority 模型一律 2，未识别值一律 1。因此当记录实际保留 fast 字符串时不会加价，且无法表达 short/long context 与各 token 类别的完整 API 价格表。[前端估价与倍率](../../plugins/cpa-manager-plus/web/src/components/MonitoringView.vue#L1491-L1541)
4. **Go 统计端没有应用服务档位。** 聚合成本由 cost(row, price) 计算，公式仅使用五个平面单价和 token 计数；ServiceTier 不在成本计算参数/公式中。因此 Go 汇总与前端逐事件金额可能不一致。[聚合调用](../../plugins/cpa-manager-plus/go/internal/store/analytics.go#L365-L386) [Go 平面成本公式](../../plugins/cpa-manager-plus/go/internal/store/analytics.go#L423-L425)
5. **采集侧已有大部分 API 估价所需事件字段。** ToEvent 原样复制 Provider、AuthType、ReasoningEffort、ServiceTier、输入/输出/推理/缓存 token 数。[用量事件映射](../../plugins/cpa-manager-plus/go/internal/ingest/ingest.go#L148-L168) usage_events 也保存 service_tier 和这些 token 分类。[表结构](../../plugins/cpa-manager-plus/go/internal/store/store.go#L90-L112) 但事件没有记录最终采用的官方 price schedule 版本或计算后的 USD API 等价值。AuthType 仅需保留作来源分组和 OAuth 标签，不应决定使用哪一套费率；实现前仍需验证 ServiceTier 是实际处理值以及缓存 token 子字段互斥。
6. **估算单位和历史费率缺少可追溯性。** 事件账本保存用量字段，汇总时根据当前 Price 重新计算成本；Price 只记录最近更新时间而非按生效时间保存费率。因此更新模型单价可能改变旧事件的汇总值，旧事件无法自动恢复当时费率。即便产品统一采用 API USD，费用结果也应返回“API 等价估算”、币种、命中的上下文/tier 和 schedule 版本，避免被误读为 OAuth 订阅实扣。
7. **模型身份已有明确约束。** 项目 README 说明上游 ResponseModel 仅用于诊断，不参与价格查找和费用计算；新实现应继续用 CPA 解析出的 routed/resolved billing model，不要拿观察到的 response_model 覆盖计费模型。[现有计费/响应模型约定](../../plugins/cpa-manager-plus/README.md#L83)

## 3. 推荐方案

### 3.1 最终口径：统一按 API 价格估算

OAuth 与 API-key 的所有用量事件都采用 OpenAI 官方 API 美元价目表，统一计算 API 等价成本。这样可在同一单位下按模型、short/long context、输入/缓存/输出 token 和 Standard/Fast 反映观察到的 token 用量。AuthType 仅用于来源筛选、分组和 UI 标签，不用于切换到 Codex Credits 价格。

- **Codex OAuth：** 显示“API 等价成本估算（USD）”，明确它不是 ChatGPT 订阅实际扣款或真实 Credits 扣减。
- **API key：** 显示“API 成本估算（USD）”，适用同一张官方 API 费率表。
- **信息不足或价格表不支持该组合：** 显示“无法估算 / 费率未配置 / 档位未知”，不要静默按 $0、Standard 或通用 2 倍处理。

这里的“完整”指对观测到的 token 用量及 API 价格规则进行完整折算；如果要展示 ChatGPT 套餐实际剩余额度或真实 Credits 扣减，必须另接官方 quota/usage 数据源，不能从 API 美元估算推导。

### 3.2 建一个小接口、隐藏计费复杂度的单一估算模块

建议新增 Go 内部 pricing 模块，负责按事件和费率表返回一个有类型的结果，而不是继续让 Vue 与 analytics 各自实现一套公式。输入为 routed model、事件时间、实际 service tier 和 input/output/cached/cache-write token 分类；AuthType 保留为来源元数据但不决定费率。输出为 USD API 等价金额、匹配的上下文档位、实际服务档位、price schedule 版本、估算状态和警告。Analytics 分组与 event JSON 都调用同一个模块；前端只格式化和解释结果。

费率表按“模型 + 生效时间 + 上下文档位 + service tier”组织，统一保存官方 API USD 单价，并为每格记录输入、缓存输入、缓存写入、输出费率。不要只保存一个 multiplier：Fast 价格和缓存类别应直接以官方费率为准。保留现有 Price 五字段作为旧模型和非 OpenAI 平面价格的兼容回退；OpenAI 官方复杂费率作为独立、带来源链接和更新时间的 schedule，避免第三方同步覆盖专用费率。

所有认证方式统一应用 API 费率，但 service tier 仍须按实际处理结果计价：API fast 与 priority 作为兼容别名；default 表示被降级后按 Standard 计费。不得把请求意图当成实际档位，也不得把 Codex 订阅 Fast 的 Credits 倍率混入 API USD 估算。若 host 没有可靠提供实际使用档位，应将估算标为不确定或补齐该事件字段。

上下文“阶梯”建议实现为**单事件选档**：按该请求的输入上下文和模型对应的规则选中一个 short/long schedule，再用该档位给此事件的各 token 类别估价，然后汇总。不要先把多事件 tokens 加总后跨阈值，也不要在没有来源的情况下推导 Fast long-context 价格或套通用门槛。GPT-5.4/5.5 的 272K 边界及统计字段与官网“context length”的精确对应关系，应在编码前核实并写成阈值边界测试。

缓存 token 应先归一化成互斥的计价桶，再匹配 API 费率，避免 cached_tokens 与 cache_read_tokens 在某些 host/provider 载荷中重叠时双重收费。官网表未列出 cache-write 费率的模型不得自行补值；需按官方规则确定其是否包含在其他 token 类别中，否则将该项标为未配置。

### 3.3 版本化与历史结果

费率会变动，建议将官方价格表做成仓库内的可审阅 schedule 快照（每条含 source URL、生效时间、抓取/更新日期、币种、每百万 token 单位、支持模型与档位），由版本更新流程显式更新；不建议运行时抓取 OpenAI 文档 HTML。为保持历史可解释性，可以选择：

- 为 usage_events 保存计算后的 USD estimate、API pricing basis、schedule_id、context tier、service tier；或
- 保存有有效期的不可变费率快照，并依据事件时间查询对应版本。

对已有历史事件，由于未保存费率版本，只能按当前 schedule 重算并标注“按当前费率回算”，不能把回算值冒充为历史真实账单。若初期不做历史快照，至少在界面解释费率调整会影响历史估算。

### 3.4 展示与价格管理

监控事件行、详情弹窗、成本排行和总览汇总都应展示同一服务端结果。可显示“Codex OAuth · API 等价估算 $0.012 · Fast · long context · 费率版本 …”或“API key · $0.012 · Fast/short context”。没有费率或上下文档位时应显示状态，不要把缺失值格式化为 0。模型价目管理视图应标明官方 OpenAI schedule 与外部候选/手动价格来源，并避免旧单价编辑覆盖官方阶梯表。

## 4. 建议实施顺序与验收

1. **样本核验：** 从运行中的 Codex OAuth 与 API-key 请求各取安全脱敏 usage 事件，确认 service_tier 是实际处理值、token 子字段互斥、模型名/别名是 routed model。确认 AuthType 仅用于标签/分组，并确保相同模型、token 用量和实际 tier 在 OAuth 与 API-key 下产出同一 API USD 估算。
2. **费率表与纯估算测试：** 固定一份官方来源的 API USD schedule 夹具；先写 Standard/Fast/上下文档位的 golden tests，再接入生产调用。
3. **单一后端计算：** 新增 internal/pricing 模块，让 Go event JSON、账本逐事件 cost 和所有 analytics 分组共用 evaluator；移除前端倍率公式和重复成本拆分逻辑。
4. **存储迁移及历史语义：** 增加估算金额、USD/API-equivalent basis、schedule version 及实际档位/上下文档位字段，或采用有有效期的费率表。旧数据显式标记为旧算法或当前价回算，不静默混进新历史。
5. **界面和 price sync 接入：** 显示 USD API 等价值、Standard/Fast、short/long、费率版本与未知状态；OAuth 标签不改变费率。外部 models.dev/LiteLLM/OpenRouter 同步仍可服务通用平面价格，但不得覆盖 OpenAI 官方专用 schedule。

必须覆盖的验收用例：

- GPT-5.4 / GPT-5.5 的 272K 附近（阈值下、等于阈值、阈值上）及 GPT-5.6 long-context，确认门槛逐模型配置；不支持的模型/档位不得外推。
- 同一事件的 Standard、fast、priority、default（包括 Fast 请求被降级）以及未知 tier；OAuth 与 API-key 对相同模型/token/tier 必须给出相同 API USD 等价值，不得套用 Codex Credits 倍率。
- 验证 GPT-5.6 Sol API Fast 按官方 API 表为 Standard 的 2 倍；即使 OAuth 事件的官方 Codex Credits Fast 为 2.5 倍，CPA-Plus 的统一 API USD 估算也不得切换到该 Credits 倍率。
- uncached input、cached input、cache-write、output 单独及组合样例；缓存桶互斥、无双计；官方未列出的 API 费率不做推测。
- event 单条金额与所有汇总/分组金额一致；更新费率后历史是按时间版本稳定，或明确显示按当前费率回算。
- model alias、空模型、无费率、未知 tier、缺少上下文信息时的状态提示；AuthType 不影响统一价格选择，不得默认为 0 或套 2 倍。

## 5. 当前范围与后续扩展

- 官方 schedule 目前只收录已核验的 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol` 费率；对应的日期版本 model ID 复用基础 schedule。其他未经核验的型号/变体不继承猜测费率，改用已有自定义平面单价；没有自定义价时返回 unpriced。
- 对以上型号按单次请求输入 token 判断上下文：`>272,000` 才采用 long，等于 `272,000` 仍为 short。GPT-5.4/5.5 Fast long-context 与未公布的 cache-write 桶明确返回无法估价，不外推。
- 使用优先级是：唯一、未冲突的 OpenAI 响应 service_tier > 记录的请求 tier > 明确标记的 Standard 假设。OpenAI 响应 tier 只接受 allow-list；缺失、冲突、多事件共用关联键或观察器被隔离时不把请求意图伪装成实际值。
- 响应正文不落盘；只存经过验证的 tier 标签、哈希关联键及冲突状态。没有高置信实际 tier 时，estimate source 会显示 requested 或 assumed-standard。
- 费率以 schedule ID 版本化，但成本不是每条事件的不可变账本金额：分析查询会使用当前版本重新计算历史事件。需要历史费率冻结时，应在后续增加生效时间表或事件级估价快照。
- 新模型/变体只有在核对官方 Standard/Fast、token 桶和上下文边界后才加入 schedule；未发布费率继续显示 `—`，不使用通用倍率。

## 6. 落地实现与验证

- **统一后端估价：** `internal/pricing` 提供版本化 schedule 和单一 estimator；事件 JSON、Analytics 总计及各分组共用结果。监控页移除重复 token 价格公式和模型价格请求，只展示后端 `cost_estimate`。
- **发布费率边界：** 官方表仅加入 GPT-5.4、GPT-5.5、GPT-5.6 Sol 已核验价格；按单请求输入 `>272,000` 切换 long-context，cached/read 重叠计数取较大值，cache-write 仅在官方公布费率时计价。未发布 Fast long、cache-write 和 `flex` 明确 unpriced。自定义平面价也只对重叠的 cached/read 计数计一次。实际响应为 Standard/default 时不会再标成“假设 Standard”。
- **Tier 证据：** 请求 `fast`/`priority` 只表示请求意图；通过关联的 OpenAI 响应读取 allow-list `service_tier`，可区分 actual、requested 和 assumed-standard。冲突或观察器隔离时不声称实际 tier；不持久化响应正文。
- **产品展示：** 页面以 API USD 等价估价展示 OAuth 与 API-key；OAuth 提示明确不代表 ChatGPT 订阅扣费或剩余额度。事件价格旁展示估价采用的实际/请求/假设档位，并附 `reasoning_effort` 和 reasoning token 说明：effort 本身没有独立价格倍率，reasoning token 按输出 token 单价计入。未核验模型使用平面单价时明确标示不按服务档位/上下文调整。各级汇总显示已定价/未定价覆盖；无有效估价显示 `—`，不误导为 `$0`。
- **历史语义：** event cost 在 Analytics 查询时以当前 schedule 计算，并带 schedule ID；未将 USD 金额快照到每个 usage event。监控页明确提示汇总按当前价格表重算，不是逐条冻结账单。
- **验证：** `cd plugins/cpa-manager-plus/go && go test ./...` 通过；`cd plugins/cpa-manager-plus/web && npm test` 通过（21 files / 155 tests）；`npm run build` 通过。

## 来源

### OpenAI 官方资料

- [OpenAI API Pricing — Standard](https://developers.openai.com/api/docs/pricing?latest-pricing=standard)
- [OpenAI API Pricing — Fast](https://developers.openai.com/api/docs/pricing?latest-pricing=fast)
- [OpenAI API Fast mode guide](https://developers.openai.com/api/docs/guides/fast-mode)
- [OpenAI API Reasoning models guide](https://developers.openai.com/api/docs/guides/reasoning) — reasoning tokens are billed as output tokens; effort itself is a generation setting, not a separate unit-price schedule.
- [Codex Speed](https://learn.chatgpt.com/codex/speed)
- [Codex Pricing](https://learn.chatgpt.com/codex/pricing)

### 仓库实现资料

- [价格 schedule 与单一 estimator](../../plugins/cpa-manager-plus/go/internal/pricing/pricing.go)
- [Analytics 价格适配、成本汇总与事件 payload](../../plugins/cpa-manager-plus/go/internal/store/pricing_estimate.go) · [analytics.go](../../plugins/cpa-manager-plus/go/internal/store/analytics.go)
- [OpenAI service_tier allow-list 解析](../../plugins/cpa-manager-plus/go/internal/responsemodel/service_tier.go)
- [响应 tracker](../../plugins/cpa-manager-plus/go/internal/responsemodel/tracker.go) · [tier 元数据存储/迁移](../../plugins/cpa-manager-plus/go/internal/store/response_metadata.go)
- [监控页 API 等价估价展示](../../plugins/cpa-manager-plus/web/src/components/MonitoringView.vue)
- [后端估价/响应 tier 测试](../../plugins/cpa-manager-plus/go/internal/pricing/pricing_test.go) · [前端回归测试](../../plugins/cpa-manager-plus/web/src/components/MonitoringView.test.js)

本方案已按统一 OpenAI API USD 口径落地。OAuth 页面金额仅为 API 等价估价，不代表实际订阅扣费或剩余额度。
