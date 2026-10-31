import {sourceDiff} from './template-diff.js';
import {EditorView,basicSetup} from 'codemirror';
import {EditorState,Compartment} from '@codemirror/state';
import {Decoration,MatchDecorator,ViewPlugin,keymap} from '@codemirror/view';
import {HighlightStyle,syntaxHighlighting} from '@codemirror/language';
import {tags} from '@lezer/highlight';
import {indentWithTab,isolateHistory} from '@codemirror/commands';
import {t} from './i18n.js';
import {html} from '@codemirror/lang-html';
import {css} from '@codemirror/lang-css';
import {javascript} from '@codemirror/lang-javascript';

export function sourceLanguage(filename){
 if(filename.endsWith('.css'))return css();
 if(filename.endsWith('.js'))return javascript();
 return html();
}
export const goDirectivePattern=/\{\{[^\n]*?\}\}/g;
const directives=new MatchDecorator({regexp:goDirectivePattern,decoration:Decoration.mark({class:'cm-go-template'})});
const goTemplates=ViewPlugin.fromClass(class {
 constructor(view){this.decorations=directives.createDeco(view);}
 update(update){this.decorations=directives.updateDeco(update,this.decorations);}
},{decorations:instance=>instance.decorations});

export function mountSourceEditor(form){
 const source=form.querySelector('[data-code-source]');
 const host=document.createElement('div');host.className='template-codemirror';source.after(host);
 const formatButton=form.querySelector('[data-format-source]'),formatError=form.querySelector('[data-format-error]');
 let disposed=false,formatting=false,pending=false,diffTimer;
 const original=form.querySelector('[data-original-source]')?.value??source.value,diff=form.querySelector('[data-source-diff]'),diffButton=form.querySelector('[data-source-diff-toggle]');
 let updateOperations=()=>{},renderDiff=()=>{};
 const access=new Compartment();
 const appearance=new Compartment();
 const skin=()=>EditorView.theme({},{dark:document.documentElement.dataset.adminTheme!=='light'});
 const view=new EditorView({
  parent:host,
  state:EditorState.create({doc:source.value,extensions:[
   basicSetup,EditorView.lineWrapping,syntaxHighlighting(HighlightStyle.define([
    {tag:tags.tagName,color:'var(--code-tag)'},{tag:tags.attributeName,color:'var(--code-attribute)'},
    {tag:tags.string,color:'var(--code-string)'},{tag:tags.keyword,color:'var(--code-keyword)'},
    {tag:tags.comment,color:'var(--code-comment)'},{tag:tags.number,color:'var(--code-number)'},
    {tag:tags.propertyName,color:'var(--code-property)'},{tag:tags.variableName,color:'var(--code-variable)'},
   ])),sourceLanguage(source.dataset.filename),goTemplates,keymap.of([indentWithTab]),
   access.of([EditorState.readOnly.of(source.readOnly),EditorView.editable.of(!source.readOnly)]),
   EditorView.cspNonce.of(document.querySelector('meta[name="style-nonce"]')?.content||''),
   EditorView.contentAttributes.of({'aria-label':source.labels?.[0]?.textContent||source.dataset.filename,'spellcheck':'false'}),
   appearance.of(skin()),
   EditorView.updateListener.of(update=>{
    if(update.docChanged){source.value=update.state.doc.toString();source.dispatchEvent(new Event('input',{bubbles:true}));updateOperations();if(diff&&!diff.hidden){clearTimeout(diffTimer);diffTimer=setTimeout(renderDiff,150);}}
   }),
  ]}),
 });
 updateOperations=()=>{const dirty=view.state.doc.toString()!==original;form.querySelectorAll('[data-file-operation]').forEach(button=>button.disabled=pending||source.readOnly||dirty);const warning=form.querySelector('[data-file-operation-warning]');if(warning)warning.hidden=!dirty;};
 renderDiff=()=>{if(!diff)return;try{const hunk=sourceDiff(original,view.state.doc.toString());diff.querySelector('[data-diff-status]').textContent=hunk.changed?t('Изменения начинаются со строки ')+hunk.start:t('Нет изменений.');diff.querySelector('[data-diff-before]').textContent=hunk.before;diff.querySelector('[data-diff-after]').textContent=hunk.after;}catch{diff.querySelector('[data-diff-status]').textContent=t('Файл слишком большой для сравнения.');}};
 const toggleDiff=()=>{diff.hidden=!diff.hidden;diffButton.setAttribute('aria-expanded',String(!diff.hidden));if(!diff.hidden)renderDiff();};
 diffButton?.addEventListener('click',toggleDiff);updateOperations();
 const updateAppearance=()=>view.dispatch({effects:appearance.reconfigure(skin())});
 document.addEventListener('admin-theme-change',updateAppearance);
 source.hidden=true;
 const format=async()=>{
  if(formatting||!view.state.facet(EditorView.editable))return;
  const before=view.state.doc;
  formatting=true;formatButton.disabled=true;if(formatError)formatError.hidden=true;
  try{
   const {formatTemplateSource}=await import('/assets/template-formatter.js');
   if(disposed||view.state.doc!==before||!view.state.facet(EditorView.editable))return;
   const formatted=formatTemplateSource(before.toString(),source.dataset.filename);
   if(formatted!==before.toString())view.dispatch({changes:{from:0,to:before.length,insert:formatted},selection:{anchor:Math.min(view.state.selection.main.head,formatted.length)},annotations:isolateHistory.of('full')});
   view.focus();
  }catch(error){
   if(!disposed&&formatError){formatError.textContent=t('Не удалось отформатировать код. Исходный текст сохранён.');formatError.hidden=false;}
  }finally{formatting=false;if(!disposed)formatButton.disabled=!view.state.facet(EditorView.editable);}
 };
 if(formatButton){formatButton.disabled=source.readOnly;formatButton.addEventListener('click',format);}
 const save=event=>{if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='s'){event.preventDefault();if(!source.readOnly)form.requestSubmit();}};
 form.addEventListener('keydown',save);
 return {form,setPending(value){pending=value;updateOperations();view.dispatch({effects:access.reconfigure([EditorState.readOnly.of(pending||source.readOnly),EditorView.editable.of(!pending&&!source.readOnly)])});if(formatButton)formatButton.disabled=pending||source.readOnly||formatting;},destroy(){disposed=true;clearTimeout(diffTimer);diffButton?.removeEventListener('click',toggleDiff);formatButton?.removeEventListener('click',format);document.removeEventListener('admin-theme-change',updateAppearance);source.value=view.state.doc.toString();view.destroy();host.remove();source.hidden=false;form.removeEventListener('keydown',save);}};
}
