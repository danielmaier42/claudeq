import {current} from '../app-shell/app-shell.js';
import {api} from '../../core/api.js';
import {$, el, esc, httpsOnly} from '../../core/dom.js';
import {toast} from '../../core/toast.js';

let fbInfo=null, fbSession='', fbMode='chat', fbSaid=[], fbLabels=[], fbBusy=false;
async function openFeedback(){
  fbSession=''; fbSaid=[]; fbLabels=[];
  $('#fb-err').textContent=''; $('#fb-chat').innerHTML=''; $('#fb-chat').hidden=true;
  $('#fb-text').value=''; $('#fb-title').value=''; $('#fb-body').value='';
  $('#fb-manual').hidden=true;
  $('#fb-ask-label').textContent='What would you like to report?';
  $('#fb-text').placeholder='Something that does not work, or something ClaudeQ should be able to do…';
  try{ fbInfo=await api('GET','/api/feedback'); }
  catch(e){ fbInfo={available:false,reason:e.message,repo:'',app_version:'',os_version:''}; }
  // The status request can land after the user has already moved on, and the
  // page's buttons leave with the toolbar.
  if(current!=='feedback') return;
  $('#fb-repo').textContent=fbInfo.repo||'GitHub';
  fbRenderMeta();
  if(fbInfo.available){ fbSetMode('chat'); }
  else { fbSetMode('review'); $('#fb-err').textContent='The assistant is unavailable ('+(fbInfo.reason||'unknown reason')+'). Write the issue yourself:'; }
  (fbInfo.available?$('#fb-text'):$('#fb-title')).focus();
}
// The environment line and the labels are stated here rather than edited here:
// they travel in the prefilled page, where they can still be removed.
function fbRenderMeta(){
  const env=[fbInfo.app_version&&('ClaudeQ '+fbInfo.app_version), fbInfo.os_version&&('macOS '+fbInfo.os_version)].filter(Boolean).join(' · ');
  const parts=[];
  if(env) parts.push(env+' is appended at the end of the issue.');
  if(fbLabels.length) parts.push('Labels: '+fbLabels.join(', ')+'.');
  $('#fb-meta').textContent=parts.join(' ');
}
function fbSetMode(m){ fbMode=m;
  $('#fb-ask').hidden=m!=='chat'; $('#fb-review').hidden=m!=='review';
  $('#fb-send').textContent=m==='chat'?'Send':'Open on GitHub';
  if(m==='review') $('#fb-manual').hidden=true;
}
function fbBubble(kind,who,text){ $('#fb-chat').hidden=false;
  const m=el('div','msg '+kind,'<div class="who">'+esc(who)+'</div>'+esc(text));
  $('#fb-chat').append(m); $('#fb-chat').scrollTop=$('#fb-chat').scrollHeight; return m; }
function fbSetBusy(on){ fbBusy=on; $('#fb-send').disabled=on; $('#fb-text').disabled=on; }
// Cmd/Ctrl+Enter sends, like the other multi-line inputs in the app.
export function initFeedback(){
  $('#fb-text').addEventListener('keydown',e=>{ if((e.metaKey||e.ctrlKey)&&e.key==='Enter'){ e.preventDefault(); $('#fb-send').click(); } });
}
async function fbSendTurn(){
  const text=$('#fb-text').value.trim();
  if(!text){ $('#fb-err').textContent='Write what you would like to report first.'; return; }
  $('#fb-err').textContent=''; fbSaid.push(text); fbBubble('user','You',text);
  $('#fb-text').value=''; fbSetBusy(true);
  const wait=fbBubble('assistant','Claude','Thinking…');
  try{
    const d=await api('POST','/api/feedback/turn',{session_id:fbSession,text});
    wait.remove(); fbSession=d.session_id||'';
    if(d.status==='ask'){ fbBubble('assistant','Claude',d.question||'');
      $('#fb-ask-label').textContent='Your answer'; $('#fb-text').placeholder='Answer Claude\u2019s question…'; $('#fb-text').focus(); }
    else { $('#fb-title').value=d.title||''; $('#fb-body').value=d.body||''; fbLabels=d.labels||[]; fbRenderMeta();
      fbBubble('result','Claude','Drafted the issue below — check it before filing.');
      fbSetMode('review'); $('#fb-title').focus(); }
  }catch(e){
    wait.remove(); fbBubble('err-msg','Error',e.message);
    $('#fb-err').textContent='The assistant could not draft the issue.';
    $('#fb-manual').hidden=false; fbSession='';
  }finally{ fbSetBusy(false); }
}
async function fbOpenIssue(){
  const title=$('#fb-title').value.trim();
  if(!title){ $('#fb-err').textContent='The issue needs a title.'; $('#fb-title').focus(); return; }
  $('#fb-err').textContent=''; fbSetBusy(true);
  try{
    const r=await api('POST','/api/feedback/url',{title,body:$('#fb-body').value,labels:fbLabels});
    const u=httpsOnly(r.url);
    if(!u) throw new Error('the issue link could not be built');
    if(window.cqOpenExternal) window.cqOpenExternal(u); else window.open(u,'_blank','noopener');
    toast('Opened GitHub — press “Create” there to file it','ok');
  }catch(e){ $('#fb-err').textContent=e.message; }
  finally{ fbSetBusy(false); }
}

// Keep the artifact unread badge current on every tab; re-render the list when
// it's the open tab.

export const view={
  title:'Feedback',
  // The page's actions sit in the toolbar, where every other page's do.
  // "Write it myself" is the fallback when the assistant fails mid-conversation:
  // it keeps what was typed and lets the user finish the issue by hand.
  toolbar(ta){
    const manual=el('button','btn','Write it myself'); manual.id='fb-manual'; manual.hidden=true;
    manual.onclick=()=>{ $('#fb-err').textContent=''; $('#fb-body').value=fbSaid.join('\n\n'); fbSetMode('review'); $('#fb-title').focus(); };
    const send=el('button','btn primary','Continue'); send.id='fb-send';
    send.onclick=()=>{ if(fbBusy) return; if(fbMode==='chat') fbSendTurn(); else fbOpenIssue(); };
    ta.append(manual, send);
  },
  enter(){ openFeedback(); },
};

const template = `
  <div id="fb-chat" class="chat" hidden></div>
  <div id="fb-ask">
    <label class="fld" id="fb-ask-label">What would you like to report?</label>
    <textarea id="fb-text" rows="5" placeholder="Something that does not work, or something ClaudeQ should be able to do…"></textarea>
    <div class="hint">Claude turns this into a GitHub issue and may ask one short question first.</div>
  </div>
  <div id="fb-review" hidden>
    <label class="fld">Title</label><input type="text" id="fb-title" placeholder="Short, concrete title">
    <label class="fld">Description</label><textarea id="fb-body" rows="12" placeholder="What happens, and what you expected"></textarea>
    <div class="hint" style="margin-top:12px" id="fb-meta"></div>
    <div class="hint" style="margin-top:8px">Nothing has been sent yet. The button opens a prefilled new issue on <span id="fb-repo" class="mono"></span> in your browser; it is created only when you press <b>Create</b> there — and you can still change or delete anything on that page.</div>
  </div>
  <p id="fb-err" class="hint" style="color:var(--danger)"></p>
`;

export function mountFeedback(){ $('#feedback').innerHTML = template; }
