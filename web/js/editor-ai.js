import {t} from './i18n.js';
import {translationParts,parseAIFields,aiParagraphs} from './editor-ai-actions.js';

export function mountEditorAI(form,editor,state,signal) {
 const panel=form.querySelector('[data-editor-ai]');
 if(!panel)return;
 const prompt=panel.querySelector('[data-ai-prompt]'),context=panel.querySelector('[data-ai-context]');
 const generate=panel.querySelector('[data-ai-generate]'),result=panel.querySelector('[data-ai-result]');
 const output=panel.querySelector('[data-ai-output]'),insert=panel.querySelector('[data-ai-insert]'),status=panel.querySelector('[data-ai-status]'),undo=panel.querySelector('[data-ai-undo]');
 let busy=false,proposal,applied,request;
 const language=()=>form.elements.content_language.value==='original'?form.elements.source_language.value:form.elements.content_language.value;
 const sameDoc=doc=>editor.state.doc===doc||editor.state.doc.eq?.(doc);
 const changed=()=>status.textContent=t('The content changed. Generate a new result; current edits are preserved.');
 const setField=(name,value)=>{form.elements[name].value=value;form.elements[name].dispatchEvent(new Event('input',{bubbles:true}));};
 signal.addEventListener('abort',()=>request?.abort(),{once:true});
 generate.addEventListener('click',async()=>{
  if(busy||state.pending||state.uploading||form.dataset.readonly==='true')return;
  const mode=form.elements.ai_action?.value||'generate';
  if(mode==='generate'&&!prompt.value.trim()){prompt.focus();return;}
  let text=context.checked?editor.getText():'',title=form.elements.title.value,translation;
  const selection=editor.state.selection;
  if(['rewrite','shorten','continue'].includes(mode)){
   if(!selection||selection.empty){status.textContent=t('Select text in the editor first.');return;}
   text=editor.state.doc.textBetween(selection.from,selection.to,'\n\n');
   if(!text.trim()){status.textContent=t('Select text in the editor first.');return;}
  }else if(mode==='translate'){
   if(form.elements.content_language.value==='original'){status.textContent=t('Add a language and switch to its translation.');return;}
   form.dispatchEvent(new Event('editor:serialize'));
   try{const original=JSON.parse(form.elements.original_translation.value);translation=translationParts(JSON.parse(original.document));title=original.title;text=JSON.stringify(translation.texts);}catch{status.textContent=t('Could not prepare the translation. The original text was preserved.');return;}
  }else if(mode==='seo')text=editor.getText();
  if(new TextEncoder().encode(text).length>64000){status.textContent=t(mode==='generate'?'Контекст слишком большой. Отключите учёт текста материала.':'Текст слишком большой для одного запроса.');return;}
  const candidate={mode,language:language(),doc:editor.state.doc,title:form.elements.title.value,selection:selection&&{from:selection.from,to:selection.to},translation,seo:mode==='seo'?{title:form.elements.seo_title.value,description:form.elements.seo_description.value}:null};
  request=new AbortController();busy=true;generate.disabled=true;status.textContent=t('AI is writing…');
  try {
   const response=await fetch('/admin/pages/ai',{method:'POST',credentials:'same-origin',signal:request.signal,headers:{'Content-Type':'application/json','X-CSRF-Token':form.elements.csrf.value},body:JSON.stringify({id:Number(form.elements.id.value),mode,prompt:prompt.value,title,language:candidate.language,text})});
   const data=await response.json().catch(()=>({}));
   if(!response.ok)throw new Error(data.error||t('Could not get text. Check your connection or try again later.'));
   if(signal.aborted)return;
   if(typeof data.text!=='string'||!data.text.trim()||new TextEncoder().encode(data.text).length>64000)throw new Error(t('AI returned an invalid result. No changes were applied.'));
   if(mode==='translate'||mode==='seo'){
    try{candidate.fields=parseAIFields(mode,data.text,translation?.texts.length);}catch{throw new Error(t('AI returned an invalid result. No changes were applied.'));}
    output.value=candidate.fields.title+'\n\n'+(mode==='seo'?candidate.fields.description:candidate.fields.texts.join('\n\n'));
   }else output.value=data.text;
   proposal=candidate;result.hidden=false;insert.textContent=t(mode==='generate'?'Вставить в конец текста':'Применить результат');
   status.textContent=t('Review the text before inserting it.');
  }catch(error){if(!signal.aborted)status.textContent=error.name==='AbortError'?t('Request cancelled.'):error.message;}
  finally {busy=false;if(!signal.aborted)generate.disabled=false;}
 },{signal});
 insert.addEventListener('click',()=>{
  if(!proposal||state.pending||state.uploading||form.dataset.readonly==='true'||!output.value)return;
  if(language()!==proposal.language){status.textContent=t('Switch to the language this text was generated for.');return;}
  if(proposal.mode!=='generate'&&(!sameDoc(proposal.doc)||form.elements.title.value!==proposal.title)){changed();return;}
  const before={doc:editor.state.doc,title:form.elements.title.value,language:language(),seo:proposal.seo};
  try{
   if(proposal.mode==='seo'){
    if(form.elements.seo_title.value!==proposal.seo.title||form.elements.seo_description.value!==proposal.seo.description){changed();return;}
    setField('seo_title',proposal.fields.title);setField('seo_description',proposal.fields.description);
   }else if(proposal.mode==='translate'){
    const doc=proposal.translation.apply(proposal.fields.texts);
    editor.commands.setContent(doc);setField('title',proposal.fields.title);
   }else{
    const content=aiParagraphs(output.value);
    const position=proposal.mode==='generate'?editor.state.doc.content.size:proposal.mode==='continue'?proposal.selection.to:proposal.selection;
    editor.chain().focus('end').insertContentAt(position,proposal.mode==='continue'?[{type:'paragraph'},...content]:content).run();
   }
  }catch{status.textContent=t('AI returned an invalid result. No changes were applied.');return;}
  applied={...before,after:editor.state.doc,afterTitle:form.elements.title.value,afterSEO:proposal.mode==='seo'?{title:form.elements.seo_title.value,description:form.elements.seo_description.value}:null};
  if(undo)undo.hidden=false;
  result.hidden=true;output.value='';proposal=null;status.textContent=t('The result was applied to the draft.');
 },{signal});
 undo?.addEventListener('click',()=>{
  if(!applied||state.pending||state.uploading||form.dataset.readonly==='true')return;
  if(language()!==applied.language||!sameDoc(applied.after)||form.elements.title.value!==applied.afterTitle){changed();return;}
  if(applied.afterSEO){
   if(form.elements.seo_title.value!==applied.afterSEO.title||form.elements.seo_description.value!==applied.afterSEO.description){changed();return;}
   setField('seo_title',applied.seo.title);setField('seo_description',applied.seo.description);
  }else{editor.commands.setContent(applied.doc.toJSON());if(form.elements.title.value!==applied.title)setField('title',applied.title);}
  applied=null;undo.hidden=true;status.textContent=t('AI changes were undone.');
 },{signal});
}
