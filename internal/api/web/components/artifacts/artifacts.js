import {current, select} from '../app-shell/app-shell.js';
import {canContinue, showLog} from '../log-sheet/log-sheet.js';
import {setConn} from '../status/status.js';
import {api} from '../../core/api.js';
import {confirmSheetAsk} from '../../core/confirm.js';
import {$, dateRange, el, emptyState, esc} from '../../core/dom.js';
import {exactTime, inDateRange, localDate, relTime} from '../../core/format.js';
import {EYE} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

let artifactsSig='', artFrom='', artTo='', artPage=0; const ART_PAGE=25;
// The group and parent filters. The parent is the job at the root of the chain
// that published (a watcher behind its review job), resolved by the daemon.
// NO_GROUP stands for "no group": a real group name never starts with a space.
let artGroup='', artOrigin=''; const NO_GROUP=' none';
let groupSel=null, originSel=null;
// artUnread narrows the list to the artifacts not opened yet.
let artUnread=false, unreadSeg=null;
// artQuery is the search text. Every word has to occur somewhere in the
// artifact — title, description, file name, task, parent, group or type — so
// "adr 0050" finds "ADR-0050 review" and "seo pdf" the PDF of the SEO job.
let artQuery='', searchIn=null;
const groupKey=a=>a.group||NO_GROUP;
const haystack=a=>[a.title,a.description,a.file_name,a.task_name,a.origin_name,a.group,extLabel(a.file_name,a.content_type)].join('\n').toLowerCase();
function matchesQuery(a){ const words=artQuery.toLowerCase().split(/\s+/).filter(Boolean); if(!words.length) return true;
  const h=haystack(a); return words.every(w=>h.includes(w)); }
const matchesSource=a=>(!artGroup||groupKey(a)===artGroup)&&(!artOrigin||a.origin_id===artOrigin)&&(!artUnread||a.unread);
const anyFilter=()=>!!(artFrom||artTo||artGroup||artOrigin||artUnread||artQuery);
// Refill both filter menus from the artifacts there are. The parent menu only
// offers parents in the chosen group, and a choice that no longer matches
// anything falls back to "all", so the list can never be filtered to nothing
// by a stale selection.
function fillSourceFilters(arts){
  if(!groupSel||!groupSel.isConnected) return;
  const groups=new Map(), origins=new Map();
  arts.forEach(a=>{ groups.set(groupKey(a),a.group||'No group');
    if(a.origin_id&&(!artGroup||groupKey(a)===artGroup)&&!origins.has(a.origin_id)) origins.set(a.origin_id,a.origin_name||a.origin_id); });
  if(artGroup&&!groups.has(artGroup)) artGroup='';
  if(artOrigin&&!origins.has(artOrigin)) artOrigin='';
  const byLabel=m=>[...m].sort((x,y)=>(x[0]===NO_GROUP)-(y[0]===NO_GROUP)||x[1].localeCompare(y[1]));
  const fill=(sel,all,m,cur)=>{
    const opts=[['',all],...byLabel(m)];
    const sig=JSON.stringify([opts,cur]); if(sel.dataset.sig===sig) return; sel.dataset.sig=sig;
    sel.innerHTML=''; opts.forEach(([v,l])=>{ const o=document.createElement('option'); o.value=v; o.textContent=l; sel.append(o); });
    sel.value=cur; };
  fill(groupSel,'All groups',groups,artGroup);
  fill(originSel,'All parents',origins,artOrigin);
}
function refilter(){ artPage=0; artifactsSig=''; loadArtifacts(); }
function sourceFilter(title,on){ const sel=el('select','tb-select'); sel.title=title;
  sel.onchange=e=>{ on(e.target.value); refilter(); }; return sel; }
// All | Unread, as a segmented control like the Queue's All | Active.
function unreadFilterSeg(){
  const seg=el('div','seg');
  const mk=(v,label,tip)=>{ const b=el('button',null,label); b.title=tip; b.classList.toggle('active',v===artUnread);
    b.onclick=()=>{ artUnread=v; seg.querySelectorAll('button').forEach(x=>x.classList.toggle('active',x===b)); refilter(); };
    return b; };
  seg.append(mk(false,'All','Show every artifact'), mk(true,'Unread','Show only the artifacts you have not opened yet'));
  return seg;
}
// The search field. Typing filters as you go (after a short pause, so a fast
// typist does not rebuild the list on every key); Escape clears it.
function searchField(){
  const i=el('input','search-in'); i.type='search'; i.placeholder='Search artifacts'; i.value=artQuery;
  i.setAttribute('aria-label','Search artifacts'); i.title='Search by title, description, file name, task, parent or group (⌘F)';
  let t=0;
  i.oninput=()=>{ clearTimeout(t); t=setTimeout(()=>{ if(i.value.trim()===artQuery.trim()) return; artQuery=i.value; refilter(); },120); };
  i.onkeydown=e=>{ if(e.key==='Escape'){ e.preventDefault(); if(i.value){ i.value=''; artQuery=''; refilter(); } else i.blur(); } };
  return i;
}
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
// Jump to the Log and open the log of the run that produced this artifact. The
// run may have been pruned from history (artifacts outlive runs), so check first.
async function openArtifactRun(a){
  if(!a.run_id) return;
  let runs=[]; try{ runs=await api('GET','/api/runs'); }catch(e){ toast(e.message,'err'); return; }
  const run=runs.find(r=>r.run_id===a.run_id);
  if(!run){ toast('The run that produced this artifact is no longer in history','err'); return; }
  select('news');
  showLog(run);
}
// The heading of one day's artifacts: Today, Yesterday, then the weekday and date.
function dayLabel(iso){
  const day=localDate(iso), now=new Date(), today=localDate(now.toISOString());
  const y=new Date(now); y.setDate(y.getDate()-1);
  if(day===today) return 'Today'; if(day===localDate(y.toISOString())) return 'Yesterday';
  const d=new Date(iso); const opts={weekday:'long',day:'numeric',month:'long'};
  if(d.getFullYear()!==now.getFullYear()) opts.year='numeric';
  return d.toLocaleDateString(undefined,opts);
}
// One artifact as one row: the dot, the title with its one-line summary, who
// made it and when, and the actions, which show when the pointer is on the
// row. The row itself opens the viewer; file name and size sit in the title's
// tooltip, where they are at hand without crowding the line.
function artifactRow(a){
  const row=el('div','row art-row'); row.tabIndex=0; row.setAttribute('role','button');
  if(a.unread) row.classList.add('unread');
  const gl=el('div','act-gl'); if(a.unread) gl.append(el('div','unread-dot'));
  const grow=el('div','grow');
  const title=el('div','title',esc(a.title)); title.dataset.tip=a.file_name+' · '+fmtBytes(a.size);
  const sub=el('div','sub');
  sub.innerHTML=`<span class="art-kind">${esc(extLabel(a.file_name,a.content_type))}</span>${esc(a.description||a.file_name)}`;
  grow.append(title,sub);
  // Who made it: the parent job when another job created the publisher (the
  // watcher is what one looks for, not the review job it filed), else the
  // publisher itself. The tooltip spells out the chain, and a click opens the
  // run's log while that run is still in history.
  const src=el('div','art-from');
  if(a.task_name){
    const parent=a.origin_id&&a.origin_id!==a.task_id&&a.origin_name;
    src.textContent=parent||a.task_name;
    src.dataset.tip=(parent?a.origin_name+' › ':'')+a.task_name+(a.run_id?' · click to open the run\'s log':'');
    if(a.run_id){ src.classList.add('art-src'); src.setAttribute('role','link'); src.tabIndex=0;
      src.onclick=e=>{ e.stopPropagation(); openArtifactRun(a); };
      src.onkeydown=e=>{ if(e.key==='Enter'){ e.stopPropagation(); openArtifactRun(a); } }; }
  }
  const when=el('div','art-when',esc(relTime(a.published_at))); when.dataset.tip=exactTime(a.published_at);
  const actions=el('div','row-actions art-actions');
  if(a.unread){ const mr=el('button','eye-btn',EYE); mr.dataset.tip='Mark read'; mr.setAttribute('aria-label','Mark read');
    mr.onclick=e=>{ e.stopPropagation(); readArtifact(a.id); }; actions.append(mr); }
  const del=el('button','btn small danger','Delete'); del.onclick=e=>{ e.stopPropagation(); deleteArtifact(a); }; actions.append(del);
  row.append(gl,grow,src,when,actions);
  row.onclick=()=>openViewer(a);
  row.onkeydown=e=>{ if(e.target===row&&(e.key==='Enter'||e.key===' ')){ e.preventDefault(); openViewer(a); } };
  return row;
}

// loadsGen makes the newest load the one that renders: a notification click
// alone starts three (the view switch, the jump to the artifact's page, the
// mark-read), and an earlier fetch answering last would paint the artifact
// unread again over the newer answer.
let loadsGen=0;
export async function loadArtifacts(){
  const gen=++loadsGen;
  let arts; try{ arts=await api('GET','/api/artifacts'); setConn(true);}catch(e){ setConn(false); return; }
  if(gen!==loadsGen) return;   // a newer load superseded us
  const unread=arts.filter(a=>a.unread).length; const badge=$('#artifactCount'); badge.hidden=unread===0; badge.textContent=unread;
  // With "Unread" the menus offer only groups and parents with something unread.
  fillSourceFilters(artUnread?arts.filter(a=>a.unread):arts);
  const filtered=arts.filter(shown);
  const pages=Math.max(1,Math.ceil(filtered.length/ART_PAGE));
  if(artPage>pages-1) artPage=pages-1; if(artPage<0) artPage=0;
  const pageArts=filtered.slice(artPage*ART_PAGE, artPage*ART_PAGE+ART_PAGE);
  const sig=JSON.stringify([artFrom,artTo,artGroup,artOrigin,artUnread,artQuery,artPage,arts.length,filtered.length,pageArts.map(a=>[a.id,a.unread,a.title])]);
  if(sig===artifactsSig && $('#artifacts').childElementCount) return;   // avoid flicker on poll
  artifactsSig=sig;
  const c=$('#artifacts'); c.innerHTML='';
  if(!arts.length){ c.append(emptyState('No artifacts yet','Files your tasks publish with “claudeq publish” appear here — reports, exports, HTML pages, PDFs.')); return; }
  if(!filtered.length){
    const why=artQuery?`Nothing matches “${artQuery.trim()}”. Try fewer or different words.`
      : artUnread?'Everything here has been read. Switch to All to see the rest.':'Adjust the date, group or parent filter to see artifacts.';
    c.append(emptyState('No artifacts match',why)); return; }
  // The page is split by day, one card per day, so the eye finds "this morning"
  // and "last week" without reading every time stamp.
  let day='', group=null;
  pageArts.forEach(a=>{
    const d=localDate(a.published_at);
    if(d!==day){ day=d; c.append(el('div','section-label',esc(dayLabel(a.published_at)))); group=el('div','group'); c.append(group); }
    group.append(artifactRow(a));
  });

  // Footer: count + pager, as in the Log (newest first, so "Newer" goes to
  // lower page indices).
  const inRange=anyFilter()?' matching':'';
  const foot=el('div','act-foot');
  foot.append(el('span','sub',`${filtered.length} artifact${filtered.length!==1?'s':''}${inRange} · page ${artPage+1} of ${pages}`));
  const pager=el('div','pager');
  const prev=el('button','btn small','‹ Newer'); prev.disabled=artPage<=0; prev.onclick=()=>{artPage--;artifactsSig='';loadArtifacts();};
  const next=el('button','btn small','Older ›'); next.disabled=artPage>=pages-1; next.onclick=()=>{artPage++;artifactsSig='';loadArtifacts();};
  pager.append(prev,next); foot.append(pager);
  c.append(foot);
}
// viewerGen tags each open so that async work (a text fetch) and the close
// handler never clobber a viewer that was opened after them — the most recent
// open always wins, even if the user closes and reopens in quick succession.
let viewerGen=0;
async function openViewer(a){
  const gen=++viewerGen;
  $('#viewerTitle').textContent=a.title;
  $('#viewerMeta').textContent=[a.file_name,fmtBytes(a.size),a.task_name&&('from '+a.task_name)].filter(Boolean).join(' · ');
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
  // ⌘F (Ctrl+F elsewhere) puts the cursor in the search field while the
  // Artifacts view is showing; other views keep the browser's own find, and
  // so does an open viewer, where the toolbar is behind the modal.
  document.addEventListener('keydown',e=>{
    if(current!=='artifacts'||$('#viewerSheet').open||!(e.metaKey||e.ctrlKey)||e.key!=='f'||!searchIn||!searchIn.isConnected) return;
    e.preventDefault(); searchIn.focus(); searchIn.select(); });
}
// Drop every filter, in the state and in the fields the toolbar shows: the
// toolbar is built once per view switch, so clearing the state alone would
// leave the old dates standing in it. The menus refill on the next load.
function clearFilters(){ artFrom=''; artTo=''; artGroup=''; artOrigin=''; artUnread=false; artQuery='';
  if(unreadSeg) unreadSeg.querySelectorAll('button').forEach((b,i)=>b.classList.toggle('active',i===0));
  if(searchIn) searchIn.value='';
  document.querySelectorAll('#toolbarActions .date-in').forEach(i=>i.value=''); }
const shown=a=>inDateRange(a.published_at,artFrom,artTo)&&matchesSource(a)&&matchesQuery(a);
// Open one artifact by id. This is what a click on a "new artifact"
// notification ends up calling (the app window pushes the id in from
// cqOpenPendingArtifact): show the Artifacts view, then open the artifact in
// the viewer. If it is already gone, the view alone is the fallback.
window.cqOpenArtifact=async function(id){
  select('artifacts');
  let arts; try{ arts=await api('GET','/api/artifacts'); }catch(e){ toast(e.message,'err'); return; }
  const a=arts.find(x=>x.id===id);
  if(!a){ toast('That artifact is no longer available','err'); return; }
  // Land on the artifact, so closing the viewer leaves it in the list instead
  // of on a page it is not on: filters that hide it are dropped, and the list
  // moves to the page that holds it.
  if(!shown(a)) clearFilters();
  artPage=Math.floor(arts.filter(shown).indexOf(a)/ART_PAGE);
  artifactsSig=''; loadArtifacts();
  openViewer(a);
};
async function readArtifact(id){ try{ await api('POST','/api/artifacts/'+encodeURIComponent(id)+'/read'); }catch{} artifactsSig=''; loadArtifacts(); }
async function markAllArtifactsRead(){ try{ await api('POST','/api/artifacts/read-all'); toast('All marked read','ok'); artifactsSig=''; loadArtifacts();}catch(e){toast(e.message,'err');} }
async function deleteArtifact(a){ if(await confirmSheetAsk('Delete artifact “'+a.title+'”? This removes the stored copy.')){ try{ await api('DELETE','/api/artifacts/'+encodeURIComponent(a.id)); toast('Deleted','ok'); artifactsSig=''; loadArtifacts();}catch(e){toast(e.message,'err');} } }

export const view={
  title:'Artifacts',
  toolbar(ta){
    ta.classList.add('art-toolbar');
    searchIn=searchField(); ta.append(searchIn);
    ta.append(dateRange(artFrom,artTo, v=>{artFrom=v;refilter();}, v=>{artTo=v;refilter();}));
    unreadSeg=unreadFilterSeg(); ta.append(unreadSeg);
    groupSel=sourceFilter('Show the artifacts of one queue group',v=>{artGroup=v;artOrigin='';});
    originSel=sourceFilter('Show the artifacts one job produced, directly or through the jobs it created',v=>{artOrigin=v;});
    ta.append(groupSel,originSel);
    const b=el('button','btn iconly',EYE); b.dataset.tip='Mark all read'; b.setAttribute('aria-label','Mark all read'); b.onclick=markAllArtifactsRead; ta.append(b);
  },
  enter(){ loadArtifacts(); },
  leave(){ $('#toolbarActions').classList.remove('art-toolbar'); },
  refresh(){ artifactsSig=''; loadArtifacts(); },
};

// The sheet that previews one artifact.
const sheetTemplate = `
<dialog id="viewerSheet">
  <div class="sheet-hd"><div class="grow"><b id="viewerTitle">Artifact</b><div class="sub" id="viewerMeta"></div></div>
    <div id="viewerActions" class="row-actions"></div></div>
  <div class="sheet-bd" id="viewerBody"></div>
  <div class="sheet-ft"><button class="btn" id="viewerContinueBtn" hidden data-tip="Opens Terminal in the task's folder and resumes the chat that published this artifact — with the full conversation context">Continue in Chat…</button><button class="btn" id="viewerDoneBtn">Done</button></div>
</dialog>
`;

export function mountViewer(){ document.body.insertAdjacentHTML('beforeend', sheetTemplate); }
