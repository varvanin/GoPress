import {test} from 'node:test';
import assert from 'node:assert/strict';
import {selectContent} from './admin-forms.js';

test('select all, partial selection and clearing keep bulk controls in sync',t=>{
 const all={checked:true,indeterminate:false};
 const boxes=[{checked:false,matches:()=>true},{checked:false,matches:()=>true}];
 const count={textContent:''},action={disabled:true};
 globalThis.document={querySelector:selector=>selector==='[data-select-all]'?all:count,querySelectorAll:selector=>selector==='[data-bulk-action]'?[action]:boxes};
 t.after(()=>delete globalThis.document);
 selectContent({target:all});assert.ok(boxes.every(box=>box.checked));assert.equal(action.disabled,false);assert.equal(count.textContent,'Выбрано: 2');
 boxes[0].checked=false;selectContent({target:boxes[0]});assert.equal(all.checked,false);assert.equal(all.indeterminate,true);
 boxes[1].checked=false;selectContent({target:boxes[1]});assert.equal(action.disabled,true);assert.equal(all.indeterminate,false);
});
