import {test} from 'node:test';
import assert from 'node:assert/strict';

function event(name, detail = {}) {
  const e = new Event(name, {cancelable:true});
  Object.defineProperty(e, 'detail', {value:detail});
  return e;
}
function formFixture() {
  const form = new EventTarget();
  const upload = new EventTarget();
  upload.files = [];
  const status = {textContent:''};
  const uploadStatus = {textContent:''};
  const button = {disabled:true};
  const fields = {disabled:false};
  form.dataset = {failed:'true'};
  form.elements = {document:{value:'{"type":"doc","content":[{"type":"paragraph"}]}'}};
  form.querySelector = (selector) => ({'#editor-status':status,'#upload-status':uploadStatus,'[data-image-upload]':upload,'[data-editor-fields]':fields})[selector] || null;
  form.querySelectorAll = () => [button];
  form.contains = (elt) => elt === form;
  return {form,button,status,fields};
}

test('disposing an editor screen removes listeners and destroys its editor', async () => {
  const {mountPage} = await import('./admin.js');
  const {form,fields,button} = formFixture();
  const state = mountPage(form);
  let destroyed = 0;
  const editability=[];
  state.editor = {destroy(){destroyed++;},getJSON(){return {type:'doc'};},setEditable(value,emitUpdate){assert.equal(emitUpdate,false);editability.push(value);}};
  state.setPending(true);
  assert.equal(fields.disabled,true);
  assert.equal(button.disabled,true);
  state.setPending(false);
  assert.equal(fields.disabled,false);
  assert.deepEqual(editability,[false,true]);
  form.dispatchEvent(event('submit'));
  assert.equal(state.submitting,true);
  assert.equal(form.elements.document.value,'{"type":"doc"}');
  state.dispose();
  assert.equal(destroyed,1);
  state.dirty=false;
  form.dispatchEvent(event('input'));
  assert.equal(state.dirty,false);
});

test('SPA guards unsaved navigation, restores cancelled history and swaps validation errors only', async (t) => {
  // Real EventTargets exercise application handlers; this is not browser/HTMX view QA.
  const {form} = formFixture();
  const doc = new EventTarget();
  const win = new EventTarget();
  const main = {id:'main',dataset:{section:'/admin/posts'},focus(){},querySelectorAll:()=>[]};
 doc.getElementById=()=>null;
  const status = {textContent:''};
  const links = ['/admin','/admin/posts'].map(href => ({
    attrs:new Map([['href',href]]),
    getAttribute(name){return this.attrs.get(name);},
    setAttribute(name,value){this.attrs.set(name,value);},
    removeAttribute(name){this.attrs.delete(name);},
    toggleAttribute(name,active){if(active)this.attrs.set(name,'');else this.attrs.delete(name);},
  }));
  doc.querySelector = (selector) => ({'[data-page-editor]':form,'#main':main,'#admin-nav-status':status,'meta[name="csrf-token"]':{content:'csrf'}})[selector] || null;
  doc.querySelectorAll = selector => selector.startsWith('[data-slot=') ? links : [];
  let confirmations = 0;
  const moves=[];
  const redirects=[];
  win.location={assign(url){redirects.push(url);}};
  win.confirm=()=>{confirmations++;return false;};
  win.history={state:{htmx:true},replaceState(value){this.state=value;},go(delta){moves.push(delta);}};
  globalThis.document=doc;globalThis.window=win;
  t.after(()=>{
    doc.dispatchEvent(event('htmx:beforeCleanupElement',{elt:form}));
    delete globalThis.document;delete globalThis.window;
  });
  await import('./admin.js?navigation-test');
  assert.equal(links[1].attrs.get('aria-current'),'page');
  const click=event('htmx:beforeRequest',{target:main,elt:links[0]});
  doc.dispatchEvent(click);
  assert.equal(click.defaultPrevented,true);
  assert.equal(confirmations,1);
  const media=event('htmx:beforeRequest',{target:{id:'media-picker'},elt:form});
  doc.dispatchEvent(media);
  assert.equal(media.defaultPrevented,false);
  const templateFile=event('htmx:beforeRequest',{target:{id:'template-file-editor'},elt:links[0]});
  doc.dispatchEvent(templateFile);
  assert.equal(templateFile.defaultPrevented,true);

  win.history.state={htmx:true};
  doc.dispatchEvent(event('htmx:beforeHistoryUpdate'));
  assert.equal(win.history.state.gpIndex,0);
  win.history.state={htmx:true};
  doc.dispatchEvent(event('htmx:pushedIntoHistory'));
  assert.equal(win.history.state.gpIndex,1);
  const pop=event('popstate');Object.defineProperty(pop,'state',{value:{htmx:true,gpIndex:0}});
  win.dispatchEvent(pop);
  assert.deepEqual(moves,[1]);
  const revert=event('popstate');Object.defineProperty(revert,'state',{value:{htmx:true,gpIndex:1}});
  win.dispatchEvent(revert);
  assert.equal(confirmations,3);
  for (const responseText of ['<main id="main">saved fields</main>','Version conflict']) {
    const detail={target:main,xhr:{status:409,responseText},shouldSwap:false,isError:true};
    doc.dispatchEvent(event('htmx:beforeSwap',detail));
    assert.equal(detail.shouldSwap,responseText.includes('id="main"'));
  }
  const headers={};
  doc.dispatchEvent(event('htmx:configRequest',{headers}));
  assert.equal(headers['X-CSRF-Token'],'csrf');
  assert.equal(headers.Accept,'text/html');
  doc.dispatchEvent(event('htmx:historyCacheMissLoad',{xhr:{getResponseHeader(){return '/login';}}}));
  assert.deepEqual(redirects,['/login']);
  const cancel={hasAttribute:name=>name==='data-editor-cancel'};
  form.contains=element=>element===form||element===cancel;
  const cancelConfirm=event('htmx:confirm',{target:main,elt:cancel,issueRequest(){throw new Error('Cancel must not force autosave');}});
  doc.dispatchEvent(cancelConfirm);assert.equal(cancelConfirm.defaultPrevented,false);
  const cancelRequest=event('htmx:beforeRequest',{target:main,elt:cancel});
  doc.dispatchEvent(cancelRequest);assert.equal(cancelRequest.defaultPrevented,true);assert.equal(confirmations,4);
  const seoTab=event('htmx:beforeRequest',{target:{id:'seo-panel'},elt:links[0]});
  doc.dispatchEvent(seoTab);assert.equal(seoTab.defaultPrevented,true);
  for(const status of [400,409]){
    const detail={target:{id:'seo-panel'},xhr:{status,responseText:'<div id="seo-panel">unsaved fields</div>'},shouldSwap:false,isError:true};
    doc.dispatchEvent(event('htmx:beforeSwap',detail));assert.equal(detail.shouldSwap,true);assert.equal(detail.isError,false);
  }


});

test('adding an inline image preserves the selected cover',async(t)=>{
 const {mountPage}=await import('./admin.js');
 const fixture=formFixture();const {form}=fixture;
 const upload=new EventTarget();upload.click=()=>{};upload.files=[{size:10,type:'image/png'}];
 const cover={value:'old-cover.png'};
 const originalQuery=form.querySelector;
 form.querySelector=selector=>selector==='[data-image-upload]'?upload:selector==='input[name="cover"]:checked'?cover:originalQuery(selector);
 const original={document:globalThis.document,window:globalThis.window,fetch:globalThis.fetch};
 let pickerURL='';
 globalThis.document={querySelector:()=>({content:'csrf'}),getElementById:()=>null};
 globalThis.window={htmx:{ajax:async(method,url)=>{pickerURL=url;}}};
 const name='a'.repeat(64)+'.png';
 globalThis.fetch=async()=>({ok:true,json:async()=>({name})});
 const state=mountPage(form);state.editor={destroy(){}};
 t.after(()=>{state.dispose();Object.assign(globalThis,original);});
 const inserted=new Promise(resolve=>form.addEventListener('editor:insert-image',event=>resolve(event.detail.name),{once:true}));
 form.dispatchEvent(new Event('editor:upload-image'));
 upload.dispatchEvent(new Event('change'));
 assert.equal(await inserted,name);
 assert.equal(cover.value,'old-cover.png');
 assert.ok(pickerURL.endsWith('selected=old-cover.png'));
});

test('searching media does not autosave an unchanged material',async()=>{
 const {mountPage}=await import('./admin.js');
 const {form}=formFixture();form.dataset.failed='false';
 const state=mountPage(form);
 const search=event('input');Object.defineProperty(search,'target',{value:{name:'q',form}});
 form.dispatchEvent(search);assert.equal(state.dirty,false);
 const seo=event('input');Object.defineProperty(seo,'target',{value:{name:'seo_description',form}});
 form.dispatchEvent(seo);assert.equal(state.dirty,true);
 state.dispose();
});


test('page parent, order and template changes mark the editor dirty',async()=>{
 const {mountPage}=await import('./admin.js');
 const {form}=formFixture();const state=mountPage(form);
 for(const name of ['parent_id','order','template']) {
  state.dirty=false;
  const event=new Event('change');Object.defineProperty(event,'target',{value:{name,form}});
  form.dispatchEvent(event);
  assert.equal(state.dirty,true,name);
 }
 state.dispose();
});
