export const adminThemeKey='gopress.admin-theme';
export function readAdminTheme(storage){try{return storage.getItem(adminThemeKey)==='light'?'light':'dark';}catch{return 'dark';}}
export function applyAdminTheme(mode,root){mode=mode==='light'?'light':'dark';root.dataset.adminTheme=mode;root.classList.toggle('dark',mode==='dark');return mode;}
export function storeAdminTheme(mode,storage){try{storage.setItem(adminThemeKey,mode);}catch{/* Browser privacy settings may disable persistence. */}}

if(typeof document!=='undefined'){
 const root=document.documentElement;
 const storage=()=>{try{return window.localStorage;}catch{return null;}};
 const syncButtons=()=>document.querySelectorAll('[data-admin-theme-toggle]').forEach(button=>{
  const label=root.dataset.adminTheme==='light'?button.dataset.labelDark:button.dataset.labelLight;
  button.setAttribute('aria-label',label);button.setAttribute('title',label);
 });
 const apply=mode=>{applyAdminTheme(mode,root);syncButtons();document.dispatchEvent(new Event('admin-theme-change'));};
 apply(readAdminTheme(storage()));
 document.addEventListener('DOMContentLoaded',syncButtons,{once:true});
 document.addEventListener('click',event=>{
  if(!event.target.closest?.('[data-admin-theme-toggle]'))return;
  const mode=root.dataset.adminTheme==='light'?'dark':'light';apply(mode);storeAdminTheme(mode,storage());
 });
 window.addEventListener('storage',event=>{if(event.key===adminThemeKey||event.key===null)apply(readAdminTheme(storage()));});
}
