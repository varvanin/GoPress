import {t} from './i18n.js';
import {confirmAction} from './feedback.js';
import {rekeySelects} from './selects.js';
// A menu item moves with its descendants, and only across siblings.
export function moveMenuBranch(items,index,direction) {
 const depth=items[index].depth;
 const endOf=start=>{let end=start+1;while(end<items.length&&items[end].depth>items[start].depth)end++;return end;};
 const end=endOf(index);
 if(direction==='up'){
  let before=index-1;
  while(before>=0&&items[before].depth>depth)before--;
  if(before<0||items[before].depth!==depth)return items;
  return [...items.slice(0,before),...items.slice(index,end),...items.slice(before,index),...items.slice(end)];
 }
 if(end>=items.length||items[end].depth!==depth)return items;
 const nextEnd=endOf(end);
 return [...items.slice(0,index),...items.slice(end,nextEnd),...items.slice(index,end),...items.slice(nextEnd)];
}
function rowDepth(row){return Number(row.querySelector('[name="depth"]').value);}
function updateSummary(row){
 row.querySelector('[data-menu-label]').textContent=row.querySelector('[name="label"]').value.trim()||t('Новая ссылка');
 row.dataset.depth=String(rowDepth(row));
}
export function updateMenuRow(event){const row=event.target.closest?.('[data-menu-row]');if(row)updateSummary(row);}
export async function editMenu(event) {
 const button=event.target.closest('[data-menu-add],[data-menu-add-page],[data-menu-remove],[data-menu-up],[data-menu-down]');
 const form=button?.closest('[data-menu-form]');if(!form)return;
 const items=form.querySelector('[data-menu-items]');
 const row=button.closest('[data-menu-row]');
 if(button.hasAttribute('data-menu-add')||button.hasAttribute('data-menu-add-page')){
  if(items.children.length>=50){form.querySelector('[data-menu-status]').textContent=t('Не больше 50 ссылок в одном меню.');return;}
  const fragment=form.querySelector('template[data-menu-template]').content.cloneNode(true);
  rekeySelects(fragment);items.append(fragment);
  const added=items.lastElementChild;
  if(button.hasAttribute('data-menu-add-page')){
   const choice=form.querySelector('[data-menu-page]');
   added.querySelector('[name="label"]').value=choice.querySelector('[data-slot="select-value"]').textContent.trim();
   added.querySelector('[name="url"]').value=choice.querySelector('input[aria-hidden="true"]').value;
   added.querySelector('details').open=false;
   updateSummary(added);added.querySelector('summary').focus();
  }else added.querySelector('input').focus();
 }else if(button.hasAttribute('data-menu-remove')){
  const label=row.querySelector('[name="label"]').value.trim()||t('Без названия');
  if(!await confirmAction(t('Удалить ссылку «')+label+t('» из меню? Изменение применится после сохранения.'),button)||!row.isConnected)return;
  let child=row.nextElementSibling;
  const depth=rowDepth(row);
  while(child&&rowDepth(child)>depth){const field=child.querySelector('[name="depth"]');const value=String(Number(field.value)-1);
   window.templ.select.setValue(child.querySelector('[data-slot="select-trigger"]'),value);updateSummary(child);child=child.nextElementSibling;}
  const next=row.nextElementSibling||row.previousElementSibling;row.remove();
  (next?.querySelector('summary')||form.querySelector('[data-menu-add]')).focus();
 }else{
  const entries=Array.from(items.children).map(row=>({row,depth:rowDepth(row)}));
  const moved=moveMenuBranch(entries,entries.findIndex(item=>item.row===row),button.hasAttribute('data-menu-up')?'up':'down');
  if(moved===entries)return;
  items.replaceChildren(...moved.map(item=>item.row));button.focus();
 }
 form.querySelector('[data-menu-empty]').hidden=items.children.length>0;
 form.dataset.formDirty='true';form.querySelector('[data-menu-status]').textContent=t('Есть несохранённые изменения.');
}
