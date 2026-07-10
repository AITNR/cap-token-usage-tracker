# CAP Token Usage Tracker

A [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) plugin that tracks and visualizes LLM token usage across all models and providers in real time.

## Overview

CAP Token Usage Tracker is a CGO-compiled C shared library (`.dll` / `.so` / `.dylib`) that plugs into the CLIProxyAPI host. It intercepts every API request handled by the proxy, aggregates token-level statistics — input, output, reasoning, cached, cache read/creation — and exposes them through a JSON API and a built-in web dashboard.

### Key Features

- **Real-time tracking** — Captures token usage for every request, grouped by model + provider
- **Comprehensive metrics** — Input/output/reasoning/cached tokens, cache read & creation, request count, success rate, latency, and TTFT (time to first token)
- **Built-in dashboard** — A self-contained dark-themed HTML dashboard with auto-refresh (every 5 seconds), no external dependencies
- **JSON API** — Machine-readable `/stats` endpoint for integration with external tools
- **Reset API** — POST `/reset` to clear all counters
- **Thread-safe** — Mutex-protected data store, safe for concurrent proxy traffic
- **Cross-platform** — Pre-built binaries for Linux (amd64/arm64), Windows (amd64), and macOS (amd64/arm64)

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    CLIProxyAPI Host                      │
│                                                          │
│  ┌──────────────┐    ┌─────────────────────────────────┐│
│  │  Proxy Core   │───▶│   Token Usage Tracker Plugin    ││
│  │  (request     │    │                                 ││
│  │   routing)    │    │  ┌─────────────┐                ││
│  └──────────────┘    │  │   Tracker    │ (in-memory)    ││
│                      │  │  (mutex-safe)│                ││
│  ┌──────────────┐    │  └──────┬──────┘                ││
│  │ Plugin RPC   │◀───┴─────────┤                       ││
│  │ (ABI v7)     │               │                       ││
│  └──────┬───────┘               │                       ││
│         │              ┌────────┴────────┐              ││
│         │              │                  │              ││
│         │     ┌────────▼───┐    ┌─────────▼────────┐    ││
│         │     │ Management │    │  Resource Route   │    ││
│         │     │   Routes   │    │    /dashboard      │    ││
│         │     │ /stats     │    │  (HTML dashboard)  │    ││
│         │     │ /reset     │    └───────────────────┘    ││
│         │     └────────────┘                             ││
└─────────┼────────────────────────────────────────────────┘
          │
          ▼
   HTTP response (JSON / HTML)
```

## Plugin Capabilities

| Capability | Description |
|---|---|
| `usage_plugin` | Receives `UsageRecord` for every proxied request |
| `management_api` | Registers management routes (`/stats`, `/reset`) and a resource route (`/dashboard`) |

### Registered Routes

| Type | Path | Method | Description |
|---|---|---|---|
| Management | `/v0/management/plugins/token-usage-tracker/stats` | GET | Aggregated usage statistics (JSON) |
| Management | `/v0/management/plugins/token-usage-tracker/reset` | POST | Reset all counters |
| Resource | `/v0/resource/plugins/token-usage-tracker/dashboard` | GET | Real-time HTML dashboard |

## Tracked Metrics

| Metric | Description |
|---|---|
| `requests` | Total request count |
| `failed_requests` | Failed request count |
| `input_tokens` | Total input (prompt) tokens |
| `output_tokens` | Total output (completion) tokens |
| `reasoning_tokens` | Reasoning/thinking tokens (e.g. o1/o3) |
| `cached_tokens` | Cached tokens (deprecated, sum of cache read + creation) |
| `cache_read_tokens` | Cache read tokens |
| `cache_creation_tokens` | Cache creation tokens |
| `total_tokens` | Sum of all token types |
| `avg_latency` | Average request latency |
| `avg_ttft` | Average time to first token |

## Build

### Prerequisites

- **Go 1.26+**
- **GCC** (required for CGO)
  - Windows: Install [MinGW-w64](https://github.com/brechtsanders/winlibs_mingw) and add `bin` to PATH
  - Linux: `sudo apt install gcc` (or `gcc-aarch64-linux-gnu` for ARM64 cross-compile)
  - macOS: Xcode Command Line Tools (`xcode-select --install`)

### Local Build

```bash
# Set CGO and ensure gcc is in PATH
CGO_ENABLED=1 go build -buildmode=c-shared -o token-usage-tracker.dll .
```

Windows (PowerShell):

```powershell
$env:PATH = "C:\Program Files\Go\bin;C:\mingw64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = 1
go build -buildmode=c-shared -o token-usage-tracker.dll .
```

### CI/CD

GitHub Actions workflow (`.github/workflows/build.yml`) automatically builds for all supported platforms on push and creates releases on version tags (`v*`).

## Deployment

1. Download or build the shared library for your platform
2. Place the `.dll` / `.so` / `.dylib` file in the CLIProxyAPI plugins directory
3. Restart CLIProxyAPI — the plugin auto-registers

## Project Structure

```
cap-token-usage-tracker/
├── main.go          # Plugin entry point, ABI bindings, RPC dispatch
├── tracker.go       # Thread-safe in-memory usage data store
├── management.go    # Management API handlers (/stats, /reset)
├── dashboard.go     # HTML dashboard renderer (embedded CSS/JS)
├── go.mod           # Module definition (depends on CLIProxyAPI v7 SDK)
├── .github/
│   └── workflows/
│       ├── build.yml               # Multi-platform CI build
│       └── qodana_code_quality.yml  # Code quality checks
└── qodana.yaml      # Qodana configuration
```

## Tech Stack

- **Language:** Go 1.26
- **Host SDK:** [CLIProxyAPI v7](https://github.com/router-for-me/CLIProxyAPI) plugin SDK
- **Build Mode:** `c-shared` (CGO compiled dynamic library)
- **CI/CD:** GitHub Actions (5-platform build matrix)
- **Code Quality:** Qodana

## License

This project is part of the [router-for-me](https://github.com/router-for-me) ecosystem.
