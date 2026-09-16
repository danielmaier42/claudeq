import {current} from '../app-shell/app-shell.js';
import {showLog} from '../log-sheet/log-sheet.js';
import {PROVIDERS, setProviderSnapshot} from '../providers/providers.js';
import {invalidateTasks, loadTasks} from '../queue/queue.js';
import {LIMITED_UNTIL, setConn} from '../status/status.js';
import {openReplay} from '../task-sheet/task-sheet.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, el, emptyState, esc} from '../../core/dom.js';
import {baseName, pad2, relTime, resumeText, runTimeTitle} from '../../core/format.js';
import {EYE, REPLAY} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

let runsSig='', actFrom='', actTo='', actPage=0; const ACT_PAGE=25;
function actLocalDate(iso){ const d=new Date(iso); return d.getFullYear()+'-'+pad2(d.getMonth()+1)+'-'+pad2(d.getDate()); }
function actInRange(iso){ const d=actLocalDate(iso); if(actFrom && d<actFrom) return false; if(actTo && d>actTo) return false; return true; }
// The next poll re-renders instead of matching its signature and skipping the
// work — what a caller means when it changed a run behind the list's back.
export function invalidateRuns(){ runsSig=''; }

export async function loadRuns(){
  let runs;
  try{
    // The provider list rides along: which harness a run used is only worth
    // saying once there is more than one it could have been.
    const [rs,ps]=await Promise.all([api('GET','/api/runs'),api('GET','/api/providers')]);
    runs=rs; if(ps) setProviderSnapshot({providers:ps}); setConn(true);
  }catch(e){ setConn(false); return; }
  const unread=runs.filter(r=>r.unread).length; const badge=$('#unreadCount'); badge.hidden=unread===0; badge.textContent=unread;
  const filtered=runs.filter(r=>actInRange(r.started_at));
  const pages=Math.max(1,Math.ceil(filtered.length/ACT_PAGE));
  if(actPage>pages-1) actPage=pages-1; if(actPage<0) actPage=0;
  const pageRuns=filtered.slice(actPage*ACT_PAGE, actPage*ACT_PAGE+ACT_PAGE);
  // Re-render only when the data or view actually changed, so the 5s poll doesn't
  // rebuild the DOM (and make the buttons flicker) on every tick.
  const sig=JSON.stringify([actFrom,actTo,actPage,LIMITED_UNTIL&&LIMITED_UNTIL.toISOString(),PROVIDERS.length,
    pageRuns.map(r=>[r.run_id,r.status,r.unread,r.resume_pending,r.resume_at,r.workflow_id])]);
  if(sig===runsSig && $('#news').childElementCount) return;
  runsSig=sig;
  const c=$('#news'); c.innerHTML='';
  if(!runs.length){ c.append(emptyState('No activity yet','Runs will appear here after tasks execute.')); return; }
  if(!filtered.length){ c.append(emptyState('No runs in this range','Adjust the date filter to see activity.')); return; }
  const list=el('div','act-list');
  // Runs that came out of one piece of work are shown together: a fan-out and
  // its join are one thing that happened, not four unrelated entries that
  // happen to sit near each other.
  renderActivity(list, pageRuns);
  c.append(list);

  // Footer: count + pager (newest-first, so "Newer" goes to lower page indices).
  const inRange=(actFrom||actTo)?' in range':'';
  const foot=el('div','act-foot');
  foot.append(el('span','sub',`${filtered.length} run${filtered.length!==1?'s':''}${inRange} · page ${actPage+1} of ${pages}`));
  const pager=el('div','pager');
  const prev=el('button','btn small','‹ Newer'); prev.disabled=actPage<=0; prev.onclick=()=>{actPage--;runsSig='';loadRuns();};
  const next=el('button','btn small','Older ›'); next.disabled=actPage>=pages-1; next.onclick=()=>{actPage++;runsSig='';loadRuns();};
  pager.append(prev,next); foot.append(pager);
  c.append(foot);
}

// renderActivity fills the list, grouping the runs of one workflow under a
// single header. A workflow with only one run on this page is not a group —
// every ordinary run has a workflow id, and boxing each one would be noise.
function renderActivity(list, runs){
  const byWorkflow=new Map();
  runs.forEach(r=>{ if(!r.workflow_id) return;
    byWorkflow.set(r.workflow_id,(byWorkflow.get(r.workflow_id)||[]).concat([r])); });
  const done=new Set();
  runs.forEach(r=>{
    if(done.has(r.run_id)) return;
    const group=r.workflow_id?byWorkflow.get(r.workflow_id):null;
    if(!group || group.length<2){ list.append(runLine(r)); return; }
    group.forEach(g=>done.add(g.run_id));
    list.append(workflowGroup(group));
  });
}

// workflowGroup boxes one workflow's runs, oldest first — the order the work
// actually happened in, which is the only order a fan-out and its join read in.
function workflowGroup(group){
  const box=el('div','wf-group');
  const ordered=group.slice().sort((a,b)=>new Date(a.started_at)-new Date(b.started_at));
  const unread=ordered.filter(r=>r.unread).length;
  const head=el('div','wf-head');
  head.innerHTML=`<span class="wf-label">Workflow</span><span class="sub">${ordered.length} runs · ${relTime(ordered[ordered.length-1].started_at)}</span>`
    +(unread?`<span class="chip accent">${unread} unread</span>`:'');
  box.append(head);
  ordered.forEach(r=>box.append(runLine(r,true)));
  return box;
}

// runLine renders one run's row. Inside a workflow box (inset) the unread dot
// and the mark-read eye move into the card itself, so the box keeps one even
// inset on every side instead of two gutters of different widths.
function runLine(r,inset){
  const line=el('div','act-line');
  const gl=el('div','act-gl'); if(r.unread) gl.append(el('div','unread-dot'));
  const card=el('div','act-card');
  const dir=r.task&&r.task.working_dir?baseName(r.task.working_dir):'';
  const grow=el('div','grow'); grow.innerHTML=`<div class="title">${esc(r.task_name)}</div><div class="sub"><span class="hint-time" data-tip="${esc(runTimeTitle(r))}">${relTime(r.started_at)}</span>${dir?' · <span class="mono">'+esc(dir)+'</span>':''}${runProviderText(r)}${r.resume_pending?' · <span class="resume">'+esc(resumeText(r))+'</span>':''}</div>`;
  const pill=el('span','pill '+r.status); pill.innerHTML=`<span class="d"></span>${esc(statusLabel(r))}`;
  const actions=el('div','row-actions');
  if(r.task){ const rp=el('button','btn small iconly',REPLAY); rp.title='Re-run this task'; rp.onclick=()=>openReplay(r.task); actions.append(rp); }
  const log=el('button','btn small','Log'); log.onclick=()=>showLog(r); actions.append(log);
  if(r.resume_pending){ const cx=el('button','btn small danger','Cancel resume');
    cx.title='Drop the scheduled resume so this task does not start again'; cx.onclick=()=>cancelResume(r); actions.append(cx); }
  card.append(grow,actions,pill);   // status pill right-aligned (last)
  const gr=el('div','act-gr'); if(r.unread){ const mr=el('button','eye-btn',EYE); mr.title='Mark read'; mr.onclick=()=>readRun(r.run_id); gr.append(mr); }
  if(inset){ card.prepend(gl); if(r.unread) actions.prepend(gr.firstChild); line.append(card); }
  else line.append(gl,card,gr);
  return line;
}
// runProviderText names the harness a run used, with the model it used there —
// what the run recorded, not what the task says today.
//
// It is left out while only one provider is configured: on every row it would
// be noise, and the answer to "which one?" is already "the only one".
function runProviderText(r){
  const p=r.provider;
  if(!p||!p.name||PROVIDERS.length<2) return '';
  return ' · '+esc(p.name)+(p.model?' <span class="mono">'+esc(p.model)+'</span>':'');
}

// A rate-limited run reads as "rescheduled" while its session is still queued
// to continue; once that plan is gone (resumed, canceled, task deleted) the
// pause is just what happened to this run.
function statusLabel(r){
  if(r.status==='rate_limited_waiting') return r.resume_pending?'rescheduled':'rate limited';
  return r.status.replace(/_/g,' ');
}
// Returns whether the resume was actually dropped, so a caller can keep its own
// state in step instead of assuming success.
export async function cancelResume(r){
  const q=!r.task
    ? 'Cancel the scheduled resume of “'+r.task_name+'”? The interrupted Claude session is dropped and the run is recorded as canceled.'
    : (r.task.trigger==='cron'
      ? 'Cancel the scheduled resume of “'+r.task_name+'”? The interrupted Claude session is dropped; the task keeps its schedule and runs again at its next occurrence.'
      : 'Cancel the scheduled resume of “'+r.task_name+'”? The interrupted Claude session is dropped and the task leaves the queue — it will not run again.');
  if(!await confirmSheetAsk(q,'Cancel resume','Keep it')) return false;
  let ok=true;
  try{ await api('POST',`/api/runs/${r.run_id}/cancel`); toast('Resume canceled','ok'); }
  catch(e){ ok=false; toast(e.message,'err'); }
  invalidateRuns(); invalidateTasks(); loadRuns(); if(current==='tasks') loadTasks();
  return ok;
}
export async function readRun(id){ try{ await api('POST',`/api/runs/${id}/read`); }catch{} runsSig=''; loadRuns(); }
async function markAllRead(){ try{await api('POST','/api/runs/read-all');toast('All marked read','ok'); runsSig=''; loadRuns();}catch(e){toast(e.message,'err')} }

export const view={
  title:'Activity',
  toolbar(ta){
    const mkDate=(val,on)=>{ const i=el('input','date-in'); i.type='date'; if(val)i.value=val; i.onchange=on; return i; };
    const from=mkDate(actFrom,e=>{actFrom=e.target.value;actPage=0;runsSig='';loadRuns();}); from.title='From date';
    const to=mkDate(actTo,e=>{actTo=e.target.value;actPage=0;runsSig='';loadRuns();}); to.title='To date';
    const range=el('div','date-range'); range.append(from, el('span','date-sep','–'), to); ta.append(range);
    const b=el('button','btn',EYE+'<span>Mark all read</span>'); b.onclick=markAllRead; ta.append(b);
  },
  enter(){ loadRuns(); },
};
