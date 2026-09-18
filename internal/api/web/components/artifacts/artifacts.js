import {current, select} from '../app-shell/app-shell.js';
import {canContinue, showLog} from '../log-sheet/log-sheet.js';
import {setConn} from '../status/status.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, el, emptyState, esc} from '../../core/dom.js';
import {exactTime, relTime} from '../../core/format.js';
import {EYE} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

let artifactsSig='';
const fmtBytes=n=>{ n=n||0; if(n<1024)return n+' B'; if(n<1048576)return (n/1024).toFixed(1)+' KB'; if(n<1073741824)return (n/1048576).toFixed(1)+' MB'; return (n/1073741824).toFixed(1)+' GB'; };
function artifactKind(ct){ ct=(ct||'').toLowerCase();
  if(ct.includes('pdf'))return 'pdf';
  if(ct.startsWith('text/html'))return 'html';
  if(ct.startsWith('image/'))return 'image';
  if(ct.startsWith('text/')||ct.includes('json')||ct.includes('xml')||ct.includes('javascript')||ct.includes('csv'))return 'text';
  return 'other'; }
function extLabel(name,ct){ const m=String(name).match(/\.([a-z0-9]+)$/i); if(m)return m[1].toUpperCase();
  const k=artifactKind(ct); return k==='other'?'FILE':k.toUpperCase(); }
function contentURL(a,download){ return '/api/artifacts/'+encodeURIComponent(a.id)+'/content'+(download?'?download=1':''); }
function openArtifactExternal(a){ const url=location.origin+contentURL(a,false);
  if(window.cqOpenExternal) window.cqOpenExternal(url); else window.open(url,'_blank','noopener');
  markReadOnOpen(a); }
// Opening an artifact — in the in-app viewer or externally — counts as reading
// it. The local flag is cleared first so a second open (e.g. "Open externally"
// from an already-opened viewer) does not re-post and re-render the list.
function markReadOnOpen(a){ if(!a||!a.unread) return; a.unread=false; readArtifact(a.id); }
// Jump to Activity and open the log of the run that produced this artifact. The
// run may have been pruned from history (artifacts outlive runs), so check first.
async function openArtifactRun(a){
  if(!a.run_id) return;
  let runs=[]; try{ runs=await api('GET','/api/runs'); }catch(e){ toast(e.message,'err'); return; }
  const run=runs.find(r=>r.run_id===a.run_id);
  if(!run){ toast('The run that produced this artifact is no longer in history','err'); return; }
  select('news');
  showLog(run);
}

async function loadArtifacts(){
  let arts; try{ arts=await api('GET','/api/artifacts'); setConn(true);}catch(e){ setConn(false); return; }
  const unread=arts.filter(a=>a.unread).length; const badge=$('#artifactCount'); badge.hidden=unread===0; badge.textContent=unread;
  const sig=JSON.stringify(arts.map(a=>[a.id,a.unread,a.title]));
  if(sig===artifactsSig && $('#artifacts').childElementCount) return;   // avoid flicker on poll
  artifactsSig=sig;
  const c=$('#artifacts'); c.innerHTML='';
  if(!arts.length){ c.append(emptyState('No artifacts yet','Files your tasks publish with “claudeq publish” appear here — reports, exports, HTML pages, PDFs.')); return; }
  const label=el('div','section-label',arts.length+' artifact'+(arts.length!==1?'s':'')); c.append(label);
  const list=el('div','act-list');
  arts.forEach(a=>{
    const line=el('div','act-line');
    const gl=el('div','act-gl'); if(a.unread) gl.append(el('div','unread-dot'));
    const card=el('div','act-card');
    const grow=el('div','grow');
    // "from <task>" links to the producing run's log in Activity (when the run is
    // still in history); the rest of the line is the file name, size and time.
    const srcHtml=a.task_name
      ? (a.run_id ? `<span class="art-src" role="button" tabindex="0">from ${esc(a.task_name)}</span> · `
                  : `from ${esc(a.task_name)} · `)
      : '';
    const meta=`${esc(a.file_name)} · ${esc(fmtBytes(a.size))} · <span class="hint-time" data-tip="${esc(exactTime(a.published_at))}">${esc(relTime(a.published_at))}</span>`;
    grow.innerHTML=`<div class="title">${esc(a.title)}</div>`
      +(a.description?`<div class="sub" style="white-space:normal">${esc(a.description)}</div>`:'')
      +`<div class="sub">${srcHtml}${meta}</div>`;
    if(a.task_name && a.run_id){ const s=grow.querySelector('.art-src'); if(s) s.onclick=()=>openArtifactRun(a); }
    const actions=el('div','row-actions');
    // Every artifact opens the viewer — types without a preview get a placeholder
    // there, and keep the sheet's "Open externally" and "Continue in Chat…".
    const v=el('button','btn small','View'); v.onclick=()=>openViewer(a); actions.append(v);
    const del=el('button','btn small danger','Delete'); del.title='Delete'; del.onclick=()=>deleteArtifact(a); actions.append(del);
    const kind=el('span','art-meta',extLabel(a.file_name,a.content_type));
    card.append(grow,actions,kind);
    // Dot and eye sit inside the card, as in Activity, so an artifact row is
    // exactly as wide as a Queue group or a Usage card.
    if(a.unread){ const mr=el('button','eye-btn',EYE); mr.title='Mark read'; mr.onclick=()=>readArtifact(a.id); actions.prepend(mr); }
    card.prepend(gl);
    line.append(card);
    list.append(line);
  });
  c.append(list);
}
// viewerGen tags each open so that async work (a text fetch) and the close
// handler never clobber a viewer that was opened after them — the most recent
// open always wins, even if the user closes and reopens in quick succession.
let viewerGen=0;
async function openViewer(a){
  const gen=++viewerGen;
  $('#viewerTitle').textContent=a.title;
  updateViewerContinue(a,gen);   // async: reveals "Continue in Chat…" if the producing run can be resumed
  const body=$('#viewerBody'); body.innerHTML='';
  const kind=artifactKind(a.content_type);
  const src=contentURL(a,false);
  if(kind==='image'){ const img=el('img','viewer-img'); img.src=src; img.alt=a.title; body.append(img); }
  else if(kind==='text'){
    let txt=''; try{ txt=await api('GET',src); }catch(e){ txt='(could not load file)'; }
    if(gen!==viewerGen) return;   // a newer open (or a close) superseded us
    const pre=el('pre','log viewer-text'); pre.textContent=typeof txt==='string'?txt:JSON.stringify(txt,null,2); body.append(pre);
  } else if(kind==='pdf'){
    // PDFs render in the browser's built-in viewer. Chromium refuses to show a
    // PDF inside a sandboxed iframe, so this one is not sandboxed — safe because
    // the file is served as application/pdf with X-Content-Type-Options: nosniff,
    // so it can never be interpreted/executed as HTML, and the native PDF viewer
    // does not expose the dashboard's origin to the document.
    const frame=document.createElement('iframe'); frame.className='viewer-frame';
    frame.src=src; body.append(frame);
  } else if(kind==='html'){
    // HTML runs in an opaque origin (sandbox without allow-same-origin) so it
    // cannot reach the loopback API or the parent DOM; the content CSP
    // additionally blocks any network access, so it cannot phone home.
    const frame=document.createElement('iframe'); frame.className='viewer-frame';
    frame.setAttribute('sandbox','allow-scripts');
    frame.src=src; body.append(frame);
  } else {
    // Archives, binaries and anything else the browser cannot render inline:
    // the sheet still opens, so the file can be handed to the browser from the
    // header and the publishing session continued from the footer.
    body.append(emptyState('This file type has no preview',
      a.file_name+' · '+fmtBytes(a.size)+' · use “Open externally” to open or save it'));
  }
  const acts=$('#viewerActions'); acts.innerHTML='';
  const open=el('button','btn small','Open externally'); open.onclick=()=>openArtifactExternal(a); acts.append(open);
  $('#viewerSheet').showModal();
  markReadOnOpen(a);   // opening an artifact marks it read
}
// "Continue in Chat…" resumes the chat that published the artifact, the same
// way the log sheet does. Artifacts outlive runs, so the producing run has to be
// looked up first: it may be gone from history, or not resumable (still running,
// no session id, no working dir), in which case the button stays hidden.
let viewerRunId='';
async function updateViewerContinue(a,gen){
  $('#viewerContinueBtn').hidden=true; viewerRunId='';
  if(!a.run_id) return;
  let runs=[]; try{ runs=await api('GET','/api/runs'); }catch{ return; }
  if(gen!==viewerGen) return;   // a newer open (or a close) superseded us
  const run=runs.find(r=>r.run_id===a.run_id);
  if(!canContinue(run)) return;
  viewerRunId=run.run_id; $('#viewerContinueBtn').hidden=false;
}
async function continueArtifactRun(){
  if(!viewerRunId) return;
  try{ await api('POST',`/api/runs/${viewerRunId}/continue`); toast('Opening Terminal…','ok'); }
  catch(e){ toast('Continue failed: '+e.message,'err'); }
}
// On close, clear the body (stops iframe playback/loading) only if no newer
// viewer has since opened — deferred so a close-then-reopen keeps the new view.
export function initViewer(){
  $('#viewerSheet').addEventListener('close',()=>{ const gen=viewerGen;
    setTimeout(()=>{ if(gen===viewerGen && !$('#viewerSheet').open) $('#viewerBody').innerHTML=''; },0); });
  $('#viewerContinueBtn').onclick=continueArtifactRun;
  $('#viewerDoneBtn').onclick=()=>$('#viewerSheet').close();
}
// Open one artifact by id. This is what a click on a "new artifact"
// notification ends up calling (the app window pushes the id in from
// cqOpenPendingArtifact): show the Artifacts view, then open the artifact in
// the viewer. If it is already gone, the view alone is the fallback.
window.cqOpenArtifact=async function(id){
  select('artifacts');
  let arts; try{ arts=await api('GET','/api/artifacts'); }catch(e){ toast(e.message,'err'); return; }
  const a=arts.find(x=>x.id===id);
  if(!a){ toast('That artifact is no longer available','err'); return; }
  openViewer(a);
};
async function readArtifact(id){ try{ await api('POST','/api/artifacts/'+encodeURIComponent(id)+'/read'); }catch{} artifactsSig=''; loadArtifacts(); }
async function markAllArtifactsRead(){ try{ await api('POST','/api/artifacts/read-all'); toast('All marked read','ok'); artifactsSig=''; loadArtifacts();}catch(e){toast(e.message,'err');} }
async function deleteArtifact(a){ if(await confirmSheetAsk('Delete artifact “'+a.title+'”? This removes the stored copy.')){ try{ await api('DELETE','/api/artifacts/'+encodeURIComponent(a.id)); toast('Deleted','ok'); artifactsSig=''; loadArtifacts();}catch(e){toast(e.message,'err');} } }

export const view={
  title:'Artifacts',
  toolbar(ta){ const b=el('button','btn',EYE+'<span>Mark all read</span>'); b.onclick=markAllArtifactsRead; ta.append(b); },
  enter(){ loadArtifacts(); },
};

// The sheet that previews one artifact.
const sheetTemplate = `
<dialog id="viewerSheet">
  <div class="sheet-hd"><b id="viewerTitle">Artifact</b><div class="spacer" style="flex:1"></div>
    <div id="viewerActions" class="row-actions"></div></div>
  <div class="sheet-bd" id="viewerBody"></div>
  <div class="sheet-ft"><button class="btn" id="viewerContinueBtn" hidden data-tip="Opens Terminal in the task's folder and resumes the chat that published this artifact — with the full conversation context">Continue in Chat…</button><button class="btn" id="viewerDoneBtn">Done</button></div>
</dialog>
`;

export function mountViewer(){ document.body.insertAdjacentHTML('beforeend', sheetTemplate); }

// Keep the artifact unread badge current on every tab; re-render the list when
// it's the open tab.
export async function refreshArtifacts(){
  if(current==='artifacts'){ loadArtifacts(); return; }
  try{ const arts=await api('GET','/api/artifacts'); const unread=arts.filter(a=>a.unread).length;
    const badge=$('#artifactCount'); badge.hidden=unread===0; badge.textContent=unread; }catch{}
}
