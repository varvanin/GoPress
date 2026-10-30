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
  const navigate=async path=>{await send('Page.navigate',{url:base+path});await until(()=>evaluate('document.readyState==="complete" && location.pathname==='+JSON.stringify(path)),'page load');};
  await navigate('/admin/updates');
  assert(await evaluate('Array.from(document.forms).some(form=>form.getAttribute("action")==="/admin/updates/check")'),'update check action missing');
  for(const palette of ['dark','light']) {
   await evaluate(`document.documentElement.dataset.adminTheme=${JSON.stringify(palette)}`);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'updates settings overflow');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/updates-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'updates mobile overflow');
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  await navigate('/admin/ai');
  for(const palette of ['dark','light']) {
   await evaluate(`document.documentElement.dataset.adminTheme=${JSON.stringify(palette)}`);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'AI settings overflow');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/ai-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await navigate(`/admin/pages/${page.id}/edit`);await until(()=>evaluate('!!document.querySelector(".tiptap[contenteditable=true],.prose-editor[contenteditable=true]")'),'Tiptap mount');
  assert(await evaluate('!!document.querySelector("[data-editor-command=bold]")'));
  await evaluate('document.querySelector(".tiptap[contenteditable=true],.prose-editor[contenteditable=true]").focus()');
  await send('Input.insertText',{text:'Browser acceptance edit. '});
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return r.status===200&&JSON.parse(r.body).document.includes('Browser acceptance edit.');},'browser edit autosaved');
  assert(!(await anon.request('/smoke-page')).body.includes('Browser acceptance edit.'),'browser edit leaked before publication');
  const language=async value=>evaluate(`(()=>{const field=document.querySelector('[name="content_language"]');window.templ.select.setValue(document.getElementById('content-language'),${JSON.stringify(value)});field.dispatchEvent(new Event('change',{bubbles:true}));return field.value;})()`);
  assert.equal(await evaluate('document.querySelector("[name=source_language]").value'),'ru');
  assert.equal(await evaluate('document.querySelectorAll("#content-language-popup [role=option]").length'),1,'an untranslated material offers unadded languages');
  await evaluate('document.querySelector(".editor-add-language").open=true;document.querySelector("[data-add-content-language]").click()');
  await until(()=>evaluate('document.querySelector("[name=content_language]").value==="en" && !!document.getElementById("content-language")'),'language added');
  assert.equal(await evaluate('document.querySelectorAll("#content-language-popup [role=option]").length'),2,'language switcher contains unadded languages');
  assert.equal(await evaluate('document.querySelector("[name=title]").value'),'');
  await evaluate('(()=>{const title=document.querySelector("[name=title]");title.value="English browser title";title.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".prose-editor[contenteditable=true]").focus();})()');
  await send('Input.insertText',{text:'English browser body.'});
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return JSON.parse(r.body).translations?.en?.document.includes('English browser body.');},'translation autosaved');
  assert(await evaluate('document.querySelector("[data-saved-preview]").href.endsWith("?lang=en")'),'autosave lost preview language');
  // Mock only the provider-facing generation response: no external API usage or paid request.
  await send('Fetch.enable',{patterns:[{urlPattern:base+'/admin/pages/ai',requestStage:'Request'}]});
  let aiRequest;
  const interceptAI=event=>{const message=JSON.parse(event.data);if(message.method!=='Fetch.requestPaused')return;aiRequest=JSON.parse(message.params.request.postData);void send('Fetch.fulfillRequest',{requestId:message.params.requestId,responseCode:200,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify({text:'AI browser text. <script>literal</script>'})).toString('base64')});};
  chromeSocket.addEventListener('message',interceptAI);
  await evaluate('document.querySelector("[data-editor-ai]").open=true;document.querySelector("[data-ai-prompt]").value="Write an introduction";document.querySelector("[data-ai-generate]").click()');
  await until(()=>evaluate('!document.querySelector("[data-ai-result]").hidden'),'AI result');
  assert.equal(aiRequest.language,'en');assert.equal(aiRequest.text,'');
  assert(!(await evaluate('document.querySelector(".prose-editor").textContent.includes("AI browser text.")')),'AI changed content without insertion');
  for(const palette of ['dark','light']) {
   await evaluate(`document.documentElement.dataset.adminTheme=${JSON.stringify(palette)}`);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'AI editor overflow');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/editor-ai-'+palette+'.png',Buffer.from(shot.data,'base64'));
  }
  await evaluate('document.querySelector("[data-ai-insert]").click()');
  assert(await evaluate('document.querySelector(".prose-editor").textContent.includes("AI browser text.")'));
  assert.equal(await evaluate('document.querySelectorAll(".prose-editor script").length'),0,'AI output interpreted as HTML');
  await until(async()=>{const r=await admin.request('/admin/pages/'+page.id);return JSON.parse(r.body).translations?.en?.document.includes('AI browser text.');},'AI insertion autosaved');
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
   await evaluate(`document.documentElement.dataset.adminTheme=${JSON.stringify(palette)}`);
   await evaluate('document.querySelector("[data-editor-command=anchor]").click()');
   const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/anchor-'+palette+'.png',Buffer.from(shot.data,'base64'));
   await evaluate('document.querySelector("[data-anchor-panel]").hidden=true');
  }

  assert(await evaluate('document.querySelector(".prose-editor").textContent.includes("Browser acceptance edit.")'));
  assert(!(await evaluate('document.querySelector(".prose-editor").textContent.includes("English browser body.")')));
  await evaluate('document.querySelector("[data-editor-ai]").open=false;document.querySelector(".editor-add-language").open=true');
  assert(await evaluate('(()=>{const button=document.querySelector("[data-add-content-language]").getBoundingClientRect();const field=document.querySelector(".editor-add-language>.field").getBoundingClientRect();return button.width<field.width&&Math.abs(button.right-field.right)<2;})()'),'add-language action is not compact and aligned right');
  for(const palette of ['dark','light']) {
   await evaluate(`document.documentElement.dataset.adminTheme=${JSON.stringify(palette)}`);
   assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'editor horizontal overflow');
   const screenshot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await mkdir('artifacts',{recursive:true});await writeFile('artifacts/editor-'+palette+'.png',Buffer.from(screenshot.data,'base64'));
  }


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
 chromeSocket?.close();chrome?.kill('SIGTERM');
 try{await stopSite();}finally{
  if(docker){spawnSync('docker',['rm','-f',container],{stdio:'ignore'});spawnSync('docker',['volume','rm',volume],{stdio:'ignore'});}
  await rm(work,{recursive:true,force:true});
 }
}
