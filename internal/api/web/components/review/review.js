import {api} from '../../core/api.js';
import {$, el, esc} from '../../core/dom.js';
import {SPARK} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

/* ---- Prompt review: Claude checks a draft prompt against this machine ---- */
// Purely advisory — it never blocks saving. A review costs real Claude usage, so
// it runs only when the prompt or the working directory actually changes, never
// merely because a sheet was opened: reopening a task nobody has touched shows
// the finding from last time, or nothing, and asks Claude for nothing. A change
// really is analysed from scratch, because a finding about the previous text
// says nothing about the new one, and whatever is in flight is aborted first
// (which kills the daemon's Claude process too), so only the newest answer can
// reach the banner.
const REVIEW_DELAY=900;   // ms of quiet typing before a review is worth starting

// area is the textarea under review, banner the element to render into, and
// dir() the working directory to resolve relative paths against ('' for the
// global system prompt, which has none).
// Answers are remembered on disk, keyed by a hash of exactly what was asked, so
// reopening the same task's sheet — or the app itself — shows the last finding
// instead of paying for the same review again. Entries are dropped after a day
// because the machine they judge may have moved on since.
const REVIEW_STORE='cq.prompt-review';
const REVIEW_CACHE_TTL=86400000, REVIEW_CACHE_MAX=40;
let reviewCache=null;
// Storage can be unavailable or hold something else's data; a cache is never
// worth an exception on the way to writing a task.
function reviewStore(){
  if(reviewCache) return reviewCache;
  reviewCache=new Map();
  try{ const raw=JSON.parse(localStorage.getItem(REVIEW_STORE)||'[]');
    if(Array.isArray(raw)) for(const e of raw) if(e&&e.k&&e.at) reviewCache.set(e.k,{at:e.at,res:e.res}); }catch{}
  return reviewCache;
}
function reviewPersist(){
  const m=reviewStore();
  try{ localStorage.setItem(REVIEW_STORE,JSON.stringify([...m].map(([k,v])=>({k,at:v.at,res:v.res})))); }catch{}
}
// reviewSettings asks the daemon who reviews, and whether reviews happen at
// all. It costs nothing (no model is involved), and a remembered finding must
// not outlive either answer: switching the review off has to clear the banner,
// and another reviewer may judge the same prompt differently.
async function reviewSettings(){
  try{ const s=await api('GET','/api/settings');
    return {on:!s.prompt_review_disabled,
            who:(s.prompt_review_provider||'')+'/'+(s.prompt_review_model||'')}; }
  catch{ return null; }   // unreadable settings say nothing about the prompt
}

// reviewKey hashes the question instead of storing it: prompts are long, and the
// cache only ever has to tell one question apart from another (FNV-1a).
function reviewKey(kind,prompt,dir,who){
  const s=JSON.stringify([kind,prompt,dir,who]);
  let h=0x811c9dc5;
  for(let i=0;i<s.length;i++){ h^=s.charCodeAt(i); h=Math.imul(h,0x01000193); }
  return kind+':'+s.length+':'+(h>>>0).toString(36);
}
function reviewCached(key){ const m=reviewStore(), hit=m.get(key);
  if(!hit) return null;
  if(Date.now()-hit.at>REVIEW_CACHE_TTL){ m.delete(key); reviewPersist(); return null; }  // the machine may have changed since
  return hit.res; }
function reviewForget(key){ const m=reviewStore(); if(m.delete(key)) reviewPersist(); }
function reviewRemember(key,res){ const m=reviewStore(); m.set(key,{at:Date.now(),res});
  while(m.size>REVIEW_CACHE_MAX) m.delete(m.keys().next().value);
  reviewPersist(); }

export function makeReview({kind,area,banner,dir}){
  let timer=null, ctrl=null, seq=0;

  // stop() retires whatever is pending or in flight: the bumped sequence number
  // makes any answer still on its way arrive too late to be rendered.
  function stop(){ clearTimeout(timer); timer=null; if(ctrl){ ctrl.abort(); ctrl=null; } seq++; }
  function hide(){ banner.hidden=true; banner.innerHTML=''; banner.classList.remove('busy'); }
  function head(cls,who){ banner.className='ai-banner'+(cls?' '+cls:''); banner.hidden=false;
    banner.innerHTML=`<span class="spark">${SPARK}</span><div class="grow"><span class="who">${esc(who)}</span></div>`;
    return banner.querySelector('.grow'); }

  function show(res,key){
    const box=head('','ClaudeQ suggests:');
    box.append(el('div','msg',esc(res.message)));
    const acts=el('div','acts'); box.append(acts);
    if(res.revised_prompt){
      const ap=el('button','btn small ai','Apply');
      ap.dataset.tip='Rewrites the prompt as suggested. The result lands in the box above, where you can still change it.';
      ap.onclick=()=>{ area.value=res.revised_prompt; toast('Prompt rewritten','ok'); run(); };
      acts.append(ap);
    }
    // Dismissed for good: forgetting the answer stops it from coming back the
    // next time this sheet opens, and re-reviewing is one edit away.
    const no=el('button','btn small','Dismiss'); no.onclick=()=>{ stop(); hide(); reviewForget(key); }; acts.append(no);
  }

  function render(res,key){ if(res && res.enabled && !res.ok && res.message) show(res,key); else hide(); }

  // run asks Claude about what is in the box now; restore only shows what was
  // already found out about it. Opening a sheet uses restore, so looking at a
  // task twice costs nothing — only editing it asks again.
  async function restore(){
    stop();
    const mine=seq;
    const prompt=area.value, workingDir=dir();
    if(!prompt.trim() || (kind==='task' && !workingDir)){ hide(); return; }
    const st=await reviewSettings();
    if(mine!==seq) return;
    if(!st || !st.on){ hide(); return; }
    const key=reviewKey(kind,prompt,workingDir,st.who);
    render(reviewCached(key),key);
  }

  async function run(){
    stop();
    const mine=seq;
    const prompt=area.value, workingDir=dir();
    // An imported task lands here with its folder dropped, because the exporter's
    // path is not on this Mac. Reviewing then would call every relative path
    // unresolvable and bury the sheet's own "choose a folder" hint under it, so
    // the review waits for the folder — chooseFolder starts it.
    if(!prompt.trim() || (kind==='task' && !workingDir)){ hide(); return; }
    const st=await reviewSettings();
    if(mine!==seq) return;
    if(!st || !st.on){ hide(); return; }
    const key=reviewKey(kind,prompt,workingDir,st.who);
    const cached=reviewCached(key);
    if(cached){ render(cached,key); return; }
    ctrl=new AbortController();
    head('busy','ClaudeQ is checking this prompt…');
    let res;
    // A review that is aborted, unreachable or fails is silently dropped: it is
    // an extra pair of eyes, and a broken one must never get in the way of
    // writing a task.
    try{ res=await api('POST','/api/review/prompt',{kind,prompt,working_dir:workingDir},ctrl.signal); }
    catch(e){ if(mine===seq) hide(); return; }
    if(mine!==seq) return;
    ctrl=null;
    if(res && res.enabled) reviewRemember(key,res);   // a disabled or superseded answer says nothing about this prompt
    render(res,key);
  }

  return { run, restore, stop, reset(){ stop(); hide(); }, schedule(){ stop(); hide(); timer=setTimeout(run,REVIEW_DELAY); } };
}

// The task sheet's own review controller, bound once the sheet is in the page.
export let taskReview=null;
export function initTaskReview(){
  taskReview=makeReview({kind:'task',area:$('#f-prompt'),banner:$('#f-review'),dir:()=>$('#f-dir').value.trim()});
  $('#f-prompt').addEventListener('input',()=>taskReview.schedule());
  // A hand-typed folder changes what the prompt's relative paths mean, so a
  // finding about the old one is retired at once. Reviewing every prefix of a
  // path being typed would only ask about directories that do not exist yet, so
  // the new one is reviewed when it is picked, or when the prompt next changes.
  $('#f-dir').addEventListener('input',()=>taskReview.reset());
  $('#addSheet').addEventListener('close',()=>taskReview.reset());
}
