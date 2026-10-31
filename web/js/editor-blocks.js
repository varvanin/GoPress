import Paragraph from '@tiptap/extension-paragraph';
import {t} from './i18n.js';

import {parseBlock} from './editor-shortcodes.js';
export {parseBlock} from './editor-shortcodes.js';
export function blockSettingsURL(block){
 if(block.kind==='form')return '/admin/forms/'+block.id+'/edit';
 if(block.kind==='module')return '/admin/modules/package/'+block.id+'/edit';
 return '/admin/modules/'+block.kind+'/'+block.id+'/edit';
}
export function blockParagraph(form){
 const labels=new Map();
 try{for(const b of JSON.parse(form.querySelector('[data-editor-blocks]')?.textContent||'[]'))labels.set(b.Kind+':'+b.ID,b.Name);}catch{}
 return Paragraph.extend({addNodeView(){return ({node,getPos,editor})=>{
  let current=node;
  const block=node.childCount===1&&node.firstChild.isText&&!node.firstChild.marks.length?parseBlock(node.textContent):null;
  const template=form.querySelector('[data-block-card]');
  const dom=block&&template?template.content.firstElementChild.cloneNode(true):document.createElement('p');
  const card=!!block&&!!template;
  const attrs=n=>{if(n.attrs.anchor)dom.id='section-'+n.attrs.anchor;else dom.removeAttribute('id');if(!card)dom.className=n.attrs.textAlign?'align-'+n.attrs.textAlign:'';};
  attrs(node);
  if(card){
   dom.querySelector('[data-block-name]').textContent=(labels.get(block.kind+':'+block.id)||t('Блок')+' '+block.id)+' · '+({faq:'FAQ',form:t('Форма'),gallery:t('Галерея'),testimonials:t('Отзывы'),module:t('Модуль')}[block.kind]);
   const settings=dom.querySelector('[data-block-settings]');settings.href=blockSettingsURL(block);
   if(form.dataset.role!=='admin')settings.hidden=true;
   dom.querySelector('[data-block-remove]').addEventListener('click',()=>{
    const pos=getPos();if(!editor.isEditable||typeof pos!=='number'||!editor.state.doc.nodeAt(pos)?.eq(current))return;
    editor.chain().focus().deleteRange({from:pos,to:pos+current.nodeSize}).run();
   });
  }
  return {dom,...(!card?{contentDOM:dom}:{}),update(next){
   const nextBlock=next.childCount===1&&next.firstChild.isText&&!next.firstChild.marks.length?parseBlock(next.textContent):null;
   if(next.type!==current.type||!!nextBlock!==!!block||(card&&next.textContent!==current.textContent))return false;
   current=next;attrs(next);return true;
  },stopEvent:event=>card&&!!event.target.closest('button,a'),ignoreMutation:()=>card};
 };}});
}

export function mountBlockPicker(form,editor,report){
 const panel=form.querySelector('[data-block-panel]');
 form.querySelector('[data-insert-block]')?.addEventListener('click',()=>{
  const code=form.elements.content_block?.value;
  if(!editor.isEditable||!parseBlock(code||'')){report(t('Выберите блок.'));return;}
  const end=editor.state.selection.$to;
  const position=end.depth?end.after(1):editor.state.doc.content.size;
  editor.chain().focus().insertContentAt(position,[{type:'paragraph',content:[{type:'text',text:code}]},{type:'paragraph'}]).run();
  panel.hidden=true;
 });
 return ()=>{if(panel){panel.hidden=!panel.hidden;}};
}
