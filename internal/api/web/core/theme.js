import {api} from './api.js';

// The dashboard's colour scheme: macOS' unless Settings → General → Appearance
// pins light or dark. The choice is the daemon's (config.toml, appearance), so
// the app window and a browser tab agree; a copy in localStorage lets
// index.html put it on <html> before the first paint, and the app window's
// own chrome is told through cqSetAppearance (cmd/claudeqapp).
const KEY='cq-appearance';
export const APPEARANCES=[['','System'],['light','Light'],['dark','Dark']];
export function applyAppearance(mode){
  mode=mode==='light'||mode==='dark'?mode:'';
  if(mode) document.documentElement.dataset.theme=mode; else delete document.documentElement.dataset.theme;
  try{ if(mode) localStorage.setItem(KEY,mode); else localStorage.removeItem(KEY); }catch{}
  if(window.cqSetAppearance) window.cqSetAppearance(mode);
}
// loadAppearance applies what the daemon holds; a fresh page load runs it so a
// choice made in another window (or by hand in config.toml) is picked up.
export async function loadAppearance(){
  try{ applyAppearance((await api('GET','/api/settings')).appearance); }catch{}
}
