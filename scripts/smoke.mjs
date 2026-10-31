import assert from 'node:assert/strict';
import {spawn,spawnSync} from 'node:child_process';
import {mkdtemp,mkdir,readFile,rm,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:net';
import {randomBytes} from 'node:crypto';
import {once} from 'node:events';

const args=new Set(process.argv.slice(2));
for(const arg of args)assert(['--docker','--browser'].includes(arg),'Unknown argument: '+arg);
const docker=args.has('--docker'),browserQA=args.has('--browser');
// Reserve the port before creating fixture data, so a denied socket leaves no files behind.
const reservation=createServer();
await new Promise((resolve,reject)=>reservation.once('error',reject).listen(0,'127.0.0.1',resolve));
const port=reservation.address().port;
await new Promise(resolve=>reservation.close(resolve));
const work=await mkdtemp(join(tmpdir(),'gopress-smoke-'));
const base='http://127.0.0.1:'+port;
const password='smoke-'+randomBytes(16).toString('hex'),email='smoke@example.test';
const container='gopress-smoke-'+randomBytes(6).toString('hex'),volume=container+'-data';
const data=docker?'/data/site':join(work,'site');
const snapshot=docker?'/data/backup':join(work,'backup');
const restored=docker?'/data/restored':join(work,'restored');
const environment={...process.env,GOPRESS_SMTP_HOST:'',GOPRESS_SMTP_FROM:'',GOPRESS_SMTP_USER:'',GOPRESS_SMTP_PASSWORD:'',GOPRESS_TRUSTED_PROXIES:''};
let server,chrome,chromeSocket,logs='',token='',containerRunning=false;
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function until(check,description,timeout=15000){const end=Date.now()+timeout;let last;while(Date.now()<end){try{const result=await check();if(result)return result;}catch(error){last=error;}await delay(100);}throw new Error('Timed out: '+description+(last?' ('+last.message+')':''));}
function cli(args,success=true){
 const result=docker?spawnSync('docker',['run','--rm','--mount',`type=volume,source=${volume},target=/data`,'gopress:ci',...args],{encoding:'utf8',timeout:60000}):spawnSync('./bin/gopress',args,{encoding:'utf8',timeout:60000,env:environment});
 assert(!result.error&&!result.signal,'CLI could not finish: '+args[0]);
 if(success)assert.equal(result.status,0,result.stderr);else assert.notEqual(result.status,0,'CLI unexpectedly accepted: '+args[0]);
 return result;
}
async function startSite(directory){
 logs='';token='';
 if(docker){
  const result=spawnSync('docker',['run','--rm','-d','--name',container,'--mount',`type=volume,source=${volume},target=/data`,'-p',`127.0.0.1:${port}:8080`,'gopress:ci','serve','--data',directory,'--addr','0.0.0.0:8080'],{encoding:'utf8',timeout:30000});
  assert.equal(result.status,0,result.stderr);containerRunning=true;
 }else{
  server=spawn('./bin/gopress',['serve','--data',directory,'--addr','127.0.0.1:'+port],{stdio:['ignore','ignore','pipe'],env:environment});
  server.stderr.on('data',chunk=>{logs+=chunk.toString();const match=logs.match(/installation token[^:]*: ([a-f0-9]{64})/);if(match)token=match[1];});
  server.on('error',error=>{logs+=error.message;});
 }
 await until(async()=>{if(server&&(server.exitCode!==null||server.signalCode!==null))throw new Error('Server exited');return (await fetch(base+'/readyz',{signal:AbortSignal.timeout(3000)})).ok;},'site readiness');
 if(docker){const output=spawnSync('docker',['logs',container],{encoding:'utf8'});const match=(output.stdout+output.stderr).match(/installation token[^:]*: ([a-f0-9]{64})/);if(match)token=match[1];}
}
async function stopSite(){
 if(docker&&containerRunning){
  const result=spawnSync('docker',['stop','--time','20',container],{encoding:'utf8',timeout:30000});
  assert.equal(result.status,0,result.stderr);containerRunning=false;
 }else if(server){
  const child=server;server=null;
  if(child.exitCode!==null||child.signalCode!==null)return;
  const exited=once(child,'exit');
  const timer=setTimeout(()=>child.kill('SIGKILL'),15000);
  try{child.kill('SIGTERM');const [code,signal]=await exited;assert.equal(code,0,'Server did not shut down cleanly: '+signal);}finally{clearTimeout(timer);}
 }
}
class BrowserHTTP {
 cookies=new Map();
 async request(path,{method='GET',data,json,raw,contentType='application/octet-stream',csrf=true}={}){
  const headers={Cookie:[...this.cookies].map(([name,value])=>name+'='+value).join('; ')};
  if(method!=='GET'&&csrf)headers['X-CSRF-Token']=this.cookies.get('csrf')||'';
  let body;
  if(json!==undefined){headers['Content-Type']='application/json';body=JSON.stringify(json);}
  else if(data){headers['Content-Type']='application/x-www-form-urlencoded';body=data.toString();}
  else if(raw!==undefined){headers['Content-Type']=contentType;body=raw;}
  const response=await fetch(base+path,{method,headers,body,redirect:'manual',signal:AbortSignal.timeout(10000)});
  for(const cookie of response.headers.getSetCookie()){const pair=cookie.split(';')[0];const index=pair.indexOf('=');this.cookies.set(pair.slice(0,index),pair.slice(index+1));}
  const bytes=Buffer.from(await response.arrayBuffer());
  return {status:response.status,location:response.headers.get('location'),body:bytes.toString('utf8'),bytes,headers:response.headers};
 }
}
const field=(html,name)=>{const match=html.match(new RegExp('name="'+name+'"[^>]*value="([^"]*)"'));assert(match,'Missing field '+name);return match[1];};
try{
 if(docker){const result=spawnSync('docker',['volume','create',volume],{encoding:'utf8'});assert.equal(result.status,0,result.stderr);}
 await startSite(data);
 await until(()=>token,'installation token');
 const admin=new BrowserHTTP(),anon=new BrowserHTTP();
 assert.equal((await admin.request('/install')).status,200);
 const installation=new URLSearchParams({token,title:'Smoke site',base_url:base,timezone:'UTC',email,password});
 assert.equal((await admin.request('/install',{method:'POST',data:installation,csrf:false})).status,403);
 const wrongToken=new URLSearchParams(installation);wrongToken.set('token','invalid');
 assert.equal((await admin.request('/install',{method:'POST',data:wrongToken})).status,401);
 const installed=await Promise.all([admin.request('/install',{method:'POST',data:installation}),admin.request('/install',{method:'POST',data:installation})]);
 assert.deepEqual(installed.map(r=>r.status).sort(),[303,409],'installation was not atomic');
 assert.equal((await admin.request('/install')).status,409);
 await admin.request('/login');
 let response=await admin.request('/login',{method:'POST',data:new URLSearchParams({email,password})});assert.equal(response.status,303);
 assert.equal((await admin.request('/admin')).status,200);
 if(!docker)cli(['healthcheck','--addr','127.0.0.1:'+port,'--installed']);
 assert.equal((await admin.request('/admin/updates')).status,200);
 const png=await readFile(new URL('./fixtures/smoke.png',import.meta.url));
 assert.equal((await anon.request('/admin/media',{method:'POST',raw:png,contentType:'image/png'})).status,401);
 assert.equal((await admin.request('/admin/media',{method:'POST',raw:png,contentType:'image/png',csrf:false})).status,403);
 response=await admin.request('/admin/media',{method:'POST',raw:png,contentType:'image/png'});assert.equal(response.status,201,response.body);
 const image=JSON.parse(response.body).name;
 for(const raw of ['<svg onload="alert(1)"/>','<html>unsafe</html>',Buffer.alloc(8*1024*1024+1)])assert.equal((await admin.request('/admin/media',{method:'POST',raw,contentType:'image/png'})).status,400);
 const publicImage=await anon.request('/uploads/'+image);assert.equal(publicImage.status,200);assert.equal(publicImage.headers.get('content-type'),'image/png');assert(publicImage.bytes.length>0);
 const document=JSON.stringify({type:'doc',content:[{type:'paragraph',content:[{type:'text',text:'Smoke publication visible text'}]},{type:'image',attrs:{src:'/uploads/'+image,alt:'Smoke image',caption:'Smoke caption'}}]});
 const newPage={id:0,version:0,title:'Smoke page',slug:'smoke-page',kind:'page',cover:image,document};
 assert.equal((await anon.request('/admin/pages',{method:'POST',json:newPage})).status,401);
 assert.equal((await admin.request('/admin/pages',{method:'POST',json:newPage,csrf:false})).status,403);
 response=await admin.request('/admin/pages',{method:'POST',json:newPage});assert.equal(response.status,200,response.body);let page=JSON.parse(response.body);
 assert.equal((await anon.request('/smoke-page')).status,404);
 assert.equal((await anon.request(`/admin/pages/${page.id}/preview`)).status,401);
 response=await admin.request(`/admin/pages/${page.id}/preview`);assert.equal(response.status,200);assert.equal(response.headers.get('cache-control'),'no-store');assert(response.headers.get('x-robots-tag')?.includes('noindex'));assert(response.body.includes(image));
 assert.equal((await admin.request('/admin/pages/'+page.id,{method:'POST',json:{...page,version:page.version-1}})).status,409);
 for(const slug of ['smoke-page','admin','theme-assets','module-assets'])assert([400,409].includes((await admin.request('/admin/pages',{method:'POST',json:{...newPage,slug}})).status),'reserved or duplicate URL accepted');
 assert.equal((await admin.request(`/admin/pages/${page.id}/publish`,{method:'POST',json:{version:page.version}})).status,204);
 response=await anon.request('/smoke-page');assert.equal(response.status,200);assert(response.body.includes('Smoke publication visible text'));assert(response.body.includes(image));
 const publicBody=response.body;
 response=await anon.request('/search?q=Smoke');assert.equal(response.status,200);assert(response.body.includes('Smoke page'));
 assert.equal((await anon.request('/missing-page')).status,404);
 page=JSON.parse((await admin.request('/admin/pages/'+page.id)).body);
 response=await admin.request('/admin/media',{method:'POST',raw:png,contentType:'image/png'});assert.equal(response.status,201);
 const privateImage=JSON.parse(response.body).name;
 const privateDocument=JSON.stringify({type:'doc',content:[{type:'paragraph',content:[{type:'text',text:'PRIVATE DRAFT BODY'}]},{type:'image',attrs:{src:'/uploads/'+privateImage,alt:'Private image',caption:'Private caption'}}]});
 response=await admin.request('/admin/pages/'+page.id,{method:'POST',json:{...page,title:'PRIVATE DRAFT TITLE',slug:'private-smoke-page',document:privateDocument,cover:privateImage,seo:{title:'PRIVATE SEO TITLE',description:'PRIVATE SEO DESCRIPTION'}}});assert.equal(response.status,200,response.body);page=JSON.parse(response.body);
 response=await anon.request('/smoke-page');assert.equal(response.body,publicBody,'draft changed public snapshot');assert(!response.body.includes(privateImage));
 assert.equal((await anon.request('/private-smoke-page')).status,404);
 assert(!(await anon.request('/search?q=PRIVATE')).body.includes('PRIVATE DRAFT'));
 const formData=new URLSearchParams({id:'0',version:'0',name:'Smoke contact',recipient:'owner@example.test',button:'Send',success:'Smoke submission saved',enabled:'true'});
 for(const [label,type] of [['Name','text'],['Email','email'],['Message','textarea']])for(const [key,value] of [['field_label',label],['field_type',type],['field_required','true'],['field_options','']])formData.append(key,value);
 response=await admin.request('/admin/forms/save',{method:'POST',data:formData});assert.equal(response.status,303,response.body);const formID=response.location.match(/forms\/(\d+)/)[1];
 const publicForm=await anon.request('/forms/'+formID);assert.equal(publicForm.status,200);
 const submitted=new URLSearchParams({csrf:anon.cookies.get('csrf'),version:field(publicForm.body,'version'),submission_key:field(publicForm.body,'submission_key'),field_0:'Visitor',field_1:'visitor@example.test',field_2:'=CSV_FORMULA()'});
 for(let i=0;i<2;i++)assert.equal((await anon.request('/forms/'+formID,{method:'POST',data:submitted})).status,303);
 response=await admin.request(`/admin/forms/${formID}/entries`);assert.equal(response.status,200);assert.equal((response.body.match(/class="cn-card[^\"]*form-entry/g)||[]).length,1,'duplicate submission stored');
 response=await admin.request(`/admin/forms/${formID}/entries/export`);assert.equal(response.status,200);assert(response.body.includes("'=CSV_FORMULA()"),'CSV formula was not escaped');
 assert.equal((await anon.request('/admin/tools')).status,401);
 response=await admin.request('/admin/tools/backups/create',{method:'POST'});assert.equal(response.status,200,response.body);
 response=await admin.request('/admin/tools/maintenance',{method:'POST',data:new URLSearchParams({enabled:'true'})});assert.equal(response.status,303);assert.equal((await anon.request('/smoke-page')).status,503);
 assert.equal((await admin.request('/admin/tools/maintenance',{method:'POST',data:new URLSearchParams({enabled:'false'})})).status,303);
 response=await admin.request('/admin/ai');assert.equal(response.status,200);
 const aiSettings=new URLSearchParams({version:field(response.body,'version'),model:'openrouter/auto',key:'smoke-ai-test-key'});
 assert.equal((await admin.request('/admin/ai/settings',{method:'POST',data:aiSettings})).status,303);
 response=await admin.request('/admin/ai');assert(!response.body.includes('smoke-ai-test-key'),'AI key leaked into settings HTML');
 if(browserQA){
  const binary=process.env.GOPRESS_BROWSER||(process.platform==='darwin'?'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome':'google-chrome');
  chrome=spawn(binary,['--headless=new','--no-sandbox','--disable-dev-shm-usage','--no-first-run','--remote-debugging-port=0','--user-data-dir='+join(work,'chrome'),'about:blank'],{stdio:['ignore','ignore','pipe']});
  let websocketURL='',chromeDiagnostics='';chrome.stderr.on('data',chunk=>{chromeDiagnostics=(chromeDiagnostics+chunk.toString()).slice(-8192);const match=chromeDiagnostics.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(match)websocketURL=match[1];});chrome.on('error',error=>{chromeDiagnostics+=error.message;});
  await until(()=>{if(chrome.exitCode!==null)throw new Error(chromeDiagnostics||'Chrome exited');return websocketURL;},'Chrome DevTools',30000);chromeSocket=new WebSocket(websocketURL);await once(chromeSocket,'open');
  let commandID=0;const pending=new Map(),exceptions=[];
  chromeSocket.addEventListener('message',event=>{const message=JSON.parse(event.data);if(message.id){const promise=pending.get(message.id);if(!promise)return;pending.delete(message.id);clearTimeout(promise.timer);message.error?promise.reject(new Error(message.error.message)):promise.resolve(message.result);}else if(message.method==='Runtime.exceptionThrown')exceptions.push(message.params.exceptionDetails.text);});
  const command=(method,params={},sessionId)=>new Promise((resolve,reject)=>{const id=++commandID;const timer=setTimeout(()=>{pending.delete(id);reject(new Error('CDP timeout: '+method));},10000);pending.set(id,{resolve,reject,timer});chromeSocket.send(JSON.stringify({id,method,params,...(sessionId?{sessionId}:{})}));});
  const {targetId}=await command('Target.createTarget',{url:'about:blank'});const {sessionId}=await command('Target.attachToTarget',{targetId,flatten:true});
  const send=(method,params)=>command(method,params,sessionId);
  const evaluate=async expression=>{const result=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(result.exceptionDetails)throw new Error(result.exceptionDetails.text);return result.result.value;};
  await send('Page.enable');await send('Runtime.enable');await send('Network.enable');
  chromeSocket.addEventListener('message',event=>{const message=JSON.parse(event.data);if(message.method==='Page.javascriptDialogOpening')void send('Page.handleJavaScriptDialog',{accept:true});});
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  await send('Network.setCookies',{cookies:[...admin.cookies].map(([name,value])=>({name,value,url:base}))});
  const navigate=async path=>{await send('Page.navigate',{url:base+path});await until(()=>evaluate('document.readyState==="complete" && location.pathname+location.search==='+JSON.stringify(path)),'page load');};
  await navigate('/admin/updates');
  assert(await evaluate('Array.from(document.forms).some(form=>form.getAttribute("action")==="/admin/updates/check")'),'update check action missing');
  for(const palette of ['dark','light']) {
   await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'updates settings overflow');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/updates-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'updates mobile overflow');
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  await navigate('/admin/ai');
  for(const palette of ['dark','light']) {
   await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'AI settings overflow');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/ai-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await navigate(`/admin/pages/${page.id}/edit`);await until(()=>evaluate('!!document.querySelector(".tiptap[contenteditable=true],.prose-editor[contenteditable=true]")'),'Tiptap mount');
  assert(await evaluate('!!document.querySelector("[data-editor-command=bold]")'));
  await evaluate('document.querySelector(".tiptap[contenteditable=true],.prose-editor[contenteditable=true]").focus()');
  await send('Input.insertText',{text:'Browser acceptance edit. '});
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return r.status===200&&JSON.parse(r.body).document.includes('Browser acceptance edit.');},'browser edit autosaved');
  assert(!(await anon.request('/smoke-page')).body.includes('Browser acceptance edit.'),'browser edit leaked before publication');
  await evaluate(`document.querySelector('[data-editor-command=insertTable]').click()`);
  assert.equal(await evaluate('document.querySelectorAll(".prose-editor table tr").length'),3,'table insertion');
  await send('Input.insertText',{text:'Table cell text'});
  assert(await evaluate('document.querySelector(".prose-editor th").textContent.includes("Table cell text")'),'table focus did not enter first cell');
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return JSON.parse(r.body).document.includes('Table cell text');},'table autosaved');
  await evaluate(`document.querySelector('[data-editor-command=blocks]').click();window.templ.select.setValue(document.getElementById('editor-block-choice'),${JSON.stringify('[form id='+formID+']')});document.querySelector('[data-insert-block]').click()`);
  await until(()=>evaluate('!!document.querySelector(".prose-editor .editor-block-card")'),'block card mount');
  assert(await evaluate('document.querySelector(".editor-block-card").textContent.includes("Smoke contact")'),'block card missing its safe name');
  await evaluate('document.querySelector("[data-block-remove]").click()');
  assert.equal(await evaluate('document.querySelectorAll(".prose-editor .editor-block-card").length'),0,'block removal');
  for(const palette of ['dark','light']){
   await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'content tools overflow');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/content-tools-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await evaluate(`window.templ.select.setValue(document.getElementById('original-content-language'),'de');document.querySelector('[data-set-original-language]').click()`);
  await until(()=>evaluate('document.querySelector("[name=source_language]").value==="de"'),'original language changed');
  const language=async value=>evaluate(`(()=>{const field=document.querySelector('[name="content_language"]');window.templ.select.setValue(document.getElementById('content-language'),${JSON.stringify(value)});field.dispatchEvent(new Event('change',{bubbles:true}));return field.value;})()`);
  assert.equal(await evaluate('document.querySelector("[name=source_language]").value'),'de');
  assert.equal(await evaluate('document.querySelectorAll("#content-language-popup [role=option]").length'),1,'an untranslated material offers unadded languages');
  await evaluate('document.querySelector("[data-add-language-panel]").open=true;document.querySelector("[data-add-content-language]").click()');
  await until(()=>evaluate('document.querySelector("[name=content_language]").value==="en" && !!document.getElementById("content-language")'),'language added');
  assert.equal(await evaluate('document.querySelectorAll("#content-language-popup [role=option]").length'),2,'language switcher contains unadded languages');
  assert.equal(await evaluate('document.querySelector("[name=title]").value'),'');
  await evaluate('(()=>{const title=document.querySelector("[name=title]");title.value="English browser title";title.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".prose-editor[contenteditable=true]").focus();})()');
  await send('Input.insertText',{text:'English browser body.'});
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return JSON.parse(r.body).translations?.en?.document.includes('English browser body.');},'translation autosaved');
  assert(await evaluate('document.querySelector("[data-saved-preview]").href.endsWith("?lang=en")'),'autosave lost preview language');
  // Simulate a failed save, then recover from this browser without a database write.
  await send('Fetch.enable',{patterns:[{urlPattern:base+'/admin/pages/save',requestStage:'Request'}]});
  const failSave=event=>{const message=JSON.parse(event.data);if(message.method==='Fetch.requestPaused')void send('Fetch.fulfillRequest',{requestId:message.params.requestId,responseCode:503,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify({message:'Temporary test failure'})).toString('base64')});};
  chromeSocket.addEventListener('message',failSave);
  await evaluate('document.querySelector(".prose-editor[contenteditable=true]").focus()');await send('Input.insertText',{text:' Local recovery acceptance.'});
  await until(()=>evaluate('Object.keys(localStorage).some(key=>key.startsWith("gopress:draft:")&&localStorage[key].includes("Local+recovery+acceptance"))'),'local draft persisted');
  await until(()=>evaluate('document.getElementById("editor-status").textContent.includes("Temporary test failure")'),'failed autosave');
  await send('Fetch.disable');chromeSocket.removeEventListener('message',failSave);
  await navigate(`/admin/pages/${page.id}/edit`);
  await until(()=>evaluate('!!document.querySelector(".prose-editor[contenteditable=true]")&&!document.querySelector("[data-draft-recovery]").hidden'),'local recovery offered');
  await evaluate('document.querySelector("[data-recovery-restore]").click()');
  await until(()=>evaluate('document.querySelector("[data-page-editor]")?.dataset.recovered==="true"&&document.querySelector(".prose-editor")?.textContent.includes("Local recovery acceptance.")'),'local draft recovered for review');
  assert(!(JSON.parse((await admin.request('/admin/pages/'+page.id)).body).translations.en.document.includes('Local recovery acceptance.')),'recovery unexpectedly wrote the database');
  await evaluate('document.querySelector(".prose-editor[contenteditable=true]").focus()');await send('Input.insertText',{text:' Reviewed.'});
  await until(async()=>JSON.parse((await admin.request('/admin/pages/'+page.id)).body).translations.en.document.includes('Local recovery acceptance.'),'reviewed recovery autosaved');
  // Mock only the provider-facing generation response: no external API usage or paid request.
  await send('Fetch.enable',{patterns:[{urlPattern:base+'/admin/pages/ai',requestStage:'Request'}]});
  let aiRequest;
  const interceptAI=event=>{const message=JSON.parse(event.data);if(message.method!=='Fetch.requestPaused')return;aiRequest=JSON.parse(message.params.request.postData);const text=aiRequest.mode==='rewrite'?'Browser rewritten selection.':aiRequest.mode==='translate'?JSON.stringify({title:'Translated browser title',texts:JSON.parse(aiRequest.text).map(text=>'Translated '+text)}):aiRequest.mode==='seo'?JSON.stringify({title:'Browser SEO title',description:'Browser SEO description'}):'AI browser text. <script>literal</script>';void send('Fetch.fulfillRequest',{requestId:message.params.requestId,responseCode:200,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify({text})).toString('base64')});};
  chromeSocket.addEventListener('message',interceptAI);
  await evaluate('document.querySelector("[data-editor-ai]").open=true;document.querySelector("[data-ai-prompt]").value="Write an introduction";document.querySelector("[data-ai-generate]").click()');
  await until(()=>evaluate('!document.querySelector("[data-ai-result]").hidden'),'AI result');
  assert.equal(aiRequest.language,'en');assert.equal(aiRequest.text,'');
  assert(!(await evaluate('document.querySelector(".prose-editor").textContent.includes("AI browser text.")')),'AI changed content without insertion');
  for(const palette of ['dark','light']) {
   await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'AI editor overflow');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/editor-ai-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await evaluate('document.querySelector("[data-ai-insert]").click()');
  assert(await evaluate('document.querySelector(".prose-editor").textContent.includes("AI browser text.")'));
  assert.equal(await evaluate('document.querySelectorAll(".prose-editor script").length'),0,'AI output interpreted as HTML');
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return JSON.parse(r.body).translations?.en?.document.includes('AI browser text.');},'AI insertion autosaved');
  const beforeActions=await evaluate('document.querySelector(".prose-editor").textContent');
  const aiMode=async mode=>{await evaluate(`window.templ.select.setValue(document.getElementById('editor-ai-action'),${JSON.stringify(mode)});document.querySelector('[data-ai-generate]').click()`);await until(()=>evaluate('!document.querySelector("[data-ai-result]").hidden'),'AI '+mode+' preview');};
  await evaluate('(()=>{const paragraph=document.querySelector(".prose-editor p");document.querySelector(".prose-editor").focus();const range=document.createRange();range.selectNodeContents(paragraph);const selection=getSelection();selection.removeAllRanges();selection.addRange(range);})()');
  await aiMode('rewrite');assert.equal(aiRequest.mode,'rewrite');
  await evaluate('document.querySelector("[data-ai-insert]").click()');assert(await evaluate('document.querySelector(".prose-editor").textContent.includes("Browser rewritten selection.")'),'AI selection replacement');
  await evaluate('document.querySelector("[data-ai-undo]").click()');assert.equal(await evaluate('document.querySelector(".prose-editor").textContent'),beforeActions,'AI selection undo');
  await aiMode('translate');assert.equal(aiRequest.mode,'translate');
  await evaluate('document.querySelector("[data-ai-insert]").click()');assert.equal(await evaluate('document.querySelector("[name=title]").value'),'Translated browser title');assert(await evaluate('document.querySelector(".prose-editor table")!==null'),'translation lost table structure');
  await evaluate('document.querySelector("[data-ai-undo]").click()');assert.equal(await evaluate('document.querySelector(".prose-editor").textContent'),beforeActions,'AI translation undo');
  const beforeSEO=await evaluate('document.querySelector("[name=seo_title]").value');await aiMode('seo');
  await evaluate('document.querySelector("[data-ai-insert]").click()');assert.equal(await evaluate('document.querySelector("[name=seo_title]").value'),'Browser SEO title');
  await evaluate('document.querySelector("[data-ai-undo]").click()');assert.equal(await evaluate('document.querySelector("[name=seo_title]").value'),beforeSEO,'AI SEO undo');
  await send('Fetch.disable');chromeSocket.removeEventListener('message',interceptAI);
  assert.equal(await language('original'),'original');
  assert(!(await evaluate('JSON.parse(document.querySelector("[name=translations]").value).pl')),'unadded language exists');
  await until(()=>evaluate('!document.querySelector("[data-editor-fields]").disabled'),'editor ready for anchor');
  await evaluate('(()=>{const paragraph=document.querySelector(".prose-editor p");const range=document.createRange();range.selectNodeContents(paragraph);range.collapse(true);const selection=getSelection();selection.removeAllRanges();selection.addRange(range);document.querySelector(".prose-editor").focus();})()');
  await evaluate('document.querySelector("[data-editor-command=anchor]").click()');
  assert(await evaluate('!document.querySelector("[data-anchor-panel]").hidden'),'anchor panel did not open');
  await evaluate('const anchorName=document.querySelector("[data-anchor-name]");anchorName.value="contacts";anchorName.dispatchEvent(new Event("input",{bubbles:true}))');
  assert.equal(await evaluate('document.querySelector("[data-anchor-link]").value'),'#section-contacts');
  await evaluate('document.querySelector("[data-anchor-apply]").click()');
  assert(await evaluate('!!document.querySelector(".prose-editor #section-contacts")'),'anchor not applied');
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return JSON.parse(r.body).document.includes('"anchor":"contacts"');},'anchor autosaved');
  const anchoredPreview=await admin.request('/admin/pages/'+page.id+'/preview');assert(anchoredPreview.body.includes('id="section-contacts"'),'preview lost anchor');
  assert(!(await anon.request('/smoke-page')).body.includes('id="section-contacts"'),'draft anchor leaked');
  await until(()=>evaluate('!document.querySelector("[data-editor-fields]").disabled'),'anchor save finished');
  for(const palette of ['dark','light']) {
   await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
   await evaluate('document.querySelector("[data-editor-command=anchor]").click()');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/anchor-'+palette+'.png',Buffer.from(shot.data,'base64'));
   await evaluate('document.querySelector("[data-anchor-panel]").hidden=true');
  }

  assert(await evaluate('document.querySelector(".prose-editor").textContent.includes("Browser acceptance edit.")'));
  assert(!(await evaluate('document.querySelector(".prose-editor").textContent.includes("English browser body.")')));
  await evaluate('document.querySelector("[data-editor-ai]").open=false;document.querySelector("[data-add-language-panel]").open=true');
  assert(await evaluate('(()=>{const button=document.querySelector("[data-add-content-language]").getBoundingClientRect();const field=document.querySelector(".editor-add-language>.field").getBoundingClientRect();return button.width<field.width&&Math.abs(button.right-field.right)<2;})()'),'add-language action is not compact and aligned right');
  for(const palette of ['dark','light']) {
   await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'editor horizontal overflow');
   const screenshot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/editor-'+palette+'.png',Buffer.from(screenshot.data,'base64'));
  }


  await navigate('/admin/themes/editor?theme=minimal&file=layout.html');
  await until(()=>evaluate('!!document.querySelector(".cm-editor .cm-content")'),'source editor');
  await evaluate('document.querySelector(".cm-content").focus()');await send('Input.insertText',{text:'<p data-private-preview>Browser unsaved template</p>\n'});
  await evaluate('document.querySelector("[data-source-diff-toggle]").click()');
  assert(await evaluate('document.querySelector("[data-diff-after]").textContent.includes("Browser unsaved template")'),'source diff missing edits');
  assert(await evaluate('document.querySelector("[data-file-operation]").disabled'),'file operations accepted unsaved source');
  const previousTargets=new Set((await command('Target.getTargets')).targetInfos.map(target=>target.targetId));
  await evaluate('document.querySelector("[data-template-preview]").click()');
  const previewTarget=await until(async()=>{const targets=(await command('Target.getTargets')).targetInfos;return targets.find(target=>target.type==='page'&&!previousTargets.has(target.targetId)&&target.url.includes('/admin/themes/editor/previews/'));},'private template tab');
  const previewSession=(await command('Target.attachToTarget',{targetId:previewTarget.targetId,flatten:true})).sessionId;
  const previewHTML=await until(async()=>{const r=await command('Runtime.evaluate',{expression:'new XMLSerializer().serializeToString(document)',returnByValue:true},previewSession);return r.result.value?.includes('Browser unsaved template')&&r.result.value;},'unsaved source preview');
  assert(previewHTML.includes('/admin/themes/editor/previews/'),'preview has no private asset paths');
  await command('Target.closeTarget',{targetId:previewTarget.targetId});
  assert(!(await admin.request('/admin/themes/editor?theme=minimal&file=layout.html')).body.includes('Browser unsaved template'),'preview saved source');
  for(const palette of ['dark','light']){await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'template diff overflow');const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/template-diff-'+palette+'.png',Buffer.from(shot.data,'base64'));}
  await evaluate('document.querySelector(".cm-content").focus()');await send('Input.dispatchKeyEvent',{type:'keyDown',key:'z',code:'KeyZ',modifiers:4});await send('Input.dispatchKeyEvent',{type:'keyUp',key:'z',code:'KeyZ',modifiers:0});
  await until(()=>evaluate('!document.querySelector("[data-code-source]").value.includes("Browser unsaved template")'),'source undo');
  const fileOperation=async(name,operation)=>{await evaluate(`(()=>{const field=document.querySelector('[name=new_name]');field.value=${JSON.stringify(name)};field.dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('[name=intent][value="file-${operation}"]').click();})()`);};
  await fileOperation('assets/browser.css','create');await until(()=>evaluate('document.querySelector("[name=file]")?.value==="assets/browser.css"&&!!document.querySelector(".cm-editor")'),'create file');
  await fileOperation('assets/browser-renamed.css','rename');await until(()=>evaluate('document.querySelector("[name=file]")?.value==="assets/browser-renamed.css"&&!!document.querySelector(".cm-editor")'),'rename file');
  await fileOperation('','delete');await until(()=>evaluate('window.templ.dialog.isOpen(document.getElementById("system-confirm"))'),'file delete confirmation');
  await evaluate('document.querySelector("[data-confirm-result=accept]").click()');await until(()=>evaluate('document.querySelector("[name=file]")?.value==="layout.html"&&![...document.querySelectorAll("[data-template-file]")].some(link=>link.dataset.templateFile==="assets/browser-renamed.css")'),'delete file');
  for(const path of ['/admin/account/security','/admin/seo/redirects','/admin/posts/review','/admin/tools','/admin/setup']) {
   await navigate(path);await until(()=>evaluate('document.querySelector("#main")?.textContent.length>0'),'management page');
   for(const palette of ['dark','light']) {
    await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
    assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'management desktop overflow: '+path);
    const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/management-'+path.split('/').pop()+'-'+palette+'.png',Buffer.from(shot.data,'base64'));
   }
   await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'management mobile overflow: '+path);
   await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1100,deviceScaleFactor:1,mobile:false});
  }
  const exportPage=JSON.parse((await admin.request('/admin/pages/'+page.id)).body);
  const exported=await admin.request('/admin/content/bulk',{method:'POST',data:new URLSearchParams({kind:'page',action:'export',selection:page.id+':'+exportPage.version})});assert.equal(exported.status,200,exported.body);
  const contentFile=join(work,'content.json');await writeFile(contentFile,exported.body);
  await navigate('/admin/pages');
  assert(await evaluate('document.querySelector("[data-templ-controls=content-import-dialog]").previousElementSibling.value==="export"'),'import action is not after export');
  await evaluate('document.querySelector("[data-templ-controls=content-import-dialog]").click()');
  await until(()=>evaluate('window.templ.dialog.isOpen("content-import-dialog")'),'content import dialog');
  for(const palette of ['dark','light']){
   await evaluate(`(()=>{if(document.documentElement.dataset.adminTheme!==${JSON.stringify(palette)})document.querySelector("[data-admin-theme-toggle]").click()})()`);await delay(250);
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/content-import-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  assert(await evaluate('(()=>{const r=document.querySelector("#content-import-dialog").getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight})()'),'import dialog outside mobile viewport');
  const importMobile=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/content-import-mobile.png',Buffer.from(importMobile.data,'base64'));
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1100,deviceScaleFactor:1,mobile:false});
  await send('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape'});
  await until(()=>evaluate('!window.templ.dialog.isOpen("content-import-dialog")'),'import Escape dismissal');
  await evaluate('document.querySelector("[data-templ-controls=content-import-dialog]").click()');
  await until(()=>evaluate('window.templ.dialog.isOpen("content-import-dialog")'),'reopen import dialog');
  const dom=await send('DOM.getDocument');const fileInput=await send('DOM.querySelector',{nodeId:dom.root.nodeId,selector:'[data-content-import-file]'});
  await send('DOM.setFileInputFiles',{nodeId:fileInput.nodeId,files:[contentFile]});
  await evaluate("document.querySelector('[hx-post=\"/admin/tools/content/inspect\"]').click()");
  await until(()=>evaluate('!!document.querySelector("[name=digest]")'),'content import preview');
  assert(await evaluate('document.querySelector("[data-content-import-file]").files.length===1'),'import preview lost selected file');
  await evaluate('document.querySelector("#content-import-review button[type=submit]").click()');
  await until(()=>evaluate('location.pathname==="/admin/pages"'),'content import navigation');
  await navigate('/admin/forms/'+formID+'/edit');await until(()=>evaluate('document.querySelector("[data-form-preview]")?.children.length>0'),'form builder');
  const initial=await evaluate('document.querySelectorAll("[data-form-fields]>[data-form-field]").length');await evaluate('document.querySelector("[data-add-form-field]").click()');assert.equal(await evaluate('document.querySelectorAll("[data-form-fields]>[data-form-field]").length'),initial+1);
  await navigate('/admin/pages');await until(()=>evaluate('!!document.querySelector("[data-select-all]")'),'selection control');
  assert((await evaluate('document.querySelector(".content-selection").getBoundingClientRect().width'))<70,'selection column too wide');
  await evaluate('document.querySelector("[data-select-all]").click()');assert(await evaluate('!document.querySelector("[data-bulk-action]").disabled'));
  await mkdir('artifacts',{recursive:true});const desktop=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/browser-desktop.png',Buffer.from(desktop.data,'base64'));
  await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'mobile horizontal overflow');const mobile=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/browser-mobile.png',Buffer.from(mobile.data,'base64'));
  assert.deepEqual(exceptions,[],'browser JavaScript exceptions');await command('Browser.close');
 }

 const working=await admin.request('/admin/pages/'+page.id);assert.equal(working.status,200);
 const workingSnapshot=JSON.parse(working.body);
 cli(['backup','create','--data',data,'--output',snapshot],false);
 await stopSite();
 await startSite(data);
 assert.equal(token,'','restart reopened installation');
 assert.equal((await anon.request('/smoke-page')).body,publicBody,'restart changed publication');
 response=await admin.request('/admin/pages/'+page.id);assert.equal(response.status,200);assert.deepEqual(JSON.parse(response.body),workingSnapshot,'restart lost working revision');
 await stopSite();
 cli(['backup','create','--data',data,'--output',snapshot]);
 cli(['backup','verify','--source',snapshot]);
 cli(['backup','restore','--source',snapshot,'--data',restored]);
 cli(['backup','restore','--source',snapshot,'--data',restored],false);
 await startSite(restored);
 assert.equal(token,'','restore reopened installation');
 response=await anon.request('/smoke-page');assert.equal(response.status,200);assert.equal(response.body,publicBody,'restored publication differs');
 response=await anon.request('/uploads/'+image);assert.equal(response.status,200);assert.deepEqual(response.bytes,publicImage.bytes,'restored image differs');
 assert.equal((await anon.request('/private-smoke-page')).status,404);
 assert.equal((await admin.request('/admin/pages/'+page.id)).status,401,'restore kept old session');
 await admin.request('/login');
 response=await admin.request('/login',{method:'POST',data:new URLSearchParams({email,password})});assert.equal(response.status,303,'restored administrator cannot sign in');
 response=await admin.request('/admin/pages/'+page.id);assert.equal(response.status,200);assert.deepEqual(JSON.parse(response.body),workingSnapshot,'restore lost working revision');
 assert.equal((await admin.request('/logout',{method:'POST'})).status,303);
 assert.equal((await admin.request('/admin/pages/'+page.id)).status,401);
 console.log('PASS: protected concurrent install, sign-in, image upload, draft/preview, publish, restart, offline CLI backup/verify/restore and restored site launch; forms, search and maintenance'+(browserQA?'; browser editing/autosave, AI insertion (mock response), section anchors and mobile layout':''));
}finally{
 chromeSocket?.close();
 if(chrome&&chrome.exitCode===null&&chrome.signalCode===null){const exited=once(chrome,'exit');chrome.kill('SIGTERM');await Promise.race([exited,delay(5000)]);}
 try{await stopSite();}finally{
  if(docker){spawnSync('docker',['rm','-f',container],{stdio:'ignore'});spawnSync('docker',['volume','rm',volume],{stdio:'ignore'});}
  await rm(work,{recursive:true,force:true,maxRetries:10,retryDelay:100});
 }
}
