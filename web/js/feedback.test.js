import {test} from 'node:test';
import assert from 'node:assert/strict';
import {deletionPrompt,actionMessage,confirmAction,showNotices,toast} from './feedback.js';

const form=(action,question)=>({querySelector:()=>({value:action}),getAttribute:name=>name==='hx-confirm'?question:'/admin/media/delete'});
const button=(action,f)=>({name:'action',value:action,closest:selector=>selector==='form'?f:selector==='[hx-confirm]'&&f?.getAttribute('hx-confirm')?f:null});
test('only destructive actions ask for confirmation, including mixed save/delete forms',()=>{
 const f=form('save');
 assert.equal(deletionPrompt(button('save',f)),null);
 assert.equal(deletionPrompt(button('delete',f)).label,'Удалить');
 assert.equal(deletionPrompt(button('trash',f)).label,'В корзину');
 assert.equal(deletionPrompt(button('restore',f)),null);
 assert.match(deletionPrompt(button('',form('', 'Удалить файл навсегда?'))).message,/навсегда/);
 for(const path of ['/admin/content/bulk','/admin/pages/12/state']) {
  const contentForm={getAttribute:()=>path};
  assert.match(deletionPrompt(button('delete',contentForm)).message,/Восстановление будет невозможно/);
  assert.equal(deletionPrompt(button('delete',contentForm)).label,'Удалить навсегда');
 }
 assert.equal(actionMessage(null,{action:'trash'}),'Материал перемещён в корзину.');
 assert.equal(actionMessage(null,{action:'restore'}),'Материал восстановлен.');
});
test('confirmation uses the existing dialog, defaults to cancel, ignores duplicate requests and resolves Escape',async t=>{
 const original={document:globalThis.document,window:globalThis.window};
 t.after(()=>Object.assign(globalThis,original));
 const handlers=new Map();const fields={};
 const popup={querySelector:key=>fields[key]??=( {} ),addEventListener:(type,fn)=>handlers.set(type,fn),removeEventListener:type=>handlers.delete(type)};
 let opened=0;
 globalThis.document={getElementById:()=>popup};
 globalThis.window={templ:{dialog:{open:()=>opened++,close:()=>handlers.get('dialog-close')()}}};
 const first=confirmAction('Удалить файл?',{});
 assert.equal(await confirmAction('Second',{}),false);assert.equal(opened,1);
 handlers.get('dialog-close')();assert.equal(await first,false);
 const second=confirmAction('В корзину?',{},'В корзину');
 handlers.get('click')({target:{closest:()=>({dataset:{confirmResult:'accept'}})}});
 assert.equal(await second,true);assert.equal(fields['[data-confirm-result="accept"]'].textContent,'В корзину');
 const third=confirmAction('Delete',{});
 handlers.get('click')({target:{closest:()=>({dataset:{confirmResult:'cancel'}})}});
 assert.equal(await third,false);
});
test('server notices are converted once and hidden only after enhancement',t=>{
 const original=globalThis.document;t.after(()=>{globalThis.document=original;});
 globalThis.document={getElementById:()=>null};
 const node={dataset:{},textContent:'Saved',childNodes:[],classList:{contains:()=>false},hidden:false};
 const root={querySelectorAll:()=>[node]};
 assert.equal(showNotices(root),1);assert.equal(node.hidden,true);assert.equal(showNotices(root),0);
 toast('No region yet');
});

test('toasts escape plain messages, cap the stack and support close and pause',t=>{
 const original={document:globalThis.document,setTimeout:globalThis.setTimeout,clearTimeout:globalThis.clearTimeout};
 t.after(()=>Object.assign(globalThis,original));
 const timers=new Set();globalThis.setTimeout=fn=>{timers.add(fn);return fn;};globalThis.clearTimeout=fn=>timers.delete(fn);
 function node(){return {children:[],attrs:{},events:{},dataset:{},setAttribute(key,value){this.attrs[key]=value;},addEventListener(type,fn){this.events[type]=fn;},append(...children){this.children.push(...children);for(const child of children)child.parent=this;},remove(){this.parent.children.splice(this.parent.children.indexOf(this),1);}};}
 const region=node();Object.defineProperty(region,'firstElementChild',{get:()=>region.children[0]});
 globalThis.document={getElementById:()=>region,createElement:()=>node()};
 toast('<img onerror=alert(1)>');
 const first=region.children[0];assert.equal(first.children[1].textContent,'<img onerror=alert(1)>');
 first.events.pointerenter();assert.equal(timers.size,0);first.events.pointerleave();assert.equal(timers.size,1);
 toast('Ошибка','error');assert.equal(region.children[1].attrs.role,'alert');
 toast('Third');toast('Fourth');assert.equal(region.children.length,3);
 const last=region.children[2];last.children[2].events.click();assert.equal(region.children.length,2);
});

test('publishing and saving ignore the trash submit button, including a button owned by another form',()=>{
 const trash={name:'action',value:'trash'};
 const editor={querySelector:selector=>selector==='[name="action"]'?trash:null,getAttribute:()=>'/admin/pages/save'};
 for(const intent of ['publish','save']){
  const submitter={name:'intent',value:intent,form:editor,closest:selector=>selector==='form'?editor:null};
  assert.equal(deletionPrompt(submitter),null);
 }
 assert.equal(deletionPrompt({closest:selector=>selector==='form'?editor:null}),null);
 const state={getAttribute:()=>'/admin/pages/12/state'};
 const remove={name:'action',value:'delete',form:state,closest:selector=>selector==='form'?editor:null};
 assert.equal(deletionPrompt(remove).label,'Удалить навсегда');
 assert.equal(deletionPrompt({form:editor,closest:()=>null},{action:'trash'}).label,'В корзину');
});


test('updating uses its own confirmation label and reports preparation rather than success',()=>{
 const updateForm={querySelector:()=>null,getAttribute:name=>name==='action'?'/admin/updates/install':null};
 const submitter={form:updateForm,closest:selector=>selector==='form'?updateForm:null};
 assert.equal(deletionPrompt(submitter).label,'Обновить');
 assert.equal(actionMessage(submitter),'Проверка перед обновлением начата.');
});

test('restoring a template file confirms restoration rather than deletion',()=>{
 const f={getAttribute:name=>name==='action'?'/admin/themes/editor/save':null,querySelector:()=>null};
 const restore={name:'intent',value:'restore',form:f,closest:selector=>selector==='[hx-confirm]'?{getAttribute:()=> 'Вернуть предыдущую версию?'}:f};
 assert.deepEqual(deletionPrompt(restore,{intent:'restore'}),{message:'Вернуть предыдущую версию?',label:'Восстановить'});
});
