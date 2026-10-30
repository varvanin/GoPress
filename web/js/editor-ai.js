import {t} from './i18n.js';

export function mountEditorAI(form,editor,state,signal) {
 const panel=form.querySelector('[data-editor-ai]');
 if(!panel)return;
 const prompt=panel.querySelector('[data-ai-prompt]'),context=panel.querySelector('[data-ai-context]');
 const generate=panel.querySelector('[data-ai-generate]'),result=panel.querySelector('[data-ai-result]');
 const output=panel.querySelector('[data-ai-output]'),insert=panel.querySelector('[data-ai-insert]'),status=panel.querySelector('[data-ai-status]');
 let busy=false,resultLanguage='',request;
 const language=()=>form.elements.content_language.value==='original'?form.elements.source_language.value:form.elements.content_language.value;
 signal.addEventListener('abort',()=>request?.abort(),{once:true});
 generate.addEventListener('click',async()=>{
  if(busy||state.pending||form.dataset.readonly==='true')return;
  if(!prompt.value.trim()){prompt.focus();return;}
  const text=context.checked?editor.getText():'';
  if(new TextEncoder().encode(text).length>64000){status.textContent=t('Контекст слишком большой. Отключите учёт текста материала.');return;}
  const requestedLanguage=language();
  request=new AbortController();
  busy=true;generate.disabled=true;status.textContent=t('ИИ готовит текст…');
  try {
   const response=await fetch('/admin/pages/ai',{method:'POST',credentials:'same-origin',signal:request.signal,headers:{'Content-Type':'application/json','X-CSRF-Token':form.elements.csrf.value},body:JSON.stringify({id:Number(form.elements.id.value),prompt:prompt.value,title:form.elements.title.value,language:requestedLanguage,text})});
   const data=await response.json().catch(()=>({}));
   if(!response.ok)throw new Error(data.error||t('Не удалось получить текст. Проверьте подключение или повторите позже.'));
   if(signal.aborted)return;
   output.value=data.text;resultLanguage=requestedLanguage;result.hidden=false;
   status.textContent=t('Проверьте текст перед вставкой.');
  }catch(error){if(!signal.aborted)status.textContent=error.name==='AbortError'?t('Запрос отменён.'):error.message;}
  finally {busy=false;if(!signal.aborted)generate.disabled=false;}
 },{signal});
 insert.addEventListener('click',()=>{
  if(state.pending||state.uploading||form.dataset.readonly==='true'||!output.value)return;
  if(language()!==resultLanguage){status.textContent=t('Переключитесь на язык, для которого создан этот текст.');return;}
  const paragraphs=output.value.replace(/\r\n?/g,'\n').split(/\n\s*\n/).filter(text=>text.trim()).map(text=>({type:'paragraph',content:[{type:'text',text:text.trim()}]}));
  editor.chain().focus('end').insertContentAt(editor.state.doc.content.size,paragraphs).run();
  result.hidden=true;output.value='';status.textContent=t('Текст добавлен в материал.');
 },{signal});
}
