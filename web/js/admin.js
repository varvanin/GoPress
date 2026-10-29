import {mediaDrop,mediaChoose} from './media-upload.js';
import {updateSEOSettings} from './seo-settings.js';
import {mountTemplateEditor,setTemplatePending} from './template-editor-loader.js';
import './faq.js';
import './collections.js';
import {t} from './i18n.js';
import {mountFormBuilder,editFormBuilder,updateFormBuilder} from './form-builder.js';
import {toast,showNotices,confirmAction,deletionPrompt,actionMessage} from './feedback.js';
import { editMenu,updateMenuRow } from './menus.js';
import {dirtyForms,previewTheme,selectContent,settingsSaved,freezeForms} from './admin-forms.js';
import {createAutosave} from './autosave.js';
let page = null;
const pendingForms=new WeakMap();
const csrf = () => document.querySelector('meta[name="csrf-token"]')?.content;
const notice = (message) => { const node = document.querySelector('#admin-nav-status'); if (node) node.textContent = message; };

export function mountPage(form) {
  const controller = new AbortController();
  const { signal } = controller;
 let saver;let insertAfterUpload=false;
  const state = { form, dirty: form.dataset.failed === 'true', submitting: false, uploading: false, pending: false, editor: null,
    dispose() { controller.abort(); state.editor?.destroy(); },
    setPending(pending) {
      state.pending = pending;
      const readonly = form.dataset.readonly === 'true';
      const fields = form.querySelector('[data-editor-fields]');
      if (fields) fields.disabled = pending || readonly;
      state.editor?.setEditable(!pending && !readonly, false);
      updateButtons();
      if(!pending&&state.dirty)saver?.schedule();
    },
  };
  const status = form.querySelector('#editor-status');
  const uploadStatus = form.querySelector('#upload-status');
  const buttons = form.querySelectorAll('[data-editor-submit]');
  const upload = form.querySelector('[data-image-upload]');
  const updateButtons = () => buttons.forEach((button) => { button.disabled = !state.editor || state.uploading || state.pending || form.dataset.readonly === 'true'; });
  saver=createAutosave(form,state,signal,message=>{status.textContent=message;});
 state.autosave=saver;
  const markDirty = () => { state.dirty = true; status.textContent = t('Есть несохранённые изменения.'); saver.schedule(); };
  const fieldChange=event=>{if(['title','slug','cover','term_ids','seo_title','seo_description','seo_canonical','seo_social_title','seo_social_description','seo_social_image','seo_nofollow','noindex'].includes(event.target?.name)&&(!event.target.form||event.target.form===form))markDirty();};
  form.addEventListener('input', fieldChange, { signal });
  form.addEventListener('change', fieldChange, { signal });
  form.addEventListener('submit', (event) => {
    if(saver.pending){
      event.preventDefault();event.stopImmediatePropagation();
      const submitter=event.submitter;
      saver.pending.then(saved=>{if(saved&&!signal.aborted)form.requestSubmit(submitter);});
      return;
    }

    if (!state.editor || state.uploading) {
      event.preventDefault();
      event.stopImmediatePropagation();
      status.textContent = state.uploading ? t('Дождитесь загрузки изображения.') : t('Редактор недоступен. Сохранение отключено, чтобы не потерять текст.');
      return;
    }
    form.elements.document.value = JSON.stringify(state.editor.getJSON());
    state.submitting = true;
  }, { signal, capture: true });
  import('/assets/editor.bundle.js').then(({ mountEditor }) => {
    if (signal.aborted) return;
    state.editor = mountEditor(form, markDirty);
    if (!state.dirty) status.textContent = '';
    state.setPending(state.pending);
  }).catch(() => {
    if (signal.aborted) return;
    status.textContent = t('Не удалось загрузить редактор. Сохранение отключено, чтобы не потерять текст.');
  });
  form.addEventListener('editor:upload-image',()=>{insertAfterUpload=true;upload.click();},{signal});
  upload.addEventListener('cancel',()=>{insertAfterUpload=false;},{signal});
  upload.addEventListener('change', async () => {
    const file = upload.files[0];
    if (!file) return;
    if (file.size > 8 * 1024 * 1024) { insertAfterUpload=false;upload.value='';uploadStatus.textContent = t('Изображение должно быть не больше 8 MiB.');toast(uploadStatus.textContent,'error'); return; }
    state.uploading = true; upload.disabled = true; updateButtons();
    uploadStatus.textContent = t('Изображение загружается…');
    try {
      const response = await fetch('/admin/media', {
        method: 'POST', credentials: 'same-origin', signal,
        headers: { 'X-CSRF-Token': csrf(), 'Content-Type': file.type || 'application/octet-stream' }, body: file,
      });
      if (!response.ok) throw new Error(response.status === 401 || response.status === 403 ? t('Войдите в админку заново в другой вкладке.') : t('Не удалось загрузить PNG/JPEG: до 8 MiB и 16 млн пикселей.'));
      const { name } = await response.json();
      if (signal.aborted) return;
      const selected=insertAfterUpload?(form.querySelector('input[name="cover"]:checked')?.value||''):name;
      await window.htmx.ajax('GET', '/admin/media/choices?selected=' + encodeURIComponent(selected), { target: '#media-picker', swap: 'innerHTML' });
      if(insertAfterUpload)form.dispatchEvent(new CustomEvent('editor:insert-image',{detail:{name}}));
      markDirty();
      toast(t('Изображение загружено.'));
      uploadStatus.textContent = insertAfterUpload?t('Изображение вставлено в текст.'):t('Изображение выбрано. Можно назначить обложкой или вставить в текст.');
    } catch (error) {
      if (!signal.aborted) {uploadStatus.textContent = error.message;toast(error.message,'error');}
    } finally {
      insertAfterUpload=false;
      if (!signal.aborted) { state.uploading = false; upload.disabled = false; upload.value = ''; updateButtons(); if(state.dirty)saver.schedule(); }
    }
  }, { signal });
  return state;
}

function initialize() {
  updateSEOSettings(document.querySelector('[data-seo-settings]'));
  mountTemplateEditor();
  mountFormBuilder();
  const form = document.querySelector('[data-page-editor]');
  if (page?.form !== form) { page?.dispose(); page = form ? mountPage(form) : null; }
  const section = document.querySelector('#main')?.dataset.section;
  document.querySelectorAll('[data-slot="sidebar-menu-button"][href^="/admin"]').forEach((link) => {
    const active = link.getAttribute('href') === section;
    link.toggleAttribute('data-active', active);
    if (active) link.setAttribute('aria-current', 'page'); else link.removeAttribute('aria-current');
  });
  showNotices();
}
const unsaved = () => page && (page.dirty || page.uploading);
function start() {
 document.addEventListener('click',mediaChoose);
 document.addEventListener('change',mediaChoose);
 for(const type of ['dragenter','dragover','dragleave','drop'])document.addEventListener(type,mediaDrop);
 document.addEventListener('settingsSaved',settingsSaved);
 document.addEventListener('click',editFormBuilder);
 document.addEventListener('input',updateFormBuilder);
 document.addEventListener('change',updateFormBuilder);
 document.addEventListener('change',selectContent);
 document.addEventListener("click",editMenu);
 document.addEventListener("input",updateMenuRow);
 document.addEventListener("change",updateMenuRow);
 document.addEventListener('click',previewTheme);
 for(const type of ['input','change'])document.addEventListener(type,event=>{const form=event.target.closest('[data-change-form]');if(form&&event.target.name)form.dataset.formDirty='true';});
  document.addEventListener('htmx:configRequest', (event) => {
    event.detail.headers['X-CSRF-Token'] = csrf();
    event.detail.headers.Accept = 'text/html';
  });
  document.addEventListener('htmx:confirm',event=>{
    const prompt=deletionPrompt(event.detail.triggeringEvent?.submitter||event.detail.elt,event.detail.parameters);
    const flush=event.detail.target?.id==='main'&&page?.autosave&&!page.autosave.failed&&!page.uploading&&page.editor&&(page.dirty||page.autosave.pending)&&!event.detail.elt?.hasAttribute?.('data-editor-cancel');
    if(!prompt&&!flush)return;
    event.preventDefault();
    const current=page;
    (async()=>{
      if(prompt&&!await confirmAction(prompt.message,event.detail.triggeringEvent?.submitter||event.detail.elt,prompt.label))return;
      if(!event.detail.elt.isConnected)return;
      if(flush){
        let saved=await current.autosave.flush();
        if(saved&&current.dirty)saved=await current.autosave.flush();
        if(!saved||current.dirty||page!==current)return;
      }
      event.detail.issueRequest(true);
    })();
  });
  document.addEventListener('htmx:beforeRequest', (event) => {
    if (!['main','template-file-editor','seo-panel','media-inspector'].includes(event.detail.target?.id)) return;
    if(dirtyForms(event.detail.elt)&&!window.confirm(t('Уйти без сохранения изменений?'))){event.preventDefault();return;}
    if (unsaved() && (!page.form.contains(event.detail.elt)||event.detail.elt?.hasAttribute?.('data-editor-cancel')) && !window.confirm(t('Уйти без сохранения изменений?'))) { event.preventDefault(); return; }
    setTemplatePending(true);
    // Freeze the sent revision so a delayed response cannot overwrite newly typed text.
    if (page) { page.submitting = false; page.setPending(true); }
    pendingForms.set(event.detail.xhr,freezeForms(event.detail.target));
    notice(t('Загрузка…'));
  });
  document.addEventListener('htmx:beforeSwap', (event) => {
    if (['main','seo-panel'].includes(event.detail.target?.id) && [400, 409].includes(event.detail.xhr.status) && event.detail.xhr.responseText.includes('id="'+event.detail.target.id+'"')) {
      event.detail.shouldSwap = true;
      event.detail.isError = false;
    }
  });
  document.addEventListener('htmx:beforeCleanupElement', (event) => {
    if (event.detail.elt === page?.form) { page.dispose(); page = null; }
  });
  document.addEventListener('htmx:afterSwap', (event) => {
    if(event.detail.target?.id==='media-inspector'){
      const selected=document.querySelector('#media-inspector input[name="name"]')?.value;
      document.querySelectorAll('[data-media-name]').forEach(tile=>{
        const active=tile.dataset.mediaName===selected;
        tile.classList.toggle('is-selected',active);
        if(active)tile.setAttribute('aria-current','page');else tile.removeAttribute('aria-current');
      });
    }
    if(event.detail.target?.id==='template-file-editor')mountTemplateEditor();
    if(event.detail.target?.id==='seo-panel'){updateSEOSettings(document.querySelector('[data-seo-settings]'));showNotices();}
    if (event.detail.target?.id === 'main') { initialize(); document.querySelector('#main')?.focus({ preventScroll: true }); }
  });
  document.addEventListener('htmx:historyRestore', initialize);
  // History cache misses use HTMX's separate XHR path, which does not follow HX-Redirect.
  document.addEventListener('htmx:historyCacheMissLoad', (event) => {
    const redirect = event.detail.xhr.getResponseHeader('HX-Redirect');
    if (redirect) window.location.assign(redirect);
  });
  document.addEventListener('htmx:afterRequest', (event) => {
    pendingForms.get(event.detail.xhr)?.();
    pendingForms.delete(event.detail.xhr);
    setTemplatePending(false);
    const detail=event.detail;
    const hasNotice=/class="[^"]*\bnotice\b/.test(detail.xhr.responseText||'');
    if(!hasNotice&&!detail.xhr.getResponseHeader('HX-Redirect')){
      if(detail.failed)toast(t('Не удалось выполнить действие. Правки сохранены на экране; проверьте соединение или обновите версию материала.'),'error');
      else if(detail.successful&&detail.requestConfig?.verb?.toLowerCase()==='post')toast(actionMessage(detail.elt,detail.requestConfig.parameters));
    }
    if (['seo-panel','template-file-editor','media-inspector'].includes(event.detail.target?.id)){notice('');return;}
    if (event.detail.target?.id !== 'main') return;
    if (page) { page.submitting = false; page.setPending(false); }
    notice(event.detail.failed ? t('Не удалось выполнить действие. Правки сохранены на экране; проверьте соединение или обновите версию материала.') : '');
  });
  window.addEventListener('beforeunload', (event) => { if ((unsaved() && !page.submitting)||dirtyForms()) event.preventDefault(); });
  window.addEventListener('pagehide', (event) => { if (!event.persisted) { page?.dispose(); page = null; } });
  // Annotate HTMX history so cancelling Back/Forward restores the exact position.
  let index = window.history.state?.gpIndex ?? 0;
  const annotate = () => window.history.replaceState({ ...window.history.state, gpIndex: index }, '');
  annotate();
  // HTMX replaces history.state before pushing/restoring; preserve our position marker.
  document.addEventListener('htmx:beforeHistoryUpdate', annotate);
  document.addEventListener('htmx:historyRestore', annotate);
  document.addEventListener('htmx:pushedIntoHistory', () => { index++; annotate(); });
  document.addEventListener('htmx:replacedInHistory', annotate);
  let reversing = false;
  window.addEventListener('popstate', (event) => {
    if (reversing) { reversing = false; event.stopImmediatePropagation(); return; }
    const target = event.state?.gpIndex;
    if (target !== undefined && target !== index && (unsaved()||dirtyForms()) && !window.confirm(t('Уйти без сохранения изменений?'))) {
      event.stopImmediatePropagation(); reversing = true; window.history.go(index - target); return;
    }
    if (target !== undefined) index = target;
  }, { capture: true });
  initialize();
}
if (typeof document !== 'undefined') start();
