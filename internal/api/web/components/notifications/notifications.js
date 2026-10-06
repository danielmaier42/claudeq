import {openRun} from '../activity/activity.js';
import {current, select} from '../app-shell/app-shell.js';
import {openArtifact} from '../artifacts/artifacts.js';
import {setConn} from '../status/status.js';
import {api} from '../../core/api.js';
import {$, el, emptyState, esc} from '../../core/dom.js';
import {dayLabel, exactTime, localDate, relTime} from '../../core/format.js';
import {byLabel, fillSelect, filterBar, listOf, matchesWords, searchField, segFilter, selectFilter} from '../../core/filters.js';
import {EYE} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

// The Notifications view: every notification the daemon sent — a run's
// outcome, a new artifact, a provider in trouble, what a task said with
// `claudeq notify` — as it was shown, newest first, with the ones not looked
// at yet counted in the sidebar. A row does what clicking the macOS
// notification does: it turns read and what it is about opens.

let notifSig='', ntfPage=0; const NTF_PAGE=25;
let ntfUnread=false, ntfKind='', ntfQuery='';
let searchIn=null, unreadSeg=null, kindSel=null, listEl=null;
const KIND_LABELS={success:'Run succeeded',failure:'Run failed',artifact:'New artifact',provider:'Provider',task:'From a task'};
const kindLabel=k=>KIND_LABELS[k]||'Other';
// The glyph in the gutter says at a glance what kind of notification a row is.
const KIND_GLYPHS={success:'✓',failure:'✗',artifact:'📄',provider:'⚠',task:'🔔'};
// Inside the ClaudeQ window the sender's name is noise, and the glyph the
// daemon puts in a title is already in the gutter: "ClaudeQ ✗ nightly failed"
// reads as "nightly failed".
const shortTitle=t=>String(t||'').replace(/^ClaudeQ\s*:?\s*/,'').replace(/^(?:✓|✗|📄)\s*/u,'');
const haystack=n=>[n.title,n.message,n.task_name,kindLabel(n.kind)].join('\n');
const shown=n=>(!ntfUnread||n.unread)&&(!ntfKind||n.kind===ntfKind)&&matchesWords(ntfQuery,haystack(n));
const anyFilter=()=>!!(ntfUnread||ntfKind||ntfQuery);
function refilter(){ ntfPage=0; notifSig=''; loadNotifications(); }
// The kind menu offers the kinds there are; a choice that no longer matches
// anything falls back to "all".
function fillKindFilter(notes){
  const kinds=new Map(); notes.forEach(n=>kinds.set(n.kind||'',kindLabel(n.kind)));
  if(ntfKind&&!kinds.has(ntfKind)) ntfKind='';
  fillSelect(kindSel,'All kinds',byLabel(kinds),ntfKind);
}

// One notification as one row: the dot, the kind's glyph, the title with the
// message under it, when it was sent, and the mark-read eye, which shows when
// the pointer is on the row.
function notificationRow(n){
  const row=el('div','row ntf-row'); row.tabIndex=0; row.setAttribute('role','button');
  if(n.unread) row.classList.add('unread');
  const gl=el('div','ntf-gl'); if(n.unread) gl.append(el('div','unread-dot'));
  const glyph=el('div','ntf-glyph',esc(KIND_GLYPHS[n.kind]||'•')); glyph.dataset.tip=kindLabel(n.kind);
  const grow=el('div','grow');
  grow.append(el('div','title',esc(shortTitle(n.title))), el('div','sub ntf-msg',esc(n.message)));
  const when=el('div','ntf-when',esc(relTime(n.sent_at))); when.dataset.tip=exactTime(n.sent_at);
  const actions=el('div','row-actions ntf-actions');
  if(n.unread){ const mr=el('button','eye-btn',EYE); mr.dataset.tip='Mark read'; mr.setAttribute('aria-label','Mark read');
    mr.onclick=e=>{ e.stopPropagation(); readNotification(n.id); }; actions.append(mr); }
  row.append(gl,glyph,grow,when,actions);
  row.onclick=()=>openNotification(n);
  row.onkeydown=e=>{ if(e.target===row&&(e.key==='Enter'||e.key===' ')){ e.preventDefault(); openNotification(n); } };
  return row;
}

// loadsGen makes the newest load the one that renders: a click starts several
// (the mark-read, the view switch), and an earlier fetch answering last would
// paint the notification unread again over the newer answer.
let loadsGen=0;
export async function loadNotifications(){
  const gen=++loadsGen;
  let notes; try{ notes=await api('GET','/api/notifications'); setConn(true);}catch(e){ setConn(false); return; }
  if(gen!==loadsGen) return;   // a newer load superseded us
  const unread=notes.filter(n=>n.unread).length; const badge=$('#notifCount'); badge.hidden=unread===0; badge.textContent=unread;
  // With "Unread" the menu offers only kinds with something unread.
  fillKindFilter(ntfUnread?notes.filter(n=>n.unread):notes);
  const filtered=notes.filter(shown);
  const pages=Math.max(1,Math.ceil(filtered.length/NTF_PAGE));
  if(ntfPage>pages-1) ntfPage=pages-1; if(ntfPage<0) ntfPage=0;
  const pageNotes=filtered.slice(ntfPage*NTF_PAGE, ntfPage*NTF_PAGE+NTF_PAGE);
  if(!listEl) listEl=listOf($('#notifications'));
  const sig=JSON.stringify([ntfUnread,ntfKind,ntfQuery,ntfPage,notes.length,filtered.length,pageNotes.map(n=>[n.id,n.unread])]);
  if(sig===notifSig && listEl.childElementCount) return;   // avoid flicker on poll
  notifSig=sig;
  const c=listEl; c.innerHTML='';
  if(!notes.length){ c.append(emptyState('No notifications yet','Everything ClaudeQ announces — a run that failed, a new artifact, a message from a task — is listed here, even after the macOS banner is gone.')); return; }
  if(!filtered.length){
    const why=ntfQuery?`Nothing matches “${ntfQuery.trim()}”. Try fewer or different words.`
      : ntfUnread?'Everything here has been read. Switch to All to see the rest.':'Pick another kind to see notifications.';
    c.append(emptyState('No notifications match',why)); return; }
  // Split by day, one card per day, as in Artifacts.
  let day='', group=null;
  pageNotes.forEach(n=>{
    const d=localDate(n.sent_at);
    if(d!==day){ day=d; c.append(el('div','section-label',esc(dayLabel(n.sent_at)))); group=el('div','group'); c.append(group); }
    group.append(notificationRow(n));
  });
  const inRange=anyFilter()?' matching':'';
  const foot=el('div','act-foot');
  foot.append(el('span','sub',`${filtered.length} notification${filtered.length!==1?'s':''}${inRange} · page ${ntfPage+1} of ${pages}`));
  const pager=el('div','pager');
  const prev=el('button','btn small','‹ Newer'); prev.disabled=ntfPage<=0; prev.onclick=()=>{ntfPage--;notifSig='';loadNotifications();};
  const next=el('button','btn small','Older ›'); next.disabled=ntfPage>=pages-1; next.onclick=()=>{ntfPage++;notifSig='';loadNotifications();};
  pager.append(prev,next); foot.append(pager);
  c.append(foot);
}

// Open what a notification is about: the artifact in the viewer, the run's
// log, the link in the browser. A notification about nothing in particular (a
// provider's state) has nowhere to go; the row only turns read. Reports
// whether anything opened.
async function openTarget(t){
  if(t.artifact_id){ openArtifact(t.artifact_id); return true; }
  if(t.run_id) return openRun(t.run_id);
  if(t.url){ if(window.cqOpenExternal) window.cqOpenExternal(t.url); else window.open(t.url,'_blank','noopener'); return true; }
  return false;
}
// A row does what the macOS notification does on click. The local flag is
// cleared first so the list does not paint the row unread while the daemon
// answers, and the target is not kept waiting for that answer.
function openNotification(n){
  if(n.unread){ n.unread=false; readNotification(n.id); }
  openTarget(n);
}
// What a click on a macOS notification ends up calling: the app window hands
// the clicked notification in (cmd/claudeqapp/main_darwin.go). It is marked
// read here, the way a row click is, and what it is about opens; a
// notification about nothing in particular — or one whose run has left
// history — lands in this view.
window.cqOpenNotificationTarget=async function(t){
  if(t&&t.id) readNotification(t.id); else { notifSig=''; loadNotifications(); }
  if(t&&await openTarget(t)) return;
  select('notifications');
};
async function readNotification(id){ try{ await api('POST','/api/notifications/'+encodeURIComponent(id)+'/read'); }catch{} notifSig=''; loadNotifications(); }
async function markAllRead(){ try{ await api('POST','/api/notifications/read-all'); toast('All marked read','ok'); notifSig=''; loadNotifications();}catch(e){toast(e.message,'err');} }

// The filter bar sits above the list: the search, All | Unread, and the kind
// menu. Built on every entry so the fields show the state.
function renderFilters(){
  const {list,row}=filterBar($('#notifications')); listEl=list;
  const top=row();
  searchIn=searchField(ntfQuery,v=>{ if(v.trim()===ntfQuery.trim()) return; ntfQuery=v; refilter(); },
    'Search notifications','Search by title, message, task or kind (⌘F)');
  unreadSeg=segFilter([[false,'All','Show every notification'],[true,'Unread','Show only the notifications you have not looked at yet']],ntfUnread,v=>{ ntfUnread=v; refilter(); });
  kindSel=selectFilter('Show one kind of notification',v=>{ ntfKind=v; refilter(); });
  top.append(searchIn,unreadSeg,el('span','spacer'),kindSel);
}
// ⌘F (Ctrl+F elsewhere) puts the cursor in the search field while this view
// is showing and no sheet is open; otherwise the browser keeps its own find.
document.addEventListener('keydown',e=>{
  if(current!=='notifications'||document.querySelector('dialog[open]')||!(e.metaKey||e.ctrlKey)||e.key!=='f'||!searchIn||!searchIn.isConnected) return;
  e.preventDefault(); searchIn.focus(); searchIn.select(); });

export const view={
  title:'Notifications',
  toolbar(ta){
    const b=el('button','btn',EYE+'<span>Mark all read</span>'); b.onclick=markAllRead; ta.append(b);
  },
  enter(){ renderFilters(); notifSig=''; loadNotifications(); },
  refresh(){ notifSig=''; loadNotifications(); },
};
