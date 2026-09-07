# CAP Token Usage Tracker - DIY

基于 [AITNR/cap-token-usage-tracker](https://github.com/AITNR/cap-token-usage-tracker) **v2.0.5** 的魔改版（MIT 协议，上游许可保留，见 `LICENSE`；上游完整文档见 `README_UPSTREAM.md`）。

魔改目的：给插件的用量统计增加 **sub2api 兼容用量查询接口** 和 **可自定义配额**，方便 sub2api 协议客户端直接查询用量（配额按 999999 USD 假想设置，因为插件没有真实总额度概念）。

---

## 魔改对象（相对 v2.0.5 的改动）

改动文件：`management.go`、`lifecycle.go`、`dashboard.go`、`management_test.go`（均为本地功能，不涉及任何外部调用）。

### 1. 新增 sub2api 兼容用量接口

```
GET /v0/resource/plugins/cap-token-usage-tracker/v1/usage
```

返回结构仿 sub2api `GET /v1/usage`（unrestricted 钱包模式），数据全部来自插件自身统计：

```json
{
  "mode": "unrestricted",
  "isValid": true,
  "planName": "CAP Token Usage Tracker",
  "unit": "USD",
  "quota": 999999,
  "used": 24.88,
  "remaining": 999974.12,
  "balance": 999974.12,
  "range": "retention",
  "usage": {
    "total":    { "requests": 1413, "input_tokens": 230737048, "output_tokens": 570202, "cache_read_tokens": 212461964, "cache_creation_tokens": 0, "total_tokens": 231564603, "cost": 24.88, "actual_cost": 24.88 },
    "today":    { ...同上，当日统计... },
    "average_duration_ms": 12175.06,
    "rpm": 0,
    "tpm": 0
  },
  "daily_usage": [
    { "date": "2026-08-29", "requests": 523, "input_tokens": 106661338, "output_tokens": 235351, "cache_read_tokens": 98151247, "cache_write_tokens": 0, "total_tokens": 107006549, "cost": 15.18, "actual_cost": 15.18 }
  ],
  "model_stats": [
    { "model": "gemini-3.7-flash-high", "requests": 904, "input_tokens": 111762197, "output_tokens": 157531, "cache_creation_tokens": 0, "cache_read_tokens": 99456081, "total_tokens": 112173924, "cost": 8.64, "actual_cost": 8.64, "account_cost": 8.64 }
  ]
}
```

- 可选查询参数：`?range=24h|7d|30d|retention`（默认 `retention`，即全部保留统计）。
- `cost`/`actual_cost`/`account_cost` 均取插件估算的用量金额（插件只有一种成本估算）；`cache_write_tokens` 映射自插件的 `cache_creation_tokens`。

### 2. 新增配额设置接口

```
GET /v0/resource/plugins/cap-token-usage-tracker/quota            # 读当前配额 {"quota": 999999}
GET /v0/resource/plugins/cap-token-usage-tracker/quota?set=12345  # 设置配额（0 ~ 1e12，非法值返回 400）
```

- 配额持久化到插件数据目录下的 `quota.json`（与 `token-usage-tracker.db` 同目录），**重启不丢**。
- `/v1/usage` 的 `quota / remaining / balance` 使用该配额值（默认 999999.00 USD）。

### 3. 仪表盘新增"限额"按钮

- 工具栏（与"模型价格"按钮同一行）新增 **限额** 按钮。
- 点击弹出小窗口：输入框（默认 `999999.00`）+ **确认 / 取消** 按钮。
- 确认后保存配额，`/v1/usage` 立即联动。

### 4. 关于顶级 `/v1/usage` 路径

CPA 插件宿主强制所有插件路由挂在 `/v0/resource/plugins/<插件ID>/` 前缀下（插件无法注册顶级路径）。如需 sub2api 客户端以 `base_url=https://你的域名` 直接查询 `/v1/usage`，在反代（nginx）上加一条精确转发：

```nginx
location = /v1/usage {
    proxy_pass http://cli-proxy-api:8317/v0/resource/plugins/cap-token-usage-tracker/v1/usage;
}
```

（`cli-proxy-api:8317` 按你的实际上游容器名/端口调整；若客户端访问 `/v1/usage/` 带尾斜杠，再补一条 `location = /v1/usage/`。）

---

## 安装方法

### 方式一：使用预编译 .so（推荐，linux/amd64，glibc）

1. 下载 `dist/cap-token-usage-tracker-v2.0.5-diy.so`
   - SHA-256：`3e5da4af30550898d2e7947f9af32f65336cdc729ca0de2b67426d1df437e8a5`
2. 替换 CPA 容器内的插件文件并重启：

```bash
docker cp ./cap-token-usage-tracker-v2.0.5-diy.so <cpa容器名>:/CLIProxyAPI/plugins/linux/amd64/cap-token-usage-tracker-v2.0.5.so
docker compose restart cli-proxy-api   # 或 docker restart <cpa容器名>
```

> 注意：插件文件路径必须与 CPA 插件配置中的路径一致（通常是 `plugins/linux/amd64/cap-token-usage-tracker-v2.0.5.so`，可通过 `/v0/management/plugins` 查询）。

3. （可选）nginx 顶级 `/v1/usage` 转发，见上文"关于顶级 /v1/usage 路径"。

### 方式二：源码构建

需要 Go 1.26 + gcc（CGO）。**构建环境 glibc 版本需与 CPA 容器匹配**（CPA 容器为 Debian 12/glibc 2.36 时，用 `golang:1.26-bookworm` 构建；Alpine/musl 环境构建的 .so 无法在 glibc 容器加载）。

低内存服务器（如 1GB）构建示例（限制并行、配合 swap）：

```bash
docker run --rm -v $PWD:/src -w /src golang:1.26-bookworm sh -c \
  "export CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct GOFLAGS=-p=1 GOMAXPROCS=1 GOGC=50; \
   go build -buildmode=c-shared -buildvcs=false -trimpath -gcflags=all=-l -ldflags='-s -w' -o cap-token-usage-tracker-diy.so ."
```

构建产物替换到容器内插件路径，步骤同方式一。运行 `go test -p 1 -parallel 1 ./...` 可跑全量单元测试。

### 持久化建议（重要）

插件 .so、用量数据库（`token-usage-tracker.db`）、`quota.json` 默认位于容器可写层，**容器重建会丢失**。建议给 CPA compose 增加卷挂载并迁移现有文件：

```yaml
volumes:
  - ./config.yaml:/CLIProxyAPI/config.yaml
  - ./auths:/root/.cli-proxy-api
  - ./logs:/CLIProxyAPI/logs
  - ./data:/CLIProxyAPI/data          # 用量库 + quota.json
  - ./plugins:/CLIProxyAPI/plugins    # 插件 .so
```

首次挂载前先把容器内现有文件复制到宿主机目录：

```bash
mkdir -p ./data ./plugins/linux/amd64
docker cp <cpa容器名>:/CLIProxyAPI/data/. ./data/
docker cp <cpa容器名>:/CLIProxyAPI/plugins/linux/amd64/. ./plugins/linux/amd64/
docker compose up -d
```

---

## 注意事项

- **插件商店"更新"会覆盖自定义 .so**，更新后需重新部署本版本。
- `/v1/usage` 与 `/quota` 为**无鉴权**资源路由（与官方 `/stats` 同级），仅返回用量数字与配额，不含敏感数据；完整模式/备份/重置等仍与官方一致（需要管理密钥）。**请勿将面板暴露到不可信网络**。
- 配额接口写入的 `quota.json` 仅存一个数值，无任何凭据。

## 安全说明（已审计）

- 本魔改仅改动上述 4 个文件，均为本地功能（用量聚合输出 + 配额读写），**不向任何外部服务器发送本地数据**。
- 无遥测/后门：外部网络调用仅官方自带的 models.dev 价格同步与 open.er-api.com 汇率获取（用户手动触发或官方已有功能），且只拉取不下发。
- 无可疑加密：API Key 加密沿用官方 AES-GCM（`api_key_secret`）；无硬编码凭据（除官方文档化的默认加密密钥 `123456`，该默认值仅在未配置自定义密钥时用于加密存储的 API Key）。
- 本仓库不含任何真实密钥、IP、域名等敏感信息（本文中的域名为占位符）。
