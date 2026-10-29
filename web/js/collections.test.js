import test from 'node:test';
import assert from 'node:assert/strict';
import {chooseCollectionImage,searchCollectionMedia} from './collections.js';
test('collection picker changes only its own row and can clear a photo',()=>{
 const input={value:'',dispatchEvent:()=>{}},preview={hidden:true,removeAttribute:()=>{delete preview.src;}},details={open:true};let changed=0;
 const row={closest:()=>null,querySelector:s=>s.startsWith('input')?input:s==='details'?details:preview,dispatchEvent:()=>changed++};
 chooseCollectionImage({target:{closest:()=>({dataset:{collectionChoice:'safe.png'},closest:()=>row})}});
 assert.equal(input.value,'safe.png');assert.equal(preview.src,'/uploads/safe.png');assert.equal(preview.hidden,false);assert.equal(details.open,false);
 chooseCollectionImage({target:{closest:()=>({dataset:{},closest:()=>row})}});
 assert.equal(input.value,'');assert.equal(preview.hidden,true);assert.equal(preview.src,undefined);assert.equal(changed,2);
 chooseCollectionImage({target:{closest:()=>null}});assert.equal(changed,2);
});

test('Enter in media search searches without submitting the editor',t=>{
 const old=globalThis.htmx;t.after(()=>{globalThis.htmx=old;});let prevented=0,searched=0;
 globalThis.htmx={trigger:()=>searched++};
 searchCollectionMedia({key:'Enter',target:{matches:()=>true},preventDefault:()=>prevented++});
 assert.equal(prevented,1);assert.equal(searched,1);
 searchCollectionMedia({key:'Enter',target:{matches:()=>false},preventDefault:()=>prevented++});
 searchCollectionMedia({key:'Escape',target:{matches:()=>true},preventDefault:()=>prevented++});
 assert.equal(prevented,1);
});
