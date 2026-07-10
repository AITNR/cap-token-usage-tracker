package main

import "strings"

func buildDashboardHTML(statsJSON string) string {
	if statsJSON == "" {
		statsJSON = "{}"
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta http-equiv="refresh" content="5">
<title>Token Usage Tracker</title>
<style>
:root {
  --bg-primary: #0f0f23;
  --bg-secondary: #1a1a35;
  --bg-card: #1e1e3a;
  --bg-table-row: #16162e;
  --bg-table-hover: #24244a;
  --border: #2a2a50;
  --text-primary: #e4e4f0;
  --text-secondary: #9090b0;
  --text-muted: #606080;
  --accent: #7c5cff;
  --accent-light: #9d85ff;
  --green: #34d399;
  --red: #f87171;
  --yellow: #fbbf24;
  --blue: #60a5fa;
  --cyan: #22d3ee;
}
* { margin:0; padding:0; box-sizing:border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  background: var(--bg-primary);
  color: var(--text-primary);
  line-height: 1.6;
  min-height: 100vh;
}
.container { max-width: 1400px; margin: 0 auto; padding: 24px; }
.header {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 28px; padding-bottom: 16px;
  border-bottom: 1px solid var(--border);
}
.header h1 {
  font-size: 24px; font-weight: 600;
  background: linear-gradient(135deg, var(--accent-light), var(--cyan));
  -webkit-background-clip: text; -webkit-text-fill-color: transparent;
  background-clip: text;
}
.header .meta { font-size: 12px; color: var(--text-muted); }
.header .meta span { margin-left: 16px; }
.refresh-dot {
  display: inline-block; width: 6px; height: 6px;
  background: var(--green); border-radius: 50%;
  margin-right: 4px; animation: pulse 2s infinite;
}
@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.3} }

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px; margin-bottom: 28px;
}
.card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 20px;
  transition: border-color 0.2s;
}
.card:hover { border-color: var(--accent); }
.card .label { font-size: 12px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 8px; }
.card .value { font-size: 28px; font-weight: 700; font-variant-numeric: tabular-nums; }
.card .sub { font-size: 12px; color: var(--text-muted); margin-top: 4px; }
.card.accent .value { color: var(--accent-light); }
.card.green .value { color: var(--green); }
.card.yellow .value { color: var(--yellow); }
.card.blue .value { color: var(--blue); }
.card.cyan .value { color: var(--cyan); }
.card.red .value { color: var(--red); }

.section { margin-bottom: 28px; }
.section-title {
  font-size: 16px; font-weight: 600; color: var(--text-primary);
  margin-bottom: 12px; padding-bottom: 8px;
  border-bottom: 1px solid var(--border);
}

.table-wrap {
  overflow-x: auto; border-radius: 12px;
  border: 1px solid var(--border); background: var(--bg-card);
}
table { width: 100%; border-collapse: collapse; font-size: 13px; }
thead th {
  background: var(--bg-secondary); color: var(--text-secondary);
  font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px;
  font-size: 11px; padding: 12px 16px; text-align: left;
  border-bottom: 1px solid var(--border);
  position: sticky; top: 0;
}
tbody td { padding: 10px 16px; border-bottom: 1px solid var(--border); font-variant-numeric: tabular-nums; }
tbody tr:hover { background: var(--bg-table-hover); }
tbody tr:last-child td { border-bottom: none; }
.num { text-align: right; }
.model-name { color: var(--accent-light); font-weight: 500; }
.provider-name { color: var(--cyan); font-weight: 500; }
.badge {
  display: inline-block; padding: 2px 8px; border-radius: 6px;
  font-size: 11px; font-weight: 600;
}
.badge-ok { background: rgba(52,211,153,0.15); color: var(--green); }
.badge-fail { background: rgba(248,113,113,0.15); color: var(--red); }

.empty-state {
  text-align: center; padding: 60px 20px; color: var(--text-muted);
}
.empty-state .icon { font-size: 48px; margin-bottom: 12px; }
.empty-state .msg { font-size: 14px; }

.footer {
  text-align: center; padding: 20px 0; font-size: 12px;
  color: var(--text-muted); border-top: 1px solid var(--border);
  margin-top: 20px;
}
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <h1>📊 Token Usage Tracker</h1>
    <div class="meta">
      <span class="refresh-dot"></span>Auto-refresh 5s
      <span id="updateTime"></span>
    </div>
  </div>

  <div class="summary-grid" id="summaryCards"></div>

  <div class="section" id="modelsSection">
    <div class="section-title">Model Breakdown</div>
    <div id="modelsTable"></div>
  </div>

  <div class="section" id="providersSection">
    <div class="section-title">Provider Breakdown</div>
    <div id="providersTable"></div>
  </div>

  <div class="footer">Token Usage Tracker &middot; CLIProxyAPI Plugin</div>
</div>

<script>
const initialData = `)
	b.WriteString(statsJSON)
	b.WriteString(`;

function fmt(n) {
  if (n == null) return '0';
  return n.toLocaleString();
}
function fmtK(n) {
  if (n == null || n === 0) return '0';
  if (n >= 1e9) return (n/1e9).toFixed(1) + 'B';
  if (n >= 1e6) return (n/1e6).toFixed(1) + 'M';
  if (n >= 1e3) return (n/1e3).toFixed(1) + 'K';
  return String(n);
}
function fmtMs(n) {
  if (n == null || n === 0) return '-';
  if (n >= 1000) return (n/1000).toFixed(1) + 's';
  return n.toFixed(0) + 'ms';
}

function renderCards(s) {
  const cards = [
    {label:'Total Requests', value:fmt(s.requests), sub:'all time', cls:'accent'},
    {label:'Total Tokens', value:fmtK(s.total_tokens), sub:fmt(s.total_tokens), cls:'blue'},
    {label:'Input Tokens', value:fmtK(s.input_tokens), sub:fmt(s.input_tokens), cls:'cyan'},
    {label:'Output Tokens', value:fmtK(s.output_tokens), sub:fmt(s.output_tokens), cls:'green'},
    {label:'Reasoning', value:fmtK(s.reasoning_tokens), sub:fmt(s.reasoning_tokens), cls:'yellow'},
    {label:'Cached Tokens', value:fmtK(s.cached_tokens + s.cache_read_tokens), sub:'read '+fmtK(s.cache_read_tokens)+' · create '+fmtK(s.cache_creation_tokens), cls:''},
    {label:'Avg Latency', value:fmtMs(s.avg_latency_ms), sub:'per request', cls:''},
    {label:'Failed', value:fmt(s.failed_requests), sub:s.requests > 0 ? ((s.failed_requests/s.requests*100).toFixed(1)+'%') : '0%', cls: s.failed_requests > 0 ? 'red' : 'green'},
  ];
  document.getElementById('summaryCards').innerHTML = cards.map(c =>
    '<div class="card '+c.cls+'"><div class="label">'+c.label+'</div><div class="value">'+c.value+'</div><div class="sub">'+c.sub+'</div></div>'
  ).join('');
}

function renderTable(container, rows, nameClass, nameField) {
  if (!rows || rows.length === 0) {
    container.innerHTML = '<div class="empty-state"><div class="icon">📭</div><div class="msg">No usage data yet</div></div>';
    return;
  }
  let html = '<div class="table-wrap"><table><thead><tr>' +
    '<th>'+(nameField==='model'?'Model':'Provider')+'</th>' +
    '<th class="num">Requests</th>' +
    '<th class="num">Input</th>' +
    '<th class="num">Output</th>' +
    '<th class="num">Reasoning</th>' +
    '<th class="num">Cached</th>' +
    '<th class="num">Total</th>' +
    '<th class="num">Avg Latency</th>' +
    '<th class="num">Status</th>' +
    '</tr></thead><tbody>';
  for (const r of rows) {
    const name = nameField === 'model' ? r.model : r.provider;
    const failRate = r.requests > 0 ? (r.failed_requests / r.requests * 100) : 0;
    const badge = r.failed_requests === 0
      ? '<span class="badge badge-ok">OK</span>'
      : '<span class="badge badge-fail">'+r.failed_requests+' fail</span>';
    html += '<tr>' +
      '<td class="'+nameClass+'">'+esc(name)+'</td>' +
      '<td class="num">'+fmt(r.requests)+'</td>' +
      '<td class="num">'+fmtK(r.input_tokens)+'</td>' +
      '<td class="num">'+fmtK(r.output_tokens)+'</td>' +
      '<td class="num">'+fmtK(r.reasoning_tokens)+'</td>' +
      '<td class="num">'+fmtK(r.cached_tokens + r.cache_read_tokens)+'</td>' +
      '<td class="num">'+fmtK(r.total_tokens)+'</td>' +
      '<td class="num">'+fmtMs(r.avg_latency_ms)+'</td>' +
      '<td class="num">'+badge+'</td>' +
      '</tr>';
  }
  html += '</tbody></table></div>';
  container.innerHTML = html;
}

function esc(s) {
  const d = document.createElement('div');
  d.textContent = s || '';
  return d.innerHTML;
}

function render(data) {
  const s = data.summary || {};
  renderCards(s);
  renderTable(document.getElementById('modelsTable'), data.models, 'model-name', 'model');
  renderTable(document.getElementById('providersTable'), data.providers, 'provider-name', 'provider');
  document.getElementById('updateTime').textContent = 'Updated: ' + new Date().toLocaleTimeString();
}

render(initialData);
</script>
</body>
</html>`)
	return b.String()
}
