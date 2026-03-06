package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"text/template"
	"time"
)

type SSEUpdate struct {
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

var (
	clients   = make(map[chan SSEUpdate]bool)
	clientsMu sync.Mutex
)

func broadcastSSE(t string, d interface{}, m string) {
	u := SSEUpdate{Type: t, Data: d, Message: m}
	clientsMu.Lock()
	for ch := range clients {
		select {
		case ch <- u:
		default:
		}
	}
	clientsMu.Unlock()
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad", 400)
		return
	}
	defer r.Body.Close()
	raw := string(body)
	fmt.Printf("📡 %s\n", raw)
	broadcastSSE("LOG", nil, raw)

	var g map[string]interface{}
	if err := json.Unmarshal(body, &g); err != nil {
		w.WriteHeader(200)
		return
	}

	if _, ok := g["open"]; ok {
		ts := time.Now().Unix()
		if s, ok := g["time"].(string); ok {
			if t, e := time.Parse(time.RFC3339, s); e == nil {
				ts = t.Unix()
			}
		}
		o, _ := toFloat(g["open"])
		h, _ := toFloat(g["high"])
		l, _ := toFloat(g["low"])
		c, _ := toFloat(g["close"])
		v, _ := toFloat(g["volume"])
		sym, _ := g["symbol"].(string)
		broadcastSSE("CANDLE", map[string]interface{}{
			"time": ts, "open": o, "high": h, "low": l, "close": c, "volume": v, "symbol": sym,
		}, "")
	} else if _, ok := g["cb_premium"]; ok {
		ps, _ := g["cb_premium"].(string)
		bs, _ := g["btc_price"].(string)
		p, e1 := strconv.ParseFloat(ps, 64)
		b, e2 := strconv.ParseFloat(bs, 64)
		if e1 != nil || e2 != nil {
			w.WriteHeader(200)
			return
		}
		sym, _ := g["symbol"].(string)
		broadcastSSE("TICK", map[string]interface{}{
			"time": time.Now().Unix(), "price": b, "premium": p, "symbol": sym,
		}, "")
	}
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ch := make(chan SSEUpdate, 30)
	clientsMu.Lock()
	clients[ch] = true
	clientsMu.Unlock()
	defer func() {
		clientsMu.Lock()
		delete(clients, ch)
		clientsMu.Unlock()
		close(ch)
	}()
	for {
		select {
		case m := <-ch:
			j, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", j)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	tmpl, _ := template.New("i").Parse(pageHTML)
	tmpl.Execute(w, nil)
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/events", eventsHandler)
	http.HandleFunc("/webhook", webhookHandler)
	fmt.Println("🚀 http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

const pageHTML = `<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>BTC Trading Terminal</title>
<script src="https://unpkg.com/lightweight-charts@4.1.0/dist/lightweight-charts.standalone.production.js"></script>
<link href="https://fonts.googleapis.com/css2?family=Cairo:wght@400;600;700;800;900&family=JetBrains+Mono:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{
 --bg:#0f1923;--bg2:#1a2332;--bg3:#243447;
 --border:#1c2b3a;--border2:#2a3f55;
 --text:#e0e6ed;--text2:#8899a6;--text3:#546678;
 --green:#00c087;--green-bg:rgba(0,192,135,0.08);
 --red:#ff4757;--red-bg:rgba(255,71,87,0.08);
 --blue:#1da1f2;--blue-bg:rgba(29,161,242,0.08);
 --gold:#f7931a;--gold-bg:rgba(247,147,26,0.08);
 --purple:#9b59b6;
}
body{background:var(--bg);color:var(--text);font-family:'Cairo',sans-serif;height:100vh;overflow:hidden;display:flex;flex-direction:column}
.mono{font-family:'JetBrains Mono',monospace}

/* ===== HEADER ===== */
.hdr{
 display:flex;align-items:center;justify-content:space-between;
 padding:0 16px;height:48px;
 background:var(--bg2);border-bottom:1px solid var(--border);
 flex-shrink:0;
}
.hdr-r{display:flex;align-items:center;gap:12px}
.logo{
 width:32px;height:32px;border-radius:8px;
 background:linear-gradient(135deg,var(--gold),#e6850c);
 display:flex;align-items:center;justify-content:center;
 box-shadow:0 2px 10px rgba(247,147,26,0.25);
}
.logo svg{width:16px;height:16px;color:white}
.hdr-title{font-size:14px;font-weight:800;color:white}
.hdr-conn{display:flex;align-items:center;gap:5px;margin-top:1px}
.dot{width:5px;height:5px;border-radius:50%;background:var(--red);transition:all .3s}
.dot.on{background:var(--green);box-shadow:0 0 6px rgba(0,192,135,.5);animation:pulse 2s infinite}
@keyframes pulse{0%,100%{box-shadow:0 0 4px rgba(0,192,135,.3)}50%{box-shadow:0 0 10px rgba(0,192,135,.7)}}
.conn-txt{font-size:9px;font-weight:700;color:var(--text3);transition:color .3s}
.conn-txt.on{color:var(--green)}
.hdr-l{display:flex;align-items:center;gap:8px}
.hdr-stat{
 padding:4px 12px;border-radius:4px;
 background:var(--bg);border:1px solid var(--border);
 display:flex;align-items:center;gap:6px;
}
.hdr-stat-l{font-size:8px;text-transform:uppercase;letter-spacing:1px;font-weight:700;color:var(--text3)}
.hdr-stat-v{font-size:12px;font-weight:700}

/* ===== TICKER BAR ===== */
.ticker{
 display:flex;align-items:center;
 padding:0 16px;height:44px;
 background:var(--bg2);border-bottom:1px solid var(--border);
 flex-shrink:0;gap:0;overflow-x:auto;
}
.tk{padding:0 16px;display:flex;align-items:center;gap:8px;border-left:1px solid var(--border);height:100%;flex-shrink:0}
.tk:first-child{border-left:none}
.tk-l{font-size:9px;color:var(--text3);font-weight:600;text-transform:uppercase;letter-spacing:.5px}
.tk-v{font-size:15px;font-weight:800}
.tk-v.up{color:var(--green)}.tk-v.dn{color:var(--red)}
.tk-badge{font-size:9px;font-weight:700;padding:2px 6px;border-radius:3px}
.tk-badge.up{color:var(--green);background:var(--green-bg)}
.tk-badge.dn{color:var(--red);background:var(--red-bg)}

/* ===== MAIN GRID ===== */
.grid{display:grid;grid-template-columns:1fr 360px;flex:1;min-height:0}

/* Chart column */
.chart-col{display:flex;flex-direction:column;min-height:0;border-left:1px solid var(--border)}
.chart-bar{
 display:flex;align-items:center;gap:0;
 padding:0 8px;height:32px;
 background:var(--bg2);border-bottom:1px solid var(--border);
 flex-shrink:0;
}
.cb{
 padding:3px 8px;font-size:9px;font-weight:700;letter-spacing:.3px;
 color:var(--text3);background:transparent;border:none;
 border-radius:3px;cursor:pointer;transition:all .12s;
}
.cb:hover{color:var(--text);background:var(--bg3)}
.cb.on{color:var(--blue);background:var(--blue-bg)}
.cb-sep{width:1px;height:14px;background:var(--border);margin:0 4px}
.chart-wrap{flex:1;min-height:0;background:var(--bg)}

/* Right panel */
.rpanel{display:flex;flex-direction:column;min-height:0;overflow:hidden;background:var(--bg2)}
.tabs{display:flex;border-bottom:1px solid var(--border);flex-shrink:0}
.tab{
 flex:1;padding:7px 0;text-align:center;
 font-size:10px;font-weight:700;color:var(--text3);
 cursor:pointer;border:none;background:transparent;
 border-bottom:2px solid transparent;transition:all .2s;
}
.tab:hover{color:var(--text)}
.tab.on{color:var(--blue);border-bottom-color:var(--blue)}
.pages{flex:1;overflow:hidden;position:relative;min-height:0}
.page{position:absolute;inset:0;overflow-y:auto;display:none}
.page.on{display:block}

/* ===== TRADE SIGNAL CARD ===== */
.signal-card{
 margin:8px;padding:14px;border-radius:10px;
 background:var(--bg);border:1px solid var(--border);
 flex-shrink:0;
}
.sig-hdr{display:flex;align-items:center;justify-content:space-between;margin-bottom:10px}
.sig-title{font-size:11px;font-weight:700;color:var(--text2);text-transform:uppercase;letter-spacing:1px}
.sig-time{font-size:9px;color:var(--text3)}
.sig-main{
 padding:12px;border-radius:8px;margin-bottom:10px;
 text-align:center;
}
.sig-main.bullish{background:var(--green-bg);border:1px solid rgba(0,192,135,0.15)}
.sig-main.bearish{background:var(--red-bg);border:1px solid rgba(255,71,87,0.15)}
.sig-main.neutral{background:var(--blue-bg);border:1px solid rgba(29,161,242,0.15)}
.sig-icon{font-size:28px;margin-bottom:4px}
.sig-label{font-size:16px;font-weight:900}
.sig-label.bullish{color:var(--green)}.sig-label.bearish{color:var(--red)}.sig-label.neutral{color:var(--blue)}
.sig-desc{font-size:10px;color:var(--text2);margin-top:4px}

.sig-details{display:grid;grid-template-columns:1fr 1fr;gap:6px}
.sig-d{padding:8px;border-radius:6px;background:var(--bg2);border:1px solid var(--border)}
.sig-d-label{font-size:8px;color:var(--text3);text-transform:uppercase;letter-spacing:.8px;font-weight:700;margin-bottom:2px}
.sig-d-val{font-size:13px;font-weight:800}

/* Trade suggestions */
.trade-sug{margin:8px;padding:12px;border-radius:10px;background:var(--bg);border:1px solid var(--border)}
.ts-title{font-size:10px;font-weight:700;color:var(--gold);text-transform:uppercase;letter-spacing:1px;margin-bottom:8px;display:flex;align-items:center;gap:6px}
.ts-item{
 padding:8px 10px;border-radius:6px;margin-bottom:6px;
 border:1px solid var(--border);display:flex;align-items:center;gap:10px;
 transition:border-color .2s;
}
.ts-item:hover{border-color:var(--border2)}
.ts-item:last-child{margin-bottom:0}
.ts-dir{
 width:28px;height:28px;border-radius:6px;
 display:flex;align-items:center;justify-content:center;
 font-size:14px;font-weight:900;flex-shrink:0;
}
.ts-dir.buy{background:var(--green-bg);color:var(--green)}
.ts-dir.sell{background:var(--red-bg);color:var(--red)}
.ts-dir.hold{background:var(--blue-bg);color:var(--blue)}
.ts-info{flex:1}
.ts-info-title{font-size:11px;font-weight:700;color:var(--text)}
.ts-info-desc{font-size:9px;color:var(--text3);margin-top:1px}
.ts-conf{font-size:10px;font-weight:700;padding:2px 8px;border-radius:4px}
.ts-conf.high{color:var(--green);background:var(--green-bg)}
.ts-conf.med{color:var(--gold);background:var(--gold-bg)}
.ts-conf.low{color:var(--text3);background:rgba(84,102,120,0.15)}

/* Data items */
.di{padding:8px 12px;border-bottom:1px solid var(--border);animation:sIn .25s ease}
@keyframes sIn{from{opacity:0;transform:translateX(8px)}to{opacity:1;transform:translateX(0)}}
.di:hover{background:rgba(29,161,242,0.03)}
.di-top{display:flex;justify-content:space-between;margin-bottom:3px}
.di-time{font-size:9px;color:var(--text3)}
.di-n{font-size:8px;color:var(--text3)}
.di-bot{display:flex;align-items:center;gap:10px}
.di-p{font-size:13px;font-weight:700;color:var(--text)}
.di-pr{font-size:11px;font-weight:700;padding:1px 6px;border-radius:3px}
.di-pr.up{color:var(--green);background:var(--green-bg)}
.di-pr.dn{color:var(--red);background:var(--red-bg)}
.di-arrow{font-size:10px;font-weight:700}
.di-arrow.up{color:var(--green)}.di-arrow.dn{color:var(--red)}

/* JSON cards */
.jcard{margin:6px 8px;padding:8px 10px;border-radius:6px;background:var(--bg);border:1px solid var(--border);animation:sIn .25s ease}
.jcard:hover{border-color:var(--border2)}
.jcard-h{display:flex;justify-content:space-between;margin-bottom:4px}
.jcard-t{font-size:9px;color:var(--blue);font-weight:600}
.jcard-n{font-size:8px;color:var(--text3)}
.jcard-b{white-space:pre-wrap;word-break:break-all;line-height:1.7;font-size:9px;direction:ltr;text-align:left}
.jk{color:var(--blue)}.jst{color:var(--green)}.jnu{color:var(--gold)}.jbo{color:var(--purple)}.jnl{color:var(--text3)}

.empty{display:flex;flex-direction:column;align-items:center;gap:8px;padding:40px 16px;color:var(--text3);font-size:11px;text-align:center}
.empty svg{width:28px;height:28px}

::-webkit-scrollbar{width:3px}::-webkit-scrollbar-track{background:transparent}::-webkit-scrollbar-thumb{background:var(--border);border-radius:3px}

@media(max-width:900px){
 .grid{grid-template-columns:1fr;height:auto}
 .chart-col{height:55vh}.rpanel{height:45vh}
 .ticker{flex-wrap:wrap;height:auto;padding:6px 12px}
}
</style>
</head>
<body>

<div class="hdr">
 <div class="hdr-r">
  <div class="logo"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M3 3v18h18" stroke-linecap="round"/><path d="M7 16l4-8 4 4 4-8" stroke-linecap="round"/></svg></div>
  <div>
   <div class="hdr-title">BTC Trading Terminal ₿</div>
   <div class="hdr-conn"><span class="dot" id="cDot"></span><span class="conn-txt" id="cTxt">جاري الاتصال...</span></div>
  </div>
 </div>
 <div class="hdr-l">
  <div class="hdr-stat"><span class="hdr-stat-l">تحديثات</span><span class="hdr-stat-v mono" style="color:var(--green)" id="hN">0</span></div>
  <div class="hdr-stat"><span class="hdr-stat-l">آخر تحديث</span><span class="hdr-stat-v mono" style="color:var(--text2);font-size:10px" id="hT">--:--:--</span></div>
 </div>
</div>

<div class="ticker">
 <div class="tk"><span class="tk-l">BTCUSD</span><span class="tk-v up mono" id="tP">0.00</span><span class="tk-badge up mono" id="tC">--</span></div>
 <div class="tk"><span class="tk-l">بريميوم</span><span class="tk-v up mono" id="tPr">0.00</span></div>
 <div class="tk"><span class="tk-l">الإشارة</span><span class="tk-v mono" id="tSig" style="font-size:12px;color:var(--text3)">بانتظار...</span></div>
 <div class="tk"><span class="tk-l">أعلى</span><span class="tk-v mono" id="tHi" style="color:var(--green);font-size:12px">--</span></div>
 <div class="tk"><span class="tk-l">أدنى</span><span class="tk-v mono" id="tLo" style="color:var(--red);font-size:12px">--</span></div>
 <div class="tk"><span class="tk-l">بيانات</span><span class="tk-v mono" style="color:var(--blue);font-size:12px" id="tN">0</span></div>
</div>

<div class="grid">
 <div class="chart-col">
  <div class="chart-bar">
   <button class="cb" onclick="setInt(5)" id="b5">5ث</button>
   <button class="cb on" onclick="setInt(10)" id="b10">10ث</button>
   <button class="cb" onclick="setInt(15)" id="b15">15ث</button>
   <button class="cb" onclick="setInt(30)" id="b30">30ث</button>
   <button class="cb" onclick="setInt(60)" id="b60">1د</button>
   <span class="cb-sep"></span>
   <button class="cb" onclick="chart.timeScale().fitContent()">⊞ ملائمة</button>
   <button class="cb" onclick="chart.timeScale().scrollToRealTime()">◉ حالي</button>
  </div>
  <div class="chart-wrap" id="C"></div>
 </div>

 <div class="rpanel">
  <div class="tabs">
   <button class="tab on" onclick="stab(0)" id="t0">💡 إشارات التداول</button>
   <button class="tab" onclick="stab(1)" id="t1">📊 البيانات</button>
   <button class="tab" onclick="stab(2)" id="t2">📋 JSON</button>
  </div>
  <div class="pages">
   <div class="page on" id="p0">
    <!-- Signal Card -->
    <div class="signal-card" id="sigCard">
     <div class="sig-hdr">
      <span class="sig-title">🎯 إشارة التداول الحالية</span>
      <span class="sig-time mono" id="sigTime">--:--:--</span>
     </div>
     <div class="sig-main neutral" id="sigBox">
      <div class="sig-icon" id="sigIcon">⏳</div>
      <div class="sig-label neutral" id="sigLabel">بانتظار البيانات</div>
      <div class="sig-desc" id="sigDesc">سيتم تحليل البيانات فور وصولها</div>
     </div>
     <div class="sig-details">
      <div class="sig-d"><div class="sig-d-label">السعر</div><div class="sig-d-val mono" id="sdPrice" style="color:var(--text)">--</div></div>
      <div class="sig-d"><div class="sig-d-label">البريميوم</div><div class="sig-d-val mono" id="sdPrem" style="color:var(--text)">--</div></div>
      <div class="sig-d"><div class="sig-d-label">الاتجاه</div><div class="sig-d-val" id="sdTrend" style="color:var(--text3)">--</div></div>
      <div class="sig-d"><div class="sig-d-label">القوة</div><div class="sig-d-val" id="sdStr" style="color:var(--text3)">--</div></div>
     </div>
    </div>
    <!-- Trade Suggestions -->
    <div class="trade-sug" id="tradeSug">
     <div class="ts-title">⚡ اقتراحات التداول</div>
     <div id="sugList">
      <div class="empty"><svg fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1"><path stroke-linecap="round" stroke-linejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/></svg>بانتظار بيانات كافية لتوليد الاقتراحات...</div>
     </div>
    </div>
   </div>
   <div class="page" id="p1"><div class="empty" id="dEmpty"><svg fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1"><path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"/></svg>بانتظار البيانات...</div><div id="dList" style="display:none"></div></div>
   <div class="page" id="p2"><div class="empty" id="jEmpty"><svg fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1"><path stroke-linecap="round" stroke-linejoin="round" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>بانتظار JSON...</div><div id="jList"></div></div>
  </div>
 </div>
</div>

<script>
// ===== STATE =====
let n=0, dr=0, jr=0, lastP=0, sessionHigh=0, sessionLow=Infinity;
let INT=10; // candle seconds
let pts=[]; // all {ts, price, vol}
let cMap={}; // slot -> candle
let premHistory=[]; // for analysis

// Tabs
function stab(i){document.querySelectorAll('.tab').forEach((t,x)=>t.classList.toggle('on',x===i));document.querySelectorAll('.page').forEach((p,x)=>p.classList.toggle('on',x===i))}

// ===== CHART =====
const C=document.getElementById('C');
const chart=LightweightCharts.createChart(C,{
 layout:{background:{type:'solid',color:'#0f1923'},textColor:'#546678',fontSize:11,fontFamily:"'JetBrains Mono',monospace"},
 grid:{vertLines:{color:'#1c2b3a'},horzLines:{color:'#1c2b3a'}},
 crosshair:{
  mode:LightweightCharts.CrosshairMode.Normal,
  vertLine:{color:'#2a3f55',width:1,style:0,labelBackgroundColor:'#1da1f2'},
  horzLine:{color:'#2a3f55',width:1,style:0,labelBackgroundColor:'#1da1f2'},
 },
 timeScale:{timeVisible:true,secondsVisible:true,borderColor:'#1c2b3a',rightOffset:5,barSpacing:12,minBarSpacing:5},
 rightPriceScale:{borderColor:'#1c2b3a',scaleMargins:{top:0.08,bottom:0.15}},
 handleScale:{axisPressedMouseMove:true},handleScroll:{vertTouchDrag:true},
});

const cSeries=chart.addCandlestickSeries({
 upColor:'#00c087',downColor:'#ff4757',
 borderUpColor:'#00c087',borderDownColor:'#ff4757',
 wickUpColor:'#00c087',wickDownColor:'#ff4757',
});

const vSeries=chart.addHistogramSeries({
 priceFormat:{type:'volume'},priceScaleId:'vol',
});
chart.priceScale('vol').applyOptions({scaleMargins:{top:0.85,bottom:0},borderVisible:false});

const pLine=chart.addLineSeries({
 color:'#1da1f2',lineWidth:2,priceScaleId:'pr',title:'Premium',
 crosshairMarkerVisible:true,crosshairMarkerRadius:3,
});
chart.priceScale('pr').applyOptions({scaleMargins:{top:0.78,bottom:0.01},borderVisible:false});

new ResizeObserver(e=>{const{width,height}=e[0].contentRect;chart.applyOptions({width,height})}).observe(C);

// ===== CANDLE ENGINE =====
function slot(ts){return Math.floor(ts/INT)*INT}

function rebuild(){
 cMap={};
 for(const p of pts){
  const s=Math.floor(p.ts/INT)*INT;
  if(!cMap[s])cMap[s]={time:s,open:p.price,high:p.price,low:p.price,close:p.price,vol:p.vol||0};
  else{const c=cMap[s];c.high=Math.max(c.high,p.price);c.low=Math.min(c.low,p.price);c.close=p.price;c.vol+=(p.vol||0)}
 }
 const sorted=Object.keys(cMap).map(Number).sort((a,b)=>a-b);
 const candles=[],vols=[];
 for(const s of sorted){
  const c=cMap[s];
  candles.push({time:c.time,open:c.open,high:c.high,low:c.low,close:c.close});
  vols.push({time:c.time,value:c.vol,color:c.close>=c.open?'rgba(0,192,135,0.25)':'rgba(255,71,87,0.25)'});
 }
 cSeries.setData(candles);vSeries.setData(vols);
}

function addTick(ts,price,vol){
 pts.push({ts,price,vol:vol||0});
 const s=slot(ts);
 if(!cMap[s])cMap[s]={time:s,open:price,high:price,low:price,close:price,vol:vol||0};
 else{const c=cMap[s];c.high=Math.max(c.high,price);c.low=Math.min(c.low,price);c.close=price;c.vol+=(vol||0)}
 const c=cMap[s];
 cSeries.update({time:c.time,open:c.open,high:c.high,low:c.low,close:c.close});
 vSeries.update({time:c.time,value:c.vol,color:c.close>=c.open?'rgba(0,192,135,0.25)':'rgba(255,71,87,0.25)'});
}

function addOHLC(ts,o,h,l,cl,v){
 pts.push({ts,price:o,vol:0},{ts:ts+.25,price:h,vol:0},{ts:ts+.5,price:l,vol:0},{ts:ts+.75,price:cl,vol:v||0});
 const s=slot(ts);
 if(!cMap[s])cMap[s]={time:s,open:o,high:h,low:l,close:cl,vol:v||0};
 else{const c=cMap[s];c.high=Math.max(c.high,h);c.low=Math.min(c.low,l);c.close=cl;c.vol+=(v||0)}
 const c=cMap[s];
 cSeries.update({time:c.time,open:c.open,high:c.high,low:c.low,close:c.close});
 vSeries.update({time:c.time,value:c.vol,color:c.close>=c.open?'rgba(0,192,135,0.25)':'rgba(255,71,87,0.25)'});
}

function setInt(s){
 INT=s;
 document.querySelectorAll('.cb').forEach(b=>b.classList.remove('on'));
 const el=document.getElementById('b'+s);if(el)el.classList.add('on');
 rebuild();chart.timeScale().fitContent();
}

// ===== TRADING SIGNAL ENGINE =====
function analyzeAndSuggest(price, premium) {
 premHistory.push({premium, price, time: Date.now()});
 // Keep last 50
 if(premHistory.length>50) premHistory.shift();

 const sigBox=document.getElementById('sigBox');
 const sigIcon=document.getElementById('sigIcon');
 const sigLabel=document.getElementById('sigLabel');
 const sigDesc=document.getElementById('sigDesc');
 document.getElementById('sigTime').textContent=new Date().toLocaleTimeString('en-US',{hour12:false});
 document.getElementById('sdPrice').textContent='$'+price.toLocaleString('en',{minimumFractionDigits:2,maximumFractionDigits:2});
 document.getElementById('sdPrem').textContent=premium.toFixed(2);
 document.getElementById('sdPrem').style.color=premium>=0?'var(--green)':'var(--red)';

 // Determine signal
 let signal='neutral', strength=0, trend='محايد';

 if(premium>15){signal='strong_buy';strength=90;trend='صعود قوي جداً'}
 else if(premium>8){signal='buy';strength=75;trend='صعود قوي'}
 else if(premium>3){signal='mild_buy';strength=60;trend='صعود خفيف'}
 else if(premium>0){signal='slight_buy';strength=50;trend='ميل للصعود'}
 else if(premium>-3){signal='slight_sell';strength=50;trend='ميل للهبوط'}
 else if(premium>-8){signal='mild_sell';strength=60;trend='هبوط خفيف'}
 else if(premium>-15){signal='sell';strength=75;trend='هبوط قوي'}
 else{signal='strong_sell';strength=90;trend='هبوط قوي جداً'}

 document.getElementById('sdTrend').textContent=trend;
 document.getElementById('sdTrend').style.color=premium>=0?'var(--green)':'var(--red)';
 document.getElementById('sdStr').textContent=strength+'%';
 document.getElementById('sdStr').style.color=strength>70?'var(--green)':strength>50?'var(--gold)':'var(--text3)';

 // Update signal card
 if(signal.includes('buy')){
  sigBox.className='sig-main bullish';
  sigIcon.textContent=premium>8?'🚀':'📈';
  sigLabel.textContent=premium>15?'شراء قوي جداً 🔥':premium>8?'شراء قوي':'شراء';
  sigLabel.className='sig-label bullish';
  sigDesc.textContent=premium>8?'ضغط شراء مؤسساتي قوي على Coinbase - فرصة دخول':'ضغط شراء إيجابي على Coinbase';
 } else if(signal.includes('sell')){
  sigBox.className='sig-main bearish';
  sigIcon.textContent=premium<-8?'🔻':'📉';
  sigLabel.textContent=premium<-15?'بيع قوي جداً ⚠️':premium<-8?'بيع قوي':'بيع';
  sigLabel.className='sig-label bearish';
  sigDesc.textContent=premium<-8?'ضغط بيع مؤسساتي كبير - انتبه للمخاطر':'ضغط بيع سلبي على Coinbase';
 } else {
  sigBox.className='sig-main neutral';
  sigIcon.textContent='⚖️';
  sigLabel.textContent='محايد';
  sigLabel.className='sig-label neutral';
  sigDesc.textContent='لا يوجد ضغط واضح - انتظر إشارة أقوى';
 }

 // Generate trade suggestions
 generateSuggestions(price, premium, strength, signal);
}

function generateSuggestions(price, premium, strength, signal) {
 const list=document.getElementById('sugList');

 // Calculate levels
 const entry=price;
 const stopLoss=premium>=0 ? price*0.985 : price*1.015;
 const tp1=premium>=0 ? price*1.01 : price*0.99;
 const tp2=premium>=0 ? price*1.025 : price*0.975;

 // Avg premium trend
 let avgPrem=premium;
 if(premHistory.length>=3){
  const last3=premHistory.slice(-3);
  avgPrem=last3.reduce((s,p)=>s+p.premium,0)/3;
 }
 const premTrending=premHistory.length>=3 ? (premHistory[premHistory.length-1].premium > premHistory[premHistory.length-3]?.premium ? 'up' : 'down') : 'flat';

 let html='';

 if(signal.includes('strong_buy')){
  html+=mkSug('buy','شراء فوري','البريميوم مرتفع جداً - فرصة قوية','high');
  html+=mkSug('buy','هدف أول: $'+tp1.toFixed(0),'TP1 عند +1% من السعر الحالي','high');
  html+=mkSug('buy','هدف ثاني: $'+tp2.toFixed(0),'TP2 عند +2.5% من السعر الحالي','med');
  html+=mkSug('sell','وقف خسارة: $'+stopLoss.toFixed(0),'SL عند -1.5% لحماية رأس المال','high');
 } else if(signal.includes('buy')){
  html+=mkSug('buy','شراء تدريجي','دخول على مراحل - البريميوم إيجابي','med');
  html+=mkSug('buy','هدف: $'+tp1.toFixed(0),'TP عند +1% من السعر','med');
  html+=mkSug('sell','وقف خسارة: $'+stopLoss.toFixed(0),'SL عند -1.5%','high');
  html+=mkSug('hold','راقب البريميوم',''+( premTrending==='up'?'البريميوم في ارتفاع ✅':'البريميوم في تراجع ⚠️'),'low');
 } else if(signal.includes('strong_sell')){
  html+=mkSug('sell','بيع / شورت','ضغط بيع مؤسساتي كبير','high');
  html+=mkSug('sell','هدف: $'+tp1.toFixed(0),'TP عند -1% من السعر','high');
  html+=mkSug('buy','وقف خسارة: $'+stopLoss.toFixed(0),'SL عند +1.5%','high');
  html+=mkSug('sell','حماية أرباح','قلل حجم المواقع المفتوحة','med');
 } else if(signal.includes('sell')){
  html+=mkSug('sell','تقليل المراكز','البريميوم سلبي - حذر','med');
  html+=mkSug('hold','لا تفتح صفقات جديدة','انتظر تأكيد الاتجاه','med');
  html+=mkSug('buy','وقف خسارة: $'+stopLoss.toFixed(0),'SL عند +1.5%','high');
 } else {
  html+=mkSug('hold','انتظر','لا توجد إشارة واضحة','low');
  html+=mkSug('hold','راقب البريميوم','البريميوم قريب من الصفر','low');
  if(premTrending==='up') html+=mkSug('buy','استعد للشراء','البريميوم يتحسن','low');
  else html+=mkSug('sell','استعد للبيع','البريميوم يتراجع','low');
 }

 list.innerHTML=html;
}

function mkSug(dir,title,desc,conf){
 const icon=dir==='buy'?'▲':dir==='sell'?'▼':'●';
 const confAr=conf==='high'?'عالية':conf==='med'?'متوسطة':'منخفضة';
 return '<div class="ts-item">'+
  '<div class="ts-dir '+dir+'">'+icon+'</div>'+
  '<div class="ts-info"><div class="ts-info-title">'+title+'</div><div class="ts-info-desc">'+desc+'</div></div>'+
  '<span class="ts-conf '+conf+'">'+confAr+'</span>'+
 '</div>';
}

// ===== JSON HIGHLIGHT =====
function esc(s){return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')}
function hl(j){
 j=esc(j);
 return j.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,function(m){
  let c='jnu';
  if(/^"/.test(m)){if(/:$/.test(m))return'<span class="jk">'+m.slice(0,-1)+'</span><span style="color:var(--text3)">:</span>';c='jst'}
  else if(/true|false/.test(m))c='jbo';
  else if(/null/.test(m))c='jnl';
  return'<span class="'+c+'">'+m+'</span>';
 });
}
function fmtJ(r){try{return hl(JSON.stringify(JSON.parse(r),null,2))}catch(e){return esc(r)}}

// ===== UI =====
function updateTicker(price,prem){
 const tp=document.getElementById('tP');
 tp.textContent=price.toLocaleString('en',{minimumFractionDigits:2,maximumFractionDigits:2});
 const tc=document.getElementById('tC');
 if(lastP>0){
  const d=price-lastP,pct=((d/lastP)*100).toFixed(3);
  if(d>=0){tp.className='tk-v up mono';tc.textContent='▲+'+pct+'%';tc.className='tk-badge up mono'}
  else{tp.className='tk-v dn mono';tc.textContent='▼'+pct+'%';tc.className='tk-badge dn mono'}
 }
 lastP=price;

 // Session high/low
 if(price>sessionHigh){sessionHigh=price;document.getElementById('tHi').textContent=price.toLocaleString('en',{minimumFractionDigits:0,maximumFractionDigits:0})}
 if(price<sessionLow){sessionLow=price;document.getElementById('tLo').textContent=price.toLocaleString('en',{minimumFractionDigits:0,maximumFractionDigits:0})}

 if(prem!==null&&prem!==undefined){
  const pr=document.getElementById('tPr');
  pr.textContent=prem.toFixed(2);
  pr.className=prem>=0?'tk-v up mono':'tk-v dn mono';
  const sig=document.getElementById('tSig');
  if(prem>10){sig.textContent='🚀 شراء قوي';sig.style.color='var(--green)'}
  else if(prem>0){sig.textContent='📈 شراء';sig.style.color='var(--green)'}
  else if(prem>-10){sig.textContent='📉 بيع';sig.style.color='var(--red)'}
  else{sig.textContent='🔻 بيع قوي';sig.style.color='var(--red)'}
 }
}

function addData(price,prem,sym){
 dr++;
 const e=document.getElementById('dEmpty'),l=document.getElementById('dList');
 if(e&&e.style.display!=='none'){e.style.display='none';l.style.display='block'}
 const now=new Date().toLocaleTimeString('en-US',{hour12:false});
 const pc=prem!==null&&prem>=0?'up':'dn';
 const pt=prem!==null?prem.toFixed(2):'--';
 const ar=prem!==null?(prem>=0?'▲ شراء':'▼ بيع'):'--';
 const d=document.createElement('div');d.className='di';
 d.innerHTML='<div class="di-top"><span class="di-time mono">'+now+' · '+(sym||'BTCUSD')+'</span><span class="di-n mono">#'+dr+'</span></div><div class="di-bot"><span class="di-p mono">$'+price.toLocaleString('en',{minimumFractionDigits:2,maximumFractionDigits:2})+'</span><span class="di-pr '+pc+' mono">'+pt+'</span><span class="di-arrow '+pc+'">'+ar+'</span></div>';
 if(l.firstChild)l.insertBefore(d,l.firstChild);else l.appendChild(d);
 while(l.children.length>100)l.removeChild(l.lastChild);
}

function addJSON(raw){
 jr++;const e=document.getElementById('jEmpty');if(e)e.remove();
 const l=document.getElementById('jList');
 const now=new Date().toLocaleTimeString('en-US',{hour12:false});
 const d=document.createElement('div');d.className='jcard';
 d.innerHTML='<div class="jcard-h"><span class="jcard-t mono">'+now+'</span><span class="jcard-n mono">#'+jr+'</span></div><pre class="jcard-b mono">'+fmtJ(raw)+'</pre>';
 if(l.firstChild)l.insertBefore(d,l.firstChild);else l.appendChild(d);
 while(l.children.length>50)l.removeChild(l.lastChild);
}

function bump(){
 n++;
 document.getElementById('hN').textContent=n;
 document.getElementById('tN').textContent=n;
 document.getElementById('hT').textContent=new Date().toLocaleTimeString('en-US',{hour12:false});
}

// ===== SSE =====
const src=new EventSource('/events');
src.onopen=function(){
 document.getElementById('cDot').classList.add('on');
 const t=document.getElementById('cTxt');t.textContent='متصل · مباشر';t.classList.add('on');
};
src.onerror=function(){
 document.getElementById('cDot').classList.remove('on');
 const t=document.getElementById('cTxt');t.textContent='إعادة الاتصال...';t.classList.remove('on');
};
src.onmessage=function(ev){
 try{
  const m=JSON.parse(ev.data);
  if(m.type==='CANDLE'&&m.data){
   const d=m.data;bump();
   addOHLC(d.time,d.open,d.high,d.low,d.close,d.volume);
   updateTicker(d.close,null);addData(d.close,null,d.symbol);
   chart.timeScale().scrollToRealTime();
  }
  else if(m.type==='TICK'&&m.data){
   const d=m.data;bump();
   addTick(d.time,d.price,0);
   updateTicker(d.price,d.premium);
   try{pLine.update({time:slot(d.time),value:d.premium})}catch(e){}
   addData(d.price,d.premium,d.symbol);
   analyzeAndSuggest(d.price,d.premium);
   chart.timeScale().scrollToRealTime();
  }
  else if(m.type==='LOG'){addJSON(m.message)}
 }catch(e){console.error(e)}
};
</script>
</body>
</html>
`