import assert from 'node:assert/strict';
import {spawn,spawnSync} from 'node:child_process';
import {mkdtemp,mkdir,rm,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:net';
import {randomBytes} from 'node:crypto';
import {once} from 'node:events';

const args=new Set(process.argv.slice(2));
for(const arg of args)assert(['--docker','--browser'].includes(arg),'Unknown argument: '+arg);
const docker=args.has('--docker'),browserQA=args.has('--browser');
const work=await mkdtemp(join(tmpdir(),'gopress-smoke-'));
const reservation=createServer();await new Promise((resolve,reject)=>reservation.once('error',reject).listen(0,'127.0.0.1',resolve));
const port=reservation.address().port;await new Promise(resolve=>reservation.close(resolve));
const base='http://127.0.0.1:'+port;
const password='smoke-'+randomBytes(16).toString('hex'),email='smoke@example.test';
const container='gopress-smoke-'+randomBytes(6).toString('hex');
let server,chrome,chromeSocket,logs='',token='';
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function until(check,description,timeout=15000){const end=Date.now()+timeout;let last;while(Date.now()<end){try{const result=await check();if(result)return result;}catch(error){last=error;}await delay(100);}throw new Error('Timed out: '+description+(last?' ('+last.message+')':''));}
class BrowserHTTP {
 cookies=new Map();
 async request(path,{method='GET',data,json}={}){
  const headers={Cookie:[...this.cookies].map(([name,value])=>name+'='+value).join('; ')};
  if(method!=='GET')headers['X-CSRF-Token']=this.cookies.get('csrf')||'';
  let body;
  if(json!==undefined){headers['Content-Type']='application/json';body=JSON.stringify(json);}
  else if(data){headers['Content-Type']='application/x-www-form-urlencoded';body=data.toString();}
  const response=await fetch(base+path,{method,headers,body,redirect:'manual'});
  for(const cookie of response.headers.getSetCookie()){const pair=cookie.split(';')[0];const index=pair.indexOf('=');this.cookies.set(pair.slice(0,index),pair.slice(index+1));}
  return {status:response.status,location:response.headers.get('location'),body:await response.text(),headers:response.headers};
 }
}
const field=(html,name)=>{const match=html.match(new RegExp('name="'+name+'"[^>]*value="([^"]*)"'));assert(match,'Missing field '+name);return match[1];};
try{
 if(docker){
  const run=spawnSync('docker',['run','--rm','-d','--name',container,'-p',`127.0.0.1:${port}:8080`,'gopress:ci'],{encoding:'utf8'});assert.equal(run.status,0,run.stderr);
 }else{
  server=spawn('./bin/gopress',['serve','--data',join(work,'site'),'--addr','127.0.0.1:'+port],{stdio:['ignore','ignore','pipe'],env:{...process.env,GOPRESS_SMTP_HOST:'',GOPRESS_SMTP_FROM:'',GOPRESS_SMTP_USER:'',GOPRESS_SMTP_PASSWORD:'',GOPRESS_TRUSTED_PROXIES:''}});
  server.stderr.on('data',chunk=>{logs+=chunk.toString();const match=logs.match(/installation token[^:]*: ([a-f0-9]{64})/);if(match)token=match[1];});
  server.on('error',error=>{logs+=error.message;});
 }
 await until(async()=>{if(server?.exitCode!==null&&server?.exitCode!==undefined)throw new Error('Server exited');return (await fetch(base+'/healthz')).ok;},'server health');
 if(docker){const output=spawnSync('docker',['logs',container],{encoding:'utf8'});const match=(output.stdout+output.stderr).match(/installation token[^:]*: ([a-f0-9]{64})/);assert(match,'No installation token');token=match[1];}
 await until(()=>token,'installation token');
 const admin=new BrowserHTTP();assert.equal((await admin.request('/install')).status,200);
 let response=await admin.request('/install',{method:'POST',data:new URLSearchParams({token,title:'Smoke site',base_url:base,timezone:'UTC',email,password})});assert.equal(response.status,303);
 assert.equal((await admin.request('/install')).status,409);
 await admin.request('/login');response=await admin.request('/login',{method:'POST',data:new URLSearchParams({email,password})});assert.equal(response.status,303);
 assert.equal((await admin.request('/admin')).status,200);
 const document=JSON.stringify({type:'doc',content:[{type:'paragraph',content:[{type:'text',text:'Smoke publication visible text'}]}]});
 response=await admin.request('/admin/pages',{method:'POST',json:{id:0,version:0,title:'Smoke page',slug:'smoke-page',kind:'page',document}});assert.equal(response.status,200,response.body);let page=JSON.parse(response.body);
 assert.equal((await admin.request(`/admin/pages/${page.id}/publish`,{method:'POST',json:{version:page.version}})).status,204);
 response=await admin.request('/smoke-page');assert.equal(response.status,200);assert(response.body.includes('Smoke publication visible text'));
 response=await admin.request('/search?q=Smoke');assert.equal(response.status,200);assert(response.body.includes('Smoke page'));
 page=JSON.parse((await admin.request('/admin/pages/'+page.id)).body);
 assert.equal((await admin.request('/admin/pages/'+page.id,{method:'POST',json:{...page,title:'PRIVATE DRAFT TITLE'}})).status,200);
 assert(!(await admin.request('/smoke-page')).body.includes('PRIVATE DRAFT TITLE'));
 const formData=new URLSearchParams({id:'0',version:'0',name:'Smoke contact',recipient:'owner@example.test',button:'Send',success:'Smoke submission saved',enabled:'true'});
 for(const [label,type] of [['Name','text'],['Email','email'],['Message','textarea']])for(const [key,value] of [['field_label',label],['field_type',type],['field_required','true'],['field_options','']])formData.append(key,value);
 response=await admin.request('/admin/forms/save',{method:'POST',data:formData});assert.equal(response.status,303,response.body);const formID=response.location.match(/forms\/(\d+)/)[1];
 const anon=new BrowserHTTP();const publicForm=await anon.request('/forms/'+formID);assert.equal(publicForm.status,200);
 const submitted=new URLSearchParams({csrf:anon.cookies.get('csrf'),version:field(publicForm.body,'version'),submission_key:field(publicForm.body,'submission_key'),field_0:'Visitor',field_1:'visitor@example.test',field_2:'=CSV_FORMULA()'});
 for(let i=0;i<2;i++)assert.equal((await anon.request('/forms/'+formID,{method:'POST',data:submitted})).status,303);
 response=await admin.request(`/admin/forms/${formID}/entries`);assert.equal(response.status,200);assert.equal((response.body.match(/class="cn-card[^\"]*form-entry/g)||[]).length,1,'duplicate submission stored');
 response=await admin.request(`/admin/forms/${formID}/entries/export`);assert.equal(response.status,200);assert(response.body.includes("'=CSV_FORMULA()"),'CSV formula was not escaped');
 assert.equal((await anon.request('/admin/tools')).status,401);
 response=await admin.request('/admin/tools/backups/create',{method:'POST'});assert.equal(response.status,200,response.body);
 response=await admin.request('/admin/tools/maintenance',{method:'POST',data:new URLSearchParams({enabled:'true'})});assert.equal(response.status,303);assert.equal((await anon.request('/smoke-page')).status,503);
 assert.equal((await admin.request('/admin/tools/maintenance',{method:'POST',data:new URLSearchParams({enabled:'false'})})).status,303);
 if(browserQA){
  const binary=process.env.GOPRESS_BROWSER||(process.platform==='darwin'?'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome':'google-chrome');
  chrome=spawn(binary,['--headless=new','--no-sandbox','--disable-dev-shm-usage','--no-first-run','--remote-debugging-port=0','--user-data-dir='+join(work,'chrome'),'about:blank'],{stdio:['ignore','ignore','pipe']});
  let websocketURL='';chrome.stderr.on('data',chunk=>{const match=chunk.toString().match(/DevTools listening on (ws:\/\/[^\s]+)/);if(match)websocketURL=match[1];});chrome.on('error',error=>{logs+=error.message;});
  await until(()=>websocketURL,'Chrome DevTools');chromeSocket=new WebSocket(websocketURL);await once(chromeSocket,'open');
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
  await navigate(`/admin/pages/${page.id}/edit`);await until(()=>evaluate('!!document.querySelector(".tiptap[contenteditable=true],.prose-editor[contenteditable=true]")'),'Tiptap mount');
  assert(await evaluate('!!document.querySelector("[data-editor-command=bold]")'));
  await navigate('/admin/forms/'+formID+'/edit');await until(()=>evaluate('document.querySelector("[data-form-preview]")?.children.length>0'),'form builder');
  const initial=await evaluate('document.querySelectorAll("[data-form-fields]>[data-form-field]").length');await evaluate('document.querySelector("[data-add-form-field]").click()');assert.equal(await evaluate('document.querySelectorAll("[data-form-fields]>[data-form-field]").length'),initial+1);
  await navigate('/admin/pages');await until(()=>evaluate('!!document.querySelector("[data-select-all]")'),'selection control');
  assert((await evaluate('document.querySelector(".content-selection").getBoundingClientRect().width'))<70,'selection column too wide');
  await evaluate('document.querySelector("[data-select-all]").click()');assert(await evaluate('!document.querySelector("[data-bulk-action]").disabled'));
  await mkdir('artifacts',{recursive:true});const desktop=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/browser-desktop.png',Buffer.from(desktop.data,'base64'));
  await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});assert(await evaluate('document.documentElement.scrollWidth<=innerWidth'),'mobile horizontal overflow');const mobile=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});await writeFile('artifacts/browser-mobile.png',Buffer.from(mobile.data,'base64'));
  assert.deepEqual(exceptions,[],'browser JavaScript exceptions');await command('Browser.close');
 }
 console.log('PASS: clean install, authentication, publish/draft isolation, FTS search, form deduplication, CSV safety, backup and maintenance'+(browserQA?', editor, form builder and mobile browser QA':''));
}finally{
 chromeSocket?.close();chrome?.kill('SIGTERM');
 if(server){server.kill('SIGTERM');await Promise.race([once(server,'exit'),delay(10000)]);if(server.exitCode===null)server.kill('SIGKILL');}
 if(docker)spawnSync('docker',['rm','-f',container],{stdio:'ignore'});
 await rm(work,{recursive:true,force:true});
}
