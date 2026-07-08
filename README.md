# CLIProxyAPI 插件商店

中文 | [English](README.en.md)

这是给 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 使用的第三方插件商店仓库。仓库同时维护兼容老版本 CPA 的 schema v1 registry，以及推荐给新版 CPA 的 schema v2 direct-install registry，并把 registry 与 release 资产镜像到 jsDelivr CDN。

## 可用插件

| 插件 | 说明 |
|------|------|
| [developer-role-normalizer](plugins/developer-role-normalizer/) | 将不兼容上游里的 `developer` 消息角色转换为 `system`，主要面向 DeepSeek 等 OpenAI-compatible provider。 |
| [cpa-manager-plus](plugins/cpa-manager-plus/) | 在 CPA 管理端提供 Manager Plus 风格的仪表盘、用量分析、请求监控、账号巡检与配置页，并反向代理到 Manager Server。 |

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

### 老版本 CPA：schema v1 兼容入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

schema v1 仍然保留给老 CPA 使用。注意：v1 使用 GitHub `releases/latest` 模型，多个插件共用一个仓库时会受到 latest release 的限制。

## CPA 版本建议

| CPA 版本 | 推荐 registry | 说明 |
|----------|---------------|------|
| `< v7.2.44` | `registry.json` | 这些版本早于 schema v2 direct install。 |
| `v7.2.44` - `v7.2.45` | `registry.json`，或谨慎测试 `registry-v2.json` | 已有 direct install 初版，但缺少后续下载/auth/错误处理修复。 |
| `>= v7.2.46` | `registry-v2.json`，优先 CDN URL | 推荐路径。包含 direct install 及后续 plugin-store 修复。 |

证据：CPA upstream commit `1f16e87` 从 `v7.2.44` 起包含 direct install；`3ea7f18`、`8970873`、`caf7052` 从 `v7.2.46` 起包含后续 plugin-store 修复。

## 架构

```text
CPA
 └─ plugin store registry
     ├─ registry.json       schema v1，兼容老 CPA，GitHub Release latest 模型
     └─ registry-v2.json    schema v2，新 CPA 推荐，direct install 模型

GitHub Actions
 ├─ 按 tag 发现版本匹配的插件
 ├─ 构建 linux/darwin/windows × amd64/arm64 动态库 zip
 ├─ 发布 GitHub Release
 ├─ 刷新 main 分支 registry-v2.json（GitHub Release URL）
 └─ 发布 cdn 分支
     ├─ registry.json
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
  - `registry.json`
  - `registry-v2.json`
  - `latest/checksums.txt`
  - `latest/*.zip`

手动刷新示例：

```text
https://purge.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

## 发布流程

发布使用标准 semver tag，例如 `v0.3.8`。不要使用插件后缀 tag，例如 `v0.3.8-cpa-manager-plus`。

1. 修改插件代码。
2. 选择新版本号，例如 `0.3.9`。
3. 同步版本号：
   - `plugins/<plugin-id>/go/main.go` → `var pluginVersion = "0.3.9"`
   - `registry.json` → 对应插件的 `"version"`
   - `plugins/<plugin-id>/Makefile` → `VERSION := 0.3.9`（如存在）
4. commit 并 push 到 `main`。
5. 创建并推送 tag：

```bash
git tag -a v0.3.9 -m "v0.3.9"
git push origin v0.3.9
```

6. GitHub Actions 自动执行：
   - 只构建源码版本等于 tag 版本的插件。
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
for f in ['registry.json', 'registry-v2.json']:
    j = json.load(open(f))
    print(f, j['schema_version'], len(j['plugins']))
PY
```

## 详细文档

- [registry / CDN 分发策略](docs/registry-schema-strategy.md)
- [Registry / CDN distribution strategy](docs/registry-schema-strategy.en.md)
- [developer-role-normalizer](plugins/developer-role-normalizer/)
- [cpa-manager-plus](plugins/cpa-manager-plus/)

## License

MIT
