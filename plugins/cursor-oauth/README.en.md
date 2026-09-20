# cursor-oauth

[中文](README.md) | English

`cursor-oauth` is a native C-shared library plugin for [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) published in the official CLIProxyAPI Plugins Store. It bridges user-authorized **Cursor** subscription accounts to the OpenAI-compatible `/v1/chat/completions` interface.

Migrated from the community projects [kilolonion/cursor-cpa-plugin](https://github.com/kilolonion/cursor-cpa-plugin) and [yobo2u/omsub](https://github.com/yobo2u/omsub), this plugin is deeply integrated into the CLIProxyAPI Plugins Store multi-platform build and release infrastructure, bringing **reasoning stream passthrough**, **system constraint injection**, **official brand logo**, and **session checkpoint reuse**.

> [!IMPORTANT]
> This project is an unofficial community plugin and is not affiliated with or endorsed by Cursor, Anysphere, CLIProxyAPI, or CPA Manager Plus. Please read the [Disclaimer](DISCLAIMER.md) before use.

## Features

- ✅ **OpenAI Chat Completions Compatible**: Standard `/v1/chat/completions` endpoint compatible with any OpenAI client.
- ✅ **Real-Time Reasoning Stream**:
  - In streaming mode, pushes thinking steps in real time via the standard `delta.reasoning_content` field.
  - In non-streaming mode, returns complete thinking text in `message.reasoning_content`.
- ✅ **System Constraint Injection**: Injects conversational constraints when no tools are requested, preventing Composer 2.5 default tools from spinning and timing out.
- ✅ **Official Brand Logo**: Displays Cursor's official cube brand logo in CPA Manager Plus dashboards.
- ✅ **Function Calling**: Full support for standard function tools, `tool_choice`, multi-turn tool loops, and continuation turns.
- ✅ **Session Checkpoint Reuse**: Intelligently reuses session checkpoints to reduce multi-turn context overhead.
- ✅ **Multimodal & Attachments**: Supports image inputs and file attachments.
- ✅ **Dynamic OAuth Model Discovery**: Automatically queries and reflects models available to the authenticated account.
- ✅ **Cross-Platform**: Compiles for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64/arm64).

## Architecture

```text
Client Request (/v1/chat/completions)
    |
    v
CLIProxyAPI (CPA) Host Routing
    |
    +-- Match OAuth credential where type == "cursor"
    |
    v
cursor-oauth Plugin (C ABI Dynamic Library: .so / .dylib / .dll)
    +-- Check session checkpoints (Session Checkpoint Reuse)
    +-- Inject system conversational constraint
    +-- Call Cursor upstream API (https://api2.cursor.sh)
    |
    v
Stream / Non-Stream Response Processing
    +-- delta.reasoning_content (thinking stream)
    +-- delta.content (completion text)
    +-- delta.tool_calls (tool calls)
```

## Installation

### Method 1: CPA Plugin Store Direct Install (Recommended)

Configure `store-sources` pointing to this repository's CDN registry:

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

Install `cursor-oauth` directly from the CPA management interface.

### Method 2: Manual Dynamic Library Installation

Download the platform zip archive from [GitHub Releases](https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/releases) (e.g. `cursor-oauth_0.6.3_darwin_arm64.zip` or `cursor-oauth_0.6.3_linux_amd64.zip`).

Extract the library into your CPA plugins directory:

```text
plugins/
└── <os>/
    └── <arch>/
        └── cursor-oauth-v0.6.3.{so|dylib|dll}
```

Enable in `config.yaml`:

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    cursor-oauth:
      enabled: true
```

### Method 3: Automated Script Installation

Use the provided installation script:

```sh
sudo ./packaging/install.sh --plugins-dir /path/to/cliproxyapi/plugins
```

Uninstall:

```sh
sudo ./packaging/uninstall.sh --plugins-dir /path/to/cliproxyapi/plugins
```

## Usage Examples

### 1. Streaming with Thinking Content

```bash
curl https://your-cpa.example/v1/chat/completions \
  -H "Authorization: Bearer $CPA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cursor/composer-2-5",
    "messages": [{"role": "user", "content": "Explain quicksort"}],
    "stream": true
  }'
```

### 2. Non-Streaming Request

```bash
curl https://your-cpa.example/v1/chat/completions \
  -H "Authorization: Bearer $CPA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cursor/auto",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

## Development & Building

### Requirements

- Go 1.25+ (recommended Go 1.27.1)
- CGO compiler (GCC / Clang)

### Makefile Commands

```sh
# Run unit tests
make test

# Build c-shared library for current host platform
make build

# Package store-compliant zip archives and checksums.txt
make package-all

# Clean build artifacts
make clean
```

## License & Credits

- **License**: [MIT License](LICENSE)
- **Credits**:
  - [kilolonion/cursor-cpa-plugin](https://github.com/kilolonion/cursor-cpa-plugin)
  - [yobo2u/omsub](https://github.com/yobo2u/omsub)
  - [opencodex](https://github.com/lidge-jun/opencodex)
  - [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)
