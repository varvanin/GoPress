import {t} from './i18n.js';
const prefix='gopress:draft:',maxSize=4*1024*1024,maxAge=7*86400000;
const allowed=new Set(['id','version','kind','title','slug','document','translations','original_translation','content_language','source_language','cover','term_ids','parent_id','order','template','seo_title','seo_description','seo_canonical','seo_social_title','seo_social_description','seo_social_image','seo_nofollow','noindex']);
export function draftSnapshot(form){
 form.dispatchEvent(new Event('editor:serialize'));
 const data=new URLSearchParams();for(const [name,value]of new FormData(form))if(allowed.has(name)&&typeof value==='string')data.append(name,value);
 return data.toString();
}
export function readDraft(storage,key,now=Date.now()){
 try{const raw=storage.getItem(key);if(!raw||raw.length>maxSize+4096)return null;const d=JSON.parse(raw);if(typeof d.token!=='string'||typeof d.snapshot!=='string'||d.snapshot.length>maxSize||!Number.isFinite(d.at)||d.at>now+60000||now-d.at>maxAge)return null;return d;}catch{return null;}
}
export function mountDraftRecovery(form,state,signal){
 const panel=form.closest('#main')?.querySelector('[data-draft-recovery]');if(!panel||form.dataset.readonly==='true')return;
 let storage;try{storage=window.localStorage;}catch{return;}
 const scope=prefix+location.origin+':'+form.dataset.draftUser+':';
 const key=()=>scope+form.elements.kind.value+':'+form.elements.id.value;
 let currentKey=key(),token=null,timer;
 const existing=readDraft(storage,currentKey);
 if(existing&&form.dataset.recovered!=='true'){
  const fields=new URLSearchParams(existing.snapshot),recovery=panel.querySelector('[data-recovery-form]');
  if(fields.get('id')===form.elements.id.value&&fields.get('kind')===form.elements.kind.value){
   recovery.elements.snapshot.value=existing.snapshot;
   const same=fields.get('version')===form.elements.version.value;
   panel.querySelector('[data-recovery-restore]').disabled=!same;
   panel.querySelector('[data-recovery-description]').textContent=t(same?'Копия хранится в этом браузере до семи дней. Восстановление откроет её для проверки.':'Серверная версия изменилась. Скачайте локальную копию для сравнения; автоматическое восстановление отключено.');
   panel.hidden=false;
  }
 }
 const clear=()=>{
  clearTimeout(timer);
  try{if(token&&readDraft(storage,currentKey)?.token===token)storage.removeItem(currentKey);}catch{}
 };
 const save=()=>{
  clearTimeout(timer);
  if(!state.dirty||state.submitting||state.pending||signal.aborted)return;
  try{
   const snapshot=draftSnapshot(form);if(snapshot.length>maxSize)return;
   const next=key();if(next!==currentKey){clear();currentKey=next;}
   token=crypto.randomUUID();
   // Keep at most ten recent drafts for this browser; quota failures never interrupt editing.
   const keys=[];for(let i=0;i<storage.length;i++){const k=storage.key(i);if(k?.startsWith(scope))keys.push(k);}
   keys.sort((a,b)=>(readDraft(storage,b)?.at||0)-(readDraft(storage,a)?.at||0));
   for(const k of keys.slice(9))if(k!==currentKey)storage.removeItem(k);
   storage.setItem(currentKey,JSON.stringify({at:Date.now(),token,snapshot}));
  }catch{}
 };
 const schedule=()=>{clearTimeout(timer);timer=setTimeout(save,500);};
 panel.querySelector('[data-recovery-discard]').addEventListener('click',()=>{try{if(readDraft(storage,currentKey)?.token===existing?.token)storage.removeItem(currentKey);}catch{}panel.hidden=true;},{signal});
 panel.querySelector('[data-recovery-download]').addEventListener('click',()=>{
  if(!existing)return;const url=URL.createObjectURL(new Blob([JSON.stringify({fields:[...new URLSearchParams(existing.snapshot)]},null,2)],{type:'application/json'}));
  const link=document.createElement('a');link.href=url;link.download='gopress-local-draft.json';link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
 },{signal});
 form.addEventListener('editor:autosaved',()=>{if(state.dirty)save();else clear();},{signal});
 window.addEventListener('beforeunload',save,{signal});
 signal.addEventListener('abort',()=>clearTimeout(timer),{once:true});
 return {schedule,clear};
}
