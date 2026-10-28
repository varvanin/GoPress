import {EditorView,basicSetup} from 'codemirror';
import {EditorState,Compartment} from '@codemirror/state';
import {Decoration,MatchDecorator,ViewPlugin,keymap} from '@codemirror/view';
import {HighlightStyle,syntaxHighlighting} from '@codemirror/language';
import {tags} from '@lezer/highlight';
import {indentWithTab} from '@codemirror/commands';
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
 const status=form.querySelector('[data-code-status]');
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
    if(update.docChanged){source.value=update.state.doc.toString();source.dispatchEvent(new Event('input',{bubbles:true}));}
    if(status){const cursor=update.state.selection.main.head,line=update.state.doc.lineAt(cursor);status.textContent=`${line.number}:${cursor-line.from+1} · ${update.state.doc.lines}`;}
   }),
  ]}),
 });
 const updateAppearance=()=>view.dispatch({effects:appearance.reconfigure(skin())});
 document.addEventListener('admin-theme-change',updateAppearance);
 source.hidden=true;
 const save=event=>{if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='s'){event.preventDefault();if(!source.readOnly)form.requestSubmit();}};
 form.addEventListener('keydown',save);
 return {form,setPending(pending){view.dispatch({effects:access.reconfigure([EditorState.readOnly.of(pending||source.readOnly),EditorView.editable.of(!pending&&!source.readOnly)])});},destroy(){document.removeEventListener('admin-theme-change',updateAppearance);source.value=view.state.doc.toString();view.destroy();host.remove();source.hidden=false;form.removeEventListener('keydown',save);}};
}
