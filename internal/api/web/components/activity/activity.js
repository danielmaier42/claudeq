import {current} from '../app-shell/app-shell.js';
import {showLog} from '../log-sheet/log-sheet.js';
import {PROVIDERS, setProviderSnapshot} from '../providers/providers.js';
import {invalidateTasks, loadTasks} from '../queue/queue.js';
import {LIMITED_UNTIL, setConn} from '../status/status.js';
import {openReplay} from '../task-sheet/task-sheet.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, dateRange, el, emptyState, esc} from '../../core/dom.js';
import {byLabel, fillSelect, filterBar, listOf, matchesWords, searchField, segFilter, selectFilter} from '../../core/filters.js';
import {baseName, inDateRange, relTime, resumeText, runTimeTitle} from '../../core/format.js';
import {EYE, REPLAY} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

let runsGen=0, runsSig='', actFrom='', actTo='', actPage=0; const ACT_PAGE=25;
// The other filters: unread only, the task's queue group, the task itself, the
// outcome, and the search. NO_GROUP stands for "no group": a real group name
// never starts with a space.
let actUnread=false, actGroup='', actTask='', actStatus='', actQuery=''; const NO_GROUP=' none';
let searchIn=null, unreadSeg=null, groupSel=null, taskSel=null, statusSel=null, listEl=null;
const groupKey=r=>(r.task&&r.task.group)||NO_GROUP;
// What the search looks through: the task, its folder, the harness and model,
// the group, the outcome and the error, if any. The outcome is in both of its
// spellings, so "rate limited" also finds a run the row calls "rescheduled".
const haystack=r=>[r.task_name,r.task&&r.task.working_dir,r.provider&&r.provider.name,r.provider&&r.provider.model,
  r.provider&&r.provider.pool,r.task&&r.task.group,statusLabel(r),r.status.replace(/_/g,' '),r.error].join('\n');
const shown=r=>inDateRange(r.started_at,actFrom,actTo)&&(!actUnread||r.unread)&&(!actGroup||groupKey(r)===actGroup)
  &&(!actTask||r.task_id===actTask)&&(!actStatus||r.status===actStatus)&&matchesWords(actQuery,haystack(r));
const anyFilter=()=>!!(actFrom||actTo||actUnread||actGroup||actTask||actStatus||actQuery);
function refilter(){ actPage=0; runsSig=''; loadRuns(); }
// Refill the menus from the runs there are: groups and tasks that occur, the
// outcomes that occur. The task menu only offers tasks in the chosen group,
// and a choice that no longer matches anything falls back to "all", so the
// list can never be filtered to nothing by a stale selection.
function fillFilters(runs){
  const groups=new Map(), tasks=new Map(), statuses=new Map();
  runs.forEach(r=>{ groups.set(groupKey(r),(r.task&&r.task.group)||'No group');
    if(r.task_id&&(!actGroup||groupKey(r)===actGroup)&&!tasks.has(r.task_id)) tasks.set(r.task_id,r.task_name);
    statuses.set(r.status,statusLabel(r)==='rescheduled'?'rate limited':statusLabel(r)); });
  if(actGroup&&!groups.has(actGroup)) actGroup='';
  if(actTask&&!tasks.has(actTask)) actTask='';
  if(actStatus&&!statuses.has(actStatus)) actStatus='';
  fillSelect(groupSel,'All groups',byLabel(groups,NO_GROUP),actGroup);
  fillSelect(taskSel,'All tasks',byLabel(tasks),actTask);
  fillSelect(statusSel,'All outcomes',byLabel(statuses),actStatus);
}
// The filter bar sits above the list; the toolbar keeps Mark all read, the
// one action that applies to everything. Built on every entry so the fields
// show the state.
function renderFilters(){
  const {list,row}=filterBar($('#news')); listEl=list;
  const top=row(), where=row();
  searchIn=searchField(actQuery,v=>{ if(v.trim()===actQuery.trim()) return; actQuery=v; refilter(); },
    'Search runs','Search by task, folder, harness, model, group, outcome or error (⌘F)');
  unreadSeg=segFilter([[false,'All','Show every run'],[true,'Unread','Show only the runs you have not looked at yet']],actUnread,v=>{ actUnread=v; refilter(); });
  top.append(searchIn,unreadSeg);
  groupSel=selectFilter('Show the runs of one queue group',v=>{ actGroup=v; actTask=''; refilter(); });
  taskSel=selectFilter('Show the runs of one task',v=>{ actTask=v; refilter(); });
  statusSel=selectFilter('Show the runs with one outcome',v=>{ actStatus=v; refilter(); });
  where.append(groupSel,taskSel,statusSel,el('span','spacer'),dateRange(actFrom,actTo, v=>{actFrom=v;refilter();}, v=>{actTo=v;refilter();}));
}
// ⌘F (Ctrl+F elsewhere) puts the cursor in the search field while the Log is
// showing and no sheet is open; otherwise the browser keeps its own find.
document.addEventListener('keydown',e=>{
  if(current!=='news'||document.querySelector('dialog[open]')||!(e.metaKey||e.ctrlKey)||e.key!=='f'||!searchIn||!searchIn.isConnected) return;
  e.preventDefault(); searchIn.focus(); searchIn.select(); });
// The next poll re-renders instead of matching its signature and skipping the
// work — what a caller means when it changed a run behind the list's back.
export function invalidateRuns(){ runsSig=''; }

export async function loadRuns(){
  const gen=++runsGen;
  let runs;
  try{
    // The provider list rides along: which harness a run used is only worth
    // saying once there is more than one it could have been.
    const [rs,ps]=await Promise.all([api('GET','/api/runs'),api('GET','/api/providers')]);
    runs=rs; if(ps) setProviderSnapshot({providers:ps}); setConn(true);
  }catch(e){ setConn(false); return; }
  if(gen!==runsGen) return;   // a newer load superseded us
  const unread=runs.filter(r=>r.unread).length; const badge=$('#unreadCount'); badge.hidden=unread===0; badge.textContent=unread;
  // With "Unread" the menus offer only groups, tasks and outcomes with something unread.
  fillFilters(actUnread?runs.filter(r=>r.unread):runs);
  const filtered=runs.filter(shown);
  const pages=Math.max(1,Math.ceil(filtered.length/ACT_PAGE));
  if(actPage>pages-1) actPage=pages-1; if(actPage<0) actPage=0;
  const pageRuns=filtered.slice(actPage*ACT_PAGE, actPage*ACT_PAGE+ACT_PAGE);
  // Re-render only when the data or view actually changed, so the 5s poll doesn't
  // rebuild the DOM (and make the buttons flicker) on every tick.
  if(!listEl) listEl=listOf($('#news'));
  const sig=JSON.stringify([actFrom,actTo,actUnread,actGroup,actTask,actStatus,actQuery,actPage,LIMITED_UNTIL&&LIMITED_UNTIL.toISOString(),PROVIDERS.length,
    pageRuns.map(r=>[r.run_id,r.status,r.unread,r.resume_pending,r.resume_at,r.workflow_id])]);
  if(sig===runsSig && listEl.childElementCount) return;
  runsSig=sig;
  const c=listEl; c.innerHTML='';
  if(!runs.length){ c.append(emptyState('Nothing logged yet','Runs will appear here after tasks execute.')); return; }
  if(!filtered.length){
    const why=actQuery?`Nothing matches “${actQuery.trim()}”. Try fewer or different words.`
      : actUnread?'Everything here has been looked at. Switch to All to see the rest.':'Adjust the date, group, task or outcome filter to see runs.';
    c.append(emptyState('No runs match',why)); return; }
  const list=el('div','act-list');
  // Runs that came out of one piece of work are shown together: a fan-out and
  // its join are one thing that happened, not four unrelated entries that
  // happen to sit near each other.
  renderActivity(list, pageRuns);
  c.append(list);

  // Footer: count + pager (newest-first, so "Newer" goes to lower page indices).
  const inRange=anyFilter()?' matching':'';
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
  ordered.forEach(r=>box.append(runLine(r)));
  return box;
}

// runLine renders one run's row. The unread dot and the mark-read eye sit
// inside the card, not in gutters beside it, so a row is exactly as wide as a
// Queue group or a Usage card and the whole dashboard keeps one left and one
// right edge.
function runLine(r){
  const line=el('div','act-line');
  const gl=el('div','act-gl'); if(r.unread) gl.append(el('div','unread-dot'));
  const card=el('div','act-card');
  const dir=r.task&&r.task.working_dir?baseName(r.task.working_dir):'';
  const grow=el('div','grow'); grow.innerHTML=`<div class="title">${esc(r.task_name)}</div><div class="sub"><span class="hint-time" data-tip="${esc(runTimeTitle(r))}">${relTime(r.started_at)}</span>${dir?' · <span class="mono">'+esc(dir)+'</span>':''}${runProviderText(r)}${r.resume_pending?' · <span class="resume">'+esc(resumeText(r))+'</span>':''}</div>`;
  const pill=el('span','pill '+r.status); pill.innerHTML=`<span class="d"></span>${esc(statusLabel(r))}`;
  const actions=el('div','row-actions');
  if(r.task){ const rp=el('button','btn small iconly',REPLAY); rp.title='Re-run this task'; rp.onclick=()=>replay(r); actions.append(rp); }
  const log=el('button','btn small','Log'); log.onclick=()=>showLog(r); actions.append(log);
  if(r.resume_pending){ const cx=el('button','btn small danger','Cancel resume');
    cx.title='Drop the scheduled resume so this task does not start again'; cx.onclick=()=>cancelResume(r); actions.append(cx); }
  card.append(grow,actions,pill);   // status pill right-aligned (last)
  if(r.unread){ const mr=el('button','eye-btn',EYE); mr.title='Mark read'; mr.onclick=()=>readRun(r.run_id); actions.prepend(mr); }
  card.prepend(gl);
  line.append(card);
  return line;
}
// Replaying a run needs its prompt, which the list does not carry (it is the
// bulk of it, and no row shows it), so the one run is fetched for it. An
// unanswered fetch says so instead of opening a sheet that looks complete and
// has no prompt in it.
async function replay(r){
  let full;
  try{ full=await api('GET',`/api/runs/${r.run_id}`); }
  catch(e){ toast('Could not read this run: '+e.message,'err'); return; }
  openReplay((full&&full.task)||r.task);
}

// runProviderText names the harness a run used, with the model it used there —
// what the run recorded, not what the task says today.
//
// It is left out while only one provider is configured: on every row it would
// be noise, and the answer to "which one?" is already "the only one".
function runProviderText(r){
  const p=r.provider;
  if(!p||!p.name) return '';
  // A script run always says so: that it ran without a model is the one thing
  // that sets it apart from the agent runs around it.
  if(p.kind!=='script'&&PROVIDERS.length<2) return '';
  return ' · '+esc(p.name)+(p.model?' <span class="mono">'+esc(p.model)+'</span>':'')
    +(p.pool?' <span class="sub">via pool '+esc(p.pool)+'</span>':'');
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
  title:'Log',
  toolbar(ta){
    const b=el('button','btn',EYE+'<span>Mark all read</span>'); b.onclick=markAllRead; ta.append(b);
  },
  enter(){ renderFilters(); invalidateRuns(); loadRuns(); },
  refresh(){ invalidateRuns(); loadRuns(); },
};
