import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mediaDrop,mediaChoose} from './media-upload.js';

test('media drop uploads one valid image and rejects multiple, oversized, unsupported or busy drops',t=>{
 const previous=globalThis.document;
 globalThis.document={getElementById:()=>null};
 t.after(()=>{globalThis.document=previous;});
 const classes=new Set();let submitted=0,prevented=0;
 const input={};
 const form={classList:{add:v=>classes.add(v),remove:v=>classes.delete(v),contains:v=>classes.has(v)},contains:v=>v===input,querySelector:()=>input,requestSubmit:()=>submitted++};
 const event={target:{closest:()=>form},type:'dragover',dataTransfer:{types:['Files'],files:[{type:'image/png',size:123}]},preventDefault:()=>prevented++};
 mediaDrop(event);assert.equal(classes.has('is-dragging'),true);assert.equal(event.dataTransfer.dropEffect,'copy');
 event.type='dragleave';event.relatedTarget=input;mediaDrop(event);assert.equal(classes.has('is-dragging'),true);
 event.relatedTarget=null;mediaDrop(event);assert.equal(classes.has('is-dragging'),false);
 event.type='drop';mediaDrop(event);assert.equal(submitted,1);assert.equal(input.files,event.dataTransfer.files);
 for(const files of [[{type:'text/html',size:1}],[{type:'image/jpeg',size:8*1024*1024+1}],[{type:'image/png',size:1},{type:'image/png',size:1}]]){
  event.dataTransfer.files=files;mediaDrop(event);assert.equal(submitted,1);
 }
 classes.add('htmx-request');event.dataTransfer.files=[{type:'image/jpeg',size:1}];mediaDrop(event);assert.equal(submitted,1);
 const before=prevented;event.dataTransfer.types=['text/plain'];mediaDrop(event);assert.equal(prevented,before);
});

test('media chooser opens the picker and submits a selection, but ignores cancellation and pending uploads',()=>{
 let picked=0,submitted=0,busy=false;
 const input={click:()=>picked++,id:'library-file',files:[]};
 const form={classList:{contains:()=>busy},querySelector:()=>input,requestSubmit:()=>submitted++};
 input.form=form;
 const button={form};
 mediaChoose({type:'click',target:{closest:()=>button}});assert.equal(picked,1);
 mediaChoose({type:'change',target:input});assert.equal(submitted,0);
 input.files=[{}];mediaChoose({type:'change',target:input});assert.equal(submitted,1);
 busy=true;mediaChoose({type:'click',target:{closest:()=>button}});mediaChoose({type:'change',target:input});
 assert.equal(picked,1);assert.equal(submitted,1);
});
