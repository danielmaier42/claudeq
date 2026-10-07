import {$} from './dom.js';

export function confirmSheetAsk(text,yesLabel,noLabel){ return new Promise(res=>{ $('#confirmText').textContent=text; $('#confirmYes').textContent=yesLabel||'Delete'; $('#confirmNo').textContent=noLabel||'Cancel';
  const dlg=$('#confirmSheet'); let done=false;
  const finish=v=>{ if(done)return; done=true;
    $('#confirmYes').removeEventListener('click',onYes); $('#confirmNo').removeEventListener('click',onNo);
    dlg.removeEventListener('close',onClose); dlg.close(); res(v); };
  const onYes=()=>finish(true), onNo=()=>finish(false), onClose=()=>finish(false);
  $('#confirmYes').addEventListener('click',onYes); $('#confirmNo').addEventListener('click',onNo); dlg.addEventListener('close',onClose);
  dlg.showModal(); }); }

// The same sheet, with a line to type in: used where a gesture needs one word
// from the user (naming a new queue group) and a whole form would be in the way.
// Resolves to the trimmed text, or null when the user backs out.
export function promptSheetAsk(text,{placeholder='',value='',okLabel='Create',maxLength=60}={}){ return new Promise(res=>{
  $('#promptText').textContent=text; $('#promptOk').textContent=okLabel;
  const input=$('#promptInput'); input.placeholder=placeholder; input.value=value; input.maxLength=maxLength;
  const dlg=$('#promptSheet'); let done=false;
  const finish=v=>{ if(done)return; done=true;
    $('#promptOk').removeEventListener('click',onOk); $('#promptCancel').removeEventListener('click',onCancel);
    input.removeEventListener('keydown',onKey); dlg.removeEventListener('close',onClose); dlg.close(); res(v); };
  const value_=()=>{ const v=input.value.trim(); return v?v:null; };
  const onOk=()=>finish(value_()), onCancel=()=>finish(null), onClose=()=>finish(null);
  const onKey=e=>{ if(e.key==='Enter'){ e.preventDefault(); finish(value_()); } };
  $('#promptOk').addEventListener('click',onOk); $('#promptCancel').addEventListener('click',onCancel);
  input.addEventListener('keydown',onKey); dlg.addEventListener('close',onClose);
  dlg.showModal(); input.focus(); }); }

const template = `
<dialog id="confirmSheet">
  <div class="sheet-bd" style="padding-top:20px"><b id="confirmText">Are you sure?</b></div>
  <div class="sheet-ft"><button class="btn" id="confirmNo">Cancel</button><button class="btn danger" id="confirmYes">Delete</button></div>
</dialog>
<dialog id="promptSheet">
  <div class="sheet-bd" style="padding-top:20px">
    <b id="promptText">Name</b>
    <div class="form-row" style="margin-top:10px"><input id="promptInput" type="text" autocomplete="off"></div>
  </div>
  <div class="sheet-ft"><button class="btn" id="promptCancel">Cancel</button><button class="btn primary" id="promptOk">Create</button></div>
</dialog>
`;

export function mountConfirm(){ document.body.insertAdjacentHTML('beforeend', template); }
