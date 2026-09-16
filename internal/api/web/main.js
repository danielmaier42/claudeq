import {loadRuns} from './components/activity/activity.js';
import {current, initShell, mountShell, select} from './components/app-shell/app-shell.js';
import {initViewer, mountViewer, refreshArtifacts} from './components/artifacts/artifacts.js';
import {initFeedback, mountFeedback} from './components/feedback/feedback.js';
import {initLogSheet, mountLogSheet} from './components/log-sheet/log-sheet.js';
import {initProviderSheet, mountProviderSheet} from './components/providers/providers.js';
import {loadTasks} from './components/queue/queue.js';
import {initTaskReview} from './components/review/review.js';
import {mountSidebar} from './components/sidebar/sidebar.js';
import {checkHealth, mountBanners} from './components/status/status.js';
import {initTaskSheet, mountTaskSheet, openAdd} from './components/task-sheet/task-sheet.js';
import {loadUpdate} from './components/update/update.js';
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
// and, if it is one of the six views, what the toolbar and the title say while
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
mountConfirm();
mountToasts();

initShell();
initTaskSheet();
initTaskReview();
initLogSheet();
initViewer();
initProviderSheet();
initFeedback();
initDialogs();

// The app window's menu bar drives the page through these two, and its
// "new artifact" notification through window.cqOpenArtifact (published where it
// is implemented). Modules keep everything else to themselves, so what the
// native side may call is exactly this list — see cmd/claudeqapp/main_darwin.go.
window.openAdd=openAdd;
window.select=select;

// Poll activity for the unread badge + live updates.
setInterval(()=>{ if(current==='tasks') loadTasks(); loadRuns(); refreshArtifacts(); }, 5000);
refreshArtifacts();
setInterval(checkHealth, 30000); checkHealth();
// The daemon checks GitHub hourly; the UI just reads its cached result, so a
// slow poll keeps the Settings badge current without any extra network calls.
setInterval(loadUpdate, 60000); loadUpdate();
loadModels().then(()=>select('tasks'));
