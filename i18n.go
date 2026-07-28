package main

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Language string

const (
	LanguageAuto    Language = "auto"
	LanguageEnglish Language = "en"
	LanguageChinese Language = "zh-CN"

	unlabeledModel       = "unlabeled"
	legacyUnlabeledModel = "未标记模型"

	legacyResultSuccess = "成功"
	legacyResultFailed  = "失败"
)

type languageDefinition struct {
	Code     Language
	Locale   string
	LabelKey string
}

var languageDefinitions = []languageDefinition{
	{Code: LanguageEnglish, Locale: "en", LabelKey: "lang.en"},
	{Code: LanguageChinese, Locale: "zh-CN", LabelKey: "lang.zh-CN"},
}

var supportedLanguages = func() []Language {
	result := make([]Language, 0, len(languageDefinitions))
	for _, definition := range languageDefinitions {
		result = append(result, definition.Code)
	}
	return result
}()

var messages = map[Language]map[string]string{
	LanguageEnglish: {
		"menu.token_usage":           "Token Usage",
		"menu.token_usage_desc":      "View persisted token usage, request, and latency statistics.",
		"dashboard.title":            "Token Usage Analytics",
		"dashboard.eyebrow":          "Usage analytics",
		"dashboard.subtitle":         "Model share, Input/Output token trends, and exact costs from the backend price book.",
		"dashboard.controls_aria":    "Dashboard controls",
		"control.range":              "Time range",
		"control.granularity":        "Aggregation",
		"control.language":           "Language",
		"range.24h":                  "Last 24 hours",
		"range.7d":                   "Last 7 days",
		"range.30d":                  "Last 30 days",
		"range.retention":            "All retained data",
		"granularity.minute":         "By minute",
		"granularity.hour":           "By hour",
		"granularity.day":            "By day",
		"granularity.week":           "By week",
		"granularity.month":          "By month",
		"button.pricing":             "Model prices",
		"button.export":              "Export",
		"button.export_csv":          "Export CSV",
		"button.export_png":          "Export chart image",
		"button.refresh":             "Refresh",
		"button.reset":               "Reset",
		"button.language":            "Language",
		"status.loading":             "Loading statistics…",
		"card.total_tokens":          "Total tokens",
		"card.estimated_cost":        "Estimated total cost",
		"card.total_requests":        "Total requests",
		"card.top_model":             "Top model",
		"card.token_detail":          "Input + Output + other tokens",
		"card.cost_waiting":          "Waiting for exact cost data",
		"card.request_detail":        "Current time filter",
		"card.top_model_detail":      "Ranked by request count",
		"unit.full":                  "Full",
		"unit.switch_to":             "Switch to {unit}",
		"unit.aria":                  "Token display unit: {unit}",
		"currency.usd":               "USD",
		"currency.cny":               "CNY",
		"currency.aria":              "Cost currency: {currency}",
		"currency.switch_usd":        "Switch to USD",
		"currency.switch_cny":        "Switch to CNY",
		"currency.usd_label":         "US dollars",
		"currency.cny_label":         "Chinese yuan",
		"panel.token_trend":          "Token consumption trend",
		"panel.token_trend_sub":      "Input / Output token stack",
		"panel.model_share":          "Model call share",
		"panel.model_share_sub":      "Click a segment or model name to drill down",
		"panel.cost_trend":           "Cost trend",
		"panel.cost_trend_sub":       "Exact per-request calculation from the price book",
		"panel.efficiency":           "Model efficiency",
		"panel.efficiency_sub":       "Average tokens / request × average latency",
		"panel.requests":             "Request details",
		"panel.requests_sub":         "Per-request records, newest first",
		"panel.dimensions":           "Dimension details",
		"panel.dimensions_sub":       "Current filtered view",
		"button.view_all_models":     "View all models",
		"button.reset_zoom":          "Reset zoom",
		"button.prev_page":           "Previous",
		"button.next_page":           "Next",
		"zoom.tip":                   "Scroll to zoom · Shift + scroll to pan",
		"zoom.out":                   "Zoom out",
		"zoom.in":                    "Zoom in",
		"series.input":               "Input",
		"series.output":              "Output",
		"chart.token_bars":           "Stacked token consumption chart",
		"chart.model_donut":          "Model call share doughnut",
		"chart.cost_line":            "Estimated cost trend line chart",
		"chart.efficiency":           "Model efficiency scatter plot",
		"chart.legend":               "Model legend",
		"metrics.aria":               "Key metrics",
		"analysis.aria":              "Advanced analysis",
		"table.time":                 "Time",
		"table.model_name":           "Model",
		"table.source":               "Source",
		"table.tier":                 "Tier",
		"table.result":               "Result",
		"table.ttft":                 "TTFT",
		"table.generation":           "Generation",
		"table.tps":                  "TPS",
		"table.reasoning":            "Reasoning effort",
		"table.input":                "Input",
		"table.output":               "Output",
		"table.thinking":             "Reasoning",
		"table.cache_read":           "Cache read",
		"table.cache_creation":       "Cache creation",
		"table.total_tokens":         "Total tokens",
		"table.cache_hit":            "Cache hit",
		"table.estimated_cost":       "Estimated cost",
		"table.price_source":         "Price source",
		"table.model":                "Model",
		"table.provider":             "Provider",
		"table.alias":                "Alias",
		"table.executor":             "Executor",
		"table.auth_type":            "Auth type",
		"table.service_tier":         "Service tier",
		"table.reasoning_effort":     "Reasoning effort",
		"table.requests":             "Requests",
		"table.failures":             "Failures",
		"table.avg_latency":          "Avg latency",
		"table.avg_ttft":             "Avg TTFT",
		"pricing.title":              "Model prices & sync",
		"pricing.intro":              "The model catalog is loaded directly from CLIProxyAPI /v1/models. The CLIProxyAPI API key and Management key are used only for this session. Unit prices are always USD / 1M tokens.",
		"pricing.revision":           "Price book revision: —",
		"pricing.coverage_wait":      "Price coverage: waiting for cost data",
		"pricing.missing":            "Missing prices: —",
		"pricing.models_unloaded":    "CLIProxyAPI models: not loaded yet",
		"pricing.pending_delete":     "Pending deletions: 0",
		"pricing.never_synced":       "models.dev has not been synchronized yet",
		"pricing.cli_key":            "CLIProxyAPI API key (not saved)",
		"pricing.load_models":        "Load current models",
		"pricing.model_source":       "After a successful load, the pricing editor and models.dev sync both use that model list.",
		"pricing.provider_priority":  "Provider priority (comma or newline separated)",
		"pricing.ignored_suffixes":   "Ignored model suffixes (comma or newline separated)",
		"pricing.mappings":           "Model mappings (one source=target per line)",
		"pricing.management_key":     "Management key (not saved)",
		"placeholder.api_key":        "API key",
		"placeholder.management_key": "Management key",
		"pricing.cancel":             "Cancel",
		"pricing.sync":               "Sync models.dev",
		"pricing.save":               "Save prices & settings",
		"reset.title":                "Management key confirmation",
		"reset.intro":                "Clearing statistics requires a CLIProxyAPI management key. The key is used only for this request.",
		"reset.cancel":               "Cancel",
		"reset.confirm":              "Confirm clear",
		"lang.auto":                  "Automatic (browser)",
		"lang.en":                    "English",
		"lang.zh-CN":                 "中文",
		"css.pending_delete":         "Deletes on save",

		"js.unlabeled_model":             "Unlabeled model",
		"js.all_models":                  "All models",
		"js.request_failed":              "Request failed (HTTP {status})",
		"js.invalid_exchange_rate":       "The exchange-rate endpoint returned invalid data.",
		"js.cny_unavailable":             "CNY exchange rate unavailable: {error}",
		"js.exact_cost_unavailable":      "Exact cost unavailable",
		"js.price_coverage":              "Price coverage {priced} / {total} · Unpriced {unpriced}",
		"js.cost_breakdown":              "Input {input} · Output {output} · Cache Read {cacheRead} · Cache Creation {cacheCreation} · Mode {mode} · Tier {tier}",
		"js.token_summary_detail":        "Input {input} · Output {output}",
		"js.tier_base":                   "Base",
		"js.status_range":                "Range: {range} · Last activity: {lastUsed}{costStatus} · Updated: {updated}",
		"js.cost_unavailable":            " · Cost unavailable",
		"js.cost_status":                 " · {coverage} · Price book #{revision}",
		"js.cost_data_error":             "Cost data unavailable: {error}",
		"js.cached_rate":                 " · cached rate",
		"js.rate_line":                   " · 1 USD = {rate} CNY{stale} · {time}",
		"js.unavailable":                 "Unavailable",
		"js.cost_api_error":              "Cost API error",
		"js.cost_data_unavailable":       "Cost data unavailable",
		"js.success_rate":                "Success rate {rate}%",
		"js.calls_tokens":                "{requests} calls · {tokens} tokens",
		"js.only_model":                  "Showing only {model}",
		"js.model_filter":                "Model filter: {model}",
		"js.requests_sub_filtered":       "Model filter: {model} · Newest first",
		"js.requests_sub_costs":          "Newest first · Price book #{revision}",
		"js.requests_sub_no_cost":        "Newest first · Cost unavailable",
		"js.no_usage":                    "No usage in this time range",
		"js.no_requests":                 "No requests in this time range",
		"js.groups_count":                "{count} groups",
		"js.result_success":              "Success",
		"js.result_failed":               "Failed",
		"js.cache_hit":                   "Hit",
		"js.cache_miss":                  "Miss",
		"js.unpriced":                    "Unpriced",
		"js.request_title_priced":        "Mode {mode} · Context {context} · Billable input {billable} · Cache read {cacheRead}",
		"js.request_title_unpriced":      "This request did not match a price",
		"js.requests_count":              "{total} requests · Price book #{revision}",
		"js.week_suffix":                 " wk",
		"js.exact_cost":                  "Exact cost",
		"js.unpriced_requests":           "Unpriced requests",
		"js.avg_tokens_request":          "Avg tokens / request",
		"js.avg_latency":                 "Avg latency",
		"js.request_count":               "Requests",
		"js.total_tokens":                "Total tokens",
		"js.no_data":                     "No data",
		"js.exact_cost_data_unavailable": "Exact cost data unavailable",
		"js.no_cost_data":                "No cost data in this time range",
		"js.models_count":                "{count} models",
		"js.no_efficiency":               "No model efficiency data in this time range",
		"js.empty_hint":                  "Try another time range or restore hidden models",
		"js.bar_aria":                    "{label} total tokens {total}",
		"js.donut_segment_aria":          "{model}, share {share}",
		"js.cost_point_aria":             "{label}, cost {cost}",
		"js.no_model_data":               "No model data yet",
		"js.show_model":                  "Show {model}",
		"js.hide_model":                  "Hide {model}",
		"js.legend_detail":               "{requests} calls · {tokens} tokens",
		"js.legend_tooltip":              "{model} · {detail}",
		"js.call_count":                  "Calls",
		"js.tokens_used":                 "Tokens used",
		"js.share":                       "Share",
		"js.remove":                      "Remove",
		"js.invalid_models_response":     "Invalid CLIProxyAPI models response.",
		"js.model_name_too_long":         "CLIProxyAPI returned an overly long model name.",
		"js.too_many_models":             "CLIProxyAPI returned too many models.",
		"js.no_available_models":         "CLIProxyAPI currently has no available models.",
		"js.enter_cli_key":               "Enter a CLIProxyAPI API key before loading models.",
		"js.missing_cli_key":             "Missing CLIProxyAPI API key",
		"js.models_loading":              "CLIProxyAPI models: loading…",
		"js.models_loaded":               "CLIProxyAPI models: {count} · {time}",
		"js.models_load_failed":          "CLIProxyAPI models: load failed",
		"js.models_not_loaded":           "CLIProxyAPI models: not loaded yet",
		"js.pending_delete_status":       "Pending deletions: {count}{extra}",
		"js.pending_delete_extra":        " · Costs recalculate after save",
		"js.price_revision":              "Price book revision: #{revision}",
		"js.price_coverage_unavailable":  "Price coverage: cost data unavailable",
		"js.missing_prices":              "Missing prices: {list}",
		"js.missing_prices_more":         " and {count} more",
		"js.missing_prices_none":         "Missing prices: none",
		"js.last_sync":                   "Last sync {completed} · Source models {observed} · Pending {pending} · Matched {matched} · Errors/unmatched {unmatched} · Created {created} · Updated {updated} · Kept manual {skipped}",
		"js.undo_delete":                 "Undo delete",
		"js.delete_price":                "Delete price",
		"js.price_head_model":            "Model",
		"js.price_head_source":           "Source / provenance",
		"js.price_input":                 "Input",
		"js.price_output":                "Output",
		"js.price_cache_read":            "Cache Read",
		"js.price_cache_creation":        "Cache Creation",
		"js.no_configurable_models":      "CLIProxyAPI has no configurable models right now",
		"js.load_models_prompt":          "Enter a CLIProxyAPI API key and load the current models",
		"js.unit_price":                  "{model} {kind} unit price",
		"js.tier_threshold_aria":         "{model} context tier threshold",
		"js.tier_price_aria":             "{model} context tier {kind} unit price",
		"js.source_unset":                "Source not set",
		"js.manual_price":                "Manual price",
		"js.not_in_catalog":              " · Not in the current CLIProxyAPI model list",
		"js.context_tiers":               "Context tiers: {count}",
		"js.expand_tiers":                "Expand context tiers: {count}",
		"js.collapse_tiers":              "Collapse context tiers: {count}",
		"js.threshold_tokens":            "Threshold tokens",
		"js.actions":                     "Actions",
		"js.accounting_mode":             "Accounting mode",
		"js.mode_default":                "Default",
		"js.mode_excludes_cache":         "Input excludes cache",
		"js.mode_includes_cache":         "Input includes cache",
		"js.add_context_tier":            "Add context tier",
		"js.mapping_invalid":             "Model mappings must use source=target, one per line.",
		"js.price_range_invalid":         "Model prices must be numbers between 0 and 1000000.",
		"js.tier_invalid":                "Context tier thresholds must be integers greater than 0, and prices must be in range.",
		"js.enter_management_key_save":   "Enter a Management key before saving.",
		"js.enter_management_key_sync":   "Enter a Management key before synchronizing.",
		"js.prices_saved":                "Model prices and sync settings saved · Price book #{revision} · {time}",
		"js.syncing_models":              "Syncing · Reading current CLIProxyAPI models…",
		"js.syncing_match":               "Syncing · Matching {count} CLIProxyAPI models…",
		"js.sync_done":                   "models.dev sync complete · Source models {count} · Price book #{revision} · {time}",
		"js.sync_failed":                 "Sync failed: {error}",
		"js.csv_preparing":               "Preparing per-request CSV…",
		"js.csv_yes":                     "Yes",
		"js.csv_no":                      "No",
		"js.csv_input_usd":               "Input USD",
		"js.csv_output_usd":              "Output USD",
		"js.csv_cache_read_usd":          "Cache Read USD",
		"js.csv_cache_creation_usd":      "Cache Creation USD",
		"js.csv_total_usd":               "Total USD",
		"js.csv_tier_threshold":          "Tier threshold",
		"js.csv_exported":                "Exported {count} per-request records · Exact costs from estimated_cost",
		"js.png_range":                   "Range: {range} · Exported: {time}",
		"js.png_all_visible":             "All visible models",
		"js.png_model":                   "Model: {model}",
		"js.reset_confirm":               "Permanently delete all token statistics? This cannot be undone.",
		"js.reset_prompt":                "Type reset to confirm:",
		"js.plugin_id_error":             "Unable to identify the plugin ID from the resource URL.",
		"js.cost_missing_price":          "This request is missing a price, so cost cannot be estimated",
		"js.syncing":                     "Syncing",
		"js.price_coverage_label":        "Price coverage",
	},
	LanguageChinese: {
		"menu.token_usage":           "Token 用量",
		"menu.token_usage_desc":      "查看持久化的 Token 用量、请求和延迟统计。",
		"dashboard.title":            "Token 用量统计",
		"dashboard.eyebrow":          "用量分析",
		"dashboard.subtitle":         "查看模型调用占比、输入/输出 Token 趋势与后端价格簿计算的精确费用。",
		"dashboard.controls_aria":    "仪表盘控件",
		"control.range":              "时间范围",
		"control.granularity":        "聚合粒度",
		"control.language":           "语言",
		"range.24h":                  "最近 24 小时",
		"range.7d":                   "最近 7 天",
		"range.30d":                  "最近 30 天",
		"range.retention":            "全部保留数据",
		"granularity.minute":         "按分钟",
		"granularity.hour":           "按小时",
		"granularity.day":            "按日",
		"granularity.week":           "按周",
		"granularity.month":          "按月",
		"button.pricing":             "模型价格",
		"button.export":              "导出",
		"button.export_csv":          "导出 CSV",
		"button.export_png":          "导出图表图片",
		"button.refresh":             "刷新",
		"button.reset":               "重置",
		"button.language":            "语言",
		"status.loading":             "正在加载统计数据…",
		"card.total_tokens":          "总消耗 Token",
		"card.estimated_cost":        "预估总费用",
		"card.total_requests":        "总请求次数",
		"card.top_model":             "最常用模型",
		"card.token_detail":          "输入 + 输出 + 其他 Token",
		"card.cost_waiting":          "等待精确费用数据",
		"card.request_detail":        "当前时间筛选范围",
		"card.top_model_detail":      "按调用次数统计",
		"unit.full":                  "完整",
		"unit.switch_to":             "切换为 {unit} 单位",
		"unit.aria":                  "Token 显示单位：{unit}",
		"currency.usd":               "USD",
		"currency.cny":               "CNY",
		"currency.aria":              "费用显示货币：{currency}",
		"currency.switch_usd":        "切换为美元",
		"currency.switch_cny":        "切换为人民币",
		"currency.usd_label":         "美元",
		"currency.cny_label":         "人民币",
		"panel.token_trend":          "Token 消耗趋势",
		"panel.token_trend_sub":      "输入/输出 Token 堆叠",
		"panel.model_share":          "模型调用占比",
		"panel.model_share_sub":      "点击区块或模型名称下钻",
		"panel.cost_trend":           "费用趋势",
		"panel.cost_trend_sub":       "后端价格簿逐请求精确计算",
		"panel.efficiency":           "模型效率",
		"panel.efficiency_sub":       "平均 Token / 请求 × 平均响应延迟",
		"panel.requests":             "请求明细",
		"panel.requests_sub":         "逐请求记录，最新请求优先",
		"panel.dimensions":           "维度明细",
		"panel.dimensions_sub":       "当前筛选视图",
		"button.view_all_models":     "查看全部模型",
		"button.reset_zoom":          "重置缩放",
		"button.prev_page":           "上一页",
		"button.next_page":           "下一页",
		"zoom.tip":                   "滚轮缩放 · Shift + 滚轮平移",
		"zoom.out":                   "缩小",
		"zoom.in":                    "放大",
		"series.input":               "输入",
		"series.output":              "输出",
		"chart.token_bars":           "Token 消耗堆叠柱状图",
		"chart.model_donut":          "模型调用占比环形图",
		"chart.cost_line":            "预估费用趋势折线图",
		"chart.efficiency":           "模型效率散点图",
		"chart.legend":               "模型图例",
		"metrics.aria":               "关键指标",
		"analysis.aria":              "高级分析",
		"table.time":                 "时间",
		"table.model_name":           "模型名称",
		"table.source":               "来源",
		"table.tier":                 "层级",
		"table.result":               "结果",
		"table.ttft":                 "首字延迟",
		"table.generation":           "生成时间",
		"table.tps":                  "TPS",
		"table.reasoning":            "思考强度",
		"table.input":                "输入",
		"table.output":               "输出",
		"table.thinking":             "思考",
		"table.cache_read":           "缓存读取",
		"table.cache_creation":       "缓存创建",
		"table.total_tokens":         "总 Token 数",
		"table.cache_hit":            "缓存命中",
		"table.estimated_cost":       "预估费用",
		"table.price_source":         "价格来源",
		"table.model":                "模型",
		"table.provider":             "提供商",
		"table.alias":                "别名",
		"table.executor":             "执行器",
		"table.auth_type":            "认证类型",
		"table.service_tier":         "服务层级",
		"table.reasoning_effort":     "推理强度",
		"table.requests":             "请求",
		"table.failures":             "失败",
		"table.avg_latency":          "平均延迟",
		"table.avg_ttft":             "平均 TTFT",
		"pricing.title":              "模型价格与同步",
		"pricing.intro":              "模型目录直接读取 CLIProxyAPI /v1/models；CLIProxyAPI API 密钥与管理密钥均仅用于本次操作。单价单位固定为 USD / 1M Token。",
		"pricing.revision":           "价格簿版本：—",
		"pricing.coverage_wait":      "价格覆盖：等待费用数据",
		"pricing.missing":            "缺失价格：—",
		"pricing.models_unloaded":    "CLIProxyAPI 模型：尚未加载",
		"pricing.pending_delete":     "待保存删除：0 项",
		"pricing.never_synced":       "尚未同步 models.dev",
		"pricing.cli_key":            "CLIProxyAPI API 密钥（不会保存）",
		"pricing.load_models":        "加载当前模型",
		"pricing.model_source":       "加载成功后，价格编辑器和 models.dev 同步均以该模型列表为准。",
		"pricing.provider_priority":  "提供商优先级（逗号或换行分隔）",
		"pricing.ignored_suffixes":   "忽略模型后缀（逗号或换行分隔）",
		"pricing.mappings":           "模型映射（每行 source=target）",
		"pricing.management_key":     "管理密钥（不会保存）",
		"placeholder.api_key":        "API 密钥",
		"placeholder.management_key": "管理密钥",
		"pricing.cancel":             "取消",
		"pricing.sync":               "同步 models.dev",
		"pricing.save":               "保存价格与设置",
		"reset.title":                "管理密钥确认",
		"reset.intro":                "清空统计需要 CLIProxyAPI 管理密钥。密钥只用于本次请求。",
		"reset.cancel":               "取消",
		"reset.confirm":              "确认清空",
		"lang.auto":                  "自动（跟随浏览器）",
		"lang.en":                    "English",
		"lang.zh-CN":                 "中文",
		"css.pending_delete":         "保存后删除",

		"js.unlabeled_model":             "未标记模型",
		"js.all_models":                  "全部模型",
		"js.request_failed":              "请求失败（HTTP {status}）",
		"js.invalid_exchange_rate":       "汇率接口返回了无效数据。",
		"js.cny_unavailable":             "人民币汇率不可用：{error}",
		"js.exact_cost_unavailable":      "精确费用不可用",
		"js.price_coverage":              "价格覆盖 {priced} / {total} · 未定价 {unpriced}",
		"js.cost_breakdown":              "输入 {input} · 输出 {output} · 缓存读取 {cacheRead} · 缓存创建 {cacheCreation} · 计费模式 {mode} · 层级 {tier}",
		"js.token_summary_detail":        "输入 {input} · 输出 {output}",
		"js.tier_base":                   "基础",
		"js.status_range":                "范围：{range} · 最近活动：{lastUsed}{costStatus} · 更新：{updated}",
		"js.cost_unavailable":            " · 费用不可用",
		"js.cost_status":                 " · {coverage} · 价格簿 #{revision}",
		"js.cost_data_error":             "费用数据不可用：{error}",
		"js.cached_rate":                 " · 缓存汇率",
		"js.rate_line":                   " · 1 USD = {rate} CNY{stale} · {time}",
		"js.unavailable":                 "不可用",
		"js.cost_api_error":              "费用接口错误",
		"js.cost_data_unavailable":       "费用数据不可用",
		"js.success_rate":                "成功率 {rate}%",
		"js.calls_tokens":                "{requests} 次调用 · {tokens} Token",
		"js.only_model":                  "仅显示 {model}",
		"js.model_filter":                "模型筛选：{model}",
		"js.requests_sub_filtered":       "模型筛选：{model} · 最新请求优先",
		"js.requests_sub_costs":          "最新请求优先 · 价格簿 #{revision}",
		"js.requests_sub_no_cost":        "最新请求优先 · 费用不可用",
		"js.no_usage":                    "该时间段内暂无调用记录",
		"js.no_requests":                 "该时间段内暂无请求记录",
		"js.groups_count":                "{count} 个分组",
		"js.result_success":              "成功",
		"js.result_failed":               "失败",
		"js.cache_hit":                   "命中",
		"js.cache_miss":                  "未命中",
		"js.unpriced":                    "未定价",
		"js.request_title_priced":        "计费模式 {mode} · 上下文 {context} · 可计费输入 {billable} · 缓存读取 {cacheRead}",
		"js.request_title_unpriced":      "此请求未匹配价格",
		"js.requests_count":              "{total} 条请求 · 价格簿 #{revision}",
		"js.week_suffix":                 " 周",
		"js.exact_cost":                  "精确费用",
		"js.unpriced_requests":           "未定价请求",
		"js.avg_tokens_request":          "平均 Token / 请求",
		"js.avg_latency":                 "平均响应延迟",
		"js.request_count":               "请求次数",
		"js.total_tokens":                "总 Token 数",
		"js.no_data":                     "暂无数据",
		"js.exact_cost_data_unavailable": "精确费用数据不可用",
		"js.no_cost_data":                "该时间段内暂无费用数据",
		"js.models_count":                "{count} 个模型",
		"js.no_efficiency":               "该时间段内暂无模型效率数据",
		"js.empty_hint":                  "尝试调整时间范围或恢复已隐藏的模型",
		"js.bar_aria":                    "{label} 总 Token {total}",
		"js.donut_segment_aria":          "{model}，占比 {share}",
		"js.cost_point_aria":             "{label}，费用 {cost}",
		"js.no_model_data":               "暂无模型数据",
		"js.show_model":                  "显示 {model}",
		"js.hide_model":                  "隐藏 {model}",
		"js.legend_detail":               "{requests} 次 · {tokens} Token",
		"js.legend_tooltip":              "{model} · {detail}",
		"js.call_count":                  "调用次数",
		"js.tokens_used":                 "消耗 Token",
		"js.share":                       "占比",
		"js.remove":                      "移除",
		"js.invalid_models_response":     "CLIProxyAPI 模型响应格式无效。",
		"js.model_name_too_long":         "CLIProxyAPI 返回了过长的模型名称。",
		"js.too_many_models":             "CLIProxyAPI 返回的模型数量过多。",
		"js.no_available_models":         "CLIProxyAPI 当前没有可用模型。",
		"js.enter_cli_key":               "请输入 CLIProxyAPI API 密钥后再加载模型。",
		"js.missing_cli_key":             "缺少 CLIProxyAPI API 密钥",
		"js.models_loading":              "CLIProxyAPI 模型：加载中…",
		"js.models_loaded":               "CLIProxyAPI 模型：{count} 个 · {time}",
		"js.models_load_failed":          "CLIProxyAPI 模型：加载失败",
		"js.models_not_loaded":           "CLIProxyAPI 模型：尚未加载",
		"js.pending_delete_status":       "待保存删除：{count} 项{extra}",
		"js.pending_delete_extra":        " · 保存后费用将重新计算",
		"js.price_revision":              "价格簿版本：#{revision}",
		"js.price_coverage_unavailable":  "价格覆盖：费用数据不可用",
		"js.missing_prices":              "缺失价格：{list}",
		"js.missing_prices_more":         " 等 {count} 项",
		"js.missing_prices_none":         "缺失价格：无",
		"js.last_sync":                   "上次同步 {completed} · 来源模型 {observed} · 待处理 {pending} · 成功 {matched} · 错误/未匹配 {unmatched} · 新增 {created} · 更新 {updated} · 保留手动 {skipped}",
		"js.undo_delete":                 "撤销删除",
		"js.delete_price":                "删除价格",
		"js.price_head_model":            "模型",
		"js.price_head_source":           "来源 / 溯源",
		"js.price_input":                 "输入",
		"js.price_output":                "输出",
		"js.price_cache_read":            "缓存读取",
		"js.price_cache_creation":        "缓存创建",
		"js.no_configurable_models":      "CLIProxyAPI 当前没有可配置模型",
		"js.load_models_prompt":          "请输入 CLIProxyAPI API 密钥并加载当前模型",
		"js.unit_price":                  "{model} {kind}单价",
		"js.tier_threshold_aria":         "{model} 上下文层级阈值",
		"js.tier_price_aria":             "{model} 上下文层级{kind}单价",
		"js.source_unset":                "未设置来源",
		"js.manual_price":                "手动价格",
		"js.not_in_catalog":              " · 不在当前 CLIProxyAPI 模型列表",
		"js.context_tiers":               "上下文分层 {count} 项",
		"js.expand_tiers":                "展开上下文分层 {count} 项",
		"js.collapse_tiers":              "收起上下文分层 {count} 项",
		"js.threshold_tokens":            "Token 阈值",
		"js.actions":                     "操作",
		"js.accounting_mode":             "计费模式",
		"js.mode_default":                "默认",
		"js.mode_excludes_cache":         "输入不含缓存",
		"js.mode_includes_cache":         "输入包含缓存",
		"js.add_context_tier":            "添加上下文层级",
		"js.mapping_invalid":             "模型映射必须使用 source=target，每行一条。",
		"js.price_range_invalid":         "模型价格必须是 0 到 1000000 之间的数字。",
		"js.tier_invalid":                "上下文层级阈值必须是大于 0 的整数，价格必须在有效范围内。",
		"js.enter_management_key_save":   "请输入管理密钥后再保存。",
		"js.enter_management_key_sync":   "请输入管理密钥后再同步。",
		"js.prices_saved":                "模型价格与同步设置已保存 · 价格簿 #{revision} · {time}",
		"js.syncing_models":              "同步中 · 正在读取 CLIProxyAPI 当前模型…",
		"js.syncing_match":               "同步中 · 正在匹配 {count} 个 CLIProxyAPI 模型…",
		"js.sync_done":                   "models.dev 同步完成 · 来源模型 {count} 个 · 价格簿 #{revision} · {time}",
		"js.sync_failed":                 "同步失败：{error}",
		"js.csv_preparing":               "正在准备逐请求 CSV…",
		"js.csv_yes":                     "是",
		"js.csv_no":                      "否",
		"js.csv_input_usd":               "输入费用（USD）",
		"js.csv_output_usd":              "输出费用（USD）",
		"js.csv_cache_read_usd":          "缓存读取费用（USD）",
		"js.csv_cache_creation_usd":      "缓存创建费用（USD）",
		"js.csv_total_usd":               "总费用（USD）",
		"js.csv_tier_threshold":          "层级阈值",
		"js.csv_exported":                "已导出 {count} 条逐请求记录 · 精确费用来自 estimated_cost",
		"js.png_range":                   "范围：{range} · 导出：{time}",
		"js.png_all_visible":             "全部可见模型",
		"js.png_model":                   "模型：{model}",
		"js.reset_confirm":               "确定永久删除全部 Token 统计吗？此操作不可撤销。",
		"js.reset_prompt":                "请输入 reset 确认：",
		"js.plugin_id_error":             "无法从插件资源 URL 识别 plugin ID。",
		"js.cost_missing_price":          "此请求缺少价格，无法估算费用",
		"js.syncing":                     "同步中",
		"js.price_coverage_label":        "价格覆盖",
	},
}

func normalizeLanguage(raw string) Language {
	value := strings.TrimSpace(strings.ToLower(raw))
	value = strings.ReplaceAll(value, "_", "-")
	if value == "" || value == "auto" {
		return LanguageAuto
	}
	for _, definition := range languageDefinitions {
		if value == strings.ToLower(string(definition.Code)) {
			return definition.Code
		}
	}
	switch {
	case strings.HasPrefix(value, "en-"):
		return LanguageEnglish
	case strings.HasPrefix(value, "zh-cn-"), value == "zh-sg", strings.HasPrefix(value, "zh-sg-"), value == "zh-hans", strings.HasPrefix(value, "zh-hans-"):
		return LanguageChinese
	default:
		return ""
	}
}

func parseAcceptLanguage(header string) Language {
	if strings.TrimSpace(header) == "" {
		return LanguageEnglish
	}
	bestLang := LanguageEnglish
	bestWeight := -1.0
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag := part
		weight := 1.0
		if semi := strings.Index(part, ";"); semi >= 0 {
			tag = strings.TrimSpace(part[:semi])
			for _, param := range strings.Split(part[semi+1:], ";") {
				param = strings.TrimSpace(param)
				if len(param) >= 2 && (param[0] == 'q' || param[0] == 'Q') && param[1] == '=' {
					if parsed, err := strconv.ParseFloat(strings.TrimSpace(param[2:]), 64); err == nil {
						weight = parsed
					}
				}
			}
		}
		if weight <= 0 {
			continue
		}
		lang := normalizeLanguage(tag)
		if lang == "" || lang == LanguageAuto {
			continue
		}
		if weight > bestWeight {
			bestWeight = weight
			bestLang = lang
		}
	}
	return bestLang
}

func resolveLanguage(configured Language, acceptLanguage, queryLang string) Language {
	if strings.TrimSpace(queryLang) != "" {
		if explicit := normalizeLanguage(queryLang); explicit != "" {
			if explicit == LanguageAuto {
				return parseAcceptLanguage(acceptLanguage)
			}
			return explicit
		}
	}
	if configured != LanguageAuto && configured != "" {
		if normalized := normalizeLanguage(string(configured)); normalized != "" && normalized != LanguageAuto {
			return normalized
		}
	}
	return parseAcceptLanguage(acceptLanguage)
}

func languagePreference(configured Language, queryLang string) Language {
	if strings.TrimSpace(queryLang) != "" {
		if explicit := normalizeLanguage(queryLang); explicit != "" {
			return explicit
		}
	}
	if normalized := normalizeLanguage(string(configured)); normalized != "" {
		return normalized
	}
	return LanguageAuto
}

func effectiveLanguage(configured Language) Language {
	if configured == LanguageAuto || configured == "" {
		return LanguageEnglish
	}
	if normalized := normalizeLanguage(string(configured)); normalized != "" && normalized != LanguageAuto {
		return normalized
	}
	return LanguageEnglish
}

func T(lang Language, key string) string {
	lang = effectiveLanguage(lang)
	if catalog := messages[lang]; catalog != nil {
		if value, ok := catalog[key]; ok {
			return value
		}
	}
	if catalog := messages[LanguageEnglish]; catalog != nil {
		if value, ok := catalog[key]; ok {
			return value
		}
	}
	return key
}

func supportedLanguageValues(includeAuto bool) []string {
	values := make([]string, 0, len(languageDefinitions)+1)
	if includeAuto {
		values = append(values, string(LanguageAuto))
	}
	for _, definition := range languageDefinitions {
		values = append(values, string(definition.Code))
	}
	return values
}

func languageOptionsHTML(lang, preferred Language) string {
	var result strings.Builder
	options := []struct {
		code Language
		key  string
	}{{code: LanguageAuto, key: "lang.auto"}}
	for _, definition := range languageDefinitions {
		options = append(options, struct {
			code Language
			key  string
		}{code: definition.Code, key: definition.LabelKey})
	}
	for _, option := range options {
		result.WriteString(`<option value="`)
		result.WriteString(html.EscapeString(string(option.code)))
		result.WriteString(`"`)
		if option.code == preferred {
			result.WriteString(" selected")
		}
		result.WriteString(">")
		result.WriteString(html.EscapeString(T(lang, option.key)))
		result.WriteString("</option>")
	}
	return result.String()
}

func catalogJSON() ([]byte, error) {
	payload := make(map[string]map[string]string, len(supportedLanguages))
	for _, lang := range supportedLanguages {
		payload[string(lang)] = messages[lang]
	}
	return json.Marshal(payload)
}

func languageFromHTTP(configured Language, headers http.Header, query url.Values) Language {
	accept := ""
	if headers != nil {
		accept = headers.Get("Accept-Language")
	}
	queryLang := ""
	if query != nil {
		queryLang = query.Get("lang")
	}
	return resolveLanguage(configured, accept, queryLang)
}

func displayModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || model == unlabeledModel || model == legacyUnlabeledModel {
		return legacyUnlabeledModel
	}
	return model
}

func matchesModelFilter(itemModel, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	return displayModelName(itemModel) == displayModelName(filter)
}

func priceLookupKeys(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" || model == unlabeledModel || model == legacyUnlabeledModel {
		return []string{legacyUnlabeledModel, unlabeledModel}
	}
	return []string{model}
}

func formatResultLabel(failed bool, failureStatus int) string {
	if !failed {
		return legacyResultSuccess
	}
	if failureStatus > 0 {
		return legacyResultFailed + " (HTTP " + strconv.Itoa(failureStatus) + ")"
	}
	return legacyResultFailed
}

func localeTag(lang Language) string {
	lang = effectiveLanguage(lang)
	for _, definition := range languageDefinitions {
		if definition.Code == lang {
			return definition.Locale
		}
	}
	return languageDefinitions[0].Locale
}
