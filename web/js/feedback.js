import {t} from './i18n.js';
let pendingConfirmation = false;

export function toast(message, kind = 'success', content) {
 const region=document.getElementById('system-toasts');
 if(!region||!message)return;
 const item=document.createElement('div');item.className='gp-toast';item.dataset.kind=kind;
 if(kind==='error')item.setAttribute('role','alert');
 const mark=document.createElement('span');mark.className='gp-toast-mark';mark.setAttribute('aria-hidden','true');mark.textContent=kind==='error'?'!':'✓';
 const text=document.createElement('div');text.className='gp-toast-content';
 if(content)text.append(...Array.from(content.childNodes).map(node=>node.cloneNode(true)));else text.textContent=message;
 const close=document.createElement('button');close.type='button';close.className='gp-toast-close';close.setAttribute('aria-label',t('Закрыть уведомление'));close.textContent='×';
 item.append(mark,text,close);region.append(item);
 while(region.children.length>3)region.firstElementChild.remove();
 let timer;const pause=()=>clearTimeout(timer);const dismiss=()=>{pause();item.remove();};
 const resume=()=>{pause();timer=setTimeout(dismiss,kind==='error'?10000:5000);};
 close.addEventListener('click',dismiss);item.addEventListener('pointerenter',pause);item.addEventListener('pointerleave',resume);item.addEventListener('focusin',pause);item.addEventListener('focusout',resume);resume();
}

export function showNotices(root=document.querySelector('#main')) {
 let count=0;
 root?.querySelectorAll('.notice:not(noscript .notice)').forEach(node=>{
  if(node.dataset.toastShown)return;
  node.dataset.toastShown='true';
  toast(node.textContent.trim(),node.classList.contains('notice-error')?'error':'success',node);
  node.hidden=true;count++;
 });
 return count;
}

export async function confirmAction(message, trigger, label=t('Удалить')) {
 const popup=document.getElementById('system-confirm');
 if(pendingConfirmation)return false;
 if(!popup||!window.templ?.dialog){toast(t('Не удалось открыть подтверждение. Обновите страницу и повторите попытку.'),'error');return false;}
 pendingConfirmation=true;
 popup.querySelector('#system-confirm-description').textContent=message;
 popup.querySelector('#system-confirm-title').textContent=label===t('В корзину')?t('Переместить в корзину?'):[t('Удалить'),t('Удалить навсегда')].includes(label)?t('Подтвердите удаление'):t('Подтвердите действие');
 popup.querySelector('[data-confirm-result="accept"]').textContent=label;
 return new Promise(resolve=>{
  let accepted=false;
  const choose=event=>{
   const button=event.target.closest('[data-confirm-result]');if(!button)return;
   accepted=button.dataset.confirmResult==='accept';window.templ.dialog.close(popup);
  };
  const finish=()=>{popup.removeEventListener('click',choose);pendingConfirmation=false;resolve(accepted);};
  popup.addEventListener('click',choose);popup.addEventListener('dialog-close',finish,{once:true});
  window.templ.dialog.open(popup,trigger);
 });
}

export function deletionPrompt(element, parameters={}) {
 const form=element?.form||element?.closest?.('form');
 // Other submit buttons are not submitted controls; they must never choose the action.
 const value=name=>parameters[name]??(element?.name===name?element.value:form?.querySelector(`input[name="${name}"]:not([type="submit"]):not([type="button"]):not([type="reset"]),select[name="${name}"],textarea[name="${name}"]`)?.value);
 const action=value('action');
 if(action==='trash')return {message:t('Переместить выбранные материалы в корзину? Их можно будет восстановить.'),label:t('В корзину')};
 if(action==='delete'&&/\/(content\/bulk|pages\/\d+\/state)$/.test(form?.getAttribute('action')||''))return {message:t('Удалить выбранные материалы навсегда вместе с историей изменений? Восстановление будет невозможно.'),label:t('Удалить навсегда')};
 if(action==='delete')return {message:t('Удалить выбранную категорию или тег? Связи с материалами будут удалены.'),label:t('Удалить')};
 const question=element?.closest?.('[hx-confirm]')?.getAttribute('hx-confirm');
 return question?{message:question,label:form?.getAttribute('action')?.endsWith('/backups/restore')?t('Восстановить'):t('Удалить')}:null;
}
export function actionMessage(element, parameters={}) {
 const action=parameters.action;
 if(action==='trash')return t('Материал перемещён в корзину.');
 if(action==='restore')return t('Материал восстановлен.');
 if(action==='delete'||element?.closest?.('form')?.getAttribute('action')?.endsWith('/delete'))return t('Удалено.');
 return t('Изменения сохранены.');
}
