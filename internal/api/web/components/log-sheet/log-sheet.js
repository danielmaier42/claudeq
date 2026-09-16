import {cancelResume, invalidateRuns, loadRuns, readRun} from '../activity/activity.js';
import {current} from '../app-shell/app-shell.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, el, esc} from '../../core/dom.js';
import {toast} from '../../core/toast.js';

let logRaw='', logMode='chat', logRunId='', logTimer=null, logPrev=null, logPrompt='', logTaskName='';
let logRun=null, logCancelMode='', logCanceled=false;
let logOpenTools=new Set(), logToolIdx=0; // remember expanded tool blocks across live re-renders
export async function showLog(run){ logRunId=run.run_id; logPrompt=(run.task&&run.task.prompt)||''; logTaskName=run.task_name; $('#logTitle').textContent='Log · '+run.task_name;
  logOpenTools=new Set(); logRun=run; logCanceled=false;
  setLogCancel(run);
  setLogContinueVisible(canContinue(run));
  logPrev=null; await refreshLog(true); $('#logSheet').showModal(); startLogPolling();
  readRun(run.run_id); }  // opening a run's log marks it read
// The footer's cancel button covers both ways a run can be called off: killing
// the process of a running one, and dropping the scheduled resume of one that
// is only waiting for the rate limit.
function setLogCancel(run){
  const btn=$('#logCancelBtn');
  logCancelMode = run&&run.status==='running' ? 'running' : (run&&run.resume_pending ? 'resume' : '');
  btn.textContent = logCancelMode==='resume' ? 'Cancel resume' : 'Cancel task';
  btn.hidden = !logCancelMode;
}
// A run can be continued interactively once it has finished for good (a
// rate-limited run is about to be resumed by the queue itself) and its Claude
// session + working directory are on record.
export function canContinue(r){ return !!(r && ['success','failed','auth_error','canceled'].includes(r.status) && r.session_id && r.task && r.task.working_dir); }
function setLogContinueVisible(on){ $('#logContinueBtn').hidden=!on; }
async function continueLogRun(){
  try{ await api('POST',`/api/runs/${logRunId}/continue`); toast('Opening Terminal…','ok'); }
  catch(e){ toast('Continue failed: '+e.message,'err'); }
}
async function cancelLogRun(){
  if(logCancelMode==='resume'){
    const run=logRun;
    if(!await cancelResume(run)) return;   // refused or failed: leave the button as it is
    logCanceled=true; setLogCancel(null);
    // The session is nobody's plan any more, so it can be picked up by hand.
    run.status='canceled'; run.resume_pending=false; setLogContinueVisible(canContinue(run));
    await refreshLog(true); return;
  }
  if(!await confirmSheetAsk('Cancel the running task “'+logTaskName+'”? Its Claude process is terminated.','Cancel task','Keep running')) return;
  try{ await api('POST',`/api/runs/${logRunId}/cancel`); logCanceled=true; setLogCancel(null); toast('Task canceled','ok'); }
  catch(e){ toast(e.message,'err'); }
  invalidateRuns(); await refreshLog(true); if(current==='news') loadRuns();
}
async function refreshLog(force){
  try{ logRaw=await api('GET',`/api/runs/${logRunId}/log`)||''; }catch(e){ logRaw=''; }
  if(!force && logRaw===logPrev) return;   // nothing new -> don't re-render (no flicker)
  logPrev=logRaw; renderLog();
  const box=$('#logBody'); box.scrollTop=box.scrollHeight; // fast-forward to the newest output
}
function startLogPolling(){ stopLogPolling(); logTimer=setInterval(async()=>{
  let runs=[]; try{ runs=await api('GET','/api/runs'); }catch{}
  const run=runs.find(r=>r.run_id===logRunId);
  // A run can go from running to "waiting for the rate limit", so the button
  // changes mode rather than only disappearing. Once the user has cancelled,
  // stop following the poll: a stale response must not offer the action again.
  if(run && !logCanceled){ logRun=run; setLogCancel(run); }
  // Continue is the mirror image: a terminal status is final, so a stale poll
  // can only ever re-affirm the same visibility.
  if(run) setLogContinueVisible(canContinue(run));
  await refreshLog(false);
  if(run && run.status!=='running'){ stopLogPolling(); invalidateRuns(); if(current==='news') loadRuns(); }
}, 1500); }
function stopLogPolling(){ if(logTimer){ clearInterval(logTimer); logTimer=null; } }
export function initLogSheet(){
  $('#logSheet').addEventListener('close',stopLogPolling);
  $('#logMode').querySelectorAll('button').forEach(b=>b.onclick=()=>{ logMode=b.dataset.v;
    $('#logMode').querySelectorAll('button').forEach(x=>x.classList.toggle('active',x.dataset.v===logMode)); renderLog(); });
  $('#logCancelBtn').onclick=cancelLogRun;
  $('#logContinueBtn').onclick=continueLogRun;
  $('#logDoneBtn').onclick=()=>$('#logSheet').close();
}
function renderLog(){
  const box=$('#logBody');
  if(logMode==='raw'){ box.className=''; box.innerHTML='<pre class="log">'+esc(logRaw||'(empty log)')+'</pre>'; return; }
  box.className='chat'; box.innerHTML='';
  logToolIdx=0;
  let shown=0;
  if(logPrompt && logPrompt.trim()){ box.append(el('div','msg user','<div class="who">You</div>'+esc(logPrompt))); shown++; }
  for(const raw of logRaw.split('\n')){ const line=raw.trim(); if(!line) continue;
    let ev; try{ ev=JSON.parse(line); }catch{ box.append(el('div','rawline',esc(line))); shown++; continue; }
    shown += appendLogEvent(box, ev);
  }
  if(!shown) box.append(el('div','rawline','(empty log)'));
}
const shortJSON=o=>{ let s; try{ s=JSON.stringify(o); }catch{ s=String(o); } return s.length>800?s.slice(0,800)+' …':s; };
function toolResultText(c){ if(typeof c==='string') return c; if(Array.isArray(c)) return c.map(x=>x&&x.text?x.text:(typeof x==='string'?x:JSON.stringify(x))).join('\n'); return c==null?'':JSON.stringify(c); }
// Collapsible tool block (collapsed by default). The log is append-only, so the
// sequential index is stable across re-renders and can key the expanded-state set.
function toolBlock(label, body){
  const idx=logToolIdx++;
  const d=el('details','tool','<summary><span class="tname">'+label+'</span></summary><pre>'+body+'</pre>');
  if(logOpenTools.has(idx)) d.open=true;
  d.addEventListener('toggle',()=>{ if(d.open) logOpenTools.add(idx); else logOpenTools.delete(idx); });
  return d;
}
function appendLogEvent(box, ev){
  if(ev.type==='assistant' && ev.message && Array.isArray(ev.message.content)){
    let n=0;
    for(const b of ev.message.content){
      if(b.type==='text' && b.text && b.text.trim()){ const m=el('div','msg assistant','<div class="who">Claude</div>'+esc(b.text)); box.append(m); n++; }
      else if(b.type==='tool_use'){ box.append(toolBlock('🔧 '+esc(b.name||'tool'), esc(shortJSON(b.input)))); n++; }
    }
    return n;
  }
  if(ev.type==='user' && ev.message && Array.isArray(ev.message.content)){
    let n=0;
    for(const b of ev.message.content){
      if(b.type==='tool_result'){ const txt=toolResultText(b.content);
        box.append(toolBlock('↳ result', esc(txt.length>800?txt.slice(0,800)+' …':txt))); n++; }
      else if(b.type==='text' && b.text && b.text.trim()){ box.append(el('div','msg user','<div class="who">You</div>'+esc(b.text))); n++; }
    }
    return n;
  }
  if(ev.type==='user' && typeof ev.message?.content==='string' && ev.message.content.trim()){
    box.append(el('div','msg user','<div class="who">You</div>'+esc(ev.message.content))); return 1;
  }
  if(ev.type==='result'){ box.append(el('div','msg result','<div class="who">Result'+(ev.is_error?' · error':'')+'</div>'+esc(ev.result||'(no text)'))); return 1; }
  if(ev.type==='claudeq_status'){ const kind=ev.status==='note'?'note-msg':'err-msg'; box.append(el('div','msg '+kind,'<div class="who">'+esc((ev.status||'error').replace(/_/g,' '))+'</div>'+esc(ev.message||''))); return 1; }
  if(ev.type==='system' && ev.subtype==='init'){ box.append(el('div','rawline','session started'+(ev.model?' · '+esc(ev.model):''))); return 0; }
  // opencode's own JSONL shape — its adapter writes these events to the run
  // log verbatim, they never go through the Claude Code schema above.
  if(ev.type==='text' && ev.part){ const t=(ev.part.text||'').trim();
    if(t){ box.append(el('div','msg assistant','<div class="who">opencode</div>'+esc(t))); return 1; } return 0; }
  if(ev.type==='tool_use' && ev.part){ const st=ev.part.state||{};
    let body=shortJSON(st.input);
    if(st.output!=null){ const out=String(st.output); body+='\n\n→ '+(out.length>800?out.slice(0,800)+' …':out); }
    box.append(toolBlock('🔧 '+esc(ev.part.tool||'tool'), esc(body))); return 1; }
  if(ev.type==='step_start' || ev.type==='step_finish'){ return 0; }
  if(ev.type==='error' && ev.error){
    const msg=(ev.error.data&&ev.error.data.message)||ev.error.name||'';
    box.append(el('div','msg err-msg','<div class="who">error</div>'+esc(msg))); return 1; }
  // Codex's own JSONL shape — same situation as opencode above.
  if(ev.type==='thread.started'){ box.append(el('div','rawline','session started')); return 0; }
  if(ev.type==='item.started' || ev.type==='turn.started' || ev.type==='turn.completed'){ return 0; }
  if(ev.type==='item.completed' && ev.item){ const it=ev.item;
    if(it.type==='agent_message'){ const t=(it.text||'').trim();
      if(t){ box.append(el('div','msg assistant','<div class="who">Codex</div>'+esc(t))); return 1; } return 0; }
    if(it.type==='error'){ box.append(el('div','msg err-msg','<div class="who">error</div>'+esc(it.message||''))); return 1; }
    if(it.type==='command_execution'){ let body=it.command||'';
      if(it.aggregated_output) body+='\n\n'+it.aggregated_output;
      box.append(toolBlock('🔧 bash', esc(body.length>800?body.slice(0,800)+' …':body))); return 1; }
    box.append(toolBlock('🔧 '+esc(it.type||'item'), esc(shortJSON(it)))); return 1; }
  if(ev.type==='turn.failed'){ box.append(el('div','msg err-msg','<div class="who">error</div>'+esc((ev.error&&ev.error.message)||'turn failed'))); return 1; }
  return 0;
}

/* ---- Artifacts ---- */

// The sheet that shows one run as a chat or as its raw log.
const sheetTemplate = `
<dialog id="logSheet">
  <div class="sheet-hd"><b id="logTitle">Log</b><div class="spacer" style="flex:1"></div>
    <div class="seg" id="logMode"><button data-v="chat" class="active">Chat</button><button data-v="raw">Raw</button></div></div>
  <div class="sheet-bd"><div id="logBody"></div></div>
  <div class="sheet-ft"><button class="btn danger" id="logCancelBtn" hidden>Cancel task</button><button class="btn" id="logContinueBtn" hidden data-tip="Opens Terminal in the task's folder and resumes this chat interactively — with the full conversation context">Continue in Chat…</button><button class="btn" id="logDoneBtn">Done</button></div>
</dialog>
`;

export function mountLogSheet(){ document.body.insertAdjacentHTML('beforeend', sheetTemplate); }
