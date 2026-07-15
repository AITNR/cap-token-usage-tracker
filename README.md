# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[English](#english)** | [中文](#中文)

---

## 中文

CLIProxyAPI 的持久化 Token 用量统计插件。Token 采集和存储机制按 `Fwindy/cpa-usage-statistics` 的实现方式重建：**每个 `usage.handle` 回调对应一条 SQLite 请求记录，并在同步 `INSERT` 成功后才向宿主返回**。

当前项目在该逐请求存储基础上继续提供原有中文仪表盘、聚合统计、逐请求脱敏明细和模型价格功能。

### 核心特性

- 使用纯 Go SQLite 驱动 `modernc.org/sqlite`，数据库固定为 `usage.db`
- 每次请求生成 UUID，并保存 UTC RFC3339Nano 时间
- `usage.handle` 内同步写入，不再使用内存 FIFO、store actor 或异步批量刷盘
- 保存完整的上游 `usage.Record` / `usage.Detail` 用量元数据
- 打开数据库时自动检查并补齐 `usage_records` 缺失列
- `retention_days=0` 表示永久保留；启用保留策略后最多每小时执行一次过期清理
- 保留仪表盘所需的分钟级聚合、请求分页、模型过滤、价格设置和重置功能
- 提供与参考插件一致的受保护 `GET/DELETE /usage` 接口

### 每次请求如何记录

1. CLIProxyAPI 调用插件的 `usage.handle`。
2. 插件直接将回调解码为官方 SDK 的 `pluginapi.UsageRecord`。
3. 为请求生成 UUID。
4. 使用 `RequestedAt` 作为请求时间；为空时使用插件收到回调的时间。
5. 时间统一转换为 UTC，以 RFC3339Nano 文本写入 SQLite；`Latency` 和 `TTFT` 转换为毫秒。
6. Token 计数负值按 0 写入；`TotalTokens=0` 时按输入、输出和推理 Token 推导。
7. 执行单条 SQLite `INSERT`。只有数据库提交成功后 `usage.handle` 才返回 `{"stored":true}`。
8. 空回调或无效 JSON 按参考实现返回 `{"ignored":true}`。

没有异步模式，也没有待刷盘队列。因此插件成功确认的请求已经写入数据库。

### 每请求保存的字段

SQLite 表 `usage_records` 每行保存：

- `id`、`timestamp`
- `api_key`、`provider`、`model`、`alias`
- `source`、`auth_id`、`auth_index`、`auth_type`、`executor_type`
- `reasoning_effort`、`service_tier`
- `latency_ms`、`ttft_ms`
- `input_tokens`、`output_tokens`、`reasoning_tokens`
- `cached_tokens`、`cache_read_tokens`、`cache_creation_tokens`、`total_tokens`
- `failed`、`failure_status_code`、`failure_body`

插件不会保存 prompt、生成结果正文或完整响应头。模型价格保存在同一 SQLite 数据库的 `plugin_settings` 表中。

### 隐私与接口边界

完整记录中包含 API Key 标识、认证 ID/索引和失败响应正文等敏感元数据，因此：

- 完整记录只通过 CLIProxyAPI 管理鉴权保护的 `/usage` 接口返回。
- 无管理鉴权的仪表盘 `/requests` 资源接口只返回脱敏视图，不包含 API Key、Auth ID、Auth Index 或 Failure Body。
- `/stats` 只返回聚合维度和计数。
- 插件不保存请求 prompt 或模型响应正文。

请仍然只在受信网络中暴露 CLIProxyAPI 的资源接口。

### 配置

将共享库放入 CLIProxyAPI 的平台插件目录，例如：

```text
plugins/linux/arm64/cap-token-usage-tracker.so
```

最小配置：

```yaml
plugins:
  enabled: true
  configs:
    cap-token-usage-tracker:
      enabled: true
```

完整配置：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      data_dir: /var/lib/cliproxyapi/cap-token-usage-tracker
      retention_days: 30
```

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `data_dir` | `~/.cli-proxy-api/plugins/cap-token-usage-tracker` | 数据目录；数据库文件固定为目录下的 `usage.db` |
| `retention_days` | `0` | 保留天数，范围 `0..3650`；`0` 禁用自动清理 |
| `data_path` | 无 | 旧版本兼容别名，可直接指定 SQLite 文件；不能与 `data_dir` 同时设置 |

环境变量优先级：

1. 配置项 `data_dir`
2. `CAP_TOKEN_USAGE_TRACKER_DIR`
3. 兼容变量 `USAGE_STATISTICS_DIR`
4. 默认目录

旧的 `flush_interval`、`flush_max_records` 和 `sync_on_record` 已移除，因为当前实现始终逐请求同步提交。

### 从旧 bbolt 版本升级

旧 bbolt 文件不会被覆盖或原地转换为 SQLite。若 `data_path` 指向非 SQLite 文件，插件会拒绝打开并返回明确错误。

升级时请：

1. 保留旧文件作为备份。
2. 配置新的 `data_dir`，或把 `data_path` 改为新的 SQLite 文件路径。
3. 重启 CLIProxyAPI，让插件创建 `usage.db`。

### 页面和接口

插件 ID 由共享库文件名决定。以 `cap-token-usage-tracker.so` 为例：

#### 只读资源接口

- 仪表盘：`/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- 聚合统计：`GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- 脱敏请求明细：`GET /v0/resource/plugins/cap-token-usage-tracker/requests?range=24h&offset=0&limit=100&model=gpt-4.1`
- 模型价格：`GET /v0/resource/plugins/cap-token-usage-tracker/prices`

统计范围支持 `24h`、`7d`、`30d`、`retention`。请求明细按时间倒序返回；`limit` 默认 100，最大 500。

#### 受保护的管理接口

- 完整逐请求用量：`GET /v0/management/plugins/cap-token-usage-tracker/usage`
- 删除指定请求：`DELETE /v0/management/plugins/cap-token-usage-tracker/usage`
- 聚合统计：`GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- 保存模型价格：`PUT /v0/management/plugins/cap-token-usage-tracker/prices`
- 重置全部请求记录：`POST /v0/management/plugins/cap-token-usage-tracker/reset`

`GET /usage` 可使用 RFC3339 时间边界；`start` 包含，`end` 不包含：

```text
GET /v0/management/plugins/cap-token-usage-tracker/usage?start=2026-07-01T00:00:00Z&end=2026-08-01T00:00:00Z
```

响应按 API Key（为空时按 Provider）和 Model 分组：

```json
{
  "client-key": {
    "gpt-4.1": [
      {
        "id": "0e4d4a14-79a0-44b0-96e8-5de60e7abbbc",
        "timestamp": "2026-07-15T08:00:00Z",
        "provider": "openai",
        "auth_id": "credential-id",
        "latency_ms": 1250,
        "ttft_ms": 180,
        "tokens": {
          "input_tokens": 100,
          "output_tokens": 25,
          "reasoning_tokens": 0,
          "cached_tokens": 0,
          "cache_read_tokens": 0,
          "cache_creation_tokens": 0,
          "total_tokens": 125
        },
        "failed": false
      }
    ]
  }
}
```

删除请求体：

```json
{"ids":["request-uuid-1","request-uuid-2"]}
```

重置请求体：

```json
{"confirm":"reset"}
```

### 诊断字段

`/stats` 响应保留原仪表盘兼容的 `diagnostics`：

| 字段 | 含义 |
|---|---|
| `callbacks_received` | 进入 `usage.handle` 的回调数 |
| `decoded` | 成功解码的回调数 |
| `enqueued` | 兼容字段；当前表示同步存储成功的回调数 |
| `processed` | 本次打开数据库后执行过的写入数 |
| `persisted_since_open` | 本次打开数据库后成功提交的写入数 |
| `decode_errors` / `enqueue_errors` | 解码错误 / SQLite 写入错误 |
| `mailbox_depth` / `pending_flush` | 兼容字段；同步 SQLite 实现中恒为 0 |

### 构建与测试

本地验证：

```bash
gofmt -w *.go
go vet ./...
CGO_ENABLED=0 go test ./...
go test ./...
go test -race ./...
```

Linux ARM64 构建：

```bash
bash scripts/build-linux-arm64.sh
bash scripts/verify-linux-arm64.sh
```

`modernc.org/sqlite` 是纯 Go SQLite 驱动；插件 ABI 的 `c-shared` 构建仍然需要 CGO 和目标平台交叉编译器。

---

## English

A persistent token-usage plugin for CLIProxyAPI. Its ingestion and storage path has been rebuilt to follow `Fwindy/cpa-usage-statistics`: **one SQLite row is synchronously inserted for every `usage.handle` callback, and the callback is acknowledged only after the insert succeeds**.

The existing dashboard, aggregate statistics, sanitized request list, and model-price features remain available on top of the per-request SQLite records.

### Highlights

- Pure-Go SQLite through `modernc.org/sqlite`; the database file is `usage.db`
- A UUID and UTC RFC3339Nano timestamp for every request
- No in-memory FIFO, store actor, or asynchronous batch flush
- Full metadata aligned with upstream `usage.Record` / `usage.Detail`
- Automatic missing-column repair when an existing SQLite database is opened
- `retention_days=0` disables cleanup; enabled cleanup runs at most once per hour
- Protected reference-compatible `GET/DELETE /usage` endpoints
- Existing dashboard statistics, request pagination, model filters, model prices, and reset support

### Per-request recording flow

1. CLIProxyAPI invokes `usage.handle`.
2. The callback is decoded directly into the official SDK `pluginapi.UsageRecord`.
3. The plugin generates a UUID.
4. `RequestedAt` is used as the request time, falling back to callback receipt time.
5. Time is stored in UTC RFC3339Nano form; latency and TTFT are stored in milliseconds.
6. Negative token values are written as zero. A zero total is derived from input, output, and reasoning tokens.
7. One SQLite `INSERT` is executed synchronously. `usage.handle` returns `{"stored":true}` only after the insert succeeds.
8. Empty or malformed callbacks return `{"ignored":true}`, matching the reference implementation.

### Persisted fields

Each `usage_records` row contains:

- `id`, `timestamp`
- `api_key`, `provider`, `model`, `alias`
- `source`, `auth_id`, `auth_index`, `auth_type`, `executor_type`
- `reasoning_effort`, `service_tier`
- `latency_ms`, `ttft_ms`
- all input/output/reasoning/cache/total token counters
- `failed`, `failure_status_code`, `failure_body`

Prompts, generated response bodies, and full response headers are not stored. Model prices are stored in the same SQLite database in `plugin_settings`.

### Privacy boundary

The complete records contain sensitive identifiers and failure text:

- The protected management `/usage` API exposes the full reference-compatible view.
- The unauthenticated dashboard `/requests` resource returns a sanitized view without API keys, auth identifiers/indexes, or failure bodies.
- `/stats` exposes only aggregate dimensions and counters.

Expose CLIProxyAPI resource endpoints only on trusted networks.

### Configuration

Minimal configuration:

```yaml
plugins:
  enabled: true
  configs:
    cap-token-usage-tracker:
      enabled: true
```

Full configuration:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      data_dir: /var/lib/cliproxyapi/cap-token-usage-tracker
      retention_days: 30
```

| Field | Default | Description |
|---|---:|---|
| `data_dir` | `~/.cli-proxy-api/plugins/cap-token-usage-tracker` | Directory containing the fixed `usage.db` file |
| `retention_days` | `0` | `0..3650`; zero disables automatic cleanup |
| `data_path` | none | Legacy compatibility alias for an explicit SQLite file; cannot be combined with `data_dir` |

Directory environment fallback order:

1. `data_dir`
2. `CAP_TOKEN_USAGE_TRACKER_DIR`
3. compatibility variable `USAGE_STATISTICS_DIR`
4. the default directory

`flush_interval`, `flush_max_records`, and `sync_on_record` have been removed because every request is now committed synchronously.

### Upgrading from the old bbolt format

Legacy bbolt files are not overwritten or migrated in place. If `data_path` points to a non-SQLite file, startup fails with an explicit error.

Keep the old file as a backup, configure a new `data_dir` or SQLite `data_path`, and restart CLIProxyAPI to create `usage.db`.

### Endpoints

With plugin ID `cap-token-usage-tracker`:

Read-only resources:

- `/v0/resource/plugins/cap-token-usage-tracker/dashboard`
- `GET /v0/resource/plugins/cap-token-usage-tracker/stats?range=24h`
- `GET /v0/resource/plugins/cap-token-usage-tracker/requests?range=24h&offset=0&limit=100&model=gpt-4.1`
- `GET /v0/resource/plugins/cap-token-usage-tracker/prices`

Protected management endpoints:

- `GET /v0/management/plugins/cap-token-usage-tracker/usage?start=<RFC3339>&end=<RFC3339>`
- `DELETE /v0/management/plugins/cap-token-usage-tracker/usage`
- `GET /v0/management/plugins/cap-token-usage-tracker/stats?range=24h`
- `PUT /v0/management/plugins/cap-token-usage-tracker/prices`
- `POST /v0/management/plugins/cap-token-usage-tracker/reset`

For `GET /usage`, `start` is inclusive and `end` is exclusive. Results are grouped first by API key (or provider when the key is empty), then by model.

Delete body:

```json
{"ids":["request-uuid-1","request-uuid-2"]}
```

Reset body:

```json
{"confirm":"reset"}
```

### Build and test

```bash
gofmt -w *.go
go vet ./...
CGO_ENABLED=0 go test ./...
go test ./...
go test -race ./...

bash scripts/build-linux-arm64.sh
bash scripts/verify-linux-arm64.sh
```

`modernc.org/sqlite` is pure Go. The plugin's `c-shared` ABI build still requires CGO and a target-platform cross compiler.

### License

[MIT License](LICENSE)
