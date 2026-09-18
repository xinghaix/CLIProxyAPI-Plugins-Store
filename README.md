# CLIProxyAPI 插件商店

中文 | [English](README.en.md)

这是给 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 使用的第三方插件商店仓库，要求 CPA `v7.2.46+`。仓库只发布 schema v2 direct-install registry（`registry-v2.json`），并把 registry 与 release 资产镜像到 jsDelivr CDN。**从 2026-09-18 起不再兼容 schema v1。** 每个插件拥有独立的版本线，使用插件级 tag 发布。

## 可用插件

| 插件 | 说明 |
|------|------|
| [developer-role-normalizer](plugins/developer-role-normalizer/) | 将不兼容上游里的 `developer` 消息角色转换为 `system`，主要面向 DeepSeek 等 OpenAI-compatible provider。 |
| [cpa-manager-plus](plugins/cpa-manager-plus/) | 在 CPA 管理端提供 Manager Plus 风格的仪表盘、用量分析、请求监控、账号巡检与配置页，并反向代理到 Manager Server。 |
| [codex-oauth-base-url](plugins/codex-oauth-base-url/) | 改写 Codex OAuth（ChatGPT 订阅账号）凭据的上游 base URL，让 Codex 账号指向自建或第三方上游，无需修改 CPA 源码。 |

## 推荐安装入口

### CPA v7.2.46+：推荐 CDN + schema v2

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json"
```

这是当前主推入口：

- registry 本身走 jsDelivr CDN。
- `registry-v2.json` 使用 `install.type = direct`。
- 每个插件固定到自己的版本和平台资产 URL，不再依赖 GitHub `releases/latest`。
- CDN 分支里的 v2 registry 会把插件 zip 指向 jsDelivr CDN 资产 URL。

### CPA v7.2.46+：GitHub raw 备用入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

这个入口同样是 schema v2 direct install，但 artifact URL 指向 GitHub Release。适合排查 CDN 问题。

### 已停止支持：schema v1

schema v1 入口（`registry.json`）已下线：本仓库既不发布也不再维护 v1，`cdn` 分支上的 `registry.json` 会在下一次发布时移除并 purge。

仍在使用 v1 的 CPA 必须先升级到 `v7.2.46+`，然后改用上面的 v2 入口。

## CPA 版本建议

| CPA 版本 | 支持 | 说明 |
|----------|------|------|
| `< v7.2.46` | 不支持 | 缺少 direct install 或后续 plugin-store 修复。schema v1 已下线，请先升级 CPA。 |
| `>= v7.2.46` | 支持 | 使用 `registry-v2.json`，优先 CDN URL。包含 direct install 及后续 plugin-store 修复。 |

证据：CPA upstream commit `1f16e87` 从 `v7.2.44` 起包含 direct install；`3ea7f18`、`8970873`、`caf7052` 从 `v7.2.46` 起包含后续 plugin-store 修复。

## 架构

```text
CPA
 └─ plugin store registry
     └─ registry-v2.json    schema v2 direct install（唯一发布的 registry）

plugins.json               仓库源清单，仅用于生成 registry-v2.json

GitHub Actions
 ├─ 按 tag 发现目标插件（插件级 tag 对应单个插件）
 ├─ 构建 linux/darwin/windows × amd64/arm64 动态库 zip
 ├─ 发布 GitHub Release
 ├─ 刷新 main 分支 registry-v2.json（GitHub Release URL）
 └─ 发布 cdn 分支
     ├─ registry-v2.json（jsDelivr artifact URL）
     ├─ latest/
     └─ vX.Y.Z/

jsDelivr
 └─ https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/...
```

## CDN URL 规则

GitHub raw 文件 URL：

```text
https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}
```

可转换为 jsDelivr：

```text
https://cdn.jsdelivr.net/gh/{owner}/{repo}@{branch}/{path}
```

本仓库不建议用 `@main` 作为主要 CDN 入口；推荐使用专门生成的 `cdn` 分支：

```text
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/v0.3.8/cpa-manager-plus_0.3.8_linux_amd64.zip
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/latest/checksums.txt
```

原因：`cdn` 分支只放分发产物和生成后的 registry，结构稳定，适合被 jsDelivr 缓存。

## jsDelivr 缓存与刷新

- 版本路径如 `@cdn/v0.3.8/...` 应视为不可变路径，适合生产使用。
- `@cdn/latest/...` 和 `@cdn/registry-v2.json` 是可变路径，可能存在 CDN 缓存传播延迟。
- workflow 会自动 purge 这些可变路径：
  - `registry-v2.json`
  - `latest/checksums.txt`
  - `latest/*.zip`
  - `registry.json`（仅用于把已下线的 v1 文件清出 jsDelivr 缓存）

手动刷新示例：

```text
https://purge.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

## 发布流程

发布使用标准 semver tag，支持两种格式：

| tag 格式 | 行为 |
|----------|------|
| `v<version>` | 发布列车：构建所有源码版本等于该 tag 版本的插件。 |
| `<plugin-id>-v<version>` | 插件级发布：只构建该插件，版本号即插件自己的版本。 |

**插件级 tag 是本仓库的标准做法**：每个插件都有自己的版本号列表，互不复用。即使某个版本号已被别的插件占用（例如 `v0.1.0` 属于 developer-role-normalizer），新插件仍可以从 `0.1.0` 开始。

发布列车 tag 只在确实需要一次发布多个同版本插件时使用。不要把插件名放在版本号后面（例如 `v0.3.8-cpa-manager-plus`），那种形式 CI 不会识别。

既有插件（cpa-manager-plus、developer-role-normalizer）保留各自当前版本号，历史 release 继续可用，从下一个版本起也改用插件级 tag。

1. 修改插件代码。
2. 选择新版本号，例如 `0.3.9`。
3. 同步版本号：
   - `plugins/<plugin-id>/go/main.go` → `var pluginVersion = "0.3.9"`
   - `plugins.json` → 对应插件的 `"version"`
   - `plugins/<plugin-id>/Makefile` → `VERSION := 0.3.9`（如存在）
4. commit 并 push 到 `main`。
5. 创建并推送 tag：

插件级 tag（标准做法，版本号只属于该插件）：

```bash
git tag -a cpa-manager-plus-v0.3.9 -m "cpa-manager-plus 0.3.9"
git push origin cpa-manager-plus-v0.3.9
```

发布列车 tag（仅在需要一次发布多个同版本插件时使用）：

```bash
git tag -a v0.3.9 -m "v0.3.9"
git push origin v0.3.9
```

6. GitHub Actions 自动执行：
   - 只构建与该 tag 匹配的插件（插件级 tag 对应单个插件，发布列车 tag 对应所有版本相同的插件）。
   - 每个插件生成 6 平台 zip。
   - 发布 GitHub Release。
   - 生成 main 分支 `registry-v2.json`。
   - 发布/刷新 `cdn` 分支。
   - purge jsDelivr 可变路径。

## artifact 规范

每个发布版本必须包含 6 个平台：

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

Release asset 名称：

```text
{plugin-id}_{version}_{goos}_{goarch}.zip
```

zip 根目录必须包含动态库：

```text
{plugin-id}-v{version}.{so|dylib|dll}
```

示例：

```text
cpa-manager-plus_0.3.8_linux_amd64.zip
└── cpa-manager-plus-v0.3.8.so
```

## 本地验证

```bash
python3 -m py_compile scripts/generate-registry-v2.py
scripts/generate-registry-v2.py --check
python3 - <<'PY'
import json
manifest = json.load(open('plugins.json'))
print('plugins.json', [(p['id'], p['version']) for p in manifest['plugins']])
registry = json.load(open('registry-v2.json'))
print('registry-v2.json', registry['schema_version'], len(registry['plugins']))
PY
```

## 详细文档

- [registry / CDN 分发策略](docs/registry-schema-strategy.md)
- [Registry / CDN distribution strategy](docs/registry-schema-strategy.en.md)
- [developer-role-normalizer](plugins/developer-role-normalizer/)
- [cpa-manager-plus](plugins/cpa-manager-plus/)
- [codex-oauth-base-url](plugins/codex-oauth-base-url/)

## License

MIT
