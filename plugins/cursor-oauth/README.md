# cursor-oauth

中文 | [English](README.en.md)

`cursor-oauth` 是 [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) 的官方插件商店原生动态库插件，把用户本人授权的 **Cursor** 订阅账号接入 OpenAI 兼容的 `/v1/chat/completions` 接口。

本插件基于社区 [kilolonion/cursor-cpa-plugin](https://github.com/kilolonion/cursor-cpa-plugin) 与 [yobo2u/omsub](https://github.com/yobo2u/omsub) 迁移并深度适配 CLIProxyAPI Plugins Store 的多端构建与发布体系，提供**思考流透传**、**纯文本约束注入**、**官方品牌图标**以及**会话检查点复用**等核心能力。

> [!IMPORTANT]
> 本项目是非官方社区插件，与 Cursor、Anysphere、CLIProxyAPI、CPA Manager Plus 无隶属或授权关系。使用前请完整阅读 [免责声明](DISCLAIMER.md)。

## 核心特性

- ✅ **OpenAI Chat Completions 兼容**：标准 `/v1/chat/completions` 接口，无缝对接各类 AI 客户端
- ✅ **思考流实时透传（Reasoning Content）**：
  - 流式响应中通过标准 `delta.reasoning_content` 实时推送模型的思考过程
  - 非流式响应中在 `message.reasoning_content` 中完整返回思考文本
- ✅ **纯文本约束注入（System Constraint Injection）**：在未声明工具时主动注入纯文本对话约束，避免 Composer 默认工具链导致空转超时
- ✅ **官方品牌图标（Official Brand Logo）**：携带 Cursor 官方品牌立方体标志，完美融入 CPA Manager Plus 管理端
- ✅ **函数工具调用（Tool Calling）**：支持标准 function tools、`tool_choice` 与多轮工具调用及结果续轮
- ✅ **会话检查点复用（Session Checkpoints）**：智能复用会话检查点，大幅优化多轮对话上下文传输开销
- ✅ **图片与文件输入**：支持图片等富媒体格式输入
- ✅ **OAuth 动态模型发现**：自动发现并适配当前账号支持的模型列表
- ✅ **多端跨平台支持**：支持 Linux (amd64/arm64)、macOS (amd64/arm64)、Windows (amd64/arm64)

## 架构概览

```text
客户端请求 (/v1/chat/completions)
    |
    v
CLIProxyAPI (CPA) 核心路由
    |
    +-- 匹配 type == "cursor" 的 OAuth 凭据
    |
    v
cursor-oauth 插件 (C ABI 动态库: .so / .dylib / .dll)
    +-- 检查会话检查点 (Session Checkpoint Reuse)
    +-- 注入文本对话约束 (避免工具空转)
    +-- 请求 Cursor 上游 API (https://api2.cursor.sh)
    |
    v
流式 / 非流式响应解析
    +-- delta.reasoning_content 思考流透传
    +-- delta.content 文本内容
    +-- delta.tool_calls 工具调用
```

## 安装与使用

### 方式一：通过插件商店一键安装（推荐）

配置 `store-sources` 指向本仓库的 CDN registry：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  store-sources:
    - "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@cdn/registry-v2.json"
  configs:
    cursor-oauth:
      enabled: true
```

在 CPA 管理端即可浏览并一键安装 `cursor-oauth`。

### 方式二：手动安装动态库

从 [GitHub Releases](https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/releases) 下载对应平台的 ZIP 压缩包（例如 `cursor-oauth_0.6.2_darwin_arm64.zip` 或 `cursor-oauth_0.6.2_linux_amd64.zip`）。

解压后将动态库放入 CPA 插件目录：

```text
plugins/
└── <os>/
    └── <arch>/
        └── cursor-oauth-v0.6.2.{so|dylib|dll}
```

在 `config.yaml` 中启用：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    cursor-oauth:
      enabled: true
```

### 方式三：脚本一键安装

使用 `packaging/` 目录下的自动化安装脚本：

```sh
sudo ./packaging/install.sh --plugins-dir /path/to/cliproxyapi/plugins
```

卸载：

```sh
sudo ./packaging/uninstall.sh --plugins-dir /path/to/cliproxyapi/plugins
```

## 调用示例

### 1. 流式请求（含思考过程透传）

```bash
curl https://your-cpa.example/v1/chat/completions \
  -H "Authorization: Bearer $CPA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cursor/composer-2-5",
    "messages": [{"role": "user", "content": "请简要解释什么是快排"}],
    "stream": true
  }'
```

流式返回中的思考内容与正式回复：

```json
{"id":"chatcmpl-xxx","choices":[{"index":0,"delta":{"reasoning_content":"正在思考快排的核心思想..."}}]}
{"id":"chatcmpl-xxx","choices":[{"index":0,"delta":{"content":"快速排序是一种分治算法..."}}]}
```

### 2. 非流式请求

```bash
curl https://your-cpa.example/v1/chat/completions \
  -H "Authorization: Bearer $CPA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cursor/auto",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": false
  }'
```

响应包含 `reasoning_content` 思考字段与 `content` 字段。

## 本地构建与打包

### 环境要求

- Go 1.25+（推荐 Go 1.27.1）
- CGO 编译器（GCC / Clang）

### 常用命令

```sh
# 运行单元测试
make test

# 构建当前平台的 c-shared 动态库
make build

# 多端打包商店规范格式的 ZIP 资产与 checksums.txt
make package-all

# 清理构建产物
make clean
```

## 许可证与致谢

- **许可证**：[MIT License](LICENSE)
- **原始代码与致谢**：
  - [kilolonion/cursor-cpa-plugin](https://github.com/kilolonion/cursor-cpa-plugin)（自研补丁与增强）
  - [yobo2u/omsub](https://github.com/yobo2u/omsub)（原始 cursor 插件实现）
  - [opencodex](https://github.com/lidge-jun/opencodex)（协议参考）
  - [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)（原生插件 ABI 与宿主平台）
