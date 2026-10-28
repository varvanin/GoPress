import test from 'node:test';
import assert from 'node:assert/strict';
import {adminThemeKey,readAdminTheme,applyAdminTheme,storeAdminTheme} from './admin-theme.js';

test('admin color preference is local, validated and survives blocked storage',()=>{
 const values=new Map(),storage={getItem:key=>values.get(key),setItem:(key,value)=>values.set(key,value)};
 assert.equal(readAdminTheme(storage),'dark');storeAdminTheme('light',storage);assert.equal(values.get(adminThemeKey),'light');assert.equal(readAdminTheme(storage),'light');
 values.set(adminThemeKey,'unexpected');assert.equal(readAdminTheme(storage),'dark');
 const blocked={getItem(){throw new Error('denied');},setItem(){throw new Error('denied');}};
 assert.equal(readAdminTheme(blocked),'dark');assert.doesNotThrow(()=>storeAdminTheme('light',blocked));
 const classes=new Set(['dark','existing']),root={dataset:{},classList:{toggle(name,active){active?classes.add(name):classes.delete(name);}}};
 applyAdminTheme('light',root);assert.equal(root.dataset.adminTheme,'light');assert.equal(classes.has('dark'),false);assert.equal(classes.has('existing'),true);
 applyAdminTheme('dark',root);assert.equal(classes.has('dark'),true);
});
test('theme loads before content, toggles without navigation and syncs browser tabs',async t=>{
 const values=new Map([[adminThemeKey,'light']]),classes=new Set(['dark']);
 const doc=new EventTarget(),win=new EventTarget(),button={dataset:{labelLight:'Light',labelDark:'Dark'},attrs:{},setAttribute(key,value){this.attrs[key]=value;}};
 doc.documentElement={dataset:{},classList:{toggle(name,active){active?classes.add(name):classes.delete(name);}}};doc.querySelectorAll=()=>[button];
 win.localStorage={getItem:key=>values.get(key),setItem:(key,value)=>values.set(key,value)};
 globalThis.document=doc;globalThis.window=win;t.after(()=>{delete globalThis.document;delete globalThis.window;});
 let changes=0;doc.addEventListener('admin-theme-change',()=>changes++);
 await import('./admin-theme.js?bootstrap');
 assert.equal(doc.documentElement.dataset.adminTheme,'light');assert.equal(button.attrs['aria-label'],'Dark');
 const click=new Event('click');Object.defineProperty(click,'target',{value:{closest:()=>button}});doc.dispatchEvent(click);
 assert.equal(doc.documentElement.dataset.adminTheme,'dark');assert.equal(values.get(adminThemeKey),'dark');assert.equal(button.attrs.title,'Light');
 values.set(adminThemeKey,'light');const storage=new Event('storage');Object.defineProperty(storage,'key',{value:adminThemeKey});win.dispatchEvent(storage);
 assert.equal(doc.documentElement.dataset.adminTheme,'light');assert.equal(changes,3);
});
