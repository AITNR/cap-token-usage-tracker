# Issue #83：趋势图总 Tokens 和缓存读取重复计数修复计划

- Issue：https://github.com/AITNR/cap-token-usage-tracker/issues/83
- 编写日期：2026-10-04（Asia/Shanghai）
- 核查基线：alpha / 5d3cc471ffe27df244f8c5bc4d83be3ca6ca29d5（v2.0.9）
- 执行模型：gpt-5.6-luna
- 状态：已实施；主代理完成代码审查及回归验收，待提交推送。

## 1. 核查结论与证据

该 bug 在当前源码中仍存在。GitHub alpha HEAD 与本地核查基线相同；GitHub main（59af056315feb31e77b9647e78c3e676c6dab6b4）也已通过读取当前源文件确认同样的三项重复计数/堆叠逻辑。工作区在核查开始时干净。

`internal/plugin/dashboard.go` 中：

1. `pointStackTotal(point)` 累加可见的 input、output 和 cacheRead。
2. `showBarTooltip()` 使用该函数显示“总 Tokens”。
3. `renderBar()` 使用该函数计算 SVG Y 轴最大值，缓存 rect 位于 `base-inputH-outputH-cacheReadH`。
4. `exportPNG()` 使用该函数计算导出图比例，缓存 rect 位于 `chartY+chartH-inputH-outputH-cacheReadH`。
5. `aggregateTrend()` 已累加后端 `total_tokens` 到 `bucket.total`，但 tooltip 没有使用它。

直接从现有源码提取并执行 pointStackTotal，输入 Issue 的示例：

| 字段 | 数值 |
| --- | ---: |
| input | 48,342,180 |
| output | 168,768 |
| cacheRead | 46,221,824 |
| 后端 total | 48,511,163 |
| input + output | 48,510,948 |
| 当前 tooltip / Y 轴计算值 | 94,732,772 |
| 相对于 input + output 重复计入的缓存量 | 46,221,824 |

Issue 的主要根因成立，但“后端 total 永远等于 input+output”并不适用于所有记录。`usage.go:decodeUsage()` 保留上游提供的 TotalTokens；没有有效总量时回退 input+output+reasoning，必要时回退 cached。此次修复须保留后端总量语义，不更改后台统计。

## 2. 目标与范围

修复页面 tooltip、SVG 柱高与 Y 轴、PNG 导出，让缓存读取作为输入的构成部分展示。保持模型筛选、时间聚合、缩放、系列图例开关和缓存命中率功能。

本次实现仅修改内嵌 dashboard 和相关回归测试。不得修改 bbolt 格式、用量 API、历史数据、计费逻辑、reasoning/TPS 计算或其他 provider 的缓存统计语义。不上线、不推送、不创建 PR。

## 3. 实施步骤

### A. 分离业务总量与绘图高度

1. tooltip 的“总 Tokens”显示当前时间桶 `point.total`，即 aggregateTrend 已汇总的后端 total_tokens。兼容确实缺失总量的测试/旧数据时，可用 input+output 作明确的安全回退；有效的 0 总量不得被 truthy 检查误判。
2. 图例仅控制明细行及图形；切换 input/output/cacheRead 不改变时间桶业务总量，模型筛选则按现有 aggregateTrend 重新汇总。
3. pointStackTotal 改为可见几何总高度（保留函数名以减少影响）：
   - input 可见：输入贡献 = input；缓存不再另外累加。
   - input 隐藏、cacheRead 可见：输入贡献 = min(cacheRead, input)，缓存可以独立显示。
   - input 与 cacheRead 均隐藏：输入贡献 = 0。
   - output 可见时叠加 output。
4. 数值以非负、有限值处理。缓存绘图量夹在 [0,input]，防止历史异常 cacheRead>input 让覆盖区域溢出或倒置；tooltip 可保留原始缓存明细，不改数据。可抽取小的共享布局 helper，供 SVG 与 canvas 使用，避免两条路径分叉。

### B. 修正 SVG 几何

1. 显示 input 时，input 背景仍覆盖完整输入高度，output 位于 input 之上。
2. cacheRead 覆盖输入区域的上层：y=base-inputH，height=clampedCache/max*plotH。覆盖必须在 [base-inputH,base] 内，不占用 output 区域。
3. input 隐藏且 cacheRead 显示时，将缓存部分作为唯一的可见输入贡献，从 baseline 向上画；output 接在该贡献之上。避免缓存单独显示时 max=1 或柱子越界。
4. 全部系列隐藏、无数据和 input=0 时保持现有空态且不产生 NaN、Infinity 或负矩形尺寸。

### C. 同步 PNG 导出

exportPNG 使用与 SVG 相同的可见高度和覆盖规则。保留既有导出布局、费用/环图展示、模型筛选与导出权限。

### D. 添加行为回归覆盖

使用仓库现有 Go + playwright-core + 已安装 Chrome 的浏览器测试机制，可新增专门的 Node 场景与 Go 驱动。不能只增加源码字符串断言；核心断言应实际执行页面代码和验证矩形几何、tooltip/轴标签，PNG 可通过捕获真实 canvas fillRect 或像素验证。

覆盖至少：

- input=100、output=20、cacheRead=80、total=125：tooltip 总量为125，完整柱高/轴比例按120绘制，缓存位于输入内部。
- Issue 数值：旧结果94,732,772变为几何48,510,948；tooltip为后端48,511,163。
- 隐藏缓存：总量、完整柱高及Y轴不变化，仅缓存覆盖消失。
- 隐藏输入但显示缓存和输出：几何高度为100（80+20），tooltip总量仍125；只缓存为80，只输出为20。
- 无缓存、缓存等于输入、缓存超过输入、输入为0但缓存非0、全部隐藏和空桶。
- 历史 cached_tokens 回退仍工作。
- 多模型/多桶聚合和模型隐藏或下钻：缓存叠绘和业务总量按当前过滤正确变化。
- SVG 与 PNG 使用同样的规则；正常 dashboard 与 Full Mode 的生成HTML均包含修复。

## 4. 验证与验收

PowerShell 中设置：

```powershell
$env:CHROME_PATH = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
$env:REQUIRE_BROWSER_TESTS = '1'
```

执行针对本问题的行为回归，确认旧实现会失败、新实现通过。再执行：

```powershell
go test ./... -count=1
go vet ./...
git diff --check
```

Go 文件执行 gofmt。现有 node_modules/playwright-core 与 Chrome 已检查存在，package.json 没有 npm test 脚本。若环境导致检查不可执行，报告具体限制，不能把跳过当成通过。

验收要求：缓存不重复抬高柱高或Y轴；tooltip使用后端总量；图例开关没有越界；PNG与SVG一致；全部既有检查通过且无无关修改。执行完成由主代理审查实际 diff 和验证证据，并更新本计划状态。
## 5. 实施与验收记录（2026-10-05）

- gpt-5.6-luna 完成核心实现和 SVG/tooltip 浏览器回归。
- 主代理审查后将运行时字符串补丁改为直接修改共用 HTML 模板，避免未来模板调整导致替换失效。
- 缓存覆盖输入区域上部；SVG 和 PNG 共用 trendGeometry，图例控制可见高度，tooltip 读取业务总量。
- 增加 Full Mode 浏览器回归并捕获真实 PNG canvas fillRect 参数，验证正常显示、隐藏缓存及隐藏输入后的几何一致性；没有进行 PNG 文件像素解码比较。
- 验证 Issue 数值、缓存超过输入、输入为零、仅缓存、全部隐藏、有效零总量、缺失总量回退及历史 cached_tokens 回退。
- 原有模型筛选与聚合路径保留；本次未新增多模型下钻专项浏览器场景。
- 验证命令：启用 REQUIRE_BROWSER_TESTS=1 和 CHROME_PATH 后执行 go test ./... -count=1；另执行 go vet ./...、node --check test/dashboard_date_range.mjs、git diff --check。
- 用户已授权审查无问题后提交并推送当前 alpha 分支。