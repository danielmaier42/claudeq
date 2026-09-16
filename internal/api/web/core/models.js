import {api} from './api.js';
import {esc} from './dom.js';

let MODELS=[];
export async function loadModels(){ try{ MODELS=await api('GET','/api/models'); }catch{ MODELS=[]; } }
export function modelOptions(selected,globalLabel){ return modelOptionsFrom(MODELS,selected,globalLabel); }
// modelOptionsFrom renders a model picker. A stored model the list does not
// contain is kept and marked custom: a catalog is a suggestion, and a task must
// never lose the model it was given because discovery came back short.
export function modelOptionsFrom(models,selected,emptyLabel){
  let h=`<option value="">${esc(emptyLabel)}</option>`;
  (models||[]).forEach(m=>{ h+=`<option value="${esc(m.id)}"${m.id===selected?' selected':''}>${esc(m.label)}</option>`; });
  if(selected && !(models||[]).some(m=>m.id===selected)) h+=`<option value="${esc(selected)}" selected>${esc(selected)} (custom)</option>`;
  return h;
}
export function optionsFor(list,val,customLabel){
  let h=list.map(([v,l])=>`<option value="${v}"${v===val?' selected':''}>${esc(l)}</option>`).join('');
  if(!list.some(([v])=>v===val)) h=`<option value="${val}" selected>${esc(customLabel(val))}</option>`+h;
  return h;
}
