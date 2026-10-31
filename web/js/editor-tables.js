import {Node} from '@tiptap/core';
import {TextSelection} from '@tiptap/pm/state';
import {TableMap,tableEditing,addRowAfter,deleteRow,addColumnAfter,deleteColumn,deleteTable,toggleHeaderRow,goToNextCell} from '@tiptap/pm/tables';

const roles={table:'table',tableRow:'row',tableCell:'cell',tableHeader:'header_cell'};
const attrs=()=>({colspan:{default:1,parseHTML:el=>Math.max(1,Math.min(20,Number(el.getAttribute('colspan'))||1))},rowspan:{default:1,parseHTML:el=>Math.max(1,Math.min(100,Number(el.getAttribute('rowspan'))||1))},colwidth:{default:null,parseHTML:()=>null,renderHTML:()=>({})}});
export function validTableSizes(doc){
 let valid=true;
 doc.descendants(node=>{if(node.type.name==='table'){
  if(node.childCount>100||[...Array(node.childCount)].some((_,i)=>node.child(i).childCount>20)||TableMap.get(node).width>20)valid=false;
 }});
 return valid;
}
function tableKey(editor,direction){return goToNextCell(direction)(editor.state,tr=>{if(validTableSizes(tr.doc))editor.view.dispatch(tr);});}
const cell=(name,tag)=>Node.create({name,content:'paragraph+',isolating:true,addAttributes:attrs,parseHTML:()=>[{tag}],renderHTML:({HTMLAttributes})=>[tag,HTMLAttributes,0]});
export const tableNodes=[
 Node.create({name:'table',group:'block',content:'tableRow+',isolating:true,
  extendNodeSchema(extension){return roles[extension.name]?{tableRole:roles[extension.name]}:{};},
  parseHTML:()=>[{tag:'table'}],renderHTML:()=>['table',{},['tbody',0]],
  addProseMirrorPlugins:()=>[tableEditing()],
  addKeyboardShortcuts(){return {Tab:()=>tableKey(this.editor,1),'Shift-Tab':()=>tableKey(this.editor,-1)};},
 }),
 Node.create({name:'tableRow',content:'(tableCell|tableHeader)+',parseHTML:()=>[{tag:'tr'}],renderHTML:()=>['tr',0]}),
 cell('tableCell','td'),cell('tableHeader','th'),
];
export function tableCommands(editor,report){
 const run=command=>()=>command(editor.state,tr=>{
  if(validTableSizes(tr.doc)){editor.view.dispatch(tr);editor.view.focus();}else report('Таблица: не более 100 строк и 20 столбцов.');
 });
 return {
  insertTable:()=>editor.chain().focus().insertContent([{type:'table',content:Array.from({length:3},(_,row)=>({type:'tableRow',content:Array.from({length:3},()=>({type:row===0?'tableHeader':'tableCell',content:[{type:'paragraph'}]}))}))},{type:'paragraph'}]).command(({tr})=>{
   let position=null;tr.doc.descendants((node,pos)=>{if(node.type.name==='table'&&pos<tr.selection.from)position=pos;});
   if(position!==null)tr.setSelection(TextSelection.near(tr.doc.resolve(position+3)));return true;
  }).run(),
  addTableRow:run(addRowAfter),deleteTableRow:run(deleteRow),addTableColumn:run(addColumnAfter),deleteTableColumn:run(deleteColumn),deleteTable:run(deleteTable),tableHeader:run(toggleHeaderRow),
 };
}
