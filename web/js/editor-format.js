import {Extension,Mark} from '@tiptap/core';

export const inlineMarks = [['highlight','mark',''],['superscript','sup','subscript'],['subscript','sub','superscript']].map(([name,tag,excludes])=>Mark.create({
  name,
  ...(excludes ? {excludes} : {}),
  parseHTML(){return [{tag}];},
  renderHTML(){return [tag,{},0];},
}));
export const textAlignment=Extension.create({
  name:'textAlignment',
  addGlobalAttributes(){
    return [{types:['paragraph','heading'],attributes:{textAlign:{
      default:null,
      parseHTML:element=>['left','center','right','justify'].find(value=>element.classList.contains('align-'+value)||element.style.textAlign===value)||null,
      renderHTML:attributes=>['left','center','right','justify'].includes(attributes.textAlign)?{class:'align-'+attributes.textAlign}:{},
    }}}];
  },
});

// Search one text block at a time; never allocate an array for every match.
export function findText(doc,query,from=0){
  if(!query)return null;
  const escaped=query.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
  const pattern=new RegExp(escaped,'iu');
  let found=null,first=null;
  doc.descendants((node,pos)=>{
    if(found)return false;
    if(!node.isTextblock)return;
    const text=node.textBetween(0,node.content.size,'\n','\n');
    const initial=pattern.exec(text);
    if(initial&&!first)first={from:pos+1+initial.index,to:pos+1+initial.index+initial[0].length};
    const offset=Math.max(0,from-pos-1);
    const match=pattern.exec(text.slice(offset));
    if(match)found={from:pos+1+offset+match.index,to:pos+1+offset+match.index+match[0].length};
    return false;
  });
  return found||first;
}
