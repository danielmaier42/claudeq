import {invalidateRuns, loadRuns} from '../activity/activity.js';
import {current, select} from '../app-shell/app-shell.js';
import {showLog} from '../log-sheet/log-sheet.js';
import {BETA_PROVIDERS, PROVIDERS, setProviderSnapshot} from '../providers/providers.js';
import {LIMITED_UNTIL, setConn} from '../status/status.js';
import {openAdd, openEdit} from '../task-sheet/task-sheet.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk, promptSheetAsk} from '../../core/confirm.js';
import {$, el, emptyState, esc} from '../../core/dom.js';
import {relTime, resumeTimeText} from '../../core/format.js';
import {EXPORT, GRIP, IMPORT, PENCIL} from '../../core/icons.js';
import {modelLabel} from '../../core/models.js';
import {toast} from '../../core/toast.js';

// Hover text for a cron expression: when it runs next, and when it last ran
// (absent until the task has actually run once).
function cronTip(t){ const l=[];
  if(t.next_run) l.push('Next run: '+new Date(t.next_run).toLocaleString());
  l.push('Last run: '+(t.last_run?new Date(t.last_run).toLocaleString()+' ('+relTime(t.last_run)+')':'never'));
  return l.join('\n'); }

// What a task actually runs on: the provider instance it names (or the default
// one) and the model that provider will use for it. The task's own id used to
// stand here, which nobody needs — the row is the task.
function runsOn(t){
  const p=PROVIDERS.find(x=>x.id===(t.provider||''))||PROVIDERS.find(x=>x.default);
  const who=p?(p.name||p.id):(t.provider||'default provider');
  const model=modelLabel(t.model||(p&&p.default_model)||'');
  return model?who+' · '+model:who;
}

let tasksSig='', tasksOnlyActive=false;
// GROUPS maps a group name to whether its section is folded shut. The daemon
// owns it, so the fold survives a reload, a restart and the window closing.
let GROUPS={};
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
  // A rebuild in the middle of a drag would pull the row out from under the
  // cursor, so the poll waits until the drop has been written.
  if(dragging||draggingGroup) return;
  // The pause state rides along with the queue: it decides the banner and
  // whether Run now is offered at all, and both must be right on every poll.
  let tasks, paused=false;
  try{ const [t,s,ps,gs]=await Promise.all([api('GET','/api/tasks'),api('GET','/api/settings'),api('GET','/api/providers'),api('GET','/api/groups')]);
    tasks=t; paused=!!s.paused; setConn(true);
    GROUPS={}; (gs||[]).forEach(g=>{ GROUPS[g.name]=!!g.collapsed; });
    setProviderSnapshot({providers:ps||[],showBeta:s.beta_features,defaultDir:s.default_working_dir});
  }catch(e){ setConn(false); return; }
  tasks=tasks||[];
  const shown=tasksOnlyActive?tasks.filter(t=>t.enabled):tasks;
  const sig=JSON.stringify([paused,tasksOnlyActive,tasks.length,LIMITED_UNTIL&&LIMITED_UNTIL.toISOString(),GROUPS,shown.map(t=>[t.id,t.name,t.trigger,t.enabled,t.parallel,t.permissions,t.notify_on_result,t.quiet_history,t.fixed_at,t.cron,t.next_run,t.last_run,t.running,t.waiting_for_limit,t.blocked_reason,t.provider,t.model,t.group||'',(t.waiting_for||[]).join(',')])]);
  if(sig===tasksSig && $('#tasks').childElementCount) return;   // avoid flicker on poll
  tasksSig=sig;
  const c=$('#tasks'); c.innerHTML='';
  if(paused) c.append(pauseBanner());
  if(!tasks.length){ c.append(emptyState('No tasks yet','Create your first task to queue work for Claude.')); return; }
  if(!shown.length){ c.append(emptyState('No active tasks','All '+tasks.length+' task'+(tasks.length!==1?'s are':' is')+' paused — switch the filter to All to see them.')); return; }
  // Order is priority, and a group is a run of tasks that share a name: the
  // queue renders the blocks in the order they first appear, so what the file
  // says and what the list shows are the same thing.
  ROW_ORDER=tasks.map(t=>t.id);
  const blocks=[], at={};
  shown.forEach(t=>{ const g=t.group||'';
    if(at[g]===undefined){ at[g]=blocks.length; blocks.push({group:g,tasks:[]}); }
    blocks[at[g]].tasks.push(t); });
  BLOCK_ORDER=blocks.map(b=>b.group);
  blocks.forEach(b=>c.append(groupBlock(b,tasks,paused)));
  // Dragging a task onto these makes it leave its group, or puts it in one that
  // did not exist a moment ago. They only take up room while a drag is on.
  if(at['']===undefined) c.append(dropZone('Drop here to take a task out of its group',t=>drop(t,'',tasks.length-1)));
  c.append(dropZone('Drop here to start a new group',newGroupWith));
}

// One section of the queue: a named group with a header that folds it, or the
// ungrouped tasks, which need no header at all.
function groupBlock(b,tasks,paused){
  const wrap=el('div','grp');
  wrap.dataset.group=b.group;
  const collapsed=!!(b.group&&GROUPS[b.group]);
  if(b.group){
    const hd=el('div','grp-hd'+(collapsed?' collapsed':''));
    hd.innerHTML=`<span class="grp-chevron">▸</span><span class="grp-name">${esc(b.group)}</span>`
      +`<span class="chip">${b.tasks.length}</span>`;
    hd.title=(collapsed?'Show':'Hide')+' the tasks in “'+b.group+'” — or drag the header to move the whole group';
    hd.onclick=()=>setCollapsed(b.group,!collapsed);
    // The header drags the section itself, so the groups can be put in the
    // order the work happens in.
    groupDragSource(hd,b.group);
    // A group takes a task dropped anywhere on its header, folded or not. The
    // task lands at the end of the group — behind its last row, which is not
    // the dragged one when that row is already in this group.
    dropTarget(hd,()=>({group:b.group,into:true,after:lastOther(b,dragging)}),
      ()=>!dragging||!lastOther(b,dragging));   // its own one-task group: nothing to do
    groupDropTarget(hd,b.group);
    wrap.append(hd);
  }
  const g=el('div','group'+(collapsed?' folded':''));
  b.tasks.forEach((t,i)=>g.append(taskRow(t,i,b,tasks,paused)));
  if(!b.group) groupDropTarget(g,'');   // a section can also be dropped around the ungrouped rows
  wrap.append(g);
  return wrap;
}

// lastOther names the last task of a block that is not the one being dragged —
// the row a dropped task goes behind.
function lastOther(block,t){
  for(let i=block.tasks.length-1;i>=0;i--){ if(!t||block.tasks[i].id!==t.id) return block.tasks[i].id; }
  return null;
}

function taskRow(t,i,block,tasks,paused){
  let when;
  if(t.trigger==='fixed') when=esc(new Date(t.fixed_at).toLocaleString());
  else if(t.trigger==='cron'){ const tip=cronTip(t); when=`<span class="cron"${tip?` data-tip="${esc(tip)}"`:''}>${esc(t.cron)}</span>`; }
  else when='as soon as possible';
  const row=el('div','row');
  row.dataset.id=t.id; row.dataset.group=t.group||'';
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
  const grip=el('span','grip',GRIP);
  grip.title='Drag to reorder, or onto a group to move it there';
  dragSource(grip,row,t);
  const grow=el('div','grow'), name=esc(t.name); grow.innerHTML=`<div class="title task-title"><span class="name" data-tip="${name}">${name}</span> ${t.running?'<span class="chip running">running…</span>':''}</div>
    <div class="sub">${esc(runsOn(t))} · ${esc(t.trigger)} · ${when}</div>`
    + (tags.length?`<div class="task-tags">${tags.join('')}</div>`:'');
  row.append(grip,grow);
  const up=el('button','btn small iconly','↑'); up.disabled=i===0;
  up.title=i===0?'Already first in its group — drag it to move it elsewhere':'Move up';
  up.onclick=()=>move(t.id,tasks.indexOf(block.tasks[i-1]));
  const run=el('button','btn small','Run now'); run.disabled=!!t.running||paused;
  run.title=t.running?'Already running':(paused?'All runs are paused':'Run now'); run.onclick=()=>runNow(t.id);
  const edit=el('button','btn small iconly',PENCIL); edit.title='Edit'; edit.onclick=()=>openEdit(t);
  const exp=el('button','btn small iconly',EXPORT); exp.title='Export as .claudeq file'; exp.onclick=()=>exportTask(t,exp);
  const sw=el('label','switch'); sw.innerHTML=`<input type="checkbox" ${t.enabled?'checked':''}><span class="sl"></span>`;
  sw.querySelector('input').onchange=()=>toggle(t.id,t.enabled);
  const del=el('button','btn small danger','✕'); del.title='Delete'; del.onclick=()=>del_(t.id,t.name);
  row.append(up,run,edit,exp,sw,del);
  // Dropping on the upper half of a row puts the dragged task above it, on the
  // lower half below it — the same as dragging a file between two others.
  dropTarget(row,e=>{
    const r=row.getBoundingClientRect(), above=e.clientY<r.top+r.height/2;
    return {group:row.dataset.group,before:above?t.id:null,after:above?null:t.id};
  },()=>dragging&&dragging.id===t.id);   // a row is never dropped on itself
  return row;
}

/* ---- Drag and drop ----
   The gesture writes one request: the task's group and its new position go to
   the daemon together, so a drop can never half-apply. Positions are expressed
   as "above this task" or "below that one", which the caller turns into the
   index the move endpoint wants: the index in the list with the dragged task
   already taken out. */

let dragging=null;        // the task being dragged, or null
let draggingGroup=null;   // the group whose header is being dragged, or null
// BLOCK_ORDER is the order the sections are rendered in ('' is the ungrouped
// one), which is what "put this group in front of that one" is expressed in.
let BLOCK_ORDER=[];

function dragSource(handle,row,t){
  handle.draggable=true;
  handle.addEventListener('dragstart',e=>{
    dragging=t;
    e.dataTransfer.effectAllowed='move';
    e.dataTransfer.setData('text/plain',t.id);
    // The handle alone would be a two-pixel ghost; drag the whole row instead.
    if(e.dataTransfer.setDragImage) e.dataTransfer.setDragImage(row,20,row.offsetHeight/2);
    row.classList.add('dragging');
    $('#tasks').classList.add('dragging');
  });
  handle.addEventListener('dragend',()=>{
    dragging=null; row.classList.remove('dragging');
    $('#tasks').classList.remove('dragging');
    clearMarks();
  });
}

function groupDragSource(hd,group){
  hd.draggable=true;
  hd.addEventListener('dragstart',e=>{
    draggingGroup=group;
    e.dataTransfer.effectAllowed='move';
    e.dataTransfer.setData('text/plain',group);
    hd.classList.add('dragging');
    $('#tasks').classList.add('dragging-group');
  });
  hd.addEventListener('dragend',()=>{
    draggingGroup=null; hd.classList.remove('dragging');
    $('#tasks').classList.remove('dragging-group');
    clearMarks();
  });
}

// groupDropTarget takes a dragged section: the upper half of a block puts the
// dragged group in front of it, the lower half behind it.
function groupDropTarget(elm,key){
  const where=e=>{
    const r=elm.getBoundingClientRect();
    if(e.clientY<r.top+r.height/2) return key;           // in front of this block
    const next=BLOCK_ORDER[BLOCK_ORDER.indexOf(key)+1];  // behind it = in front of the next
    return next===undefined?null:next;
  };
  elm.addEventListener('dragover',e=>{
    if(!draggingGroup||draggingGroup===key) return;
    e.preventDefault(); e.dataTransfer.dropEffect='move';
    const r=elm.getBoundingClientRect();
    elm.classList.remove('drop-above','drop-below');
    elm.classList.add(e.clientY<r.top+r.height/2?'drop-above':'drop-below');
  });
  elm.addEventListener('dragleave',()=>elm.classList.remove('drop-above','drop-below'));
  elm.addEventListener('drop',async e=>{
    if(!draggingGroup||draggingGroup===key) return;
    e.preventDefault(); e.stopPropagation();
    const name=draggingGroup, before=where(e);
    clearMarks();
    try{ await api('POST','/api/groups/move',before===null?{name}:{name,before}); }
    catch(err){ toast(err.message,'err'); }
    invalidateTasks(); loadTasks();
  });
}

function clearMarks(){
  document.querySelectorAll('.drop-into,.drop-above,.drop-below').forEach(x=>x.classList.remove('drop-into','drop-above','drop-below'));
}

// dropTarget wires an element to accept a dragged task. where(e) says where the
// task would land; the element shows that, and the drop writes it.
function dropTarget(elm,where,skip){
  const mark=e=>{
    if(!dragging||(skip&&skip())) return null;
    const w=where(e);
    elm.classList.remove('drop-into','drop-above','drop-below');
    elm.classList.add(w.into?'drop-into':(w.before?'drop-above':'drop-below'));
    return w;
  };
  elm.addEventListener('dragover',e=>{ if(!dragging||(skip&&skip())) return; e.preventDefault(); e.dataTransfer.dropEffect='move'; mark(e); });
  elm.addEventListener('dragleave',()=>elm.classList.remove('drop-into','drop-above','drop-below'));
  elm.addEventListener('drop',e=>{
    if(!dragging||(skip&&skip())) return;
    e.preventDefault(); e.stopPropagation();
    const w=where(e), t=dragging;
    elm.classList.remove('drop-into','drop-above','drop-below');
    drop(t,w.group,indexFor(t,w));
  });
}

// dropZone is a target with nothing in it: the dashed tile that takes a task
// out of every group, and the one that starts a new group.
function dropZone(label,onDrop){
  const z=el('div','dropzone',esc(label));
  z.addEventListener('dragover',e=>{ if(!dragging) return; e.preventDefault(); e.dataTransfer.dropEffect='move'; z.classList.add('drop-into'); });
  z.addEventListener('dragleave',()=>z.classList.remove('drop-into'));
  z.addEventListener('drop',e=>{ if(!dragging) return; e.preventDefault(); z.classList.remove('drop-into'); onDrop(dragging); });
  return z;
}

// indexFor turns "above a" / "below b" into the index the move endpoint takes:
// the position in the list once the dragged task has been removed from it.
function indexFor(t,w){
  const rest=ROW_ORDER.filter(id=>id!==t.id);
  if(w.before&&w.before!==t.id){ const i=rest.indexOf(w.before); return i<0?rest.length:i; }
  if(w.after&&w.after!==t.id){ const i=rest.indexOf(w.after); return i<0?rest.length:i+1; }
  return rest.length;
}
// ROW_ORDER is the full task order the list was built from — the unfiltered
// one, because a hidden paused task still occupies a position in the queue.
let ROW_ORDER=[];

async function drop(t,group,to){
  try{ await api('POST',`/api/tasks/${encodeURIComponent(t.id)}/move?to=${to}&group=${encodeURIComponent(group)}`); }
  catch(e){ toast(e.message,'err'); }
  invalidateTasks(); loadTasks();
}

// A drop on the "new group" tile asks for the name first — the group is made of
// the task that lands in it, so naming and moving are one step.
async function newGroupWith(t){
  const name=await promptSheetAsk('Name the new group',{placeholder:'e.g. Nightly sweeps',maxLength:60});
  if(!name) return;
  drop(t,name,ROW_ORDER.length-1);
}

async function setCollapsed(group,collapsed){
  GROUPS[group]=collapsed;
  try{ await api('POST','/api/groups/collapse',{name:group,collapsed}); }
  catch(e){ toast(e.message,'err'); }
  invalidateTasks(); loadTasks();
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
async function move(id,to){ try{ await api('POST',`/api/tasks/${id}/move?to=${to}`); invalidateTasks(); loadTasks();}catch(e){toast(e.message,'err')} }
async function del_(id,name){ if(await confirmSheetAsk('Delete task “'+name+'”?')){ try{await api('DELETE',`/api/tasks/${id}`);toast('Deleted','ok');invalidateTasks();loadTasks();}catch(e){toast(e.message,'err')} } }

export const view={
  title:'Queue',
  toolbar(ta){
    ta.append(taskFilterSeg());
    const imp=el('button','btn',IMPORT+'<span>Import…</span>'); imp.title='Add a task from a .claudeq file'; imp.onclick=()=>$('#importFile').click(); ta.append(imp);
    const b=el('button','btn primary','+ New task'); b.onclick=openAdd; ta.append(b);
  },
  enter(){ loadTasks(); },
};
