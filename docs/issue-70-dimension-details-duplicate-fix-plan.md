# Issue #70 修复计划书：维度明细中失败请求产生重复行

- Issue: <https://github.com/AITNR/cap-token-usage-tracker/issues/70>
- 报告版本: 插件 v2.0.5 / 宿主 CLIProxyAPI v7.2.152
- 问题标题: 请求错误的记录会导致维度明细中出现重复的错误数据
- 期望行为: 无重复数据（或干脆不显示错误记录）
- 目标版本: v2.0.6

## 1. 问题背景

用户在仪表盘「维度明细」（Dimension details）表格中观察到：只要发生请求失败，同一模型/维度就会额外出现一行（或多行）外观几乎完全相同的"错误数据"行——模型、提供商、来源等列与正常行一致，仅请求数为 1、失败数为 1、Token 全为 0。失败次数越多（或失败状态码种类越多），重复行越多。

## 2. 根因分析

### 2.1 数据流

```
CLIProxyAPI 宿主（每次请求完成，无论成败）
  └─ RPC "usage.handle" → handleUsage()                     [lifecycle.go:147]
      └─ decodeUsage() 解析出 Dimensions + Counters           [usage.go:23]
          Dimensions = {Provider, ExecutorType, Model, Alias, Source,
                        APIKey*, AuthType, ServiceTier, ReasoningEffort,
                        Failed bool, FailureStatus int}        ← 关键
          Counters   = {Requests:1, FailedRequests:0|1, 各类 Token, ...}
      └─ storeActor.record()                                  [persistence.go:2005]
          聚合键 = {Hour(分钟), Dimensions 完整结构体}          ← Failed/FailureStatus 参与聚合键
      └─ 持久化到 bbolt 分钟级桶 + 请求明细桶（请求明细保留完整失败信息）

仪表盘「维度明细」/stats/groups
  ├─ 常规路径: buildGroupsForRange()     [aggregate.go:694]
  └─ 秒级自定义范围路径: queryExactStats() [persistence.go:2519]
      → buildStatsForRangeWithFilter()   [aggregate.go:332]
  两条路径均按「完整 Dimensions（含 Failed/FailureStatus）」分组出行
```

### 2.2 重复行的产生机理

`Dimensions` 结构体中的两个**瞬态字段**参与了维度分组键（`aggregate.go:30-31`）：

```go
Failed           bool   `json:"failed"`
FailureStatus    int    `json:"failure_status"`
```

而仪表盘维度明细表格的列定义（`dashboard.go:459` `dimensionColumns`）**并不包含** `failed` / `failure_status` 列。于是：

1. 同一模型只要出现过失败，就至少分裂成 2 行：`Failed=false` 一行 + `Failed=true` 一行；
2. 失败行再按 `FailureStatus`（0 / 401 / 429 / 500 / …）继续分裂成 N 行——每个状态码一行；
3. CLIProxyAPI 侧重试/多凭据切换场景下，一次客户端请求可能产生多条失败用量记录（每次上游尝试一条），进一步放大行数；
4. 失败记录 Token 计数全为 0，这些行排序时因 `TotalTokens` 相同而聚在表格末尾，外观上就是一整块"重复的错误数据"。

代码侧证据：

- `aggregate_test.go:144-145` 的 `TestBuildStatsStableDimensionOrdering` 把 `{FailureStatus: 500}` 与 `{Failed: true}` 当作两个**不同维度**构造数据——印证当前设计确实按这两个字段分裂分组行；
- `compareDimensions()`（`aggregate.go:788-795`）把 `Failed` / `FailureStatus` 纳入排序比较，佐证二者被视为维度组成部分；
- 前端 `dimensionColumns` 无对应列，`modelRows()` / `visibleGroups()` 等聚合展示不受影响，仅维度明细表格暴露此问题。

### 2.3 结论

失败次数本身有正确承载（每行 `Counters.FailedRequests`，且前端已有"失败"列并高亮），失败明细也有正确承载（请求明细表 `/requests` 的 `Result` 列显示"失败 (HTTP xxx)"，且支持 成功/失败 结果过滤）。**把 `Failed` / `FailureStatus` 纳入维度分组键没有任何展示收益，只产生重复行。**

## 3. 修复目标与非目标

### 目标

- 维度明细中，同一维度组合（provider/executor/model/alias/source/api_key/auth_type/service_tier/reasoning_effort）**只出现一行**，无论成功失败混合多少次、失败状态码有多少种；
- 行内 `requests` 为总请求数、`failed_requests` 为失败数，语义保持现有口径；
- 请求明细表保持逐条展示失败请求及其 HTTP 状态码（现状能力不回退）；
- 兼容存量数据：升级后历史"重复行"立即消失，无需数据迁移、不改 bbolt 存储格式。

### 非目标

- 不改变失败记录的采集与持久化（`/requests` 明细、失败计数、成功率计算全部保留）；
- 不改仪表盘前端列结构（本次为纯后端查询侧修复，前端零改动）；
- 不处理 CLIProxyAPI 侧重试导致的"一次请求多次尝试各计一条"的统计口径问题（那是宿主行为，且请求明细按条展示是合理口径）。

## 4. 候选方案对比

| 方案 | 描述 | 优点 | 缺点 | 结论 |
|---|---|---|---|---|
| A. 查询时合并（推荐） | 构建 Groups 行时把 `Failed`/`FailureStatus` 从分组键中剥离 | 存量+新增数据一并修复；无迁移；改动集中（仅 `aggregate.go`）；失败计数仍按行累计 | `/stats`、`/stats/groups` 响应中 Groups 行的 `failed`/`failure_status` 恒为 false/0（字段保留、语义中性化，需文档说明） | ✅ 采用 |
| B. 写入时合并 | `storeActor.record()` 聚合前清零两字段 | 新数据聚合键更紧凑 | **存量数据仍重复显示**直至留存期淘汰；仍需查询侧配合；改动面更大 | ❌ |
| C. 不记录失败请求 | 丢弃 `Failed=true` 的用量记录 | 最简单 | 丢失失败计数、成功率、失败明细过滤；issue 作者也只是二选一 | ❌ |
| D. 前端补列 | 维度明细增加 failed/failure_status 列 | 无后端改动 | 表格更拥挤；"同一模型多行"的观感问题依旧，违背期望"无重复数据" | ❌ |

## 5. 推荐方案（A）详细设计

### 5.1 新增辅助函数（internal/plugin/aggregate.go）

```go
// groupIdentity strips failure-only fields so rows that differ only by
// Failed/FailureStatus merge into one dimension row. Failure counts stay
// visible through Counters.FailedRequests, and per-request failure details
// remain available on the /requests endpoint.
func groupIdentity(dimensions Dimensions) Dimensions {
    dimensions.Failed = false
    dimensions.FailureStatus = 0
    return dimensions
}
```

### 5.2 改动点 1：`buildStatsForRangeWithFilter()`（aggregate.go:332）

覆盖 `/stats` 完整响应的 `Groups`，以及秒级自定义范围下 `/stats/groups`（经 `queryExactStats` 复用本函数）。

```go
dimensions := sanitizeDimensionsSource(key.Dimensions)   // 现有逻辑不变
// sources / apiKeyRefs / filter.matches(dimensions)      // 均不变（过滤器不含失败字段）
identity := groupIdentity(dimensions)                     // 新增
group := groups[identity]
group.add(counters)
groups[identity] = group
```

注意：行构造时从 map 键取回的 `Dimensions` 已是合并后身份，`APIKey` 密文挂接（按 `APIKeyGeneration`+`APIKeyHash` 取 ref）不受影响。

### 5.3 改动点 2：`buildGroupsForRange()`（aggregate.go:694）

覆盖常规路径 `/stats/groups`（维度明细表格的直接数据源）。改法与 5.2 相同。

### 5.4 不改动的部分（明确说明）

- `storeActor.record()`：聚合键保持含失败字段，存储格式、schema_version、写路径零变化（查询时合并天然兼容）；
- `queryExactStats()`：重建的中间 map 保持原样，输出经 `buildStatsForRangeWithFilter` 自动合并；
- `compareDimensions()`：保留失败字段比较（合并后恒等，无副作用），维持排序稳定性；
- `GroupStatsPage.SchemaVersion` 保持 2：JSON 结构未变，仅行合并；
- 前端 `dashboard.go`：零改动（`failed_requests` 列已存在并高亮）。

### 5.5 行为变化（对外可见）

| 端点 | 变化前 | 变化后 |
|---|---|---|
| `/stats/groups`（维度明细） | 同维度成功/失败各一行，失败再按状态码分裂 | 同维度一行，`requests`=总数，`failed_requests`=失败数 |
| `/stats` 完整响应 `Groups` | 同上分裂 | 同上合并 |
| `/stats/initial`、`/stats/trends`、汇总卡片 | 无重复（本就按模型/小时聚合） | 不变 |
| `/requests` 请求明细 | 逐条展示"失败 (HTTP xxx)" | 不变 |
| 成功率、费用估算 | — | 不变（失败行 Token 为 0，合并对 Token/费用无影响；请求数口径不变） |

## 6. 测试计划

### 6.1 新增用例

1. `aggregate_test.go` — `TestBuildStatsMergesFailureDimensionsIntoSingleGroup`
   - 构造同一维度 4 个聚合键：成功 ×2、`Failed+500` ×1、`Failed+429` ×1、`Failed+0` ×1；
   - 断言 `buildStats` 与 `buildGroupsForRange` 均只产出 1 行；
   - 断言行内 `Requests==5`、`FailedRequests==4`、`Failed==false`、`FailureStatus==0`、Token 仅含成功行的值。
2. `persistence_test.go` — `TestGroupsPageMergesFailedRecordsAcrossStatusCodes`
   - 通过 `store.Record()` 落库上述混合记录后：
     - `queryGroupsByFilter`（常规路径）→ 1 行，计数正确；
     - `queryStatsByFilter`（自定义含秒级时间范围，触发 `queryExactStats` 精确路径）→ Groups 同样 1 行；
     - `QueryRequests` 仍返回 5 条明细，失败条目保留 `Failed`/`FailureStatus` 与 `Result="失败 (HTTP 500)"` 等，success/failed 结果过滤不受影响。
3. 回归现有用例：
   - `TestBuildStatsStableDimensionOrdering`：失败维度合并后行数 6→5，排序断言依旧成立（如需显式锁定，补充行数断言）；
   - `api_key_integration_test.go` 中 `len(stats.Groups)` 断言：其构造的维度不含失败字段，预期不受影响，跑通即回归通过。

### 6.2 验证步骤（手工）

1. `go test ./internal/plugin/...` 全绿；
2. 本地起 CLIProxyAPI v7.2.152 + dev 构建插件，配置一个必然失败的凭据/模型触发若干失败请求（不同状态码）；
3. 打开仪表盘：
   - 维度明细：该模型仅一行，"失败"列计数正确且高亮；
   - 请求明细：逐条失败请求仍显示"失败 (HTTP xxx)"，结果过滤器可用；
   - 顶部卡片请求数/成功率与请求明细总数一致；
4. 切换到秒级自定义时间范围（触发精确路径）复查第 3 步；
5. 用 v2.0.5 时代的旧数据库文件直接挂载验证：历史重复行立即消失（无迁移验证）。

## 7. 兼容性与风险

- **存储**：bbolt 桶结构、schema_version、备份/恢复格式均不变；
- **API**：`Groups[]` 行的 `failed` / `failure_status` 字段仍在 JSON 中（结构兼容），但值恒为 `false` / `0`，失败信息以 `failed_requests` 计数器为准——需在 README「资源接口」小节与 Release Notes 中注明；
- **风险点**：若有第三方直接消费 `/stats` 的 `Groups` 并依赖失败字段拆行（极小概率，该拆分正是本 bug），升级说明中已声明语义变化；
- **回滚**：纯查询侧改动，回滚即恢复旧行为，无数据残留。

## 8. 实施与发布

| 步骤 | 内容 | 预估 |
|---|---|---|
| 1 | `aggregate.go` 新增 `groupIdentity()` 并在两处 build 函数应用 | 0.5h |
| 2 | 新增/调整测试用例 | 2h |
| 3 | README 接口语义说明 + docs 本计划书归档 | 0.5h |
| 4 | `go test ./...` + 手工验证（含旧库回归） | 1h |
| 5 | 合入 alpha → 打 tag v2.0.6 → Release Notes 注明修复 #70 | — |

总计约 0.5 人日。修复落在查询层（`internal/plugin/aggregate.go`），不动存储与前端。

## 9. 附录：涉及文件清单

| 文件 | 位置 | 动作 |
|---|---|---|
| `internal/plugin/aggregate.go` | :332 `buildStatsForRangeWithFilter`、:694 `buildGroupsForRange` | 修改（新增 `groupIdentity` 并应用） |
| `internal/plugin/aggregate_test.go` | — | 新增用例 |
| `internal/plugin/persistence_test.go` | — | 新增用例 |
| `README.md` | 资源接口说明 | 补充 Groups 行合并语义 |
| `docs/issue-70-dimension-details-duplicate-fix-plan.md` | 本文档 | 归档 |
