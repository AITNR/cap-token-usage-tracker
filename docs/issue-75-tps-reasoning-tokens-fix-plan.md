# Issue #75 修复计划书：独立思考模型的 TPS 被低估

- Issue: <https://github.com/AITNR/cap-token-usage-tracker/issues/75>
- 计划基线: `alpha` @ `29c764e9`，CLIProxyAPI SDK `v7.2.159`
- 问题类型: 请求明细统计口径缺陷（非安全漏洞）
- 目标版本: 下一个 `v2.0.6-alpha` 版本
- 影响面: `/requests` 请求明细与 CSV 导出的 `tps` 字段；不影响 Token 汇总、费用估算和请求计数

## 1. 问题背景

插件在 `internal/plugin/request_log.go` 的 `requestDetailForUsage()` 中计算单次请求 TPS：

```go
generationNS := usage.LatencyNS
if usage.TTFTNS > 0 && usage.LatencyNS >= usage.TTFTNS {
    generationNS = usage.LatencyNS - usage.TTFTNS
}
tps = float64(usage.Counters.OutputTokens) /
    (float64(generationNS) / float64(time.Second))
```

该公式假设 `OutputTokens` 覆盖了从 TTFT 到请求结束期间生成的所有输出 Token。这个假设对 OpenAI 兼容协议通常成立：`ReasoningTokens` 是 `OutputTokens` 的子集。但对 Gemini / Vertex / Antigravity 等独立思考协议不成立：`OutputTokens` 只代表正文 Token，`ReasoningTokens` 是另一个输出桶。

同时，TTFT 记录的是首个流式数据包到达时间。对独立思考模型，首个流式数据包通常就是思考过程开始输出。因此：

```text
GenerationNS = LatencyNS - TTFTNS
             = 思考生成时间 + 正文生成时间
```

当前分子只使用正文 `OutputTokens`，分母却包含思考与正文的完整生成时间。Issue 中的例子是 600 个思考 Token + 50 个正文 Token，生成阶段 2 秒，真实体感速度为 `(600+50)/2 = 325 TPS`，当前结果却是 `50/2 = 25 TPS`。

## 2. 根因分析

### 2.1 数据流

```text
CLIProxyAPI 宿主
  └─ pluginapi.UsageRecord.Detail
       {InputTokens, OutputTokens, ReasoningTokens, ..., TotalTokens}
  └─ handleUsage() / decodeUsage()          [internal/plugin/usage.go]
  └─ requestDetailForUsage()                [internal/plugin/request_log.go]
  └─ RequestDetail.TPS 持久化到 bbolt
  └─ /requests 与 CSV 导出直接展示 item.tps
```

关键限制是：宿主内部已有 `TokenBreakdown` 与 provider 语义归一化，但 `pluginapi.UsageDetail` 传给插件的是扁平字段，不携带“reasoning 是否已经包含在 output 中”的显式标记。插件目前只能根据 `Provider` / `ExecutorType` / Token 计数推断。

### 2.2 协议语义差异

CLIProxyAPI SDK `v7.2.159` 的归一化逻辑可以归纳为：

| 协议族 | 输出 Token 语义 | TPS 分子应为 |
|---|---|---|
| OpenAI / OpenAI-compatible / Codex / xAI / Grok / Kimi / Qwen / DeepSeek / OpenRouter | `ReasoningTokens` 包含在 `OutputTokens` 中 | `OutputTokens` |
| Gemini / Vertex / AI Studio / Antigravity / Interactions | `ReasoningTokens` 与 `OutputTokens` 分离 | `OutputTokens + ReasoningTokens` |
| Anthropic / Claude | thinking/reasoning 包含在 `output_tokens` 中，缓存字段另有独立口径 | `OutputTokens` |
| 未知 provider | 无法可靠判断 | 保守处理，仅在证据充分时合并 |

当前实现没有任何语义判断，统一使用 `OutputTokens`，因此独立思考模型的 TPS 被系统性低估。

### 2.3 Issue 建议条件的风险

Issue 建议使用：

```go
TotalTokens >= InputTokens + OutputTokens + ReasoningTokens
```

这个方向正确，但不能直接照抄，原因是：

1. `decodeUsage()` 在 `TotalTokens <= 0` 时会把 `TotalTokens` 回填为 `InputTokens + OutputTokens + ReasoningTokens`。回填值无法与上游显式提供的 total 区分，导致缺失 total 的 OpenAI 兼容记录也可能被误判为独立思考。
2. 该条件无法覆盖缓存口径差异。例如 Anthropic 一条记录为 `Input=100, Output=50`（其中 reasoning=20），`CacheRead=30, CacheCreation=10, Total=190`。此时 `190 >= 100+50+20` 成立，但 reasoning 已经包含在 output 中；直接合并会把 TPS 分子从 50 错算成 70。
3. `>=` 对存在额外未分类 Token 的记录过于宽松，容易引入新的误判。

因此修复需要“协议语义优先、算术启发保守、未知情况不猜测”的策略。

## 3. 修复目标与非目标

### 目标

1. Gemini / Vertex / AI Studio / Antigravity / Interactions 等独立思考协议的请求明细 TPS 使用 `OutputTokens + ReasoningTokens` 作为分子；
2. OpenAI 兼容、Anthropic 等已将 reasoning 包含在 output 中的协议不发生双计；
3. 不修改 `Counters` 中的 `OutputTokens`、`ReasoningTokens`、`TotalTokens`，也不影响汇总统计、费用估算、缓存命中率；
4. 不修改 `/requests` JSON 结构和 CSV 列结构，仅修正 `tps` 数值；
5. 对已知独立思考协议的存量请求明细，在查询时无迁移地修正显示值；
6. 为协议分类、边界计数和持久化查询路径建立回归测试。

### 非目标

1. 不修改 bbolt 存储结构，不做一次性数据迁移；
2. 不改变宿主 `pluginapi.UsageDetail` 的字段契约；
3. 不尝试在证据不足时猜测所有未知 provider 的 Token 重叠关系；
4. 不改变 TTFT、Latency、GenerationNS 的现有定义；
5. 不修正 CLIProxyAPI 宿主本身的上游 usage 归一化问题。

## 4. 候选方案对比

| 方案 | 描述 | 结论 |
|---|---|---|
| A. 始终加上 `ReasoningTokens` | `tps = (OutputTokens + ReasoningTokens) / GenerationNS` | 否决：OpenAI / Anthropic 会双计 |
| B. 仅按 Issue 中的 total 不等式判断 | `Total >= Input+Output+Reasoning` 时合并 | 否决：受 total 回填和缓存口径影响，可能误判 |
| C. 协议语义优先 + 保守算术回退 + 查询期修正 | 先按 Provider/ExecutorType 识别已知协议；未知 provider 仅在证据充分时合并；查询旧数据时重算 TPS | 采用 |

## 5. 推荐实现方案

### 5.1 新增 reasoning 计数语义分类

在 `internal/plugin/request_log.go`（或相邻文件）新增内部枚举：

```go
type reasoningTokenAccounting uint8

const (
    reasoningAccountingUnknown reasoningTokenAccounting = iota
    reasoningAccountingIncludedInOutput
    reasoningAccountingSeparateFromOutput
)
```

新增分类函数，语义与 CLIProxyAPI SDK `v7.2.159` 的 provider 归一化保持一致：

- `gemini`、`vertex`、`aistudio`、`antigravity`、`interaction` / `interactions` → `SeparateFromOutput`；
- `openaicompatexecutor`、`openai-compatibility`、`openai-compatible-*`、`openai`、`codex`、`xai`、`grok`、`kimi`、`qwen`、`deepseek`、`openrouter` → `IncludedInOutput`；
- `claude` / `anthropic` → `IncludedInOutput`；
- 其他 → `Unknown`。

匹配时统一 lowercase + trim，并同时检查 `Provider` 与 `ExecutorType`，避免大小写或空格造成漏判。

### 5.2 保守处理未知 provider

未知 provider 不直接套用 Issue 的不等式。建议规则：

1. `ReasoningTokens > OutputTokens` 时，视为独立思考。因为非负且一致的“子集”语义下，reasoning 不可能大于 output；
2. 上游显式提供 `TotalTokens` 且恰好等于 `InputTokens + OutputTokens + ReasoningTokens` 时，视为独立思考；
3. 上游显式提供 `TotalTokens` 且等于 `InputTokens + OutputTokens` 时，视为 reasoning 已包含在 output 中；
4. 其他未知情况保持现状，只使用 `OutputTokens`，避免引入新的误判。

为实现第 2/3 条，`decodeUsage()` 需要在执行 total 回填之前记录 `explicitTotalTokens bool`，并将其作为 `normalizedUsage` 的瞬态字段传递给 `requestDetailForUsage()`。该字段不持久化、不进入 JSON。

### 5.3 新增 TPS 分子计算函数

新增类似函数：

```go
func effectiveOutputTokensForTPS(
    dimensions Dimensions,
    counters Counters,
    explicitTotal bool,
) uint64
```

行为：

- `ReasoningTokens == 0`：返回 `OutputTokens`；
- 已知 `IncludedInOutput`：返回 `OutputTokens`；
- 已知 `SeparateFromOutput`：返回 `saturatingAdd(OutputTokens, ReasoningTokens)`；
- 未知：按 5.2 的保守规则处理；
- 所有加法必须使用现有 `saturatingAdd` 风格，避免 `uint64` 溢出。

`requestDetailForUsage()` 保留现有 `GenerationNS` 计算，只把 TPS 分子替换为该函数结果。

### 5.4 查询期修正存量数据

`RequestDetail` 中已经持久化了计算 TPS 所需的 `Dimensions`、`Counters`、`LatencyNS`、`TTFTNS`。因此不需要迁移 bbolt：

- 在 `storeActor.queryRequests()` 反序列化 `RequestDetail` 后、追加到 `page.Items` 前，调用同一个 TPS 归一化函数重算 `item.TPS`；
- 已知独立思考协议的历史记录立即显示修正后的 TPS；
- 已知 reasoning 已包含在 output 的协议重算后结果保持不变；
- 未知 provider 保留保守结果；
- 不写回数据库，保留原数据作为回滚依据。

这样 `/requests` 与 CSV 导出会自然获得一致结果，因为二者都消费查询出的 `RequestDetail.TPS`。

### 5.5 明确不修改的部分

- `Counters.OutputTokens` / `ReasoningTokens` / `TotalTokens` 的存储值；
- 聚合统计、模型统计、维度统计、费用估算；
- `RequestDetail` JSON 字段和 schema 版本；
- 仪表盘前端列定义与 CSV 表头；
- bbolt 读写格式。

## 6. 测试计划

### 6.1 单元测试

新增 `internal/plugin/request_log_test.go`，至少覆盖：

1. **Issue 示例**：Gemini 记录 `Input=100`、`Output=50`、`Reasoning=600`、`Total=750`、`Latency=2.25s`、`TTFT=250ms`，断言 `GenerationNS=2s` 且 `TPS=325`；
2. **OpenAI 子集语义**：`Output=50`、`Reasoning=10`、`Total=Input+50`，断言 TPS 只按 50 计算，不变成 60；
3. **Anthropic 缓存语义**：构造 `Input=100, Output=50, Reasoning=20, CacheRead=30, CacheCreation=10, Total=190`，断言 TPS 分子仍为 50，防止 Issue 建议的不等式引入双计；
4. **Vertex / AI Studio / Antigravity / Interactions**：分别命中独立思考语义；
5. **未知 provider 且 reasoning > output**：即使缺少 total，也合并 reasoning；
6. **未知 provider 且 total 显式等于 input+output+reasoning**：合并 reasoning；
7. **未知 provider 且 total 显式等于 input+output**：不合并 reasoning；
8. **total 缺失并触发回填**：当 reasoning 未大于 output 时不因回填值误判为独立思考；
9. **GenerationNS 为 0**：TPS 保持 0；
10. **OutputTokens/ReasoningTokens 极大值**：使用饱和加法，不 panic、不回绕。

### 6.2 持久化与 API 回归

调整或新增 `internal/plugin/persistence_test.go` 用例：

1. 通过 `store.Record()` 写入 Gemini 独立思考记录，查询 `/requests` 数据源时 TPS 为 `Output+Reasoning` 口径；
2. 重启 store 后再次查询，TPS 仍正确；
3. 构造一条已持久化但 `TPS` 为旧口径的 Gemini `RequestDetail`，确认查询时会重算为正确值，且不需要修改存储桶；
4. 现有 OpenAI 用例保持 TPS 不变；
5. CSV 导出继续使用同一 `tps` 字段，不需要新增前端测试即可保持一致性。

### 6.3 验证命令

```powershell
go test ./internal/plugin -run "TestRequest|TestStorePersistsAndQueriesPerRequestDetails"
go test ./...
```

如果本地具备 CLIProxyAPI v7.2.159 与 Gemini / OpenAI 凭据，再执行手工验证：

1. 分别触发一次 Gemini 独立思考请求和一次 OpenAI reasoning 请求；
2. 打开仪表盘「请求明细」，核对 Gemini TPS 与 `(reasoning_tokens + output_tokens) / generation_seconds` 一致；
3. 核对 OpenAI TPS 与 `output_tokens / generation_seconds` 一致；
4. 导出 CSV，确认 TPS 与页面一致；
5. 用升级前数据库查询旧 Gemini 记录，确认显示值已被查询期修正，数据库文件未被迁移改写。

## 7. 兼容性与风险

- **API 兼容**：字段结构不变，仅数值口径修正；
- **存储兼容**：无 schema 变更、无迁移、无写回；回滚后新记录会恢复旧口径，历史数据本身未被破坏；
- **汇总兼容**：Token 总数、请求数、失败数、费用、缓存命中率均不受影响；
- **误判风险**：provider/executor 别名不在已知列表或上游语义发生变化时可能仍不完美。缓解措施是与 SDK v7.2.159 的语义表对齐，并对未知 provider 采用保守规则；
- **历史数据风险**：查询期重算会改变已知独立思考协议旧记录的显示 TPS，这是期望行为；
- **长期方案**：若未来 `pluginapi.UsageDetail` 暴露宿主 `TokenBreakdown` 或显式 accounting mode，应移除 provider 推断，直接消费权威语义。

## 8. 实施步骤与预估

| 步骤 | 内容 | 预估 |
|---|---|---|
| 1 | 定义 reasoning 语义分类与 TPS 分子 helper | 1h |
| 2 | 修改 `decodeUsage()` 记录 total 是否显式提供 | 0.5h |
| 3 | 修改 `requestDetailForUsage()` 与查询期重算逻辑 | 1h |
| 4 | 新增/调整单元测试与持久化测试 | 2h |
| 5 | 全量测试、手工验证、更新 TPS 口径文档 | 1h |
| 6 | 合入 alpha 并在 Release Notes 中关联 Issue #75 | 0.5h |

总计约 0.5–1 人日。

## 9. 涉及文件

| 文件 | 动作 |
|---|---|
| `internal/plugin/request_log.go` | 修改：新增语义分类、TPS 分子计算、查询期重算 helper |
| `internal/plugin/usage.go` | 修改：保留 total 是否显式提供的瞬态信息 |
| `internal/plugin/request_log_test.go` | 新增：语义分类与 TPS 边界测试 |
| `internal/plugin/persistence_test.go` | 修改：新增新记录、重启、旧记录查询期修正测试 |
| `internal/plugin/persistence.go` | 修改：`queryRequests()` 反序列化后重算 TPS |
| `README.md` | 可选：补充 TPS 对独立思考协议的分子口径 |
| `docs/issue-75-tps-reasoning-tokens-fix-plan.md` | 归档本计划 |
