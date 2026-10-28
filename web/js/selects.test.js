import {test} from 'node:test';
import assert from 'node:assert/strict';
import {rekeySelects} from './selects.js';

test('cloned selects receive independent popup, trigger and list ids before mounting',()=>{
 function copy(){
  const attrs=new Map([['data-templ-controls','original'],['aria-controls','original-list'],['aria-activedescendant','old-option']]);
  const trigger={id:'original-trigger',setAttribute:(name,value)=>attrs.set(name,value),removeAttribute:name=>attrs.delete(name)};
  const list={id:'original-list'},input={id:'original-hidden-input',value:'2',name:'depth'};
  const popup={id:'original',querySelector:()=>list};
  const select={querySelector:selector=>({'[data-slot="select-trigger"]':trigger,'[data-slot="select-content"]':popup,'input[aria-hidden="true"]':input})[selector]};
  return {root:{querySelectorAll:()=>[select]},attrs,trigger,list,popup,input};
 }
 const a=copy(),b=copy();rekeySelects(a.root);rekeySelects(b.root);
 assert.notEqual(a.popup.id,b.popup.id);
 for(const item of [a,b]){
  assert.equal(item.attrs.get('data-templ-controls'),item.popup.id);assert.equal(item.list.id,item.popup.id+'-list');
  assert.equal(item.trigger.id,item.popup.id+'-trigger');assert.equal(item.attrs.has('aria-controls'),false);assert.equal(item.attrs.has('aria-activedescendant'),false);
  assert.equal(item.input.value,'2');assert.equal(item.input.name,'depth');
 }
});
