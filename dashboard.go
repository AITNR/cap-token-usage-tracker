package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// handleDashboard renders the full HTML dashboard page.
func handleDashboard(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	stats := tracker.GetStats()
	html := renderDashboardHTML(stats)
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       []byte(html),
	}
}

// formatDuration converts a time.Duration to a human-readable string.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// formatNumber adds thousands separators to large numbers.
func formatNumber(n int64) string {
	if n == 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// renderDashboardHTML generates the complete dashboard HTML with embedded CSS and JavaScript.
func renderDashboardHTML(stats StatsResponse) string {
	var sb strings.Builder

	// Summary card data
	successRate := float64(0)
	if stats.Summary.Requests > 0 {
		successRate = float64(stats.Summary.Requests-stats.Summary.FailedRequests) / float64(stats.Summary.Requests) * 100
	}
	sinceStr := stats.Since.Format("2006-01-02 15:04:05 MST")
	lastUsedStr := "—"
	if !stats.LastUsed.IsZero() {
		lastUsedStr = stats.LastUsed.Format("2006-01-02 15:04:05 MST")
	}

	// Build model rows
	var rows strings.Builder
	if len(stats.Models) == 0 {
		rows.WriteString(`<tr><td colspan="12" class="empty-state">暂无用量数据。开始发起 API 请求后，统计数据将显示在此处。</td></tr>`)
	} else {
		for _, m := range stats.Models {
			successPct := float64(0)
			if m.Requests > 0 {
				successPct = float64(m.Requests-m.FailedRequests) / float64(m.Requests) * 100
			}
			providerDisplay := m.Provider
			if providerDisplay == "" {
				providerDisplay = "—"
			}
			modelDisplay := m.Model
			if modelDisplay == "" {
				modelDisplay = "—"
			}
			failureBadge := ""
			if m.FailedRequests > 0 {
				failureBadge = fmt.Sprintf(`<span class="badge badge-error">%d 次失败</span>`, m.FailedRequests)
			}
			rows.WriteString(fmt.Sprintf(`
				<tr>
					<td class="col-model">%s</td>
					<td class="col-provider">%s</td>
					<td class="num">%s</td>
					<td class="num">%s</td>
					<td class="num">%s</td>
					<td class="num">%s</td>
					<td class="num">%s</td>
					<td class="num">%s</td>
					<td class="num">%s</td>
					<td class="num">%.1f%%</td>
					<td class="num">%s</td>
					<td class="num">%s</td>
				</tr>`,
				modelDisplay,
				providerDisplay,
				formatNumber(m.Requests),
				formatNumber(m.InputTokens),
				formatNumber(m.OutputTokens),
				formatNumber(m.ReasoningTokens),
				formatNumber(m.CachedTokens),
				formatNumber(m.CacheReadTokens),
				formatNumber(m.TotalTokens),
				successPct,
				formatDuration(m.AvgLatency()),
				formatDuration(m.AvgTTFT()),
			))
			_ = failureBadge // failure badge rendered inline below if needed
		}
	}

	sb.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<meta http-equiv="refresh" content="5">
	<title>Token 用量追踪 — 仪表盘</title>
	<style>
		:root {
			--bg: #0d1117;
			--bg-card: #161b22;
			--bg-card-hover: #1c2330;
			--border: #30363d;
			--text: #e6edf3;
			--text-muted: #8b949e;
			--text-dim: #6e7681;
			--accent: #58a6ff;
			--accent-glow: rgba(88, 166, 255, 0.15);
			--green: #3fb950;
			--green-bg: rgba(63, 185, 80, 0.1);
			--red: #f85149;
			--red-bg: rgba(248, 81, 73, 0.1);
			--yellow: #d29922;
			--yellow-bg: rgba(210, 153, 34, 0.1);
			--purple: #bc8cff;
			--purple-bg: rgba(188, 140, 255, 0.1);
			--orange: #e8873a;
		}
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
			background: var(--bg);
			color: var(--text);
			min-height: 100vh;
			padding: 24px;
		}
		.header {
			display: flex;
			justify-content: space-between;
			align-items: center;
			margin-bottom: 32px;
			flex-wrap: wrap;
			gap: 16px;
		}
		.header-left h1 {
			font-size: 24px;
			font-weight: 600;
			display: flex;
			align-items: center;
			gap: 10px;
		}
		.header-left h1 .icon {
			width: 28px;
			height: 28px;
			background: var(--accent);
			border-radius: 6px;
			display: flex;
			align-items: center;
			justify-content: center;
			font-size: 16px;
			color: var(--bg);
		}
		.header-left p {
			color: var(--text-muted);
			font-size: 13px;
			margin-top: 4px;
		}
		.header-right {
			display: flex;
			gap: 12px;
			align-items: center;
		}
		.auto-refresh {
			display: flex;
			align-items: center;
			gap: 6px;
			font-size: 13px;
			color: var(--text-muted);
		}
		.auto-refresh .dot {
			width: 8px;
			height: 8px;
			background: var(--green);
			border-radius: 50%;
			animation: pulse 2s ease-in-out infinite;
		}
		@keyframes pulse {
			0%%, 100%% { opacity: 1; }
			50%% { opacity: 0.4; }
		}
		.btn {
			padding: 8px 16px;
			border-radius: 6px;
			font-size: 13px;
			font-weight: 500;
			cursor: pointer;
			border: 1px solid var(--border);
			background: var(--bg-card);
			color: var(--text);
			text-decoration: none;
			display: inline-flex;
			align-items: center;
			gap: 6px;
			transition: all 0.2s;
		}
		.btn:hover { background: var(--bg-card-hover); border-color: var(--accent); }
		.btn-danger { border-color: var(--red); color: var(--red); }
		.btn-danger:hover { background: var(--red-bg); }

		.cards {
			display: grid;
			grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
			gap: 16px;
			margin-bottom: 32px;
		}
		.card {
			background: var(--bg-card);
			border: 1px solid var(--border);
			border-radius: 12px;
			padding: 20px;
			transition: border-color 0.2s;
		}
		.card:hover { border-color: var(--text-dim); }
		.card .label {
			font-size: 12px;
			color: var(--text-muted);
			text-transform: uppercase;
			letter-spacing: 0.5px;
			margin-bottom: 8px;
		}
		.card .value {
			font-size: 28px;
			font-weight: 700;
			line-height: 1.2;
		}
		.card .sub {
			font-size: 12px;
			color: var(--text-dim);
			margin-top: 4px;
		}
		.card-accent .value { color: var(--accent); }
		.card-green .value { color: var(--green); }
		.card-purple .value { color: var(--purple); }
		.card-orange .value { color: var(--orange); }
		.card-yellow .value { color: var(--yellow); }

		.meta-bar {
			display: flex;
			gap: 24px;
			margin-bottom: 24px;
			font-size: 13px;
			color: var(--text-muted);
			flex-wrap: wrap;
		}
		.meta-bar span { display: flex; align-items: center; gap: 4px; }

		.table-container {
			background: var(--bg-card);
			border: 1px solid var(--border);
			border-radius: 12px;
			overflow: hidden;
		}
		.table-header {
			padding: 16px 20px;
			border-bottom: 1px solid var(--border);
			font-size: 14px;
			font-weight: 600;
		}
		.table-wrap { overflow-x: auto; }
		table {
			width: 100%%;
			border-collapse: collapse;
			font-size: 13px;
		}
		th {
			text-align: left;
			padding: 10px 16px;
			color: var(--text-muted);
			font-weight: 500;
			font-size: 12px;
			text-transform: uppercase;
			letter-spacing: 0.5px;
			border-bottom: 1px solid var(--border);
			white-space: nowrap;
		}
		td {
			padding: 10px 16px;
			border-bottom: 1px solid var(--border);
			white-space: nowrap;
		}
		tr:last-child td { border-bottom: none; }
		tr:hover td { background: var(--bg-card-hover); }
		.num { text-align: right; font-variant-numeric: tabular-nums; }
		.col-model { font-weight: 500; }
		.col-provider { color: var(--text-muted); }
		.empty-state {
			text-align: center !important;
			padding: 48px 16px !important;
			color: var(--text-muted);
			font-size: 14px;
		}
		.badge {
			display: inline-block;
			padding: 2px 8px;
			border-radius: 12px;
			font-size: 11px;
			font-weight: 500;
		}
		.badge-error { background: var(--red-bg); color: var(--red); }
		.footer {
			margin-top: 24px;
			text-align: center;
			font-size: 12px;
			color: var(--text-dim);
		}
	</style>
</head>
<body>
	<div class="header">
		<div class="header-left">
			<h1><span class="icon">📊</span> Token 用量追踪</h1>
			<p>所有模型和提供商的实时 Token 用量统计</p>
		</div>
		<div class="header-right">
			<div class="auto-refresh"><span class="dot"></span> 自动刷新 5秒</div>
			<a class="btn" href="stats">查看 JSON</a>
			<button class="btn btn-danger" onclick="resetStats()">重置数据</button>
		</div>
	</div>

	<div class="cards">
		<div class="card card-accent">
			<div class="label">总 Token 数</div>
			<div class="value">%s</div>
			<div class="sub">输入 + 输出 + 缓存</div>
		</div>
		<div class="card card-green">
			<div class="label">总请求数</div>
			<div class="value">%s</div>
			<div class="sub">成功率 %.1f%%</div>
		</div>
		<div class="card card-purple">
			<div class="label">输入 Token</div>
			<div class="value">%s</div>
		</div>
		<div class="card card-orange">
			<div class="label">输出 Token</div>
			<div class="value">%s</div>
		</div>
		<div class="card card-yellow">
			<div class="label">推理 Token</div>
			<div class="value">%s</div>
		</div>
		<div class="card">
			<div class="label">缓存 Token</div>
			<div class="value">%s</div>
			<div class="sub">读取: %s · 创建: %s</div>
		</div>
	</div>

	<div class="meta-bar">
		<span>追踪起始：<strong>%s</strong></span>
		<span>最近活动：<strong>%s</strong></span>
		<span>追踪模型数：<strong>%d</strong></span>
	</div>

	<div class="table-container">
		<div class="table-header">按模型明细</div>
		<div class="table-wrap">
			<table>
				<thead>
					<tr>
						<th>模型</th>
						<th>提供商</th>
						<th>请求数</th>
						<th>输入</th>
						<th>输出</th>
						<th>推理</th>
						<th>缓存</th>
						<th>缓存读取</th>
						<th>总 Token</th>
						<th>成功率</th>
						<th>平均延迟</th>
						<th>平均首Token</th>
					</tr>
				</thead>
				<tbody>
					%s
				</tbody>
			</table>
		</div>
	</div>

	<div class="footer">
		由 <strong>Token Usage Tracker</strong> — CLIProxyAPI 插件 提供支持
	</div>

	<script>
		async function resetStats() {
			if (!confirm('确定要重置所有用量计数器吗？此操作不可撤销。')) return;
			try {
				const resp = await fetch('reset', { method: 'POST' });
				if (resp.ok) {
					alert('所有计数器已重置。');
					location.reload();
				} else {
					alert('重置失败：' + resp.status);
				}
			} catch (e) {
				alert('重置失败：' + e.message);
			}
		}
	</script>
</body>
</html>`,
		// Cards
		formatNumber(stats.Summary.TotalTokens),
		formatNumber(stats.Summary.Requests),
		successRate,
		formatNumber(stats.Summary.InputTokens),
		formatNumber(stats.Summary.OutputTokens),
		formatNumber(stats.Summary.ReasoningTokens),
		formatNumber(stats.Summary.CachedTokens),
		formatNumber(stats.Summary.CacheReadTokens),
		formatNumber(stats.Summary.CacheCreationTokens),
		// Meta bar
		sinceStr,
		lastUsedStr,
		len(stats.Models),
		// Table rows
		rows.String(),
	))

	return sb.String()
}
