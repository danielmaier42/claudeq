export const $=s=>document.querySelector(s);
export const el=(t,c,h)=>{const e=document.createElement(t); if(c)e.className=c; if(h!=null)e.innerHTML=h; return e;};

// Cursor-following tooltip for [data-tip] elements. WKWebView doesn't render
// native title= tooltips, so we draw our own.
const _tip=el('div','tip'); _tip.hidden=true; _tip.setAttribute('popover','manual'); document.body.append(_tip);
function _posTip(e){ const pad=12, r=_tip.getBoundingClientRect(); let x=e.clientX+pad, y=e.clientY+pad;
  if(x+r.width>innerWidth) x=e.clientX-r.width-pad; if(y+r.height>innerHeight) y=e.clientY-r.height-pad;
  _tip.style.left=Math.max(4,x)+'px'; _tip.style.top=Math.max(4,y)+'px'; }
// Shown as a manual popover: the top layer paints above modal <dialog>s, which
// would otherwise cover tooltips triggered from inside a sheet.
document.addEventListener('mouseover',e=>{ const t=e.target.closest&&e.target.closest('[data-tip]'); if(!t)return; _tip.textContent=t.getAttribute('data-tip'); _tip.hidden=false; try{_tip.showPopover();}catch{} _posTip(e); });
document.addEventListener('mousemove',e=>{ if(!_tip.hidden) _posTip(e); });
document.addEventListener('mouseout',e=>{ const t=e.target.closest&&e.target.closest('[data-tip]'); if(t){ _tip.hidden=true; try{_tip.hidePopover();}catch{} } });

// WKWebView doesn't open target=_blank links; route external links through the
// native app (which opens them in the default browser). Harmless in a browser.
document.addEventListener('click',e=>{
  const a=e.target.closest&&e.target.closest('a[href^="http"]');
  if(a&&window.cqOpenExternal){ e.preventDefault(); window.cqOpenExternal(a.href); }
},true);
export const esc=s=>(s??'').toString().replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
// Only allow http(s) URLs through to an href, so a non-web scheme can never be injected.
export const httpsOnly=u=>/^https?:\/\//i.test(u||'')?u:'';
// The same "nothing here yet" panel for every list that can be empty.
export function emptyState(big,sub){ const d=el('div','empty'); d.innerHTML=`<div class="big">${esc(big)}</div><div>${esc(sub)}</div>`; return d; }

// Click on the dimmed backdrop closes any open modal. Wired once every sheet is
// in the page, so a component mounted later is covered too.
export function initDialogs(){
  document.querySelectorAll('dialog').forEach(d=>d.addEventListener('click',e=>{ if(e.target===d) d.close(); }));
}
