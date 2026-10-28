import test from 'node:test';
import assert from 'node:assert/strict';
import {sourceLanguage,goDirectivePattern} from './template-editor.js';

test('template editor selects the source language',()=>{
 for(const [file,name] of [['layout.html','html'],['parts/card.html','html'],['assets/site.css','css'],['assets/site.js','javascript']])assert.equal(sourceLanguage(file).language.name,name);
});
test('Go template expressions are highlighted without swallowing HTML',()=>{
 const text='<h1>{{.Title}}</h1>{{if .Home}}<p>Home</p>{{else}}<p>Page</p>{{end}}';
 assert.deepEqual([...text.matchAll(goDirectivePattern)].map(m=>m[0]),['{{.Title}}','{{if .Home}}','{{else}}','{{end}}']);
});
