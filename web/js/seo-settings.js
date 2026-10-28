export function updateSEOSettings(form){
 if(!form)return;
 const tab=form.elements.tab?.value;
 if(tab)document.querySelectorAll('[data-seo-tab-link]').forEach(link=>{const active=new URL(link.href).searchParams.get('tab')===tab;link.setAttribute('aria-current',active?'page':'false');});
 form.querySelectorAll('[data-seo-count]').forEach(counter=>{const field=form.elements[counter.dataset.seoCount];if(field)counter.textContent=`${Array.from(field.value).length} / ${field.maxLength}`;});
 const preview=document.querySelector('[data-seo-preview]');if(!preview)return;
 const field=name=>form.elements[name]?.value;
 if(field('seo_title')!==undefined){preview.querySelector('[data-seo-preview-title]').textContent=field('seo_title')||field('site_name')||preview.dataset.site;preview.querySelector('[data-seo-preview-description]').textContent=field('seo_description')||field('description_fallback')||preview.dataset.fallbackDescription;}
}
if(typeof document!=='undefined')document.addEventListener('input',event=>{const form=event.target.closest?.('[data-seo-settings]');if(form)updateSEOSettings(form);});
