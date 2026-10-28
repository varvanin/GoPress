import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { mountEditor, safeLink, uploadImagePath } from './editor.js';

const fixture = JSON.parse(readFileSync(new URL('../testdata/editor-document.json', import.meta.url), 'utf8'));

// Exercise Tiptap's real headless state; this does not simulate browser rendering.
function setup(t, content = fixture) {
  const events = new Map();
  globalThis.window = { addEventListener: (name, listener) => events.set(name, listener) };
  const field = { value: JSON.stringify(content) };
  const form = Object.assign(new EventTarget(), { elements: { document: field }, querySelector: () => null, querySelectorAll: () => [] });
  let changes = 0;
  const editor = mountEditor(form, () => changes++);
  t.after(() => { editor.destroy(); delete globalThis.window; });
  return { editor, field, events, form, get changes() { return changes; } };
}

test('editor round-trips the shared server document without adding unsupported attributes', (t) => {
  const { editor } = setup(t);
  assert.deepEqual(JSON.parse(JSON.stringify(editor.getJSON())), fixture);
  assert.deepEqual(Object.keys(editor.schema.nodes).sort(), ['blockquote', 'bulletList', 'codeBlock', 'doc', 'hardBreak', 'heading', 'horizontalRule', 'image', 'listItem', 'orderedList', 'paragraph', 'text']);
  assert.deepEqual(Object.keys(editor.schema.marks).sort(), ['bold', 'code', 'highlight', 'italic', 'link', 'strike', 'subscript', 'superscript', 'underline']);
});

test('real Tiptap commands update the saved JSON', (t) => {
  const state = setup(t, { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Editable text' }] }] });
  state.editor.commands.selectAll();
  assert.equal(state.editor.commands.toggleBold(), true);
  assert.equal(state.editor.commands.toggleHeading({ level: 2 }), true);
  const saved = JSON.parse(state.field.value);
  assert.equal(saved.content[0].type, 'heading');
  assert.equal(saved.content[0].attrs.level, 2);
  assert.deepEqual(saved.content[0].content[0].marks, [{ type: 'bold' }]);
  assert.ok(state.changes >= 2);
});

test('client schema rejects nodes disabled by the server', (t) => {
  const { editor } = setup(t);
  assert.throws(() => editor.schema.nodeFromJSON({ type: 'doc', content: [{ type: 'iframe', attrs: { src: 'https://bad.example' } }] }), /Unknown node/);
});


test('the shared rich fixture round-trips lists, links, code and upload images', (t) => {
  const content = JSON.parse(readFileSync(new URL('../testdata/editor-rich-document.json', import.meta.url), 'utf8'));
  const { editor } = setup(t, content);
  assert.deepEqual(JSON.parse(JSON.stringify(editor.getJSON())), content);
});

test('links and upload paths reject active content and external images', () => {
  for (const value of ['https://example.com/a?q=1', '/contacts', '#part', 'mailto:owner@example.com']) assert.ok(safeLink(value), value);
  for (const value of ['javascript:alert(1)', 'data:text/html,bad', '//evil.test', '/\\evil', 'https://user:pass@evil.test', '/a\nb']) assert.equal(safeLink(value), false, value);
  assert.ok(uploadImagePath.test('/uploads/' + 'a'.repeat(64) + '.png'));
  assert.equal(uploadImagePath.test('https://evil.test/a.png'), false);
});

test('real list and code commands preserve serializable content', (t) => {
  const {editor,field}=setup(t,{type:'doc',content:[{type:'paragraph',content:[{type:'text',text:'Item'}]}]});
  assert.equal(editor.commands.toggleBulletList(),true);
  assert.equal(JSON.parse(field.value).content[0].type,'bulletList');
  assert.equal(editor.commands.toggleBulletList(),true);
  assert.equal(editor.commands.toggleCodeBlock(),true);
  assert.equal(JSON.parse(field.value).content[0].type,'codeBlock');
});


test('temporarily freezing an editor does not create unsaved changes', (t) => {
  const state=setup(t);
  state.editor.setEditable(false,false);
  state.editor.setEditable(true,false);
  assert.equal(state.changes,0);
  assert.deepEqual(JSON.parse(state.field.value),fixture);
});

test('toolbar formatting commands share the validated Tiptap schema', (t) => {
  const {editor}=setup(t,{type:'doc',content:[{type:'paragraph',content:[{type:'text',text:'Formatted'}]}]});
  editor.commands.selectAll();
  editor.commands.toggleUnderline();editor.commands.toggleStrike();
  editor.commands.toggleMark('highlight');editor.commands.toggleMark('superscript');
  editor.commands.updateAttributes('paragraph',{textAlign:'center'});
  const json=editor.getJSON();
  assert.equal(json.content[0].attrs.textAlign,'center');
  assert.deepEqual(json.content[0].content[0].marks.map(mark=>mark.type).sort(),['highlight','strike','superscript','underline']);
  editor.commands.toggleMark('subscript');
  assert.equal(editor.getJSON().content[0].content[0].marks.some(mark=>mark.type==='superscript'),false);
});

test('editor search crosses marks, treats regex punctuation literally and wraps', async(t)=>{
  const {findText}=await import('./editor-format.js');
  const {editor}=setup(t,{type:'doc',content:[{type:'paragraph',content:[{type:'text',text:'Hello '},{type:'text',marks:[{type:'bold'}],text:'world.'}]},{type:'paragraph',content:[{type:'text',text:'Next world'}]}]});
  assert.deepEqual(findText(editor.state.doc,'hello world',0),{from:1,to:12});
  assert.deepEqual(findText(editor.state.doc,'.',0),{from:12,to:13});
  const second=findText(editor.state.doc,'world',13);
  assert.ok(second.from>13);
  assert.deepEqual(findText(editor.state.doc,'world',1000),{from:7,to:12});
});

test('all toolbar formatting round-trips through the shared server fixture',t=>{
 const content=JSON.parse(readFileSync(new URL('../testdata/editor-format-document.json',import.meta.url),'utf8'));
 const {editor}=setup(t,content);
 assert.deepEqual(JSON.parse(JSON.stringify(editor.getJSON())),content);
});


test('library insertion uses metadata without replacing the chosen cover',t=>{
 const {editor,form}=setup(t);
 const cover={value:'old-cover.png'};
 form.querySelector=selector=>selector==='input[name="cover"]:checked'?cover:null;
 let inserted;
 const chain={focus(){return this;},insertContent(node){inserted=node;return this;},run(){return true;}};
 editor.chain=()=>chain;
 const name='a'.repeat(64)+'.png';
 const button={dataset:{libraryInsert:name,mediaAlt:'Пейзаж',mediaCaption:'Подпись'}};
 const click=new Event('click');Object.defineProperty(click,'target',{value:{closest:()=>button}});
 form.dispatchEvent(click);
 assert.deepEqual(inserted,{type:'image',attrs:{src:'/uploads/'+name,alt:'Пейзаж',caption:'Подпись'}});
 assert.equal(cover.value,'old-cover.png');
});
