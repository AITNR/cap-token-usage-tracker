# 完整模式价格清单：支持从宿主读取模型 计划

- 关联：`docs/full-mode-pricing-without-api-key-plan.md`（已实施，清单 = 观测模型 ∪ 价格表）
- 编写日期：2026-11-25（Asia/Shanghai）
- 核查基线：main / 01b0477
- 状态：**已按方案 A 实施，待提交**

## 1. 需求

当前价格编辑器只有两个来源：插件自动推导的清单（已使用过的模型 ∪ 已定价模型）和手动添加。
要求补上第三个来源：**读取宿主当前提供的模型列表**（等价于 CLIProxyAPI `GET /v1/models`），
让用户能给"还没用过、也没定过价"的模型预先定价。

## 2. 关键结论：插件无法免凭据读取宿主模型

这一点必须先说清楚，它决定方案形态。逐条核对过宿主与插件的接口：

| 可能路径 | 结论 | 证据 |
| --- | --- | --- |
| 进程内模型注册表 `cliproxy.GlobalModelRegistry().GetAvailableModels()` | **不可用** | 插件以 `-buildmode=c-shared` 构建（`scripts/build.sh`），经 C ABI（`main_cgo.go` / `bridge.c`）加载，是独立 Go 运行时；`GlobalModelRegistry()` 只能取到插件自己那份空注册表 |
| 宿主回调 `host.*` 方法 | **无模型列表方法** | `sdk/pluginabi/types.go:95-110` 只有 `host.http.do`、`host.auth.*`、`host.model.execute` 等，没有列模型 |
| `host.auth.list` 取凭据里的模型 | **不含模型** | `pluginapi.HostAuthFileEntry` 只有 ID/Provider/BaseURL/状态等字段，无 models |
| 服务端自行请求宿主 `/v1/models` | **不可行** | `ManagementRequest` 不含宿主地址，插件配置（`config.go`）也没有 base_url/端口；且该接口要求**客户端** API Key，与管理密钥、完整模式 session 都不是同一套凭据 |
| 复用解锁完整模式用的管理密钥 | **不可行** | 设计上刻意不保存管理密钥（只留不透明 session，见 README），且管理密钥不能用于 `/v1/models` |

所以"/v1/models 需要凭据"是**宿主自身的鉴权要求**，插件无法绕开。能免凭据做到的只能是
"从插件自己的记录推导"——这就是已实现的自动清单。

要真正读到宿主全量模型，只有两条路：浏览器带一个 CLIProxyAPI API Key 直连（本计划的方案 A），
或把该 Key 写进插件配置由服务端代调（方案 B）。

## 3. 方案 A（推荐）：浏览器直连，Key 可选、只在页面内存

### 3.1 交互

价格对话框的"模型清单"区域改为三源结构：

```
[从宿主读取模型]  ← 可选 API Key 输入框（password，不保存、不落库）
模型清单：自动 12 个 · 宿主导入 87 个 · 手动 1 个
提示：宿主导入仅在本次页面有效，保存价格后并入价格表，下次自动出现在清单里
```

- 不填 Key：按钮点击后提示"读取宿主模型需要 CLIProxyAPI API Key"（`pricing.hostKeyRequired`），
  其余功能完全不受影响。
- 填 Key：`GET <publicPathPrefix>/v1/models`（`Authorization: Bearer <key>`），把结果并入编辑器，
  标记来源 `host`；已存在的模型不重复。
- 同步 models.dev 与保存价格/设置**仍然只用 session**（保持上一个改动的成果）。

### 3.2 数据与状态

- 新增页面内存 `hostModels`（`Set`）与 `hostModelsLoadedAt`，不写 `localStorage`/`sessionStorage`。
- `pricingNames()` = `cliModels`（自动清单）∪ `hostModels` ∪ 价格表键 ∪ `manualDraftModels`。
- 导入的模型若不填价格就保存，不会写入价格表（现有 `collectPricing` 逻辑：0 价视为未定价？
  需按现状确认，见 3.4）。
- 刷新/重开页面后宿主导入丢失，但已保存价格的模型会通过自动清单继续出现。

### 3.3 前端改动点

1. `model-source-controls`：加回 `input#hostModelsKeyInput` + `button#loadHostModels`，保持
   `#loadCLIModels`（重新载入自动清单）不变。
2. `fetchHostModels()`：校验 Key → 请求 `/v1/models` → `normalizeCLIModels` → 合并 → 重渲染；
   错误映射到 `#priceError`，不清空已有清单。
3. `pricingNames()`、`renderPricingEditor()`、`capturePricingEditor()` 兼容第三来源。
4. `clearCLIModelState()` 不清空宿主输入框（Key 由用户决定是否保留到页面关闭）。
5. 来源标记：价格行里已有 `pricing.notInCatalog` 之类的提示逻辑，需要区分"自动清单里没有"与
   "宿主导入才有"。

### 3.4 已核实与仍需处理的点

- **0 价行不会入库**（已核实 `collectPricing()`）：只有当模型已在价格表、属于手动添加、或任一费率/
  分层/时间档非零时才会提交，宿主导入但未填价格的模型不会被写库，也不会污染覆盖率统计。这一条
  在浏览器用例里加断言锁住，防止以后改动破坏。
- 仍需处理：导入几十上百个模型时编辑器行数暴涨，需要沿用 `maxModelPriceEntries` 上限并在提示里
  说明截断；来源标记要区分"自动清单里没有"与"只有宿主导入才有"。

### 3.5 文案（4 语言）

新增：`button.loadHostModels`、`pricing.hostKey`、`pricing.hostKeyHint`、`pricing.hostKeyRequired`、
`pricing.hostLoading`、`pricing.hostLoaded`、`pricing.hostFailed`、`pricing.sourceHost`。
调整：`pricing.modelSourceHint`（说明三源）。

## 4. 方案 B（备选）：Key 写进插件配置，服务端代调

- 配置新增 `host_base_url`（默认 `http://127.0.0.1:<port>`）与 `host_api_key`，插件服务端自行
  `GET /v1/models`，`/full-mode/models` 直接返回合并后的全量清单。
- 优点：用户完全不用在浏览器里粘贴 Key；导入结果对所有设备一致。
- 代价与风险：**客户端 API Key 会以明文存进插件配置**（与"Key 只在内存"的既有安全取向冲突），
  需要额外的配置校验、脱敏、日志隔离；还要处理 base_url 自签证书/回环白名单。
- 结论：不推荐作为默认实现；如需要，应单独立项并做安全评审。

## 5. 测试计划

| 类型 | 用例 |
| --- | --- |
| 单元 | `pricingNames()`/`collectPricing()` 三源合并；0 价行不入库；超限截断 |
| 契约 | `dashboard_test.go`：`#hostModelsKeyInput`、`/v1/models` 直连仅出现在 `loadHostModels` 路径；同步/保存路径不含 `Authorization` |
| 浏览器 | 新场景 `dashboard_pricing_host_models.mjs`：①不填 Key 点读取 → 本地提示、无请求；②填 Key → 请求带 Bearer、清单合并、行数增加；③保存只写入有价格的行；④同步仍不带 Authorization |
| 回归 | 现有 `dashboard_time_pricing.mjs` 全部断言保持通过 |

## 6. 非目标

- 不改变"同步 models.dev / 保存价格与设置 只需要 session"的现状。
- 不保存宿主 API Key（方案 A 前提下）。
- 不改 `/full-mode/models` 的服务端清单语义（仍是观测 ∪ 价格表）。

## 7. 验收

1. 打开价格对话框：自动清单可用，无需任何凭据。
2. 填一次 Key 点"从宿主读取模型"：清单出现宿主全量模型，可逐行定价；Key 不落库、刷新即失效。
3. 不填 Key 点该按钮：只出现本地提示，不产生请求，不影响其它功能。
4. 同步 models.dev：仍无 Authorization 头。
5. `gofmt` / `go vet` / `go build` / `go test ./...`（含浏览器用例）全绿。

## 8. 决策

- 采用**方案 A**：由用户自己在对话框里输入 CLIProxyAPI API Key，仅用于一次 `/v1/models` 请求，不落库。
- 不做"高级折叠"：需求就是让用户能自己填，折叠反而像隐藏功能。

## 9. 实施记录（方案 A）

已按方案 A 落地，宿主 Key 由用户在对话框内自行输入，不做折叠（保持一行可见，文案标注"可选，不保存"）。

| 文件 | 变更 |
| --- | --- |
| `internal/plugin/dashboard.go` | 新增 `#hostModelsKeyInput` / `#loadHostModels` / `#hostModelsStatus`；`fetchHostModels()` 带 `Authorization: Bearer <key>` 请求 `publicPathPrefix+'/v1/models'`，结果并入 `hostModels`；`pricingNames()` 变三源；`notInCatalog` 判定同时认宿主清单；`setPricingBusy` 一并禁用 |
| `internal/plugin/locales/*.json` | 新增 `button.loadHostModels`、`pricing.hostKey`、`pricing.hostKeyHint`、`pricing.hostKeyRequired`、`pricing.hostLoading`、`pricing.hostLoaded`、`pricing.hostFailed`（4 语言） |
| `internal/plugin/dashboard_test.go` | 契约新增宿主导入标记；锁定全页仅两处 `Authorization`（session 握手 + 宿主导入），且不含 `Bearer '+managementKey` |
| `test/dashboard_pricing_host_models.mjs` + runner | 新浏览器用例：无 Key 点击不发请求、带 Key 请求带 Bearer 并合并清单、保存只写有价格的行、同步不带凭据、刷新后 Key 消失而价格保留 |
| `README.md` | 中英文补充"可选宿主导入"的说明与 Key 使用范围 |

与计划的差异：

- 未做"高级折叠"：需求是让用户自己输入 Key，折叠会让人以为功能不存在；改为同屏一行 + "可选，不保存"标注。
- 3.4 的 0 价过滤经核实已由 `collectPricing()` 保证，本次只补了浏览器断言，没有改保存逻辑。

验收：`go build` / `go vet` / `go test ./...`（含 16 个浏览器用例，42.9s）全绿。

