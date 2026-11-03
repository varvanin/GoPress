export async function initializeInstallation({document,location,history,fetch,Intl}) {
 const parameters=new URLSearchParams(location.hash.slice(1));
 const token=parameters.get('setup');
 if(token!==null){
  history.replaceState(null,'',location.pathname+location.search);
  const error=document.querySelector('[data-installation-error]');
  try{
   if(!/^[a-f0-9]{64}$/.test(token))throw new Error('Invalid installation link');
   const response=await fetch('/install/unlock',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({token,csrf:document.getElementById('installation-csrf').value})});
   if(!response.ok)throw new Error('Installation link rejected');
   location.replace('/install');
  }catch{error.hidden=false;}
  return;
 }
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
