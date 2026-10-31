import {test} from 'node:test';
import assert from 'node:assert/strict';
import {sourceDiff} from './template-diff.js';
test('source comparison includes every changed line with bounded common context',()=>{
 assert.deepEqual(sourceDiff('same','same'),{changed:false,before:'',after:''});
 const before=Array.from({length:30},(_,i)=>'line '+i),after=[...before];after[12]='changed';after[20]='also changed';
 const diff=sourceDiff(before.join('\n'),after.join('\n'));assert.equal(diff.start,10);assert(diff.before.includes('line 12'));assert(diff.after.includes('changed'));assert(diff.after.includes('also changed'));assert(!diff.after.includes('line 0\n'));assert(!diff.after.includes('line 29'));
 assert(sourceDiff('', 'new').changed);assert(sourceDiff('old','').changed);assert.throws(()=>sourceDiff('', 'x'.repeat(512*1024+1)));
});
