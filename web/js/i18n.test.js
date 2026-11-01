import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {t as translate} from './i18n.js';
import {deletionPrompt} from './feedback.js';

test('interface catalogs translate JS actions and preserve whitespace and unknown text',t=>{
 t.after(()=>delete globalThis.GoPressTranslations);
 for(const language of ['ru','pl']){
  globalThis.GoPressTranslations=JSON.parse(readFileSync(new URL(`../../internal/i18n/locales/${language}.json`,import.meta.url)));
  assert.notEqual(translate('Settings'),'Settings');assert.equal(translate('  Settings '),'  '+translate('Settings')+' ');
  assert.equal(translate('My title'),'My title');
  const prompt=deletionPrompt({name:'action',value:'trash',closest:()=>null});
  assert.equal(prompt.label,translate('Move to trash'));assert.notEqual(prompt.message,'Move the selected content to trash? You can restore it later.');
 }
 delete globalThis.GoPressTranslations;assert.equal(translate('Settings'),'Settings');
});
