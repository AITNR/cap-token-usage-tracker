# 完整模式价格同步/保存不再要求 CLIProxyAPI API Key 计划

- 关联：issue #89 修复后的后续需求（由 AITNR 提出）
- 编写日期：2026-11-25（Asia/Shanghai）
- 核查基线：main / 5efa0e2（v2.0.11 + #89 修复）
- 状态：**已实施（方案 A），待提交**

## 1. 现状核查

### 1.1 服务端其实已经不需要 API Key

| 端点 | 认证方式 | 实现位置 |
| --- | --- | --- |
| `GET /full-mode/prices` | 仅校验完整模式 session | `management.go:245`、`full_mode.go:155` |
| `GET /full-mode/prices/save`（stage=begin/chunk/commit） | 仅校验完整模式 session | `management.go:253`、`fullModeStagedPayloadResponse` |
| `GET /full-mode/prices/sync` | 仅校验完整模式 session | `management.go:258`、同上 |

`savePricesResponse`（`management.go:863`）只解析 `{prices, sync_settings}` 写库；
`syncPricesResponse`（`management.go:891`）只解析 `{source, models, sync_settings}` 后调用
`syncModelsDev`。两者都**不读任何 Authorization / API Key**。所以"保存价格与设置"和"同步
models.dev"在协议层已经是 session-only。

### 1.2 真正卡住用户的是浏览器端

`internal/plugin/dashboard.go`：

1. `fetchCLIModels()`（模板约 :609）第一行就强制读取 `cliModelsKeyInput.value`，为空直接
   `text('priceError', t('error.missingAPIKey'))` 并 `throw`，然后用
   `Authorization: Bearer <key>` 去请求宿主 `GET /v1/models`：
   `var modelsURL=publicPathPrefix+'/v1/models'`。
2. `syncPricing()`（:625）第一步就是 `var models = await fetchCLIModels(false)`，
   于是同步被 API Key 硬绑定。
3. `savePricing()`（:624）本身不碰 Key，但 `renderPricingEditor()` 的行来自
   `pricingNames() = cliModels ∪ 价格表 ∪ 手动添加`。新装环境价格表为空、也没有手动添加时，
   编辑器是空列表，`pricing.loadModelsHint` 提示"先加载模型"——用户感知就是"保存也必须先给 Key"。
4. 价格对话框（`<dialog id="pricingDialog">`）只存在于完整模式页面，普通面板已被
   `withoutTemplateSection` 移除，因此这个 API Key 输入框**只在完整模式里存在**，且每次打开
   都要重新粘贴。

### 1.3 结论

用户诉求成立：完整模式打开后（session 有效）就不该再要 CLIProxyAPI API Key。缺的不是认证，
而是一份"要定价哪些模型"的清单。

## 2. 目标

1. 完整模式下，打开价格对话框、同步 models.dev、保存价格与设置，全部只需要有效 session，
   全程不出现 API Key 输入。
2. 清单来源改为插件自有的权威数据：**已观测到用量的模型 ∪ 现有价格表模型**。
3. 不放松任何认证：`/full-mode/*` 依旧 session-only，未解锁时照旧 401。
4. 普通面板（`/dashboard`）不受影响，仍无价格 UI。

## 3. 方案

### 3.1 新增 `GET /full-mode/models`（session 保护）

请求：`GET resourceBase + '/full-mode/models'`，`X-Full-Mode-Session: <session>`。

响应 200：

```json
{
  "models": ["claude-opus-4", "gpt-5.2", "qwen3-max"],
  "observed": 12,
  "priced": 9,
  "generated_at": "2026-11-25T05:20:00Z"
}
```

- `models`：`Store.ObservedModels()`（保留期内的用量模型，`persistence.go:603`，目前只有测试在用）
  ∪ `QueryPriceBook().prices` 的键；去重、排序、上限 `maxModelPriceEntries`（10000）。
- `observed` / `priced`：两个来源各自的数量，供前端提示"清单来源"。
- 无 session 或已过期：401 `{"error":"full-mode session is missing or expired"}`，与其它
  full-mode 资源路由一致。
- 空清单合法（新装、无用量、无价格）：返回 `{"models":[]}`，不报错。

路由登记：`management.go` 的 `registeredRoutes` 增加 `fullModeModelsPath`，
`Resources` 增加 `/full-mode/models`（Resources 数量 20 → 21，需同步改
`management_test.go:37`）。

### 3.2 同步允许缺省模型清单

`syncPricesResponse`：`models` 字段变为可选。为空时服务端回退到 3.1 的同一份派生清单
（抽成 `observedPriceModels()` 供两处复用）。派生清单仍为空时返回 400，错误文案改为
不涉及凭据的中性描述：`"no models to synchronize yet; record usage or add a model manually first"`。

`syncModelsDev` / `normalizeSyncModels` 的语义不变（仍需至少一个模型），只是把"必须由客户端
提供"改成"缺省时用服务端清单"。

### 3.3 前端改造（`dashboardHTMLTemplate`）

1. **删除** API Key 区块：`<label class="pricing-key-label">…cliModelsKeyInput…</label>`、
   `loadCLIModels` 按钮原来的"加载当前模型"语义，以及 `var cliModelsKeyInput=document.getElementById(...)`。
2. `modelsURL` 改为 `resourceBase+'/full-mode/models'`（保留 `publicPathPrefix` 拼接方式，
   反代路径前缀契约不变）。`api()` 已按 `'/full-mode/'` 前缀自动注入 session，无需额外处理。
3. `normalizeCLIModels(payload)` 支持新形状 `{models:[...]}`，并保留对旧形状
   `{data:[{id}]}` 的容错（去掉对 `/v1/models` 的依赖后此分支理论上是死代码，保留成本极低）。
4. `fetchCLIModels(renderEditor)`：去掉 Key 校验与 `Authorization` 头；失败时只更新
   `modelCatalogStatus` 为 `pricing.catalogFailed` 并在 `priceError` 里显示原因，不抛出中断流程。
5. `openPricing()`：打开对话框后自动 `fetchCLIModels(true)`（失败静默降级为空清单 + 提示），
   用户不必先点任何按钮。
6. `syncPricing()`：先刷新清单（复用 `fetchCLIModels`），清单为空则在本地提示
   `pricing.catalogEmpty` 并中止，不发无意义请求。
7. `clearCLIModelState()`：不再清空输入框（已无输入框）。
8. 对话框结构：`model-source-controls` 改为一行只读说明 + "重新载入清单"按钮
   （沿用 `id="loadCLIModels"`，减少无谓 diff）；`pricing.description`、`pricing.modelSourceHint`
   文案去掉"CLIProxyAPI API Key / Management Key"字样。

### 3.4 文案（4 个语言包，key 集合保持一致）

| key | 变更 |
| --- | --- |
| `pricing.description` | 改为"模型清单来自本插件已记录的用量模型与现有价格表；价格固定为 USD / 1M Tokens" |
| `pricing.apiKey` | 删除（不再有输入框） |
| `error.missingAPIKey` / `error.missingKey` | 删除（仅 `fetchCLIModels` 使用过） |
| `pricing.modelSourceHint` | 改为"清单 = 已记录用量的模型 ∪ 现有价格表；未出现在清单中的模型可手动添加" |
| `button.loadModels` | 文案改为"重新载入清单"（key 保留，避免大面积改引用） |
| `pricing.catalogNotLoaded` | 改为"模型清单：尚未载入" |
| `pricing.catalogEmpty`（新增） | "还没有可同步的模型：先产生用量或手动添加模型" |

### 3.5 安全

- `/full-mode/models` 只暴露模型名（本就在 `/stats`、`/requests` 的响应里），且需要 session。
- 不做"服务端拿管理密钥/API Key 代调宿主 `/v1/models`"：插件运行时既拿不到宿主地址，也不
  保存管理密钥（现有设计明确只留不透明 session），这条路要改安全模型，拒绝。
- 保持 CSP、响应头、session TTL、分片上传限制不变。

## 4. 测试计划

| 类型 | 用例 |
| --- | --- |
| 单元 | `TestFullModeModelCatalogRequiresSession`：无/过期 session → 401；有效 session → 200 且 `models` = 观测模型 ∪ 价格表键（去重排序） |
| 单元 | `TestFullModePriceSyncFallsBackToObservedModels`：payload 省略 `models` → 用派生清单；派生清单为空 → 400 且错误文案不含 "CLIProxyAPI model" |
| 单元 | `TestFullModePriceSaveAcceptsSessionWithoutAPIKey`：仅带 session 的保存成功（回归锁死"不需要 Key"） |
| 契约 | `TestDashboardUsesExactBackendCostsAndPricingSync`：删 `id="cliModelsKeyInput"`、`var modelsURL=…'/v1/models'`、`cliModelsPromise=api(modelsURL` 断言；新增 `resourceBase+'/full-mode/models'`、`pricingNames()` 联合来源断言 |
| 契约 | `TestDashboardPreservesReverseProxyPathPrefix`：`modelsURL` 断言改为新端点 |
| 注册 | `management_test.go`：Resources 20 → 21，新增 `/full-mode/models` 路径断言 |
| 浏览器 | `test/dashboard_time_pricing.mjs`：桩服务增加 `/full-mode/models`（返回 2 个模型），断言"打开对话框即自动载入清单、无任何凭据输入、保存成功" |
| 浏览器 | 新增 `test/dashboard_pricing_without_key.mjs` 场景：断言页面 DOM 中不存在 `#cliModelsKeyInput`，同步按钮在无凭据下发出 `/full-mode/prices/sync` 且 payload 使用服务端/SDK 清单 |

## 5. 兼容性与风险

- 旧前端（未升级的缓存页面）继续带 `models` 字段调 sync → 行为不变。
- 升级后清单来自观测数据：**从未使用且未定价的模型不会出现在编辑器里**，需手动添加
  （已有 `manualModelInput`）。这是本次唯一的可见行为变化，会写进提示文案。
- 观测模型受"保留期"限制（默认 365 天），超出保留期的历史模型可能不在清单里；已定价的不受
  影响（价格表键独立保留）。
- 旧数据/旧备份无需迁移，bbolt 结构不变。

## 6. 非目标

- 不新增"用管理密钥代取宿主 `/v1/models`"的通道。
- 不改变 models.dev 匹配算法、价格表结构、revision 语义。
- 不改动同步设置（provider priority / ignored suffixes / mappings）的语义与校验。
- 不把价格 UI 带回普通面板。

## 7. 验收标准

1. 完整模式打开 → 点"模型价格"→ 清单自动出现，无 API Key 输入框。
2. 点"同步 models.dev" → 无任何凭据输入即成功，价格表与 revision 更新。
3. 修改价格/同步设置 → "保存"成功，无需任何凭据。
4. 未解锁（无 session）直接请求 `/full-mode/models`、`/full-mode/prices*` → 全部 401。
5. `gofmt` / `go vet` / `go build` / `go test ./...`（含全部浏览器用例）在 CI 全绿。

## 8. 实施记录

按方案 A 落地，与计划的两处偏差：

1. **同步缺省清单只作用于完整模式路由**。`syncPricesResponse`（管理接口 `/v0/management/.../prices/sync`）
   保持原契约：`models` 为空仍返回 400。该接口已由管理密钥鉴权、不存在凭据问题，改它会破坏
   对外契约。完整模式走 `fullModeSyncPricesResponse` → `syncPrices(request, true)` 才启用回退。
2. **`normalizeCLIModels` 不再因空清单抛错**。旧实现把"宿主没有模型"当成错误，新清单在全新
   安装时合法为空，改为返回空数组，由 `pricing.catalogEmpty` / `pricing.loadModelsHint` 提示。

文件清单：

| 文件 | 变更 |
| --- | --- |
| `internal/plugin/model_catalog.go` | 新增：`modelCatalog` 类型、`validCatalogModel`、`modelCatalog()`、`fullModeModelCatalogResponse()` |
| `internal/plugin/management.go` | 新增 `/full-mode/models` 路由登记与分发；`syncPrices` 拆出 `allowCatalogFallback` |
| `internal/plugin/dashboard.go` | 删除 API Key 输入与 `/v1/models` 直连；自动载入清单；空清单本地拦截同步 |
| `internal/plugin/locales/*.json` | 4 语言文案更新（删 4 键、增 2 键、改 12 键） |
| `internal/plugin/model_catalog_test.go` | 新增 3 个用例（session-only 清单、同步回退、空清单拒绝） |
| `internal/plugin/management_test.go` | Resources 20→21、新增 `/full-mode/models` 断言 |
| `internal/plugin/dashboard_test.go` | 价格契约与反代前缀契约更新，新增"禁止 API Key 输入/`/v1/models`"断言 |
| `test/dashboard_time_pricing.mjs` | 桩服务支持 `/full-mode/models` 与同步分片；断言无 Key 输入、清单自动载入、同步不带 Authorization 且用服务端清单 |
| `README.md` | 中英文价格说明改为清单派生语义 |

验收结果：`go build` / `go vet` / `go test ./...`（含 15 个浏览器用例，38s）全绿。

