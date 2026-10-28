import {t} from './i18n.js';
import {rekeySelects} from './selects.js';
import {toast} from './feedback.js';

export function mountFormBuilder() {
 const form=document.querySelector('[data-form-builder]');
 const preview=document.querySelector('[data-form-preview]');if(!form||!preview)return;
 const rows=Array.from(form.querySelector('[data-form-fields]').children);
 const heading=document.createElement('h3');heading.textContent=form.elements.namedItem('name').value||t('Название формы');
 preview.replaceChildren(heading);
 rows.forEach((row,index)=>{
  const type=row.querySelector('[name="field_type"]').value;
  const required=row.querySelector('[name="field_required"]').value==='true';
  const options=row.querySelector('[name="field_options"]');
  row.querySelector('[data-form-field-options]').hidden=type!=='select';
  if(type!=='select')options.value='';
  row.querySelector('[data-form-field-up]').disabled=index===0;
  row.querySelector('[data-form-field-down]').disabled=index===rows.length-1;
  row.querySelector('[data-remove-form-field]').disabled=rows.length===1;
  const label=document.createElement('label'),caption=document.createElement('span');
  caption.textContent=(row.querySelector('[name="field_label"]').value||t('Новое поле'))+(required?' *':'');label.append(caption);
  const control=document.createElement(type==='textarea'?'textarea':type==='select'?'select':'input');
  if(control.tagName==='INPUT')control.type=type;
  if(type==='select')for(const text of [t('Выберите вариант'),...options.value.split('\n').filter(value=>value.trim())]){const option=document.createElement('option');option.textContent=text;control.append(option);}
  control.disabled=true;label.append(control);preview.append(label);
 });
 const submit=document.createElement('button');submit.type='button';submit.disabled=true;submit.textContent=form.elements.namedItem('button').value||t('Отправить');preview.append(submit);
 form.querySelector('[data-add-form-field]').disabled=rows.length>=20;
}
export function updateFormBuilder(event) {
 if(event.target.closest?.('[data-form-builder]'))mountFormBuilder();
}
export async function editFormBuilder(event) {
 const copy=event.target.closest('[data-copy-form]');
 if(copy){const input=document.querySelector('#form-code');try{await navigator.clipboard.writeText(input.value);toast(t('Код формы скопирован.'));}catch{input.focus();input.select();toast(t('Код выделен — скопируйте его.'));}return;}
 const button=event.target.closest('[data-add-form-field],[data-remove-form-field],[data-form-field-up],[data-form-field-down]');
 const form=button?.closest('[data-form-builder]');if(!form)return;
 const fields=form.querySelector('[data-form-fields]');const row=button.closest('[data-form-field]');
 if(button.hasAttribute('data-add-form-field')){
  if(fields.children.length>=20)return;
  const fragment=form.querySelector('template[data-form-field-template]').content.cloneNode(true);
  rekeySelects(fragment);fields.append(fragment);fields.lastElementChild.querySelector('input').focus();
 }else if(button.hasAttribute('data-remove-form-field')){
  if(fields.children.length<=1)return;
  const next=row.nextElementSibling||row.previousElementSibling;row.remove();next.querySelector('input').focus();
 }else if(button.hasAttribute('data-form-field-up')){
  if(row.previousElementSibling)fields.insertBefore(row,row.previousElementSibling);
 }else if(row.nextElementSibling)fields.insertBefore(row.nextElementSibling,row);
 form.dataset.formDirty='true';form.querySelector('[data-form-builder-status]').textContent=t('Есть несохранённые изменения.');
 mountFormBuilder();
}
