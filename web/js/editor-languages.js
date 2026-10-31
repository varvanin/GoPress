import {confirmAction} from './feedback.js';
import {t} from './i18n.js';
const seoFields = {title:'seo_title',description:'seo_description',canonical:'seo_canonical',social_title:'seo_social_title',social_description:'seo_social_description',social_image:'seo_social_image',nofollow:'seo_nofollow',noindex:'noindex'};
const emptyDocument = {type:'doc',content:[{type:'paragraph'}]};

export function mountEditorLanguages(form, editor, signal) {
  let selector = form.elements.content_language;
  if (!selector) return;
  const translations = JSON.parse(form.elements.translations.value) || {};
  let original = {seo:{}};
  let active = 'original';
  const read = () => {
    const seo = {};
    for (const [key,name] of Object.entries(seoFields)) {
      const field=form.elements[name];
      if (field) seo[key]=field.type==='checkbox'?field.checked:field.value;
    }
    if(active==='original')seo.language=form.elements.source_language?.value||original.seo.language||'ru';
    return {title:form.elements.title.value,document:form.elements.document.value,seo};
  };
  original=read();
  const sync = () => {
    const value=read();
    if(active==='original')original=value;
    else if(translations[active])translations[active]=value;
    else {
      const content=JSON.parse(value.document).content;
      const empty=content?.length===1&&content[0].type==='paragraph'&&!content[0].content?.length;
      if(value.title||!empty||Object.values(value.seo).some(Boolean))translations[active]=value;
    }
  };
  const serialize = () => {
    sync();
    const otherLanguages={...translations};delete otherLanguages[active];
    form.elements.original_translation.value=active==='original'?'':JSON.stringify(original);
    form.elements.translations.value=JSON.stringify(otherLanguages);
  };
  const switchLanguage = () => {
    sync();
    active=selector.value;
    const value=active==='original'?original:translations[active] || {title:'',document:JSON.stringify(emptyDocument),seo:{}};
    form.elements.title.value=value.title;
    form.elements.document.value=value.document;
    // A fresh state keeps undo history from crossing language boundaries.
    editor.commands.setContent(JSON.parse(value.document),{emitUpdate:false});
    editor.view?.updateState(editor.state.constructor.create({schema:editor.schema,doc:editor.state.doc,plugins:editor.state.plugins}));
    for(const [key,name] of Object.entries(seoFields)) {
      const field=form.elements[name];
      if(field) { if(field.type==='checkbox')field.checked=!!value.seo[key];else field.value=value.seo[key]||''; }
    }
    if(Number(form.elements.id?.value)>0) {
      const url=new URL(window.location.href);url.searchParams.set('lang',active);
      window.history.replaceState({...window.history.state},'',url.href);
    }
    serialize();
    const preview=form.querySelector('[data-saved-preview]');
    if(preview) { const url=new URL(preview.href,window.location.href);url.searchParams.set('lang',active);preview.href=url.href; }
  };
  form.addEventListener('change',event=>{if(event.target===selector)switchLanguage();},{signal});
  form.addEventListener('editor:serialize',serialize,{signal});
  form.addEventListener('editor:original-saved',event=>{
    const {title,expectedTitle}=event.detail;
    if(original.title!==expectedTitle||active==='original'&&form.elements.title.value!==expectedTitle)return;
    original.title=title;
    if(active==='original')form.elements.title.value=title;
    serialize();
  },{signal});
  const enableAdd = () => { const button=form.querySelector('[data-add-content-language]');if(button)button.disabled=false; };
  enableAdd();
  async function changeLanguages(language, translation, remove=false, source=form.elements.source_language.value) {
    sync();
    const panel=form.querySelector('[data-editor-languages]');
    const controls=[...panel.querySelectorAll('button')];
    const disabled=controls.map(control=>control.disabled);
    try {
      const data=new URLSearchParams({source_language:source,content_language:language});
      const codes=new Set(Object.keys(translations));
      if(remove)codes.delete(active);else if(language!=='original')codes.add(language);
      for(const code of codes)data.append('languages',code);
      controls.forEach(control=>{control.disabled=true;});
      const response=await fetch('/admin/pages/languages',{method:'POST',credentials:'same-origin',signal,headers:{'X-CSRF-Token':form.elements.csrf.value},body:data});
      if(!response.ok)throw new Error(t('Не удалось добавить язык. Попробуйте ещё раз.'));
      const html=await response.text();
      if(!form.isConnected)return;
      sync();
      if(remove)delete translations[active];
      else if(translation)translations[language]=translation;
      original.seo.language=source;
      active='original';
      form.elements.title.value=original.title;
      form.elements.document.value=original.document;
      for(const [key,name] of Object.entries(seoFields)){const field=form.elements[name];if(field){if(field.type==='checkbox')field.checked=!!original.seo[key];else field.value=original.seo[key]||'';}}
      panel.outerHTML=html;
      selector=form.elements.content_language;
      switchLanguage();
      enableAdd();
      form.dispatchEvent(new Event('editor:language-added'));
    }catch(error){
      if(signal?.aborted)return;
      controls.forEach((control,index)=>{control.disabled=disabled[index];});
      const status=form.querySelector('#editor-status');if(status)status.textContent=error.message;
    }
  }
  form.addEventListener('click',async event=>{
    const remove=event.target.closest?.('[data-remove-content-language]');
    if(remove&&!remove.disabled){
      if(await confirmAction(t('Удалить этот перевод из черновика?'),remove,t('Удалить перевод')))void changeLanguages('original',null,true);
      return;
    }
    const source=event.target.closest?.('[data-set-original-language]');
    if(source&&!source.disabled){void changeLanguages(selector.value,null,false,form.elements.original_content_language.value);return;}
    const button=event.target.closest?.('[data-add-content-language]');
    if(!button||button.disabled)return;
    const language=form.elements.new_content_language.value;
    if(!language||language===form.elements.source_language.value||translations[language])return;
    void changeLanguages(language,{title:'',document:JSON.stringify(emptyDocument),seo:{}});
  },{signal});
  if(selector.value!=='original')switchLanguage();
}
