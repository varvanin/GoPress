import { build } from 'esbuild';
import { readFileSync, writeFileSync } from 'node:fs';

await build({entryPoints:['web/js/admin-theme.js'],bundle:true,format:'iife',minify:true,outfile:'internal/httpui/assets/admin-theme.js'});

const result = await build({
  entryPoints: ['web/js/editor.js'], bundle: true, format: 'esm', minify: true,
  legalComments: 'eof', metafile: true, outfile: 'internal/httpui/assets/editor.bundle.js',
  banner: { js: '/*! Third-party licenses: /assets/frontend-LICENSES.txt */' },
});
await build({entryPoints:['web/js/admin.js'],bundle:true,format:'esm',minify:true,external:['/assets/*'],outfile:'internal/httpui/assets/admin.js'});
const codeResult=await build({entryPoints:['web/js/template-editor.js'],bundle:true,format:'esm',minify:true,metafile:true,legalComments:'eof',outfile:'internal/httpui/assets/template-editor.js',banner:{js:'/*! CodeMirror; third-party licenses: /assets/frontend-LICENSES.txt */'}});
const packages = new Set();
for (const input of Object.keys({...result.metafile.inputs,...codeResult.metafile.inputs})) {
  if (!input.startsWith('node_modules/')) continue;
  const parts = input.slice('node_modules/'.length).split('/');
  packages.add(parts[0].startsWith('@') ? parts.slice(0, 2).join('/') : parts[0]);
}
const notices = [];
for (const name of [...packages].sort()) {
  const root = `node_modules/${name}`;
  const pkg = JSON.parse(readFileSync(`${root}/package.json`, 'utf8'));
  let license;
  for (const file of ['LICENSE', 'LICENSE.md', 'LICENSE.txt']) {
    try { license = readFileSync(`${root}/${file}`, 'utf8'); break; }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
  // A dependency without its license must stop the distribution build.
  if (!license) throw new Error(`Missing license for ${name}`);
  notices.push(`${name}@${pkg.version}\n${license.trim()}`);
}
for (const name of ['shadcn-templ','shadcn-ui','base-ui','floating-ui','lucide','tailwindcss','lobe-icons']) {
 notices.push(`${name}\n${readFileSync(`third_party/${name}/LICENSE`, 'utf8').trim()}`);
}
await build({entryPoints:['web/js/components.js'],bundle:true,format:'iife',minify:true,legalComments:'eof',outfile:'internal/httpui/assets/components.bundle.js',banner:{js:'/*! shadcn-templ sidebar runtime; licenses: /assets/frontend-LICENSES.txt */'}});
notices.push('lucide toolbar icons v1.8.0\n'+readFileSync('third_party/lucide/EDITOR-LICENSE','utf8').trim());
writeFileSync('internal/httpui/assets/frontend-LICENSES.txt', `${notices.join('\n\n---\n\n')}\n`);
console.log(`Editor and shadcn sidebar bundled; ${packages.size} npm dependency notices plus UI licenses included.`);
