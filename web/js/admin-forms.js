import {t} from './i18n.js';
export function dirtyForms(submitter) {
 return Array.from(document.querySelectorAll('[data-change-form][data-form-dirty="true"]')).some(form=>!submitter||form!==submitter.form&&!form.contains(submitter));
}
export function previewTheme(event) {
 const button=event.target.closest('[data-theme-preview],[data-template-preview]');
 if(!button)return;
 const form=button.form||button.closest('form');
 if(!form.reportValidity())return;
 // A native POST keeps unsaved settings out of URLs and opens a private preview tab.
 const preview=document.createElement('form');
 preview.method='post';preview.action=button.hasAttribute?.('data-template-preview')?'/admin/themes/editor/preview':'/admin/themes/preview';preview.target='_blank';preview.hidden=true;
 preview.setAttribute('hx-boost','false');
 for(const [name,value] of new FormData(form)){
  const input=document.createElement('input');input.type='hidden';input.name=name;input.value=value;preview.append(input);
 }
 document.body.append(preview);preview.submit();preview.remove();
}

export function selectContent(event) {
 const all=document.querySelector('[data-select-all]');
 const formID=all?.dataset?.selectionForm||'content-bulk';
 const selector='[name="selection"][form="'+formID+'"]';
 if(!all || !(event.target===all || event.target.matches?.(selector)))return;
 const boxes=Array.from(document.querySelectorAll(selector)).filter(box=>!box.disabled);
 if(event.target===all)boxes.forEach(box=>{box.checked=all.checked;});
 const count=boxes.filter(box=>box.checked).length;
 all.checked=boxes.length>0&&count===boxes.length;all.indeterminate=count>0&&count<boxes.length;
 document.querySelector('[data-selection-count]').textContent=t('Выбрано: ')+count;
 document.querySelectorAll('[data-bulk-action]').forEach(button=>{button.disabled=count===0;});
}

export function settingsSaved(event) {
 const form=event.target.form||event.target.closest?.('[data-change-form]');
 if(form)form.dataset.formDirty='false';
}

const pendingFields=new WeakMap();
export function freezeForms(root) {
 const fields=new Set();
 root?.querySelectorAll('[data-change-form]:not([data-page-editor]):not([data-template-editor])').forEach(form=>{
  for(const field of form.elements)fields.add(field);
 });
 for(const field of fields){
  const state=pendingFields.get(field)||{count:0,disabled:field.disabled};
  state.count++;pendingFields.set(field,state);field.disabled=true;
 }
 return ()=>{
  for(const field of fields){
   const state=pendingFields.get(field);
   if(!state)continue;
   if(--state.count===0){field.disabled=state.disabled;pendingFields.delete(field);}
  }
 };
}
