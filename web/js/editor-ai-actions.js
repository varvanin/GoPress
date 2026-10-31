import {parseBlock} from './editor-shortcodes.js';

export function translationParts(document){
 const doc=structuredClone(document),slots=[];
 const visit=node=>{
  if(node.type==='codeBlock'||node.type==='paragraph'&&node.content?.length===1&&parseBlock(node.content[0].text||''))return;
  if(node.type==='text'&&typeof node.text==='string'&&!node.marks?.some(mark=>mark.type==='code'))slots.push([node,'text']);
  if(node.type==='image')for(const key of ['alt','caption'])if(node.attrs?.[key])slots.push([node.attrs,key]);
  for(const child of node.content||[])visit(child);
 };
 visit(doc);return {doc,texts:slots.map(([object,key])=>object[key]),apply(texts){if(texts.length!==slots.length||texts.some(text=>typeof text!=='string'))throw new Error('Invalid translation');if(slots.some(([_,key],i)=>key!=='text'&&new TextEncoder().encode(texts[i]).length>2000))throw new Error('Invalid image translation');slots.forEach(([object,key],i)=>object[key]=texts[i]);return doc;}};
}
export function parseAIFields(mode,text,count){
 const value=JSON.parse(text);
 if(!value||typeof value.title!=='string'||!value.title.trim()||[...value.title].length>200)throw new Error('Invalid AI title');
 if(mode==='seo'){
 if(typeof value.description!=='string'||!value.description.trim()||[...value.description].length>500)throw new Error('Invalid AI description');
 }else if(!Array.isArray(value.texts)||value.texts.length!==count||value.texts.some(text=>typeof text!=='string'))throw new Error('Invalid AI translation');
 return value;
}
export function aiParagraphs(text){return text.replace(/\r\n?/g,'\n').split(/\n\s*\n/).filter(text=>text.trim()).map(text=>({type:'paragraph',content:[{type:'text',text:text.trim()}]}));}
