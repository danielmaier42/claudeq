import {select} from '../app-shell/app-shell.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, el, esc} from '../../core/dom.js';
import {modelOptionsFrom} from '../../core/models.js';
import {toast} from '../../core/toast.js';

/* ---- Providers ----
   One card per configured harness. The card is the only place the binary path,
   the configuration directory and the provider's default model live, so there
   is exactly one answer to "which claude does ClaudeQ run?". Readiness comes
   from the daemon, which probes the CLI itself — the card never guesses. */

const PROVIDER_STATE={
  ready:        ['ok',   'Ready'],
  not_installed:['err',  'Not installed'],
  not_authenticated:['err','Not logged in'],
  invalid_configuration:['err','Invalid configuration'],
  disabled:     ['warn', 'Switched off'],
  check_failed: ['warn', 'Could not be checked'],
};
function providerStateChip(h){
  const [cls,label]=PROVIDER_STATE[h.state]||['warn',h.state||'unknown'];
  const chip=cls==='ok'?'chip accent':(cls==='err'?'chip danger':'chip warn');
  return `<span class="${chip}">${esc(label)}</span>`;
}
// PROVIDERS is the last list the daemon reported, kept so the task form can
// offer the providers a task may run on without asking again on every keystroke.
export let PROVIDERS=[], PROVIDER_KINDS=[];
// BETA_PROVIDERS are the configured instances claudeq does not consider
// finished, so every row that names one can say so.
export let BETA_PROVIDERS=new Set();
// SHOW_BETA is the operator's "beta features" preference, read from the
// settings rather than from the Settings form — the task sheet needs it on a
// fresh page load, long before that form exists.
//
// It gates offering only: a beta provider that is already configured keeps its
// block and runs its tasks either way.
export let SHOW_BETA=false;
export function betaAllowed(){ return SHOW_BETA; }
// DEFAULT_WORKING_DIR prefills a new task's directory field (openAdd). Kept in
// sync with Settings the same way SHOW_BETA is: every /api/settings fetch below
// updates it, so it is current by the time "+ New task" is clicked.
export let DEFAULT_WORKING_DIR='';
export async function loadProviders(){
  const box=$('#s-providers');
  try{ setProviderSnapshot({providers:await api('GET','/api/providers')||[]}); }catch(e){
    if(box) box.innerHTML=`<div class="group"><div class="row"><div class="grow"><div class="sub">${esc(e.message)}</div></div></div></div>`;
    return; }
  if(!PROVIDER_KINDS.length){ try{ PROVIDER_KINDS=await api('GET','/api/providers/kinds')||[]; }catch{} }
  if(!box) return;
  box.innerHTML='';
  [...PROVIDERS].sort((a,b)=>(a.name||a.id).localeCompare(b.name||b.id)).forEach(p=>box.append(providerBlock(p)));
  // Adding a provider is one of the parts claudeq does not consider finished,
  // so the button appears only once the operator has asked for those.
  const add=$('#s-provider-add');
  if(add) add.hidden=!betaAllowed();
}
// providerBlock is one provider's settings block: its name above it, the way
// every other group in Settings is introduced.
function providerBlock(p){
  const wrap=el('div');
  const detected=p.detected?`Detected: ${p.detected}`:'';
  const chips=providerStateChip(p.health)
    +(p.default?'<span class="chip">default</span>':'')
    +(p.beta?'<span class="chip beta">beta</span>':'');
  wrap.innerHTML=`
    <div class="section-label" style="margin-top:18px">${esc(p.name||p.id)}</div>
    <div class="group">
      <div class="row"><div class="grow">
          <div class="title">${chips}</div>
          <div class="sub multi"><span class="mono">${esc(p.id)}</span>${p.health.reason||p.health.detail?' — '+esc(p.health.reason||p.health.detail):''}</div></div>
        <label class="switch"><input type="checkbox" ${p.enabled?'checked':''}><span class="sl"></span></label></div>
      <div class="row"><div class="grow"><div class="title">Name</div>
          <div class="sub">What this provider is called in the app and in run messages</div></div>
        <input type="text" class="p-name" style="max-width:260px"></div>
      <div class="row"><div class="grow"><div class="title">Binary</div>
          <div class="sub">Absolute path to the CLI. The background daemon can't see your shell's PATH, so a full path is safest. Empty auto-detects. ${esc(detected)}</div></div>
        <input type="text" class="p-path" placeholder="${esc(p.detected||'/path/to/the CLI')}" style="max-width:260px"></div>
      <div class="row"><div class="grow"><div class="title">Configuration directory</div>
          <div class="sub">Where the CLI keeps its account and sessions — another directory is another account. Absolute path (<code>~</code> is fine); empty uses its own default.</div></div>
        <input type="text" class="p-dir" placeholder="${esc(p.default_config_dir||"the CLI's own")}" style="max-width:260px"></div>
      <div class="row"><div class="grow"><div class="title">Default model</div>
          <div class="sub">Used for tasks on this provider that name no model</div></div>
        <select class="p-model" style="max-width:230px"></select></div>
      <div class="row"><div class="grow"><div class="title">When the limit is reached</div>
          <div class="sub">Where this provider's tasks run while its allowance is used up. Without one they wait for the window to reopen. An interrupted session doesn't travel — the substitute starts fresh.</div></div>
        <select class="p-fallback" style="max-width:230px"></select></div>
      <div class="row"><div class="grow"><div class="sub">${esc(p.health.binary||'')}</div></div>
        <button class="btn p-default"${p.default||!p.enabled?' disabled':''}>Make default</button>
        <button class="btn p-check">Check again</button>
        <button class="btn danger p-remove"${p.default?' disabled':''}>Remove</button></div>
    </div>`;
  wrap.querySelector('.p-name').value=p.name||'';
  wrap.querySelector('.p-path').value=p.binary_path||'';
  wrap.querySelector('.p-dir').value=p.config_dir||'';
  fillProviderModels(wrap.querySelector('.p-model'),p);
  fillFallbackChoices(wrap.querySelector('.p-fallback'),p);
  // The block is a form like every other group in Settings: Save at the top of
  // the page writes it (see saveSettings). What sits here are actions, not
  // edits — they take effect at once and reload the list.
  wrap.dataset.providerId=p.id;
  wrap.querySelector('.p-check').onclick=async ()=>{
    try{ await api('POST','/api/providers/'+encodeURIComponent(p.id)+'/check'); }catch(e){ toast(e.message,'err'); }
    loadProviders();
  };
  wrap.querySelector('.p-default').onclick=async ()=>{
    try{ await api('POST','/api/providers/'+encodeURIComponent(p.id)+'/default'); toast('Default provider set','ok'); }
    catch(e){ toast(e.message,'err'); }
    loadProviders();
  };
  wrap.querySelector('.p-remove').onclick=async ()=>{
    if(!await confirmSheetAsk(`Remove the provider “${p.name||p.id}”?`,'Remove','Keep it')) return;
    try{ await api('DELETE','/api/providers/'+encodeURIComponent(p.id)); toast('Provider removed','ok'); }
    catch(e){ toast(e.message,'err'); }
    loadProviders();
  };
  return wrap;
}
// openAddProvider asks for the things that cannot be changed afterwards — the
// id and the kind — plus the two that decide what the new provider *is*. The
// rest is on its card once it exists.
export function openAddProvider(){
  const kinds=PROVIDER_KINDS.filter(k=>!k.beta||betaAllowed());
  $('#np-kind').innerHTML=kinds.map(k=>`<option value="${esc(k.kind)}">${esc(k.name||k.kind)}${k.beta?' (beta)':''}</option>`).join('');
  $('#np-id').value=''; $('#np-name').value=''; $('#np-dir').value=''; $('#np-err').textContent='';
  // The placeholder is the chosen harness's own location, so an empty field
  // says what it falls back to rather than leaving it to be guessed.
  const showDefaultDir=()=>{
    const k=PROVIDER_KINDS.find(x=>x.kind===$('#np-kind').value);
    $('#np-dir').placeholder=(k&&k.default_config_dir)||"the CLI's own";
  };
  $('#np-kind').onchange=showDefaultDir;
  showDefaultDir();
  $('#providerSheet').showModal();
  $('#np-id').focus();
}
export async function submitProvider(){
  const id=$('#np-id').value.trim();
  if(!id){ $('#np-err').textContent='Give the provider an id — tasks name it by that.'; return; }
  const body={id, kind:$('#np-kind').value, name:$('#np-name').value.trim()||id,
    config_dir:$('#np-dir').value.trim()};
  try{ await api('POST','/api/providers',body); }
  catch(e){ $('#np-err').textContent=e.message; return; }
  $('#providerSheet').close();
  toast('Provider added','ok');
  loadProviders();
}

// fillAsideChoices fills a provider/model pair for one of the jobs claudeq runs
// on its own behalf — prompt review, the feedback assistant.
//
// Only providers that can actually answer such a question are offered (see
// internal/aside): a harness that cannot would leave the feature silently
// unavailable, and an empty list is more honest than a choice that does nothing.
// A stored provider that is gone keeps its place, marked, so a save does not
// quietly move the job somewhere else.
export async function fillAsideChoices(providerSel,modelSel,providerID,model,modelEmptyLabel){
  const offered=PROVIDERS.filter(p=>p.enabled&&p.asides&&(!p.beta||betaAllowed()));
  const dflt=PROVIDERS.find(p=>p.default);
  const known=offered.some(p=>p.id===providerID);
  providerSel.innerHTML=`<option value="">${esc(dflt?`Default (${dflt.name||dflt.id})`:'Default provider')}</option>`
    +offered.map(p=>`<option value="${esc(p.id)}"${p.id===providerID?' selected':''}>${esc((p.name||p.id)+(p.beta?' (beta)':''))}</option>`).join('')
    +(providerID&&!known?`<option value="${esc(providerID)}" selected>${esc(providerID)} — unavailable</option>`:'');
  providerSel.onchange=()=>fillAsideModels(modelSel,providerSel.value,'',modelEmptyLabel);
  await fillAsideModels(modelSel,providerID,model,modelEmptyLabel);
}
// fillAsideModels offers what the chosen provider suggests. Changing the
// provider clears the model: a model name means nothing to another harness.
async function fillAsideModels(select,providerID,model,emptyLabel){
  select.innerHTML=modelOptionsFrom([],model||'',emptyLabel);
  let models=[];
  try{ models=await api('GET','/api/models'+(providerID?'?provider='+encodeURIComponent(providerID):''))||[]; }catch{ return; }
  select.innerHTML=modelOptionsFrom(models,model||'',emptyLabel);
}
// fillProviderModels asks the provider itself what it suggests, so a Codex card
// offers Codex models and a Claude card offers Claude's. The stored value is
// kept even when the list does not contain it: a model catalog is a suggestion,
// never validation.
async function fillProviderModels(select,p){
  // The stored value is rendered first and replaced only once the real list has
  // arrived: blanking the field while the request is out would let a Save in
  // that window write an empty default model.
  select.innerHTML=modelOptionsFrom([],p.default_model||'','Provider default');
  let models=[];
  try{ models=await api('GET','/api/models?provider='+encodeURIComponent(p.id))||[]; }catch{ return; }
  select.innerHTML=modelOptionsFrom(models,p.default_model||'','Provider default');
}
// fillFallbackChoices offers every other configured provider as the one that
// takes over while this one is rate-limited. A stored id that has since gone
// keeps its place, marked, so saving the card does not quietly drop the
// fallback the operator chose.
function fillFallbackChoices(select,p){
  const others=PROVIDERS.filter(x=>x.id!==p.id);
  const known=others.some(x=>x.id===p.fallback_provider);
  select.innerHTML='<option value="">Wait for the limit</option>'
    +others.map(x=>`<option value="${esc(x.id)}"${x.id===p.fallback_provider?' selected':''}>${esc((x.name||x.id)+(x.enabled?'':' (switched off)'))}</option>`).join('')
    +(p.fallback_provider&&!known?`<option value="${esc(p.fallback_provider)}" selected>${esc(p.fallback_provider)} — unavailable</option>`:'');
}
// Show one settings pane. The choice is remembered for the session, so leaving
// Settings and coming back lands on the pane you were last on.

// The Queue, the task sheet, Activity and Settings each fetch /api/providers
// alongside their own data. They hand the answer here instead of keeping a
// second copy that could disagree with this one.
export function setProviderSnapshot({providers,showBeta,defaultDir}={}){
  if(providers) PROVIDERS=providers;
  if(showBeta!==undefined) SHOW_BETA=!!showBeta;
  if(defaultDir!==undefined) DEFAULT_WORKING_DIR=defaultDir||'';
  BETA_PROVIDERS=new Set(PROVIDERS.filter(p=>p.beta).flatMap(p=>p.default?[p.id,'']:[p.id]));
}

// The sheet that adds a provider.
const sheetTemplate = `
<dialog id="providerSheet">
  <div class="sheet-hd"><b>Add provider</b></div>
  <div class="sheet-bd">
    <div class="form-row">
      <div><label class="fld">Id</label><input type="text" id="np-id" placeholder="claude-work"></div>
      <div><label class="fld">Type</label><select id="np-kind"></select></div>
    </div>
    <label class="fld">Name</label><input type="text" id="np-name" placeholder="Shown in the app and in run messages">
    <label class="fld">Configuration directory</label><input type="text" id="np-dir" placeholder="the CLI's own">
    <div class="hint" style="margin-top:6px">The id is how tasks name this provider and cannot be changed later, nor can the type. Give it its own configuration directory to make it a second account, with its own sessions and its own rate limit. Everything else is set on its card afterwards.</div>
    <p id="np-err" class="hint" style="color:var(--danger)"></p>
  </div>
  <div class="sheet-ft"><button class="btn" id="np-cancel">Cancel</button>
    <button class="btn primary" id="np-add">Add provider</button></div>
</dialog>
`;

export function mountProviderSheet(){ document.body.insertAdjacentHTML('beforeend', sheetTemplate); }

export function initProviderSheet(){ $('#np-cancel').onclick=()=>$('#providerSheet').close(); }
