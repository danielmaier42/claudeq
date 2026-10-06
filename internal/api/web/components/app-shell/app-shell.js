import {$} from '../../core/dom.js';
import {view as activity} from '../activity/activity.js';
import {view as artifacts} from '../artifacts/artifacts.js';
import {view as dashboard} from '../dashboard/dashboard.js';
import {view as feedback} from '../feedback/feedback.js';
import {view as notifications} from '../notifications/notifications.js';
import {view as queue} from '../queue/queue.js';
import {view as settings} from '../settings/settings.js';
import {view as usage} from '../usage/usage.js';

// The shell around every view: the tab bar's selection, the page title and the
// toolbar. Which of those a view wants is the view's own business — it says so
// in its exported `view`, so adding one never means editing the shell.

// Every view the sidebar can reach, keyed by the id of its <section> and of its
// tab button. Looked up on demand: the views import the shell back, and a table
// built while those modules are still loading would read half of them as empty.
const VIEW_IDS=['dashboard','tasks','news','artifacts','notifications','usage','feedback','settings'];
function viewOf(tab){ return {dashboard,tasks:queue,news:activity,artifacts,notifications,usage,feedback,settings}[tab]; }

export let current='dashboard';

export function select(tab){
  const prev=viewOf(current);
  if(prev&&prev.leave) prev.leave();
  current=tab;
  document.querySelectorAll('.nav button[data-tab]').forEach(x=>x.classList.toggle('active',x.dataset.tab===tab));
  document.querySelectorAll('.tab').forEach(t=>t.hidden=t.id!==tab);
  const v=viewOf(tab);
  $('#pageTitle').textContent=v.title;
  const ta=$('#toolbarActions'); ta.innerHTML='';
  if(v.toolbar) v.toolbar(ta);
  if(v.enter) v.enter();
}

// refreshAll redraws every view that can be redrawn from the daemon's present
// state, open or not, so the next tab clicked shows the same moment. A view
// says how in its `refresh`; one without (Feedback) holds only what was typed.
export function refreshAll(){
  VIEW_IDS.forEach(id=>{ const v=viewOf(id); if(v.refresh) v.refresh(); });
}

export function initShell(){
  document.querySelectorAll('.nav button[data-tab]').forEach(b=>b.onclick=()=>select(b.dataset.tab));
}

// The frame: the toolbar, the banner slot and one empty section per view. What
// goes inside a section is that view's own business.
export function mountShell(){
  document.body.insertAdjacentHTML('beforeend', `
<div class="main">
  <div class="toolbar">
    <h1 id="pageTitle">Dashboard</h1>
    <div class="spacer"></div>
    <div id="toolbarActions"></div>
  </div>
  <div class="content"><div class="wrap">
    <div id="banners"></div>
    ${VIEW_IDS.map(id=>`<section id="${id}" class="tab" hidden></section>`).join('\n    ')}
  </div></div>
</div>`);
}
