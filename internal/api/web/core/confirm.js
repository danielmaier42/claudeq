import {$} from './dom.js';

export function confirmSheetAsk(text,yesLabel,noLabel){ return new Promise(res=>{ $('#confirmText').textContent=text; $('#confirmYes').textContent=yesLabel||'Delete'; $('#confirmNo').textContent=noLabel||'Cancel';
  const dlg=$('#confirmSheet'); let done=false;
  const finish=v=>{ if(done)return; done=true;
    $('#confirmYes').removeEventListener('click',onYes); $('#confirmNo').removeEventListener('click',onNo);
    dlg.removeEventListener('close',onClose); dlg.close(); res(v); };
  const onYes=()=>finish(true), onNo=()=>finish(false), onClose=()=>finish(false);
  $('#confirmYes').addEventListener('click',onYes); $('#confirmNo').addEventListener('click',onNo); dlg.addEventListener('close',onClose);
  dlg.showModal(); }); }

const template = `
<dialog id="confirmSheet">
  <div class="sheet-bd" style="padding-top:20px"><b id="confirmText">Are you sure?</b></div>
  <div class="sheet-ft"><button class="btn" id="confirmNo">Cancel</button><button class="btn danger" id="confirmYes">Delete</button></div>
</dialog>
`;

export function mountConfirm(){ document.body.insertAdjacentHTML('beforeend', template); }
