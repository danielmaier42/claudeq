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
// global system prompt, which has none). kind is 'task', 'system' or 'script',
// or a function returning one when the same box can hold either a prompt or a
// script.
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
// reviewContext asks the daemon whether a review can run at all and who would
// answer it. It costs nothing — no model is involved — and only the daemon can
// say: "the default provider" resolves to a different account and model as
// Settings change, and a provider whose CLI is gone cannot review anything. A
// remembered finding must outlive neither answer.
async function reviewContext(){
  try{ return await api('GET','/api/review/context'); }
  catch{ return null; }   // an unreachable daemon says nothing about the prompt
}

// reviewKey hashes the question rather than keeping a copy of it: prompts are
// long, and the cache only ever has to tell one question apart from another
// (FNV-1a). The answer is stored as it came, rewrite included, because that is
// what the banner shows.
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
  const kindNow=typeof kind==='function'?kind:()=>kind;

  // stop() retires whatever is pending or in flight: the bumped sequence number
  // makes any answer still on its way arrive too late to be rendered.
  function stop(){ clearTimeout(timer); timer=null; if(ctrl){ ctrl.abort(); ctrl=null; } seq++; }
  function hide(){ banner.hidden=true; banner.innerHTML=''; banner.classList.remove('busy'); }
  function head(cls,who){ banner.className='ai-banner'+(cls?' '+cls:''); banner.hidden=false;
    banner.innerHTML=`<span class="spark">${SPARK}</span><div class="grow"><span class="who">${esc(who)}</span></div>`;
    return banner.querySelector('.grow'); }

  // remembered marks a finding that comes from the cache rather than from a
  // review just made: it is shown as what it is, and can be re-checked on the
  // spot, because the prompt may be fine by now without the text having changed
  // (the missing file was created, the folder exists).
  function show(res,key,remembered,noun){
    const box=head('',remembered?'ClaudeQ suggested earlier:':'ClaudeQ suggests:');
    box.append(el('div','msg',esc(res.message)));
    const acts=el('div','acts'); box.append(acts);
    if(res.revised_prompt){
      const ap=el('button','btn small ai','Apply');
      ap.dataset.tip='Rewrites the '+noun+' as suggested. The result lands in the box above, where you can still change it.';
      ap.onclick=()=>{ area.value=res.revised_prompt; toast((noun==='script'?'Script':'Prompt')+' rewritten','ok'); run(); };
      acts.append(ap);
    }
    // Dismissed for good: forgetting the answer stops it from coming back the
    // next time this sheet opens, and re-reviewing is one edit away.
    const no=el('button','btn small','Dismiss'); no.onclick=()=>{ stop(); hide(); if(key) reviewForget(key); }; acts.append(no);
    if(remembered){
      const again=el('button','btn small','Check again');
      again.dataset.tip='Asks Claude about this '+noun+' again. Costs a little usage.';
      again.onclick=()=>{ if(key) reviewForget(key); run(); };
      acts.append(again);
    }
  }

  function render(res,key,remembered,noun){ if(res && res.enabled && !res.ok && res.message) show(res,key,remembered,noun); else hide(); }

  // run asks Claude about what is in the box now; restore only shows what was
  // already found out about it. Opening a sheet uses restore, so looking at a
  // task twice costs nothing — only editing it asks again.
  async function restore(){
    stop();
    const mine=seq;
    const prompt=area.value, workingDir=dir(), k=kindNow(), noun=k==='script'?'script':'prompt';
    if(!prompt.trim() || (k!=='system' && !workingDir)){ hide(); return; }
    const ctx=await reviewContext();
    if(mine!==seq) return;
    if(!ctx || !ctx.enabled){ hide(); return; }   // no review to remember anything about
    const key=reviewKey(k,prompt,workingDir,ctx.reviewer);
    render(reviewCached(key),key,true,noun);
  }

  async function run(){
    stop();
    const mine=seq;
    const prompt=area.value, workingDir=dir(), k=kindNow(), noun=k==='script'?'script':'prompt';
    // An imported task lands here with its folder dropped, because the exporter's
    // path is not on this Mac. Reviewing then would call every relative path
    // unresolvable and bury the sheet's own "choose a folder" hint under it, so
    // the review waits for the folder — chooseFolder starts it.
    if(!prompt.trim() || (k!=='system' && !workingDir)){ hide(); return; }
    // Who reviews is part of the question, so an answer can be filed under it.
    // When the daemon cannot say, the review still goes ahead — the endpoint
    // decides anyway — and its answer is simply not remembered.
    const ctx=await reviewContext();
    if(mine!==seq) return;
    if(ctx && !ctx.enabled){ hide(); return; }
    const key=ctx?reviewKey(k,prompt,workingDir,ctx.reviewer):null;
    const cached=key?reviewCached(key):null;
    if(cached){ render(cached,key,true,noun); return; }
    ctrl=new AbortController();
    head('busy','ClaudeQ is checking this '+noun+'…');
    let res;
    // A review that is aborted, unreachable or fails is silently dropped: it is
    // an extra pair of eyes, and a broken one must never get in the way of
    // writing a task.
    try{ res=await api('POST','/api/review/prompt',{kind:k,prompt,working_dir:workingDir},ctrl.signal); }
    catch(e){ if(mine===seq) hide(); return; }
    if(mine!==seq) return;
    ctrl=null;
    if(key && res && res.enabled) reviewRemember(key,res);   // a disabled or superseded answer says nothing about this prompt
    render(res,key,false,noun);
  }

  return { run, restore, stop, reset(){ stop(); hide(); }, schedule(){ stop(); hide(); timer=setTimeout(run,REVIEW_DELAY); } };
}

// The task sheet's own review controller, bound once the sheet is in the page.
export let taskReview=null;
export function initTaskReview(){
  // A script job is reviewed as what it is — a program run under the daemon's
  // environment — so the kind is read when the review starts, not fixed here.
  const kind=()=>{ const seg=$('#f-kind'); return seg && seg.dataset.value==='script'?'script':'task'; };
  taskReview=makeReview({kind,area:$('#f-prompt'),banner:$('#f-review'),dir:()=>$('#f-dir').value.trim()});
  $('#f-prompt').addEventListener('input',()=>taskReview.schedule());
  $('#addSheet').addEventListener('close',()=>taskReview.reset());
}
