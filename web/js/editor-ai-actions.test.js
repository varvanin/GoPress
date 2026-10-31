import {test} from 'node:test';
import assert from 'node:assert/strict';
import {translationParts,parseAIFields,aiParagraphs} from './editor-ai-actions.js';
test('translation changes only content strings and preserves structure, code, links and block ids',()=>{
 const source={type:'doc',content:[{type:'heading',attrs:{level:2,anchor:'about'},content:[{type:'text',text:'About',marks:[{type:'bold'}]}]},{type:'paragraph',content:[{type:'text',text:'Contact',marks:[{type:'link',attrs:{href:'/contact'}}]}]},{type:'paragraph',content:[{type:'text',text:'[form id=12]'}]},{type:'codeBlock',content:[{type:'text',text:'const a=1;'}]},{type:'image',attrs:{src:'/uploads/example.png',alt:'Photo',caption:'Caption'}}]};
 const parts=translationParts(source);assert.deepEqual(parts.texts,['About','Contact','Photo','Caption']);
 const translated=parts.apply(['Über uns','Kontakt','Bild','Beschreibung']);
 assert.equal(source.content[0].content[0].text,'About');assert.equal(translated.content[0].attrs.anchor,'about');assert.deepEqual(translated.content[0].content[0].marks,[{type:'bold'}]);assert.equal(translated.content[1].content[0].marks[0].attrs.href,'/contact');assert.equal(translated.content[2].content[0].text,'[form id=12]');assert.equal(translated.content[3].content[0].text,'const a=1;');assert.equal(translated.content[4].attrs.src,'/uploads/example.png');assert.equal(translated.content[4].attrs.alt,'Bild');
 assert.throws(()=>parts.apply(['wrong count']));assert.throws(()=>parts.apply(['About','Contact','x'.repeat(2001),'Caption']));
});
test('structured AI fields reject malformed values and plain output never becomes HTML',()=>{
 assert.throws(()=>parseAIFields('seo','{"title":"A","description":{}}'));
 assert.throws(()=>parseAIFields('translate','{"title":"A","texts":["one"]}',2));
 assert.throws(()=>parseAIFields('translate','{"title":"A","texts":[{},"two"]}',2));
 assert.deepEqual(aiParagraphs('<script>literal</script>\n\nSecond'),[{type:'paragraph',content:[{type:'text',text:'<script>literal</script>'}]},{type:'paragraph',content:[{type:'text',text:'Second'}]}]);
});
