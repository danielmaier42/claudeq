// The source list: the tabs, their unread badges and the connection footer.
// It only draws itself — what a tab does is the shell's business.
const template = `
<aside class="sidebar">
  <div class="brand"><img class="logo" src="/logo.svg" alt="ClaudeQ"><b>ClaudeQ</b></div>
  <nav class="nav">
    <button data-tab="dashboard" class="active">
      <svg viewBox="0 0 20 20"><rect x="3" y="3" width="6" height="6" rx="1.2"/><rect x="11" y="3" width="6" height="6" rx="1.2"/><rect x="3" y="11" width="6" height="6" rx="1.2"/><rect x="11" y="11" width="6" height="6" rx="1.2"/></svg> Dashboard</button>
    <button data-tab="tasks">
      <svg viewBox="0 0 20 20"><path d="M4 6h12M4 10h12M4 14h12"/></svg> Queue</button>
    <button data-tab="news">
      <svg viewBox="0 0 20 20"><path d="M3.5 10a6.5 6.5 0 1 0 1.9-4.6"/><path d="M3 3v3.5h3.5M10 6.5V10l2.5 1.5"/></svg>
      Log <span id="unreadCount" class="count" hidden>0</span></button>
    <button data-tab="artifacts">
      <svg viewBox="0 0 20 20"><path d="M6 2.5h5L15 6.5V17a.5.5 0 0 1-.5.5h-9A.5.5 0 0 1 5 17V3a.5.5 0 0 1 .5-.5z"/><path d="M11 2.5V7h4"/></svg>
      Artifacts <span id="artifactCount" class="count" hidden>0</span></button>
    <button data-tab="usage">
      <svg viewBox="0 0 20 20"><path d="M3 14a7 7 0 1 1 14 0"/><path d="M10 14l4-4"/></svg>
      Usage</button>
  </nav>
  <nav class="nav nav-foot">
    <button data-tab="feedback" data-tip="Report a bug or suggest a feature — Claude drafts the GitHub issue, you file it">
      <svg viewBox="0 0 20 20"><path d="M16.5 12.5a1.5 1.5 0 0 1-1.5 1.5H7l-3.5 3V5A1.5 1.5 0 0 1 5 3.5h10A1.5 1.5 0 0 1 16.5 5z"/></svg>
      Feedback</button>
    <button data-tab="settings">
      <svg viewBox="0 0 20 20"><circle cx="10" cy="10" r="2.5"/><path d="M10 3v2M10 15v2M3 10h2M15 10h2M5.5 5.5l1.4 1.4M13.1 13.1l1.4 1.4M14.5 5.5l-1.4 1.4M6.9 13.1l-1.4 1.4"/></svg>
      Settings <span id="updateBadge" class="count" hidden>1</span></button>
  </nav>
  <div class="side-foot"><span class="dot" id="connDot"></span><span id="connText">connected</span>
    <a href="https://github.com/danielmaier42/claudeq" target="_blank" rel="noopener" title="claudeq on GitHub"
       style="margin-left:auto;color:var(--fg2);text-decoration:none">GitHub ↗</a></div>
</aside>
`;

export function mountSidebar(){ document.body.insertAdjacentHTML('beforeend', template); }
