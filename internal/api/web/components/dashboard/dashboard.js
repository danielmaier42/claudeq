import {loadPoolList} from '../pools/pools.js';
import {setConn} from '../status/status.js';
import {api} from '../../core/api.js';
import {$, el, emptyState, esc} from '../../core/dom.js';
import {relTime} from '../../core/format.js';
import {toast} from '../../core/toast.js';

/* ---- Dashboard ----
   The state of things at a glance: how much of each provider's allowance is
   left, how urgent it is to spend it, and the threshold above which backfill
   tasks do. The daemon reads the allowance without spending any — every
   quarter of an hour, after every run, and whenever this page opens or View >
   Refresh asks — and the background poll only ever reads what it remembered. */

// A window shows what is left of it, so the bar empties as it is used. It is
// coloured before it runs dry: at 0% left the provider's tasks already wait,
// and the point is to see it coming.
function left(w){ return Math.min(100,Math.max(0,100-w.used_percent)); }
function level(left){ return left<=10?'danger':left<=25?'warn':''; }
function pct(n){ return Math.round(n)+'% left'; }
function resetText(w){
  if(!w.resets_at) return '';
  const at=new Date(w.resets_at);
  if(at<=new Date()) return 'resets now';
  const day=at.toLocaleDateString(undefined,{weekday:'short'}), time=at.toLocaleTimeString(undefined,{hour:'2-digit',minute:'2-digit'});
  return 'resets '+relTime(w.resets_at)+' · '+day+' '+time;
}

// limitWindowsHTML draws a provider's windows as meters. Settings uses it too,
// so a window looks the same wherever it is shown.
export function limitWindowsHTML(l){
  return `<div class="limit-windows">${(l.windows||[]).map(w=>`
    <div class="limit-window w-${esc(w.id)}">
      <div class="limit-head"><span>${esc(w.label)}</span><b class="${level(left(w))}">${pct(left(w))}</b></div>
      <div class="meter ${level(left(w))}"><span style="width:${left(w)}%"></span></div>
      <div class="sub">${esc(resetText(w))}</div>
    </div>`).join('')}</div>`;
}

// limitStatusText is the line under a provider's name: when the figures are
// from, or why there are none.
export function limitStatusText(l){
  switch(l.state){
    case 'ok': return 'Updated '+relTime(l.updated_at);
    case 'unsupported': return l.reason||'No limit';
    case 'disabled': return 'Switched off';
  }
  const when=l.updated_at?' Showing the reading from '+relTime(l.updated_at)+'.':'';
  return (l.reason||'Could not be read.')+when;
}

// urgencyText is a provider's urgency as the row under its name says it.
function urgencyText(u){
  if(!u) return '';
  if(u.tier>0) return 'Urgency – '+u.note;
  return 'Urgency '+u.urgency.toFixed(1)+(u.running?' · shared by '+u.running+' run'+(u.running>1?'s':''):'');
}

// The backfill threshold is a slider on this page, so the rows it lights up
// are next to it. It applies on release, without Save, and the poll leaves the
// page alone while it is held, or the redraw would pull it out from under the
// pointer.
let threshold=1.5, sliding=false;
function backfillRow(){
  const row=el('div','row backfill-row');
  row.innerHTML=`<div class="grow"><div class="title">Backfill threshold</div>
      <div class="sub multi">Backfill tasks run on a provider whose urgency is above this. Urgency 1 means spending evenly from now on uses up the week; 2 means half of what is left would expire unused.</div></div>
    <div class="backfill-slider"><input type="range" id="d-backfill" min="0.5" max="5" step="0.1" value="${threshold}"><b id="d-backfill-val">${threshold.toFixed(1)}</b></div>`;
  const input=row.querySelector('input'), val=row.querySelector('b');
  input.addEventListener('pointerdown',()=>{ sliding=true; });
  input.addEventListener('input',()=>{ sliding=true; val.textContent=Number(input.value).toFixed(1); });
  input.addEventListener('change',async()=>{
    try{ await api('POST','/api/backfill',{urgency:Number(input.value)}); }
    catch(e){ toast(e.message||'Could not save the threshold'); }
    sliding=false; sig=''; loadDashboard(false);
  });
  return row;
}

let gen=0, sig='';
export async function loadDashboard(fresh){
  const g=++gen;
  let list, pools, settings;
  try{
    // The pools rank on the readings the limits call just took, so they are
    // asked second.
    list=await api('GET','/api/limits'+(fresh?'?fresh=1':''))||[];
    [pools,settings]=await Promise.all([loadPoolList(),api('GET','/api/settings')]);
    setConn(true);
  }catch(e){ setConn(false); return; }
  if(g!==gen || sliding) return;   // a newer load superseded us, or the slider is held
  threshold=settings.backfill_urgency||1.5;
  // The poll redraws only on a change, so a hover is not lost every 5 seconds.
  // The minute is part of the change: "updated 3 min ago" and "resets in 2 h"
  // move on their own between two readings.
  const s=JSON.stringify([list,pools,threshold])+'@'+Math.floor(Date.now()/60000);
  if(s===sig && $('#dashboard').childElementCount) return;
  sig=s;
  const c=$('#dashboard'); c.innerHTML='';
  const shown=list.filter(p=>p.limits.state!=='disabled');
  if(!shown.length){ c.append(emptyState('No providers','Enable a provider in Settings → Providers to see its limits here.')); return; }
  const box=el('div');
  // A provider above the threshold is lit up: its allowance is at risk of
  // going unused, and backfill tasks may run on it right now.
  box.innerHTML=`<div class="section-label">Provider limits</div><div class="group">`
    +shown.map(p=>{
      const l=p.limits, problem=l.state==='unavailable';
      return `<div class="row limit-row${p.backfill?' backfill-on':''}">
        <div class="limit-name"><div class="title">${esc(p.name)} <span class="chip">${esc(p.type_name||'')}</span>${p.backfill?' <span class="chip ok" title="Its weekly allowance would otherwise go unused, so backfill tasks run on it">backfill</span>':''}</div>
          <div class="sub multi${problem?' limit-problem':''}">${esc(limitStatusText(l))}</div>
          ${p.urgency?`<div class="sub urgency">${esc(urgencyText(p.urgency))}</div>`:''}</div>
        ${(l.windows||[]).length?limitWindowsHTML(l):''}
      </div>`;
    }).join('')+`</div>`;
  box.querySelector('.group').prepend(backfillRow());
  c.append(box);
  if(pools.length) c.append(poolsSection(pools));
}

// poolsSection shows, per pool, where a run started now would go and how the
// members stand: the same order the daemon takes them in.
function poolsSection(pools){
  const box=el('div');
  box.innerHTML=`<div class="section-label">Pools</div><div class="group">`
    +pools.map(p=>{
      const next=(p.ranking||[]).find(r=>r.tier<3);
      return `<div class="row pool-row">
        <div class="limit-name"><div class="title">${esc(p.name)} <span class="chip">Pool</span> <span class="chip beta">beta</span></div>
          <div class="sub multi">${next?'Next run goes to <b>'+esc(next.name)+'</b>':'No member can take work right now'}</div></div>
        <ol class="pool-rank">${(p.ranking||[]).map(r=>`
          <li class="${r.tier>=2?'pool-back':''}"><span class="pool-member">${esc(r.name)}</span>
            <span class="sub">${esc(r.note)} · weight ${esc(String(Math.round(r.weight*100)/100))}×</span></li>`).join('')}</ol>
      </div>`;
    }).join('')+`</div>`;
  return box;
}

export const view={ title:'Dashboard', enter(){ loadDashboard(true); }, refresh(){ sig=''; loadDashboard(true); } };
