# developer-role-normalizer

中文 | [English](README.en.md)

`developer-role-normalizer` 是一个 CPA request normalizer 插件，用于兼容不支持 `developer` 消息角色的 OpenAI-compatible 上游。插件会在 CPA 转发请求前，把命中的 `messages[*].role == "developer"` 改写为 `system`。

默认策略是保守的：只处理目标格式为 `openai` / `codex`，且模型名包含 `deepseek` 的请求。

## 解决的问题

部分 OpenAI-compatible 服务虽然暴露 Chat Completions 风格接口，但不接受新角色 `developer`。例如某些 DeepSeek-compatible 目标只接受：

```text
system / user / assistant / tool
```

如果请求中包含：

```json
{"role":"developer","content":"Follow these rules..."}
```

上游可能返回 400 或类似校验错误。本插件只在指定目标上做最小兼容改写，不影响原生支持 `developer` 的模型。

## 架构

```text
client request
    │
    ▼
CPA protocol translation / routing
    │
    ▼
request.normalize hook
    │
    ├─ 读取插件配置
    ├─ 检查目标格式：默认 openai/codex
    ├─ 检查模型名：默认包含 deepseek
    ├─ 检查策略：role_to_system
    │
    ▼
rewrite messages[*].role == "developer" to "system"
    │
    ▼
upstream OpenAI-compatible provider
```

## 运行流程

1. CPA 加载动态库并调用 `plugin.register`。
2. 插件声明 `request_normalizer` capability。
3. CPA 配置变化时调用 `plugin.reconfigure`，传入 `plugins.configs.developer-role-normalizer`。
4. 每次请求规范化时，CPA 传入目标格式、模型名和请求 body。
5. 插件按配置检查目标格式、模型名、启用状态和策略。
6. 命中后只改写顶层 `messages` 数组里的精确角色值 `developer`。
7. 未命中或 body 不符合预期时，原样返回。

## 配置

### 最小配置

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
```

默认行为等价于：

```yaml
normalize_enabled: true
target_formats:
  - openai
  - codex
model_match:
  mode: contains
  include:
    - deepseek
  exclude: []
strategy: role_to_system
```

### 完整配置示例

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
      normalize_enabled: true
      target_formats:
        - openai
        - codex
      model_match:
        mode: contains
        include:
          - deepseek
          - qwen
        exclude:
          - deepseek-official-compatible
      strategy: role_to_system
```

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | boolean | CPA 控制 | CPA 插件启用开关。 |
| `normalize_enabled` | boolean | `true` | 插件逻辑开关；设为 `false` 时不改写。 |
| `target_formats` | array/string | `[openai, codex]` | 允许处理的目标协议格式。 |
| `model_match.mode` | enum | `contains` | 支持 `contains`、`exact`、`prefix`、`suffix`。 |
| `model_match.include` | array/string | `[deepseek]` | 模型必须命中至少一个 include；空数组表示全部包含。 |
| `model_match.exclude` | array/string | `[]` | 排除规则，优先级高于 include。 |
| `strategy` | enum | `role_to_system` | 当前只支持 `role_to_system`。 |

数组字段也支持逗号分隔字符串，例如：

```yaml
target_formats: openai,codex
model_match:
  include: deepseek,qwen
  exclude: deepseek-official-compatible
```

## 行为示例

输入：

```json
{
  "model": "deepseek-chat",
  "messages": [
    {"role": "developer", "content": "You are concise."},
    {"role": "user", "content": "Hello"}
  ]
}
```

输出：

```json
{
  "model": "deepseek-chat",
  "messages": [
    {"role": "system", "content": "You are concise."},
    {"role": "user", "content": "Hello"}
  ]
}
```

如果模型不匹配，例如 `gpt-4.1`，请求会原样返回。

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
curl -X POST http://localhost:8317/v0/management/plugin-store/developer-role-normalizer/install \
  -H "Authorization: Bearer ***"
```

启用：

```yaml
plugins:
  enabled: true
  configs:
    developer-role-normalizer:
      enabled: true
      priority: 1
```

## 构建与验证

```bash
cd plugins/developer-role-normalizer/go
go test ./...
go vet ./...
CGO_ENABLED=1 go build -buildmode=c-shared -o developer-role-normalizer.dylib .
nm developer-role-normalizer.dylib | grep cliproxy_plugin_init
```

插件必须使用 `CGO_ENABLED=1` 编译，因为它通过 cgo 导出 CPA C ABI。

## 手动安装

将动态库放入 CPA 插件目录，例如：

```text
plugins/linux/amd64/developer-role-normalizer-v0.3.8.so
```

安装或升级后需要重启 CPA 进程，已加载的动态库不会热替换。

## 兼容性说明

- 默认只处理 `openai` / `codex` 目标格式和模型名包含 `deepseek` 的请求。
- 模型匹配大小写不敏感。
- 插件优先使用 transform request 的 `Model` 字段，缺失时回退到 `body.model`。
- 插件不合并消息内容，只替换顶层 `messages` 数组中的角色字段。
- 没有有效顶层 `messages` 数组的请求原样返回。

## License

MIT
