import test from 'node:test';
import assert from 'node:assert/strict';
import {editFAQ} from './faq.js';

test('FAQ moves whole question and answer rows and preserves one question',()=>{
 const rows=[];let changes=0;
 const list={children:rows,insertBefore(a,b){rows.splice(rows.indexOf(a),1);rows.splice(rows.indexOf(b),0,a);}};
 const form={querySelector(){return list;},dispatchEvent(){changes++;}};
 const row=name=>({name,get previousElementSibling(){return rows[rows.indexOf(this)-1];},get nextElementSibling(){return rows[rows.indexOf(this)+1];},remove(){rows.splice(rows.indexOf(this),1);},querySelector(){return {focus(){}};}});
 const a=row('question + answer A'),b=row('question + answer B');rows.push(a,b);
 const click=(action,item)=>editFAQ({target:{closest(selector){if(selector==='[data-faq-code]')return null;return {closest(selector){return selector==='[data-faq-editor]'?form:item;},hasAttribute(name){return name===action;}};}}});
 click('data-faq-down',a);assert.deepEqual(rows,[b,a]);
 click('data-faq-up',a);assert.deepEqual(rows,[a,b]);
 click('data-faq-remove',b);assert.deepEqual(rows,[a]);
 click('data-faq-remove',a);assert.deepEqual(rows,[a]);assert.equal(changes,3);
});
test('FAQ code is selected for copying',()=>{let selected=false;editFAQ({target:{closest(selector){return selector==='[data-faq-code]'?{select(){selected=true;}}:null;}}});assert.equal(selected,true);});

test('new collection rows mount local media controls and mark the editor dirty',t=>{
 const old=globalThis.htmx;t.after(()=>{globalThis.htmx=old;});let mounted=0,focused=0;
 globalThis.htmx={process:()=>mounted++};
 const row={querySelector:()=>({focus:()=>focused++})};
 const list={children:[],append(){this.children.push(row);},lastElementChild:row};
 const form={dataset:{},querySelector:s=>s==='[data-faq-items]'?list:{content:{cloneNode:()=>({})}},dispatchEvent(){}};
 const action={closest:s=>s==='[data-faq-editor]'?form:null,hasAttribute:s=>s==='data-faq-add'};
 editFAQ({target:{closest:s=>s==='[data-faq-code]'?null:action}});
 assert.equal(mounted,1);assert.equal(focused,1);assert.equal(form.dataset.formDirty,'true');
});
