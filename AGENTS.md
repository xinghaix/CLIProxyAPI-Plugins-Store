# AGENTS.md

## 1. 多插件仓库契约 (Monorepo & Registry)
- **目录隔离**：每个插件位于 `plugins/<plugin-id>/`（`go/` 后端、可选 `web/` 前端、`assets/` 图标），拥有独立 Go Module 与独立版本线，发布 Tag 格式为 `<plugin-id>-v<version>`。
- **注册表单一事实源**：仅手工维护 [`plugins.json`](./plugins.json)。[`registry-v2.json`](./registry-v2.json) 与 `cdn` 分支均由 [`.github/workflows/build-release.yml`](./.github/workflows/build-release.yml) 在 6 平台产物构建完成后调用 [`scripts/generate-registry-v2.py`](./scripts/generate-registry-v2.py) 自动生成合入，禁止手动修改 `registry-v2.json`。
- **测试执行**：Go 测试统一带 `GOCACHE=/tmp/go-build go test ./...`（规避沙盒宿主缓存只读限制）；含 `web/` 的插件需先通过 `npm test && npm run build` 生成嵌入产物。

## 2. 版本升级同步矩阵 (Version Bump Matrix)
升级任意插件版本时，在单个 `release(<plugin-id>): bump to v<version>` 提交中同步修改公共项与插件专属项，并用 `git grep "<old-version>" plugins/<plugin-id> plugins.json` 确认无残留（历史 Release 文档除外）：

| 插件 ID | 公共必改项 | 插件专属必改项 |
| :--- | :--- | :--- |
| `developer-role-normalizer` | `plugins.json`、`go/main.go` (`pluginVersion`) | 无 |
| `codex-oauth-base-url` | `plugins.json`、`go/main.go` (`pluginVersion`) | 无 |
| `cursor-oauth` | `plugins.json`、`go/main.go` (`pluginVersion`) | `Makefile` (`VERSION`)、`go/internal/plugin/handler.go` (`PluginVersion`)、`go/internal/plugin/handler_test.go`、`packaging/build-release.sh`、`packaging/build-store-release.sh`、`README.md`、`README.en.md` |
| `cpa-manager-plus` | `plugins.json`、`go/main.go` (`pluginVersion`) | `Makefile` (`VERSION`)、`go/main_test.go`、`go/internal/app/runtime.go` (`runtimeVersion`)、`go/internal/app/runtime_test.go`、`go/internal/pricesync/sync.go` (`User-Agent`)、`web/package.json`、`web/package-lock.json` (顶层与 `packages[""]`) |

## 3. CPA 宿主与管理台鉴权契约 (Host & Auth Gotchas)
- **宿主边界**：CPA 管理台通过 `<iframe src="/v0/resource/plugins/...">` 加载插件资源页，**不会**通过 URL、Header 或 `postMessage` 注入 `managementKey`；插件管理 API（`/v0/management/...`）由宿主统一校验 `Authorization: Bearer <key>` 或 `X-Management-Key: <key>`。
- **`cli-proxy-auth` 混淆协议（`enc::v2::` & `enc::v1::`）**：
  - `Cli-Proxy-API-Management-Center`（`1650cf2f`）将 `localStorage['cli-proxy-auth']` 升级为 `enc::v2::` 并移除 `navigator.userAgent` 盐值：
    - `enc::v2::` 密钥：`cli-proxy-api-webui::secure-storage|${window.location.host}`
    - `enc::v1::` 密钥：`cli-proxy-api-webui::secure-storage|${window.location.host}|${navigator.userAgent}`
  - 复用管理台登录态的插件前端（如 [`mcAuthStorage.js`](./plugins/cpa-manager-plus/web/src/utils/mcAuthStorage.js)、[`data.js`](./plugins/cpa-manager-plus/web/src/utils/data.js)）必须同时支持 `enc::v2::` 与 `enc::v1::` 动态解混淆，并用 `isObfuscated()` 拦截父窗口状态（`__CPA_AUTH_STORE__` / `cpaAuthStore`）中未解混淆的密文。

## 4. 发布流程与 Release 说明规范 (Release Standard)
1. **区间盘点**：执行 `git log <plugin-id>-v<prev>..HEAD -- plugins/<plugin-id>` 汇总距上次发版 Tag 的**全部**插件提交。
2. **推送与创建**：推送 `main` 与 `<plugin-id>-v<version>` Tag 后，**必须**用单引号 heredoc（`--notes-file - <<'EOF'`）执行 `gh release create` / `edit`，防止 Bash 将 Markdown 反引号当作命令替换执行；最后用 `gh release view <tag> --json body` 校验。
3. **标准模板**：
   ```markdown
   ## <Plugin Name> v<version>

   ### ✨ 功能与体验优化 (Features & UX)
   - **<模块/特性> (`<short-sha>`)**：<面向用户的变更与交互效果>

   ### 🐛 问题修复 (Bug Fixes)
   - **<问题简述> (`<short-sha>`)**：<根因与修复结果>

   ### 📦 构建产物 (Assets)
   提供 `linux` / `darwin` / `windows` 的 `amd64` 与 `arm64` 共 6 平台预编译包及 `checksums.txt`（由 GitHub Actions 自动上传）。

   **Full Changelog**: https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/compare/<plugin-id>-v<prev>...<plugin-id>-v<version>
   ```

## 5. CDN 刷新与插件市场排障 (CDN & Store Diagnostics)
- **jsDelivr Purge 生效判据**：请求 `https://purge.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json`（及 `@main/registry-v2.json`、`@cdn/latest/*`）后，必须确认响应 `"throttled": false` 且 `"providers": {"CF": true, "FY": true}`（若 `"throttled": true` 则边缘缓存未清除，需等待 `throttlingReset` 秒后重试），并在 `cdn` / `fastly` / `gcore` / `testingcf.jsdelivr.net` 实测回源版本已更新。
- **CPA 插件市场不显示更新的 3 个根因**：
  1. **卡片展示逻辑**：管理台 `PluginStorePage.tsx` 在 `installed && !updateAvailable` 时仅渲染本地 `installedVersion`，不显示远端 `version`。
  2. **缺少 `@cdn` 分支限定**：不带 `@ref` 的 `.../CLIProxyAPI-Plugins-Store/registry-v2.json` 会被 jsDelivr 路由到旧纯数字 Tag（停留在 `0.5.28`），必须配置 `@cdn/registry-v2.json`。
  3. **源绑定校验或旧库抢占**：CPA 校验 `config.yaml` 中 `store.source_url` 与当前商店源一致才允许更新（不一致需卸载重装）；手动替换 `<plugins_dir>/<goos>/<goarch>/` 动态库时必须删除旧版本文件（同目录多版本按字典序优先命中旧版）。
