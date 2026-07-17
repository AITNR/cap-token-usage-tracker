# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[English](#english)** | [中文](#中文)

---

## 中文

CLIProxyAPI 的持久化 Token 用量统计插件。插件通过官方 `usage_plugin` 接收用量记录，通过 `management_api` 注册只读资源接口和受保护的模型价格保存、重置接口，并在 Management Center 菜单中提供内嵌 iframe 仪表盘。

## 功能

- 按 UTC 分钟持久化聚合，并保存逐请求用量元数据；不保存请求或响应正文
- 按模型、提供商、执行器、别名、来源、认证类型、服务层级、推理强度和失败状态分组
- 统计请求数、失败数、输入/输出/推理/缓存 Token、延迟、TTFT、生成时间、TPS 和缓存命中
- 支持最近 24 小时、7 天、30 天或全部保留数据，趋势图可按分钟/小时/日/周/月聚合
- 自包含中文仪表盘，无第三方前端依赖，包含指标卡片、堆叠 Token 趋势、模型环形占比、精确费用趋势、模型效率散点图和逐请求明细
- 支持 Input、Output、Cache Read、Cache Creation 四类模型价格、逐请求 Context Tier、免费模型、价格覆盖率和缺价提示
- 支持从 models.dev 手动同步 CLIProxyAPI `/v1/models` 当前返回的模型价格，可配置提供商优先级、忽略后缀和显式模型映射；手工价格优先
- 支持模型下钻联动、趋势图滚轮缩放/平移、移动端自适应坐标轴、总 Token 完整/k/m 切换、全页面 USD/CNY 最新汇率显示、当前筛选数据 CSV 和 Dashboard PNG 导出
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

数据库包含分钟级聚合维度与计数、逐请求用量元数据（例如时间、模型、来源、Tier、结果、延迟、推理强度、Token 计数和缓存命中），以及用户设置或从 models.dev 同步的模型价格、Context Tier、匹配设置和同步来源元数据；不会保存 prompt、响应内容或其他请求/响应正文。维度字段和逐请求元数据仍可能反映模型、来源或服务层级等运行信息。为使仪表盘打开时无需再次输入密钥，插件的只读资源接口不经过 CLIProxyAPI management 鉴权；请只在受信网络中暴露 CLIProxyAPI。受保护的 management 统计、模型价格保存、models.dev 同步和重置接口仍需管理鉴权。

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
      sync_on_record: true
```

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `data_path` | `./data/token-usage-tracker.db` | bbolt 数据库路径；相对路径基于 CLIProxyAPI 进程工作目录，服务部署建议使用绝对路径 |
| `retention_days` | `30` | 保留的 UTC 天数，范围 1–3650 |
| `flush_interval` | `5s` | 批量刷盘最长间隔，范围 1 秒–1 小时 |
| `flush_max_records` | `100` | 接收指定数量记录后立即刷盘 |
| `sync_on_record` | `true` | 默认每条记录提交数据库后才确认；设为 `false` 可启用批量模式以提高吞吐 |

默认同步模式会在 `usage.handle` 返回前提交每条统计，避免正常记录停留在未刷盘窗口。仅当显式设置 `sync_on_record: false` 时启用批量模式；进程被强制终止时，批量模式最多可能损失一个 `flush_interval` 或未达到 `flush_max_records` 的窗口。

修改 `data_path` 会切换到一个独立数据库，不会自动迁移或删除旧文件。

## 页面与接口

插件 ID 取自共享库文件名。以 `cap-token-usage-tracker.so` 为例：

- 仪表盘：`/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- 仪表盘只读统计（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- 逐请求明细与当前价格下的 `estimated_cost`（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/requests?range=24h&offset=0&limit=100&model=gpt-4.1`
- 逐请求精确汇总费用（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/costs?range=24h`
- 最新 USD/CNY 显示汇率（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/exchange-rate`
- 模型价格、同步设置和最近同步结果读取（无需 management key）：`GET /v0/resource/plugins/cap-token-usage-tracker/prices`
- 受保护统计：`GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- 模型价格完整替换保存（需要 management key）：`PUT /v0/management/plugins/cap-token-usage-tracker/prices`
- 从 models.dev 同步价格（需要 management key）：`POST /v0/management/plugins/cap-token-usage-tracker/prices/sync`
- 受保护重置：`POST /v0/management/plugins/cap-token-usage-tracker/reset`

统计范围：`24h`、`7d`、`30d`、`retention`。逐请求明细按时间倒序返回，`offset` 必须为非负整数，`limit` 默认为 100、最大为 500，`model` 可选并用于精确筛选模型。

Management Center 会把插件页面放入 iframe。仪表盘通过只读资源接口自动加载，打开和刷新页面都不需要 management key。价格弹窗使用临时 CLIProxyAPI API Key 从同源 `/v1/models` 加载当前模型目录；保存价格、同步 models.dev 或重置数据时仍要求 Management Key。两种密钥都只保存在当前 DOM/内存中，关闭对话框后清空，不会写入插件数据库、浏览器存储或 URL。模型价格、同步设置和同步来源元数据保存在插件 bbolt 数据库中，刷新页面和重启服务后仍会保留；重置统计不会删除价格簿。

## 价格、Context Tier 与费用估算

每个模型可配置以下 USD / 1M Token 单价：

- `input`
- `output`
- `cache_read`
- `cache_creation`

Context Tier 按**单次请求**选择，而不是按模型或时间段聚合总量选择。`context_tokens > threshold` 时启用对应档位；等于 threshold 时仍使用较低档，多个档位同时满足时选择 threshold 最大的一档。每个档位完整替换四类基础价格。

费用计算优先使用 `CacheReadTokens`；其为 0 时才使用兼容字段 `CachedTokens`，两者不会重复收费。Provider 为精确的 `anthropic` 或执行器为 `claude` 时，Input 按“不含缓存”处理；其他或未知 Provider 默认按“Input 已含缓存”处理并先扣除 Cache Read/Creation，避免重复收费。Reasoning Token 当前不单独计价。

所有费用都是使用**当前价格簿**对保留的逐请求数据重新估算。修改或同步价格后，历史请求的预估费用会随之变化；这些值不是供应商账单，也不是请求发生时的价格快照。显式保存四类价格均为 0 的模型表示免费模型，仍计入“已定价”覆盖率。`PUT /prices` 是完整替换：省略某个已有模型即删除该价格；未修改的 models.dev 条目保留同步来源，编辑后转为手工覆盖。

models.dev 同步只导入同步操作前从 CLIProxyAPI `/v1/models` 重新获取的当前模型，不再使用 retention 中的历史用量模型，也不保存整个 models.dev 目录。默认提供商优先级为 `openai, google, anthropic`，并支持忽略模型后缀及 `source=target` 显式映射。手工价格不会被后续同步覆盖。同步使用固定的 `https://models.dev/api.json`、标准 Go HTTP 代理环境变量、约 15 秒超时和 16 MiB 响应上限；并发同步或同步期间价格簿被修改会返回 HTTP 409，远端超时返回 504，其他目录/网络错误返回 502。

USD/CNY 切换只改变页面与 PNG 的显示：价格簿、后端 `*_usd` 字段和 CSV 始终保持 USD。插件通过固定 HTTPS 汇率源获取最新 USD/CNY，进程内缓存 1 小时；刷新失败时最多使用 24 小时内的缓存汇率并在页面标记，完全无可用汇率时保持 USD。价格删除在弹窗中先显示为可撤销的“待保存删除”，保存完整价格簿后才会重新计算历史费用。

当上游 `TotalTokens <= 0` 时，新接收记录按 `max(input,0) + max(output,0) + max(reasoning,0)` 饱和求和；若结果仍为 0，再使用正数 `CachedTokens`。`CacheReadTokens` 和 `CacheCreationTokens` 不参与该 fallback，已有历史记录不会被重写。

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

- Persistent aggregation by UTC minute plus per-request usage metadata; request and response bodies are not stored
- Grouped by model, provider, executor, alias, source, auth type, service tier, reasoning intensity, and failure status
- Counts requests, failures, input/output/reasoning/cached tokens, latency, TTFT, generation time, TPS, and cache hits
- Supports the last 24 hours, 7 days, 30 days, or all retained data, with minute/hour/day/week/month trend granularity
- Self-contained Chinese dashboard with no third-party frontend dependencies, including stat cards, stacked Token trends, a model doughnut chart, exact cost trends, a model-efficiency scatter plot, and per-request details
- Supports Input, Output, Cache Read, and Cache Creation prices, per-request context tiers, free models, pricing coverage, and missing-price reporting
- Supports manual synchronization from models.dev for the current models returned by CLIProxyAPI `/v1/models`, with configurable provider priority, ignored suffixes, and explicit model mappings; manual prices take precedence
- Supports linked model drill-down, wheel zoom/pan, responsive mobile chart axes, full/k/m total-Token display, dashboard-wide USD/CNY latest-rate display, filtered CSV export, and Dashboard PNG export
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

The database contains minute-level aggregation dimensions and counts, per-request usage metadata such as time, model, source, tier, result, latency, reasoning intensity, Token counters, and cache-hit status, plus manually configured or models.dev-synchronized prices, context tiers, matching settings, and synchronization provenance. It does not store prompts, generated content, or other request/response bodies. Dimensions and request metadata may still reflect operational information such as model, source, or service tier. To let the dashboard open without asking for the key again, the read-only resource endpoints do not use CLIProxyAPI management authentication; expose CLIProxyAPI only on a trusted network. Protected management statistics, model-price saves, models.dev synchronization, and reset still require management authentication.

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
      sync_on_record: true
```

| Field | Default | Description |
|---|---:|---|
| `data_path` | `./data/token-usage-tracker.db` | bbolt database path; relative paths are based on the CLIProxyAPI process working directory. Absolute paths are recommended for service deployments |
| `retention_days` | `30` | Retention period in UTC days, range 1–3650 |
| `flush_interval` | `5s` | Maximum interval for batch flush, range 1 second–1 hour |
| `flush_max_records` | `100` | Flush immediately after receiving this many records |
| `sync_on_record` | `true` | Commits each record before acknowledgement by default; set to `false` to enable higher-throughput batching |

The default synchronous mode commits each statistic before `usage.handle` returns, avoiding an unflushed normal-operation window. Batching is enabled only when `sync_on_record: false` is set explicitly; if the process is forcefully terminated in batch mode, up to one `flush_interval` or unflushed `flush_max_records` window may be lost.

Changing `data_path` switches to a separate database; the old file is not automatically migrated or deleted.

### Pages & Endpoints

The plugin ID is derived from the shared library filename. Using `cap-token-usage-tracker.so` as an example:

- Dashboard: `/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- Dashboard read-only statistics (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- Per-request details with current-price `estimated_cost` (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/requests?range=24h&offset=0&limit=100&model=gpt-4.1`
- Exact per-request-derived cost summary (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/costs?range=24h`
- Latest USD/CNY display exchange rate (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/exchange-rate`
- Model prices, synchronization settings, and last synchronization result (no management key): `GET /v0/resource/plugins/cap-token-usage-tracker/prices`
- Protected statistics: `GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- Full-replacement model-price save (management key required): `PUT /v0/management/plugins/cap-token-usage-tracker/prices`
- Synchronize prices from models.dev (management key required): `POST /v0/management/plugins/cap-token-usage-tracker/prices/sync`
- Protected reset: `POST /v0/management/plugins/cap-token-usage-tracker/reset`

Statistics ranges: `24h`, `7d`, `30d`, `retention`. Request details are returned newest first; `offset` must be a non-negative integer, `limit` defaults to 100 and is capped at 500, and optional `model` applies an exact model filter.

The Management Center embeds the plugin page in an iframe. The dashboard loads automatically through the read-only resource endpoints, so opening and refreshing it does not require a management key. The pricing dialog uses a temporary CLIProxyAPI API Key to load the current model directory from same-origin `/v1/models`; a Management Key is still required to save prices, synchronize models.dev, or reset data. Both keys exist only in the current DOM/memory, are cleared when the dialog closes, and are never written to the plugin database, browser storage, or URL. Prices, synchronization settings, and provenance are stored in bbolt, survive page refreshes and service restarts, and are not removed by statistics reset.

### Pricing, Context Tiers, and Cost Estimation

Each model can define the following USD-per-million-Token rates:

- `input`
- `output`
- `cache_read`
- `cache_creation`

Context tiers are selected **per request**, never from an aggregated model or time-range total. A tier applies only when `context_tokens > threshold`; equality stays on the lower rate, and the greatest qualifying threshold wins. Each selected tier replaces all four base rates.

Cost calculation prefers `CacheReadTokens` and falls back to the compatibility `CachedTokens` counter only when Cache Read is zero; the two counters are never charged together. When the exact provider is `anthropic` or the executor is `claude`, Input is treated as excluding cache tokens. Other and unknown providers default to Input-includes-cache accounting and subtract Cache Read/Creation before charging ordinary Input, avoiding double billing. Reasoning Tokens are not priced separately.

All costs are **current-price estimates** over retained per-request records. Changing or synchronizing prices reprices historical requests; the result is neither a provider invoice nor a request-time price snapshot. An explicitly saved model with all four rates set to zero is a valid free model and still counts as priced coverage. `PUT /prices` is a full replacement: omitting an existing model deletes it. An unchanged models.dev entry retains its provenance; editing it creates a manual override.

models.dev synchronization imports only the current model list freshly loaded from CLIProxyAPI `/v1/models` before the synchronization request; it no longer uses historical models observed in the retention window and never stores the full models.dev catalog. The default provider priority is `openai, google, anthropic`; ignored model suffixes and explicit `source=target` mappings are configurable. Manual prices are never overwritten by synchronization. Runtime synchronization uses the fixed `https://models.dev/api.json` endpoint, standard Go HTTP proxy environment variables, an approximately 15-second timeout, and a 16 MiB response limit. Concurrent synchronization or a price-book change during an in-flight synchronization returns HTTP 409; remote timeout returns 504 and other catalog/network failures return 502.

USD/CNY switching affects only dashboard and PNG display. The price book, backend `*_usd` fields, and CSV always remain in USD. The plugin fetches the latest USD/CNY rate from a fixed HTTPS provider, caches it in-process for one hour, and may use a cache no older than 24 hours after refresh failure while marking it as stale. If no rate is available, display remains in USD. Deleting a price first creates a reversible pending-delete draft in the dialog; retained-request costs are recalculated only after the complete price book is saved.

For newly received records where upstream `TotalTokens <= 0`, the plugin uses a saturating positive sum of Input + Output + Reasoning. If that sum is still zero, it falls back to positive `CachedTokens`. Cache Read and Cache Creation do not enter this fallback, and existing historical records are not rewritten.

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
