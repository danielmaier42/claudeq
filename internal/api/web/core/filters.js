// The filter bar the Log and the Artifacts share: a search field, a
// segmented switch, menus that fill themselves from the data, and the two
// containers a filtered list lives in. A view owns its state; this module
// owns the controls and how a query is matched.
import {el} from './dom.js';

// Every word has to occur somewhere in the text, so "adr 0050" finds
// "ADR-0050 review" and "seo pdf" the PDF of the SEO job.
export function matchesWords(query,text){
  const words=(query||'').toLowerCase().split(/\s+/).filter(Boolean);
  if(!words.length) return true;
  const h=(text||'').toLowerCase();
  return words.every(w=>h.includes(w));
}
// The search field. Typing filters as you go (after a short pause, so a fast
// typist does not rebuild the list on every key); Escape clears it, a second
// Escape leaves it.
export function searchField(initial,onChange,placeholder,tip){
  const i=el('input','search-in'); i.type='search'; i.placeholder=placeholder; i.value=initial||'';
  i.setAttribute('aria-label',placeholder); if(tip) i.title=tip;
  let t=0;
  i.oninput=()=>{ clearTimeout(t); t=setTimeout(()=>onChange(i.value),120); };
  i.onkeydown=e=>{ if(e.key==='Escape'){ e.preventDefault(); clearTimeout(t); if(i.value){ i.value=''; onChange(''); } else i.blur(); } };
  return i;
}
// A segmented switch like the Queue's All | Active. options: [value, label, tip].
// seg.set(v) moves the highlight without firing, for a reset from outside.
export function segFilter(options,cur,on){
  const seg=el('div','seg');
  const set=v=>seg.querySelectorAll('button').forEach(b=>b.classList.toggle('active',b.dataset.v===String(v)));
  options.forEach(([v,label,tip])=>{ const b=el('button',null,label); b.dataset.v=String(v); if(tip) b.title=tip;
    b.onclick=()=>{ set(v); on(v); }; seg.append(b); });
  set(cur); seg.set=set;
  return seg;
}
// A menu filter; its options come later, from the data, via fillSelect.
export function selectFilter(title,on){
  const sel=el('select','tb-select'); sel.title=title;
  sel.onchange=e=>on(e.target.value); return sel;
}
// Refill a menu from [value, label] pairs behind an "all" entry, keeping the
// current choice; the DOM is only touched when the options actually changed,
// so an open menu is not yanked shut by the poll.
export function fillSelect(sel,all,entries,cur){
  if(!sel||!sel.isConnected) return;
  const opts=[['',all],...entries];
  const sig=JSON.stringify([opts,cur]); if(sel.dataset.sig===sig) return; sel.dataset.sig=sig;
  sel.innerHTML=''; opts.forEach(([v,l])=>{ const o=document.createElement('option'); o.value=v; o.textContent=l; sel.append(o); });
  sel.value=cur;
}
// Sort [value, label] pairs by label, with the entry for "none" last.
export function byLabel(m,noneKey){ return [...m].sort((x,y)=>(x[0]===noneKey)-(y[0]===noneKey)||x[1].localeCompare(y[1])); }
// The two containers of a filtered view: the bar on top, the list below.
// listOf makes them once and hands back the list — the poll renders into it
// before the view was ever opened; filterBar also empties the bar for the
// caller to fill with rows when the view is entered.
function containers(sec){
  let bar=sec.querySelector(':scope > .filters'), list=sec.querySelector(':scope > .flist');
  if(!bar){ bar=el('div','filters'); list=el('div','flist'); sec.append(bar,list); }
  return {bar,list};
}
export function listOf(sec){ return containers(sec).list; }
export function filterBar(sec){
  const {bar,list}=containers(sec);
  bar.innerHTML='';
  const row=()=>{ const r=el('div','frow'); bar.append(r); return r; };
  return {bar,list,row};
}
