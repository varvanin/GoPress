import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {t as translate} from './i18n.js';
import {deletionPrompt} from './feedback.js';

test('interface catalogs translate JS actions and preserve whitespace and unknown text',t=>{
 t.after(()=>delete globalThis.GoPressTranslations);
 for(const language of ['en','pl']){
  globalThis.GoPressTranslations=JSON.parse(readFileSync(new URL(`../../internal/i18n/locales/${language}.json`,import.meta.url)));
  assert.notEqual(translate('Настройки'),'Настройки');assert.equal(translate('  Настройки '),'  '+translate('Настройки')+' ');
  assert.equal(translate('My title'),'My title');
  const prompt=deletionPrompt({name:'action',value:'trash',closest:()=>null});
  assert.equal(prompt.label,translate('В корзину'));assert.notEqual(prompt.message,'Переместить выбранные материалы в корзину? Их можно будет восстановить.');
 }
 delete globalThis.GoPressTranslations;assert.equal(translate('Настройки'),'Настройки');
});
