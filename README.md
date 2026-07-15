# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[English](#english)** | [中文](#中文)

---

## 中文

CLIProxyAPI 的持久化 Token 用量统计插件。插件通过官方 `usage_plugin` 接收用量记录，通过 `management_api` 注册只读资源接口和受保护的模型价格保存、重置接口，并在 Management Center 菜单中提供内嵌 iframe 仪表盘。

## 功能

- 默认可靠模式在 `usage.handle` 返回前完成 bbolt 提交，避免已回调记录因进程退出或内存队列积压而丢失；可显式启用异步批处理模式
- 统计接口暴露从宿主回调、解码、入队、处理到持久化的链路诊断计数，便于准确区分宿主漏发与插件内部积压
- 按 UTC 分钟持久化聚合，并保存逐请求用量元数据；不保存请求或响应正文
- 按模型、提供商、执行器、别名、来源、认证类型、服务层级、推理强度和失败状态分组
- 统计请求数、失败数、输入/输出/推理/缓存 Token、延迟、TTFT、生成时间、TPS 和缓存命中
- 支持最近 24 小时、7 天、30 天或全部保留数据，趋势图可按分钟/小时/日/周/月聚合
- 自包含中文仪表盘，无第三方前端依赖，包含指标卡片、堆叠 Token 趋势、模型环形占比、费用趋势、模型效率散点图和逐请求明细
- 支持模型下钻联动、趋势图滚轮缩放/平移、模型自定义价格，以及当前筛选数据 CSV 和 Dashboard PNG 导出
- 主题由 CLIProxyAPI Management Center 统一控制，自动同步跟随系统、纯白、羊毛纸和暗色模式
- 数据重置需 CLIProxyAPI 管理鉴权和显式 `reset` 确认
- Linux ARM64 `c-shared` 构建

## 隐私

插件不会保存或通过统计接口返回：

- API Key
- Auth ID / Auth Index
- 失败响应正文
- 响应头
- 请求或响应正文

数据库包含分钟级聚合维度与计数、逐请求用量元数据（例如时间、模型、来源、Tier、结果、延迟、推理强度、Token 计数和缓存命中），以及用户设置的模型单价；不会保存 prompt、响应内容或其他请求/响应正文。维度字段和逐请求元数据仍可能反映模型、来源或服务层级等运行信息。为使仪表盘打开时无需再次输入密钥，插件的只读资源接口不经过 CLIProxyAPI management 鉴权；请只在受信网络中暴露 CLIProxyAPI。受保护的 management 统计、模型价格保存和重置接口仍需管理鉴权。

## 配置

将共享库放在 CLIProxyAPI 的平台插件目录：

```text
plugins/linux/arm64/cap-token-usage-tracker.so
```

最小配置如下。默认配置已启用可靠模式，不需要修改或重新编译 CLIProxyAPI：

```yaml
plugins:
  enabled: true
  configs:
    cap-token-usage-tracker:
      enabled: true
```

如需指定持久化路径或调整保留策略，可使用完整配置：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      data_path: /var/lib/cliproxyapi/token-usage-tracker.db
      retention_days: 30
      flush_interval: 5s
      flush_max_records: 100
      sync_on_record: true
```

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `data_path` | `./data/token-usage-tracker.db` | bbolt 数据库路径；相对路径基于 CLIProxyAPI 进程工作目录，服务部署建议使用绝对路径 |
| `retention_days` | `30` | 保留的 UTC 天数，范围 1–3650 |
| `flush_interval` | `5s` | 批量刷盘最长间隔，范围 1 秒–1 小时 |
| `flush_max_records` | `100` | 接收指定数量记录后立即刷盘 |
| `sync_on_record` | `true` | `true`：回调返回前完成记录处理和 bbolt 提交，优先保证不漏记；`false`：仅进入内存 FIFO 后返回，并按批次刷盘 |

默认 `sync_on_record: true` 时，`usage.handle` 会等待存储 actor 完成记录处理和 bbolt transaction，数据库提交成功后才向宿主返回。这与参考插件在回调内同步执行 SQLite `INSERT` 后再返回的可靠性策略一致，只需加载本插件即可生效，不依赖任何 CLIProxyAPI 宿主补丁。

只有显式设置 `sync_on_record: false` 时，回调才会在记录进入进程内 FIFO 后立即返回，并按 `flush_interval` / `flush_max_records` 批量提交。该模式吞吐更高，但进程被强制终止时，尚在队列中或尚未刷盘的记录可能丢失。正常 shutdown 会按 FIFO 排空并刷盘。

统计接口中的 `diagnostics` 可用于定位实际链路：

| 字段 | 含义 |
|---|---|
| `callbacks_received` | 真正进入本插件 `usage.handle` 的次数 |
| `decoded` / `enqueued` | 成功解码并被存储层接受的次数 |
| `processed` | 存储 actor 已处理的次数 |
| `persisted_since_open` | 本次打开数据库后已成功提交到 bbolt 的次数 |
| `mailbox_depth` / `pending_flush` | 尚待 actor 处理 / 尚待刷盘的记录数 |
| `decode_errors` / `enqueue_errors` | 解码或存储错误数 |

默认可靠模式下，成功回调返回后 `callbacks_received`、`decoded`、`enqueued`、`processed` 和 `persisted_since_open` 应同步增长，且 `mailbox_depth` / `pending_flush` 应回到 0。`persisted_since_open` 是进程启动后的计数，不包含数据库中原有记录。

修改 `data_path` 会切换到一个独立数据库，不会自动迁移或删除旧文件。

## 页面与接口

插件 ID 取自共享库文件名。以 `cap-token-usage-tracker.so` 为例：

- 仪表盘：`/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- 仪表盘只读统计（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- 逐请求明细（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/requests?range=24h&offset=0&limit=100&model=gpt-4.1`
- 模型价格读取（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/prices`
- 受保护统计：`GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- 模型价格保存（需要 management key）：`PUT /v0/management/plugins/cap-token-usage-tracker/prices`
- 受保护重置：`POST /v0/management/plugins/cap-token-usage-tracker/reset`

统计范围：`24h`、`7d`、`30d`、`retention`。逐请求明细按时间倒序返回，`offset` 必须为非负整数，`limit` 默认为 100、最大为 500，`model` 可选并用于精确筛选模型。

Management Center 会把插件页面放入 iframe。仪表盘通过只读资源接口自动加载，打开和刷新页面都不需要 management key。保存模型价格或重置数据时会要求输入 Management Key；密钥仅用于当次保存或重置请求，不会写入插件数据库、浏览器存储或 URL。模型价格本身保存在插件 bbolt 数据库中，刷新页面和重启服务后仍会保留；重置统计不会删除模型价格。

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

A persistent Token usage tracking plugin for CLIProxyAPI. The plugin receives usage records via the official `usage_plugin`, registers read-only resource endpoints plus protected model-price persistence and reset endpoints through `management_api`, and provides an embedded iframe dashboard in the Management Center menu.

### Features

- Reliable mode commits each usage record to bbolt before `usage.handle` returns, preventing acknowledged callbacks from being lost to process exit or in-memory backlog; asynchronous batching remains opt-in
- Statistics expose callback-to-persistence diagnostics so host-side non-delivery can be distinguished from plugin backlog or storage failures
- Persistent aggregation by UTC minute plus per-request usage metadata; request and response bodies are not stored
- Grouped by model, provider, executor, alias, source, auth type, service tier, reasoning intensity, and failure status
- Counts requests, failures, input/output/reasoning/cached tokens, latency, TTFT, generation time, TPS, and cache hits
- Supports the last 24 hours, 7 days, 30 days, or all retained data, with minute/hour/day/week/month trend granularity
- Self-contained Chinese dashboard with no third-party frontend dependencies, including stat cards, stacked Token trends, a model doughnut chart, cost trends, a model-efficiency scatter plot, and per-request details
- Supports linked model drill-down, wheel zoom/pan for trends, custom model pricing, filtered CSV export, and Dashboard PNG export
- Theme is controlled by the CLIProxyAPI Management Center and automatically syncs Follow System, Pure White, Wool Paper, and Dark modes
- Data reset requires CLIProxyAPI management authentication and explicit `reset` confirmation
- Linux ARM64 `c-shared` build

### Privacy

The plugin does not store or return via statistics endpoints:

- API Key
- Auth ID / Auth Index
- Failure response body
- Response headers
- Request or response body

The database contains minute-level aggregation dimensions and counts, per-request usage metadata such as time, model, source, tier, result, latency, reasoning intensity, Token counters, and cache-hit status, and user-configured model prices. It does not store prompts, generated content, or other request/response bodies. Dimensions and request metadata may still reflect operational information such as model, source, or service tier. To let the dashboard open without asking for the key again, the read-only resource endpoints do not use CLIProxyAPI management authentication; expose CLIProxyAPI only on a trusted network. The protected management statistics, model-price save, and reset endpoints still require management authentication.

### Configuration

Place the shared library in the CLIProxyAPI platform plugin directory:

```text
plugins/linux/arm64/cap-token-usage-tracker.so
```

The minimal configuration is shown below. Reliable mode is enabled by default and does not require patching or rebuilding CLIProxyAPI:

```yaml
plugins:
  enabled: true
  configs:
    cap-token-usage-tracker:
      enabled: true
```

Use the full configuration only when you need to customize persistence or retention:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      data_path: /var/lib/cliproxyapi/token-usage-tracker.db
      retention_days: 30
      flush_interval: 5s
      flush_max_records: 100
      sync_on_record: true
```

| Field | Default | Description |
|---|---:|---|
| `data_path` | `./data/token-usage-tracker.db` | bbolt database path; relative paths are based on the CLIProxyAPI process working directory. Absolute paths are recommended for service deployments |
| `retention_days` | `30` | Retention period in UTC days, range 1–3650 |
| `flush_interval` | `5s` | Maximum interval for batch flush, range 1 second–1 hour |
| `flush_max_records` | `100` | Flush immediately after receiving this many records |
| `sync_on_record` | `true` | `true`: process and commit to bbolt before the callback returns; `false`: return after FIFO acceptance and flush in batches |

With the default `sync_on_record: true`, `usage.handle` waits for the store actor to process the record and commit its bbolt transaction before acknowledging the host callback. This matches the reference plugin's durability strategy of returning only after its synchronous SQLite `INSERT` succeeds. Loading this plugin is sufficient; no CLIProxyAPI host patch is required.

Only an explicit `sync_on_record: false` enables asynchronous batching: the callback returns after FIFO acceptance, and records are committed according to `flush_interval` / `flush_max_records`. This improves throughput, but a forced process termination can lose records still queued or awaiting a flush. Normal shutdown drains the FIFO and flushes it.

The statistics response includes `diagnostics` for tracing the callback-to-persistence path:

| Field | Meaning |
|---|---|
| `callbacks_received` | Calls that actually entered this plugin's `usage.handle` |
| `decoded` / `enqueued` | Records successfully decoded and accepted by storage |
| `processed` | Records processed by the store actor |
| `persisted_since_open` | Records committed to bbolt since this database was opened |
| `mailbox_depth` / `pending_flush` | Records awaiting actor processing / disk flush |
| `decode_errors` / `enqueue_errors` | Decode or storage errors |

In reliable mode, after a successful callback returns, `callbacks_received`, `decoded`, `enqueued`, `processed`, and `persisted_since_open` should advance together, while `mailbox_depth` and `pending_flush` return to zero. `persisted_since_open` excludes records that existed before process startup.

Changing `data_path` switches to a separate database; the old file is not automatically migrated or deleted.

### Pages & Endpoints

The plugin ID is derived from the shared library filename. Using `cap-token-usage-tracker.so` as an example:

- Dashboard: `/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- Dashboard read-only statistics (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- Per-request details (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/requests?range=24h&offset=0&limit=100&model=gpt-4.1`
- Model prices (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/prices`
- Protected statistics: `GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- Save model prices (management key required): `PUT /v0/management/plugins/cap-token-usage-tracker/prices`
- Protected reset: `POST /v0/management/plugins/cap-token-usage-tracker/reset`

Statistics ranges: `24h`, `7d`, `30d`, `retention`. Request details are returned newest first; `offset` must be a non-negative integer, `limit` defaults to 100 and is capped at 500, and optional `model` applies an exact model filter.

The Management Center embeds the plugin page in an iframe. The dashboard loads automatically through the read-only resource endpoints, so opening and refreshing it does not require a management key. A Management Key is requested when saving model prices or resetting data; it is used only for that save or reset request and is not written to the plugin database, browser storage, or URL. Model prices themselves are stored in the plugin bbolt database, survive page refreshes and service restarts, and are not removed by resetting statistics.

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
