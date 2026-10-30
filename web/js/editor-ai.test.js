import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mountEditorAI} from './editor-ai.js';

function fixture() {
 const controller=new AbortController();
 const node=()=>Object.assign(new EventTarget(),{value:'',disabled:false,hidden:false,focus(){}});
 const nodes=Object.fromEntries(['prompt','context','generate','result','output','insert','status'].map(name=>[name,node()]));
 nodes.prompt.value='Write a short introduction';nodes.result.hidden=true;
 const panel={querySelector:selector=>nodes[selector.match(/data-ai-(.*)\]/)[1]]};
 const form={dataset:{},elements:{id:{value:'1'},csrf:{value:'csrf-test'},title:{value:'Title'},content_language:{value:'original'},source_language:{value:'ru'}},querySelector:()=>panel};
 const inserted=[];
 const editor={getText:()=>'<script>existing</script>',state:{doc:{content:{size:42}}},chain(){return{focus(value){assert.equal(value,'end');return this;},insertContentAt(position,content){inserted.push({position,content});return this;},run(){return true;}};}};
 const state={pending:false,uploading:false};
 mountEditorAI(form,editor,state,controller.signal);
 const generate=async()=>{nodes.generate.dispatchEvent(new Event('click'));await new Promise(resolve=>setImmediate(resolve));};
 return{...nodes,controls:nodes,form,inserted,state,controller,generate};
}

test('AI sends only opted-in context and inserts provider text as plain nodes on explicit action',async t=>{
 const original=globalThis.fetch;t.after(()=>{globalThis.fetch=original;});
 let request;
 globalThis.fetch=async(path,options)=>{assert.equal(path,'/admin/pages/ai');assert.equal(options.headers['X-CSRF-Token'],'csrf-test');request=JSON.parse(options.body);return{ok:true,json:async()=>({text:'<script>alert(1)</script>\n\nSecond paragraph'})};};
 const f=fixture();await f.generate();assert.equal(request.text,'');assert.equal(request.language,'ru');assert.equal(f.inserted.length,0);assert.equal(f.result.hidden,false);
 f.context.checked=true;await f.generate();assert.equal(request.text,'<script>existing</script>');
 f.insert.dispatchEvent(new Event('click'));
 assert.deepEqual(f.inserted,[{position:42,content:[{type:'paragraph',content:[{type:'text',text:'<script>alert(1)</script>'}]},{type:'paragraph',content:[{type:'text',text:'Second paragraph'}]}]}]);
 assert.equal(f.output.value,'');assert.equal(f.result.hidden,true);
});
test('AI keeps results out of a different language and preserves text on errors',async t=>{
 const original=globalThis.fetch;t.after(()=>{globalThis.fetch=original;});
 globalThis.fetch=async()=>({ok:true,json:async()=>({text:'Original language text'})});
 const f=fixture();await f.generate();
 f.form.elements.content_language.value='en';f.insert.dispatchEvent(new Event('click'));assert.equal(f.inserted.length,0);assert.equal(f.output.value,'Original language text');
 globalThis.fetch=async()=>({ok:false,json:async()=>({error:'Provider unavailable'})});
 await f.generate();assert.equal(f.output.value,'Original language text');assert.equal(f.status.textContent,'Provider unavailable');assert.equal(f.controls.generate.disabled,false);
 f.state.pending=true;f.form.elements.content_language.value='original';f.insert.dispatchEvent(new Event('click'));assert.equal(f.inserted.length,0);
});
test('leaving the editor aborts AI and ignores late responses',async t=>{
 const original=globalThis.fetch;t.after(()=>{globalThis.fetch=original;});
 let finish,signal;
 globalThis.fetch=async(_,options)=>{signal=options.signal;return new Promise(resolve=>{finish=resolve;});};
 const f=fixture();await f.generate();f.controller.abort();assert.equal(signal.aborted,true);
 finish({ok:true,json:async()=>({text:'Late text'})});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(f.result.hidden,true);assert.equal(f.output.value,'');
});
