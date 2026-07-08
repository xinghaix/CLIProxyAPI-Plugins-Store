# Registry / CDN 分发策略

中文 | [English](registry-schema-strategy.en.md)

本仓库同时发布两个 registry 入口，并通过 `cdn` 分支接入 jsDelivr：

- `registry.json` — schema v1，兼容老 CPA。
- `registry-v2.json` — schema v2 direct install，新 CPA 推荐。
- `cdn` 分支 — 镜像 registry 与 release zip，用于 jsDelivr CDN。

长期维护策略：新版 CPA 默认使用 schema v2 direct install + jsDelivr CDN；schema v1 只作为老 CPA 兼容入口保留。

## CPA 版本建议

| CPA 版本 | 推荐 registry | 原因 |
|----------|---------------|------|
| `< v7.2.44` | `registry.json` | 早于 schema v2 direct install 支持。 |
| `v7.2.44` - `v7.2.45` | `registry.json`，或谨慎测试 `registry-v2.json` | direct install 初版已存在，但缺少后续 plugin-store 下载/auth/错误处理修复。 |
| `>= v7.2.46` | `registry-v2.json`，优先 CDN URL | 推荐。包含 direct install 与后续 plugin-store 修复。 |

依据：

- `1f16e87`（`feat(pluginstore): introduce support for direct install type and version management`）从 `v7.2.44` 起包含。
- `3ea7f18`、`8970873`、`caf7052` 等后续 plugin-store 修复从 `v7.2.46` 起包含。

对用户的默认建议：**CPA v7.2.46+ 使用 CDN 版 `registry-v2.json`；更老版本使用 `registry.json`。**

## 用户配置入口

### 推荐：CPA v7.2.46+ CDN 入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json"
```

### 备用：CPA v7.2.46+ GitHub raw 入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry-v2.json"
```

### 兼容：老 CPA schema v1 入口

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/xinghaix/CLIProxyAPI-Plugins-Store/main/registry.json"
```

## 为什么需要 v2 + CDN

schema v1 使用 GitHub Release `latest` 模型：

1. CPA 从 `registry.json` 读取插件。
2. CPA 根据插件 `repository` 调 GitHub `GET /repos/{owner}/{repo}/releases/latest`。
3. CPA 从 latest tag 推导版本号。
4. CPA 从同一个 latest release 下载 `{plugin-id}_{version}_{goos}_{goarch}.zip` 和 `checksums.txt`。

这对多插件仓库不理想：如果 latest release 只包含插件 A，老 CPA 安装插件 B 会因为 zip 不存在而失败。要让 v1 完美兼容，每次 latest release 都要重打所有插件。

schema v2 direct install 解决这个问题：

1. 每个插件声明自己的 `version`。
2. 每个插件声明各平台 artifact URL。
3. 每个 artifact 内联 `sha256` 和 `size`。
4. CPA 直接下载匹配平台的 artifact 并校验 sha256。

这样插件 A 可以升级到 `1.2.0`，插件 B 继续固定在 `1.1.0`，互不影响。

CDN 进一步解决两个问题：

- registry 文件和插件 zip 走 jsDelivr，降低 GitHub raw/release 下载失败概率。
- `cdn` 分支提供稳定、干净、只面向分发的文件树。

## CDN 分支结构

`cdn` 分支由 GitHub Actions 自动生成，不手工维护：

```text
cdn branch
├── README.md
├── registry.json
├── registry-v2.json
├── latest/
│   ├── checksums.txt
│   ├── cpa-manager-plus_0.3.8_linux_amd64.zip
│   └── ...
└── v0.3.8/
    ├── checksums.txt
    ├── cpa-manager-plus_0.3.8_linux_amd64.zip
    └── ...
```

CDN registry：

```text
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

版本固定资产：

```text
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/v0.3.8/cpa-manager-plus_0.3.8_linux_amd64.zip
```

latest 资产：

```text
https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/latest/checksums.txt
```

## CDN 缓存策略

- `@cdn/vX.Y.Z/...`：版本路径，不应修改，适合生产和复现。
- `@cdn/latest/...`：可变路径，方便人工下载最新包，但可能有缓存传播延迟。
- `@cdn/registry-v2.json`：可变 registry，workflow 会自动 purge。

手动 purge：

```text
https://purge.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json
```

workflow 会 purge：

- `registry.json`
- `registry-v2.json`
- `latest/checksums.txt`
- `latest/*.zip`

## registry 文件职责

### `registry.json`

用途：

- 老 CPA 兼容。
- schema v1。
- 保留 GitHub `repository` 字段。

限制：

- 多插件共用同一个 GitHub repository 时依赖同一个 `releases/latest`。
- latest release 不包含目标插件 zip 时安装会失败。
- 如果必须完整兼容老 CPA，latest release 需要包含所有插件 zip。

### main 分支 `registry-v2.json`

用途：

- schema v2 direct install。
- artifact URL 指向 GitHub Release。
- 用作 CDN 生成输入和 GitHub raw 备用入口。

生成命令：

```bash
scripts/generate-registry-v2.py
```

检查命令：

```bash
scripts/generate-registry-v2.py --check
```

### cdn 分支 `registry-v2.json`

用途：

- schema v2 direct install。
- artifact URL 指向 jsDelivr CDN。
- CPA v7.2.46+ 推荐使用这个文件。

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
   - `registry.json` → `"version": "0.3.9"`
   - `plugins/<id>/Makefile` → `VERSION := 0.3.9`（如存在）
4. commit 并 push 到 `main`。
5. 推送标准 semver tag：

```bash
git tag -a v0.3.9 -m "v0.3.9"
git push origin v0.3.9
```

6. workflow 自动：
   - 发现版本匹配的插件。
   - 构建 6 平台 zip。
   - 发布 GitHub Release。
   - 刷新 main 分支 `registry-v2.json`。
   - 发布/刷新 `cdn` 分支。
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
2. 在 `registry.json` 添加插件元数据和版本。
3. 推送匹配版本的 tag。
4. 等 CI 发布 release 和 cdn 分支。
5. 验证 CDN registry 中有新插件的 `install.artifacts`。

## 排障

如果插件安装提示 502：

1. CPA `>= v7.2.46`：优先切换到 CDN 版 `registry-v2.json`。
2. CPA `< v7.2.44`：升级 CPA，或继续使用 `registry.json`。
3. schema v1：检查当前 latest release 是否包含目标插件 zip。
4. schema v2：检查 artifact URL 是否能从 CPA 运行环境访问；Docker 内要单独验证。
5. 安装/升级动态库后重启 CPA 进程，因为已加载的 dylib/so 不会热替换。
