import {api} from '../../core/api.js';
import {$, el, esc} from '../../core/dom.js';
import {SPARK} from '../../core/icons.js';
import {toast} from '../../core/toast.js';

/* ---- Prompt review: Claude checks a draft prompt against this machine ---- */
// Purely advisory — it never blocks saving. A review runs when a prompt sheet
// opens and again, from scratch, on every change to the prompt or the working
// directory, because a finding about the previous text says nothing about the
// new one. Whatever is in flight is aborted first (which kills the daemon's
// Claude process too), so only the newest answer can reach the banner.
const REVIEW_DELAY=900;   // ms of quiet typing before a review is worth starting

// area is the textarea under review, banner the element to render into, and
// dir() the working directory to resolve relative paths against ('' for the
// global system prompt, which has none).
// Answers are remembered for a few minutes, keyed by exactly what was asked, so
// reopening the same task's sheet or stepping back into Settings shows the last
// finding instead of paying for the same review again. Any edit changes the key
// and really is analysed from scratch.
const reviewCache=new Map();
const REVIEW_CACHE_TTL=180000, REVIEW_CACHE_MAX=20;
function reviewCached(key){ const hit=reviewCache.get(key);
  if(!hit) return null;
  if(Date.now()-hit.at>REVIEW_CACHE_TTL){ reviewCache.delete(key); return null; }  // the machine may have changed since
  return hit.res; }
function reviewRemember(key,res){ reviewCache.set(key,{at:Date.now(),res});
  while(reviewCache.size>REVIEW_CACHE_MAX) reviewCache.delete(reviewCache.keys().next().value); }

export function makeReview({kind,area,banner,dir}){
  let timer=null, ctrl=null, seq=0;

  // stop() retires whatever is pending or in flight: the bumped sequence number
  // makes any answer still on its way arrive too late to be rendered.
  function stop(){ clearTimeout(timer); timer=null; if(ctrl){ ctrl.abort(); ctrl=null; } seq++; }
  function hide(){ banner.hidden=true; banner.innerHTML=''; banner.classList.remove('busy'); }
  function head(cls,who){ banner.className='ai-banner'+(cls?' '+cls:''); banner.hidden=false;
    banner.innerHTML=`<span class="spark">${SPARK}</span><div class="grow"><span class="who">${esc(who)}</span></div>`;
    return banner.querySelector('.grow'); }

  function show(res){
    const box=head('','ClaudeQ suggests:');
    box.append(el('div','msg',esc(res.message)));
    const acts=el('div','acts'); box.append(acts);
    if(res.revised_prompt){
      const ap=el('button','btn small ai','Apply');
      ap.dataset.tip='Rewrites the prompt as suggested. The result lands in the box above, where you can still change it.';
      ap.onclick=()=>{ area.value=res.revised_prompt; toast('Prompt rewritten','ok'); run(); };
      acts.append(ap);
    }
    const no=el('button','btn small','Dismiss'); no.onclick=()=>{ stop(); hide(); }; acts.append(no);
  }

  function render(res){ if(res && res.enabled && !res.ok && res.message) show(res); else hide(); }

  async function run(){
    stop();
    const mine=seq;
    const prompt=area.value, workingDir=dir();
    // An imported task lands here with its folder dropped, because the exporter's
    // path is not on this Mac. Reviewing then would call every relative path
    // unresolvable and bury the sheet's own "choose a folder" hint under it, so
    // the review waits for the folder — chooseFolder starts it.
    if(!prompt.trim() || (kind==='task' && !workingDir)){ hide(); return; }
    const key=JSON.stringify([kind,prompt,workingDir]);
    const cached=reviewCached(key);
    if(cached){ render(cached); return; }
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
    render(res);
  }

  return { run, stop, reset(){ stop(); hide(); }, schedule(){ stop(); hide(); timer=setTimeout(run,REVIEW_DELAY); } };
}

// The task sheet's own review controller, bound once the sheet is in the page.
export let taskReview=null;
export function initTaskReview(){
  taskReview=makeReview({kind:'task',area:$('#f-prompt'),banner:$('#f-review'),dir:()=>$('#f-dir').value.trim()});
  $('#f-prompt').addEventListener('input',()=>taskReview.schedule());
  $('#addSheet').addEventListener('close',()=>taskReview.reset());
}
