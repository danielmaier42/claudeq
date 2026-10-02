import {PROVIDERS, betaAllowed} from '../providers/providers.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, el, esc} from '../../core/dom.js';
import {toast} from '../../core/toast.js';

/* ---- Pools ----
   A pool is several providers of one type that a task can be given as a whole.
   Each run goes to the member whose weekly allowance would otherwise expire
   unused soonest (free share × weight ÷ hours to reset); the daemon decides,
   and the Dashboard shows the order it would take right now. */

// POOLS is the last list the daemon reported, with each pool's ranking. The
// task sheet and the queue read it to offer and name pools.
export let POOLS=[];
export async function loadPoolList(){
  try{ POOLS=await api('GET','/api/pools')||[]; }catch{}
  return POOLS;
}
export function poolByID(id){ return POOLS.find(p=>p.id===id); }

// The capacity each provider's plan reports, for the weight field's
// placeholder: an empty weight means exactly that number.
let capacities={};
async function loadCapacities(){
  try{ (await api('GET','/api/limits')||[]).forEach(l=>{ capacities[l.id]=l.limits.capacity||0; }); }catch{}
}
function weightPlaceholder(id){ const c=capacities[id]; return c?`auto · ${c}×`:'auto · 1×'; }

// onPoolsDrawn is told whenever the pool list in Settings has been redrawn, so
// Settings can show or hide the Pools tab for what is configured now.
let onPoolsDrawn=()=>{};
export function setOnPoolsDrawn(f){ onPoolsDrawn=f; }

export async function loadPools(){
  const box=$('#s-pools');
  if(!box) return;
  await Promise.all([loadPoolList(), loadCapacities()]);
  drawPools(box);
  onPoolsDrawn();
}
function drawPools(box){
  box.innerHTML='';
  if(!POOLS.length){
    if(!betaAllowed()) return;
    box.innerHTML=`<div class="group"><div class="row"><div class="grow"><div class="sub multi">No pools yet. A pool groups providers of one type — two Claude subscriptions, say — and every run of a task on it goes to the account whose weekly allowance would otherwise expire unused soonest.</div></div></div></div>`;
    return;
  }
  POOLS.forEach(p=>box.append(poolBlock(p)));
}

// poolBlock is one pool's settings block: its name, which providers are in it
// and with what weight. Only providers of the members' type are offered —
// a task's model has to mean the same thing on every one of them.
function poolBlock(p){
  const wrap=el('div');
  wrap.dataset.poolId=p.id;
  const kindOf=id=>(PROVIDERS.find(x=>x.id===id)||{}).kind;
  const kind=kindOf((p.members[0]||{}).provider);
  const candidates=PROVIDERS.filter(x=>!kind||x.kind===kind);
  const member=id=>p.members.find(m=>m.provider===id);
  wrap.innerHTML=`
    <div class="section-label" style="margin-top:18px">${esc(p.name||p.id)} <span class="chip beta">beta</span></div>
    <div class="group">
      <div class="row"><div class="grow"><div class="title">Name</div>
          <div class="sub"><span class="mono">${esc(p.id)}</span> — what tasks name it by</div></div>
        <input type="text" class="pl-name" style="max-width:260px"></div>
      ${candidates.map(x=>{ const m=member(x.id);
        return `<div class="row pl-member" data-provider="${esc(x.id)}">
          <label class="switch"><input type="checkbox" ${m?'checked':''}><span class="sl"></span></label>
          <div class="grow"><div class="title">${esc(x.name||x.id)}</div>
            <div class="sub">Weight: how large its plan is next to the others'. Empty takes what the provider reports.</div></div>
          <input type="number" class="pl-weight" min="0" step="any" style="max-width:120px"
            placeholder="${esc(weightPlaceholder(x.id))}" value="${m&&m.weight?m.weight:''}"></div>`; }).join('')}
      <div class="row"><div class="grow"></div>
        <button class="btn danger pl-remove">Remove</button></div>
    </div>`;
  wrap.querySelector('.pl-name').value=p.name||'';
  wrap.querySelector('.pl-remove').onclick=async ()=>{
    if(!await confirmSheetAsk(`Remove the pool “${p.name||p.id}”? Tasks that run on it have to be moved first.`,'Remove')) return;
    try{ await api('DELETE','/api/pools/'+encodeURIComponent(p.id)); toast('Pool removed','ok'); }
    catch(e){ toast(e.message,'err'); }
    loadPools();
  };
  return wrap;
}

// poolEdits reads back the pool blocks that differ from what the daemon has,
// as the payload each pool takes; the Settings Save writes them with
// everything else. An untouched pool is not written, so a Save elsewhere in
// Settings can never fail over a pool nobody edited.
export function poolEdits(){
  const same=(e)=>{ const p=poolByID(e.id); if(!p) return false;
    const norm=ms=>JSON.stringify(ms.map(m=>[m.provider,m.weight||0]).sort());
    return e.body.name===(p.name||p.id) && norm(e.body.members)===norm(p.members); };
  return [...document.querySelectorAll('#s-pools [data-pool-id]')].map(w=>({
    id:w.dataset.poolId,
    body:{
      name:w.querySelector('.pl-name').value.trim(),
      members:[...w.querySelectorAll('.pl-member')].filter(r=>r.querySelector('input[type=checkbox]').checked)
        .map(r=>({provider:r.dataset.provider, weight:parseFloat(r.querySelector('.pl-weight').value)||0})),
    },
  })).filter(e=>!same(e));
}

// The sheet that adds a pool: its id, name and members. Weights are set on its
// block afterwards, where the reported plan sizes are shown.
const sheetTemplate = `
<dialog id="poolSheet">
  <div class="sheet-hd"><b>Add pool</b></div>
  <div class="sheet-bd">
    <div class="form-row">
      <div><label class="fld">Id</label><input type="text" id="npl-id" placeholder="claude-pool"></div>
      <div><label class="fld">Name</label><input type="text" id="npl-name" placeholder="Shown in the app and in run messages"></div>
    </div>
    <label class="fld">Members</label>
    <div id="npl-members"></div>
    <div class="hint" style="margin-top:6px">Providers of one type only. The id is how tasks name the pool and cannot be changed later.</div>
    <p id="npl-err" class="hint" style="color:var(--danger)"></p>
  </div>
  <div class="sheet-ft"><button class="btn" id="npl-cancel">Cancel</button>
    <button class="btn primary" id="npl-add">Add pool</button></div>
</dialog>
`;
export function mountPoolSheet(){ document.body.insertAdjacentHTML('beforeend', sheetTemplate); }
export function initPoolSheet(){
  $('#npl-cancel').onclick=()=>$('#poolSheet').close();
  $('#npl-add').onclick=submitPool;
}
export function openAddPool(){
  $('#npl-id').value=''; $('#npl-name').value=''; $('#npl-err').textContent='';
  $('#npl-members').innerHTML=PROVIDERS.map(p=>`<label class="chk"><input type="checkbox" value="${esc(p.id)}"> ${esc(p.name||p.id)} <span class="sub">${esc(p.type_name||p.kind)}</span></label>`).join('');
  $('#poolSheet').showModal();
  $('#npl-id').focus();
}
async function submitPool(){
  const id=$('#npl-id').value.trim();
  if(!id){ $('#npl-err').textContent='Give the pool an id — tasks name it by that.'; return; }
  const members=[...document.querySelectorAll('#npl-members input:checked')].map(i=>({provider:i.value,weight:0}));
  try{ await api('POST','/api/pools',{id,name:$('#npl-name').value.trim()||id,members}); }
  catch(e){ $('#npl-err').textContent=e.message; return; }
  $('#poolSheet').close();
  toast('Pool added','ok');
  loadPools();
}
