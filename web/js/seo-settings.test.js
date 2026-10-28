import test from 'node:test';
import assert from 'node:assert/strict';
import {updateSEOSettings} from './seo-settings.js';

test('SEO counters count characters and previews use text content',t=>{
 const counter={dataset:{seoCount:'seo_title'},textContent:''},title={},description={};
 const preview={dataset:{site:'Site',fallbackDescription:'Fallback'},querySelector:s=>s==='[data-seo-preview-title]'?title:description};
 globalThis.document={querySelector:()=>preview};t.after(()=>delete globalThis.document);
 const form={elements:{seo_title:{value:'Title 😀',maxLength:200},seo_description:{value:'<script>text</script>'},site_name:{value:'Brand'},description_fallback:{value:''}},querySelectorAll:()=>[counter]};
 updateSEOSettings(form);assert.equal(counter.textContent,'7 / 200');assert.equal(title.textContent,'Title 😀');assert.equal(description.textContent,'<script>text</script>');
 form.elements.seo_title.value='';form.elements.seo_description.value='';updateSEOSettings(form);assert.equal(title.textContent,'Brand');assert.equal(description.textContent,'Fallback');
});

test('SEO partial navigation updates the selected tab from the new form',t=>{
 const links=['search','social'].map(tab=>({href:'https://cms.test/admin/seo?tab='+tab,attrs:{},setAttribute(name,value){this.attrs[name]=value;}}));
 globalThis.document={querySelectorAll:()=>links,querySelector:()=>null};t.after(()=>delete globalThis.document);
 updateSEOSettings({elements:{tab:{value:'social'}},querySelectorAll:()=>[]});
 assert.equal(links[0].attrs['aria-current'],'false');assert.equal(links[1].attrs['aria-current'],'page');
});
