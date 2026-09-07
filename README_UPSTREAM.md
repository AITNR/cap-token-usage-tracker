# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![访问计数](https://count.getloli.com/get/@cap-token-usage-tracker?theme=gelbooru)](https://github.com/journey-ad/Moe-Counter)

**[English](#english)** | [中文](#中文)

---

## 中文

CAP Token Usage Tracker 是 CLIProxyAPI 的持久化 Token 用量统计插件。它通过官方 `usage_plugin` 接收用量记录，将分钟级聚合、逐请求元数据、模型价格和仪表盘偏好保存到本地 bbolt 数据库，并通过 `management_api` 注册仪表盘与管理接口。

插件不保存 prompt、请求正文或模型响应正文。启用 API Key 跟踪时，API Key 只以加密密文保存，并且仅在经过鉴权的完整模式中尝试解密显示。

### 主要功能

- 按 UTC 分钟持久化聚合，同时保存逐请求用量元数据
- 按模型、提供商、执行器、别名、来源、认证类型、服务层级、推理强度和失败状态分组
- 统计请求数、失败数、输入/输出/推理/缓存 Token、延迟、TTFT、生成时间、TPS 和缓存命中率
- 支持今天、最近 5 小时、最近 7 天、最近 30 天、本月及自定义日期时间范围
- 趋势图支持分钟、小时、日、周、月聚合，以及滚轮缩放和平移
- 提供 Token 趋势、模型占比、费用趋势、模型效率和逐请求明细
- 支持来源、认证账号、模型和请求结果筛选
- 支持请求表和维度表分页、排序、列显示偏好持久化
- 完整模式支持多选 API Key 并按并集筛选、设置显示标签，并隔离不同加密密钥代际
- 支持 USD/CNY 汇率展示和总 Token 完整值、k、m、B（10 亿）单位切换
- 自动跟随 CLIProxyAPI Management Center 主题和浏览器语言
- 内置英文、简体中文、繁体中文和俄文
- 提供独立的普通模式和完整模式前端
- 支持 Linux amd64/arm64、Windows amd64 和 macOS amd64/arm64 `c-shared` 构建

### 普通模式与完整模式

普通模式是 Management Center 菜单默认打开的页面：

```text
/v0/resource/plugins/cap-token-usage-tracker/dashboard
```

普通模式可以查看当前项目已有的非敏感统计数据，包括概览、趋势、费用估算、维度统计和逐请求元数据。它保留筛选、时间范围、刷新、表格分页、排序和列设置等日常查看功能。

仪表盘会优先请求紧凑的首屏统计并立即渲染摘要、模型汇总和聚合趋势；逐模型趋势、维度表、逐请求明细、价格和费用随后异步加载。首屏的 `24h` 趋势按 5 分钟桶聚合，`7d` 按小时桶聚合；更长或自定义范围会自动选择足以控制点数的更粗粒度。

普通模式不显示以下入口和页面：

- 模型价格配置和 models.dev 价格同步
- CSV 和 Dashboard PNG 导出
- 数据库备份与恢复

点击普通模式顶部的“完整模式”按钮后，页面才显示管理密钥输入框。管理密钥通过 CLIProxyAPI Management API 鉴权成功后，插件签发一个随机、短期、仅保存在内存中的完整模式会话令牌，并导航到独立页面：

```text
/v0/resource/plugins/cap-token-usage-tracker/full-dashboard
```

完整模式与普通模式保持相同的主体布局和统计功能，并额外显示：

- 模型价格配置与保存
- CLIProxyAPI `/v1/models` 模型加载
- models.dev 价格同步
- 当前筛选数据 CSV 导出
- Dashboard PNG 导出
- bbolt 数据库备份与恢复
- API Key 明文查看、筛选、标签管理和密钥安全状态提示

完整模式会话默认有效期为 15 分钟，可通过 `full_mode_session_ttl_minutes` 配置为 1-1440 分钟。会话令牌仅保存在页面 JavaScript 内存中，通过 `X-Full-Mode-Session` 请求头发送；不会写入数据库、日志、`localStorage` 或 `sessionStorage`。刷新或关闭页面、重启插件或 CLIProxyAPI 后需要重新鉴权；退出完整模式时会主动撤销会话。页面导航时使用 URL fragment 临时传递令牌，加载后立即从地址栏清除。管理密钥不作为后续操作的鉴权凭据保存在前端，价格、导出、备份和恢复直接使用当前内存中的会话令牌。

完整模式 HTML 本身不嵌入受保护数据。API Key 明文、标签和密钥安全状态由带 `X-Full-Mode-Session` 鉴权的资源接口按需返回，不能写进普通模式 HTML、普通资源响应或前端静态脚本。仅通过 CSS 隐藏元素不能保护敏感数据。

普通模式和完整模式共享统计数据源，但普通模式会删除 API Key 明文、引用、指纹、加密代际和解密状态。完整模式会根据当前配置的 `api_key_secret` 逐项解密；无法解密的历史密文显示“明文不可用”，不会影响其他统计数据。

`/stats/initial` 与 `/stats/groups` 和旧版 `/stats` 一样执行脱敏：普通模式响应不含 API Key 集合或任何维度中的 API Key 字段；`/stats/trends` 只包含时间、模型和计数器。完整模式可重复传入 `api_key_ref`，服务端按这些 API Key 的并集筛选；不传该参数即恢复全量。`api_key_ref` 与兼容的单值 `api_key_hash` 筛选在所有统计、趋势、维度、请求和费用接口中都必须携带有效的 `X-Full-Mode-Session`，未授权请求会返回 `403`，不能通过查询参数绕过完整模式鉴权。

### 隐私与安全边界

插件不会持久化：

- API Key 明文
- Auth ID 或 Auth Index 原始值
- prompt、请求正文或模型响应正文
- 失败响应正文和响应头

数据库会保存：

- 分钟级聚合维度和计数
- 逐请求时间、模型、来源、服务层级、结果、延迟、推理强度和 Token 计数
- API Key 加密密文、带密钥指纹、加密代际和用户设置的显示标签
- 经过清理的认证账号显示信息
- 模型价格、Context Tier、服务层级价格和同步元数据
- 仪表盘时间范围、分页大小和隐藏列偏好

来源字段会进行凭据清理。疑似 API Key、Bearer Token 或其他凭据形式的来源不会按原值保存；插件会尽量回退到规范化的提供商服务地址。

API Key 跟踪默认使用公开密钥 `123456`。该默认值只能提供误显示防护，任何获得数据库或备份的人都可以使用它解密其中保存的 API Key；完整模式会持续显示安全警告。生产环境应配置至少 32 字节的自定义 `api_key_secret`。成功应用自定义密钥后，警告会在重新打开完整模式、手动刷新或下一次 15 秒自动刷新时消失。

更换 `api_key_secret` 不会删除数据库或历史统计，而是创建或激活对应的加密代际。当前密钥无法解密的旧代 API Key 显示“明文不可用”；切回对应旧密钥后可以再次读取。将 `api_key_secret` 设为空字符串会禁用 API Key 跟踪，之后收到的记录不会保存 API Key 密文或指纹。

普通模式统计资源无需再次输入管理密钥，因此任何能访问 CLIProxyAPI Management Center 的浏览器都可以读取这些非敏感统计数据。不要将 Management Center 直接暴露到不受信任网络。完整模式入口由管理密钥保护，但它不能替代 TLS、网络访问控制和宿主 Management API 安全配置。

### 安装与配置

将目标平台的共享库放入 CLIProxyAPI 对应目录。文件名必须保持为 `cap-token-usage-tracker`，CLIProxyAPI 会根据共享库文件名派生 plugin ID。

| 平台 | 安装路径 |
|---|---|
| Linux amd64 | `plugins/linux/amd64/cap-token-usage-tracker.so` |
| Linux arm64 | `plugins/linux/arm64/cap-token-usage-tracker.so` |
| Windows amd64 | `plugins/windows/amd64/cap-token-usage-tracker.dll` |
| macOS amd64 (Intel) | `plugins/darwin/amd64/cap-token-usage-tracker.dylib` |
| macOS arm64 | `plugins/darwin/arm64/cap-token-usage-tracker.dylib` |

macOS Intel 使用 `darwin/amd64` 插件，Apple Silicon 原生运行使用 `darwin/arm64` 插件；插件架构必须与 CLIProxyAPI 进程架构一致。若 CLIProxyAPI 在 Apple Silicon 上通过 Rosetta 以 x86_64 运行，则使用 `darwin/amd64` 插件。

CLIProxyAPI 配置示例：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      priority: 0
      retention_days: 365
      flush_interval: 5s
      flush_max_records: 100
      sync_on_record: true
      api_key_secret: "replace-with-a-random-secret-at-least-32-bytes"
      response_compression: true
      response_compression_min_bytes: 1024
      full_mode_session_ttl_minutes: 15
```

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `data_path` | `CLIProxyAPI/data/token-usage-tracker.db` | bbolt 数据库路径；显式相对路径以 CLIProxyAPI 进程工作目录为基准 |
| `retention_days` | `365` | 统计和逐请求明细保留天数，范围 1-3650 |
| `flush_interval` | `5s` | 批量模式最长刷盘间隔，范围 1 秒-1 小时 |
| `flush_max_records` | `100` | 批量模式达到该记录数时立即刷盘，范围 1-1000000 |
| `sync_on_record` | `true` | 每条记录提交数据库后再确认；设为 `false` 时启用批量模式 |
| `api_key_secret` | `123456` | API Key 加密和带密钥指纹使用的密钥；自定义值至少 32 字节，空字符串禁用 API Key 跟踪 |
| `response_compression` | `true` | 客户端支持 gzip 时压缩公共仪表盘 HTML 和 JSON 响应；管理接口保持未压缩 |
| `response_compression_min_bytes` | `1024` | 启用 gzip 的最小响应字节数，范围 0-16777216 |
| `full_mode_session_ttl_minutes` | `15` | 完整模式会话有效期（分钟），范围 1-1440；令牌只保存在页面内存，刷新、关闭页面或重启插件后需要重新鉴权 |

示例中的 `api_key_secret` 只是占位符，部署时必须替换。含 `#`、`:`、`{}` 等特殊字符的值应使用 YAML 引号包裹；长度按 UTF-8 字节计算。该密钥会保存在 CLIProxyAPI 配置中，因此应限制配置文件权限，避免提交到公开仓库，也不要与数据库备份一起分发。

默认 `sync_on_record: true` 优先保证记录持久化。设为 `false` 可以减少写入次数，但进程被强制终止时，最多可能丢失一个 `flush_interval` 或尚未达到 `flush_max_records` 的窗口。

默认 `response_compression: true` 通过标准 `Accept-Encoding` 协商启用 gzip，因此直接访问 CLIProxyAPI IP 和端口的现代浏览器也能获得压缩响应。不支持 gzip 或显式发送 `gzip;q=0` 的客户端仍会收到原始响应；二进制备份、已编码响应和 `/v0/management/` 接口不会由插件压缩。

未配置 `data_path` 时，插件按以下顺序定位数据库：

1. 从已加载共享库路径向上查找 `plugins` 目录
2. 检查 CLIProxyAPI 可执行文件同级的 `plugins` 目录
3. 检查当前工作目录下的 `plugins` 目录
4. 无法识别时回退到 `./data/token-usage-tracker.db`

### 仪表盘操作

普通模式和完整模式都支持：

- 选择时间预设或自定义起止日期与时间
- 按来源和认证账号筛选
- 切换趋势聚合粒度并缩放或平移趋势图
- 点击模型图表下钻，再次点击清除模型筛选
- 切换 Token 显示单位（完整值、k、m、B，其中 B = 1,000,000,000）和 USD/CNY
- 调整逐请求表和维度表的可见列、排序和分页大小
- 手动刷新；页面默认每 15 秒自动刷新

重置统计入口只在完整模式可用，需要当前完整模式会话和显式确认。

表格偏好和时间范围保存在插件数据库中。自定义时间按浏览器本地时区选择，再转换为 UTC RFC3339 时间戳请求。

### 模型价格与费用估算

模型价格入口只在完整模式显示。所有价格单位均为每 100 万 Token 的美元价格，支持 Input、Output、Cache Read、Cache Creation、Context Tier、Service Tier 独立价格及其 Context Tier，以及 `input_excludes_cache` 和 `input_includes_cache` 两种计费方式。所有价格为 0 的模型按免费模型处理。

价格可手工维护，也可从 models.dev 同步。同步先读取 CLIProxyAPI `/v1/models` 当前返回的模型，再根据提供商优先级、忽略后缀和显式模型映射匹配 models.dev。

手工价格优先，不会被同步覆盖。价格簿使用 revision 防止并发覆盖。费用根据逐请求记录和匹配的价格规则计算；缺价请求会显示在价格覆盖率和缺价提示中，不会作为零成本混入已知费用。

### 导出、备份与恢复

以下功能只在完整模式可用，并在执行时校验当前会话：

- 导出当前筛选数据为 CSV
- 将当前 Dashboard 导出为 PNG
- 下载完整 bbolt 数据库备份
- 从备份文件恢复数据库
- 重置统计数据（需通过完整模式会话鉴权）

备份文件最大为 64 MiB。恢复会替换当前数据库，需要用户确认，并在服务端校验 `X-Confirm-Restore: replace`。完整模式通过分段上传传输恢复数据；上传暂存使用独立的短时固定有效期，不随完整模式会话有效期配置变化。

直接调用 CLIProxyAPI Management API 时，仍可使用管理密钥访问备份、恢复、价格保存、价格同步和重置路由。

### 页面与接口

以下路径以 plugin ID `cap-token-usage-tracker` 为例。

普通资源：

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/dashboard` | 普通模式页面 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats` | 兼容客户端的完整聚合统计 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats/initial` | 首屏摘要、紧凑模型汇总和聚合趋势 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats/trends` | 下采样后的逐模型趋势，供首屏后异步加载 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats/groups` | 服务端排序和分页的详细维度统计 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/requests` | 分页逐请求明细 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/costs` | 基于逐请求记录计算的费用统计 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/exchange-rate` | 缓存的 USD/CNY 汇率 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/prices` | 读取当前价格簿，用于费用展示 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/preferences` | 读取或保存仪表盘偏好 |

完整模式资源：

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-dashboard` | 独立完整模式页面壳 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/data` | 校验会话并返回受保护数据 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/api-key-labels` | 通过 `X-API-Key-Label` JSON 请求头保存或删除 API Key 显示标签 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/session/revoke` | 撤销当前会话 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/prices` | 读取受保护价格配置 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/prices/save` | 分段保存价格配置 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/prices/sync` | 分段提交 models.dev 同步请求 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/backup` | 下载数据库备份 |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/restore` | 分段上传并恢复数据库 |
| `POST` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/reset` | 校验会话后重置统计 |

除页面壳外，完整模式资源均要求：

```http
X-Full-Mode-Session: <session-token>
```

受 CLIProxyAPI Management API 鉴权的路由：

| 方法 | 路径 | 用途 |
|---|---|---|
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/full-mode/session` | 签发完整模式会话 |
| `GET` | `/v0/management/plugins/cap-token-usage-tracker/stats` | 读取聚合统计 |
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/reset` | 重置统计 |
| `PUT` | `/v0/management/plugins/cap-token-usage-tracker/prices` | 保存模型价格 |
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/prices/sync` | 同步 models.dev 价格 |
| `GET` | `/v0/management/plugins/cap-token-usage-tracker/backup` | 下载数据库备份 |
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/restore` | 恢复数据库 |

统计、逐请求和费用接口支持 `range`，或 `start` 与 `end`，以及 `source` 等筛选参数。完整模式还支持重复的 `api_key_ref`，多个值按并集筛选；逐请求接口另支持 `offset`、`limit`、`model` 和 `result`。`/stats/groups` 另支持 `offset`、`limit`、`sort`、`direction`、`model` 和重复的 `exclude_model`；每页最多 500 条。

重置请求正文：

```json
{"confirm":"reset"}
```

恢复请求需要：

```http
Content-Type: application/octet-stream
X-Confirm-Restore: replace
```

### 构建与开发

要求 Go 1.26+、`CGO_ENABLED=1`。Windows amd64 需要 MinGW-w64；Linux arm64 交叉构建需要 `aarch64-linux-gnu-gcc`。插件支持 CLIProxyAPI RPC schema 1-3 和原生 ABI 1；宿主声明更高 schema 时会协商到 schema 3。

```bash
# Linux amd64
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.so .

# Linux arm64
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.so .

# macOS amd64 (Intel)
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.dylib .

# macOS arm64
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.dylib .
```

Windows PowerShell：

```powershell
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
go build -buildmode=c-shared -trimpath -buildvcs=false `
  -ldflags="-s -w -X main.version=1.0.0" `
  -o cap-token-usage-tracker.dll .
```

`build_dll.ps1` 包含当前工作区固定的 MinGW 和路径设置，在其他机器使用前需要调整。仓库还提供 Linux ARM64 构建/验证脚本以及 macOS amd64/arm64 验证脚本。

本地验证：

```bash
gofmt -w *.go
go test -count=1 ./...
go vet ./...
```

浏览器日期范围测试在缺少 Node.js、Chrome 或 `playwright-core` 时会跳过。需要本地运行它们时执行 `npm ci`；需要把依赖缺失视为失败时执行：

```bash
REQUIRE_BROWSER_TESTS=1 CHROME_PATH=/path/to/google-chrome go test -count=1 ./...
```

发布前必须执行目标平台的 `c-shared` 构建。GitHub Actions 构建五个平台；分支推送发布 `-alpha.<run number>` 测试版，`v*` 标签或手动稳定发布创建正式 Release。

### 协议

[MIT License](LICENSE)

---

## English

CAP Token Usage Tracker is a persistent token-usage statistics plugin for CLIProxyAPI. It receives usage records through the official `usage_plugin`, stores minute-level aggregates, per-request metadata, model prices, and dashboard preferences in a local bbolt database, and registers dashboard and management endpoints through `management_api`.

The plugin does not store prompts, request bodies, or model response bodies. When API-key tracking is enabled, API keys are persisted only as encrypted ciphertext and are revealed only when possible in authenticated full mode.

### Features

- Persistent aggregation by UTC minute with per-request usage metadata
- Grouping by model, provider, executor, alias, source, auth type, service tier, reasoning effort, and failure status
- Request, failure, input/output/reasoning/cache token, latency, TTFT, generation-time, TPS, and cache-hit statistics
- Today, last 5 hours, last 7 days, last 30 days, current month, and custom local date-time ranges
- Minute, hour, day, week, and month trend aggregation with wheel zoom and pan
- Token trends, model share, cost trends, model efficiency, and paginated request details
- Source, model, and request-result filters
- Persistent table pagination, sorting, and column visibility preferences
- Full-mode API-key multi-selection with union filtering, display labels, and isolation between encryption-key generations
- USD/CNY display and full, k, m, or B (one billion) total-token units
- Automatic Management Center theme and browser-language synchronization
- Built-in English, Simplified Chinese, Traditional Chinese, and Russian locales
- Separate normal-mode and full-mode frontends
- Linux amd64/arm64, Windows amd64, and macOS amd64/arm64 `c-shared` builds

### Normal Mode and Full Mode

Normal mode is the default Management Center page:

```text
/v0/resource/plugins/cap-token-usage-tracker/dashboard
```

It displays the project's current non-sensitive statistics, including summaries, trends, cost estimates, grouped dimensions, and per-request metadata. Filters, date ranges, refresh, pagination, sorting, and column settings remain available.

The dashboard requests compact first-screen statistics and renders the summary, model totals, and aggregate trend first. Per-model trends, grouped dimensions, request details, prices, and costs load asynchronously afterwards. The first-screen trend uses five-minute buckets for `24h`, hourly buckets for `7d`, and automatically chooses coarser buckets for longer or custom ranges to keep the point count bounded.

Normal mode does not expose model-price configuration, models.dev synchronization, CSV or Dashboard PNG export, or database backup and restore.

The management-key dialog appears only after the user clicks Full Mode. After CLIProxyAPI Management API authentication succeeds, the plugin issues a random short-lived in-memory capability and navigates to:

```text
/v0/resource/plugins/cap-token-usage-tracker/full-dashboard
```

Full mode keeps the same dashboard layout and statistics while adding:

- Model-price editing and persistence
- Model loading from CLIProxyAPI `/v1/models`
- models.dev synchronization
- Filtered CSV and Dashboard PNG export
- bbolt database backup and restore
- API-key reveal, filtering, label management, and secret-security status

The full-mode session defaults to 15 minutes and can be configured from 1 to 1440 minutes with `full_mode_session_ttl_minutes`. The capability is held only in page JavaScript memory and sent in the `X-Full-Mode-Session` header; it is never written to the database, logs, `localStorage`, or `sessionStorage`. Refreshing or closing the page, or restarting the plugin or CLIProxyAPI, requires authentication again; exiting Full Mode revokes the session. Navigation temporarily carries it in the URL fragment, which is removed immediately after page initialization. The management key is not retained for later operations; pricing, export, backup, and restore use the current in-memory capability.

The full-mode HTML does not embed protected data. API-key plaintext, labels, and secret-security status are returned on demand only by capability-protected resource endpoints. They are not included in normal-mode HTML, normal resource responses, or static frontend scripts. CSS visibility is not a security boundary.

Normal and full modes share the same statistics source, but normal mode removes API-key plaintext, references, fingerprints, encryption generations, and reveal statuses. Full mode attempts item-by-item decryption with the configured `api_key_secret`; historical ciphertext that cannot be decrypted is shown as "Plaintext unavailable" without affecting other statistics.

Like the legacy `/stats` resource, `/stats/initial` and `/stats/groups` apply redaction: normal-mode responses contain neither an API-key collection nor API-key fields in dimension rows. `/stats/trends` contains only timestamps, model names, and counters. Full mode accepts repeated `api_key_ref` values and filters by their union; omitting the parameter restores the full data set. The `api_key_ref` and compatible single-value `api_key_hash` filters require a valid `X-Full-Mode-Session` on every statistics, trend, group, request, and cost endpoint. Requests without that capability receive `403`; query parameters cannot bypass full-mode authorization.

### Privacy and Security Boundary

The plugin does not persist:

- Plaintext API keys
- Raw Auth ID or Auth Index values
- Prompts, request bodies, or model response bodies
- Failure response bodies or response headers

The database contains minute-level aggregates, per-request operational metadata, encrypted API-key ciphertext, keyed fingerprints, encryption-generation metadata, user-defined API-key labels, sanitized source display data, model pricing and synchronization metadata, and dashboard preferences.

Source fields are credential-sanitized. Values that resemble API keys, bearer tokens, or other credentials are not persisted verbatim; the plugin falls back to a normalized provider service address when possible.

API-key tracking defaults to the public secret `123456`. This default only prevents accidental display: anyone who obtains the database or a backup can use it to decrypt stored API keys, so full mode continuously shows a security warning. Production deployments should configure a custom `api_key_secret` of at least 32 bytes. After the custom secret is successfully applied, the warning disappears when full mode is reopened, manually refreshed, or automatically refreshed within 15 seconds.

Changing `api_key_secret` does not delete the database or historical statistics. It creates or activates the matching crypto generation. API keys from generations unavailable under the current secret are shown as "Plaintext unavailable" and become readable again after switching back to the matching older secret. Setting `api_key_secret` to an empty string disables API-key tracking for subsequently received records.

Normal-mode statistics resources do not ask for the management key again, so any browser that can access the CLIProxyAPI Management Center can read these non-sensitive statistics. Do not expose the Management Center directly to untrusted networks. Full-mode entry is protected by the management key, but this does not replace TLS, network access controls, or secure host Management API configuration.

### Installation and Configuration

Place the shared library in the matching CLIProxyAPI plugin directory. Keep the base filename `cap-token-usage-tracker`, because CLIProxyAPI derives the plugin ID from it.

| Platform | Install path |
|---|---|
| Linux amd64 | `plugins/linux/amd64/cap-token-usage-tracker.so` |
| Linux arm64 | `plugins/linux/arm64/cap-token-usage-tracker.so` |
| Windows amd64 | `plugins/windows/amd64/cap-token-usage-tracker.dll` |
| macOS amd64 (Intel) | `plugins/darwin/amd64/cap-token-usage-tracker.dylib` |
| macOS arm64 | `plugins/darwin/arm64/cap-token-usage-tracker.dylib` |

Use `darwin/amd64` on Intel Macs and `darwin/arm64` when CLIProxyAPI runs natively on Apple Silicon. The plugin architecture must match the CLIProxyAPI process architecture. Use `darwin/amd64` when CLIProxyAPI runs as x86_64 through Rosetta on Apple Silicon.

CLIProxyAPI configuration example:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      priority: 0
      retention_days: 365
      flush_interval: 5s
      flush_max_records: 100
      sync_on_record: true
      api_key_secret: "replace-with-a-random-secret-at-least-32-bytes"
      response_compression: true
      response_compression_min_bytes: 1024
      full_mode_session_ttl_minutes: 15
```

| Field | Default | Description |
|---|---:|---|
| `data_path` | `CLIProxyAPI/data/token-usage-tracker.db` | bbolt database path; explicit relative paths use the CLIProxyAPI working directory |
| `retention_days` | `365` | Statistics and request-detail retention, from 1 to 3650 days |
| `flush_interval` | `5s` | Maximum batch-mode flush interval, from 1 second to 1 hour |
| `flush_max_records` | `100` | Flush after this many batched records, from 1 to 1000000 |
| `sync_on_record` | `true` | Commit each record before acknowledgment; set to `false` for batch mode |
| `api_key_secret` | `123456` | Secret used for API-key encryption and keyed fingerprints; custom values must be at least 32 bytes, and an empty string disables API-key tracking |
| `response_compression` | `true` | Compress public dashboard HTML and JSON responses when the client supports gzip; management endpoints remain uncompressed |
| `response_compression_min_bytes` | `1024` | Minimum response size in bytes before gzip is used, from 0 to 16777216 |
| `full_mode_session_ttl_minutes` | `15` | Full-mode session lifetime in minutes, from 1 to 1440. The token stays only in page memory; refreshing, closing the page, or restarting the plugin requires authentication again |

The `api_key_secret` in the example is a placeholder and must be replaced for deployment. Quote YAML values containing special characters such as `#`, `:`, or `{}`; the minimum length is measured in UTF-8 bytes. The secret is stored in CLIProxyAPI configuration, so restrict access to that file, do not commit it to a public repository, and do not distribute it with database backups.

The default `sync_on_record: true` prioritizes durability. With batch mode enabled, a forced process termination may lose up to one `flush_interval` or the records below the `flush_max_records` threshold.

The default `response_compression: true` negotiates gzip through the standard `Accept-Encoding` header, so modern browsers connecting directly to the CLIProxyAPI IP and port also receive compressed responses. Clients that do not support gzip or explicitly send `gzip;q=0` still receive the original response. Binary backups, already encoded responses, and `/v0/management/` endpoints are not compressed by the plugin.

Without an explicit `data_path`, the plugin resolves the database in this order:

1. Walk upward from the loaded shared-library path to find `plugins`
2. Check for `plugins` next to the CLIProxyAPI executable
3. Check for `plugins` in the current working directory
4. Fall back to `./data/token-usage-tracker.db`

### Dashboard Operations

Both modes support preset or custom date-time ranges, source filtering, trend granularity and zoom, model drill-down, full, k, m, or B token units (`B = 1,000,000,000`), currency units, table columns and sorting, manual refresh, 15-second automatic refresh, and preset/custom table page sizes. Statistics reset is available only in full mode and requires the active session plus explicit confirmation.

Table preferences and the selected range are stored in the plugin database. Custom browser-local times are converted to UTC RFC3339 timestamps for requests.

### Model Pricing and Cost Estimation

The model-price UI is available only in full mode. Prices are USD per one million tokens and support Input, Output, Cache Read, Cache Creation, context tiers, service-tier-specific pricing, and the `input_excludes_cache` and `input_includes_cache` accounting modes. Models with all rates set to zero are treated as free.

Prices can be maintained manually or synchronized from models.dev. Synchronization first reads the model list currently returned by CLIProxyAPI `/v1/models`, then matches models.dev using provider priority, ignored suffixes, and explicit model mappings.

Manual entries take precedence and are not overwritten by synchronization. The price book uses a revision to prevent concurrent overwrite. Costs are calculated from individual request records and the matching pricing rule. Requests without a matching price are reported as missing-price coverage rather than silently treated as free.

### Export, Backup, and Restore

CSV export, Dashboard PNG export, database backup, database restore, and statistics reset are available only in full mode and validate the active session when executed.

Backup files are limited to 64 MiB. Restore replaces the current database, requires user confirmation, and is checked server-side with `X-Confirm-Restore: replace`. Full mode uses staged uploads for restore payloads; upload staging has its own short fixed lifetime and does not follow the full-mode session lifetime configuration.

Management-key-protected CLIProxyAPI Management API routes remain available for direct backup, restore, price persistence, price synchronization, and reset operations.

### Pages and Endpoints

The following examples use plugin ID `cap-token-usage-tracker`.

Normal resources:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/dashboard` | Normal-mode page |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats` | Complete aggregates for compatible clients |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats/initial` | First-screen summary, compact model totals, and aggregate trend |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats/trends` | Downsampled per-model trends loaded after first paint |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/stats/groups` | Server-sorted and paginated detailed dimension statistics |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/requests` | Paginated per-request details |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/costs` | Per-request-derived cost statistics |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/exchange-rate` | Cached USD/CNY exchange rate |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/prices` | Current price book for cost display |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/preferences` | Read or persist dashboard preferences |

Full-mode resources:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-dashboard` | Separate full-mode page shell |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/data` | Validate the session and return protected data |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/api-key-labels` | Save or delete an API-key display label with an `X-API-Key-Label` JSON request header |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/session/revoke` | Revoke the active session |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/prices` | Read protected pricing configuration |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/prices/save` | Persist pricing through a staged payload |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/prices/sync` | Synchronize models.dev through a staged payload |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/backup` | Download a database backup |
| `GET` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/restore` | Upload and restore a backup in stages |
| `POST` | `/v0/resource/plugins/cap-token-usage-tracker/full-mode/reset` | Reset statistics after validating the session |

All full-mode resources except the page shell require:

```http
X-Full-Mode-Session: <session-token>
```

Management API routes:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/full-mode/session` | Issue a session after management authentication |
| `GET` | `/v0/management/plugins/cap-token-usage-tracker/stats` | Read aggregate statistics |
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/reset` | Reset statistics |
| `PUT` | `/v0/management/plugins/cap-token-usage-tracker/prices` | Persist model prices |
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/prices/sync` | Synchronize models.dev prices |
| `GET` | `/v0/management/plugins/cap-token-usage-tracker/backup` | Download a database backup |
| `POST` | `/v0/management/plugins/cap-token-usage-tracker/restore` | Restore the database |

Statistics, request, and cost resources accept `range`, or `start` and `end`, plus filters such as `source`. Full mode also accepts repeated `api_key_ref` values and applies their union. The request resource additionally accepts `offset`, `limit`, `model`, and `result`. `/stats/groups` additionally accepts `offset`, `limit`, `sort`, `direction`, `model`, and repeated `exclude_model`; pages are limited to 500 rows.

Reset body:

```json
{"confirm":"reset"}
```

Restore headers:

```http
Content-Type: application/octet-stream
X-Confirm-Restore: replace
```

### Build and Development

Go 1.26+ and `CGO_ENABLED=1` are required. Windows amd64 requires MinGW-w64; Linux arm64 cross-compilation requires `aarch64-linux-gnu-gcc`. The plugin supports CLIProxyAPI RPC schemas 1-3 and native ABI 1; newer host schemas negotiate down to schema 3.

```bash
# Linux amd64
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.so .

# Linux arm64
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.so .

# macOS amd64 (Intel)
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.dylib .

# macOS arm64
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X main.version=1.0.0" -o cap-token-usage-tracker.dylib .
```

Windows PowerShell:

```powershell
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
go build -buildmode=c-shared -trimpath -buildvcs=false `
  -ldflags="-s -w -X main.version=1.0.0" `
  -o cap-token-usage-tracker.dll .
```

`build_dll.ps1` contains workspace-specific MinGW and directory paths and must be adjusted for other machines. The repository also includes Linux ARM64 build/verification scripts and macOS amd64/arm64 verification scripts.

Local verification:

```bash
gofmt -w *.go
go test -count=1 ./...
go vet ./...
```

Browser date-range tests skip when Node.js, Chrome, or `playwright-core` is unavailable. Run `npm ci` to include them locally, or require them explicitly:

```bash
REQUIRE_BROWSER_TESTS=1 CHROME_PATH=/path/to/google-chrome go test -count=1 ./...
```

Run an actual target-platform `c-shared` build before release. GitHub Actions builds all five targets; branch pushes publish `-alpha.<run number>` prereleases, while `v*` tags or manual stable releases publish normal releases.

### License

[MIT License](LICENSE)
