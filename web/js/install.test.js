import {test} from 'node:test';
import assert from 'node:assert/strict';
import {initializeInstallation,initializeInstallationLanguage} from './install.js';

function environment(hash=''){
 const calls=[],error={hidden:true},address={value:''},zone={value:'UTC'};
 const settings={open:false},button={disabled:false},listeners={};
 const form={querySelector:()=>settings,querySelectorAll:()=>[button],addEventListener:(name,fn)=>listeners[name]=fn};
 const env={
  document:{getElementById:id=>({'installation-csrf':{value:'csrf'},base_url:address}[id]),querySelector:selector=>({'[data-installation-error]':error,'[data-detect-timezone]':zone,'.installation-form':form}[selector])},
  location:{hash,pathname:'/install',search:'',origin:'https://example.com',replace:path=>calls.push(['replace',path])},
  history:{replaceState:(_state,_title,path)=>calls.push(['clean',path])},
  fetch:async(path,options)=>{calls.push(['fetch',path,options]);return {ok:true};},
  Intl:{DateTimeFormat:()=>({resolvedOptions:()=>({timeZone:'Europe/Istanbul'})})}
 };
 return {env,calls,error,address,zone,settings,button,listeners};
}
test('installation link removes the secret before a CSRF-protected exchange',async()=>{
 const v=environment('#setup='+'a'.repeat(64));await initializeInstallation(v.env);
 assert.deepEqual(v.calls.map(c=>c[0]),['clean','fetch','replace']);
 assert.equal(v.calls[0][1],'/install');assert.equal(v.calls[1][1],'/install/unlock');
 const request=v.calls[1][2];assert.equal(request.method,'POST');assert.equal(request.credentials,'same-origin');
 assert.equal(request.body.get('token'),'a'.repeat(64));assert.equal(request.body.get('csrf'),'csrf');
 assert.equal(v.calls[2][1],'/install');
});
test('invalid, rejected and interrupted links remain locked and show an error',async()=>{
 for(const failure of ['malformed','rejected','network']){
  const v=environment('#setup='+(failure==='malformed'?'bad':'b'.repeat(64)));
  if(failure!=='malformed')v.env.fetch=async()=>{if(failure==='network')throw new Error('offline');return {ok:false};};
  await initializeInstallation(v.env);assert.equal(v.error.hidden,false);assert.ok(!v.calls.some(c=>c[0]==='replace'));
 }
});
test('setup detects address and time zone, keeps entered values and opens invalid settings',async()=>{
 const v=environment();await initializeInstallation(v.env);
 assert.equal(v.address.value,'https://example.com');assert.equal(v.zone.value,'Europe/Istanbul');
 v.listeners.invalid();assert.equal(v.settings.open,true);v.listeners.submit();assert.equal(v.button.disabled,true);
 v.address.value='https://custom.example';v.env.document.querySelector=()=>null;
 await initializeInstallation(v.env);assert.equal(v.address.value,'https://custom.example');assert.equal(v.zone.value,'Europe/Istanbul');
});

test('following a setup link from the locked page also unlocks without a reload',async t=>{
 const v=environment(),listeners={};
 const replacements={document:v.env.document,location:v.env.location,history:v.env.history,fetch:v.env.fetch,addEventListener:(name,listener)=>listeners[name]=listener};
 const previous=new Map(Object.keys(replacements).map(key=>[key,Object.getOwnPropertyDescriptor(globalThis,key)]));
 t.after(()=>{for(const [key,descriptor] of previous){if(descriptor)Object.defineProperty(globalThis,key,descriptor);else delete globalThis[key];}});
 Object.assign(globalThis,replacements);
 await import('./install.js?hash-navigation-test');
 v.env.location.hash='#setup='+'c'.repeat(64);
 await listeners.hashchange();
 assert.deepEqual(v.calls.map(call=>call[0]),['clean','fetch','replace']);
});

test('setup keeps the chosen language when exchanging an installation link',async()=>{
 const v=environment('#setup='+'a'.repeat(64));v.env.location.search='?language=fr';
 await initializeInstallation(v.env);
 assert.equal(v.calls.at(-1)[1],'/install?language=fr');
});

function languageEnvironment(){
 const v=environment(),listeners={},nodes=[{dataset:{installationCopy:'Site name'},textContent:'Site name'}];
 const language={value:'en',addEventListener:(name,fn)=>listeners[name]=fn};
 const heading={textContent:''},placeholder={dataset:{installationPlaceholder:'My site'},placeholder:''},trigger={setAttribute:()=>{}};
 v.env.document.documentElement={lang:'en',dataset:{adminTheme:'dark'}};
 v.env.document.getElementById=id=>({'auth-title':heading,'installation-language':trigger}[id]);
 v.env.document.querySelector=()=>v.error;
 v.env.document.querySelectorAll=selector=>({'[data-installation-copy]':nodes,'[data-installation-placeholder]':[placeholder],'[data-admin-theme-toggle]':[]}[selector]);
 v.error.hidden=true;
 v.env.fetch=async()=>({ok:true,json:async()=>({'Site name':'Nom du site','Install GoPress':'Installation de GoPress','My site':'Mon site'})});
 initializeInstallationLanguage({...v.env,fetch:(...args)=>v.env.fetch(...args)},language);
 return {...v,language,change:()=>listeners.change(),nodes,heading,placeholder};
}
test('interface language changes text and URL without replacing form fields',async()=>{
 const v=languageEnvironment();v.language.value='fr';await v.change();
 assert.equal(v.nodes[0].textContent,'Nom du site');assert.equal(v.placeholder.placeholder,'Mon site');
 assert.equal(v.env.document.documentElement.lang,'fr');assert.equal(v.heading.textContent,'Installation de GoPress');
 assert.equal(v.env.document.title,'Installation de GoPress · GoPress');
 assert.equal(v.calls.at(-1)[1],'/install?language=fr');assert.equal(v.error.hidden,true);
 v.language.value='en';v.env.fetch=async()=>({ok:true,json:async()=>null});await v.change();
 assert.equal(v.nodes[0].textContent,'Site name');assert.equal(v.env.document.documentElement.lang,'en');
});
test('failed language requests restore the applied language and ignore stale responses',async()=>{
 const v=languageEnvironment();v.env.fetch=async()=>({ok:false});v.language.value='fr';await v.change();
 assert.equal(v.language.value,'en');assert.equal(v.error.hidden,false);
 let finish;v.env.fetch=()=>new Promise(resolve=>finish=resolve);
 v.language.value='fr';const pending=v.change();v.language.value='en';await v.change();
 finish({ok:true,json:async()=>({'Site name':'Nom du site'})});await pending;
 assert.equal(v.env.document.documentElement.lang,'en');assert.equal(v.nodes[0].textContent,'Site name');
});
