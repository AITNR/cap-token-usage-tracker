# Issue #80 启动成本消除实施计划

- Issue: https://github.com/AITNR/cap-token-usage-tracker/issues/80
- 基础方案：`docs/issue-80-startup-readiness-fix-plan.md`（生命周期 readiness 与异步化设计）
- 目标：让插件启动不再随历史聚合数据和逐请求记录规模线性增长，同时保持现有 API 统计语义。
- 文档性质：实施规格与验收清单；实现时以本文为约束，不将本文内容当作对外部宿主契约的证明。
- 当前代码基线：工作区 `persistence.go` 的 schema v10/API Key catalog 改动；实施者开始前必须重新检查 git 状态和实际代码，保留所有既有改动。

## 1. 目标与架构决策

当前稳定 schema 已通过 API Key catalog 消除 `reload()` 对 `requests` 桶的扫描，但 `reload()` 仍将保留期内的全部聚合记录读入 `a.data`。本计划将全量内存恢复改为**热窗口驻留 + actor 内按需范围读取**，并修复 catalog 的损坏恢复策略。

本计划只承诺降低插件自身的数据库恢复成本；不承诺某个硬编码启动时限。CPA 是否等待注册完成、是否重试 usage RPC、是否允许 register 返回时 capability 尚未 ready，均须通过目标 CPA/SDK 版本验证。未经验证不得依赖异步注册或 usage 503 重试。

架构边界：

1. Actor 仍是 store 状态与数据库访问的唯一串行入口。查询、flush、prune、reset 和范围读通过 actor 命令协调；不得从新 goroutine 绕过 actor 并发读写 `a.data` 或同一个 `bolt.DB`。
2. bbolt 聚合桶名当前为 `hours`，但其 key 实际是 `RequestedAt.UTC().Truncate(time.Minute).Unix()`。外层桶是**分钟聚合桶**，不是小时桶。所有范围边界、热窗口和性能估算都必须按分钟 key 处理；保留现有 bucket 名称以避免无关 schema 迁移。
3. 热窗口是内存优化，不是数据可用性边界。查询范围超出热窗口必须由同一 actor 读取冷范围，并与内存中未 flush 的 dirty 聚合合并，结果不得不完整。
4. catalog、models index 均是可重建派生数据；不得因其缺失/损坏而静默丢数据，也不得在 `register()` 同步路径无条件触发全量重建。

## 2. 当前基线事实

启动路径为 `openStoreWithCrypto()` -> `storeActor.initialize()` -> `reload()`。稳定 schema v10 下：

- `reload()` 从 `meta` 恢复配置、价格、偏好和 API Key catalog；
- `reload()` 遍历 `hours` 下所有保留分钟桶，反序列化维度与 counters 并填充 `a.data`；
- `reload()` 不再扫描 `requests`；
- schema 升级或 catalog 缺失时，`buildAPIKeyCatalog()` 仍需一次全量 backfill；
- `flush()` 维护 catalog 与聚合、请求记录的事务一致性；prune 处理过期记录；
- `observedModels()` 当前通过 `a.data` 枚举模型，热窗口化后若不另建索引会改变其保留期语义。

这些事实实施前须用当前代码再次确认。不要依赖文档中的固定行号。

## 3. 实施阶段

### 阶段 0：基线、隔离与测量

1. 检查 `git status`、当前分支和已有未提交改动；不得覆盖用户或先前任务变更。
2. 保存当前测试基线，运行 `go test ./... -count=1` 和 `go vet ./...`。
3. 建立可重复的合成 bbolt fixture：多个分钟、多个 dimension、多个 API Key、请求明细；至少包含热窗口内外数据。
4. 记录现状 open 耗时、`reload()` 聚合解码条数、峰值内存。时间数据仅作为基准，不设环境无关的绝对毫秒承诺。

验收：fixture 可复用、测试可确定性运行，基准能证明初始化扫描记录数。

### 阶段 1：修复 catalog 派生索引可靠性

1. 合并 `initialize()` 中 catalog load/build 两分支的重复 save；仅在构建、修复或确有变更时写入。
2. 对 catalog 缺失、JSON 损坏、引用计数非法、引用 generation 无效分别定义行为。
3. Catalog 是派生索引：损坏时不得直接把统计主库判为不可用；也不得在同步注册路径自动 full-scan 修复。
4. 本阶段采用显式 degraded/rebuild-required 状态或维护入口。若完整 readiness 状态目前尚未实现，应让主统计和逐请求查询继续可用，同时禁用依赖不可信 catalog 的 API Key label/ciphertext 管理操作，并记录不含敏感数据的错误。禁止用空 catalog 覆盖损坏 catalog。
5. 提供 actor 串行执行的显式 catalog rebuild 操作；rebuild 在 bbolt transaction 中从 hours/requests 重建并校验，成功后原子替换，失败时保留旧索引与错误状态。
6. 增加 catalog format/version 或 generation 标记，使“有效空 catalog”和“缺失/损坏 catalog”可区分。
7. 维持现有写入语义：aggregate/request 与 catalog 在同一 bbolt transaction 更新；transaction 成功后再更新 actor 内存副本。

测试：valid empty catalog、missing、非法 JSON、非法计数、backfill 成功/失败、写事务回滚、prune、reset、restore、API Key generation 切换、label 引用关系。

### 阶段 2：按需访问 observed models

1. 保持 `ObservedModels()` 的现有语义：返回保留数据范围内出现过的模型，不允许悄悄缩成热窗口模型。
2. 增加独立 `observed_models` 派生索引，在成功持久化新聚合时增量维护；索引更新与聚合写入同一事务。
3. 过期分钟桶 prune 时，只有当模型的全部保留聚合引用都已移除后才删除模型。可以维护模型级计数，也可通过明确可控的重建策略；不得每小时扫描全量请求记录。
4. 旧库模型索引缺失时，允许一次 rebuild，但要与 catalog 一样避免阻塞宿主注册；应由显式维护入口或异步 readiness 阶段协调。过渡期 `ObservedModels()` 可在索引不可用时执行明确可观察的兼容扫描，不能返回不完整集合。

测试：旧模型超出热窗口但仍在 retention 内时仍可观察；模型所有聚合过期后消失；多 provider/model 组合、reset、restore 和索引 rebuild。

### 阶段 3：定义 actor 内聚合范围数据源

实现范围读取 helper/接口，供聚合查询统一使用。接口可以采用 `aggregateSource`，但必须满足：

- 输入范围使用 UTC Unix minute key，边界严格遵守现有左闭右开语义；
- bbolt cursor 从 `encodeInt64(start)` seek，到 `end` 前停止；处理空桶、非法 key、嵌套 bucket 的行为与现有校验一致；
- 在 actor command 内读取，避免和 flush/reset/restore 并发；一次查询内的数据视图一致；
- 查询合并内存 `a.dirty`/`a.data` 的未 flush 写入，避免漏掉近期记录或重复聚合；
- 冷数据读取后默认不常驻内存；若增加 LRU，容量必须有界，且 reset/restore/reconfigure/prune 能正确失效；
- 当前 `hours` 外桶是分钟粒度，不能按 24/168/720 个桶估算。24h、7d、30d 分别最多覆盖约 1,440、10,080、43,200 个分钟桶（受起止分钟、空桶影响）。

优先保留基于 map 的 source 作为测试 oracle；bbolt source 与 map source 对同一 fixture 必须逐字段等价。

### 阶段 4：迁移查询路径

逐项让以下查询从范围 source 读取，不能只改首页主 stats：

1. 完整 stats、summary、groups；
2. initial stats；
3. trends；
4. 分页 groups；
5. custom 秒级精确查询：仍以 requests 明细按精确时间重建，不得把分钟聚合误用于秒级边界；
6. costs：保持现有按 request detail 的时间范围语义，避免错误复用分钟聚合导致价格/费用改变；
7. CSV/导出及 Full Mode 使用的相同后端查询路径；
8. `retention` 全量范围明确允许全量范围读，结果必须完整。它可能仍然慢，应作为独立性能问题测量，不得以截断热窗口数据冒充完整结果。

UI/API schema 与统计口径保持不变。若某范围确实需要异步/分块响应，须另行设计并明确响应契约，本阶段不静默返回部分结果。

### 阶段 5：热窗口化 reload 与写入协调

1. 配置 `hot_window` 可暂不公开为用户配置；优先选择常量或内部默认值，以减少配置兼容面。窗口以时间长度表达，映射为分钟范围。
2. `reload()` 只把 `[now-hotWindow, now]` 的分钟聚合载入 `a.data`；冷数据留在 bbolt。
3. 初始化/恢复期间需确保窗口下界之前的数据仍可由范围 source 查询。
4. 新 usage 正常进入 actor 内存；即便 RequestedAt 落在热窗口之前或未来，必须遵循现有接受/保留边界，并保证查询结果不丢不重。迟到事件不得因只缓存 hot window 而无法 flush 或 catalog 计数错误。
5. `persistedAggregates` 只跟踪内存中热窗口内实际加载并持久化的 keys；冷范围查询不得错误修改 dirty/persisted 状态。
6. prune 需删除数据库中过期分钟桶，并同步删除内存中对应 key、model 索引引用、API Key catalog 引用；冷数据不在内存时仍必须正确完成。

### 阶段 6：异步 readiness（可选安全网，契约门控）

只有在检查目标 CLIProxyAPI/SDK 版本后确认 register 和 RPC 契约，才可实现以下内容：

- `register()` 快速返回与 `starting/ready/failed` readiness；
- 所有依赖 store 的 endpoint 行为；
- usage during startup 的严格拒绝、重试或持久化队列策略；
- shutdown/reconfigure/restore 的取消与 handover。

若 usage RPC 失败不保证重试，禁止仅返回 503 并声称无数据丢失；若实现队列，必须有界/持久、保持顺序、可观测、在 shutdown 时不丢且有回放幂等方案。若本阶段宿主契约无法确认，则不实施异步注册，并在交付说明标明；前述同步路径仍须通过范围加载降低成本。

## 4. 必须覆盖的状态与操作

实现者须逐一盘点并处理：

- store open / reopen / reconfigure / shutdown；
- flush、sync_on_record、定时 flush、retention prune；
- backup / restore / staged migration / rollback；
- reset；
- API Key rotation、labels、ciphertext catalog；
- ObservedModels；
- stats、groups、initial、trends、requests、costs、export、Full Mode；
- 空库、旧 schema、非法索引、部分索引、数据库读写错误。

不得因范围加载而改变公开统计结果、失败计数、维度过滤、source 过滤、exact custom 范围、价格账本、保留期或 API Key 脱敏语义。

## 5. 测试与验证

### 正确性

1. 对合成数据库保留旧全量 map 结果作为 oracle，比较 `24h`、`7d`、`30d`、custom、retention 的 stats/initial/trend/groups 所有字段。
2. 分钟边界：start/end 落在分钟中间、刚好边界、跨日/月/epoch 前后；验证左闭右开。
3. 写入协调：dirty 未 flush、sync_on_record、迟到 usage、flush 同时查询、prune 边界。
4. catalog/models 索引：缺失、损坏、重建失败与成功、恢复、reset、rotation、过期清理。
5. exact custom 秒级查询和成本查询必须继续读取 requests，并与当前实现逐字段一致。
6. 并发与生命周期：关闭、重配置、restore 与查询冲突时无死锁、数据竞争、重复计数或句柄泄漏。

### 性能

合成 fixtures 覆盖小库、十万级与百万级聚合/请求记录；报告：

- open/reload 耗时；
- `reload()` 解码的聚合记录数量，应与热窗口记录量相当而不是 retention 总量；
- 稳定 schema reopen 不解码 requests；旧库索引 rebuild 单独测量；
- 首次 24h/7d/30d/retention 查询耗时；
- 峰值 RSS；
- catalog/model index rebuild 的耗时与失败行为。

不设跨设备的绝对毫秒门槛。核心验收是稳定启动路径的扫描数量受热窗口约束，且完整范围查询结果不变。

### 命令

```powershell
go test ./internal/plugin -count=1
go test ./... -count=1
go vet ./...
git diff --check
```

若执行 race test 需要 CGO 且环境没有可用 C 编译器，应明确报告环境限制，不得把未运行写成通过。

## 6. 实施顺序与交付

建议依次完成阶段 0–5，再依据宿主契约决定阶段 6。按需范围读取和热窗口改造涉及 `persistence.go`、`aggregate.go`、`cost.go` 与测试，实施期间避免并行代理修改同一文件。

实现者完成后报告：

- 变更文件和各阶段完成情况；
- 性能前后测量及记录数证据；
- 全量语义一致性测试结果；
- 未完成阶段及原因；
- CPA/SDK 契约实际验证来源；
- 环境限制和剩余风险。

验收的最低标准：启动不再反序列化 retention 范围全部聚合记录；稳定 schema 不扫描 requests；所有公开查询仍返回完整且与原实现等价的结果；索引损坏不会静默破坏主数据或同步触发无界扫描；所有生命周期路径测试通过。
