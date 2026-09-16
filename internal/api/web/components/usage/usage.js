import {setConn} from '../status/status.js';
import {api} from '../../core/api.js';
import {$, el, emptyState, esc} from '../../core/dom.js';

/* ---- Consumption statistics (tokens used via claudeq) ---- */
const fmtCost=n=>'$'+(n||0).toFixed(2);
const fmtTok=n=>{ n=n||0; return n>=1e6?(n/1e6).toFixed(2)+'M':n>=1e3?(n/1e3).toFixed(1)+'k':(''+n); };
const fmtDur=ms=>{ const s=Math.round((ms||0)/1000); if(s<60)return s+'s'; const m=Math.floor(s/60); if(m<60)return m+'m'; return Math.floor(m/60)+'h '+(m%60)+'m'; };
const statCell=(label,val)=>`<div style="flex:1;min-width:84px"><div class="usage-num" style="font-size:20px">${val}</div><div class="sub">${label}</div></div>`;

async function loadUsage(){
  let stats; try{ stats=await api('GET','/api/stats'); setConn(true);}catch(e){ setConn(false); return; }
  const c=$('#usage'); c.innerHTML='';
  if(!stats) return;
  const tot=stats.totals, w=stats.last_7d;
  if(tot.runs===0){ c.append(emptyState('Nothing consumed yet','Token usage appears here after claudeq has run some tasks.')); return; }

  const totalTok=tot.input_tokens+tot.output_tokens;
  const head=el('div');
  head.innerHTML=`<div class="section-label">Total consumed via claudeq</div>
    <div class="group"><div class="row" style="display:block">
      <div class="sub">Tokens</div>
      <div class="usage-num">${fmtTok(totalTok)}</div>
      <div class="sub" style="margin-top:6px">${fmtTok(tot.input_tokens)} in · ${fmtTok(tot.output_tokens)} out · ${fmtCost(tot.cost_usd)} API-equiv. · ${tot.runs} runs · ${fmtDur(tot.duration_ms)}</div>
    </div></div>
    <div class="section-label">Last 7 days</div>
    <div class="group"><div class="row" style="gap:18px;flex-wrap:wrap">
      ${statCell('tokens',fmtTok(w.input_tokens+w.output_tokens))}${statCell('API-equiv. cost',fmtCost(w.cost_usd))}${statCell('runs',w.runs)}${statCell('succeeded',w.success)}${statCell('failed',w.failed)}${statCell('runtime',fmtDur(w.duration_ms))}
    </div></div>`;
  c.append(head);

  // Several providers mean several allowances, so the split says which account
  // the work went to. With one provider there is nothing to split.
  if((stats.by_provider||[]).length){
    const split=el('div');
    split.innerHTML=`<div class="section-label">By provider</div><div class="group">`
      +stats.by_provider.map(p=>`<div class="row">
          <div class="grow"><div class="title">${esc(p.name||p.id||'Not recorded')}</div>
            <div class="sub">${p.runs} runs · ${p.success} succeeded · ${p.failed} failed · ${fmtDur(p.duration_ms)}</div></div>
          <div style="text-align:right"><div class="usage-num" style="font-size:15px">${fmtTok(p.input_tokens+p.output_tokens)}</div>
            <div class="sub">${fmtCost(p.cost_usd)} API-equiv.</div></div>
        </div>`).join('')
      +`</div>`;
    c.append(split);
  }

  const tokChart=el('div');
  tokChart.innerHTML=`<div class="section-label">Tokens per day (14d)</div>`+barChart(stats.per_day, d=>d.tokens, fmtTok);
  c.append(tokChart);

  const runChart=el('div');
  runChart.innerHTML=`<div class="section-label">Runs per day (14d)</div>`+barChart(stats.per_day, d=>d.runs, v=>String(v));
  c.append(runChart);

  const costChart=el('div');
  costChart.innerHTML=`<div class="section-label">API-equiv. cost per day (14d)</div>`+barChart(stats.per_day, d=>d.cost_usd, fmtCost);
  c.append(costChart);

  c.append(el('p','hint','Counts only what claudeq\'s own runs consumed (tokens recorded per run) — not your overall Claude account usage. Cost is the API-equivalent price Claude Code reports (what these requests would cost at pay-as-you-go API rates); on a subscription you\'re not billed for it — it just draws on your usage limit.'));
}
// Renders a 14-day bar chart. valueOf(day)->number; fmt(value)->tooltip label.
function barChart(days, valueOf, fmt){
  const max=Math.max(1,...days.map(valueOf));
  const cols=`repeat(${days.length},minmax(0,1fr))`;
  const bars=days.map(d=>{
    const v=valueOf(d);
    if(!v) return '<div></div>';   // empty day: no bar, no tooltip
    const h=Math.max(2,Math.round((v/max)*74));
    return `<div class="bar" data-tip="${esc(fmt(v)+' · '+d.date)}" style="height:${h}px"></div>`;
  }).join('');
  const labels=days.map(d=>`<div class="xl">${esc(d.date.slice(5))}</div>`).join('');
  return `<div class="group"><div class="chart">
    <div class="plot" style="grid-template-columns:${cols}">${bars}</div>
    <div class="xlabels" style="grid-template-columns:${cols}">${labels}</div></div></div>`;
}

export const view={ title:'Usage', enter(){ loadUsage(); } };
