import {Extension} from '@tiptap/core';
import {Plugin} from '@tiptap/pm/state';
import {Fragment,Slice} from '@tiptap/pm/model';
import {t} from './i18n.js';

export const validAnchor=value=>typeof value==='string'&&/^[a-z][a-z0-9-]{0,63}$/.test(value);
// Pasted blocks get new anchor names explicitly; existing links keep their target.
export function stripPastedAnchors(slice) {
 const clean=fragment=>{
  const nodes=[];
  fragment.forEach(node=>{
   const content=clean(node.content);
   nodes.push(node.attrs.anchor?node.type.create({...node.attrs,anchor:null},content,node.marks):node.copy(content));
  });
  return Fragment.fromArray(nodes);
 };
 return new Slice(clean(slice.content),slice.openStart,slice.openEnd);
}
export const sectionAnchors=Extension.create({
 name:'sectionAnchors',
 addGlobalAttributes(){return[{types:['paragraph','heading'],attributes:{anchor:{
  default:null,keepOnSplit:false,
  parseHTML:element=>{const value=(element.getAttribute('id')||'').replace(/^section-/,'');return element.id?.startsWith('section-')&&validAnchor(value)?value:null;},
  renderHTML:attrs=>validAnchor(attrs.anchor)?{id:'section-'+attrs.anchor}:{},
 }}}];},
 addProseMirrorPlugins(){return[new Plugin({props:{transformPasted:stripPastedAnchors}})];},
});

export function mountAnchors(form,editor,report) {
 const panel=form.querySelector('[data-anchor-panel]');
 if(!panel)return()=>{};
 const name=panel.querySelector('[data-anchor-name]'),link=panel.querySelector('[data-anchor-link]');
 let target=null;
 const updateLink=()=>{link.value=validAnchor(name.value)?'#section-'+name.value:'';};
 const block=()=>{
  const {$from,$to}=editor.state.selection;
  if($from.parent!==$to.parent||!['paragraph','heading'].includes($from.parent.type.name))return null;
  return{node:$from.parent,pos:$from.before()};
 };
 const apply=value=>{
  const current=block();
  if(!current||current.pos!==target?.pos||current.node!==target?.node){report(t('Выберите один заголовок или абзац и откройте якорь заново.'));return;}
  let duplicate=false;
  editor.state.doc.descendants((node,pos)=>{if(pos!==current.pos&&node.attrs.anchor===value&&value)duplicate=true;});
  if(duplicate){report(t('Это имя якоря уже используется в тексте.'));return;}
  editor.chain().focus().updateAttributes(current.node.type.name,{anchor:value||null}).run();
  panel.hidden=true;
 };
 name.addEventListener('input',updateLink);
 name.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();panel.querySelector('[data-anchor-apply]').click();}});
 panel.querySelector('[data-anchor-apply]').addEventListener('click',()=>{
  const value=name.value.trim();
  if(!validAnchor(value)){report(t('Имя якоря: латинская буква, затем строчные буквы, цифры или дефисы. До 64 символов.'));return;}
  apply(value);
 });
 panel.querySelector('[data-anchor-remove]').addEventListener('click',()=>apply(''));
 return()=>{
  const current=block();
  if(!current){report(t('Выберите один заголовок или абзац для якоря.'));return;}
  panel.hidden=!panel.hidden;target=current;
  if(!panel.hidden){name.value=current.node.attrs.anchor||'';updateLink();name.focus();}
 };
}
