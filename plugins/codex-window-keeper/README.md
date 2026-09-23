# Codex Window Keeper

中文 | English（英文说明暂未提供）

CPA 插件：按每个 Codex OAuth 凭证实际返回的 usage 窗口跟踪额度状态；在插件曾观察到门控窗口耗尽、相关门控窗口都恢复可用后，向该凭证固定路由发送一条短 Responses 请求。

> 首次启用不会假设额度窗的激活规则。插件会先观测账号自身的 usage 响应；发送后再探测并分类为 anchored、fixed 或 inconclusive。只有实际观测到的可用额度变化才会作为证据，不把“成功回复”直接当作额度窗已重置。

## 能力与边界

- 每个账号独立识别实际返回的主窗口、额外限额与可选 Code Review 窗口；Free/Plus/Pro 等 plan 只用于账号自身窗口形状对照，不硬编码套餐与窗口组合。
- 使用 host auth-list/auth-get 识别 Codex OAuth、`auth_index`、账号 ID 和账号 plan；不会持久化凭证 JSON 或访问令牌。
- `forced_provider=codex` 与精确 `AuthID` 将测试消息固定到被观测的凭证，不允许路由到另一账号。消息通过 CPA host-model callback 执行并读取完整 SSE，只有收到 `response.completed` 且含输出文本才记为成功。
- 其他仍耗尽的门控窗口会阻止发送；达到重试上限、OAuth/配置错误暂停账号。修复后可在页面恢复账号。
- Usage 轮询、发送记录、窗口状态、账号覆盖设置和密钥密文保存在同一个 `keeper.sqlite` 中。使用 WAL 与持久事务；启动会清理过期 lease 并重新探测未完成 attempt。
- API 不提供上游幂等键。若 CPA 在上游接受消息后、但本地写入完成记录前被强制终止，插件会先重新探测再恢复；这种外部提交的不确定时刻无法在客户端完全消除极少数重复风险。
- 不会重置额度、不操作账号启停、不清理限额，也不承诺固定窗口会因发信而移动。被判定 fixed 的窗口不会反复触发。

## 安装与配置

需要带 CPA 插件 ABI/model-stream callback 的 CLIProxyAPI `v7.3.9+`，以及启用的插件管理功能。初次启动默认关闭，先检查地址和密钥，再由管理页面主动启用。

示例配置（确保 `data_dir` 位于持久化卷）：

~~~yaml
plugins:
  enabled: true
  configs:
    codex-window-keeper:
      data_dir: data/codex-window-keeper
      enabled: false
      model: gpt-5.4
      reasoning_effort: low
      prompt: Reply with exactly OK.
      include_additional: fill_gaps
      poll_interval_seconds: 20
      activation_skew_seconds: 3
~~~

在 CPA 管理中心打开插件菜单「额度窗口」：

1. 如果顶部提示缺少管理密钥，打开「调整」，填写 CPA 管理地址和具有管理 API 权限的密钥。插件通过 CPA `/v0/management/api-call` 读取 `auth_index` 对应的 Codex usage。
2. 先确认账号表、套餐对照及动态窗口正确，再打开全局开关。启用前未观测到的旧窗口不会补发消息。
3. 页面上的「重新读取额度」只探测；「按规则发一条」仍遵循全局开关、账号暂停状态与额度门控，不是绕过额度的强制发送。
4. 对认证/配置错误完成修复后，可点「恢复账号」。

默认管理地址为 `http://127.0.0.1:8317`。如果 CPA 管理端使用其他地址/端口，请在插件页调整。管理密钥使用 AES-GCM 加密写入数据库；`keeper.key` 是同一数据目录中的本地密钥材料，不是第二个数据库。请同时持久化并保护 `keeper.sqlite` 与 `keeper.key`，两者权限应限制为运行 CPA 的系统用户。

## 本地构建

~~~bash
cd plugins/codex-window-keeper
make test
make build
~~~

`make build` 生成当前系统的 c-shared 插件文件；启用 CGO。跨平台构建需要对应的 C 交叉编译器。自动发布版本 tag 使用 `codex-window-keeper-v<version>`。

## 管理接口

- 资源页：`GET /v0/resource/plugins/codex-window-keeper/app`
- 健康：`GET /v0/management/codex-window-keeper/health`
- 设置：`GET/PUT /v0/management/codex-window-keeper/settings`
- CPA 管理地址/密钥：`PUT /v0/management/codex-window-keeper/connection`
- 账号、覆盖设置、探测、启用及恢复：`/v0/management/codex-window-keeper/accounts...`
- 尝试记录：`GET /v0/management/codex-window-keeper/attempts`

本插件不创建替代服务器；管理 UI 和 API 由当前 CPA 进程提供。
