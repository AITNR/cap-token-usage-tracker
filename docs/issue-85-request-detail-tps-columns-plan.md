# Issue #85：请求明细新增总耗时与端到端 TPS 计划

- Issue：https://github.com/AITNR/cap-token-usage-tracker/issues/85
- 编写日期：2026-10-11（Asia/Shanghai）
- 核查基线：alpha / 2b5485c4aa0729fb1eaab3ba5a3e15a73722b8af（v2.0.10）
- 状态：已实施；等待提交与推送。

## 1. 需求与决策

Issue 建议在「请求明细」中并列展示流式 TPS、端到端 TPS 与总耗时。与维护者确认后的范围：

1. 保留现有 `tps` 字段与列标题原样，仅新增两列：**总耗时**、**端到端 TPS**。
2. 不引入缓冲流式异常检测或「不可靠」标记，不改变既有 `tps` 计算路径。
3. 不修正 `generation_ns` 在缺少 TTFT 时回退为 `latency_ns` 的既有行为。
4. 读取并持久化宿主 `UsageRecord.Stream` 标志，用于区分流式/非流式。

## 2. 实测证据

对本机插件数据库只读副本（`cpa-core/data/token-usage-tracker.db`，4280 条记录）统计：

| 指标 | 结果 |
| --- | --- |
| 总记录 / 有效 TTFT | 4280 / 4074（95.2%） |
| 无 TTFT 的记录 | 206 条，全部 token 为 0（失败请求） |
| `generation_ns < latency_ns` | 4074 条 |
| 端到端 TPS | min 0.47 / p50 25.6 / p90 75.0 / max 274.75 |
| 流式 TPS | min 0.95 / p50 56.7 / p90 223.2 / max 2,997,662 |
| 流式 TPS > 500 | 111 条（3.06%），其中 113/120 条 `generation_ns < 1s` |

流式 TPS 的极端值集中在 `openai-compatible-*` 中转渠道的缓冲倾倒（`generation_ns` 约 500µs）；端到端 TPS 在全部真实数据上最大仅 274.75，没有爆表。因此端到端 TPS 在不使用任何启发式的前提下提供了 #77 所需的对照口径。

## 3. 指标定义

令 `N` 为 `effectiveOutputTokensForTPS` 的有效输出 tokens，`T = latency_ns`，`L = generation_ns`：

| 指标 | 定义 | 说明 |
| --- | --- | --- |
| 流式 TPS（现有 `tps`） | `N / L` | 保持不变，仍为生成时间口径 |
| 总耗时（`latency_ns`） | 复用现有字段 | 宿主从创建用量记录到请求结束的整次时长 |
| 端到端 TPS（`end_to_end_tps`） | `N / T` | 与流式 TPS 共用分子，包含首字等待 |

`T == 0` 时返回 0（前端显示 0.00）。两者共用分子，因此独立思考协议（Gemini、Vertex、AI Studio、Antigravity、Interactions）的 reasoning 口径一致，且端到端 TPS 不会超过流式 TPS。

## 4. 实现

| 文件 | 改动 |
| --- | --- |
| `internal/plugin/request_log.go` | `RequestDetail` 新增 `EndToEndTPS`、`Stream`；新增 `requestEndToEndTPS`；`requestDetailForUsage` 填充两者，`requestTPS`/`GenerationNS` 不变 |
| `internal/plugin/usage.go` | `normalizedUsage` 新增 `Stream`；`decodeUsage` 通过新增的 `firstBoolPresent` 读取 `Stream`/`stream` |
| `internal/plugin/persistence.go` | `queryRequests` 在既有 `item.TPS = requestTPS(...)` 旁并列重算 `EndToEndTPS`，沿用查询期重算，无迁移 |
| `internal/plugin/preferences.go` | `requestColumnKeys` 加入 `latency_ns`、`end_to_end_tps` |
| `internal/plugin/dashboard.go` | 新增两列定义、排序值与单元格；`latency_ns`/`end_to_end_tps` 复用 `duration()`；生成时间/TPS/总耗时/端到端 TPS 单元格挂 `requestTimingTitle` tooltip；CSV 在 `tps` 后插入 `latency_ns`、`end_to_end_tps` |
| `internal/plugin/locales/{en,zh-CN,zh-TW,ru}.json` | 新增 `table.totalDuration`、`table.endToEndTps`、`requestTiming.streaming`、`requestTiming.nonStreaming`；`export.csvHeaders` 增至 28 列 |
| `README.md` | 中英文各自补充分节，说明总耗时边界、端到端 TPS 口径与 `stream` 语义 |

依赖说明：仓库锁定 SDK `v7.2.159`，其 `pluginapi.UsageRecord` 尚无 `Stream` 字段（v7.3.11 引入）。宿主以 `encoding/json` 序列化无 tag 结构体，字段名即 `Stream`，故直接从原始 JSON 读取，无需升级 SDK。缺少该字段的宿主与历史记录解析为 `false`，表示未知。

## 5. 测试

- `request_log_test.go`：端到端 TPS 分母为 `latency_ns`；`latency_ns` 为 0 返回 0；无 TTFT 仍可计算；`Stream` 解析（`Stream`/`stream`/缺失）；`firstBoolPresent` 区分缺失与显式 false。
- `persistence_test.go`：存量记录查询后 `EndToEndTPS` 重算，`TPS`、`GenerationNS` 不变，且不会凭空获得 `stream`。
- `preferences_test.go`：`latency_ns`、`end_to_end_tps` 可被隐藏并保留。
- `dashboard_test.go`：新增列定义、排序 case、CSV 单元格与 tooltip 函数；locale 目录补齐新键。
- `test/dashboard_date_range.mjs` 新增 `request-tps-full` 场景：完整模式下断言新表头存在、流式行与缓冲行两列取值、排序函数、tooltip 差异，并捕获真实 CSV 校验表头与取值位置。
- `internal/plugin/dashboard_date_range_browser_test.go` 新增 `TestDashboardRequestDetailTpsColumnsInBrowser` 驱动脚本。

验证命令（本机以 Edge 作为 Chrome 通道）：

```
go build ./...
go vet ./...
go test -count=1 ./...
REQUIRE_BROWSER_TESTS=1 CHROME_PATH="C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" go test -count=1 ./...
node --check test/dashboard_date_range.mjs
```

## 6. 风险与兼容性

- 明细表列数由 19 增至 21（完整模式 20 增至 22），CSV 表头由 26 增至 28 列，属用户可见变化，需在发布说明中提示。
- `stream` 对历史记录恒为 `false`，不可回溯；文档明确其为「未知」而非「非流式」。
- 未改动 bbolt schema（`persistenceSchemaVersion` 保持 10），旧备份仍可恢复；回退只需撤回提交。
- 未引入任何阈值或启发式，`tps` 及其它既有指标语义保持不变。