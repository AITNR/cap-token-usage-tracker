# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[English](#english)** | [中文](#中文)

---

## 中文

CLIProxyAPI 的持久化 Token 用量统计插件。插件通过官方 `usage_plugin` 接收用量记录，通过 `management_api` 注册只读资源统计接口和受保护的重置接口，并在 Management Center 菜单中提供内嵌 iframe 仪表盘。

## 功能

- 按 UTC 小时持久化聚合，不保存逐请求正文
- 按模型、提供商、执行器、别名、来源、认证类型、服务层级、推理强度和失败状态分组
- 统计请求数、失败数、输入/输出/推理/缓存 Token、延迟和 TTFT
- 支持最近 24 小时、7 天、30 天或全部保留数据
- 自包含中文仪表盘，无第三方前端依赖
- 数据重置需 CLIProxyAPI 管理鉴权和显式 `reset` 确认
- Linux ARM64 `c-shared` 构建

## 隐私

插件不会保存或通过统计接口返回：

- API Key
- Auth ID / Auth Index
- 失败响应正文
- 响应头
- 请求或响应正文

数据库仅包含小时聚合维度和计数。维度字段可能仍反映模型、来源或服务层级等运行信息。为使仪表盘打开时无需再次输入密钥，插件的只读资源统计接口不经过 CLIProxyAPI management 鉴权；请只在受信网络中暴露 CLIProxyAPI。受保护的 management 统计接口和重置接口仍需管理鉴权。

## 配置

将共享库放在 CLIProxyAPI 的平台插件目录：

```text
plugins/linux/arm64/cap-token-usage-tracker.so
```

CLIProxyAPI 配置示例：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      priority: 0
      data_path: /var/lib/cliproxyapi/token-usage-tracker.db
      retention_days: 30
      flush_interval: 5s
      flush_max_records: 100
      sync_on_record: false
```

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `data_path` | `./data/token-usage-tracker.db` | bbolt 数据库路径；相对路径基于 CLIProxyAPI 进程工作目录，服务部署建议使用绝对路径 |
| `retention_days` | `30` | 保留的 UTC 天数，范围 1–3650 |
| `flush_interval` | `5s` | 批量刷盘最长间隔，范围 1 秒–1 小时 |
| `flush_max_records` | `100` | 接收指定数量记录后立即刷盘 |
| `sync_on_record` | `false` | 开启后每条记录提交数据库后才确认，可靠性更高但吞吐更低 |

默认批量模式在正常 shutdown 时会刷写全部待处理数据；进程被强制终止时，最多可能损失一个 `flush_interval` 或未达到 `flush_max_records` 的窗口。需要更强持久性时开启 `sync_on_record`。

修改 `data_path` 会切换到一个独立数据库，不会自动迁移或删除旧文件。

## 页面与接口

插件 ID 取自共享库文件名。以 `cap-token-usage-tracker.so` 为例：

- 仪表盘：`/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- 仪表盘只读统计（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- 受保护统计：`GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- 受保护重置：`POST /v0/management/plugins/cap-token-usage-tracker/reset`

统计范围：`24h`、`7d`、`30d`、`retention`。

Management Center 会把插件页面放入 iframe。仪表盘通过只读资源统计接口自动加载，打开和刷新页面都不需要 management key。只有点击“重置数据”时才会要求输入 Management Key，且密钥仅用于该次请求，不会写入插件数据库、浏览器存储或 URL。

重置请求正文：

```json
{"confirm":"reset"}
```

## Linux ARM64 构建

要求：

- Go 1.26+
- `aarch64-linux-gnu-gcc`
- `file`、`readelf`、`nm`、`sha256sum`
- Clash HTTP 代理监听本机 `7897`

在 Debian/Ubuntu/WSL 中通常需要：

```bash
sudo apt install gcc-aarch64-linux-gnu libc6-dev-arm64-cross binutils-aarch64-linux-gnu file curl
```

安装包和 Go 模块下载都应通过 Clash `7897`。构建脚本默认先尝试 `http://127.0.0.1:7897`；若 WSL 无法访问 Windows localhost，会尝试 WSL 默认网关的 `7897`。也可以显式指定：

```bash
export CLASH_PROXY_URL=http://<windows-host>:7897
```

构建：

```bash
bash scripts/build-linux-arm64.sh
```

可通过 `VERSION=v1.0.0` 注入插件版本：

```bash
VERSION=v1.0.0 bash scripts/build-linux-arm64.sh
```

产物：

```text
dist/cap-token-usage-tracker-v1.0.0-linux-arm64.so  # 版本化发布文件
dist/cap-token-usage-tracker-v1.0.0-linux-arm64.h   # CGO 生成的 ABI 头文件
dist/cap-token-usage-tracker.so                      # 安装文件
```

安装时必须使用 `cap-token-usage-tracker.so` 这个文件名，因为 CLIProxyAPI 会根据共享库文件名派生 plugin ID：

```bash
cp dist/cap-token-usage-tracker.so /path/to/CLIProxyAPI/plugins/linux/arm64/
```

验证并生成可移植的 `dist/SHA256SUMS`：

```bash
bash scripts/verify-linux-arm64.sh
```

验证脚本检查 Go 格式、vet、普通/race 测试、ELF64/AArch64/DYN 类型、安装文件与发布文件字节一致性和以下 ABI 导出：

- `cliproxy_plugin_init`
- `cliproxyPluginCall`
- `cliproxyPluginFree`
- `cliproxyPluginShutdown`

## 本地开发

```bash
gofmt -w *.go
go vet ./...
CGO_ENABLED=0 go test ./...
go test ./...
```

`main_cgo.go` 只在 cgo 开启时参与编译。发布前必须实际执行 Linux ARM64 `c-shared` 构建；仅通过 `CGO_ENABLED=0` 测试不能证明 ABI 可以链接。

## 协议

[MIT License](LICENSE)

---

## English

A persistent Token usage tracking plugin for CLIProxyAPI. The plugin receives usage records via the official `usage_plugin`, registers a read-only resource statistics endpoint and protected reset endpoint through `management_api`, and provides an embedded iframe dashboard in the Management Center menu.

### Features

- Persistent aggregation by UTC hour; no per-request body is stored
- Grouped by model, provider, executor, alias, source, auth type, service tier, reasoning intensity, and failure status
- Counts requests, failures, input/output/reasoning/cached tokens, latency, and TTFT
- Supports last 24 hours, 7 days, 30 days, or all retained data
- Self-contained Chinese dashboard with no third-party frontend dependencies
- Data reset requires CLIProxyAPI management authentication and explicit `reset` confirmation
- Linux ARM64 `c-shared` build

### Privacy

The plugin does not store or return via statistics endpoints:

- API Key
- Auth ID / Auth Index
- Failure response body
- Response headers
- Request or response body

The database contains only hourly aggregation dimensions and counts. Dimension fields may still reflect operational information such as model, source, or service tier. To let the dashboard open without asking for the key again, the read-only resource statistics endpoint does not use CLIProxyAPI management authentication; expose CLIProxyAPI only on a trusted network. The protected management statistics and reset endpoints still require management authentication.

### Configuration

Place the shared library in the CLIProxyAPI platform plugin directory:

```text
plugins/linux/arm64/cap-token-usage-tracker.so
```

CLIProxyAPI configuration example:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      priority: 0
      data_path: /var/lib/cliproxyapi/token-usage-tracker.db
      retention_days: 30
      flush_interval: 5s
      flush_max_records: 100
      sync_on_record: false
```

| Field | Default | Description |
|---|---:|---|
| `data_path` | `./data/token-usage-tracker.db` | bbolt database path; relative paths are based on the CLIProxyAPI process working directory. Absolute paths are recommended for service deployments |
| `retention_days` | `30` | Retention period in UTC days, range 1–3650 |
| `flush_interval` | `5s` | Maximum interval for batch flush, range 1 second–1 hour |
| `flush_max_records` | `100` | Flush immediately after receiving this many records |
| `sync_on_record` | `false` | When enabled, each record is committed to the database before acknowledgement; higher durability but lower throughput |

In default batch mode, all pending data is flushed on normal shutdown. If the process is forcefully terminated, up to one `flush_interval` or unflushed `flush_max_records` window may be lost. Enable `sync_on_record` for stronger durability.

Changing `data_path` switches to a separate database; the old file is not automatically migrated or deleted.

### Pages & Endpoints

The plugin ID is derived from the shared library filename. Using `cap-token-usage-tracker.so` as an example:

- Dashboard: `/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- Dashboard read-only statistics (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- Protected statistics: `GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- Protected reset: `POST /v0/management/plugins/cap-token-usage-tracker/reset`

Statistics ranges: `24h`, `7d`, `30d`, `retention`.

The Management Center embeds the plugin page in an iframe. The dashboard loads automatically through the read-only resource statistics endpoint, so opening and refreshing it does not require a management key. A Management Key is requested only when “Reset Data” is clicked, and it is used for that request only; it is not written to the plugin database, browser storage, or URL.

Reset request body:

```json
{"confirm":"reset"}
```

### Linux ARM64 Build

Requirements:

- Go 1.26+
- `aarch64-linux-gnu-gcc`
- `file`, `readelf`, `nm`, `sha256sum`
- Clash HTTP proxy listening on local port `7897`

On Debian/Ubuntu/WSL you typically need:

```bash
sudo apt install gcc-aarch64-linux-gnu libc6-dev-arm64-cross binutils-aarch64-linux-gnu file curl
```

Both package installation and Go module downloads should go through Clash `7897`. The build script first tries `http://127.0.0.1:7897`; if WSL cannot reach Windows localhost it falls back to the WSL default gateway's `7897`. You can also specify explicitly:

```bash
export CLASH_PROXY_URL=http://<windows-host>:7897
```

Build:

```bash
bash scripts/build-linux-arm64.sh
```

Inject the plugin version via `VERSION=v1.0.0`:

```bash
VERSION=v1.0.0 bash scripts/build-linux-arm64.sh
```

Artifacts:

```text
dist/cap-token-usage-tracker-v1.0.0-linux-arm64.so  # Versioned release file
dist/cap-token-usage-tracker-v1.0.0-linux-arm64.h   # CGO-generated ABI header
dist/cap-token-usage-tracker.so                      # Install file
```

The install filename must be `cap-token-usage-tracker.so` because CLIProxyAPI derives the plugin ID from the shared library filename:

```bash
cp dist/cap-token-usage-tracker.so /path/to/CLIProxyAPI/plugins/linux/arm64/
```

Verify and generate portable `dist/SHA256SUMS`:

```bash
bash scripts/verify-linux-arm64.sh
```

The verification script checks Go format, vet, normal/race tests, ELF64/AArch64/DYN type, byte-level consistency between install and release files, and the following ABI exports:

- `cliproxy_plugin_init`
- `cliproxyPluginCall`
- `cliproxyPluginFree`
- `cliproxyPluginShutdown`

### Local Development

```bash
gofmt -w *.go
go vet ./...
CGO_ENABLED=0 go test ./...
go test ./...
```

`main_cgo.go` only participates in compilation when cgo is enabled. Before release, an actual Linux ARM64 `c-shared` build must be performed; passing `CGO_ENABLED=0` tests alone does not prove the ABI can link.

### License

[MIT License](LICENSE)
