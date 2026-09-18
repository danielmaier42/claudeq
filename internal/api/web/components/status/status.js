import {loadRuns} from '../activity/activity.js';
import {current} from '../app-shell/app-shell.js';
import {loadTasks} from '../queue/queue.js';
import {openNotificationSettings} from '../settings/settings.js';
import {UPDATE} from '../update/update.js';
import {api} from '../../core/api.js';
import {$, esc} from '../../core/dom.js';
import {relTime} from '../../core/format.js';

// When the global rate-limit gate reopens (a Date), or null while it is open.
export let LIMITED_UNTIL=null;
let notifWarnFor=null;
// macOS can accept every notification ClaudeQ posts and still show none of them:
// an app whose authorization is denied (or never granted) posts into the void.
// That is invisible from the app's side, so say it out loud.
function renderNotifyWarn(status){
  const blocked=status==='denied'||status==='not_determined';
  const bar=$('#notifWarn');
  if(!blocked){ bar.hidden=true; notifWarnFor=null; return; }
  if(notifWarnFor===status){ bar.hidden=false; return; }   // already rendered
  notifWarnFor=status;
  bar.innerHTML='<b>⚠ macOS is not showing ClaudeQ\'s notifications</b>'
    +(status==='denied'
      ? 'Run outcomes and new artifacts are being announced, but macOS is set to hide them — nothing reaches your screen. Turn <strong>Allow notifications</strong> back on for ClaudeQ.'
      : 'ClaudeQ hasn\'t been allowed to notify you yet, so run outcomes and new artifacts stay silent. Allow notifications for ClaudeQ.')
    +' Pick <strong>Alerts</strong> while you\'re there and an alert stays on screen until you click it.'
    +'<div class="upd-actions"><button class="btn" id="notifWarnBtn">Open System Settings</button></div>';
  $('#notifWarnBtn').onclick=openNotificationSettings;
  bar.hidden=false;
}

// The queue standing still on a rate limit is normal, not a fault — say what it
// is waiting for, and until when, so a paused queue is never mistaken for a
// stuck one.
function renderLimitBar(iso,providers){
  const prev=LIMITED_UNTIL&&LIMITED_UNTIL.getTime();
  LIMITED_UNTIL = iso?new Date(iso):null;
  const bar=$('#limitBar');
  if(!LIMITED_UNTIL||LIMITED_UNTIL<=new Date()){ LIMITED_UNTIL=null; bar.hidden=true; }
  else{
    const list=providers||[];
    const names=list.map(p=>esc(p.name)+(p.fallback?' → '+esc(p.fallback):''));
    const heading=names.length
      ?'<b>⏸ Rate limit reached — waiting on '+names.join(', ')+'</b>'
      :'<b>⏸ Rate limit reached — the queue is waiting, not stuck</b>';
    // A provider with a fallback is not holding its tasks up: they are running
    // on the substitute right now, so the banner must not claim nothing starts.
    const covered=list.length&&list.every(p=>p.fallback);
    bar.innerHTML=heading
      +(covered
        ?'Tasks keep running on the fallback provider. The blocked one takes them back from <strong>'+esc(LIMITED_UNTIL.toLocaleString())+'</strong> ('+esc(relTime(LIMITED_UNTIL.toISOString()))+'). '
        :'No task starts before <strong>'+esc(LIMITED_UNTIL.toLocaleString())+'</strong> ('+esc(relTime(LIMITED_UNTIL.toISOString()))+'). ')
      +'A run the limit interrupted is marked <strong>rescheduled</strong> in Activity and continues its Claude session then — or drop it there with <strong>Cancel resume</strong>.';
    bar.hidden=false;
  }
  // Re-render the views that show the resume time when the gate moved.
  if(prev!==(LIMITED_UNTIL&&LIMITED_UNTIL.getTime())){ if(current==='tasks') loadTasks(); if(current==='news') loadRuns(); }
}

let CONN_OK=true;
export function setConn(ok){ CONN_OK=ok; $('#connDot').style.background=ok?'var(--ok)':'var(--danger)'; renderConnText(); }
// The dot alone carries connected/disconnected; the label shows the running
// version once known (falling back to "disconnected" when the dot is red).
export function renderConnText(){
  const t=$('#connText'); if(!t) return;
  t.textContent = CONN_OK ? (UPDATE&&UPDATE.current?(UPDATE.supported?('v'+UPDATE.current):UPDATE.current):'connected') : 'disconnected';
}

// Surface a broken scheduled-wake setup so timed runs don't silently miss, and
// notifications macOS is set to swallow.
export async function checkHealth(){
  let h; try{ h=await api('GET','/api/health'); }catch{ return; }
  renderNotifyWarn(h&&h.notify_status);
  renderLimitBar(h&&h.limited_until, h&&h.limited_providers);
  const bar=$('#wakeWarn'), err=h&&h.wake_error;
  if(!err){ bar.hidden=true; return; }
  const needsSudoers=/password|sudo|not permitted|permission/i.test(err);
  if(needsSudoers){
    bar.innerHTML='<b>⚠ Scheduled wake isn\'t set up</b>Timed tasks may not run while your Mac is asleep. Allow ClaudeQ to schedule wake-ups by running this once in Terminal, then it takes effect immediately:'
      +'<code>echo "$(whoami) ALL=(root) NOPASSWD: /usr/bin/pmset" | sudo tee /etc/sudoers.d/claudeq</code>';
  } else {
    bar.innerHTML='<b>⚠ Scheduled wake failed</b>'+esc(err);
  }
  bar.hidden=false;
}

// The banners that speak for the machine rather than for a view: a failed wake,
// notifications that go nowhere, the rate-limit gate. They sit above whichever
// view is open, so they are mounted once into the shell's banner slot.
export function mountBanners(){
  $('#banners').innerHTML = `
    <div id="wakeWarn" class="warnbar" hidden></div>
    <div id="notifWarn" class="warnbar" hidden></div>
    <div id="limitBar" class="warnbar" hidden></div>`;
}
