export async function initializeInstallation({document,location,history,fetch,Intl,templ}) {
 const parameters=new URLSearchParams(location.hash.slice(1));
 const token=parameters.get('setup');
 if(token!==null){
  history.replaceState(null,'',location.pathname+location.search);
  const error=document.querySelector('[data-installation-error]');
  try{
   if(!/^[a-f0-9]{64}$/.test(token))throw new Error('Invalid installation link');
   const response=await fetch('/install/unlock',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({token,csrf:document.getElementById('installation-csrf').value})});
   if(!response.ok)throw new Error('Installation link rejected');
   location.replace('/install'+location.search);
  }catch{error.hidden=false;}
  return;
 }
 const language=document.querySelector('input[name="language"]');
 if(language)initializeInstallationLanguage({document,location,history,fetch,templ},language);
 const address=document.getElementById('base_url');
 if(address&&!address.value)address.value=location.origin;
 const zone=document.querySelector('[data-detect-timezone]');
 if(zone){try{const detected=Intl.DateTimeFormat().resolvedOptions().timeZone;if(detected)zone.value=detected;}catch{}}
 const form=document.querySelector('.installation-form');
 form?.addEventListener('invalid',()=>{const settings=form.querySelector('details');if(settings)settings.open=true;},true);
 form?.addEventListener('submit',()=>{for(const button of form.querySelectorAll('button[type="submit"]'))button.disabled=true;});
}
if(typeof document!=='undefined'){
 initializeInstallation(globalThis);
 globalThis.addEventListener('hashchange',()=>initializeInstallation(globalThis));
}

export function initializeInstallationLanguage({document,location,history,fetch,templ},language) {
 let applied=language.value,revision=0;
 language.addEventListener('change',async()=>{
  const request=++revision,selected=language.value;
  if(selected===applied)return;
  const error=document.querySelector('[data-installation-language-error]');
  try{
   const response=await fetch('/install/translations?language='+encodeURIComponent(selected));
   if(!response.ok)throw new Error('Language unavailable');
   const catalog=await response.json()||{};
   if(request!==revision)return;
   const t=source=>catalog[source]??source;
   for(const node of document.querySelectorAll('[data-installation-copy]'))node.textContent=t(node.dataset.installationCopy);
   for(const node of document.querySelectorAll('[data-installation-placeholder]'))node.placeholder=t(node.dataset.installationPlaceholder);
   document.getElementById('auth-title').textContent=t('Install GoPress');
   document.title=t('Install GoPress')+' · GoPress';
   document.documentElement.lang=selected;
   document.getElementById('installation-language').setAttribute('aria-label',t('Interface language'));
   for(const button of document.querySelectorAll('[data-admin-theme-toggle]')){
    button.dataset.labelDark=t('Dark appearance');button.dataset.labelLight=t('Light appearance');
    const label=document.documentElement.dataset.adminTheme==='light'?button.dataset.labelDark:button.dataset.labelLight;
    button.setAttribute('aria-label',label);button.setAttribute('title',label);
   }
   const params=new URLSearchParams(location.search);params.set('language',selected);
   history.replaceState(null,'',location.pathname+'?'+params.toString());
   applied=selected;error.hidden=true;
  }catch{
   if(request!==revision)return;
   language.value=applied;
   templ?.select?.setValue(document.getElementById('installation-language'),applied);
   error.hidden=false;
  }
 });
}
