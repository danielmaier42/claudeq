import {LIMITED_UNTIL} from '../components/status/status.js';
import {$} from './dom.js';

export function pad2(n){ return String(n).padStart(2,'0'); }
// The local calendar day of a timestamp, in the form an <input type=date>
// carries — the value a from–to day filter compares against.
export function localDate(iso){ const d=new Date(iso); return d.getFullYear()+'-'+pad2(d.getMonth()+1)+'-'+pad2(d.getDate()); }
// Whether a timestamp falls inside a from–to day filter; an empty end is open.
export function inDateRange(iso,from,to){ const d=localDate(iso);
  if(from && d<from) return false; if(to && d>to) return false; return true; }
export function baseName(p){ const parts=String(p).replace(/\/+$/,'').split('/').filter(Boolean); return parts.length?parts[parts.length-1]:p; }
export function relTime(iso){ const d=new Date(iso), s=(Date.now()-d)/1000, a=Math.abs(s), fut=s<0;
  const f=(n,u)=> fut ? ('in '+n+' '+u) : (n+' '+u+' ago');
  if(a<60)return 'just now'; if(a<3600)return f(Math.floor(a/60),'min'); if(a<86400)return f(Math.floor(a/3600),'h');
  if(a<604800)return f(Math.floor(a/86400),'d'); return d.toLocaleDateString(); }
// exactTime: "18:00:00 Uhr", prefixed with the date if it was on an earlier day.
export function exactTime(iso){ const d=new Date(iso), n=new Date();
  const clock=pad2(d.getHours())+':'+pad2(d.getMinutes())+':'+pad2(d.getSeconds())+' Uhr';
  const sameDay=d.getFullYear()===n.getFullYear()&&d.getMonth()===n.getMonth()&&d.getDate()===n.getDate();
  return sameDay?clock:(pad2(d.getDate())+'.'+pad2(d.getMonth()+1)+'.'+d.getFullYear()+' '+clock); }
function fmtRunDur(ms){ const s=Math.max(0,Math.round(ms/1000)), h=Math.floor(s/3600), m=Math.floor((s%3600)/60), sec=s%60;
  return h?(h+'h '+m+'m '+sec+'s'):(m?(m+'m '+sec+'s'):(sec+'s')); }
// Tooltip for a run's time: exact start + how long it ran.
// The gate's live reopen time wins over the time the run recorded: another
// task's later limit can push the resume back after this run was paused.
function resumeAt(r){
  const a=r.resume_at?new Date(r.resume_at):null, b=LIMITED_UNTIL;
  if(a&&b) return a>b?a:b;
  return a||b||null;
}
export function resumeTimeText(at){
  if(!at) return 'resumes once the rate limit resets';
  if(at<=new Date()) return 'resumes on the next check';
  return 'resumes '+relTime(at.toISOString())+' · '+at.toLocaleString();
}
export function resumeText(r){ return resumeTimeText(resumeAt(r)); }
export function runTimeTitle(r){ let t=exactTime(r.started_at);
  if(r.finished_at) t+=' - '+fmtRunDur(new Date(r.finished_at)-new Date(r.started_at));
  return t; }
