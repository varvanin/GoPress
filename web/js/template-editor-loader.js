let mounted=null,revision=0;
export async function mountTemplateEditor(){
 const turn=++revision,form=document.querySelector('[data-template-editor]');
 if(mounted?.form===form)return;
 mounted?.destroy();mounted=null;
 if(!form)return;
 const selected=form.querySelector('[name="file"]')?.value;
 document.querySelectorAll('[data-template-file]').forEach(link=>{
  const active=link.dataset.templateFile===selected;
  link.classList.toggle('is-current',active);link.setAttribute('aria-current',active?'page':'false');
 });
 try{
  const {mountSourceEditor}=await import('/assets/template-editor.js');
  if(turn===revision&&document.querySelector('[data-template-editor]')===form)mounted=mountSourceEditor(form);
 }catch(error){console.error('Template editor could not load',error);}
}

export function setTemplatePending(pending){mounted?.setPending(pending);}
