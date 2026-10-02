# Issue #80 修复方案：降低大 bbolt 数据库的启动阻塞

- Issue：<https://github.com/AITNR/cap-token-usage-tracker/issues/80>
- 文档性质：基于当前工作区代码的修复设计与已实施核心修复记录。存储层 catalog/backfill、稳定 reopen 和周期 prune 优化已落地；异步 readiness 仍未实现。
- 证据边界：Issue #80 报告了插件 `v2.0.8`、CLIProxyAPI `v8.0.4`、Linux amd64、约 1.8 GiB 数据库及约 8 分钟启动现象。这些是报告环境，不是本地复现实测结果。下文的函数、控制流和遍历结论来自当前工作区；宿主 RPC 的异步注册契约仍需在目标 CPA 版本上验证。

## 1. 已确认的调用链与扫描事实

当前注册路径是同步的：

```text
pluginRuntime.register()
  -> applyConfig()
     -> openStoreWithCrypto()
        -> storeActor.initialize()
           -> bbolt Update：清理、迁移、generation 状态
           -> storeActor.reload()
```

`internal/plugin/lifecycle.go` 中的 `register()` 在 `lifecycleMu` 保护下调用 `applyConfig()`；数据路径需要打开新 store 时，`openStoreWithCrypto()` 会在返回前执行 `actor.initialize()`。因此，`initialize()` 的耗时处于本插件注册请求的同步路径。Issue #80 进一步报告 CPA 会等待该注册完成后才进入 ready；这是待在实际 CPA/SDK 版本上回归确认的宿主行为。

### 1.1 启动时 `requestsBucket` 的已确认成本

`internal/plugin/persistence.go` 的 `initialize()` 会先执行一次写事务：

1. `pruneRequestsBucket()` 用按时间和序号排序的 request key 游标删除过期前缀；它在遇到首个未过期 key 后 `break`，因此不是必然读取全部保留 request value，也不会在该函数中 JSON 解码 `RequestDetail`。过期记录很多时，删除成本仍可能显著。
2. `migrateAPIKeyCryptoSchema()` 在 `version >= 8` 时调用 `validateAPIKeyGenerationReferences(hours, requests, generations)`。该校验遍历 `requests` 并 JSON 解码每条 `RequestDetail`，同时遍历小时聚合维度。即使没有新的 schema 迁移，当前代码路径仍会执行此校验。
3. 写事务完成后 `reload()` 再次调用同一个 `validateAPIKeyGenerationReferences()`，再次全量遍历并解码当前保留的 request records。
4. `reload()` 之后独立执行 `requests.ForEach`，第三次遍历并 JSON 解码每条 request，用于重建 `a.apiKeyCiphertexts`，并把 request 引用补入 `retainedHashes`。

因此，对 schema 已处于 `>= 8` 的常规启动，清理过期前缀之后，当前代码至少存在 **三次** 对保留 `requestsBucket` 的全量 JSON 解码遍历：迁移函数内 generation 引用校验、`reload()` 内重复校验和 `reload()` 的 ciphertext/引用重建。这个结论不依赖对单条记录大小或记录数量的估算。

不能仅凭该结论断言 Issue 中约 8 分钟全部由这三次遍历造成：数据库页缓存、磁盘性能、bbolt 写入/删除、其他迁移和聚合桶数量同样会影响总耗时，应通过阶段计时和基准测试量化。

### 1.2 周期 prune 的已确认成本

`reload()` 把 `lastPruneAt` 重置为零值。actor 的下一次 flush tick 会计算 `shouldPrune := lastPruneAt.IsZero() || now.Sub(lastPruneAt) >= time.Hour`，所以初始化后的**首个 scheduled flush**会进入 prune 分支；具体等待时长等于实际 `FlushInterval`，不能在本方案中假设为固定秒数。

该分支会：

1. 调用 `pruneHoursBucket()`；当前实现对所有顶层小时 bucket 执行 `ForEach`，收集过期 bucket 后删除。
2. 调用 `pruneRequestsBucket()`；它只扫描并删除过期 request key 前缀。
3. 调用 `retainedAPIKeyState(hours, requests)`；该函数遍历全部保留小时聚合维度，并遍历、JSON 解码全部保留 request records，重建保留的 API Key 引用集合与 ciphertext map。
4. 根据该集合删除失效 label，并把新的 ciphertext/label 状态发布回 actor。

因此，除启动时的重复扫描外，首次 flush 以及其后满足一小时间隔的 prune 都会对保留 request records 再做一次全量解码遍历。

## 2. 必须保留的语义与约束

优化不能简单删掉 request 扫描：

- API Key 引用由 `APIKeyGeneration` 和 `APIKeyHash` 共同组成；generation 记录在 `meta` 中，label 也存于 `meta`。
- usage 记录路径会把启用 crypto 时的 API Key 转为 ciphertext，并把 ciphertext 随 `RequestDetail` 写入 `requests`。当前启动与 prune 正是从 request records 重建 `a.apiKeyCiphertexts`。
- 当前 label 保留逻辑使用小时聚合和逐请求记录引用的并集。只从 `hours` 推断保留引用会改变现有语义，除非先用测试证明两者在所有兼容版本、迁移和异常恢复场景中严格等价。
- generation 引用校验是损坏/不兼容数据的保护。若从普通启动移除全库校验，必须保留 schema 升级、备份 restore 和显式完整性检查中的完整校验路径。
- `reconfigure`、backup、restore、store lease 和 `sync_on_record` 的现有行为均依赖 actor 的同步持久化边界，不能为了加速启动而发布半初始化状态或伪造写入成功。

## 3. 推荐修复：先做同步存储层去重与索引化

先减少明确存在的 I/O 和解码，而不是先改变宿主生命周期。这样可降低常规启动与周期 prune 成本，同时不假定 CPA 接受异步 `register()`。

### 3.1 消除同一次启动中的重复 generation 校验

将 API Key generation 引用校验拆为两个明确语义：

- **迁移/导入完整校验**：当数据库从需要重写 API Key 引用的旧 schema 升级、执行 restore 后验证，或管理员显式请求完整性检查时，保留一次覆盖 `hours` 和 `requests` 的完整校验。
- **常规 reopen**：已处于当前 schema 的本地数据库不得在 `migrateAPIKeyCryptoSchema()` 与紧随其后的 `reload()` 各进行一次完整校验。实现可以将本次初始化已经完成的验证结果传给 reload，或让 migration 函数在没有 schema 工作时不再做验证；两者择一，但必须保证常规 reopen 的完整 request 校验至多一次，最好结合 3.2 变为零次。

不要使用静默忽略错误的方式替代校验。若选择跳过常规 reopen 的完整校验，必须把完整校验暴露为可诊断、可运行且可测试的维护/restore 路径，并记录为什么该数据库被视为已完成迁移。

### 3.2 持久化 API Key catalog，替代常规 request 全表重建

新增 versioned `meta` catalog，例如以 `generation + hash` 为 key，保存当前行为所需的最小状态：ciphertext、是否仍被小时聚合引用、是否仍被 request 明细引用，以及迁移版本或完整性状态。名称和编码在实现阶段确定。

写入与删除必须与原始 bucket 更新在同一 bbolt 写事务中：

- flush 写入小时聚合和 pending requests 时，更新对应引用及最新 ciphertext。
- prune 删除过期小时 bucket 或 request record 时，基于即将删除的 key/value 更新相应来源的计数或引用状态。只解码被删除的过期记录；不得重新扫描全部未过期 records。
- 当两个来源都不再引用某个 ref 时，再清理 catalog 中的 ciphertext，并按与当前 `retainedAPIKeyState()` 相同的并集规则清理 `apiKeyLabelsKey`。

对已有数据库，不能假设从最新少量 request 倒序就能找到所有仍保留的 API Key：长期未使用但仍在保留期内的 key 是合法情况。首次引入 catalog 时应采用以下两种明确策略之一：

1. **同步一次性 backfill**：完整扫描当前保留 hours 与 requests，原子写入 catalog；首次升级仍可能慢，但之后的正常重启与 prune 不再扫描全部 request records。必须提供阶段日志和测试。
2. **受控后台 backfill**：仅在确认 CPA 对 readiness、usage 和失败状态的契约后使用。backfill 完成前不得根据不完整 catalog 清理 label/ciphertext，也不得把数据称为 ready。

推荐先实现策略 1，因为它不改变注册/错误语义，风险最小。

### 3.3 避免初始化后立即重复 prune

`initialize()` 已按当前 cutoff 清理 hours 和 requests。成功完成 reload 后，可将 `lastPruneAt` 设为该初始化使用的时间，而不是保留零值，避免首个 scheduled flush 无条件重复一次 prune。

该改动的前提是初始化成功执行了与 flush 等价的 retention/catalog 清理；如果初始化失败、配置在中间变更或 catalog backfill 未完成，则必须保留零值或显式标记为需要 prune。

### 3.4 对 hours prune 的谨慎优化

`pruneHoursBucket()` 当前遍历全部顶层 hour buckets。若 hour key 的编码和 bbolt 排序已由单元测试证明为按时间单调递增，可改为 cursor 从最早 hour 走到 cutoff 后立即停止，以避免检查保留期内的小时 bucket。

此优化在实施前必须加入包含负/正时间边界的排序测试；没有该证明时，不应仅凭 `encodeInt64` 的名称假设可安全 early break。

## 4. 阶段日志与可验证的性能目标

在修改前后都应对以下阶段采集结构化日志/metrics：`open_db`、`prune_hours`、`prune_requests`、`migration`、`validate_api_key_refs`、`load_hours`、`load_requests`、`catalog_backfill` 和总初始化耗时。至少记录 duration、bucket/record count、是否为首次 catalog backfill、schema version 与结果；不得记录明文 API Key、ciphertext、请求正文或凭据。

不要承诺固定的 200ms 或 50ms 启动时间：当前仓库没有可复现的 1.8 GiB 基准数据库、宿主计时和存储硬件数据。应采用下列可验收目标：

1. catalog 已存在且数据库 schema 已稳定的普通 reopen，对未过期 `requestsBucket` 的完整 JSON 解码遍历次数为 **0**；允许仅扫描过期 key 前缀和实际写入/删除的 records。
2. 同一合成 fixture 上，记录修复前后 `register()` 返回时间与各阶段耗时，报告 median 和 p95；不将未测得的绝对时延写入 release 承诺。
3. 首次 catalog backfill 单独计时，且完成后第二次 reopen 满足目标 1。
4. 加入数据库大小、request 数量、hours 维度数、保留期和冷/热缓存状态，以便结果可解释。

## 5. 异步 readiness：后续增强，非本期前提

Issue #80 建议让 `register()` 快速返回、后台加载数据库、管理接口返回 503。该方向有价值，但在本仓库中尚不能证明 CPA 的 plugin RPC 协议允许注册成功后再异步发布依赖 store 的能力，也不能证明宿主会如何重试 usage 或处理 503。

在实施前必须针对目标 CPA/SDK 版本验证：

- `register()` 返回成功后，宿主是否允许插件仍处于 starting；
- usage 回调和管理路由的并发/重试/超时契约；
- reconfigure、shutdown、restore 与后台打开同一 bbolt 路径时的句柄和 lease 规则；
- 管理 API 的路由注册时点，以及 503 是否会被 dashboard/宿主正确呈现。

只有通过这些验证后，才考虑带 generation 和取消机制的 `starting -> ready | failed` 状态机。初始化期间 usage 的策略必须明确为有界队列或拒绝并返回可识别错误；绝不能在尚未持久化时返回成功。即使采用异步 readiness，3.1 至 3.3 的存储层优化仍然需要保留，因为后台全表扫描会继续消耗 I/O 并延迟 ready。

## 6. 实施顺序

1. 为现有路径加入阶段计时和扫描计数，建立小库、合成大库与升级 fixture 的基线。
2. 编写 catalog 编码、事务更新与完整 backfill；先保持 actor 生命周期同步。
3. 调整 normal reopen 的 generation 校验策略，保留升级、restore 和显式完整性检查的全量验证。
4. 用 catalog 替换 `reload()` 与周期 prune 中对全部未过期 request records 的重建扫描。
5. 仅在排序测试通过后优化 `pruneHoursBucket()`，并让成功初始化抑制首次无条件周期 prune。
6. 完成兼容/性能验证后，再单独设计和评审异步 readiness。

## 7. 测试与验收

### 功能与兼容性

- 旧 schema 升级、当前 schema reopen、空库、损坏 generation 引用、缺失/损坏 catalog、backup/restore、reconfigure、shutdown 与 store handover。
- 多 generation、相同 hash 的不同 generation、label 存在/过期、ciphertext 仅来自历史 request、小时与 request 引用不一致的 fixture。
- request 或 hour prune 后 catalog、label、ciphertext 与当前 `retainedAPIKeyState()` 的结果完全一致。
- `sync_on_record`、Query、RequestDetails、成本计算、导出和 dashboard 的既有结果不回归。

### 扫描与性能

- 对预填充大量未过期 request 的普通 reopen，使用测试 hook、受控 wrapper 或阶段计数断言：不调用全量 request `ForEach`/全量 RequestDetail JSON 解码。
- 对首次 catalog backfill，断言完整扫描只发生一次；关闭再打开后不重复。
- 对周期 flush，断言无写入但到 prune 时不会扫描所有未过期 request records；只处理过期前缀和必要 catalog 更新。
- 跑完整 Go 测试套件，并在可用的 CPA 集成环境中记录 Issue 复现步骤的启动阶段日志。

### 已执行验证与发布前验收标准

- 修复后普通 reopen 的 request 全量解码遍历为零，首次 backfill/完整性检查例外且明确可观察。
- API Key labels、ciphertexts 和 generation 引用的保留语义与当前实现等价。
- 没有遗留 bbolt 句柄、lease、数据竞争、伪成功 usage 或 schema 兼容性破坏。
- 已执行 `go test ./internal/plugin -count=1`、针对性 catalog/prune/API Key 测试和 `go test ./... -count=1`；未验证的 CPA 异步行为不作为已实现事实发布。

## 8. 已实施范围与剩余工作

本次实际修改了 `internal/plugin/persistence.go` 与 `internal/plugin/persistence_test.go`：新增 schema v10 的 `api_key_catalog` 元数据，旧 schema/缺失 catalog 首次打开时完整 backfill；稳定 reopen 从 catalog 恢复 API Key ciphertext，不再对保留 `requestsBucket` 全量 JSON 解码；周期 prune 只解码即将删除的过期 request，并增量维护 hours/request 引用计数；保留 restore 路径的完整校验。未修改宿主 RPC 或异步 readiness。
