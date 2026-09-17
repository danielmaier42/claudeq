import {loadRuns} from '../activity/activity.js';
import {current} from '../app-shell/app-shell.js';
import {DEFAULT_WORKING_DIR, PROVIDERS, betaAllowed, setProviderSnapshot} from '../providers/providers.js';
import {loadTasks} from '../queue/queue.js';
import {taskReview} from '../review/review.js';
import {api} from '../../core/api.js';
import {$, esc} from '../../core/dom.js';
import {pad2} from '../../core/format.js';
import {modelOptionsFrom} from '../../core/models.js';
import {toast} from '../../core/toast.js';

let taskMode='add', taskEditId='';
function toLocalDT(iso){ const d=new Date(iso); return d.getFullYear()+'-'+pad2(d.getMonth()+1)+'-'+pad2(d.getDate())+'T'+pad2(d.getHours())+':'+pad2(d.getMinutes()); }
function prefillTask(t){
  $('#f-name').value=t.name||''; $('#f-prompt').value=t.prompt||''; $('#f-dir').value=t.working_dir||'';
  $('#f-parallel').checked=!!t.parallel; $('#f-skip').checked=(t.permissions==='skip'); $('#f-notify').checked=!!t.notify_on_result;
  $('#f-quiet').checked=!!t.quiet_history;
  fillProviderChoices(t.provider||'', t.model||'', t.reasoning_effort||'');
  setSeg(t.trigger||'asap');
  $('#f-at').value=(t.trigger==='fixed'&&t.fixed_at)?toLocalDT(t.fixed_at):'';
  $('#f-cron').value=t.trigger==='cron'?(t.cron||''):'';
}
// REASONING_LEVELS are what a harness that takes a reasoning effort accepts.
// The list is claudeq's, not a provider's: it is a suggestion in a picker, and a
// value a provider does not know is refused by the provider, not here.
const REASONING_LEVELS=['low','medium','high','xhigh'];

// fillProviderChoices renders the provider, model and reasoning-effort fields
// together, because they belong together: the model list comes from the provider
// the task will run on, and changing the provider must not carry a model from
// the old one into the new one.
//
// Only providers that can actually run are offered. A task already pointed at
// one that cannot keeps it in the list, marked — an existing task stays visible
// with the reason it is blocked rather than being quietly moved.
async function fillProviderChoices(providerID, model, effort){
  const sel=$('#f-provider');
  if(!PROVIDERS.length){
    try{
      const [ps,st]=await Promise.all([api('GET','/api/providers'),api('GET','/api/settings')]);
      setProviderSnapshot({providers:ps||[],showBeta:st.beta_features});
    }catch{}
  }
  const offered=PROVIDERS.filter(p=>p.enabled&&p.health&&p.health.state==='ready'&&(!p.beta||betaAllowed()))
    .sort((a,b)=>(a.name||a.id).localeCompare(b.name||b.id));
  const picked=PROVIDERS.find(p=>p.id===providerID);
  if(providerID && picked && !offered.some(p=>p.id===providerID)) offered.push(picked);
  const dflt=PROVIDERS.find(p=>p.default);
  sel.innerHTML=`<option value="">${esc(dflt?`Default (${dflt.name||dflt.id})`:'Default provider')}</option>`
    +offered.map(p=>{
      const ready=p.health&&p.health.state==='ready';
      const label=(p.name||p.id)+(p.beta?' (beta)':'')+(ready?'':' — '+(p.health?p.health.state.replace(/_/g,' '):'unavailable'));
      return `<option value="${esc(p.id)}"${p.id===providerID?' selected':''}>${esc(label)}</option>`;
    }).join('')
    +(providerID && !picked?`<option value="${esc(providerID)}" selected>${esc(providerID)} — not configured</option>`:'');
  sel.onchange=()=>fillTaskModels(sel.value,'');
  await fillTaskModels(providerID, model, effort);
}
// fillTaskModels asks the chosen provider for its models and shows the
// reasoning-effort field only where the harness takes one.
async function fillTaskModels(providerID, model, effort){
  let models=[];
  try{ models=await api('GET','/api/models'+(providerID?'?provider='+encodeURIComponent(providerID):''))||[]; }catch{}
  $('#f-model').innerHTML=modelOptionsFrom(models,model||'','Provider default');
  const chosen=PROVIDERS.find(p=>p.id===(providerID||(PROVIDERS.find(x=>x.default)||{}).id));
  const takesEffort=!!chosen && !!chosen.reasoning_effort;
  $('#f-reasoning-wrap').hidden=!takesEffort;
  if(takesEffort){
    $('#f-reasoning').innerHTML=`<option value="">Provider default</option>`
      +REASONING_LEVELS.map(l=>`<option value="${l}"${l===effort?' selected':''}>${l}</option>`).join('')
      +(effort&&!REASONING_LEVELS.includes(effort)?`<option value="${esc(effort)}" selected>${esc(effort)} (custom)</option>`:'');
  }
}

function openSheet(title,submitLabel){ $('#addErr').textContent=''; $('#f-dir-hint').hidden=true; $('#f-dir-hint').textContent='';
  $('#f-provider-hint').hidden=true; $('#f-provider-hint').textContent='';
  clearTimeout(cronTimer); cronVerdict.expr=null; showCronStatus(null); cronRecheck();
  $('#addSheetTitle').textContent=title; $('#addSubmitBtn').textContent=submitLabel; $('#addSheet').showModal();
  taskReview.restore(); }   // opening a sheet shows an earlier finding about this prompt; a new review is worth Claude usage only once the prompt changes
export function openAdd(){ taskMode='add'; taskEditId='';
  ['f-name','f-prompt','f-dir','f-at','f-cron'].forEach(x=>$('#'+x).value='');
  $('#f-dir').value=DEFAULT_WORKING_DIR;
  $('#f-parallel').checked=false; $('#f-skip').checked=false; $('#f-notify').checked=false; $('#f-quiet').checked=false; setSeg('asap');
  fillProviderChoices('','','');
  openSheet('New task','Add task'); }
export async function openEdit(t){ taskMode='edit'; taskEditId=t.id;
  // Re-fetch the latest saved task: loadTasks skips re-rendering when only
  // non-summary fields (folder, prompt, model, permissions, notify) change, so
  // the row's cached task object can be stale — prefilling it would show an old
  // working directory even though the save succeeded.
  try{ const all=await api('GET','/api/tasks'); const fresh=(all||[]).find(x=>x.id===t.id); if(fresh) t=fresh; }catch(e){}
  prefillTask(t); openSheet('Edit task','Save changes'); }
export function openReplay(t){ if(!t){ toast('No saved definition to replay','err'); return; } taskMode='add'; taskEditId=''; prefillTask(t); openSheet('Replay task','Schedule again'); }
function setSeg(v){ $('#f-trigger').querySelectorAll('button').forEach(b=>b.classList.toggle('active',b.dataset.v===v));
  $('#f-trigger').dataset.value=v; $('#f-at-wrap').hidden=v!=='fixed'; $('#f-cron-wrap').hidden=v!=='cron';
  if(v==='cron') cronRecheck(); }

/* ---- Cron field: checked while it is typed, not only when the task is saved ----
   The daemon owns the verdict (same parser that will run the schedule), so the
   sheet asks it instead of re-implementing crontab rules here. An answer that a
   later keystroke has overtaken is dropped, and the last verdict is cached so
   saving does not ask again for text that was just checked. */
const cronVerdict={expr:null,valid:false,error:'',next:[]};
let cronSeq=0, cronTimer=0;
async function cronValidate(expr){
  if(cronVerdict.expr===expr) return cronVerdict;
  const seq=++cronSeq;
  let r;
  // No answer from the daemon: don't block the save on a guess — the POST/PUT
  // validates the expression again server-side and reports it there.
  try{ r=await api('GET','/api/cron/check?expr='+encodeURIComponent(expr)); }
  catch(e){ return {expr,valid:false,error:'',next:[],unchecked:true}; }
  const v={expr,valid:!!r.valid,error:r.error||'',next:r.next||[]};
  if(seq===cronSeq) Object.assign(cronVerdict,v);
  return v;
}
// force shows the verdict even for an empty field, which is silent while typing
// (an untouched field is not an error) but must speak up on save.
function showCronStatus(v,force){
  const box=$('#f-cron-status'), inp=$('#f-cron');
  const off=!v||v.unchecked||v.expr!==inp.value.trim()||(v.expr===''&&!force);
  inp.classList.toggle('bad',!off&&!v.valid);
  if(off){ box.hidden=true; box.textContent=''; box.classList.remove('bad'); return; }
  box.hidden=false; box.classList.toggle('bad',!v.valid);
  box.textContent=v.valid
    ? 'Runs next: '+v.next.map(x=>new Date(x).toLocaleString()).join(' · ')
    : v.error;
}
async function cronRecheck(){
  const expr=$('#f-cron').value.trim();
  if(!expr){ showCronStatus(null); return; }
  showCronStatus(await cronValidate(expr));
}
async function submitTask(){
  $('#addErr').textContent='';
  const trig=$('#f-trigger').dataset.value||'asap';
  const t={name:$('#f-name').value.trim(),prompt:$('#f-prompt').value,
    working_dir:$('#f-dir').value.trim(),trigger:trig,
    fixed_at: trig==='fixed'&&$('#f-at').value?new Date($('#f-at').value).toISOString():undefined,
    cron: trig==='cron'?$('#f-cron').value.trim():undefined, model:$('#f-model').value||undefined,
    parallel:$('#f-parallel').checked, enabled:true, permissions:$('#f-skip').checked?'skip':'default',
    notify_on_result:$('#f-notify').checked, quiet_history:$('#f-quiet').checked,
    provider:$('#f-provider').value||undefined,
    reasoning_effort:$('#f-reasoning-wrap').hidden?undefined:($('#f-reasoning').value||undefined)};
  if(!t.working_dir){ $('#addErr').textContent='Please choose a working directory.'; return; }
  if(trig==='cron'){
    // The reason stays on the field itself — repeating it at the footer would say
    // the same thing twice; the field is scrolled into view instead.
    const v=await cronValidate(t.cron||'');
    showCronStatus(v,true);
    if(!v.valid && !v.unchecked){ $('#f-cron').focus(); $('#f-cron').scrollIntoView({block:'center'}); return; }
  }
  try{
    if(taskMode==='edit'){ await api('PUT','/api/tasks/'+encodeURIComponent(taskEditId),t); toast('Task updated','ok'); }
    else { await api('POST','/api/tasks',t); toast('Task added','ok'); }
    $('#addSheet').close(); loadTasks(); if(current==='news') loadRuns();
  }catch(e){ $('#addErr').textContent=e.message; }
}
/* ---- Folder picker (native macOS dialog) ---- */
async function chooseFolder(){
  try{
    const q=$('#f-dir').value?('?path='+encodeURIComponent($('#f-dir').value)):'';
    const r=await api('POST','/api/fs/choose'+q);
    if(r && typeof r!=='string' && r.path){ $('#f-dir').value=r.path; taskReview.run(); } // 204/empty => cancelled
  }catch(e){ toast('Folder dialog unavailable: '+e.message,'err'); }
}


// Importing does not queue anything by itself: the file is read, then its task
// opens in the sheet so the prompt and the paths — which come from whoever
// exported it — can be adjusted before the task is added.
function initImport(){
  $('#importFile').onchange=async e=>{
    const f=e.target.files[0]; e.target.value=''; if(!f) return;
    let d; try{ d=await api('POST','/api/tasks/import',f); }
    catch(err){ toast('Import failed: '+err.message,'err'); return; }
    taskMode='add'; taskEditId=''; prefillTask(d.task); openSheet('Import task','Add task');
    if(d.missing_working_dir){
      const h=$('#f-dir-hint');
      h.textContent='The file’s working directory ('+d.missing_working_dir+') does not exist on this Mac. Choose a folder.';
      h.hidden=false;
    }
    // The file says which harness the task was written for. When no single
    // provider here matches it, the choice is the importer's: ClaudeQ will not
    // put someone else's task on an account they did not pick.
    const ph=$('#f-provider-hint');
    ph.hidden=!d.unresolved_provider;
    if(d.unresolved_provider){
      ph.textContent='Written for '+d.unresolved_provider+', which does not match exactly one provider here. Choose where it should run.';
    }
  };
}

export function initTaskSheet(){
  $('#f-trigger').querySelectorAll('button').forEach(b=>b.onclick=()=>setSeg(b.dataset.v));
  $('#f-cron').oninput=()=>{ clearTimeout(cronTimer); cronTimer=setTimeout(cronRecheck,250); };
  $('#f-dir-btn').onclick=chooseFolder;
  $('#addCancelBtn').onclick=()=>$('#addSheet').close();
  $('#addSubmitBtn').onclick=submitTask;
  initImport();
}

// The sheet that adds, edits, replays or imports one task.
const sheetTemplate = `
<dialog id="addSheet">
  <div class="sheet-hd"><b id="addSheetTitle">New task</b></div>
  <div class="sheet-bd">
    <label class="fld">Name</label><input type="text" id="f-name" placeholder="Nightly build">
    <label class="fld">Prompt</label><textarea id="f-prompt" rows="10" placeholder="What should Claude do?"></textarea>
    <div class="ai-banner" id="f-review" hidden></div>
    <label class="fld">Working directory</label>
    <div class="dir-row"><input type="text" id="f-dir" placeholder="Choose a folder…" readonly>
      <button class="btn" id="f-dir-btn" title="Choose folder">📁</button></div>
    <div id="f-dir-hint" class="hint" style="color:var(--warn)" hidden></div>
    <label class="fld">Trigger</label>
    <div class="seg" id="f-trigger">
      <button data-v="asap" class="active">As soon as possible</button>
      <button data-v="fixed">Fixed time</button>
      <button data-v="cron">Recurring</button>
    </div>
    <div id="f-at-wrap" hidden><label class="fld">Earliest start</label><input type="datetime-local" id="f-at"></div>
    <div id="f-cron-wrap" hidden><label class="fld">Cron schedule</label><input type="text" id="f-cron" placeholder="0 20 * * *">
      <div id="f-cron-status" class="hint cron-status" hidden></div>
      <div class="hint cron-help">5 fields:
        <span data-tip="Minute of the hour (0–59). '*' = every minute, '*/15' = every 15 minutes, '0' = on the hour.">minute</span>
        <span data-tip="Hour of the day (0–23). '20' = 8 pm, '*' = every hour, '9-17' = 9 am through 5 pm.">hour</span>
        <span data-tip="Day of the month (1–31). '*' = every day, '1' = the 1st, '1,15' = 1st and 15th.">day</span>
        <span data-tip="Month (1–12). '*' = every month, '6' = June only.">month</span>
        <span data-tip="Day of the week (0–6, Sunday = 0). '*' = any day, '1-5' = Mon–Fri, '0,6' = weekends.">weekday</span>
        — <span class="mono">0 20 * * *</span> = daily at 20:00. Hover a field for details.</div>
    </div>
    <div class="form-row">
      <div><label class="fld">Provider</label><select id="f-provider"></select>
        <div class="hint" id="f-provider-hint" hidden style="color:var(--danger)"></div></div>
      <div><label class="fld">Model</label><select id="f-model"></select></div>
      <div id="f-reasoning-wrap" hidden><label class="fld">Reasoning effort</label><select id="f-reasoning"></select></div>
    </div>
    <div class="group" style="margin-top:14px">
      <div class="row"><div class="grow"><div class="title">Run in parallel</div><div class="sub">May run alongside other parallel tasks</div></div>
        <label class="switch"><input type="checkbox" id="f-parallel"><span class="sl"></span></label></div>
      <div class="row"><div class="grow"><div class="title">Skip permission prompts</div><div class="sub">Needed for unattended writes</div></div>
        <label class="switch"><input type="checkbox" id="f-skip"><span class="sl"></span></label></div>
      <div class="row"><div class="grow"><div class="title">Notify me with the result</div><div class="sub">Send outcome + last message when it finishes</div></div>
        <label class="switch"><input type="checkbox" id="f-notify"><span class="sl"></span></label></div>
      <div class="row"><div class="grow"><div class="title">Quiet history</div><div class="sub">Drop successful runs from Activity; failures are kept. For frequent watcher jobs</div></div>
        <label class="switch"><input type="checkbox" id="f-quiet"><span class="sl"></span></label></div>
    </div>
    <p id="addErr" class="hint" style="color:var(--danger)"></p>
  </div>
  <div class="sheet-ft"><button class="btn" id="addCancelBtn">Cancel</button><button class="btn primary" id="addSubmitBtn">Add task</button></div>
</dialog>
`;

export function mountTaskSheet(){
  document.body.insertAdjacentHTML('beforeend', sheetTemplate);
  document.body.insertAdjacentHTML('beforeend', '<input type="file" id="importFile" accept=".claudeq,application/zip" hidden>');
}
