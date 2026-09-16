import {$, el, esc} from './dom.js';

export function toast(msg,kind){ const t=el('div','toast'+(kind?' '+kind:'')); t.append(el('div','bar'),el('div',null,esc(msg)));
  $('#toasts').append(t); setTimeout(()=>{t.style.opacity='0';t.style.transition='.3s';setTimeout(()=>t.remove(),300)},2600); }

export function mountToasts(){ document.body.insertAdjacentHTML('beforeend', '<div id="toasts"></div>'); }
