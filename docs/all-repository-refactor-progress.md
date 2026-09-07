# 全仓库审查与重构进度

- 日期：2026-09-07
- 基线分支：`alpha`
- 工作分支：`codex/refactor-all-review`
- 基线提交：`1059bd5`
- 状态标记：`[ ]` 未开始，`[~]` 进行中，`[x]` 已完成，`[!]` 受阻

## 目标

按前一份全仓库审查结论，把明确可验证的风险拆成独立小步实施。每完成一个可验证阶段，即更新本文档，记录改动、测试和剩余事项。

## 基线验证

- [x] `go test ./... -count=1` 通过
- [x] `go vet ./...` 通过
- [x] `go build ./...` 通过
- [x] 聚焦浏览器日期范围与 Token 单位测试通过
- [x] `gofmt -l .` 发现 24 个未格式化 Go 文件
- [!] 本机缺少 GCC，无法执行 `go test -race ./...`；后续由 Linux CI 承担

## 阶段 1：Full Mode 分段上传资源上限

- [x] 先建立进度文档
- [x] 为按 endpoint 动态 chunk 上限、session/runtime 上传数量、全局字节数、撤销清理补测试
- [x] 实现资源上限并保持既有认证、归属、索引和最终 payload 校验
- [x] 运行聚焦测试与全量 Go 测试
- [x] 更新阶段结论

### 实施记录（2026-09-07）

- 修改 `full_mode.go`：
  - 删除固定 `16000` chunk 上限；
  - 新增 `fullModeUploadChunkLimit`，按 endpoint 最大解码 payload 计算 raw-url-base64 chunk 上限；
  - 新增每个 session 最多 2 个活跃 upload、runtime 最多 8 个活跃 upload；
  - 新增 96 MiB 全局预留 staged 字节上限；
  - revoke session 时同步删除该 session 的全部 uploads。
- 修改 `full_mode_test.go`：
  - 覆盖 2 MiB 价格接口和 64 MiB restore 接口的边界 chunk 数；
  - 覆盖 session/runtime 活跃数量上限；
  - 覆盖全局预留字节数上限；
  - 覆盖过期清理和 revoke 清理；
  - 覆盖跨 session 访问拒绝。
- 验证：
  - `go test ./... -count=1 -run "TestFullModeUpload|TestFullModeStagedPriceSave|TestFullModeBackupAndRestore"` 通过；
  - `go test ./... -count=1` 通过；
  - `go vet ./...` 通过。

### 验收标准

- 2 MiB 价格保存/同步接口不允许声明远超 payload 上限的 chunk 数；
- 64 MiB restore 接口按备份上限计算 chunk 数；
- 每个 session 活跃上传数有上限；
- runtime 全局活跃上传数和 staged 字节数有上限；
- 达到资源上限返回 `429 Too Many Requests`；
- revoke session 时同步删除该 session 的 uploads；
- 现有非法 chunk、跨 session、最终 payload 超限行为保持可测。

## 阶段 2：Go 格式化基线

- [x] 执行 `gofmt -w` 并确认 diff 仅格式化
- [x] `gofmt -l .` 无输出
- [x] 运行 `go test ./... -count=1`、`go vet ./...`
- [x] 更新阶段结论

### 实施记录（2026-09-07）

- 对仓库全部 Go 源码和测试文件执行 `gofmt -w`。
- `gofmt -l` 复查无输出。
- Git 内容 diff 中仅阶段 1 的 `full_mode.go` 与 `full_mode_test.go` 存在实际变更；其余格式化文件只发生行尾归一化，不产生语义 diff。
- 验证：
  - `go test ./... -count=1` 通过；
  - `go vet ./...` 通过。

## 阶段 3：CI 质量门禁与二进制清理

- [x] 新增常规 push/PR 质量门禁
- [x] CI 覆盖 gofmt、test、vet、build、race 和必需浏览器测试
- [x] 停止跟踪本地 Windows 可执行文件
- [x] 更新 `.gitignore`
- [x] 更新阶段结论

### 实施记录（2026-09-07）

- 新增 `.github/workflows/quality.yml`：
  - push 和 pull request 均触发；
  - 检查全部 Git 跟踪 Go 文件的 `gofmt` 状态；
  - 执行 `go test -count=1 ./...`；
  - 执行 `go vet ./...`；
  - 执行 `go build ./...`；
  - 执行 `go test -race -count=1 ./...`；
  - 安装 `playwright-core` 后以 `REQUIRE_BROWSER_TESTS=1` 和系统 Chrome 执行浏览器回归。
- 使用 `git rm --cached cap-token-usage-tracker.exe` 停止跟踪本地可执行文件；本地文件仍保留。
- `.gitignore` 新增 `*.exe`。
- 本地验证：
  - `git check-ignore -v -- cap-token-usage-tracker.exe` 命中 `.gitignore:25`；
  - `cap-token-usage-tracker.exe` 不再被 Git 跟踪且本地文件仍存在；
  - `go test ./... -count=1` 通过；
  - `go vet ./...` 通过。
- 边界说明：新 CI workflow 尚未在 GitHub Actions 远程执行；race 和 Ubuntu Chrome 浏览器测试需推送后由 CI 验证。

## 阶段 4：偏好设置写入路径

- [x] 补授权与读写行为测试
- [x] 将状态写入迁移到管理端路由
- [x] GET resource 保留明确兼容策略
- [ ] 更新前端调用与测试
- [x] 更新阶段结论

### 实施记录（2026-09-07）

- 修改 `management.go`：
  - 新增管理端 `POST /v0/management/plugins/<id>/preferences` 路由；
  - 注册 `POST /plugins/<id>/preferences` 管理路由；
  - 将保存逻辑拆分为管理端 JSON 请求体的 `saveDashboardPreferencesResponse` 与旧 GET 兼容路径的 `saveDashboardPreferencesLegacyResponse`；
  - resource `GET /preferences` 保持只读语义，但保留 `save=1` 旧兼容路径；
  - resource 描述改为“读取偏好”，避免继续宣传 GET 写入。
- 修改 `management_test.go`：
  - 更新管理路由注册数量和顺序断言；
  - 新增管理端 POST JSON 请求体保存、无效请求体拒绝、GET 读取、错误方法和旧 GET 兼容路径测试。
- 修改 `README.md`：
  - 中英文接口表补充管理端 `POST /preferences`；
  - 明确 resource `GET /preferences` 的读取语义和 `save=1` 旧兼容写入语义。
- 验证：
  - `go test ./... -count=1 -run "TestManagementRegistrationUsesDynamicPluginID|TestDashboardPreferences"` 通过；
  - `go test ./... -count=1` 通过；
  - `go vet ./...` 通过；
  - `go test -race ./... -count=1` 通过；
  - `REQUIRE_BROWSER_TESTS=1 CHROME_PATH="C:\Program Files\Google\Chrome\Application\chrome.exe" go test ./... -count=1` 通过。
- 未完成事项：
  - 前端仍使用旧 resource `save=1` 兼容路径，避免在宿主管理授权交互未设计前破坏自动保存体验；
  - 后续需要确定是否让用户输入/缓存管理密钥，或提供 capability 保护的偏好保存路径，再切换前端调用。

## 阶段 5：汇率刷新单飞

- [x] 补并发刷新测试
- [x] 将 HTTP fetch 移出互斥锁
- [x] 合并缓存过期时的并发刷新
- [x] 更新阶段结论

### 实施记录（2026-09-07）

- 修改 `exchange_rate.go`：
  - `latest()` 不再在持有互斥锁期间执行 HTTP fetch；
  - 新增 `exchangeRateSource` 接口，便于测试阻塞和失败的上游；
  - 新增 `exchangeRateRefresh` 单次刷新广播结构；
  - 缓存过期时第一个调用者执行刷新，其他调用者等待同一次刷新结果；
  - 成功、失败、stale fallback 和 retry backoff 状态更新保持原语义。
- 修改 `exchange_rate_test.go`：
  - 新增 20 个并发调用共享一次成功刷新的测试；
  - 新增 20 个并发调用共享一次失败刷新的测试；
  - 保持既有缓存、stale fallback、重试退避和 HTTP 校验测试。
- 验证：
  - `go test ./... -count=1 -run "TestExchangeRate" -timeout 30s` 通过；
  - `go test ./... -count=1` 通过；
  - `go vet ./...` 通过。
- 边界说明：本机仍缺少 GCC，无法执行 `go test -race`；并发等待路径将在 Linux CI 的 race 步骤验证。

## 阶段 6：未命名模型国际化

- [x] 补四语言展示测试
- [x] 后端不再输出中文自然语言作为模型展示名
- [x] 保持旧筛选兼容
- [x] 更新阶段结论

### 实施记录（2026-09-07）

- 修改 `aggregate.go`：
  - `compactModelName` 保持空模型名为空，不再生成中文自然语言；
  - 新增 `modelFilterMatches`，继续兼容旧的 `未标记模型` 筛选值；
  - `legacyUntitledModelName` 仅作为旧筛选和旧价格簿兼容常量保留。
- 修改 `cost.go`：
  - 费用模型、序列和 missing price 输出保持空模型名；
  - 空模型定价继续兼容旧价格簿中的 `未标记模型` 条目。
- 修改 `persistence.go`：
  - 逐请求模型筛选改用统一兼容匹配逻辑。
- 修改 `management.go`：
  - `exclude_model` 继续兼容旧 `未标记模型` 值，同时不把中文文案写入新响应。
- 修改测试：
  - `aggregate_test.go` 覆盖空模型聚合、初始统计、趋势 JSON 不泄漏中文占位符，并覆盖旧筛选兼容；
  - `cost_test.go` 覆盖旧价格簿兼容；
  - `dashboard_test.go` 覆盖四种语言的 `model.untitled` 与前端 `modelName` 本地化调用。
- 验证：
  - `go test ./... -count=1 -run "TestUntitledModel|TestLegacyUntitledModel|TestDashboardLocalizesUntitledModel"` 通过；
  - `go test ./... -count=1` 通过；
  - `go vet ./...` 通过；
  - 非测试源码中仅保留 `legacyUntitledModelName` 兼容常量，不再作为响应模型名输出。

## 阶段 7：dashboard 模板治理

- [x] 锁定模板 marker 和普通/Full Mode 差异契约
- [ ] 逐步拆分嵌入式前端资源
- [ ] 保持 CSP 与 capability 边界不变
- [x] 更新阶段结论

### 实施记录（2026-09-07）

- 修改 `dashboard_test.go`：
  - 新增模板契约测试，锁定全部 17 个 `LOCALE` / `FULL_MODE` marker；
  - 要求每个 marker 在原始模板中恰好出现一次；
  - 要求普通模式和 Full Mode 生成产物均不保留未替换 marker。
- 验证：
  - `go test ./... -count=1 -run "TestDashboardTemplateMarkersAreUniqueAndReplaced"` 通过。
- 后续事项：
  - 现有测试已覆盖普通模式不得残留定价、导出、备份、恢复和重置控件；
  - 还未把 HTML/CSS/JS 拆成独立 `go:embed` 静态资源，此重构应单独进行以避免一次性高风险改动。

## 记录规则

1. 每完成一个阶段或一个可验证子步骤，先更新本文档再进入下一步。
2. 每个阶段记录实际修改文件和验证命令。
3. 不混合行为修改、格式化提交和 CI 变更。
4. 遇到需要外部环境（例如 race、跨平台构建）的验证时，明确记录阻塞原因。

## 当前总体验证（2026-09-07）

- [x] `gofmt` 全量复查无输出
- [x] `go test ./... -count=1` 通过
- [x] `go vet ./...` 通过
- [x] `REQUIRE_BROWSER_TESTS=1 CHROME_PATH="C:\Program Files\Google\Chrome\Application\chrome.exe" go test ./... -count=1` 通过
- [x] `go build -o <临时路径> .` 通过；临时构建产物已删除
- [x] `git diff --check` 通过
- [x] `go test -race ./... -count=1` 通过
- [!] `quality.yml` 尚未推送，因此 GitHub Actions 远程结果未验证

## 当前剩余事项

1. 偏好设置：
   - 管理端 POST 路由已就绪，并改为接收 JSON 请求体；
   - 前端仍保留 resource `save=1` 兼容路径，切换前需确定管理密钥交互或 capability 写入路径。
2. dashboard 模板：
   - marker 契约已锁定；
   - 尚未拆分 HTML/CSS/JS 到独立 `go:embed` 静态资源。
3. CI：
   - 需要推送分支并确认 GitHub Actions 的 test、vet、build、race 和浏览器测试结果。
4. Git：
   - `cap-token-usage-tracker.exe` 已从索引移除且本地文件保留；
   - 当前变更尚未提交。

## 本地 DLL 测试构建（2026-09-07）

- [x] 在 `codex/refactor-all-review` 当前源码上编译 Windows amd64 `c-shared` DLL
- [x] 验证构建产物存在并读取 Go 构建元数据
- [x] 记录 SHA-256，便于用户确认实际测试的文件

### 构建信息

- 产物：`D:\c\cap-token-usage-tracker\dist\cap-token-usage-tracker.dll`
- 大小：`8,387,584` bytes
- 修改时间：`2026-09-07 18:31:22`
- SHA-256：`3F1CC2C2031297DD33594549233F519482A94DB344BB8126CAE7BBDBE11DC7BB`
- Go 版本：`go1.26.5`
- 构建参数：
  - `-buildmode=c-shared`
  - `-trimpath`
  - `-buildvcs=false`
  - `-ldflags "-s -w -X main.version=2.0.5-refactor-test"`
- 编译器：`C:\mingw64\mingw64\bin\gcc.exe`
- 目标平台：Windows amd64
- `go version -m` 已确认：
  - `GOOS=windows`
  - `GOARCH=amd64`
  - `CGO_ENABLED=1`
  - `-buildmode=c-shared`
  - 依赖 `CLIProxyAPI v7.2.129`

同目录生成的 `cap-token-usage-tracker.h` 也已随本次 DLL 构建更新。

## 目录整理（2026-09-07）

- [x] 将插件实现、测试和内嵌 locale 移动到 `internal/plugin/`
- [x] 根目录保留 Go 入口、C 桥接、模块元数据、项目级文件与图片资源
- [x] 按用户要求将图片资源保留在根目录，仪表盘语法参考脚本移动到 `assets/`
- [x] 将本地构建脚本移动到 `scripts/`
- [x] `scripts/build.sh` 与 `scripts/build_dll.ps1` 改为使用仓库相对路径，并在构建失败时返回非零状态
- [x] 将本地 DLL、头文件和旧 EXE 移动到 `dist/`
- [x] 更新浏览器测试的 `node_modules` 与 `test/dashboard_date_range.mjs` 相对路径
- [x] 更新 `main_cgo.go`，通过 `internal/plugin` 的公开 facade 调用运行时
- [x] 重新编译 DLL 并完成全量验证

### 新的根目录布局

- `main.go`、`main_cgo.go`、`bridge.c`：c-shared 入口与 C ABI 桥接；
- `internal/plugin/`：插件实现、测试与内嵌 locale；
- `assets/`：仪表盘语法参考脚本；图片 `log-64x64.png` 与 `log-1024x1024.png` 保留在根目录；
- `scripts/`：本地构建与平台验证脚本；
- `dist/`：本地构建产物，包括 DLL、头文件和旧 EXE；
- `docs/`、`.github/`、`test/`、`package.json` 等保持原有职责。

### 实施记录

- `internal/plugin` 包名从 `main` 改为 `plugin`；
- `internal/plugin/rpc.go` 增加仅供根目录 cgo 入口使用的公开 facade：
  - `RuntimeState`
  - `DispatchRPC`
  - `MarshalError`
  - `AuthRuntimeMetadata`
  - `RPCEnvelope`
  - `SetAuthRuntimeLookup`
  - `Shutdown`
- `main_cgo.go` 保留 C export，并调用 `internal/plugin` facade；
- `scripts/build_dll.ps1` 和 `scripts/build.sh` 改为输出到 `dist/`；
- README 中本地 DLL 构建输出路径和构建脚本路径已更新。

### 验证

- `gofmt` 全量复查无输出；
- `go test ./... -count=1` 通过；
- `go vet ./...` 通过；
- `REQUIRE_BROWSER_TESTS=1 CHROME_PATH="C:\Program Files\Google\Chrome\Application\chrome.exe" go test ./... -count=1` 通过；
- `go test -race ./... -count=1` 通过；
- Windows amd64 `c-shared` DLL 构建通过；
- `bash -n scripts/build.sh` 通过；
- `scripts/build_dll.ps1` PowerShell 语法解析通过。

### 整理后的 DLL

- 路径：`D:\c\cap-token-usage-tracker\dist\cap-token-usage-tracker.dll`
- 大小：`8,412,672` bytes
- 修改时间：`2026-09-07 18:39:34`
- SHA-256：`3BDFD791ABD108A0A12E0BB18F08E2651C9D9D1F1EFB6D36F392089D10DD45F7`
- Go 版本：`go1.26.5`
- 版本标识：`2.0.5-refactor-test`
