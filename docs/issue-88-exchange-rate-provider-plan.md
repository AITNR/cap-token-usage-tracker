# Issue #88：USD/CNY 汇率可切换上游（AllRatesToday）计划

- Issue：https://github.com/AITNR/cap-token-usage-tracker/issues/88
- 编写日期：2026-11-25（Asia/Shanghai）
- 核查基线：main / ede7e66
- 状态：**待审**

## 1. 提案内容

请求新增可选配置 `exchange_rate_provider: allratestoday`，把面板 USD/CNY 汇率源从
`https://open.er-api.com/v6/latest/USD` 换成 `https://allratestoday.com/api/open/latest/USD`；
默认仍为 `open-er-api`。提案人称两者 JSON 形状一致、现有 fetcher 与校验可直接复用，并愿意提 PR。
提案人自述是该服务的开发者（利益披露）。

## 2. 实测核对（2026-11-25）

| 项目 | AllRatesToday | open.er-api.com |
| --- | --- | --- |
| HTTP | 200，`application/json`，约 1.1 KB | 200，同格式 |
| `result` / `base_code` | `success` / `USD` | `success` / `USD` |
| `time_last_update_unix` | ECB 2026-11-24 15:00 UTC | 2026-11-25 00:02 UTC |
| `rates.CNY` | **6.6645079** | **6.678179** |
| 额外字段 | `provider`、`source`（标注 ECB）、`date`、`attribution`、`attribution_html` | `provider`、`time_eol_unix` 等 |

结论：

1. 现有 `exchangeRateProviderResponse` 结构体能直接解析新源，多出的字段被忽略；`result`/`base_code`/
   `rates.CNY` 三项校验全部通过。提案人的核心判断成立。
2. 两边 CNY 相差约 **0.2%**（6.6645 vs 6.6782）。ECB 参考汇率按**工作日 15:00 UTC** 定盘一次，
   周末与节假日不更新，因此周末拿到的可能是 1-3 天前的定盘价。
3. 响应带署名要求（`attribution` / `terms_of_use` 指向 `#attribution`），接入时来源必须可见。

### 2.1 中国大陆直连可达性实测（2026-11-25，禁用代理）

用户在国内，需要先确认这个源能不能稳定访问。本机禁用代理后连续采样：

| 指标 | allratestoday.com | open.er-api.com（现默认） |
| --- | --- | --- |
| 出口 IP / CF 边缘 | 104.21.95.43 / LAX | 104.26.5.5 |
| 5 次采样 | 1 次 1.1s；3 次 9–15s；1 次超时 | 5/5 全部 ~0.58s |
| 8 次采样（8s 上限，与插件超时一致） | 4 次成功（1.0–4.6s）、2 次 429、2 次超时 | — |
| 443 TLS 握手 | 0.8–14s，随机卡住 | 稳定 0.39s |
| 80 端口 | 301，0.5s（正常） | — |

结论：

1. **能通，但不稳定**。解析正常、80 端口秒回 301，卡住的是 443 的 TLS 握手——是典型的链路/SNI 干扰特征，而不是"Cloudflare 整体进不来"（er-api 同为 Cloudflare，5/5 稳定）。
2. 按插件现有 **8 秒**请求超时，实测成功率约 50%，且成功的几次常贴着上限。
3. 短时间连续请求会触发 **429**（本轮探测里出现 2 次）。插件是 1 小时缓存 + 失败 1 分钟退避，正常单实例 1 次/小时不会触发，但多实例或频繁重启可能撞上。
4. 走本机代理时也不快（4.2s，同样落 LAX 边缘）。

因此：**回退链不是可选优化，而是接入该源的前提**（见 4.4）。单机单运营商采样不能代表所有国内网络，但方向足够明确。


## 3. 现状与需要动的地方

`internal/plugin/exchange_rate.go`：

- `exchangeRateURL` 是包级常量；`newExchangeRateFetcher()` 用 `url.Parse(exchangeRateURL)` 的 host
  做**重定向白名单**。→ 换源必须把 URL 作为参数传入，否则重定向校验会对着 er-api 的 host 判新源。
- `ExchangeRateResponse.Source` 在 `latest()` 里**硬编码** `"open.er-api.com"`。→ 必须由 provider 提供。
- 缓存：1h 新鲜、24h 陈旧降级、失败后 1min 退避；`applyConfig` 已把 `r.exchangeRates = nil`，
  改配置天然丢缓存，无需额外失效逻辑。
- 前端 `renderVisuals()` 用 `t('currency.rate', {rate})` + `effective_at` 显示"汇率 + 日期"。
  换成 ECB 后，周末会显示较旧的日期（`effective_at` 来自 `time_last_update_unix`），能自证数据新鲜度，
  但**当前不显示来源**，不满足署名要求。

## 4. 方案

### 4.1 配置（`config.go`）

新增 `exchange_rate_provider`，取值：

| 值 | 含义 |
| --- | --- |
| `auto`（默认） | 自动选源：国内网络优先 CFETS 中间价，海外网络优先 open.er-api，失败逐级回退 |
| `cfets` | 固定主源为 CFETS 中间价 |
| `open-er-api` | 固定主源为 open.er-api.com（等价于现状） |
| `allratestoday` | 固定主源为 AllRatesToday |

未知值在 `normalizeConfig` 直接报错，空值视为 `auto`。

### 4.2 三个 Provider（`exchange_rate.go`）

| id | URL | `Source` | 口径 | 解析 |
| --- | --- | --- | --- | --- |
| `cfets` | `https://www.chinamoney.com.cn/r/cms/www/chinamoney/data/fx/ccpr.json` | `CFETS` | 人民币中间价（工作日 9:15） | `records[]` 中 `vrtEName=="USD/CNY"` 的 `price`；`effective_at` 取 `data.lastDate`（Asia/Shanghai） |
| `open-er-api` | `https://open.er-api.com/v6/latest/USD` | `open.er-api.com` | 市场参考 | 现有 `exchangeRateProviderResponse` |
| `allratestoday` | `https://allratestoday.com/api/open/latest/USD` | `AllRatesToday` | ECB 每日参考 | 与 er-api 同形状，复用解析 |

- `newExchangeRateFetcher(providerURL string)`：重定向白名单按传入 URL 的 host 判定，不再读常量。
- CFETS 单独解析并校验：`price` 必须是 0–100 之间的有限数，找不到 `USD/CNY` 记录即视为失败。

### 4.3 前端署名与降级提示

- 汇率状态文案由 `t('currency.rate')` 扩展为 `汇率 {rate}（{source}，{date}）`；回退时追加 `已回退`。
- 新增文案键 `currency.source`、`currency.fallback`（4 语言）。
- `ExchangeRateResponse` 新增 `fallback`（bool），前端只读不改接口语义。


### 4.4 自动选源与三级回退（已定案）

**选源顺序（`auto`）**：`cfets` → `open-er-api` → `allratestoday`。

- 国内服务器访问 CFETS 约 0.1s，海外访问通常不可达；因此"能否在短暂超时内取到 CFETS"本身就是最可靠的地理判定，不需要 GeoIP、也不依赖任何第三方定位服务（那些服务在国内同样可能不可达）。
- 首次冷启动顺序尝试：CFETS 成功 → 判定为国内，首选记 CFETS；CFETS 失败/超时 → 判定为海外，首选记 open.er-api，后续刷新直接从它开始，不再每次都白等一次超时。
- 回退后记忆"上次成功的源"，并在 **30 分钟后**重试链首（`exchangeRateHeadRetry`）；仍在窗口内则直接从记忆的源开始。这样网络恢复后不会长期停在低优先源，也不会每轮都白等一次 4 秒超时。
- 单次尝试超时 4s（`exchangeRateAttemptTimeout`）、整轮预算 10s（`exchangeRateBudgetTimeout`）；全部失败才使用 24h 内的陈旧缓存，否则报错。

**显式配置**：从指定的源开始，然后按 `auto` 顺序补齐其余源（去重），保证永远有兜底。

**缓存策略**：

| 情形 | 新鲜期 | 说明 |
| --- | --- | --- |
| 首选源成功 | 1h（沿用现状） | 正常路径 |
| 回退源成功 | 10min | 尽快回到首选源，同时不让上游被打爆 |
| 全部失败 | 24h 陈旧降级 + 1min 退避 | 沿用现状 |

**响应与 UI**：`fallback: true` 表示本次用的是非首选源；`source` 始终是实际来源。前端显示 `汇率 6.7088（CFETS，2026-11-25）`，回退时加"已回退"。

**口径风险**：回退意味着口径可能切换（中间价 6.7088 ↔ 市场 6.6782，差约 0.5%）。UI 必须显示来源，文档必须写明"不同来源口径不同、人民币金额会随之变化"。


## 5. 测试计划

| 类型 | 用例 |
| --- | --- |
| 单元 | 默认 provider 为 `open-er-api`，URL 与 `Source` 正确 |
| 单元 | `allratestoday` provider 使用自己的 URL，`Source` = `AllRatesToday`，解析 ECB 响应成功 |
| 单元 | 未知 provider → `parseConfig` 返回错误，错误信息包含合法取值 |
| 单元 | 重定向到其他 host 被拒（对两个 provider 各测一次，锁住白名单按 provider 生效） |
| 单元 | 切配置后缓存重建（`applyConfig` 置 nil 后 `latest()` 与旧缓存不串源） |
| 单元 | （二期）主源失败回退 er-api，`fallback=true`，`Source` 为 er-api |
| 契约 | 前端文案含 `currency.source`，`effective_at` 仍用于日期显示 |
| 浏览器 | 汇率按钮显示来源与日期；切 CNY 后数字与 `rate` 相乘一致（沿用现有汇率用例） |

## 6. 文档

- README 配置表新增 `exchange_rate_provider` 一行（中英文），说明 ECB 参考汇率与 er-api 市场汇率
  存在约 0.2% 偏差、按工作日更新、周末可能沿用前一交易日。
- 说明署名要求：数据由 AllRatesToday / ECB 提供。

## 7. 非目标

- 不引入第三个源，不做运行时热切换（改配置后下次请求重建即可）。
- 不改变计价逻辑、不改缓存 TTL、不改 `/exchange-rate` 响应结构（除二期的可选 `fallback` 字段）。
- 不代替上游做汇率插值或补周末数据。

## 8. 风险

- **供应商自荐**：接入等于为一家小型服务背书；其配额、条款与存活率成为运维成本。默认值不变可降低影响面。
- **参考汇率偏差**：ECB 定盘价与市场价差约 0.2%，用户可能拿它对照银行/交易所价格。需在文档与 UI 说明。
- **署名合规**：必须展示来源，否则不满足其条款。
- **缓存串源**：切配置必须重建服务（现有 `applyConfig` 已置 nil，需测试锁住）。

## 9. 验收

1. 不配置时行为与现在完全一致（URL、来源文案、缓存策略）。
2. 配 `allratestoday` 后 `/exchange-rate` 返回该源汇率，`source` 为 AllRatesToday，面板显示来源与日期。
3. 非法值在注册/重配置阶段报错，插件不会带着错误配置运行。
4. `gofmt` / `go vet` / `go build` / `go test ./...`（含浏览器用例）全绿。

## 10. 实施记录

已按 §4 落地（`auto` 自动选源 + 三级回退）。

| 文件 | 变更 |
| --- | --- |
| `internal/plugin/exchange_rate_providers.go` | 新增：provider 定义与链顺序、`newExchangeRateHTTPClient`、CFETS fetcher 与解析、`cfetsEffectiveUnix` |
| `internal/plugin/exchange_rate.go` | 抽出 `fetchExchangeRateJSON` 共用 HTTP 管线；服务改为按链尝试；`Fallback` 字段；回退 TTL 与"记忆成功源 + 30 分钟重试链首" |
| `internal/plugin/config.go` | 新增 `exchange_rate_provider`（`auto` 默认），未知值报错，空值回落 `auto` |
| `internal/plugin/management.go` | 服务构建传入配置值 |
| `internal/plugin/dashboard.go` + locales | 汇率旁显示来源与"已回退"，新增 `currency.source` / `currency.fallback`（4 语言） |
| `internal/plugin/exchange_rate_providers_test.go` | 链顺序、CFETS 解析与非法响应、自动回退、回退后恢复链首、AllRatesToday 兜底、配置校验 |
| `README.md` | 配置表与示例补 `exchange_rate_provider`（中英文） |

实测：真实 CFETS 接口解析得 6.7088，`effective_at` = 2026-11-25T01:15Z（09:15 CST）。
