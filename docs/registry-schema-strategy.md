# Registry / CDN 分发策略

中文 | [English](registry-schema-strategy.en.md)

本仓库只发布一个 registry：schema v2 direct install 的 `registry-v2.json`，并通过 `cdn` 分支接入 jsDelivr。**自 2026-09-18 起不再兼容 schema v1**，`registry.json` 不再发布、不再维护。

仓库内有两个 JSON，职责不同：

- `plugins.json` — 源清单，只保存插件元数据与版本号，不对外分发。
- `registry-v2.json` — 唯一发布的 registry，由 `scripts/generate-registry-v2.py` 从 `plugins.json` 生成。

## CPA 版本要求

| CPA 版本 | 支持 | 原因 |
|----------|------|------|
| `>= v7.2.46` | 支持 | 包含 direct install 与后续 plugin-store 修复。 |
| `< v7.2.46` | 不支持 | 缺少 direct install 或后续 plugin-store 修复；schema v1 已下线，请先升级 CPA。 |

依据：

- `1f16e87`（`feat(pluginstore): introduce support for direct install type and version management`）从 `v7.2.44` 起包含。
- 后续 plugin-store 修复 `3ea7f18`、`8970873`、`caf7052` 从 `v7.2.46` 起包含。

因此最低支持版本为 `v7.2.46`，推荐直接使用 CDN 版 `registry-v2.json`。

## 用户配置入口

### 推荐：CDN 入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json"
```

### 备用：GitHub raw 入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

## 为什么用 direct install + CDN

schema v2 direct install 的工作方式：

1. 每个插件声明自己的 `version`。
2. 每个插件声明各平台 artifact URL。
3. 每个 artifact 内联 `sha256` 与 `size`。
4. CPA 只下载匹配当前平台的 artifact，并校验 sha256。

这样每个插件拥有独立版本线：插件 A 可以升到 `1.2.0`，插件 B 继续固定在 `1.1.0`，互不影响。

旧版 schema v1 依赖 GitHub `releases/latest`，而 `latest` 只指向最近一次发布的 release；本仓库每次发布只包含被 tag 的那一个插件，因此其余插件经 v1 安装必然失败。这正是 v1 被移除的原因。

CDN 额外解决两点：

- registry 与插件 zip 走 jsDelivr，降低 GitHub raw / release 下载失败概率。
- `cdn` 分支是干净、稳定、只面向分发的文件树。

## 插件版本线

每个插件拥有独立的版本号序列，版本号不在插件之间复用。发布只使用插件级 tag `<plugin-id>-v<version>`，workflow 只构建该 tag 指定的那一个插件。

即使某个版本号已被别的插件占用（例如 `v0.1.0` 属于 developer-role-normalizer），新插件仍可以从 `0.1.0` 开始。

全局 `v<version>` 发布列车已于 2026-09-18 停用，推送这种 tag 会被拒绝。历史 release（`v0.3.8`、`v0.5.28` 等）仍挂在原来的全局 tag 下，因此生成脚本在解析这些既有版本时保留了向 `v<version>` 回落的逻辑；新版本一律使用插件级 tag。

## CDN 分支结构

`cdn` 分支由 GitHub Actions 自动生成，不要手工修改：

```text
cdn branch
├── README.md
├── registry-v2.json
├── latest/
│   ├── checksums.txt
│   ├── codex-oauth-base-url_0.1.0_linux_amd64.zip
│   └── ...
└── codex-oauth-base-url-v0.1.0/
    ├── checksums.txt
    ├── codex-oauth-base-url_0.1.0_linux_amd64.zip
    └── ...
```

CDN registry：

```text
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

版本固定资产（不可变）：

```text
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/codex-oauth-base-url-v0.1.0/codex-oauth-base-url_0.1.0_linux_amd64.zip
```

## CDN 缓存策略

- `@cdn/<plugin-id>-vX.Y.Z/...`：版本路径，不应修改，适合生产和复现。
- `@cdn/latest/...`：可变路径，方便人工下载最新包，可能有缓存传播延迟。
- `@cdn/registry-v2.json`：可变 registry，workflow 会自动 purge。

手动 purge：

```text
https://purge.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

workflow 会 purge：

- `registry-v2.json`
- `latest/checksums.txt`
- `latest/*.zip`
- `registry.json`（仅用于把已下线的 v1 文件清出 jsDelivr 缓存，不再是产物）

## 文件职责

### `plugins.json`（源清单）

用途：

- 维护插件元数据（id、name、description、author、logo、homepage、license、tags）。
- 维护每个插件当前的 `version`。
- 作为 `registry-v2.json` 的唯一生成输入。

不对外分发，CPA 不读取它。

### main 分支 `registry-v2.json`

用途：

- schema v2 direct install。
- artifact URL 指向 GitHub Release。
- 作为 CDN 生成输入和 GitHub raw 备用入口。

生成：

```bash
scripts/generate-registry-v2.py
```

检查：

```bash
scripts/generate-registry-v2.py --check
```

### cdn 分支 `registry-v2.json`

用途：

- schema v2 direct install。
- artifact URL 指向 jsDelivr CDN。
- CPA 推荐使用的入口。

生成命令示例：

```bash
scripts/generate-registry-v2.py \
  --output /tmp/registry-v2-cdn.json \
  --artifact-url-template "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/{tag}/{asset_name}"
```

## 发布流程

1. 修改插件代码。
2. 选择版本号，例如 `0.3.9`。
3. 同步版本：
   - `plugins/<id>/go/main.go` → `var pluginVersion = "0.3.9"`
   - `plugins.json` → 对应插件的 `"version"`
   - `plugins/<id>/Makefile` → `VERSION := 0.3.9`（如存在）
4. commit 并 push 到 `main`。
5. 推送插件级 tag：

```bash
git tag -a <plugin-id>-v0.3.9 -m "<plugin-id> 0.3.9"
git push origin <plugin-id>-v0.3.9
```

6. workflow 自动：
   - 发现该 tag 对应的插件（插件级 tag 对应单个插件）。
   - 构建 6 平台 zip。
   - 发布 GitHub Release。
   - 刷新 main 分支 `registry-v2.json`。
   - 发布/刷新 `cdn` 分支（并移除残留的 `registry.json`）。
   - purge jsDelivr 可变路径。

## artifact 要求

每个发布版本必须包含 6 平台：

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

zip 根目录动态库名称：

```text
{plugin-id}-v{version}.{so|dylib|dll}
```

## 新增插件流程

1. 在 `plugins/<plugin-id>/go/` 添加插件源码。
2. 在 `plugins.json` 添加插件元数据和版本。
3. 推送插件级 tag，例如 `<plugin-id>-v0.1.0`，让新插件从 `0.1.0` 开始自己的版本线。
4. 等 CI 发布 release 和 cdn 分支。
5. 验证 CDN registry 中有新插件的 `install.artifacts`。

## 排障

如果插件安装提示 502：

1. 确认 CPA `>= v7.2.46`；更低版本不受支持，请先升级。
2. 优先切换到 CDN 版 `registry-v2.json`。
3. 检查 artifact URL 是否能从 CPA 运行环境访问；Docker 内要单独验证。
4. 安装/升级动态库后重启 CPA 进程，因为已加载的 dylib/so 不会热替换。
