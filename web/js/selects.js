// A cloned menu row needs its own trigger, popup and list identifiers before mounting.
export function rekeySelects(root){
 for(const select of root.querySelectorAll('.gp-select')){
  const id='select-'+crypto.getRandomValues(new Uint32Array(3)).join('-');
  const trigger=select.querySelector('[data-slot="select-trigger"]');
  const popup=select.querySelector('[data-slot="select-content"]');
  trigger.id=id+'-trigger';trigger.setAttribute('data-templ-controls',id);trigger.removeAttribute('aria-controls');trigger.removeAttribute('aria-activedescendant');
  popup.id=id;popup.querySelector('[role="listbox"]').id=id+'-list';
  const input=select.querySelector('input[aria-hidden="true"]');if(input.id)input.id=id+'-hidden-input';
 }
}
