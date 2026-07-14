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
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Token 用量统计</title>
<style>
:root{color-scheme:dark;--bg:#0b1020;--panel:#131a2d;--panel2:#19233a;--line:#2a3654;--text:#edf2ff;--muted:#91a0bd;--accent:#6ea8fe;--good:#42d392;--bad:#ff6b7a;--warn:#f5c451}*{box-sizing:border-box}body{margin:0;background:linear-gradient(145deg,#0b1020,#111a2d);color:var(--text);font:14px/1.45 system-ui,-apple-system,"Segoe UI","Microsoft YaHei",sans-serif;min-height:100vh}.shell{max-width:1500px;margin:auto;padding:22px}.top{display:flex;gap:16px;align-items:center;justify-content:space-between;flex-wrap:wrap;margin-bottom:18px}h1{font-size:23px;margin:0}.subtitle{color:var(--muted);margin-top:3px}.controls{display:flex;gap:9px;align-items:center;flex-wrap:wrap}button,select,input{font:inherit}button,select{color:var(--text);background:var(--panel2);border:1px solid var(--line);border-radius:8px;padding:8px 12px}button{cursor:pointer}button:hover{border-color:var(--accent)}button.danger{color:#ffd7dc;border-color:#783846}.status{color:var(--muted);font-size:12px}.error{color:#ffabb4;min-height:20px;margin:8px 0}.login{max-width:520px;margin:70px auto;background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:26px;box-shadow:0 18px 55px #0006}.login h2{margin-top:0}.login input[type=password]{width:100%;padding:11px;border:1px solid var(--line);border-radius:8px;background:#0d1425;color:var(--text);margin:10px 0}.login label{display:flex;gap:8px;align-items:center;color:var(--muted);margin:4px 0 16px}.hidden{display:none!important}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(175px,1fr));gap:12px;margin-bottom:14px}.card,.chart,.table-box{background:var(--panel);border:1px solid var(--line);border-radius:12px}.card{padding:15px}.card .label{color:var(--muted);font-size:12px}.card .value{font-size:25px;font-weight:700;margin-top:5px}.card .detail{color:var(--muted);font-size:11px;margin-top:2px}.grid{display:grid;grid-template-columns:minmax(330px,1fr);gap:14px}.chart{padding:14px;margin-bottom:14px}.chart h2,.table-head h2{font-size:14px;margin:0}.chart svg{width:100%;height:190px;margin-top:10px;overflow:visible}.chart-line{fill:none;stroke:var(--accent);stroke-width:3}.chart-area{fill:#6ea8fe22}.chart-dot{fill:var(--accent)}.axis{stroke:var(--line);stroke-width:1}.table-box{overflow:hidden}.table-head{display:flex;align-items:center;justify-content:space-between;padding:14px 16px;border-bottom:1px solid var(--line)}.table-wrap{overflow:auto;max-height:520px}table{border-collapse:collapse;width:100%;min-width:1260px}th,td{padding:10px 12px;border-bottom:1px solid var(--line);text-align:left;white-space:nowrap}th{position:sticky;top:0;background:var(--panel2);color:var(--muted);font-size:11px;text-transform:uppercase}td.num,th.num{text-align:right;font-variant-numeric:tabular-nums}tr:hover td{background:#ffffff05}.fail{color:var(--bad)}.empty{text-align:center!important;color:var(--muted);padding:38px!important}@media(max-width:700px){.shell{padding:14px}.controls{width:100%}.controls>*{flex:1}.login{margin:30px auto}}
</style>
</head>
<body>
<div class="shell">
<section id="login" class="login">
<h2>连接管理 API</h2>
<p class="subtitle">CLIProxyAPI 不会把管理密钥传入插件 iframe。密钥仅用于当前页面请求，不会写入插件数据库。</p>
<input id="keyInput" type="password" autocomplete="current-password" placeholder="Management Key">
<label><input id="rememberKey" type="checkbox">仅在当前标签页记住</label>
<button id="connectButton" type="button">连接并加载统计</button>
<div id="loginError" class="error" role="alert"></div>
</section>

<main id="dashboard" class="hidden">
<header class="top"><div><h1>Token 用量统计</h1><div class="subtitle">持久化小时聚合 · 不保存 API Key、Auth ID 或失败响应正文</div></div><div class="controls"><select id="range"><option value="24h">最近 24 小时</option><option value="7d">最近 7 天</option><option value="30d">最近 30 天</option><option value="retention">全部保留数据</option></select><button id="refreshButton" type="button">刷新</button><button id="logoutButton" type="button">清除密钥</button><button id="resetButton" class="danger" type="button">重置数据</button></div></header>
<div id="status" class="status"></div><div id="error" class="error" role="alert"></div>
<section class="cards">
<div class="card"><div class="label">总 Token</div><div id="totalTokens" class="value">0</div></div>
<div class="card"><div class="label">请求数</div><div id="requests" class="value">0</div><div id="successRate" class="detail">成功率 —</div></div>
<div class="card"><div class="label">输入 Token</div><div id="inputTokens" class="value">0</div></div>
<div class="card"><div class="label">输出 Token</div><div id="outputTokens" class="value">0</div></div>
<div class="card"><div class="label">推理 Token</div><div id="reasoningTokens" class="value">0</div></div>
<div class="card"><div class="label">缓存读取 / 创建</div><div id="cacheTokens" class="value">0 / 0</div></div>
</section>
<section class="chart"><h2>每小时 Token 趋势</h2><svg id="chart" viewBox="0 0 1000 190" preserveAspectRatio="none" aria-label="每小时 Token 趋势"></svg></section>
<section class="table-box"><div class="table-head"><h2>维度明细</h2><span id="groupCount" class="status"></span></div><div class="table-wrap"><table><thead><tr><th>模型</th><th>提供商</th><th>别名</th><th>来源</th><th>执行器</th><th>认证类型</th><th>服务层级</th><th>推理强度</th><th class="num">请求</th><th class="num">失败</th><th class="num">输入</th><th class="num">输出</th><th class="num">推理</th><th class="num">缓存读取</th><th class="num">缓存创建</th><th class="num">总 Token</th><th class="num">平均延迟</th><th class="num">平均 TTFT</th></tr></thead><tbody id="groups"></tbody></table></div></section>
</main>
</div>
<script>
(function(){
'use strict';
var managementKey='';
var refreshTimer=0;
var activeController=null;
var pluginID=readPluginID();
var storageKey='cap-token-usage-key:'+pluginID;
var statsURL='/v0/management/plugins/'+encodeURIComponent(pluginID)+'/stats';
var resetURL='/v0/management/plugins/'+encodeURIComponent(pluginID)+'/reset';
var login=document.getElementById('login');
var dashboard=document.getElementById('dashboard');
var keyInput=document.getElementById('keyInput');
var rememberKey=document.getElementById('rememberKey');

function readPluginID(){var parts=window.location.pathname.split('/').filter(Boolean);var index=parts.indexOf('plugins');if(index<0||!parts[index+1])return '';return decodeURIComponent(parts[index+1]);}
function text(id,value){var node=document.getElementById(id);if(node)node.textContent=value;}
function fmt(value){return new Intl.NumberFormat('zh-CN').format(Number(value||0));}
function duration(ns){ns=Number(ns||0);if(!ns)return '—';if(ns<1e6)return Math.round(ns/1e3)+'µs';if(ns<1e9)return (ns/1e6).toFixed(1)+'ms';return (ns/1e9).toFixed(2)+'s';}
function authHeaders(extra){var headers=extra||{};headers.Authorization='Bearer '+managementKey;return headers;}
function clearErrors(){text('error','');text('loginError','');}
function stopRequests(){if(activeController){activeController.abort();activeController=null;}if(refreshTimer){clearInterval(refreshTimer);refreshTimer=0;}}
function showLogin(message){stopRequests();managementKey='';keyInput.value='';rememberKey.checked=false;try{sessionStorage.removeItem(storageKey);}catch(_e){}dashboard.classList.add('hidden');login.classList.remove('hidden');if(message)text('loginError',message);keyInput.focus();}
async function api(url,options){if(activeController)activeController.abort();var controller=new AbortController();activeController=controller;var timeout=setTimeout(function(){controller.abort();},10000);options=options||{};options.headers=authHeaders(options.headers);options.signal=controller.signal;try{var response=await fetch(new URL(url,window.location.origin),options);if(response.status===401||response.status===403){showLogin('管理密钥无效或已失效。');throw new Error('unauthorized');}var body;try{body=await response.json();}catch(_e){body={error:'响应不是有效 JSON'};}if(!response.ok)throw new Error(body.error||('HTTP '+response.status));return body;}catch(error){if(error.name==='AbortError')throw new Error('请求超时或已被新请求取消');throw error;}finally{clearTimeout(timeout);if(activeController===controller)activeController=null;}}
async function connect(){clearErrors();managementKey=keyInput.value.trim();if(!managementKey){text('loginError','请输入管理密钥。');return;}try{if(rememberKey.checked){sessionStorage.setItem(storageKey,managementKey);}else{sessionStorage.removeItem(storageKey);}}catch(_e){}try{await load();login.classList.add('hidden');dashboard.classList.remove('hidden');startTimer();}catch(error){if(error.message!=='unauthorized')text('loginError',error.message);}}
async function load(){clearErrors();var range=document.getElementById('range').value;var data=await api(statsURL+'?range='+encodeURIComponent(range));render(data);}
function render(data){var s=data.summary||{};text('totalTokens',fmt(s.total_tokens));text('requests',fmt(s.requests));text('inputTokens',fmt(s.input_tokens));text('outputTokens',fmt(s.output_tokens));text('reasoningTokens',fmt(s.reasoning_tokens));text('cacheTokens',fmt(s.cache_read_tokens)+' / '+fmt(s.cache_creation_tokens));var success=Number(s.requests||0)?((Number(s.requests)-Number(s.failed_requests||0))/Number(s.requests)*100):0;text('successRate','成功率 '+success.toFixed(1)+'%');text('status','范围：'+data.range+' · 最近活动：'+(data.last_used&&data.last_used.indexOf('0001-')!==0?new Date(data.last_used).toLocaleString():'—')+' · 更新：'+new Date(data.generated_at).toLocaleTimeString());renderGroups(data.groups||[]);renderChart(data.series||[]);}
function cell(row,value,cls){var td=document.createElement('td');td.textContent=value===undefined||value===null||value===''?'—':String(value);if(cls)td.className=cls;row.appendChild(td);}
function renderGroups(groups){var body=document.getElementById('groups');var fragment=document.createDocumentFragment();if(!groups.length){var emptyRow=document.createElement('tr');var empty=document.createElement('td');empty.colSpan=18;empty.className='empty';empty.textContent='当前范围暂无用量数据';emptyRow.appendChild(empty);fragment.appendChild(emptyRow);}else{groups.forEach(function(g){var row=document.createElement('tr');cell(row,g.model);cell(row,g.provider);cell(row,g.alias);cell(row,g.source);cell(row,g.executor_type);cell(row,g.auth_type);cell(row,g.service_tier);cell(row,g.reasoning_effort);cell(row,fmt(g.requests),'num');cell(row,fmt(g.failed_requests),'num'+(Number(g.failed_requests)>0?' fail':''));cell(row,fmt(g.input_tokens),'num');cell(row,fmt(g.output_tokens),'num');cell(row,fmt(g.reasoning_tokens),'num');cell(row,fmt(g.cache_read_tokens),'num');cell(row,fmt(g.cache_creation_tokens),'num');cell(row,fmt(g.total_tokens),'num');cell(row,duration(g.average_latency_ns),'num');cell(row,duration(g.average_ttft_ns),'num');fragment.appendChild(row);});}body.replaceChildren(fragment);text('groupCount',groups.length+' 个分组');}
function svgNode(name,attrs){var node=document.createElementNS('http://www.w3.org/2000/svg',name);Object.keys(attrs||{}).forEach(function(key){node.setAttribute(key,String(attrs[key]));});return node;}
function renderChart(series){var svg=document.getElementById('chart');var fragment=document.createDocumentFragment();fragment.appendChild(svgNode('line',{x1:0,y1:175,x2:1000,y2:175,class:'axis'}));if(series.length){var sampled=series;if(series.length>240){var step=Math.ceil(series.length/240);sampled=series.filter(function(_point,index){return index%step===0||index===series.length-1;});}var max=sampled.reduce(function(value,p){return Math.max(value,Number(p.total_tokens||0));},1);var points=sampled.map(function(p,index){var x=sampled.length===1?500:index/(sampled.length-1)*1000;var y=170-Number(p.total_tokens||0)/max*155;return [x,y];});var area='M '+points[0][0]+' 175 L '+points.map(function(p){return p[0]+' '+p[1];}).join(' L ')+' L '+points[points.length-1][0]+' 175 Z';fragment.appendChild(svgNode('path',{d:area,class:'chart-area'}));fragment.appendChild(svgNode('polyline',{points:points.map(function(p){return p.join(',');}).join(' '),class:'chart-line'}));points.forEach(function(p){fragment.appendChild(svgNode('circle',{cx:p[0],cy:p[1],r:3,class:'chart-dot'}));});}svg.replaceChildren(fragment);}
async function resetStats(){if(!confirm('确定永久删除全部 Token 统计吗？此操作不可撤销。'))return;var typed=prompt('请输入 reset 确认：');if(typed!=='reset')return;try{await api(resetURL,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({confirm:'reset'})});await load();}catch(error){if(error.message!=='unauthorized')text('error',error.message);}}
function startTimer(){if(refreshTimer)clearInterval(refreshTimer);refreshTimer=setInterval(function(){load().catch(function(error){if(error.message!=='unauthorized')text('error',error.message);});},15000);}
document.getElementById('connectButton').addEventListener('click',connect);keyInput.addEventListener('keydown',function(event){if(event.key==='Enter')connect();});document.getElementById('refreshButton').addEventListener('click',function(){load().catch(function(error){if(error.message!=='unauthorized')text('error',error.message);});});document.getElementById('range').addEventListener('change',function(){load().catch(function(error){if(error.message!=='unauthorized')text('error',error.message);});});document.getElementById('resetButton').addEventListener('click',resetStats);document.getElementById('logoutButton').addEventListener('click',function(){showLogin('管理密钥已清除。');});
if(!pluginID){text('loginError','无法从插件资源 URL 识别 plugin ID。');return;}try{var saved=sessionStorage.getItem(storageKey);if(saved){managementKey=saved;rememberKey.checked=true;keyInput.value=saved;connect();}}catch(_e){}
})();
</script>
</body>
</html>`
