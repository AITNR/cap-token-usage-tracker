# CAP Token Usage Tracker

[English](#english) | **中文**

一个 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 插件，用于实时追踪和可视化所有模型与提供商的 LLM Token 用量。

## 简介

CAP Token Usage Tracker 是一个通过 CGO 编译的 C 共享库（`.dll` / `.so` / `.dylib`），作为插件接入 CLIProxyAPI 宿主。它会拦截代理处理的每一个 API 请求，聚合 Token 级别的统计数据 —— 输入、输出、推理、缓存、缓存读取/创建 —— 并通过 JSON API 和内置 Web 仪表盘进行展示。

### 核心功能

- **实时追踪** — 捕获每次请求的 Token 用量，按模型 + 提供商分组
- **全面指标** — 输入/输出/推理/缓存 Token、缓存读取与创建、请求数、成功率、延迟和首 Token 时间（TTFT）
- **内置仪表盘** — 自包含的 HTML 仪表盘，支持亮色/暗色双主题切换，每 5 秒自动刷新，无需外部依赖
- **JSON API** — 机器可读的 `/stats` 端点，便于与外部工具集成
- **重置 API** — POST `/reset` 清零所有计数器
- **线程安全** — 互斥锁保护的数据存储，可安全处理并发代理流量
- **跨平台** — 预编译支持 Linux (amd64/arm64)、Windows (amd64) 和 macOS (amd64/arm64)

### 部署方式

1. 下载或编译对应平台的共享库文件
2. 将 `.dll` / `.so` / `.dylib` 文件放入 CLIProxyAPI 的插件目录
3. 重启 CLIProxyAPI — 插件将自动注册

### 访问地址

| 类型 | 路径 | 说明 |
|---|---|---|
| 管理接口 | `/v0/management/plugins/token-usage-tracker/stats` | 查看用量统计 (JSON) |
| 管理接口 | `/v0/management/plugins/token-usage-tracker/reset` | 重置所有计数器 |
| 资源页面 | `/v0/resource/plugins/token-usage-tracker/dashboard` | 实时仪表盘 |

### 架构

```
┌─────────────────────────────────────────────────────────┐
│                    CLIProxyAPI 宿主                       │
│                                                          │
│  ┌──────────────┐    ┌─────────────────────────────────┐│
│  │  代理核心     │───▶│   Token 用量追踪插件              ││
│  │ (请求路由)    │    │                                 ││
│  └──────────────┘    │  ┌─────────────┐                ││
│                      │  │  Tracker    │ (内存存储)       ││
│  ┌──────────────┐    │  │ (互斥锁安全) │                ││
│  │ 插件 RPC     │◀───┴─────────┤                       ││
│  │ (ABI v7)     │               │                       ││
│  └──────┬───────┘               │                       ││
│         │              ┌────────┴────────┐              ││
│         │              │                  │              ││
│         │     ┌────────▼───┐    ┌─────────▼────────┐    ││
│         │     │ 管理路由   │    │   资源路由         │    ││
│         │     │ /stats     │    │   /dashboard       │    ││
│         │     │ /reset     │    │  (HTML 仪表盘)    │    ││
│         │     └────────────┘    └───────────────────┘    ││
└─────────┼────────────────────────────────────────────────┘
          │
          ▼
   HTTP 响应 (JSON / HTML)
```

### 插件能力

| 能力 | 说明 |
|---|---|
| `usage_plugin` | 接收每个代理请求的 `UsageRecord` |
| `management_api` | 注册管理路由（`/stats`、`/reset`）和资源路由（`/dashboard`） |

### 追踪指标

| 指标 | 说明 |
|---|---|
| `requests` | 总请求数 |
| `failed_requests` | 失败请求数 |
| `input_tokens` | 总输入（提示）Token |
| `output_tokens` | 总输出（补全）Token |
| `reasoning_tokens` | 推理/思考 Token（如 o1/o3） |
| `cached_tokens` | 缓存 Token（已弃用，缓存读取 + 创建之和） |
| `cache_read_tokens` | 缓存读取 Token |
| `cache_creation_tokens` | 缓存创建 Token |
| `total_tokens` | 所有 Token 类型之和 |
| `avg_latency` | 平均请求延迟 |
| `avg_ttft` | 平均首 Token 时间 |

### 构建

**前置要求：**

- **Go 1.26+**
- **GCC**（CGO 必需）
  - Windows：安装 [MinGW-w64](https://github.com/brechtsanders/winlibs_mingw) 并将 `bin` 添加到 PATH
  - Linux：`sudo apt install gcc`（ARM64 交叉编译用 `gcc-aarch64-linux-gnu`）
  - macOS：Xcode 命令行工具（`xcode-select --install`）

**本地构建：**

```bash
# 设置 CGO 并确保 gcc 在 PATH 中
CGO_ENABLED=1 go build -buildmode=c-shared -o token-usage-tracker.dll .
```

Windows (PowerShell)：

```powershell
$env:PATH = "C:\Program Files\Go\bin;C:\mingw64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = 1
go build -buildmode=c-shared -o token-usage-tracker.dll .
```

**CI/CD：** GitHub Actions 工作流（`.github/workflows/build.yml`）在推送时自动构建所有支持平台，并在版本标签（`v*`）时创建发布。

### 项目结构

```
cap-token-usage-tracker/
├── main.go          # 插件入口、ABI 绑定、RPC 分发
├── tracker.go       # 线程安全的内存用量数据存储
├── management.go    # 管理 API 处理器 (/stats, /reset)
├── dashboard.go     # HTML 仪表盘渲染（内嵌 CSS/JS）
├── go.mod           # 模块定义（依赖 CLIProxyAPI v7 SDK）
├── .github/
│   └── workflows/
│       ├── build.yml               # 多平台 CI 构建
│       └── qodana_code_quality.yml  # 代码质量检查
└── qodana.yaml      # Qodana 配置
```

### 技术栈

- **语言：** Go 1.26
- **宿主 SDK：** [CLIProxyAPI v7](https://github.com/router-for-me/CLIProxyAPI) 插件 SDK
- **构建模式：** `c-shared`（CGO 编译动态库）
- **CI/CD：** GitHub Actions（5 平台构建矩阵）
- **代码质量：** Qodana

### 许可证

本项目属于 [router-for-me](https://github.com/router-for-me) 生态系统。

---

<a id="english"></a>

## English

A [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) plugin that tracks and visualizes LLM token usage across all models and providers in real time.

CAP Token Usage Tracker is a CGO-compiled C shared library (`.dll` / `.so` / `.dylib`) that plugs into the CLIProxyAPI host. It intercepts every API request, aggregates token-level statistics, and exposes them through a JSON API and a built-in web dashboard.

For full documentation, please refer to the Chinese section above.
