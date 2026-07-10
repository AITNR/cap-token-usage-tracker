# Token Usage Tracker

CLIProxyAPI 的 Token 用量统计插件，实时收集和展示各模型、各 Provider 的 Token 消耗数据，提供可视化 Dashboard 面板。

## 功能特性

- **用量采集** — 作为 `usage_plugin` 接收宿主每次请求完成后的用量记录，涵盖输入/输出/推理/缓存等各维度 Token 统计。
- **多维聚合** — 按模型（Model）和提供方（Provider）两个维度自动聚合请求数、Token 用量和平均延迟。
- **可视化面板** — 内置暗色主题 Dashboard 页面，每 5 秒自动刷新，一目了然展示总览卡片和分维度统计表格。
- **Management API** — 提供 REST 接口供外部查询统计数据和重置计数。
- **零外部依赖** — 纯 Go 标准库实现，无第三方依赖。

## 插件能力

| 能力 | 说明 |
|------|------|
| `usage_plugin` | 接收宿主推送的用量记录（`usage.handle`） |
| `management_api` | 注册管理路由和资源页面（`management.register` / `management.handle`） |

## 管理接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v0/management/plugins/token-usage-tracker/stats` | 获取当前统计快照（JSON） |
| POST | `/v0/management/plugins/token-usage-tracker/reset` | 重置所有统计数据 |

## 资源页面

| 路径 | 说明 |
|------|------|
| `/v0/resource/plugins/token-usage-tracker/dashboard` | Token 用量统计可视化面板 |

## 构建

需要启用 CGO，构建为动态库（`.dll` / `.so` / `.dylib`）：

```bash
# Windows
go build -buildmode=c-shared -o token-usage-tracker.dll .

# Linux
go build -buildmode=c-shared -o token-usage-tracker.so .

# macOS
go build -buildmode=c-shared -o token-usage-tracker.dylib .
```

## 安装与配置

1. 将构建产物放入 CLIProxyAPI 的插件目录：
   ```
   plugins/<GOOS>/<GOARCH>/token-usage-tracker.<ext>
   ```

2. 在 `config.yaml` 中开启插件：
   ```yaml
   plugins:
     enabled: true
     dir: "plugins"
     configs:
       token-usage-tracker:
         enabled: true
   ```

3. 启动 CLIProxyAPI，访问管理面板查看统计：
   ```
   http://localhost:8317/v0/resource/plugins/token-usage-tracker/dashboard
   ```

## 统计字段说明

| 字段 | 说明 |
|------|------|
| 总请求数 | 所有已记录的请求总数 |
| 失败数 | 请求失败的次数 |
| 成功率 | 请求成功百分比 |
| 输入 Token | Prompt 输入消耗的 Token 数 |
| 输出 Token | 模型输出消耗的 Token 数 |
| 推理 Token | 推理过程消耗的 Token 数（如 Thinking） |
| 缓存 Token | 缓存命中的 Token 数 |
| 缓存读取 | 从缓存读取的 Token 数 |
| 缓存创建 | 创建缓存写入的 Token 数 |
| 总 Token | 所有类型 Token 的总和 |
| 平均延迟 | 请求的平均响应时间（ms） |

## 相关文档

- [CLIProxyAPI 插件开发文档](https://help.router-for.me/cn/plugin/development.html)
