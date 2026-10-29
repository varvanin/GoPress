import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createAutosave} from './autosave.js';

function setup(t,id='5'){
 const original={FormData:globalThis.FormData,fetch:globalThis.fetch,window:globalThis.window};
 globalThis.FormData=class extends Map{constructor(form){super(Object.entries(form.elements).map(([key,input])=>[key,input.value]));}};
 const version={value:'1'},scheduleVersion={value:'1'};
 const preview={hidden:true},controls={hidden:true},scheduleForm={action:''};
 const root={querySelectorAll:()=>[version,scheduleVersion],querySelector:selector=>({'[data-saved-preview]':preview,'[data-schedule-controls]':controls,'#schedule-form':scheduleForm})[selector]||null};
 const form=Object.assign(new EventTarget(),{action:'http://example.com/admin/pages/save',dataset:{},elements:{id:{value:id},version,title:{value:'Title'},slug:{value:'title'},document:{value:'original'},csrf:{value:'token'}},closest:()=>root});
 const urls=[];globalThis.window={history:{state:{htmx:true,gpIndex:2},replaceState(state,title,url){urls.push(url);}}};
 const state={dirty:true,editor:{},pending:false,uploading:false};
 const controller=new AbortController();const messages=[];
 const saver=createAutosave(form,state,controller.signal,message=>messages.push(message));
 t.after(()=>{controller.abort();Object.assign(globalThis,original);});
 return{form,state,controller,saver,messages,urls,scheduleVersion,preview,controls,scheduleForm};
}
const ok=(version=2)=>({ok:true,json:async()=>({id:5,version,title:'Title',slug:'title',edit_url:'/admin/pages/5/edit'})});

test('autosave debounces edits and updates hidden versions without replacing the editor',async(t)=>{
 t.mock.timers.enable({apis:['setTimeout']});
 const s=setup(t);let calls=0;
 globalThis.fetch=async(url,options)=>{calls++;assert.equal(options.headers['X-CSRF-Token'],'token');assert.equal(options.body.get('intent'),'autosave');return ok();};
 s.saver.schedule();s.saver.schedule();
 t.mock.timers.tick(1499);assert.equal(calls,0);
 t.mock.timers.tick(1);await s.saver.pending;
 assert.equal(calls,1);assert.equal(s.state.dirty,false);
 assert.equal(s.form.elements.version.value,'2');assert.equal(s.scheduleVersion.value,'2');
 assert.equal(s.form.elements.document.value,'original');
});

test('typing during an in-flight save keeps the newer text and serializes the next save',async(t)=>{
 const s=setup(t);let resolveFirst;let calls=0;const documents=[];
 globalThis.fetch=(url,options)=>{
  calls++;documents.push(options.body.get('document'));
  return calls===1?new Promise(resolve=>{resolveFirst=resolve;}):Promise.resolve(ok(3));
 };
 const first=s.saver.flush();
 s.form.elements.document.value='newer local text';s.saver.schedule();
 const overlap=s.saver.flush();assert.equal(calls,1);
 resolveFirst(ok());await first;await overlap;
 assert.equal(s.state.dirty,true);assert.equal(s.form.elements.document.value,'newer local text');
 await s.saver.flush();
 assert.deepEqual(documents,['original','newer local text']);
 assert.equal(s.form.elements.version.value,'3');assert.equal(s.state.dirty,false);
});

test('a conflict stops automatic retries and leaves the local document intact',async(t)=>{
 const s=setup(t);let calls=0;
 globalThis.fetch=async()=>{calls++;return{ok:false,status:409,json:async()=>({message:'conflict'})};};
 assert.equal(await s.saver.flush(),false);
 s.saver.schedule();assert.equal(await s.saver.flush(),false);
 assert.equal(calls,1);assert.equal(s.state.dirty,true);assert.equal(s.saver.failed,true);
 assert.equal(s.form.elements.document.value,'original');
 assert.equal(s.form.elements.version.value,'1');
});

test('the first autosave creates a draft address and enables preview and scheduling',async(t)=>{
 const s=setup(t,'0');
 globalThis.fetch=async()=>ok();
 await s.saver.flush();
 assert.equal(s.form.elements.id.value,'5');
 assert.deepEqual(s.urls,['/admin/pages/5/edit']);
 assert.equal(s.preview.hidden,false);assert.equal(s.controls.hidden,false);
 assert.equal(s.scheduleForm.action,'/admin/pages/5/schedule');
});

test('leaving the screen ignores a late autosave response',async(t)=>{
 const s=setup(t);let resolve;
 globalThis.fetch=()=>new Promise(done=>{resolve=done;});
 const pending=s.saver.flush();s.controller.abort();resolve(ok());
 assert.equal(await pending,false);
 assert.equal(s.form.elements.version.value,'1');
 assert.equal(s.state.dirty,true);
});


test('a late response cannot refill a title or slug cleared while saving',async(t)=>{
 const s=setup(t);let resolve;
 globalThis.fetch=()=>new Promise(done=>{resolve=done;});
 const pending=s.saver.flush();
 s.form.elements.title.value='';s.form.elements.slug.value='';s.saver.schedule();
 resolve(ok());await pending;
 assert.equal(s.form.elements.title.value,'');
 assert.equal(s.form.elements.slug.value,'');
 assert.equal(s.state.dirty,true);
});

test('server-generated initial draft fields fill only fields that were already empty',async(t)=>{
 const s=setup(t,'0');s.form.elements.title.value='';s.form.elements.slug.value='';
 globalThis.fetch=async()=>ok();
 await s.saver.flush();
 assert.equal(s.form.elements.title.value,'Title');
 assert.equal(s.form.elements.slug.value,'title');
});

test('autosave preserves the selected translation in preview and the first draft URL',async(t)=>{
 const s=setup(t,'0');s.form.elements.content_language={value:'en'};
 globalThis.fetch=async()=>ok();
 assert.equal(await s.saver.flush(),true);
 assert.equal(s.preview.href,'/admin/pages/5/preview?lang=en');
 assert.equal(s.urls[0],'/admin/pages/5/edit?lang=en');
});
