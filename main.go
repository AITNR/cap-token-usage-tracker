package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const abiVersion uint32 = 1
const schemaVersion uint32 = 1

// ─── C ABI exports ──────────────────────────────────────────────────────────

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

// ─── Types ──────────────────────────────────────────────────────────────────

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type managementRequest struct {
	Method  string          `json:"Method"`
	Path    string          `json:"Path"`
	Headers json.RawMessage `json:"Headers"`
	Query   json.RawMessage `json:"Query"`
	Body    json.RawMessage `json:"Body"`
}

// ─── Usage record (matches host UsageRecord JSON shape) ─────────────────────

type usageRecord struct {
	Provider string        `json:"Provider"`
	Model    string        `json:"Model"`
	Failed   bool          `json:"Failed"`
	Latency  time.Duration `json:"Latency"`
	Detail   usageDetail   `json:"Detail"`
}

type usageDetail struct {
	InputTokens         int64 `json:"InputTokens"`
	OutputTokens        int64 `json:"OutputTokens"`
	ReasoningTokens     int64 `json:"ReasoningTokens"`
	CachedTokens        int64 `json:"CachedTokens"`
	CacheReadTokens     int64 `json:"CacheReadTokens"`
	CacheCreationTokens int64 `json:"CacheCreationTokens"`
	TotalTokens         int64 `json:"TotalTokens"`
}

// ─── Stats aggregation ──────────────────────────────────────────────────────

type statsSnapshot struct {
	TotalRequests       int64                `json:"total_requests"`
	TotalFailed         int64                `json:"total_failed"`
	InputTokens         int64                `json:"input_tokens"`
	OutputTokens        int64                `json:"output_tokens"`
	ReasoningTokens     int64                `json:"reasoning_tokens"`
	CachedTokens        int64                `json:"cached_tokens"`
	TotalTokens         int64                `json:"total_tokens"`
	CacheReadTokens     int64                `json:"cache_read_tokens"`
	CacheCreationTokens int64                `json:"cache_creation_tokens"`
	Models              map[string]*dimStats `json:"models"`
	Providers           map[string]*dimStats `json:"providers"`
}

type dimStats struct {
	Requests            int64   `json:"requests"`
	Failed              int64   `json:"failed"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	ReasoningTokens     int64   `json:"reasoning_tokens"`
	CachedTokens        int64   `json:"cached_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	TotalLatencyMs      float64 `json:"total_latency_ms"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
}

var (
	globalStats statsSnapshot
	statsMu     sync.RWMutex
)

func init() {
	globalStats.Models = make(map[string]*dimStats)
	globalStats.Providers = make(map[string]*dimStats)
}

func recordUsage(rec usageRecord) {
	statsMu.Lock()
	defer statsMu.Unlock()
	globalStats.TotalRequests++
	if rec.Failed {
		globalStats.TotalFailed++
	}
	globalStats.InputTokens += rec.Detail.InputTokens
	globalStats.OutputTokens += rec.Detail.OutputTokens
	globalStats.ReasoningTokens += rec.Detail.ReasoningTokens
	globalStats.CachedTokens += rec.Detail.CachedTokens
	globalStats.CacheReadTokens += rec.Detail.CacheReadTokens
	globalStats.CacheCreationTokens += rec.Detail.CacheCreationTokens
	globalStats.TotalTokens += rec.Detail.TotalTokens
	latMs := float64(rec.Latency) / float64(time.Millisecond)
	aggregateDim(globalStats.Models, rec.Model, rec, latMs)
	aggregateDim(globalStats.Providers, rec.Provider, rec, latMs)
}

func aggregateDim(m map[string]*dimStats, key string, rec usageRecord, latMs float64) {
	ds, ok := m[key]
	if !ok {
		ds = &dimStats{}
		m[key] = ds
	}
	ds.Requests++
	if rec.Failed {
		ds.Failed++
	}
	ds.InputTokens += rec.Detail.InputTokens
	ds.OutputTokens += rec.Detail.OutputTokens
	ds.ReasoningTokens += rec.Detail.ReasoningTokens
	ds.CachedTokens += rec.Detail.CachedTokens
	ds.CacheReadTokens += rec.Detail.CacheReadTokens
	ds.CacheCreationTokens += rec.Detail.CacheCreationTokens
	ds.TotalTokens += rec.Detail.TotalTokens
	ds.TotalLatencyMs += latMs
	if ds.Requests > 0 {
		ds.AvgLatencyMs = ds.TotalLatencyMs / float64(ds.Requests)
	}
}

func snapshotStats() statsSnapshot {
	statsMu.RLock()
	defer statsMu.RUnlock()
	snap := globalStats
	snap.Models = make(map[string]*dimStats, len(globalStats.Models))
	for k, v := range globalStats.Models {
		cp := *v
		snap.Models[k] = &cp
	}
	snap.Providers = make(map[string]*dimStats, len(globalStats.Providers))
	for k, v := range globalStats.Providers {
		cp := *v
		snap.Providers[k] = &cp
	}
	return snap
}

func resetStats() {
	statsMu.Lock()
	defer statsMu.Unlock()
	globalStats = statsSnapshot{
		Models:    make(map[string]*dimStats),
		Providers: make(map[string]*dimStats),
	}
}

// ─── Dashboard HTML ─────────────────────────────────────────────────────────

const dashboardHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Token 用量统计</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0f172a;color:#e2e8f0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Noto Sans SC',sans-serif;padding:24px;min-height:100vh}
.container{max-width:1440px;margin:0 auto}
.header{display:flex;justify-content:space-between;align-items:center;margin-bottom:28px;flex-wrap:wrap;gap:12px}
h1{font-size:22px;font-weight:700;color:#f1f5f9}
.refresh-info{font-size:12px;color:#475569;margin-top:4px}
.btn-reset{background:#dc2626;color:#fff;border:none;padding:8px 22px;border-radius:8px;cursor:pointer;font-size:13px;font-weight:600;transition:background .2s}
.btn-reset:hover{background:#b91c1c}
.btn-reset:disabled{opacity:.5;cursor:not-allowed}
.cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(170px,1fr));gap:14px;margin-bottom:28px}
.card{background:#1e293b;border-radius:12px;padding:18px;border:1px solid #334155}
.card-label{font-size:11px;color:#94a3b8;text-transform:uppercase;letter-spacing:.6px;margin-bottom:6px}
.card-value{font-size:26px;font-weight:700;color:#f1f5f9;line-height:1.1}
.card-value.green{color:#34d399}
.card-value.blue{color:#60a5fa}
.card-value.purple{color:#a78bfa}
.card-value.amber{color:#fbbf24}
.section{margin-bottom:28px}
.section-title{font-size:16px;font-weight:600;color:#f1f5f9;margin-bottom:12px}
.table-wrap{background:#1e293b;border-radius:12px;border:1px solid #334155;overflow-x:auto}
table{width:100%;border-collapse:collapse;font-size:13px}
thead th{background:#0f172a;color:#94a3b8;font-weight:600;text-align:left;padding:11px 14px;border-bottom:1px solid #334155;white-space:nowrap}
tbody td{padding:10px 14px;border-bottom:1px solid rgba(51,65,85,.4);color:#e2e8f0;white-space:nowrap}
tbody tr:hover{background:rgba(59,130,246,.06)}
tbody tr:last-child td{border-bottom:none}
.empty{text-align:center;padding:36px;color:#64748b;font-size:14px}
.badge{display:inline-block;padding:2px 10px;border-radius:4px;font-size:12px;font-weight:600}
.badge-ok{background:rgba(52,211,153,.12);color:#34d399}
.badge-err{background:rgba(248,113,113,.12);color:#f87171}
.loading{text-align:center;padding:80px 20px;color:#64748b;font-size:15px}
.error-bar{background:rgba(248,113,113,.1);border:1px solid rgba(248,113,113,.25);border-radius:8px;padding:12px 16px;margin-bottom:16px;color:#f87171;font-size:13px;display:none}
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <div>
      <h1>Token 用量统计</h1>
      <div class="refresh-info">每 5 秒自动刷新 · <span id="lastUpdate">--</span></div>
    </div>
    <button class="btn-reset" id="btnReset" onclick="resetStats()">重置统计</button>
  </div>
  <div class="error-bar" id="errorBar"></div>
  <div id="content"><div class="loading">加载中…</div></div>
</div>
<script>
function fmt(n){if(n==null)return'0';return n.toLocaleString('en-US')}
function esc(s){var d=document.createElement('div');d.textContent=s;return d.innerHTML}
function render(d){
  var total=d.total_requests||0,failed=d.total_failed||0;
  var rate=total>0?((total-failed)/total*100).toFixed(1):'--';
  var h='<div class="cards">';
  h+='<div class="card"><div class="card-label">总请求数</div><div class="card-value">'+fmt(total)+'</div></div>';
  h+='<div class="card"><div class="card-label">失败数</div><div class="card-value" style="color:#f87171">'+fmt(failed)+'</div></div>';
  h+='<div class="card"><div class="card-label">成功率</div><div class="card-value green">'+rate+'%</div></div>';
  h+='<div class="card"><div class="card-label">输入 Token</div><div class="card-value blue">'+fmt(d.input_tokens)+'</div></div>';
  h+='<div class="card"><div class="card-label">输出 Token</div><div class="card-value blue">'+fmt(d.output_tokens)+'</div></div>';
  h+='<div class="card"><div class="card-label">推理 Token</div><div class="card-value purple">'+fmt(d.reasoning_tokens)+'</div></div>';
  h+='<div class="card"><div class="card-label">缓存 Token</div><div class="card-value amber">'+fmt(d.cached_tokens)+'</div></div>';
  h+='<div class="card"><div class="card-label">缓存读取</div><div class="card-value amber">'+fmt(d.cache_read_tokens)+'</div></div>';
  h+='<div class="card"><div class="card-label">缓存创建</div><div class="card-value amber">'+fmt(d.cache_creation_tokens)+'</div></div>';
  h+='<div class="card"><div class="card-label">总 Token</div><div class="card-value">'+fmt(d.total_tokens)+'</div></div>';
  h+='</div>';
  h+=renderTable('按模型统计',d.models,'model');
  h+=renderTable('按 Provider 统计',d.providers,'provider');
  return h;
}
function renderTable(title,obj,keyLabel){
  var h='<div class="section"><div class="section-title">'+title+'</div><div class="table-wrap">';
  h+='<table><thead><tr><th>'+keyLabel+'</th><th>请求数</th><th>失败</th><th>成功率</th><th>输入</th><th>输出</th><th>推理</th><th>缓存</th><th>总 Token</th><th>平均延迟</th></tr></thead><tbody>';
  var keys=Object.keys(obj||{}).sort();
  if(keys.length===0){
    h+='<tr><td colspan="10" class="empty">暂无数据</td></tr>';
  }else{
    for(var i=0;i<keys.length;i++){
      var k=keys[i],s=obj[k];
      var r=s.requests||0,f=s.failed||0;
      var rt=r>0?((r-f)/r*100).toFixed(1)+'%':'--';
      h+='<tr><td><strong>'+esc(k)+'</strong></td><td>'+fmt(r)+'</td><td>';
      h+=f>0?'<span class="badge badge-err">'+fmt(f)+'</span>':'<span class="badge badge-ok">0</span>';
      h+='</td><td>'+rt+'</td><td>'+fmt(s.input_tokens)+'</td><td>'+fmt(s.output_tokens)+'</td>';
      h+='<td>'+fmt(s.reasoning_tokens)+'</td><td>'+fmt(s.cached_tokens)+'</td><td>'+fmt(s.total_tokens)+'</td>';
      h+='<td>'+(s.avg_latency_ms||0).toFixed(0)+' ms</td></tr>';
    }
  }
  h+='</tbody></table></div></div>';
  return h;
}
function getManagementKey(){
  try{return localStorage.getItem('management_key')||''}catch(e){return''}
}
async function loadStats(){
  try{
    var key=getManagementKey();
    var headers={'Content-Type':'application/json'};
    if(key)headers['Authorization']='Bearer '+key;
    var res=await fetch('/v0/management/plugins/token-usage-tracker/stats',{method:'GET',headers:headers});
    if(!res.ok)throw new Error('HTTP '+res.status);
    var data=await res.json();
    document.getElementById('content').innerHTML=render(data);
    document.getElementById('lastUpdate').textContent=new Date().toLocaleTimeString('zh-CN');
    document.getElementById('errorBar').style.display='none';
  }catch(e){
    var bar=document.getElementById('errorBar');
    bar.textContent='加载失败: '+e.message+' — 请确认管理密钥已配置。';
    bar.style.display='block';
  }
}
async function resetStats(){
  if(!confirm('确定要重置所有统计数据吗？'))return;
  try{
    var key=getManagementKey();
    var headers={'Content-Type':'application/json'};
    if(key)headers['Authorization']='Bearer '+key;
    await fetch('/v0/management/plugins/token-usage-tracker/reset',{method:'POST',headers:headers});
    await loadStats();
  }catch(e){alert('重置失败: '+e.message)}
}
loadStats();
setInterval(loadStats,5000);
</script>
</body>
</html>`

// ─── Method dispatch ────────────────────────────────────────────────────────

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		return handleRegister()
	case "usage.handle":
		return handleUsage(request)
	case "management.register":
		return handleManagementRegister()
	case "management.handle":
		return handleManagementHandle(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handleRegister() ([]byte, error) {
	reg := map[string]any{
		"schema_version": schemaVersion,
		"metadata": map[string]any{
			"Name":             "token-usage-tracker",
			"Version":          "0.1.0",
			"Author":           "CLIProxyAPI",
			"GitHubRepository": "https://github.com/router-for-me/CLIProxyAPI",
			"Logo":             "",
			"ConfigFields":     []any{},
		},
		"capabilities": map[string]any{
			"usage_plugin":   true,
			"management_api": true,
		},
	}
	return okEnvelope(reg)
}

func handleUsage(request []byte) ([]byte, error) {
	var rec usageRecord
	if err := json.Unmarshal(request, &rec); err != nil {
		// Non-blocking: silently ignore malformed records
		return okEnvelope(map[string]any{})
	}
	recordUsage(rec)
	return okEnvelope(map[string]any{})
}

func handleManagementRegister() ([]byte, error) {
	// The host converts GET routes with a Menu field into resource routes
	// (legacy menu resource mechanism). This ensures the dashboard is served
	// at /v0/resource/plugins/token-usage-tracker/dashboard without authentication.
	resp := map[string]any{
		"routes": []map[string]any{
			{
				"Method":      "GET",
				"Path":        "/plugins/token-usage-tracker/dashboard",
				"Menu":        "Token 用量统计",
				"Description": "查看各模型和 Provider 的 Token 消耗统计面板",
			},
			{
				"Method": "GET",
				"Path":   "/plugins/token-usage-tracker/stats",
			},
			{
				"Method": "POST",
				"Path":   "/plugins/token-usage-tracker/reset",
			},
		},
	}
	return okEnvelope(resp)
}

func handleManagementHandle(request []byte) ([]byte, error) {
	var req managementRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return mgmtErrorResponse(400, "invalid request")
	}

	// Normalize path: the host may pass the full URL path in several formats:
	//   /v0/resource/plugins/<id>/dashboard      (resource route)
	//   /v0/management/plugins/<id>/stats         (management API route)
	//   /plugins/<id>/dashboard                   (legacy prefix)
	//   /dashboard                                (relative path)
	// We strip known prefixes to obtain a relative path for matching.
	const (
		resourceBase   = "/v0/resource/plugins/token-usage-tracker"
		managementBase = "/v0/management/plugins/token-usage-tracker"
		legacyBase     = "/plugins/token-usage-tracker"
	)
	path := req.Path
	for _, prefix := range []string{resourceBase, managementBase, legacyBase} {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			if path == "" {
				path = "/"
			}
			break
		}
	}

	switch {
	case path == "/dashboard":
		return mgmtHTMLResponse(dashboardHTML)
	case path == "/stats" && req.Method == "GET":
		return mgmtStatsResponse()
	case path == "/stats":
		return mgmtErrorResponse(405, "method not allowed")
	case path == "/reset" && req.Method == "POST":
		resetStats()
		return mgmtJSONResponse(200, map[string]any{"status": "ok"})
	case path == "/reset":
		return mgmtErrorResponse(405, "method not allowed")
	default:
		return mgmtErrorResponse(404, "not found")
	}
}

// ─── Management response helpers ────────────────────────────────────────────

func mgmtHTMLResponse(html string) ([]byte, error) {
	resp := map[string]any{
		"StatusCode": 200,
		"Headers":    map[string][]string{"Content-Type": {"text/html; charset=utf-8"}},
		"Body":       []byte(html),
	}
	return okEnvelope(resp)
}

func mgmtStatsResponse() ([]byte, error) {
	snap := snapshotStats()
	return mgmtJSONResponse(200, snap)
}

func mgmtJSONResponse(code int, v any) ([]byte, error) {
	resp := map[string]any{
		"StatusCode": code,
		"Headers":    map[string][]string{"Content-Type": {"application/json"}},
		"Body":       mustMarshal(v),
	}
	return okEnvelope(resp)
}

func mgmtErrorResponse(code int, msg string) ([]byte, error) {
	body := map[string]any{"error": msg}
	resp := map[string]any{
		"StatusCode": code,
		"Headers":    map[string][]string{"Content-Type": {"application/json"}},
		"Body":       mustMarshal(body),
	}
	return okEnvelope(resp)
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ─── Envelope helpers ───────────────────────────────────────────────────────

func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
