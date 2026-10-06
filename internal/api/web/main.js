import {loadRuns} from './components/activity/activity.js';
import {initShell, mountShell, refreshAll, select} from './components/app-shell/app-shell.js';
import {initViewer, loadArtifacts, mountViewer} from './components/artifacts/artifacts.js';
import {loadDashboard} from './components/dashboard/dashboard.js';
import {initFeedback, mountFeedback} from './components/feedback/feedback.js';
import {initLogSheet, mountLogSheet} from './components/log-sheet/log-sheet.js';
import {loadNotifications} from './components/notifications/notifications.js';
import {initPoolSheet, mountPoolSheet} from './components/pools/pools.js';
import {initProviderSheet, mountProviderSheet} from './components/providers/providers.js';
import {loadTasks} from './components/queue/queue.js';
import {initTaskReview} from './components/review/review.js';
import {mountSidebar} from './components/sidebar/sidebar.js';
import {checkHealth, mountBanners} from './components/status/status.js';
import {initTaskSheet, mountTaskSheet, openAdd} from './components/task-sheet/task-sheet.js';
import {loadUpdate} from './components/update/update.js';
import {loadUsage} from './components/usage/usage.js';
import {mountConfirm} from './core/confirm.js';
import {initDialogs} from './core/dom.js';
import {loadModels} from './core/models.js';
import {mountToasts} from './core/toast.js';

// The dashboard is plain ES modules served straight from the daemon — no build
// step, no framework, no dependencies. The layout says where a thing lives:
//
//   styles/      design tokens and the shared primitives every component uses
//   core/        the helpers with no view of their own (dom, api, format, …)
//   components/  one directory per component, its markup, style and code together
//   main.js      this file: what the page is made of, and in which order
//
// A component owns its own markup (its `mount`), its own controls (its `init`)
// and, if it is one of the seven views, what the toolbar and the title say while
// it is open (its `view`). Nothing reaches into another component except
// through what that component exports.
//
// Boot: put every component into the page, wire the ones that own controls of
// their own, then start the polls that keep the open view current.

mountSidebar();
mountShell();
mountBanners();
mountFeedback();
mountTaskSheet();
mountLogSheet();
mountViewer();
mountProviderSheet();
mountPoolSheet();
mountConfirm();
mountToasts();

initShell();
initTaskSheet();
initTaskReview();
initLogSheet();
initViewer();
initProviderSheet();
initPoolSheet();
initFeedback();
initDialogs();

// The app window's menu bar drives the page through these three, and a click
// on a macOS notification through window.cqOpenNotificationTarget (published
// where it is implemented). Modules keep everything else to themselves, so what
// the native side may call is exactly this list — see cmd/claudeqapp/main_darwin.go.
window.openAdd=openAdd;
window.select=select;
window.cqRefresh=refresh;

// Every list is kept current in the background, not only the open one: a page
// that is clicked then shows the present straight away, and its own load on
// entering confirms it. The badges ride along with the lists. The dashboard's
// poll reads only what the daemon last read from the providers; asking them
// again is for opening it and View > Refresh.
function poll(){ loadDashboard(false); loadTasks(); loadRuns(); loadArtifacts(); loadNotifications(); loadUsage(); }
setInterval(poll, 5000);

// refresh is the explicit "show me now" of View > Refresh (Cmd+R) in the app's
// menu bar: every view is redrawn, changed or not.
function refresh(){ refreshAll(); checkHealth(); }
// Coming back to the window polls at once: WebKit holds back the timers of a
// window in the background, so what it shows on return can be minutes old. It
// is a poll, not a refresh: a row redrawn without a change could swallow the
// click that brought the window forward.
let lastBack=0;
function back(){
  if(document.visibilityState!=='visible') return;
  // Focus and visibility arrive together when the window comes back.
  const now=Date.now(); if(now-lastBack<1000) return; lastBack=now;
  poll(); checkHealth();
}
window.addEventListener('focus', back);
document.addEventListener('visibilitychange', back);

setInterval(checkHealth, 30000); checkHealth();
// The daemon checks GitHub hourly; the UI just reads its cached result, so a
// slow poll keeps the Settings badge current without any extra network calls.
setInterval(loadUpdate, 60000); loadUpdate();
loadModels().then(()=>{ select('dashboard'); poll(); });
