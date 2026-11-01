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
 const close=document.createElement('button');close.type='button';close.className='gp-toast-close';close.setAttribute('aria-label',t('Dismiss notification'));close.textContent='×';
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

export async function confirmAction(message, trigger, label=t('Delete')) {
 const popup=document.getElementById('system-confirm');
 if(pendingConfirmation)return false;
 if(!popup||!window.templ?.dialog){toast(t('Could not open the confirmation dialog. Reload the page and try again.'),'error');return false;}
 pendingConfirmation=true;
 popup.querySelector('#system-confirm-description').textContent=message;
 popup.querySelector('#system-confirm-title').textContent=label===t('Move to trash')?t('Move to trash?'):[t('Delete'),t('Delete permanently')].includes(label)?t('Confirm deletion'):t('Confirm action');
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
 if(action==='trash')return {message:t('Move the selected content to trash? You can restore it later.'),label:t('Move to trash')};
 if(action==='delete'&&/\/(content\/bulk|pages\/\d+\/state)$/.test(form?.getAttribute('action')||''))return {message:t('Permanently delete the selected content and its revision history? This cannot be undone.'),label:t('Delete permanently')};
 if(action==='delete')return {message:t('Delete the selected category or tag? Its content associations will be removed.'),label:t('Delete')};
 if(form?.getAttribute('action')==='/admin/updates/install')return {message:t('Install the update? GoPress will create a backup and temporarily enable site maintenance.'),label:t('Update')};
 const question=element?.closest?.('[hx-confirm]')?.getAttribute('hx-confirm');
 if(question&&form?.getAttribute('action')==='/admin/themes/editor/save'&&value('intent')==='restore')return {message:question,label:t('Restore')};
 return question?{message:question,label:form?.getAttribute('action')?.endsWith('/backups/restore')?t('Restore'):t('Delete')}:null;
}
export function actionMessage(element, parameters={}) {
 if(element?.closest?.('form')?.getAttribute('action')==='/admin/updates/install')return t('Pre-update verification started.');
 const action=parameters.action;
 if(action==='trash')return t('Content moved to trash.');
 if(action==='restore')return t('Content restored.');
 if(action==='delete'||element?.closest?.('form')?.getAttribute('action')?.endsWith('/delete'))return t('Deleted.');
 return t('Changes saved.');
}
