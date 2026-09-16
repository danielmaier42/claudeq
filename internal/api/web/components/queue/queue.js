import {invalidateRuns, loadRuns} from '../activity/activity.js';
import {current, select} from '../app-shell/app-shell.js';
import {showLog} from '../log-sheet/log-sheet.js';
import {BETA_PROVIDERS, setProviderSnapshot} from '../providers/providers.js';
import {LIMITED_UNTIL, setConn} from '../status/status.js';
import {openAdd, openEdit} from '../task-sheet/task-sheet.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, el, emptyState, esc} from '../../core/dom.js';
import {relTime, resumeTimeText} from '../../core/format.js';
import {EXPORT, IMPORT, PENCIL} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

// Hover text for a cron expression: when it runs next, and when it last ran
// (absent until the task has actually run once).
function cronTip(t){ const l=[];
  if(t.next_run) l.push('Next run: '+new Date(t.next_run).toLocaleString());
  l.push('Last run: '+(t.last_run?new Date(t.last_run).toLocaleString()+' ('+relTime(t.last_run)+')':'never'));
  return l.join('\n'); }

let tasksSig='', tasksOnlyActive=false;
function taskFilterSeg(){
  const seg=el('div','seg');
  const mk=(v,label,tip)=>{ const b=el('button',null,label); b.dataset.v=v; b.title=tip;
    b.classList.toggle('active',(v==='active')===tasksOnlyActive);
    b.onclick=()=>{ tasksOnlyActive=v==='active';
      seg.querySelectorAll('button').forEach(x=>x.classList.toggle('active',x===b)); loadTasks(); };
    return b; };
  seg.append(mk('all','All','Show every task, paused ones included'),
             mk('active','Active','Hide paused tasks'));
  return seg;
}
// See invalidateRuns: the next poll rebuilds the list instead of skipping it.
export function invalidateTasks(){ tasksSig=''; }

export async function loadTasks(){
  // The pause state rides along with the queue: it decides the banner and
  // whether Run now is offered at all, and both must be right on every poll.
  let tasks, paused=false;
  try{ const [t,s,ps]=await Promise.all([api('GET','/api/tasks'),api('GET','/api/settings'),api('GET','/api/providers')]);
    tasks=t; paused=!!s.paused; setConn(true);
    setProviderSnapshot({providers:ps||[],showBeta:s.beta_features,defaultDir:s.default_working_dir});
  }catch(e){ setConn(false); return; }
  tasks=tasks||[];
  const shown=tasksOnlyActive?tasks.filter(t=>t.enabled):tasks;
  const sig=JSON.stringify([paused,tasksOnlyActive,tasks.length,LIMITED_UNTIL&&LIMITED_UNTIL.toISOString(),shown.map(t=>[t.id,t.name,t.trigger,t.enabled,t.parallel,t.permissions,t.notify_on_result,t.quiet_history,t.fixed_at,t.cron,t.next_run,t.last_run,t.running,t.waiting_for_limit,t.blocked_reason,t.provider,(t.waiting_for||[]).join(',')])]);
  if(sig===tasksSig && $('#tasks').childElementCount) return;   // avoid flicker on poll
  tasksSig=sig;
  const c=$('#tasks'); c.innerHTML='';
  if(paused) c.append(pauseBanner());
  if(!tasks.length){ c.append(emptyState('No tasks yet','Create your first task to queue work for Claude.')); return; }
  if(!shown.length){ c.append(emptyState('No active tasks','All '+tasks.length+' task'+(tasks.length!==1?'s are':' is')+' paused — switch the filter to All to see them.')); return; }
  const g=el('div','group');
  shown.forEach((t,i)=>{
    let when;
    if(t.trigger==='fixed') when=esc(new Date(t.fixed_at).toLocaleString());
    else if(t.trigger==='cron'){ const tip=cronTip(t); when=`<span class="cron"${tip?` data-tip="${esc(tip)}"`:''}>${esc(t.cron)}</span>`; }
    else when='as soon as possible';
    const row=el('div','row');
    const tags=[];
    if(BETA_PROVIDERS.has(t.provider||'')) tags.push('<span class="chip beta" title="This task runs on a provider that is still in beta">beta</span>');
    if(t.blocked_reason) tags.push(`<span class="chip danger" title="${esc(t.blocked_reason+' The task keeps its place and starts by itself once the provider works again.')}">blocked</span>`);
    if(t.waiting_for_limit) tags.push(`<span class="chip warn" title="${esc('The rate limit interrupted this task. Its Claude session '+resumeTimeText(LIMITED_UNTIL)+' — drop the resume in Activity with “Cancel resume”.')}">rescheduled</span>`);
    // A job that waits for other jobs looks like one that never starts, unless
    // the queue says what it is waiting for.
    if((t.waiting_for||[]).length) tags.push(`<span class="chip" title="${esc('Starts by itself once these jobs have finished: '+t.waiting_for.join(', ')+'. It runs even if one of them fails.')}">waiting for ${t.waiting_for.length} job${t.waiting_for.length>1?'s':''}</span>`);
    if(t.parallel) tags.push('<span class="chip" title="Runs alongside other parallel tasks">parallel</span>');
    if(t.permissions==='skip') tags.push('<span class="chip warn" title="Skips permission prompts">granted</span>');
    if(t.notify_on_result) tags.push('<span class="chip accent" title="Sends outcome and last message when it finishes">notifies</span>');
    if(t.quiet_history) tags.push('<span class="chip" title="Successful runs stay out of Activity; failures are kept">silent</span>');
    const grow=el('div','grow'), name=esc(t.name); grow.innerHTML=`<div class="title task-title"><span class="name" data-tip="${name}">${name}</span> ${t.running?'<span class="chip running">running…</span>':''}</div>
      <div class="sub"><span class="mono">${esc(t.id)}</span> · ${esc(t.trigger)} · ${when}</div>`
      + (tags.length?`<div class="task-tags">${tags.join('')}</div>`:'');
    row.append(grow);
    const up=el('button','btn small','↑'); up.disabled=i===0; up.title='Move up';
    up.onclick=()=>move(t.id,tasks.indexOf(shown[i-1]));
    const run=el('button','btn small','Run now'); run.disabled=!!t.running||paused;
    run.title=t.running?'Already running':(paused?'All runs are paused':'Run now'); run.onclick=()=>runNow(t.id);
    const edit=el('button','btn small iconly',PENCIL); edit.title='Edit'; edit.onclick=()=>openEdit(t);
    const exp=el('button','btn small iconly',EXPORT); exp.title='Export as .claudeq file'; exp.onclick=()=>exportTask(t,exp);
    const sw=el('label','switch'); sw.innerHTML=`<input type="checkbox" ${t.enabled?'checked':''}><span class="sl"></span>`;
    sw.querySelector('input').onchange=()=>toggle(t.id,t.enabled);
    const del=el('button','btn small danger','✕'); del.title='Delete'; del.onclick=()=>del_(t.id,t.name);
    row.append(up,run,edit,exp,sw,del);
    g.append(row);
  });
  c.append(g);
}
// The yellow banner above the queue while the global pause switch is on: the
// queue looks idle either way, so the reason has to be visible where the tasks
// are, not only in Settings.
function pauseBanner(){
  const d=el('div','warnbar');
  d.innerHTML='<b>⏸ All runs are paused</b>Nothing starts — not a task that comes due, not <strong>Run now</strong> — until you switch this off. A run already in flight keeps going.'
    +'<div class="upd-actions"><button class="btn" id="pauseResumeBtn">Resume runs</button></div>';
  d.querySelector('#pauseResumeBtn').onclick=()=>setPaused(false);
  return d;
}
// Writes the pause switch on its own endpoint (not the settings payload), so it
// applies at once and carries nothing else with it.
export async function setPaused(on){
  try{ await api('POST','/api/pause',{paused:on}); }
  catch(e){ toast(e.message,'err');
    // The write failed, so the queue is still in the old state: put the switch
    // back rather than leaving it claiming something that never happened.
    const sw=$('#s-paused'); if(sw) sw.checked=!on;
    tasksSig=''; if(current==='tasks') loadTasks();
    return; }
  toast(on?'All runs paused':'Runs resumed','ok');
  const sw=$('#s-paused'); if(sw) sw.checked=on;
  tasksSig=''; if(current==='tasks') loadTasks();
}

/* ---- Sharing tasks as .claudeq files (zip: task.json + prompt.md) ---- */
async function exportTask(t,btn){
  btn.disabled=true; // one save panel at a time; the row re-renders on the next change anyway
  try{
    const r=await api('POST',`/api/tasks/${encodeURIComponent(t.id)}/export`);
    if(r&&r.path) toast('Exported to '+r.path,'ok'); // 204/empty => cancelled
  }catch(e){ toast('Export failed: '+e.message,'err'); }
  finally{ btn.disabled=false; }
}
async function runNow(id){
  try{ await api('POST',`/api/tasks/${id}/run-now`); }catch(e){ toast(e.message,'err'); return; }
  toast('Started');
  select('news');           // jump to Activity
  watchNewRun(id);          // find the fresh run and open its live log
}
// Poll briefly until the just-started run for this task appears, then open its
// (live-updating) log.
function watchNewRun(id){
  const deadline=Date.now()+12000;
  const tick=async()=>{
    let runs=[]; try{ runs=await api('GET','/api/runs'); }catch{}
    invalidateRuns(); loadRuns();
    const run=runs.find(r=>r.task_id===id); // newest-first, so first match is the latest
    if(run){ showLog(run); return; }
    if(Date.now()<deadline) setTimeout(tick,500);
  };
  setTimeout(tick,400);
}
async function toggle(id,on){ try{ await api('POST',`/api/tasks/${id}/${on?'disable':'enable'}`); toast(on?'Paused':'Enabled','ok'); loadTasks();}catch(e){toast(e.message,'err');loadTasks()} }
async function move(id,to){ try{ await api('POST',`/api/tasks/${id}/move?to=${to}`); loadTasks();}catch(e){toast(e.message,'err')} }
async function del_(id,name){ if(await confirmSheetAsk('Delete task “'+name+'”?')){ try{await api('DELETE',`/api/tasks/${id}`);toast('Deleted','ok');loadTasks();}catch(e){toast(e.message,'err')} } }

export const view={
  title:'Queue',
  toolbar(ta){
    ta.append(taskFilterSeg());
    const imp=el('button','btn',IMPORT+'<span>Import…</span>'); imp.title='Add a task from a .claudeq file'; imp.onclick=()=>$('#importFile').click(); ta.append(imp);
    const b=el('button','btn primary','+ New task'); b.onclick=openAdd; ta.append(b);
  },
  enter(){ loadTasks(); },
};
