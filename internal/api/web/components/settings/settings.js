import {PROVIDERS, SHOW_BETA, fillAsideChoices, loadProviders, openAddProvider, setProviderSnapshot, submitProvider} from '../providers/providers.js';
import {setPaused} from '../queue/queue.js';
import {makeReview} from '../review/review.js';
import {setConn} from '../status/status.js';
import {checkForUpdates, loadUpdate, renderUpdateBadge, renderUpdateBox, renderVersionRow} from '../update/update.js';
import {api} from '../../core/api.js';
import {$, el} from '../../core/dom.js';
import {SAVE, SPARK} from '../../core/icons.js';
import {modelOptions, optionsFor} from '../../core/models.js';
import {toast} from '../../core/toast.js';

// The Settings pane is split into sub-tabs. Every pane is rendered and stays in
// the DOM — only its visibility is toggled — so Save always sees all fields and
// the update banner keeps working whichever pane is open.

// The Settings view's review controller lives here, so leaving the tab can stop
// a review in flight instead of letting the daemon run it out.
let settingsReview=null;
let settingsPane='general';
const SETTINGS_PANES=[['general','General'],['providers','Providers'],['notifications','Notifications'],['system','System']];
async function loadSettings(){
  let s; try{ s=await api('GET','/api/settings'); setConn(true);}catch(e){ setConn(false); return; }
  const c=$('#settings');
  const hbOpts=[[15,'Every 15 minutes'],[30,'Every 30 minutes'],[60,'Every hour'],[120,'Every 2 hours'],[180,'Every 3 hours'],[360,'Every 6 hours']];
  const hbVal=s.heartbeat_minutes||60;
  let hbHtml=hbOpts.map(([v,l])=>`<option value="${v}"${v===hbVal?' selected':''}>${l}</option>`).join('');
  if(!hbOpts.some(([v])=>v===hbVal)) hbHtml=`<option value="${hbVal}" selected>Every ${hbVal} minutes</option>`+hbHtml;
  const poOn=!!s.pushover?.enabled;
  const idleVal=s.idle_timeout_minutes===0?30:s.idle_timeout_minutes; // 0 = default 30
  const histVal=s.max_run_history===0?500:s.max_run_history;          // 0 = default 500
  c.innerHTML=`
    <div id="updateBox" class="warnbar" hidden></div>
    <div class="seg subtabs" id="s-tabs">${SETTINGS_PANES.map(([id,label])=>`<button data-pane="${id}">${label}${id==='general'?'<span class="dot" id="s-tab-dot" hidden></span>':''}</button>`).join('')}</div>

    <div class="s-pane" data-pane="general">
      <div class="section-label">Defaults for every run</div>
      <div class="group">
        <div style="padding:11px 15px">
          <div class="title" style="font-weight:500">Custom system prompt</div>
          <div style="color:var(--fg2);font-size:12px;margin:1px 0 8px">Appended to every run after claudeq's built-in instructions (built-in first, yours last). Use it for standing guidance you want on every task — coding conventions, tone, tools to prefer. Leave empty for none.</div>
          <textarea id="s-sysprompt" rows="6" placeholder="e.g. Follow the repo's existing conventions. Always run the tests before finishing." style="width:100%;box-sizing:border-box"></textarea>
          <div class="ai-banner" id="s-sysprompt-review" hidden></div>
        </div>
      </div>
      <div class="section-label">Default job directory</div>
      <div class="group">
        <div style="padding:11px 15px">
          <div class="title" style="font-weight:500">Prefill new tasks with</div>
          <div style="color:var(--fg2);font-size:12px;margin:1px 0 8px">A new task's working directory starts here. Still editable per task; empty means the field starts blank, as before.</div>
          <div class="dir-row"><input type="text" id="s-default-dir" placeholder="No default" readonly><button class="btn" id="s-default-dir-btn" title="Choose folder">📁</button></div>
        </div>
      </div>
      <div class="section-label">Prompt review</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Check prompts with Claude</div>
          <div class="sub multi">Before a task is added, imported or edited, Claude reads its prompt against this Mac — paths that aren't here, a file to be written into a folder that doesn't exist, guidelines it points at — and offers a rewrite under the prompt box. It reviews the system prompt above the same way.</div></div>
          <label class="switch"><input type="checkbox" id="s-review"><span class="sl"></span></label></div>
        <div class="row"><div class="grow"><div class="title">Review provider</div>
          <div class="sub">Reviewing costs allowance too, so it can run on an account other than the one your tasks use</div></div>
          <select id="s-review-provider" style="max-width:230px"></select></div>
        <div class="row"><div class="grow"><div class="title">Review model</div>
          <div class="sub">Runs on every prompt change, so a fast model earns its keep</div></div>
          <select id="s-review-model" style="max-width:230px">${modelOptions(s.prompt_review_model||'',"The provider's default model")}</select></div>
      </div>
      <div class="section-label">Feedback</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Feedback provider</div>
          <div class="sub multi">Drafts the GitHub issue on the Feedback page. Nothing is sent anywhere: the draft opens as a prefilled issue in your browser, and you press Create there.</div></div>
          <select id="s-feedback-provider" style="max-width:230px"></select></div>
        <div class="row"><div class="grow"><div class="title">Feedback model</div>
          <div class="sub">Two short messages into an issue — a small model is enough</div></div>
          <select id="s-feedback-model" style="max-width:230px"></select></div>
      </div>
      <div class="section-label">Execution</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Pause all runs</div>
          <div class="sub multi">Global stop switch: no task starts while this is on, not even <strong>Run now</strong>. Takes effect immediately, without saving. A run already in flight keeps going.</div></div>
          <label class="switch"><input type="checkbox" id="s-paused"><span class="sl"></span></label></div>
      </div>
      <div class="section-label">About</div>
      <div class="group">
        <div class="row">
          <img src="/logo.svg" alt="ClaudeQ" style="width:34px;height:34px;border-radius:7px;flex:none">
          <div class="grow"><div class="title">ClaudeQ <span id="s-version" class="chip"></span></div>
            <div class="sub">Queue Claude Code tasks by day, run them by night — free &amp; open source.</div></div>
          <a class="btn primary" href="https://github.com/danielmaier42/claudeq" target="_blank" rel="noopener">★ Star on GitHub</a>
        </div>
        <div class="row"><div class="grow"><div class="title">Software updates</div>
          <div class="sub" id="s-update-sub">Check GitHub for a newer release of ClaudeQ</div></div>
          <button class="btn" id="s-check-updates">Check for updates</button></div>
      </div>
    </div>

    <div class="s-pane" data-pane="providers">
      <div id="s-providers"><div class="group"><div class="row"><div class="grow"><div class="sub">Loading…</div></div></div></div></div>
      <div id="s-provider-add" class="center-action" hidden>
        <button class="btn beta" id="s-add-provider">${SPARK}<span>Add provider</span></button>
      </div>
    </div>

    <div class="s-pane" data-pane="notifications">
      <div class="section-label">macOS</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Keep alerts on screen</div>
          <div class="sub">macOS decides how long a notification lingers — pick Alerts, not Banners</div></div>
          <button class="btn" id="s-notif-settings">Open System Settings</button></div>
      </div>
      <div class="section-label">Pushover</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Send to Pushover</div><div class="sub">Push run alerts to your phone</div></div>
          <label class="switch"><input type="checkbox" id="s-po-enabled"><span class="sl"></span></label></div>
        <div class="row"><div class="grow"><div class="title">API token</div></div><input type="text" id="s-po-token" style="max-width:230px"></div>
        <div class="row"><div class="grow"><div class="title">User key</div></div><input type="text" id="s-po-user" style="max-width:230px"></div>
      </div>
      <div class="section-label">ntfy</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Send to ntfy</div><div class="sub">Push to a topic on ntfy.sh or on your own server</div></div>
          <label class="switch"><input type="checkbox" id="s-ntfy-enabled"><span class="sl"></span></label></div>
        <div class="row"><div class="grow"><div class="title">Server</div></div><input type="text" id="s-ntfy-server" placeholder="https://ntfy.sh" style="max-width:230px"></div>
        <div class="row"><div class="grow"><div class="title">Topic</div><div class="sub">The name your phone is subscribed to</div></div><input type="text" id="s-ntfy-topic" style="max-width:230px"></div>
        <div class="row"><div class="grow"><div class="title">Access token</div><div class="sub">Only for a protected topic</div></div><input type="text" id="s-ntfy-token" style="max-width:230px"></div>
      </div>
      <div class="section-label">Webhook</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Post to a webhook</div><div class="sub">Any JSON endpoint: Slack, Discord, Home Assistant, n8n</div></div>
          <label class="switch"><input type="checkbox" id="s-wh-enabled"><span class="sl"></span></label></div>
        <div class="row"><div class="grow"><div class="title">Endpoint</div></div><input type="text" id="s-wh-url" placeholder="https://hooks.example.com/…" style="max-width:230px"></div>
        <div style="padding:11px 15px">
          <div class="title" style="font-weight:500">Body</div>
          <div style="color:var(--fg2);font-size:12px;margin:1px 0 8px">The JSON posted to the endpoint. <code>{{title}}</code>, <code>{{message}}</code> and <code>{{url}}</code> are filled in and escaped. Leave empty to send ClaudeQ's own body; set it to what the service expects (Slack wants <code>text</code>, Discord <code>content</code>).</div>
          <textarea id="s-wh-template" rows="3" spellcheck="false" placeholder='{"text":"{{title}}: {{message}}"}' style="width:100%;box-sizing:border-box"></textarea>
        </div>
      </div>
    </div>

    <div class="s-pane" data-pane="system">
      <div class="section-label">Runs</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Stop a run with no output for</div><div class="sub">Kills a hung run; a working run keeps streaming, so it's unaffected</div></div>
          <select id="s-idle" style="max-width:170px">${optionsFor([[-1,'Off'],[15,'15 minutes'],[30,'30 minutes'],[60,'1 hour'],[120,'2 hours'],[240,'4 hours']], idleVal, m=>m+' minutes')}</select></div>
        <div class="row"><div class="grow"><div class="title">Keep run history</div><div class="sub">Older runs and their logs are pruned</div></div>
          <select id="s-hist" style="max-width:170px">${optionsFor([[100,'100 runs'],[500,'500 runs'],[1000,'1000 runs'],[-1,'Unlimited']], histVal, n=>n+' runs')}</select></div>
      </div>
      <div class="section-label">Scheduler</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Check for due tasks every</div><div class="sub">How often the daemon wakes to look for work</div></div>
          <select id="s-hb" style="max-width:170px">${hbHtml}</select></div>
      </div>
      <div class="section-label">Beta features</div>
      <div class="group">
        <div class="row"><div class="grow"><div class="title">Beta features</div>
          <div class="sub multi">Parts of ClaudeQ that are not finished yet. It decides what the app offers and nothing else: anything already set up keeps working, and the <code>claudeq</code> CLI accepts it either way.</div></div>
          <label class="switch"><input type="checkbox" id="s-beta"><span class="sl"></span></label></div>
      </div>
    </div>

    <div class="sub" style="text-align:center;margin-top:10px">Made with <span style="color:var(--accent)">♥</span> by
      <a href="https://github.com/danielmaier42" target="_blank" rel="noopener" style="color:var(--accent);text-decoration:none">danielmaier42</a></div>`;
  $('#s-paused').checked=!!s.paused;
  $('#s-paused').onchange=e=>setPaused(e.target.checked);
  $('#s-po-enabled').checked=poOn; $('#s-po-token').value=s.pushover?.token||''; $('#s-po-user').value=s.pushover?.user_key||'';
  $('#s-ntfy-enabled').checked=!!s.ntfy?.enabled; $('#s-ntfy-server').value=s.ntfy?.server||'';
  $('#s-ntfy-topic').value=s.ntfy?.topic||''; $('#s-ntfy-token').value=s.ntfy?.token||'';
  $('#s-wh-enabled').checked=!!s.webhook?.enabled; $('#s-wh-url').value=s.webhook?.url||'';
  $('#s-wh-template').value=s.webhook?.template||'';
  $('#s-sysprompt').value=s.system_prompt||'';
  $('#s-default-dir').value=s.default_working_dir||'';
  $('#s-default-dir-btn').onclick=async()=>{
    try{
      const q=$('#s-default-dir').value?('?path='+encodeURIComponent($('#s-default-dir').value)):'';
      const r=await api('POST','/api/fs/choose'+q);
      if(r && typeof r!=='string' && r.path) $('#s-default-dir').value=r.path; // 204/empty => cancelled
    }catch(e){ toast('Folder dialog unavailable: '+e.message,'err'); }
  };
  setProviderSnapshot({showBeta:s.beta_features,defaultDir:s.default_working_dir});
  $('#s-beta').checked=SHOW_BETA;
  // Revealing beta providers changes what may be added and what a task may be
  // pointed at, so the lists are rebuilt rather than left saying something else.
  $('#s-beta').onchange=async e=>{
    try{ await api('PUT','/api/settings',{...(await api('GET','/api/settings')),beta_features:e.target.checked}); }
    catch(err){ toast(err.message,'err'); e.target.checked=!e.target.checked; return; }
    setProviderSnapshot({showBeta:e.target.checked});
    // What may be chosen for a review or a draft changes with it, so those two
    // pickers are rebuilt on what is now on offer — keeping the current choices.
    await loadProviders();
    fillAsidePickers($('#s-review-provider').value, $('#s-review-model').value,
      $('#s-feedback-provider').value, $('#s-feedback-model').value);
  };
  // The Settings view is rebuilt on every visit, so its review controller is
  // bound to the fresh nodes here. The system prompt has no working directory:
  // it is appended to every task, wherever that task runs.
  const sysReview=makeReview({kind:'system',area:$('#s-sysprompt'),banner:$('#s-sysprompt-review'),dir:()=>''});
  settingsReview=sysReview;
  $('#s-sysprompt').addEventListener('input',()=>sysReview.schedule());
  const reviewOn=!s.prompt_review_disabled;
  $('#s-review').checked=reviewOn; $('#s-review-model').disabled=!reviewOn;
  // The toggle only takes effect on Save, so the banner follows the box: off
  // clears it, on brings back what is known about the text that is there.
  $('#s-review').onchange=e=>{ $('#s-review-model').disabled=!e.target.checked;
    if(e.target.checked) sysReview.restore(); else sysReview.reset(); };
  if(reviewOn) sysReview.restore();
  $('#s-check-updates').onclick=checkForUpdates;
  $('#s-notif-settings').onclick=openNotificationSettings;
  $('#s-add-provider').onclick=openAddProvider;
  $('#np-add').onclick=submitProvider;
  c.querySelectorAll('#s-tabs button').forEach(b=>b.onclick=()=>selectSettingsPane(b.dataset.pane));
  selectSettingsPane(settingsPane);
  renderUpdateBox(); renderVersionRow(); renderUpdateBadge();
  // The two pickers below offer configured providers, so they wait for the list
  // rather than rendering an empty one and correcting itself a moment later.
  await loadProviders();
  fillAsidePickers(s.prompt_review_provider||'', s.prompt_review_model||'', s.feedback_provider||'', s.feedback_model||'');
}

// fillAsidePickers rebuilds the provider/model pairs for the two jobs claudeq
// runs on its own behalf. It is called again whenever the offered set changes.
function fillAsidePickers(reviewProvider, reviewModel, feedbackProvider, feedbackModel){
  fillAsideChoices($('#s-review-provider'), $('#s-review-model'), reviewProvider, reviewModel,
    "The provider's default model");
  fillAsideChoices($('#s-feedback-provider'), $('#s-feedback-model'), feedbackProvider, feedbackModel,
    'ClaudeQ\u2019s choice (a small, fast model)');
}
function selectSettingsPane(pane){
  if(!SETTINGS_PANES.some(([id])=>id===pane)) pane='general';
  settingsPane=pane;
  document.querySelectorAll('#s-tabs button').forEach(b=>b.classList.toggle('active',b.dataset.pane===pane));
  document.querySelectorAll('#settings .s-pane').forEach(p=>p.hidden=p.dataset.pane!==pane);
}
// Jump to macOS' notification settings, where the alert style lives (Banners
// disappear on their own, Alerts wait for a click). Only the app window can open
// a System Settings URL — in a browser tab we just say where to look.
export function openNotificationSettings(){
  const url='x-apple.systempreferences:com.apple.Notifications-Settings.extension';
  if(window.cqOpenExternal) window.cqOpenExternal(url);
  else toast('Open System Settings → Notifications → ClaudeQ','ok');
}
// providerEdits reads every provider block back as the payload its instance
// takes. The blocks are part of the Settings form, so one Save writes them all.
function providerEdits(){
  return [...document.querySelectorAll('#s-providers [data-provider-id]')].map(w=>({
    id:w.dataset.providerId,
    body:{
      name:w.querySelector('.p-name').value.trim(),
      binary_path:w.querySelector('.p-path').value.trim(),
      config_dir:w.querySelector('.p-dir').value.trim(),
      default_model:w.querySelector('.p-model').value,
      fallback_provider:w.querySelector('.p-fallback').value,
      fallback_model:w.querySelector('.p-fallback-model').value,
      enabled:w.querySelector('.switch input').checked,
    },
  }));
}

async function saveSettings(){
  // No paused: the switch writes itself through /api/pause and the server keeps
  // whatever is stored, so Save neither carries nor resets it.
  const body={heartbeat_minutes:parseInt($('#s-hb').value||'60',10),
    idle_timeout_minutes:parseInt($('#s-idle').value||'30',10),
    max_run_history:parseInt($('#s-hist').value||'500',10),
    system_prompt:$('#s-sysprompt').value,
    default_working_dir:$('#s-default-dir').value.trim(),
    prompt_review_disabled:!$('#s-review').checked,
    prompt_review_provider:$('#s-review-provider').value,
    prompt_review_model:$('#s-review-model').value,
    feedback_provider:$('#s-feedback-provider').value,
    feedback_model:$('#s-feedback-model').value,
    beta_features:$('#s-beta').checked,
    pushover:{enabled:$('#s-po-enabled').checked,token:$('#s-po-token').value,user_key:$('#s-po-user').value},
    ntfy:{enabled:$('#s-ntfy-enabled').checked,server:$('#s-ntfy-server').value.trim(),topic:$('#s-ntfy-topic').value.trim(),token:$('#s-ntfy-token').value.trim()},
    webhook:{enabled:$('#s-wh-enabled').checked,url:$('#s-wh-url').value.trim(),template:$('#s-wh-template').value.trim()}};
  try{ await api('PUT','/api/settings',body); }catch(e){ toast(e.message,'err'); return; }
  // The provider blocks are part of this form too. Each is refused on its own
  // terms, so a rejected one is named rather than failing the whole save
  // silently.
  const failed=[];
  const edits=providerEdits();
  // The blocks are written one after another, so a Save that swaps two
  // providers' fallbacks would pass through a state the daemon reads as a
  // cycle and refuse. Every fallback that changed is therefore cleared first;
  // the second pass then sets the values the form actually asks for.
  const moved=edits.filter(p=>(PROVIDERS.find(x=>x.id===p.id)||{}).fallback_provider!==p.body.fallback_provider);
  for(const p of moved){
    try{ await api('PUT','/api/providers/'+encodeURIComponent(p.id),{...p.body,fallback_provider:''}); }catch{}
  }
  for(const p of edits){
    try{ await api('PUT','/api/providers/'+encodeURIComponent(p.id),p.body); }
    catch(e){ failed.push(`${p.id}: ${e.message}`); }
  }
  if(failed.length){ toast(failed.join('\n'),'err'); }
  else toast('Settings saved','ok');
  if(document.querySelector('#s-providers')) loadProviders();
}

export const view={
  title:'Settings',
  toolbar(ta){ const b=el('button','btn primary',SAVE+'<span>Save</span>'); b.onclick=saveSettings; ta.append(b); },
  enter(){ loadSettings(); loadUpdate(); },
  leave(){ if(settingsReview){ settingsReview.reset(); settingsReview=null; } },
};
