import {test} from 'node:test';
import assert from 'node:assert/strict';
import {editFormBuilder} from './form-builder.js';

function fixture(){
 const rows=[],status={textContent:''};
 const row=label=>({label,remove(){rows.splice(rows.indexOf(this),1);},querySelector:()=>({focus(){}}),get previousElementSibling(){return rows[rows.indexOf(this)-1];},get nextElementSibling(){return rows[rows.indexOf(this)+1];}});
 const fields={get children(){return rows;},get lastElementChild(){return rows.at(-1);},append(fragment){rows.push(fragment.row);},insertBefore(child,before){rows.splice(rows.indexOf(child),1);rows.splice(rows.indexOf(before),0,child);}};
 const form={dataset:{},querySelector:selector=>selector==='[data-form-fields]'?fields:selector==='[data-form-builder-status]'?status:{content:{cloneNode:()=>({row:row('New'),querySelectorAll:()=>[]})}}};
 const click=(action,item)=>editFormBuilder({target:{closest:selector=>selector==='[data-copy-form]'?null:{closest:selector=>selector==='[data-form-builder]'?form:item,hasAttribute:name=>name===action}}});
 rows.push(row('Name'),row('Email'),row('Message'));
 return {rows,form,status,click};
}
test('builder adds independent rows, moves fields and keeps one field at minimum',async t=>{
 globalThis.document={querySelector:()=>null};t.after(()=>delete globalThis.document);
 const f=fixture();await f.click('data-add-form-field');assert.equal(f.rows.length,4);assert.equal(f.form.dataset.formDirty,'true');
 const added=f.rows[3];await f.click('data-form-field-up',added);assert.equal(f.rows[2],added);
 await f.click('data-form-field-down',added);assert.equal(f.rows[3],added);
 await f.click('data-remove-form-field',added);assert.deepEqual(f.rows.map(row=>row.label),['Name','Email','Message']);
 await f.click('data-remove-form-field',f.rows[0]);await f.click('data-remove-form-field',f.rows[0]);await f.click('data-remove-form-field',f.rows[0]);assert.equal(f.rows.length,1);
});
test('builder caps the number of fields at twenty',async t=>{
 globalThis.document={querySelector:()=>null};t.after(()=>delete globalThis.document);
 const f=fixture();for(let i=0;i<25;i++)await f.click('data-add-form-field');assert.equal(f.rows.length,20);assert.equal(new Set(f.rows).size,20);
});
