# Issue #89：Cloudflare Rocket Loader 导致面板脚本不执行修复计划

- Issue：https://github.com/AITNR/cap-token-usage-tracker/issues/89
- 编写日期：2026-11-25（Asia/Shanghai）
- 核查基线：main / 3999ae7e5a2d076b46f84322bc84af294311610c（v2.0.11）
- 状态：已实施，待提交推送

## 1. 核查结论与证据

Issue 现象：CPA 经 Cloudflare Tunnels 暴露到公网后，无论是宿主面板内嵌访问还是直接访问
`/v0/resource/plugins/cap-token-usage-tracker/dashboard`，页面都停在 `Loading statistics…`，
四张指标卡全部显示 `—`，趋势图、成本图、表格均为空。Issue 给出的解决方案是在 Cloudflare
关闭 Rocket Loader。

Issue 截图里的控制台只有三条 CSP 拦截记录，且都是 Cloudflare 自己注入的脚本：

| 被拦截脚本 | 来源功能 | 影响 |
| --- | --- | --- |
| `/cdn-cgi/rum` | Web Analytics 统计探针 | 无（不影响面板） |
| `/cdn-cgi/challenge-platform/scripts/jsd/main.js` | 安全挑战 / Bot 检测 | 无（不影响面板） |
| `/cdn-cgi/.../rocketscript.js` | **Rocket Loader 引导器** | **致命：页面脚本全部不执行** |

关键点在于第三条。面板没有任何外链脚本，`dashboardHTMLTemplate` 里只有两个内联
`<script>`（`internal/plugin/dashboard.go:252` 首屏主题同步、`:314` 主应用脚本），而
`dashboardResponse()` 下发的 CSP 是
`default-src 'none'; script-src 'unsafe-inline'; …`——只允许内联脚本，不允许任何外链脚本。
按这条策略，两个内联脚本本来一定能执行；页面却完全没有初始化，说明内联脚本没有以原生
内联形式被执行。

结合 Rocket Loader 的工作方式可以完全解释这个现象：

1. Rocket Loader 开启后会在边缘把页面里的 `<script>` 统一改写成 `type="text/rocketscript"`，
   交由 `/cdn-cgi/rocketscript.js` 这个引导器在稍后重新插入执行。
2. 该引导器是外链脚本，被站点 CSP 的 `script-src 'unsafe-inline'` 拦截。
3. 于是被改写的内联脚本既不是原生内联脚本（已被改写），也没有引导器去执行它——
   `startDashboard()` 永远不会被调用，页面停在初始状态。

即：**面板自身没有 bug，但它对「脚本被反代改写」这件事毫无防御**，任何会重写 `<script>`
的边缘优化（首当其冲是 Rocket Loader）都会让面板静默失效，且现场看不出原因。

## 2. 目标与范围

目标：让面板在开启 Rocket Loader 的 Cloudflare 后面也能正常工作，不需要用户去改 Cloudflare
配置；同时保持现有 CSP 强度不变。

范围：仅修改内嵌 dashboard 模板（`internal/plugin/dashboard.go`）及回归测试。
不修改 bbolt 格式、统计/计费逻辑、API 路由、CSP 以外的响应头，不上线、不推送。

## 3. 实施步骤

### A. 让面板脚本退出 Rocket Loader 的接管

Cloudflare 官方支持的退出方式是给 `<script>` 加 `data-cfasync="false"`，Rocket Loader 的
HTML 改写会跳过带该属性的脚本。两个内联 `<script>` 都加上：

- 首屏主题同步脚本（`<head>` 内）：负责在样式表生效前定色，被延迟执行会闪白/闪黑。
- 主应用脚本（`</body>` 前）：负责全部渲染与轮询。

两个 dashboard 变体（普通面板 `dashboardHTML`、全模式 `fullDashboardHTML`）都由
`dashboardHTMLTemplate` 经 `strings.NewReplacer` 生成，改模板即同时覆盖，无需改装配逻辑。

### B. 不放松 CSP

不为 `/cdn-cgi/` 或 `'self'` 开口子：

- 加 `'self'` 会让 `rocketscript.js` 真的被执行，从而把内联脚本变成乱序异步执行，
  比现在更不稳定；
- `/cdn-cgi/rum`、challenge-platform 被拦截不影响面板功能（Issue 已确认）。

因此 CSP 保持 `script-src 'unsafe-inline'` 不变，拦截日志里剩下的两条属于预期噪音。

### C. 回归测试

1. 新增 `TestDashboardScriptsOptOutOfRocketLoader`：遍历两种 dashboard 生成的 HTML，
   断言每个 `<script` 开标签都带 `data-cfasync="false"`，防止以后新增脚本时漏加。
2. 更新 `TestDashboardPaintsDarkBeforeRunningThemeSync`：它按字面量 `<script>` 定位首屏脚本，
   随属性变更改为匹配带属性的开标签，语义（root → 样式 → 主题脚本 的顺序）不变。

## 4. 验收

- `gofmt -l`、`go vet ./...`、`go build ./...`、`go test -count=1 ./...` 全绿。
- 现有 `TestGeneratedDashboardJavaScriptSyntax` 仍能用 `node --check` 解析两个脚本块。
- 浏览器冒烟：用 Chromium（Edge）加载生成的 dashboard HTML + 桩 API，确认状态栏正常清空、
  指标卡有数值，即面板在脚本正常执行时行为无回归。

## 5. 非目标

- 不改变 Cloudflare 侧的推荐配置，文档继续建议无法加属性时关闭 Rocket Loader。
- 不为“脚本被完全禁止执行”的场景新增 UI 兜底提示（本期只解决改写，不扩展失败提示）。
