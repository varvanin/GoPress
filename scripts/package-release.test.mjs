import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync, mkdirSync, writeFileSync, readFileSync, readdirSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join, delimiter} from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';

const script=fileURLToPath(new URL('./package-release.sh',import.meta.url));
test('release packaging emits only standalone binaries and matching checksums',t=>{
 const dir=mkdtempSync(join(tmpdir(),'gopress-package-'));
 t.after(()=>rmSync(dir,{recursive:true,force:true}));
 const bin=join(dir,'bin');mkdirSync(bin);
 writeFileSync(join(bin,'go'),`#!/bin/sh
set -eu
test "$CGO_ENABLED:$GOOS" = '0:linux'
test "$1" = build
while [ "$1" != '-o' ]; do shift; done
printf '%s' "$GOARCH" > "$2"
`,{mode:0o755});
 const run=version=>spawnSync('sh',[script,version],{cwd:dir,env:{...process.env,PATH:bin+delimiter+process.env.PATH},encoding:'utf8'});
 const result=run('0.2.0');assert.equal(result.status,0,result.stderr);
 const names=['gopress-0.2.0-linux-amd64','gopress-0.2.0-linux-arm64'];
 assert.deepEqual(readdirSync(join(dir,'dist')).sort(),['SHA256SUMS-0.2.0',...names].sort());
 const sums=readFileSync(join(dir,'dist','SHA256SUMS-0.2.0'),'utf8');
 for(const name of names){
  const data=readFileSync(join(dir,'dist',name));
  assert.equal(data.toString(),name.endsWith('amd64')?'amd64':'arm64');
  assert.ok(sums.includes(createHash('sha256').update(data).digest('hex')+'  '+name));
 }
 writeFileSync(join(dir,'dist','gopress-0.2.0-linux-amd64.tar.gz'),'old package');
 assert.equal(run('0.2.0').status,0);
 assert.equal(readFileSync(join(dir,'dist','SHA256SUMS-0.2.0'),'utf8'),sums);
 for(const invalid of ['../escape','0.2.0 bad',''])assert.notEqual(run(invalid).status,0);
});
