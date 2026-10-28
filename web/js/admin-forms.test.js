import {test} from 'node:test';
import assert from 'node:assert/strict';
import {previewTheme} from './admin-forms.js';

test('theme preview submits unsaved fields privately in a new tab without changing the form',t=>{
 const original={document:globalThis.document,FormData:globalThis.FormData};
 t.after(()=>{globalThis.document=original.document;globalThis.FormData=original.FormData;});
 const fields=[['csrf','token'],['version','3'],['theme','minimal'],['home_title','PRIVATE PREVIEW']];
 const form={valid:false,dataset:{formDirty:'true'},reportValidity(){return this.valid;}};
 let preview;
 globalThis.FormData=class{constructor(value){assert.equal(value,form);}*[Symbol.iterator](){yield* fields;}};
 globalThis.document={body:{append(value){preview=value;}},createElement(tag){return tag==='input'?{}:{inputs:[],setAttribute(name,value){this[name]=value;},append(input){this.inputs.push(input);},submit(){this.submitted=true;},remove(){this.removed=true;}};}};
 const event={target:{closest:()=>({form,closest:()=>{throw new Error('Associated form must be used');}})}};
 previewTheme(event);assert.equal(preview,undefined);
 form.valid=true;previewTheme(event);
 assert.equal(preview.method,'post');assert.equal(preview.action,'/admin/themes/preview');assert.equal(preview.target,'_blank');assert.equal(preview.hidden,true);assert.equal(preview.submitted,true);assert.equal(preview.removed,true);
 assert.deepEqual(preview.inputs.map(({name,value})=>[name,value]),fields);
 assert.equal(form.dataset.formDirty,'true');
});

test('saving settings clears its dirty flag and associated buttons belong to their form',async t=>{
 const {dirtyForms,settingsSaved}=await import('./admin-forms.js');
 const form={dataset:{formDirty:'true'},contains:()=>false};
 globalThis.document={querySelectorAll:()=>form.dataset.formDirty==='true'?[form]:[]};t.after(()=>delete globalThis.document);
 assert.equal(dirtyForms(),true);assert.equal(dirtyForms({form}),false);
 settingsSaved({target:{form}});assert.equal(form.dataset.formDirty,'false');assert.equal(dirtyForms(),false);
});

test('pending forms freeze associated controls and restore original states after overlapping requests',async()=>{
 const {freezeForms}=await import('./admin-forms.js');
 const input={disabled:false,value:'Original'},locked={disabled:true},externalSave={disabled:false};
 const root={querySelectorAll:()=>[{elements:[input,locked,externalSave]}]};
 const finish=freezeForms(root),finishSecond=freezeForms(root);
 assert.equal(input.disabled,true);assert.equal(externalSave.disabled,true);
 finish();assert.equal(input.disabled,true);
 finishSecond();assert.equal(input.disabled,false);assert.equal(locked.disabled,true);assert.equal(externalSave.disabled,false);assert.equal(input.value,'Original');
});
