import {current} from '../app-shell/app-shell.js';
import {renderConnText} from '../status/status.js';
import {api} from '../../core/api.js';
import {$, esc, httpsOnly} from '../../core/dom.js';
import {toast} from '../../core/toast.js';

export let UPDATE=null;
export async function loadUpdate(){
  try{ UPDATE=await api('GET','/api/update'); }catch{ return; }
  renderUpdateBadge(); renderConnText();
  if(current==='settings'){ renderUpdateBox(); renderVersionRow(); }
}
// Red "1" badge next to Settings whenever a non-dismissed update is available,
// plus a red dot on the General tab, which is where About and the update live.
export function renderUpdateBadge(){
  const on=!!(UPDATE&&(UPDATE.available||UPDATE.restart_required));
  const d=$('#s-tab-dot'); if(d) d.hidden=!on;
  const b=$('#updateBadge'); if(!b) return;
  b.hidden=!on; if(on) b.textContent='1';
}
// Yellow banner at the top of the Settings pane.
export function renderUpdateBox(){
  const box=$('#updateBox'); if(!box) return;
  if(UPDATE&&UPDATE.restart_required){ renderRestartBox(box); return; }
  if(!UPDATE||!UPDATE.available){ box.hidden=true; box.innerHTML=''; return; }
  box.hidden=false;
  const notes=(UPDATE.notes||'').trim();
  const skipped=UPDATE.skipped_count||0;
  // When more than one version was skipped, say so — the notes below cover them all.
  const lead=skipped>1
    ? `You're currently on ${esc(UPDATE.current||'?')} — ${skipped} newer versions are available. Their changes are below.`
    : `You're currently on ${esc(UPDATE.current||'?')}. Download the latest installer and follow the prompts to update.`;
  const allUrl=httpsOnly(UPDATE.all_releases_url);
  box.innerHTML=`<b>⬆ Update available — ClaudeQ ${esc(UPDATE.latest)}</b>`
    +lead
    +(notes?`<div class="upd-notes">${esc(notes)}</div>`:'')
    +`<div class="upd-actions">
        <button class="btn primary" id="u-download">Download &amp; install</button>
        <button class="btn" id="u-dismiss">Dismiss</button>
        ${allUrl?`<a class="btn" href="${esc(allUrl)}" target="_blank" rel="noopener">All releases on GitHub ↗</a>`:''}
      </div>`;
  $('#u-download').onclick=downloadUpdate;
  $('#u-dismiss').onclick=()=>dismissUpdate(UPDATE.latest);
}
// The installer replaced the app but the background service kept running the
// old build, so the update never took effect. Offer the hand-over instead of
// another download of a version that is already on disk.
function renderRestartBox(box){
  box.hidden=false;
  box.innerHTML=`<b>⬆ ClaudeQ ${esc(UPDATE.installed)} is installed — but not running</b>`
    +`The background service is still on ${esc(UPDATE.current||'?')}. Finish the update to switch it over; `
    +`your queue and running tasks are untouched.`
    +`<div class="upd-actions"><button class="btn primary" id="u-relaunch">Finish update</button></div>`;
  $('#u-relaunch').onclick=finishUpdate;
}
async function finishUpdate(){
  const btn=$('#u-relaunch'); if(btn){ btn.disabled=true; btn.textContent='Switching over…'; }
  try{ await api('POST','/api/update/relaunch'); }
  catch(e){
    toast('Could not finish the update: '+e.message,'err');
    if(btn){ btn.disabled=false; btn.textContent='Finish update'; }
    return;
  }
  // The daemon we are talking to is being replaced, so wait for the new one to
  // answer before reloading — otherwise the page reloads into a dead port.
  for(let i=0;i<100;i++){
    await new Promise(r=>setTimeout(r,300));
    try{
      const st=await api('GET','/api/update');
      if(!st.restart_required){ location.reload(); return; }
    }catch(e){ /* still restarting */ }
  }
  toast('The background service did not come back — reopen ClaudeQ','err');
  if(btn){ btn.disabled=false; btn.textContent='Finish update'; }
}
export function renderVersionRow(){
  const v=$('#s-version'); if(v) v.textContent=UPDATE&&UPDATE.current?(UPDATE.supported?('v'+UPDATE.current):UPDATE.current):'';
  const sub=$('#s-update-sub');
  if(sub&&UPDATE){
    if(UPDATE.restart_required) sub.textContent='ClaudeQ '+UPDATE.installed+' is installed but not running yet';
    else if(UPDATE.available) sub.textContent='ClaudeQ '+UPDATE.latest+' is available';
    else if(!UPDATE.supported) sub.textContent='Development build — updates are checked in released versions';
    else sub.textContent='You’re on the latest version'+(UPDATE.current?(' ('+UPDATE.current+')'):'');
  }
}
async function downloadUpdate(){
  const btn=$('#u-download'); if(btn){ btn.disabled=true; btn.textContent='Downloading…'; }
  try{
    await api('POST','/api/update/download');
    toast('Installer downloaded — follow the prompts to finish updating','ok');
  }catch(e){
    toast('Download failed: '+e.message,'err');
    if(btn){ btn.disabled=false; btn.innerHTML='Download &amp; install'; }
  }
}
async function dismissUpdate(v){
  try{ UPDATE=await api('POST','/api/update/dismiss',{version:v}); }catch(e){ toast(e.message,'err'); return; }
  toast('Dismissed — we’ll tell you about the next version','ok');
  renderUpdateBadge(); renderUpdateBox(); renderVersionRow();
}
export async function checkForUpdates(){
  const btn=$('#s-check-updates'); const orig=btn?btn.textContent:'';
  if(btn){ btn.disabled=true; btn.textContent='Checking…'; }
  try{ UPDATE=await api('POST','/api/update/check'); }
  catch(e){ toast('Check failed: '+e.message,'err'); }
  finally{ if(btn){ btn.disabled=false; btn.textContent=orig||'Check for updates'; } }
  renderUpdateBadge(); renderUpdateBox(); renderVersionRow();
  if(UPDATE){
    if(UPDATE.restart_required) toast('ClaudeQ '+UPDATE.installed+' is installed but not running — finish the update below','ok');
    else if(UPDATE.available) toast('Update available: ClaudeQ '+UPDATE.latest,'ok');
    else if(!UPDATE.supported) toast('Running a development build','ok');
    else if(UPDATE.error) toast('Check failed: '+UPDATE.error,'err');
    else toast('You’re up to date','ok');
  }
}
