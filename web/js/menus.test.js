import {test} from 'node:test';
import assert from 'node:assert/strict';
import {editMenu,moveMenuBranch,updateMenuRow} from './menus.js';
import {dirtyForms} from './admin-forms.js';

test('moving a parent keeps its descendants together and does not cross parent boundaries',()=>{
 const rows=[{id:'A',depth:0},{id:'child',depth:1},{id:'grandchild',depth:2},{id:'B',depth:0},{id:'B-child',depth:1},{id:'C',depth:0}];
 const moved=moveMenuBranch(rows,0,'down');
 assert.deepEqual(moved.map(item=>item.id),['B','B-child','A','child','grandchild','C']);assert.equal(moved[2],rows[0]);
 assert.deepEqual(moveMenuBranch(moved,2,'up'),rows);
 assert.equal(moveMenuBranch(rows,1,'up'),rows);assert.equal(moveMenuBranch(rows,1,'down'),rows);
});
function fixture(){
 const handlers={};const popup={querySelector:()=>({}),addEventListener:(type,fn)=>handlers[type]=fn,removeEventListener:type=>delete handlers[type]};
 globalThis.document={getElementById:()=>popup};
 globalThis.window={templ:{dialog:{open(){handlers.click({target:{closest:()=>({dataset:{confirmResult:'accept'}})}});},close(){handlers['dialog-close']();}},select:{setValue(trigger,value){trigger.field.value=value;}}}};
 const rows=[],status={textContent:''},empty={hidden:true},add={focus(){}};
 const create=(label,depth=0)=>{
  const fields={label:{value:label,focus(){}},url:{value:'/page'},depth:{value:String(depth)}};
  const heading={textContent:''},summary={focus(){}},details={open:true};
  const row={get isConnected(){return rows.includes(this);},querySelectorAll:()=>[],dataset:{depth:String(depth)},fields,get previousElementSibling(){return rows[rows.indexOf(this)-1];},get nextElementSibling(){return rows[rows.indexOf(this)+1];},remove(){rows.splice(rows.indexOf(this),1);},querySelector(selector){return {'[name="label"]':fields.label,'[name="url"]':fields.url,'[name="depth"]':fields.depth,'[data-slot="select-trigger"]':{field:fields.depth},'[data-menu-label]':heading,input:fields.label,details,summary}[selector];}};
  return row;
 };
 const items={get children(){return rows;},get lastElementChild(){return rows.at(-1);},append(row){rows.push(row);},replaceChildren(...next){rows.splice(0,rows.length,...next);}};
 const form={dataset:{},querySelector:selector=>({'[data-menu-items]':items,'[data-menu-status]':status,'[data-menu-empty]':empty,'[data-menu-add]':add,'template[data-menu-template]':{content:{cloneNode:()=>create('')}},template:{content:{cloneNode:()=>{throw new Error('The select template is not a menu row');}}},'[data-menu-page]':{querySelector:selector=>selector.includes('select-value')?{textContent:'About'}:{value:'/about'}}})[selector]};
 const click=(row,action)=>editMenu({target:{closest:()=>({closest:selector=>selector==='[data-menu-form]'?form:row,hasAttribute:name=>name===action,focus(){}})}});
 return {rows,create,form,status,empty,click};
}
test('reordering and deleting a parent preserve children and update their level',async t=>{
 t.after(()=>{delete globalThis.window;delete globalThis.document;});
 const f=fixture();f.rows.push(f.create('Home'),f.create('About'),f.create('Child',1),f.create('Contact'));
 const original=f.rows[1];f.click(original,'data-menu-up');assert.deepEqual(f.rows.map(row=>row.fields.label.value),['About','Child','Home','Contact']);
 f.click(original,'data-menu-down');assert.equal(f.rows[1],original);
 await f.click(original,'data-menu-remove');assert.deepEqual(f.rows.map(row=>row.fields.label.value),['Home','Child','Contact']);assert.equal(f.rows[1].fields.depth.value,'0');assert.equal(f.form.dataset.formDirty,'true');
});
test('adding a published page fills the link, updates its summary and clears the empty state',async t=>{
 t.after(()=>{delete globalThis.window;delete globalThis.document;});
 const f=fixture();f.empty.hidden=false;f.click(null,'data-menu-add-page');assert.equal(f.rows.length,1);
 assert.equal(f.rows[0].fields.label.value,'About');assert.equal(f.rows[0].fields.url.value,'/about');assert.equal(f.rows[0].querySelector('[data-menu-label]').textContent,'About');assert.equal(f.empty.hidden,true);
 f.rows[0].fields.label.value='Company';updateMenuRow({target:{closest:()=>f.rows[0]}});assert.equal(f.rows[0].querySelector('[data-menu-label]').textContent,'Company');
 await f.click(f.rows[0],'data-menu-remove');assert.equal(f.empty.hidden,false);
});
test('one click adds exactly one editable custom link using the dedicated row template',async t=>{
 t.after(()=>{delete globalThis.window;delete globalThis.document;});
 const f=fixture();
 await f.click(null,'data-menu-add');assert.equal(f.rows.length,1);
 assert.equal(f.form.dataset.formDirty,'true');assert.equal(f.empty.hidden,true);
 await f.click(null,'data-menu-add');assert.equal(f.rows.length,2);
 assert.notEqual(f.rows[0],f.rows[1]);
});
test('navigation warns about other unsaved menus while allowing the submitted menu',t=>{
 const header={},footer={};globalThis.document={querySelectorAll:()=>[{contains:element=>element===header},{contains:element=>element===footer}]};t.after(()=>delete globalThis.document);
 assert.equal(dirtyForms(),true);assert.equal(dirtyForms(header),true);
 globalThis.document.querySelectorAll=()=>[{contains:element=>element===header}];assert.equal(dirtyForms(header),false);assert.equal(dirtyForms(footer),true);
});
