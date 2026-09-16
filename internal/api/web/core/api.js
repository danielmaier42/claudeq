export async function api(m,path,body,signal){
  // A Blob/File body is sent as-is (an upload); anything else is JSON.
  const raw=body instanceof Blob;
  const r=await fetch(path,{method:m,signal,headers:body?{'Content-Type':raw?(body.type||'application/octet-stream'):'application/json'}:{},body:raw?body:(body?JSON.stringify(body):undefined)});
  if(!r.ok&&r.status!==204){let e;try{e=(await r.json()).error}catch{e=r.statusText} throw new Error(e);}
  const ct=r.headers.get('content-type')||''; return ct.includes('json')?r.json():r.text();
}
