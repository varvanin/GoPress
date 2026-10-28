import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {readFile,mkdir,mkdtemp,rm,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';

const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const preview=join(root,'docs/admin-preview.html');
const html=await readFile(preview,'utf8');
const visible=html.replace(/<style\b[^>]*>[\s\S]*?<\/style>/g,'').replace(/<[^>]*>/g,'');
assert(html.includes('lang="en"')&&visible.includes('Overview'),'English admin preview is missing');
assert(!/[А-Яа-яЁё]/.test(visible),'Preview contains untranslated text');
assert(!/(?:src|href)="\/assets\//.test(html),'Preview still requires a running server');
const readmePath=join(root,'README.md');
const readme=await readFile(readmePath,'utf8');
const marker=/<!-- admin-preview:start -->[\s\S]*?<!-- admin-preview:end -->/;
assert(marker.test(readme),'README screenshot section is missing');
assert(process.argv.slice(2).every(arg=>arg==='--check'),'Supported option: --check');
if(process.argv.includes('--check')){
 console.log('English preview and README screenshot section are ready.');
}else{
 const work=await mkdtemp(join(tmpdir(),'gopress-admin-capture-'));
 try{
  const screenshot=join(work,'admin-dashboard.png');
  const browser=process.env.GOPRESS_BROWSER||(process.platform==='darwin'?'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome':'google-chrome');
  const result=spawnSync(browser,['--headless=new','--no-sandbox','--disable-dev-shm-usage','--no-first-run','--hide-scrollbars','--window-size=1600,1100','--force-device-scale-factor=2','--virtual-time-budget=2000','--user-data-dir='+join(work,'profile'),'--screenshot='+screenshot,pathToFileURL(preview).href],{encoding:'utf8',timeout:30000});
  assert(!result.error&&result.status===0,'Browser could not capture the admin: '+(result.error?.message||result.signal||result.stderr.slice(-500)));
  const png=await readFile(screenshot);
  assert(png.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10]))&&png.readUInt32BE(16)>=1600&&png.readUInt32BE(20)>=900,'Invalid screenshot');
  await mkdir(join(root,'docs/images'),{recursive:true});
  await writeFile(join(root,'docs/images/admin-dashboard.png'),png);
  await writeFile(readmePath,readme.replace(marker,'<!-- admin-preview:start -->\n## Admin interface\n\n![GoPress admin dashboard in English](docs/images/admin-dashboard.png)\n\nEnglish admin interface with demonstration content from an isolated site.\n<!-- admin-preview:end -->'));
  console.log('Created docs/images/admin-dashboard.png and added it to README.md.');
 }finally{await rm(work,{recursive:true,force:true});}
}
