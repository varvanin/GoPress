import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createAutosave} from './autosave.js';
import {mountEditorLanguages} from './editor-languages.js';

function setup(language='original') {
 const base={title:'Base',document:'{"type":"doc","content":[{"type":"paragraph"}]}',seo:{description:'Base SEO'}};
 const translation={title:'English',document:'{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"English text"}]}]}',seo:{description:'English SEO',noindex:true}};
 const selector=Object.assign(new EventTarget(),{value:language});
 const fields={title:{name:'title',value:base.title},document:{value:base.document},content_language:selector,translations:{value:JSON.stringify({en:translation})},original_translation:{value:JSON.stringify(base)},seo_description:{name:'seo_description',value:base.seo.description},noindex:{name:'noindex',type:'checkbox',checked:false}};
 const form=Object.assign(new EventTarget(),{elements:fields,querySelector:()=>null});
 let doc=JSON.parse(base.document),resets=0;
 let reads=0;
 const editor={getJSON:()=>{reads++;return doc;},commands:{setContent(value,options){assert.equal(options.emitUpdate,false);doc=structuredClone(value);for(const node of doc.content)if(node.type==='paragraph')node.attrs={textAlign:null};}},state:{constructor:{create:()=>({})}},view:{updateState(){resets++;}}};
 mountEditorLanguages(form,editor);
 const select=language=>{selector.value=language;const event=new Event('change');Object.defineProperty(event,'target',{value:selector});form.dispatchEvent(event);};
 const input=(field,value)=>{fields[field].value=value;const event=new Event('input');Object.defineProperty(event,'target',{value:fields[field]});form.dispatchEvent(event);};
 return {fields,form,select,input,editor,get resets(){return resets;},get reads(){return reads;}};
}

test('switching languages preserves text and SEO within one material and resets undo',()=>{
 const s=setup();
 s.input('title','Updated base');
 s.select('en');
 assert.equal(s.fields.title.value,'English');
 assert.equal(s.fields.seo_description.value,'English SEO');
 assert.equal(s.fields.noindex.checked,true);
 s.input('title','Updated English');
 s.input('seo_description','Updated English SEO');
 s.select('original');
 assert.equal(s.fields.title.value,'Updated base');
 assert.equal(s.fields.seo_description.value,'Base SEO');
 assert.equal(s.fields.noindex.checked,false);
 assert.equal(s.fields.original_translation.value,'');
 assert.equal(JSON.parse(s.fields.translations.value).en.title,'Updated English');
 s.select('en');
 assert.equal(s.fields.title.value,'Updated English');
 assert.equal(JSON.parse(s.fields.original_translation.value).title,'Updated base');
 assert.equal(JSON.parse(s.fields.translations.value).en,undefined);
 assert.equal(s.fields.seo_description.value,'Updated English SEO');
 assert.equal(s.resets,3);
});

test('opening a saved language restores it and an untouched empty language creates no translation',()=>{
 const s=setup('en');
 assert.equal(s.fields.title.value,'English');
 s.select('pl');s.select('original');
 assert.equal(JSON.parse(s.fields.translations.value).pl,undefined);
 assert.equal(s.fields.title.value,'Base');
});


test('typing does not reserialize all language documents',()=>{
 const s=setup();
 const before=s.fields.translations.value;
 for(let i=0;i<100;i++)s.input('title','Title '+i);
 assert.equal(s.reads,0);
 assert.equal(s.fields.translations.value,before);
 s.form.dispatchEvent(new Event('editor:serialize'));
 assert.equal(s.fields.original_translation.value,'');
});

async function firstSave(t,s,change) {
 const previous={FormData:globalThis.FormData,fetch:globalThis.fetch,window:globalThis.window};
 globalThis.FormData=class extends Map{constructor(form){super(Object.entries(form.elements).map(([name,field])=>[name,field.value]));}};
 globalThis.window={history:{state:{},replaceState(){}}};
 Object.assign(s.fields,{id:{value:'0'},version:{value:'0'},slug:{value:'base'},csrf:{value:'csrf'}});
 Object.assign(s.form,{dataset:{},action:'/save',closest:()=>null,querySelectorAll:()=>[]});
 let resolve;
 globalThis.fetch=()=>new Promise(done=>{resolve=done;});
 const controller=new AbortController();
 t.after(()=>{controller.abort();Object.assign(globalThis,previous);});
 const state={editor:s.editor,dirty:true,pending:false,uploading:false};
 const saver=createAutosave(s.form,state,controller.signal,()=>{});
 const flight=saver.flush();
 change();saver.schedule();
 resolve({ok:true,json:async()=>({id:1,version:1,title:'Generated',slug:'base',edit_url:'/edit'})});
 assert.equal(await flight,true);
 assert.equal(state.dirty,true);
}

test('first autosave cannot refill an original title cleared during the request',async t=>{
 const s=setup();
 await firstSave(t,s,()=>s.input('title',''));
 assert.equal(s.fields.title.value,'');
});

test('a generated original title never fills an empty translated title',async t=>{
 const s=setup();s.input('title','');s.select('en');s.input('title','');
 await firstSave(t,s,()=>{});
 assert.equal(s.fields.title.value,'');
 assert.equal(JSON.parse(s.fields.original_translation.value).title,'Generated');
});
