import {test} from 'node:test';
import assert from 'node:assert/strict';
import {draftSnapshot,readDraft} from './draft-recovery.js';
test('local snapshots include all language fields and repeated terms but no credentials',t=>{
 const old=globalThis.FormData;t.after(()=>globalThis.FormData=old);
 let serialized=false;const form={dispatchEvent:()=>serialized=true};
 globalThis.FormData=class extends Array{constructor(){super(['id','12'],['version','3'],['title','Private draft'],['term_ids','1'],['term_ids','2'],['translations','{"en":{}}'],['csrf','secret'],['api_key','secret']);}};
 const data=new URLSearchParams(draftSnapshot(form));assert(serialized);assert.equal(data.get('title'),'Private draft');assert.deepEqual(data.getAll('term_ids'),['1','2']);assert.equal(data.get('translations'),'{"en":{}}');assert(!data.has('csrf'));assert(!data.has('api_key'));
});
test('invalid, expired and oversized local records cannot be recovered',()=>{
 const now=1000000000,storage={value:'{',getItem(){return this.value;}};
 assert.equal(readDraft(storage,'key',now),null);
 const record={at:now,token:'one',snapshot:'id=1&version=3'};storage.value=JSON.stringify(record);assert.deepEqual(readDraft(storage,'key',now),record);
 storage.value=JSON.stringify({...record,at:now-8*86400000});assert.equal(readDraft(storage,'key',now),null);
 storage.value=JSON.stringify({...record,at:now+120000});assert.equal(readDraft(storage,'key',now),null);
 storage.value=JSON.stringify({...record,snapshot:'x'.repeat(4*1024*1024+1)});assert.equal(readDraft(storage,'key',now),null);
 assert.equal(readDraft({getItem(){throw new Error('blocked storage');}},'key',now),null);
});
