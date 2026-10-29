// FAQ uses native details on the public site; only the editor needs scripting.
export function editFAQ(event) {
 const code=event.target.closest?.('[data-faq-code]');
 if(code){code.select();return;}
 const action=event.target.closest?.('[data-faq-add],[data-faq-up],[data-faq-down],[data-faq-remove]');
 const form=action?.closest('[data-faq-editor]');
 if(!form)return;
 const list=form.querySelector('[data-faq-items]'),item=action.closest('.faq-item');
 if(action.hasAttribute('data-faq-add')){
  if(list.children.length>=50)return;
  list.append(form.querySelector('[data-faq-template]').content.cloneNode(true));
  globalThis.htmx?.process(list.lastElementChild);
  list.lastElementChild.querySelector('button,summary,input:not([type=hidden]),textarea').focus();
 } else if(action.hasAttribute('data-faq-remove')){
  if(list.children.length<=1){item.querySelector('button,summary,input:not([type=hidden]),textarea').focus();return;}
  item.remove();
 } else if(action.hasAttribute('data-faq-up')&&item.previousElementSibling)list.insertBefore(item,item.previousElementSibling);
 else if(action.hasAttribute('data-faq-down')&&item.nextElementSibling)list.insertBefore(item.nextElementSibling,item);
 else return;
 if(form.dataset)form.dataset.formDirty='true';
 form.dispatchEvent(new Event('input',{bubbles:true}));
}
if(typeof document!=='undefined')document.addEventListener('click',editFAQ);
