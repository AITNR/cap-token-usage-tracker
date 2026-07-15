package main

import (
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func dashboardResponse() pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type":           []string{"text/html; charset=utf-8"},
			"Cache-Control":          []string{"no-store"},
			"X-Content-Type-Options": []string{"nosniff"},
			"Referrer-Policy":        []string{"no-referrer"},
			"Content-Security-Policy": []string{
				"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'self'",
			},
		},
		Body: []byte(dashboardHTML),
	}
}

const dashboardHTML = `<!doctype html>
<html lang="zh-CN" data-theme-mode="auto" data-theme="white">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>Token 用量统计</title>
<script>
(function(){
'use strict';
var mode='auto';
try{var saved=window.localStorage.getItem('cap-token-usage-theme');if(saved==='auto'||saved==='white'||saved==='light'||saved==='dark')mode=saved;}catch(_error){}
var dark=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches;
var applied=mode==='auto'?(dark?'dark':'white'):mode;
document.documentElement.setAttribute('data-theme-mode',mode);
if(applied==='light')document.documentElement.removeAttribute('data-theme');else document.documentElement.setAttribute('data-theme',applied);
})();
</script>
<style>
:root{
color-scheme:light;
--bg-secondary:#faf9f5;
--bg-primary:#f0eee8;
--bg-tertiary:#e9e6df;
--bg-hover:#e9e6df;
--bg-quinary:#f6f4ee;
--floating-surface:#fffdf9;
--text-primary:#2d2a26;
--text-secondary:#6d6760;
--text-tertiary:#a29c95;
--text-quaternary:#c0bab3;
--border-color:#e3e1db;
--border-primary:#d5d2cb;
--border-hover:#cecac4;
--primary-color:#8b8680;
--primary-hover:#7f7a74;
--primary-active:#726d67;
--primary-contrast:#fff;
--success-color:#10b981;
--warning-color:#c65746;
--error-color:#c65746;
--error-soft:rgba(198,87,70,.10);
--error-border:rgba(198,87,70,.35);
--chart-area:rgba(139,134,128,.16);
--row-hover:rgba(45,42,38,.035);
--shadow:0 1px 2px rgb(0 0 0/.08);
--shadow-lg:0 20px 48px rgb(0 0 0/.18);
--focus-ring:rgba(139,134,128,.22);
}
[data-theme='white']{
color-scheme:light;
--bg-secondary:#fff;
--bg-primary:#fff;
--bg-tertiary:#f6f6f6;
--bg-hover:#f6f6f6;
--bg-quinary:#fff;
--floating-surface:#fff;
--border-color:#e5e5e5;
--border-primary:#d9d9d9;
--border-hover:#ccc;
--row-hover:rgba(45,42,38,.025);
--shadow:0 1px 2px rgb(0 0 0/.08);
}
[data-theme='dark']{
color-scheme:dark;
--bg-secondary:#151412;
--bg-primary:#1d1b18;
--bg-tertiary:#262320;
--bg-hover:#2e2a26;
--bg-quinary:#191714;
--floating-surface:#2a2723;
--text-primary:#f6f4f1;
--text-secondary:#c9c3bb;
--text-tertiary:#9c958d;
--text-quaternary:#6f6962;
--border-color:#3a3530;
--border-primary:#4a453f;
--border-hover:#5a544d;
--primary-color:#8b8680;
--primary-hover:#9a948e;
--primary-active:#a6a099;
--success-color:#10b981;
--warning-color:#c65746;
--error-color:#e07a6a;
--error-soft:rgba(198,87,70,.18);
--error-border:rgba(198,87,70,.45);
--chart-area:rgba(166,160,153,.17);
--row-hover:rgba(255,255,255,.035);
--shadow:0 1px 3px rgb(0 0 0/.3);
--shadow-lg:0 20px 48px rgb(0 0 0/.42);
--focus-ring:rgba(166,160,153,.25);
}
*{box-sizing:border-box}
html{min-height:100%;background:var(--bg-secondary)}
body{margin:0;min-height:100vh;background:var(--bg-secondary);color:var(--text-primary);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei","PingFang SC",Roboto,Helvetica,Arial,sans-serif;transition:background-color .3s ease,color .3s ease}
button,select,input{font:inherit}
button,select,input,dialog,.card,.panel{transition:background-color .2s ease,border-color .2s ease,color .2s ease,box-shadow .2s ease}
button{cursor:pointer}
button:focus-visible,select:focus-visible,input:focus-visible{outline:none;box-shadow:0 0 0 3px var(--focus-ring);border-color:var(--primary-color)}
button:disabled{cursor:not-allowed;opacity:.58}
.shell{width:100%;max-width:1540px;margin:0 auto;padding:28px clamp(18px,3vw,46px) 42px}
.top{display:flex;align-items:flex-start;justify-content:space-between;gap:24px;margin-bottom:22px}
.heading{min-width:0;padding-top:2px}
.eyebrow{display:flex;align-items:center;gap:8px;margin-bottom:7px;color:var(--text-secondary);font-size:12px;font-weight:700;letter-spacing:.06em;text-transform:uppercase}
.eyebrow-mark{width:8px;height:8px;border-radius:50%;background:var(--success-color);box-shadow:0 0 0 4px rgba(16,185,129,.11)}
h1{margin:0;color:var(--text-primary);font-size:clamp(24px,2.3vw,32px);font-weight:750;letter-spacing:-.035em;line-height:1.18}
.subtitle{max-width:760px;margin-top:7px;color:var(--text-secondary);font-size:13px}
.controls{display:flex;align-items:center;justify-content:flex-end;gap:8px;flex-wrap:wrap}
.control,button.control{min-height:38px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-primary);color:var(--text-primary);box-shadow:var(--shadow)}
select.control{min-width:142px;padding:8px 34px 8px 12px;cursor:pointer}
button.control{display:inline-flex;align-items:center;justify-content:center;gap:7px;padding:8px 13px;font-weight:600}
button.control:hover,select.control:hover{border-color:var(--border-hover);background:var(--bg-tertiary)}
button.primary{border-color:var(--primary-color);background:var(--primary-color);color:var(--primary-contrast)}
button.primary:hover{border-color:var(--primary-hover);background:var(--primary-hover)}
button.danger{border-color:var(--warning-color);background:var(--warning-color);color:#fff}
button.danger:hover{filter:brightness(.94)}
.icon-button{width:38px;padding:0!important;color:var(--text-secondary)!important;background:var(--bg-primary)!important}
.icon-button:hover{color:var(--text-primary)!important;background:var(--bg-tertiary)!important}
.icon-button svg,.button-icon{width:17px;height:17px;display:block;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}
.theme-icon[hidden]{display:none}
.theme-menu{position:relative;display:inline-flex}
.theme-popover{position:absolute;top:calc(100% + 10px);right:0;z-index:20;display:flex;width:max-content;max-width:calc(100vw - 24px);gap:4px;padding:8px 8px 5px;border:1px solid color-mix(in srgb,var(--border-color) 74%,transparent);border-radius:10px;background:color-mix(in srgb,var(--bg-primary) 90%,transparent);box-shadow:var(--shadow-lg);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px)}
.theme-popover[hidden]{display:none}
.theme-card{display:flex;flex-direction:column;align-items:center;gap:4px;padding:6px 6px 4px;border:2px solid transparent;border-radius:8px;background:transparent;color:var(--text-primary)}
.theme-card:hover{background:color-mix(in srgb,var(--text-primary) 8%,transparent)}
.theme-card.active{border-color:var(--primary-color)}
.theme-preview{display:flex;flex-direction:column;width:72px;height:52px;overflow:hidden;border:1px solid var(--preview-border);border-radius:4px;background:var(--preview-bg)}
.theme-preview-head{height:10px;flex:none;border-bottom:1px solid var(--preview-border);background:var(--preview-card)}
.theme-preview-body{display:flex;flex:1;min-height:0}
.theme-preview-side{width:16px;flex:none;border-right:1px solid var(--preview-border);background:var(--preview-card)}
.theme-preview-content{display:flex;flex:1;flex-direction:column;justify-content:center;gap:4px;padding:5px 8px;background:var(--preview-bg)}
.theme-preview-line{height:3px;border-radius:1px;background:var(--preview-muted)}
.theme-preview-line.short{width:60%}
.preview-auto{--preview-bg:linear-gradient(135deg,#fff 0 50%,#111 50% 100%);--preview-card:linear-gradient(135deg,#fff 0 50%,#1a1a1a 50% 100%);--preview-border:#bdbdbd;--preview-muted:linear-gradient(135deg,#c9c9c9 0 50%,#5a5a5a 50% 100%)}
.preview-white{--preview-bg:#fff;--preview-card:#fff;--preview-border:#e5e5e5;--preview-muted:#a29c95}
.preview-light{--preview-bg:#faf9f5;--preview-card:#f0eee8;--preview-border:#e3e1db;--preview-muted:#a29c95}
.preview-dark{--preview-bg:#151412;--preview-card:#1d1b18;--preview-border:#3a3530;--preview-muted:#9c958d}
.theme-card-label{font-size:11px;font-weight:500;white-space:nowrap}
.feedback{display:grid;grid-template-columns:minmax(0,1fr);gap:6px;min-height:39px;margin-bottom:10px}
.status{display:flex;align-items:center;gap:8px;min-height:20px;color:var(--text-secondary);font-size:12px}
.status::before{content:"";width:6px;height:6px;flex:none;border-radius:50%;background:var(--text-tertiary)}
.error{min-height:0;color:var(--error-color);font-size:13px}
.error:empty{display:none}
.error:not(:empty){padding:9px 12px;border:1px solid var(--error-border);border-radius:8px;background:var(--error-soft)}
.cards{display:grid;grid-template-columns:repeat(6,minmax(145px,1fr));gap:12px;margin-bottom:14px}
.card,.panel{border:1px solid var(--border-color);border-radius:12px;background:var(--bg-primary);box-shadow:var(--shadow)}
.card{position:relative;min-height:122px;padding:17px 18px;overflow:hidden}
.card::after{content:"";position:absolute;right:-28px;bottom:-34px;width:86px;height:86px;border:1px solid color-mix(in srgb,var(--primary-color) 15%,transparent);border-radius:50%}
.card:hover{border-color:var(--border-hover);box-shadow:0 5px 15px rgb(0 0 0/.055);transform:translateY(-1px)}
.card .label{display:flex;align-items:center;gap:7px;color:var(--text-secondary);font-size:12px;font-weight:600}
.card .label::before{content:"";width:6px;height:6px;border-radius:2px;background:var(--primary-color);opacity:.72}
.card .value{position:relative;z-index:1;margin-top:10px;color:var(--text-primary);font-size:clamp(22px,2vw,29px);font-weight:750;letter-spacing:-.035em;line-height:1.1;font-variant-numeric:tabular-nums}
.card .detail{position:relative;z-index:1;margin-top:7px;color:var(--text-tertiary);font-size:11px}
.panel{overflow:hidden}
.chart-panel{margin-bottom:14px;padding:0 18px 13px}
.panel-head{display:flex;align-items:center;justify-content:space-between;gap:18px;min-height:54px;padding:13px 0}
.table-panel .panel-head{padding:13px 16px;border-bottom:1px solid var(--border-color)}
.panel-title{margin:0;color:var(--text-primary);font-size:14px;font-weight:700}
.panel-meta{color:var(--text-tertiary);font-size:12px}
.chart-wrap{position:relative;border-top:1px solid var(--border-color)}
.chart-wrap svg{display:block;width:100%;height:205px;overflow:visible}
.chart-grid{stroke:var(--border-color);stroke-width:1;stroke-dasharray:3 5}
.axis{stroke:var(--border-primary);stroke-width:1}
.chart-line{fill:none;stroke:var(--primary-active);stroke-width:2.5;stroke-linecap:round;stroke-linejoin:round;vector-effect:non-scaling-stroke}
.chart-area{fill:var(--chart-area)}
.chart-dot{fill:var(--bg-primary);stroke:var(--primary-active);stroke-width:2;vector-effect:non-scaling-stroke}
.chart-empty{fill:var(--text-tertiary);font:13px -apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei",sans-serif;text-anchor:middle}
.table-wrap{max-height:540px;overflow:auto;scrollbar-gutter:stable}
table{width:100%;min-width:1260px;border-collapse:collapse}
th,td{padding:11px 12px;border-bottom:1px solid var(--border-color);text-align:left;white-space:nowrap}
th{position:sticky;top:0;z-index:2;background:var(--bg-quinary);color:var(--text-secondary);font-size:11px;font-weight:700;letter-spacing:.025em}
td{color:var(--text-primary);font-size:12px}
td.num,th.num{text-align:right;font-variant-numeric:tabular-nums}
tbody tr:last-child td{border-bottom:0}
tbody tr:hover td{background:var(--row-hover)}
.fail{color:var(--error-color);font-weight:650}
.empty{text-align:center!important;color:var(--text-tertiary)!important;padding:48px!important}
dialog{width:min(440px,calc(100vw - 28px));padding:0;border:1px solid var(--border-color);border-radius:12px;background:var(--bg-primary);color:var(--text-primary);box-shadow:0 24px 70px rgb(0 0 0/.32)}
dialog::backdrop{background:rgb(0 0 0/.56);backdrop-filter:blur(2px)}
dialog form{padding:22px}
dialog h2{margin:0 0 7px;font-size:18px}
dialog p{margin:0;color:var(--text-secondary);font-size:13px}
dialog input{width:100%;margin:17px 0;padding:10px 12px;border:1px solid var(--border-color);border-radius:8px;background:var(--bg-secondary);color:var(--text-primary)}
.dialog-actions{display:flex;justify-content:flex-end;gap:8px}
::-webkit-scrollbar{width:8px;height:8px}
::-webkit-scrollbar-track{background:var(--bg-secondary)}
::-webkit-scrollbar-thumb{border-radius:999px;background:var(--border-color)}
::-webkit-scrollbar-thumb:hover{background:var(--border-hover)}
@media(max-width:1240px){.cards{grid-template-columns:repeat(3,minmax(160px,1fr))}}
@media(max-width:820px){.top{flex-direction:column}.controls{width:100%;justify-content:flex-start}.theme-menu{margin-left:auto}.cards{grid-template-columns:repeat(2,minmax(145px,1fr))}}
@media(max-width:560px){.shell{padding:18px 12px 28px}.top{gap:16px}.controls{display:grid;grid-template-columns:minmax(0,1fr) auto auto auto}.controls select{min-width:0;width:100%}.button-label{display:none}.cards{gap:8px}.card{min-height:108px;padding:14px}.card .value{font-size:21px}.chart-panel{padding:0 12px 9px}.theme-popover{right:0;display:grid;grid-template-columns:repeat(2,minmax(0,1fr))}.theme-card{width:100%}}
@media(prefers-reduced-motion:reduce){*,*::before,*::after{scroll-behavior:auto!important;transition:none!important;animation:none!important}}
@media(max-width:768px),(prefers-reduced-transparency:reduce){.theme-popover{background:var(--bg-primary);backdrop-filter:none;-webkit-backdrop-filter:none}}
</style>
</head>
<body>
<div class="shell">
<main id="dashboard">
<header class="top">
<div class="heading">
<div class="eyebrow"><span class="eyebrow-mark"></span>CLIProxyAPI Usage Plugin</div>
<h1>Token 用量统计</h1>
<div class="subtitle">持久化小时聚合 · 打开页面无需密钥 · 重置数据时需要 Management Key</div>
</div>
<div class="controls">
<select id="range" class="control" aria-label="统计范围"><option value="24h">最近 24 小时</option><option value="7d">最近 7 天</option><option value="30d">最近 30 天</option><option value="retention">全部保留数据</option></select>
<button id="refreshButton" class="control primary" type="button"><svg class="button-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 11a8.1 8.1 0 0 0-15.5-2M4 4v5h5"></path><path d="M4 13a8.1 8.1 0 0 0 15.5 2M20 20v-5h-5"></path></svg><span class="button-label">刷新</span></button>
<button id="resetButton" class="control danger" type="button"><svg class="button-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18"></path><path d="M8 6V4h8v2"></path><path d="M19 6l-1 14H6L5 6"></path><path d="M10 11v5M14 11v5"></path></svg><span class="button-label">重置数据</span></button>
<div class="theme-menu">
<button id="themeButton" class="control icon-button" type="button" aria-haspopup="menu" aria-expanded="false" aria-controls="themePopover" aria-label="主题：跟随系统" title="主题：跟随系统">
<span class="theme-icon" data-theme-icon="auto"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="13" rx="2"></rect><path d="M8 21h8M12 17v4"></path><path d="M8.5 9.5a3.5 3.5 0 0 0 5 3.1A4 4 0 1 1 8.5 9.5Z"></path></svg></span>
<span class="theme-icon" data-theme-icon="white" hidden><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M17.66 6.34l1.41-1.41"></path></svg></span>
<span class="theme-icon" data-theme-icon="light" hidden><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5.5h16v13H4z"></path><path d="M7 9h10M7 12h7M7 15h9"></path></svg></span>
<span class="theme-icon" data-theme-icon="dark" hidden><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.5 14.2A8.5 8.5 0 0 1 9.8 3.5 8.5 8.5 0 1 0 20.5 14.2Z"></path></svg></span>
</button>
<div id="themePopover" class="theme-popover" role="menu" aria-label="切换主题" hidden>
<button class="theme-card" type="button" data-theme-value="auto" role="menuitemradio" aria-checked="false"><span class="theme-preview preview-auto"><span class="theme-preview-head"></span><span class="theme-preview-body"><span class="theme-preview-side"></span><span class="theme-preview-content"><span class="theme-preview-line"></span><span class="theme-preview-line short"></span></span></span></span><span class="theme-card-label">跟随系统</span></button>
<button class="theme-card" type="button" data-theme-value="white" role="menuitemradio" aria-checked="false"><span class="theme-preview preview-white"><span class="theme-preview-head"></span><span class="theme-preview-body"><span class="theme-preview-side"></span><span class="theme-preview-content"><span class="theme-preview-line"></span><span class="theme-preview-line short"></span></span></span></span><span class="theme-card-label">纯白</span></button>
<button class="theme-card" type="button" data-theme-value="light" role="menuitemradio" aria-checked="false"><span class="theme-preview preview-light"><span class="theme-preview-head"></span><span class="theme-preview-body"><span class="theme-preview-side"></span><span class="theme-preview-content"><span class="theme-preview-line"></span><span class="theme-preview-line short"></span></span></span></span><span class="theme-card-label">羊毛纸</span></button>
<button class="theme-card" type="button" data-theme-value="dark" role="menuitemradio" aria-checked="false"><span class="theme-preview preview-dark"><span class="theme-preview-head"></span><span class="theme-preview-body"><span class="theme-preview-side"></span><span class="theme-preview-content"><span class="theme-preview-line"></span><span class="theme-preview-line short"></span></span></span></span><span class="theme-card-label">暗色</span></button>
</div>
</div>
</div>
</header>
<div class="feedback"><div id="status" class="status" aria-live="polite">正在加载统计数据…</div><div id="error" class="error" role="alert"></div></div>
<section class="cards" aria-label="统计概览">
<div class="card"><div class="label">总 Token</div><div id="totalTokens" class="value">0</div><div class="detail">所选范围内累计</div></div>
<div class="card"><div class="label">请求数</div><div id="requests" class="value">0</div><div id="successRate" class="detail">成功率 —</div></div>
<div class="card"><div class="label">输入 Token</div><div id="inputTokens" class="value">0</div><div class="detail">Prompt 用量</div></div>
<div class="card"><div class="label">输出 Token</div><div id="outputTokens" class="value">0</div><div class="detail">Completion 用量</div></div>
<div class="card"><div class="label">推理 Token</div><div id="reasoningTokens" class="value">0</div><div class="detail">Reasoning 用量</div></div>
<div class="card"><div class="label">缓存读取 / 创建</div><div id="cacheTokens" class="value">0 / 0</div><div class="detail">Cache Token 用量</div></div>
</section>
<section class="panel chart-panel"><div class="panel-head"><h2 class="panel-title">每小时 Token 趋势</h2><span class="panel-meta">按小时聚合</span></div><div class="chart-wrap"><svg id="chart" viewBox="0 0 1000 205" preserveAspectRatio="none" role="img" aria-label="每小时 Token 趋势"></svg></div></section>
<section class="panel table-panel"><div class="panel-head"><h2 class="panel-title">维度明细</h2><span id="groupCount" class="panel-meta"></span></div><div class="table-wrap"><table><thead><tr><th>模型</th><th>提供商</th><th>别名</th><th>来源</th><th>执行器</th><th>认证类型</th><th>服务层级</th><th>推理强度</th><th class="num">请求</th><th class="num">失败</th><th class="num">输入</th><th class="num">输出</th><th class="num">推理</th><th class="num">缓存读取</th><th class="num">缓存创建</th><th class="num">总 Token</th><th class="num">平均延迟</th><th class="num">平均 TTFT</th></tr></thead><tbody id="groups"></tbody></table></div></section>
</main>
<dialog id="resetDialog"><form method="dialog"><h2>管理鉴权</h2><p>Management Key 仅用于本次重置，不会保存。</p><input id="resetKeyInput" type="password" autocomplete="current-password" placeholder="Management Key" required><div class="dialog-actions"><button class="control" value="cancel" formnovalidate>取消</button><button class="control danger" value="confirm">确认重置</button></div></form></dialog>
</div>
<script>
(function(){
'use strict';
var refreshTimer=0;
var activeController=null;
var pluginID=readPluginID();
var statsURL='/v0/resource/plugins/'+encodeURIComponent(pluginID)+'/stats';
var resetURL='/v0/management/plugins/'+encodeURIComponent(pluginID)+'/reset';
var resetDialog=document.getElementById('resetDialog');
var resetKeyInput=document.getElementById('resetKeyInput');
var themeButton=document.getElementById('themeButton');
var themePopover=document.getElementById('themePopover');
var themeNames={auto:'跟随系统',white:'纯白',light:'羊毛纸',dark:'暗色'};
var themeMode=document.documentElement.getAttribute('data-theme-mode')||'auto';
var systemTheme=window.matchMedia?window.matchMedia('(prefers-color-scheme: dark)'):null;

function readPluginID(){var parts=window.location.pathname.split('/').filter(Boolean);var index=parts.indexOf('plugins');if(index<0||!parts[index+1])return '';return decodeURIComponent(parts[index+1]);}
function text(id,value){var node=document.getElementById(id);if(node)node.textContent=value;}
function fmt(value){return new Intl.NumberFormat('zh-CN').format(Number(value||0));}
function duration(ns){ns=Number(ns||0);if(!ns)return '—';if(ns<1e6)return Math.round(ns/1e3)+'µs';if(ns<1e9)return (ns/1e6).toFixed(1)+'ms';return (ns/1e9).toFixed(2)+'s';}
function clearErrors(){text('error','');}
function resolvedTheme(mode){if(mode==='auto')return systemTheme&&systemTheme.matches?'dark':'white';return mode;}
function applyTheme(mode,persist){var applied=resolvedTheme(mode);themeMode=mode;document.documentElement.setAttribute('data-theme-mode',mode);if(applied==='light')document.documentElement.removeAttribute('data-theme');else document.documentElement.setAttribute('data-theme',applied);if(persist){try{window.localStorage.setItem('cap-token-usage-theme',mode);}catch(_error){}}document.querySelectorAll('[data-theme-value]').forEach(function(card){var active=card.getAttribute('data-theme-value')===mode;card.classList.toggle('active',active);card.setAttribute('aria-checked',active?'true':'false');});document.querySelectorAll('[data-theme-icon]').forEach(function(icon){icon.hidden=icon.getAttribute('data-theme-icon')!==mode;});var label='主题：'+themeNames[mode];themeButton.setAttribute('aria-label',label);themeButton.setAttribute('title',label);}
function closeThemeMenu(){themePopover.hidden=true;themeButton.setAttribute('aria-expanded','false');}
function toggleThemeMenu(){var nextOpen=themePopover.hidden;themePopover.hidden=!nextOpen;themeButton.setAttribute('aria-expanded',nextOpen?'true':'false');}
async function api(url,options,trackActive){trackActive=trackActive!==false;if(trackActive&&activeController)activeController.abort();var controller=new AbortController();if(trackActive)activeController=controller;var timeout=setTimeout(function(){controller.abort();},10000);options=options||{};options.signal=controller.signal;try{var response=await fetch(new URL(url,window.location.origin),options);var body;try{body=await response.json();}catch(_e){body={error:'响应不是有效 JSON'};}if(!response.ok)throw new Error(body.error||('HTTP '+response.status));return body;}catch(error){if(error.name==='AbortError')throw new Error('请求超时或已被新请求取消');throw error;}finally{clearTimeout(timeout);if(trackActive&&activeController===controller)activeController=null;}}
async function load(){clearErrors();var range=document.getElementById('range').value;var data=await api(statsURL+'?range='+encodeURIComponent(range));render(data);}
function render(data){var s=data.summary||{};text('totalTokens',fmt(s.total_tokens));text('requests',fmt(s.requests));text('inputTokens',fmt(s.input_tokens));text('outputTokens',fmt(s.output_tokens));text('reasoningTokens',fmt(s.reasoning_tokens));text('cacheTokens',fmt(s.cache_read_tokens)+' / '+fmt(s.cache_creation_tokens));var success=Number(s.requests||0)?((Number(s.requests)-Number(s.failed_requests||0))/Number(s.requests)*100):0;text('successRate','成功率 '+success.toFixed(1)+'%');text('status','范围：'+data.range+' · 最近活动：'+(data.last_used&&data.last_used.indexOf('0001-')!==0?new Date(data.last_used).toLocaleString():'—')+' · 更新：'+new Date(data.generated_at).toLocaleTimeString());renderGroups(data.groups||[]);renderChart(data.series||[]);}
function cell(row,value,cls){var td=document.createElement('td');td.textContent=value===undefined||value===null||value===''?'—':String(value);if(cls)td.className=cls;row.appendChild(td);}
function renderGroups(groups){var body=document.getElementById('groups');var fragment=document.createDocumentFragment();if(!groups.length){var emptyRow=document.createElement('tr');var empty=document.createElement('td');empty.colSpan=18;empty.className='empty';empty.textContent='当前范围暂无用量数据';emptyRow.appendChild(empty);fragment.appendChild(emptyRow);}else{groups.forEach(function(g){var row=document.createElement('tr');cell(row,g.model);cell(row,g.provider);cell(row,g.alias);cell(row,g.source);cell(row,g.executor_type);cell(row,g.auth_type);cell(row,g.service_tier);cell(row,g.reasoning_effort);cell(row,fmt(g.requests),'num');cell(row,fmt(g.failed_requests),'num'+(Number(g.failed_requests)>0?' fail':''));cell(row,fmt(g.input_tokens),'num');cell(row,fmt(g.output_tokens),'num');cell(row,fmt(g.reasoning_tokens),'num');cell(row,fmt(g.cache_read_tokens),'num');cell(row,fmt(g.cache_creation_tokens),'num');cell(row,fmt(g.total_tokens),'num');cell(row,duration(g.average_latency_ns),'num');cell(row,duration(g.average_ttft_ns),'num');fragment.appendChild(row);});}body.replaceChildren(fragment);text('groupCount',groups.length+' 个分组');}
function svgNode(name,attrs){var node=document.createElementNS('http://www.w3.org/2000/svg',name);Object.keys(attrs||{}).forEach(function(key){node.setAttribute(key,String(attrs[key]));});return node;}
function renderChart(series){var svg=document.getElementById('chart');var fragment=document.createDocumentFragment();[40,85,130,175].forEach(function(y){fragment.appendChild(svgNode('line',{x1:0,y1:y,x2:1000,y2:y,class:'chart-grid'}));});fragment.appendChild(svgNode('line',{x1:0,y1:185,x2:1000,y2:185,class:'axis'}));if(series.length){var sampled=series;if(series.length>240){var step=Math.ceil(series.length/240);sampled=series.filter(function(_point,index){return index%step===0||index===series.length-1;});}var max=sampled.reduce(function(value,p){return Math.max(value,Number(p.total_tokens||0));},1);var points=sampled.map(function(p,index){var x=sampled.length===1?500:index/(sampled.length-1)*1000;var y=180-Number(p.total_tokens||0)/max*155;return [x,y];});var area='M '+points[0][0]+' 185 L '+points.map(function(p){return p[0]+' '+p[1];}).join(' L ')+' L '+points[points.length-1][0]+' 185 Z';fragment.appendChild(svgNode('path',{d:area,class:'chart-area'}));fragment.appendChild(svgNode('polyline',{points:points.map(function(p){return p.join(',');}).join(' '),class:'chart-line'}));points.forEach(function(p,index){if(sampled.length<=48||index%Math.ceil(sampled.length/24)===0||index===sampled.length-1)fragment.appendChild(svgNode('circle',{cx:p[0],cy:p[1],r:3,class:'chart-dot'}));});}else{var empty=svgNode('text',{x:500,y:106,class:'chart-empty'});empty.textContent='当前范围暂无趋势数据';fragment.appendChild(empty);}svg.replaceChildren(fragment);}
function askManagementKey(){resetKeyInput.value='';resetDialog.returnValue='';resetDialog.showModal();resetKeyInput.focus();return new Promise(function(resolve){resetDialog.addEventListener('close',function(){var key=resetDialog.returnValue==='confirm'?resetKeyInput.value.trim():'';resetKeyInput.value='';resolve(key);},{once:true});});}
async function resetStats(){if(!confirm('确定永久删除全部 Token 统计吗？此操作不可撤销。'))return;var typed=prompt('请输入 reset 确认：');if(typed!=='reset')return;var managementKey=await askManagementKey();if(!managementKey)return;try{await api(resetURL,{method:'POST',headers:{'Content-Type':'application/json','Authorization':'Bearer '+managementKey},body:JSON.stringify({confirm:'reset'})},false);await load();}catch(error){text('error',error.message);}}
function startTimer(){if(refreshTimer)clearInterval(refreshTimer);refreshTimer=setInterval(function(){load().catch(function(error){text('error',error.message);});},15000);}

themeButton.addEventListener('click',toggleThemeMenu);
themePopover.querySelectorAll('[data-theme-value]').forEach(function(card){card.addEventListener('click',function(){applyTheme(card.getAttribute('data-theme-value'),true);closeThemeMenu();});});
document.addEventListener('click',function(event){if(!themeButton.contains(event.target)&&!themePopover.contains(event.target))closeThemeMenu();});
document.addEventListener('keydown',function(event){if(event.key==='Escape')closeThemeMenu();});
if(systemTheme){var onSystemThemeChange=function(){if(themeMode==='auto')applyTheme('auto',false);};if(systemTheme.addEventListener)systemTheme.addEventListener('change',onSystemThemeChange);else if(systemTheme.addListener)systemTheme.addListener(onSystemThemeChange);}
document.getElementById('refreshButton').addEventListener('click',function(){load().catch(function(error){text('error',error.message);});});
document.getElementById('range').addEventListener('change',function(){load().catch(function(error){text('error',error.message);});});
document.getElementById('resetButton').addEventListener('click',resetStats);
applyTheme(themeMode,false);
if(!pluginID){text('error','无法从插件资源 URL 识别 plugin ID。');return;}load().catch(function(error){text('error',error.message);});startTimer();
})();
</script>
</body>
</html>`
