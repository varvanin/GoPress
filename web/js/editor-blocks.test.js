import test from 'node:test';
import assert from 'node:assert/strict';
import {parseBlock,blockSettingsURL} from './editor-blocks.js';

test('block cards recognize only complete safe shortcode paragraphs',()=>{
 for(const code of ['[faq id=1]','[form id=2]','[gallery id=3]','[testimonials id=4]','[module id="contact-card"]'])assert.equal(parseBlock(code).code,code);
 for(const code of ['Before [faq id=1]','[faq id=0]','[module id="../secret"]','[form id=1] after','[faq id="1"]'])assert.equal(parseBlock(code),null);
 assert.equal(blockSettingsURL(parseBlock('[module id="contact-card"]')),'/admin/modules/package/contact-card/edit');
 assert.equal(blockSettingsURL(parseBlock('[form id=2]')),'/admin/forms/2/edit');
});
