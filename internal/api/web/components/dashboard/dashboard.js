import {setConn} from '../status/status.js';
import {api} from '../../core/api.js';
import {$, el, emptyState, esc} from '../../core/dom.js';
import {relTime} from '../../core/format.js';

/* ---- Dashboard ----
   The state of things at a glance. For now that is how much of each provider's
   allowance is left: the daemon reads it without spending any — every quarter
   of an hour, after every run, and whenever this page opens or View > Refresh
   asks — and the background poll only ever reads what it remembered. */

// A window close to its end is coloured before it is full: at 100% the
// provider's tasks already wait, and the point is to see it coming.
function level(pct){ return pct>=90?'danger':pct>=75?'warn':''; }
function pct(n){ return Math.round(n)+'%'; }
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
      <div class="limit-head"><span>${esc(w.label)}</span><b class="${level(w.used_percent)}">${pct(w.used_percent)}</b></div>
      <div class="meter ${level(w.used_percent)}"><span style="width:${Math.min(100,Math.max(0,w.used_percent))}%"></span></div>
      <div class="sub">${esc(resetText(w))}</div>
    </div>`).join('')}</div>`;
}

// limitStatusText is the line under a provider's name: when the figures are
// from, or why there are none.
export function limitStatusText(l){
  switch(l.state){
    case 'ok': return 'Updated '+relTime(l.updated_at);
    case 'unsupported': return l.reason||'No allowance reported';
    case 'disabled': return 'Switched off';
  }
  const when=l.updated_at?' Showing the reading from '+relTime(l.updated_at)+'.':'';
  return (l.reason||'Could not be read.')+when;
}

let gen=0, sig='';
export async function loadDashboard(fresh){
  const g=++gen;
  let list; try{ list=await api('GET','/api/limits'+(fresh?'?fresh=1':''))||[]; setConn(true); }catch(e){ setConn(false); return; }
  if(g!==gen) return;   // a newer load superseded us
  // The poll redraws only on a change, so a hover is not lost every 5 seconds.
  // The minute is part of the change: "updated 3 min ago" and "resets in 2 h"
  // move on their own between two readings.
  const s=JSON.stringify(list)+'@'+Math.floor(Date.now()/60000);
  if(s===sig && $('#dashboard').childElementCount) return;
  sig=s;
  const c=$('#dashboard'); c.innerHTML='';
  const shown=list.filter(p=>p.limits.state!=='disabled');
  if(!shown.length){ c.append(emptyState('No providers','Enable a provider in Settings → Providers to see its limits here.')); return; }
  const box=el('div');
  box.innerHTML=`<div class="section-label">Provider limits</div><div class="group">`
    +shown.map(p=>{
      const l=p.limits, problem=l.state==='unavailable';
      return `<div class="row limit-row">
        <div class="limit-name"><div class="title">${esc(p.name)} <span class="chip">${esc(p.type_name||'')}</span></div>
          <div class="sub multi${problem?' limit-problem':''}">${esc(limitStatusText(l))}</div></div>
        ${(l.windows||[]).length?limitWindowsHTML(l):''}
      </div>`;
    }).join('')+`</div>`;
  c.append(box);
  c.append(el('p','hint','Read from each provider without using any of its allowance: when this page opens, on View → Refresh, every 15 minutes and after every run.'));
}

export const view={ title:'Dashboard', enter(){ loadDashboard(true); }, refresh(){ sig=''; loadDashboard(true); } };
