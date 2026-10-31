import test from 'node:test';
import assert from 'node:assert/strict';
import {readdirSync,readFileSync} from 'node:fs';
import {join} from 'node:path';
import {runInNewContext} from 'node:vm';
import {formatTemplateSource,templateActions} from './template-formatter.js';

const html='<html><head><title>{{.Title}}</title></head><body><section>{{if .Home}}<h1>{{.Title}}</h1>{{else}}<p>{{template "parts/card" .}}</p>{{end}}</section></body></html>';
test('HTML formatting preserves Go actions and is stable',()=>{
 const formatted=formatTemplateSource(html,'layout.html');
 assert(formatted.split('\n').length>5);
 assert.deepEqual(templateActions(formatted),templateActions(html));
 assert.equal(formatTemplateSource(formatted,'layout.html'),formatted);
});
test('Go action scanner handles quotes, raw strings, comments and trim markers',()=>{
 const actions=['{{printf "}}" .Title}}','{{- printf `raw }} {{ text` -}}','{{/* }} and {{ and " are comment text */}}','{{printf "escaped \\" quote"}}'];
 for(const action of actions.slice(0,3))assert.deepEqual(templateActions(action),[action]);
 assert.deepEqual(templateActions('{{printf "a\\\"}}b"}}'),['{{printf "a\\\"}}b"}}']);
 assert.throws(()=>templateActions('{{if .Home'),/Unclosed/);
});
test('every bundled HTML file keeps its Go actions when formatted',()=>{
 for(const folder of ['theme','theme-minimal','theme-studio']){
  const root=join('internal/httpui',folder);
  for(const file of readdirSync(root,{recursive:true}).filter(name=>name.endsWith('.html'))){
   const original=readFileSync(join(root,file),'utf8');
   const formatted=formatTemplateSource(original,file);
   assert.deepEqual(templateActions(formatted),templateActions(original),folder+'/'+file);
   assert.equal(formatTemplateSource(formatted,file),formatted,folder+'/'+file+' is not stable');
  }
 }
});
test('preformatted HTML and inline text are retained',()=>{
 const pre='<pre>  first\n\tsecond {{.Title}}\n</pre>';
 const formatted=formatTemplateSource('<div>'+pre+'<p>Hello <strong>world</strong>!</p></div>','page.html');
 assert(formatted.includes(pre));assert(formatted.includes('Hello <strong>world</strong>!'));
});
test('CSS and JavaScript are formatted without changing strings or execution',()=>{
 const css=formatTemplateSource('body{color:red;margin:0}p{content:"a  b"}','assets/site.css');
 assert(css.includes('\n  color: red;'));assert(css.includes('"a  b"'));
 const js='(()=>{const x=`a  b`;return {text:x,value:[1,2,3].map(n=>n*2).join(",")}})()';
 const formatted=formatTemplateSource(js,'assets/site.js');
 assert(formatted.includes('\n'));
 assert.equal(JSON.stringify(runInNewContext(formatted)),JSON.stringify(runInNewContext(js)));
});
test('oversized input and unclosed Go actions fail before editor replacement',()=>{
 assert.throws(()=>formatTemplateSource('x'.repeat(512*1024+1),'page.html'),/large/);
 assert.throws(()=>formatTemplateSource('<div>{{if .Home','page.html'),/Unclosed/);
});
