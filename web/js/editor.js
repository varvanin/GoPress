import {sectionAnchors,mountAnchors} from './editor-anchors.js';
import {t} from './i18n.js';
import { Editor, Node } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import {inlineMarks,textAlignment,findText} from './editor-format.js';

export const uploadImagePath = /^\/uploads\/[a-f0-9]{64}\.(png|jpg)$/;
export function safeLink(value) {
  if (!value || value.length > 2048 || /[\s\\\u0000-\u001f\u007f]/u.test(value)) return false;
  if (value.startsWith('/') && !value.startsWith('//') || value.startsWith('#')) return true;
  try {
    const url = new URL(value);
    return ['http:', 'https:'].includes(url.protocol) && !!url.hostname && !url.username && !url.password ||
      url.protocol === 'mailto:' && /^[^@?\s]+@[^@?\s]+$/.test(url.pathname) && !url.search && !url.hash;
  } catch { return false; }
}

// The installed core supplies the image schema; all files use the existing upload path.
const Image = Node.create({
  name: 'image', group: 'block', atom: true, draggable: true,
  addAttributes() { return { src: { default: '' }, alt: { default: '' }, caption: { default: '' } }; },
  parseHTML() {
    const attrs = (element) => {
      const image = element.matches('img') ? element : element.querySelector('img');
      const src = image?.getAttribute('src') || '';
      return uploadImagePath.test(src) ? { src, alt: image.getAttribute('alt') || '', caption: image.closest('figure')?.querySelector('figcaption')?.textContent || '' } : false;
    };
    return [{ tag: 'figure', getAttrs: attrs }, { tag: 'img[src]', getAttrs: attrs }];
  },
  renderHTML({ node }) {
    const { src, alt, caption } = node.attrs;
    return ['figure', {}, ['img', { src, alt }], ...(caption ? [['figcaption', {}, caption]] : [])];
  },
});

export function mountEditor(form, onChange) {
  const editor = new Editor({
    element: form.querySelector('#rich-editor'),
    content: JSON.parse(form.elements.document.value),
    editable: form.dataset?.readonly !== 'true',
    enableContentCheck: true,
    injectCSS: false,
    extensions: [StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      link: { openOnClick: false, autolink: false, HTMLAttributes: { target: null, rel: 'noopener noreferrer', class: null }, isAllowedUri: safeLink },
      trailingNode: false,
      gapcursor: false, dropcursor: false,
    }), Image,textAlignment,sectionAnchors,...inlineMarks],
    editorProps: { attributes: { class: 'prose-editor', role: 'textbox', 'aria-label': t('Текст материала'), 'aria-multiline': 'true' } },
    onUpdate({ editor }) {
      form.elements.document.value = JSON.stringify(editor.getJSON());
      onChange();
    },
  });
  const linkPanel = form.querySelector('[data-link-panel]');
  const linkURL = form.querySelector('[data-link-url]');
  const report = (message) => { const status = form.querySelector('#editor-status'); if (status) status.textContent = message; };
  const openAnchor=mountAnchors(form,editor,report);
  const commands = {
    anchor:openAnchor,
    paragraph: () => editor.chain().focus().setParagraph().run(),
    heading1: () => editor.chain().focus().toggleHeading({ level: 1 }).run(),
    heading2: () => editor.chain().focus().toggleHeading({ level: 2 }).run(),
    heading3: () => editor.chain().focus().toggleHeading({ level: 3 }).run(),
    bold: () => editor.chain().focus().toggleBold().run(),
    italic: () => editor.chain().focus().toggleItalic().run(),
    strike: () => editor.chain().focus().toggleStrike().run(),
    underline: () => editor.chain().focus().toggleUnderline().run(),
    highlight: () => editor.chain().focus().toggleMark('highlight').run(),
    superscript: () => editor.chain().focus().toggleMark('superscript').run(),
    subscript: () => editor.chain().focus().toggleMark('subscript').run(),
    image: () => form.dispatchEvent(new Event('editor:upload-image')),
    search: () => {const panel=form.querySelector('[data-search-panel]');panel.hidden=!panel.hidden;if(!panel.hidden)form.querySelector('[data-editor-search]').focus();},
    bulletList: () => editor.chain().focus().toggleBulletList().run(),
    orderedList: () => editor.chain().focus().toggleOrderedList().run(),
    blockquote: () => editor.chain().focus().toggleBlockquote().run(),
    code: () => editor.chain().focus().toggleCode().run(),
    codeBlock: () => editor.chain().focus().toggleCodeBlock().run(),
    horizontalRule: () => editor.chain().focus().setHorizontalRule().run(),
    undo: () => editor.chain().focus().undo().run(),
    redo: () => editor.chain().focus().redo().run(),
    link: () => {
      linkPanel.hidden = !linkPanel.hidden;
      if (!linkPanel.hidden) { linkURL.value = editor.getAttributes('link').href || ''; linkURL.focus(); }
    },
  };
  for(const [command,value] of Object.entries({alignLeft:'left',alignCenter:'center',alignRight:'right',alignJustify:'justify'})){
    commands[command]=()=>editor.chain().focus().updateAttributes('paragraph',{textAlign:value}).updateAttributes('heading',{textAlign:value}).run();
  }
  const toolbar = form.querySelectorAll('[data-editor-command]');
  const updateToolbar = () => {
    toolbar.forEach((button) => {
      const command = button.dataset.editorCommand;
      const align={alignLeft:'left',alignCenter:'center',alignRight:'right',alignJustify:'justify'}[command];
      const active = command==='anchor' ? !!editor.state.selection.$from.parent.attrs.anchor : align ? editor.isActive({textAlign:align}) : command.startsWith('heading') ? editor.isActive('heading', { level: Number(command.at(-1)) }) : editor.isActive(command);
      button.setAttribute('aria-pressed', String(active));
      button.disabled = !editor.isEditable || command === 'undo' && !editor.can().undo() || command === 'redo' && !editor.can().redo();
    });
  };
  toolbar.forEach((button) => button.addEventListener('click', () => {
    commands[button.dataset.editorCommand]();
    button.closest('details')?.removeAttribute('open');
  }));
  const search=form.querySelector('[data-editor-search]');
  const next=()=>{
    const match=findText(editor.state.doc,search.value,editor.state.selection.to);
    if(!match){report(t('Совпадений не найдено.'));return;}
    editor.chain().focus().setTextSelection(match).scrollIntoView().run();
  };
  form.querySelector('[data-search-next]')?.addEventListener('click',next);
  search?.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();next();}});
  linkURL?.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();form.querySelector('[data-link-apply]').click();}});
  form.addEventListener('keydown',event=>{
    if(event.key!=='Escape')return;
    form.querySelectorAll('.editor-menu[open]').forEach(menu=>menu.removeAttribute('open'));
    if(linkPanel)linkPanel.hidden=true;
    const anchors=form.querySelector('[data-anchor-panel]');if(anchors)anchors.hidden=true;
    const panel=form.querySelector('[data-search-panel]');if(panel)panel.hidden=true;
  });
  form.querySelector('[data-link-apply]')?.addEventListener('click', () => {
    const href = linkURL.value.trim();
    if (!safeLink(href)) { report(t('Укажите http/https, mailto, /путь или #якорь.')); return; }
    editor.chain().focus().extendMarkRange('link').setLink({ href }).run();
    linkPanel.hidden = true;
  });
  form.querySelector('[data-link-remove]')?.addEventListener('click', () => {
    editor.chain().focus().extendMarkRange('link').unsetLink().run();
    linkPanel.hidden = true;
  });
  form.addEventListener('change',event=>{
    if(event.target.matches('input[name="cover"]')){
      form.querySelector('[data-image-alt]').value=event.target.dataset.mediaAlt||'';
      form.querySelector('[data-image-caption]').value=event.target.dataset.mediaCaption||'';
    }
  });
  const insertImage=(selected,metadata)=>{
    const src='/uploads/'+(selected||'');
    if(!uploadImagePath.test(src)){report(t('Загрузите или выберите изображение в панели справа.'));return;}
    editor.chain().focus().insertContent({type:'image',attrs:{
      src,alt:metadata?.alt??form.querySelector('[data-image-alt]').value,
      caption:metadata?.caption??form.querySelector('[data-image-caption]').value,
    }}).run();
  };
  form.addEventListener('click',event=>{
    const button=event.target.closest('[data-library-insert]');
    if(button)insertImage(button.dataset.libraryInsert,{alt:button.dataset.mediaAlt||'',caption:button.dataset.mediaCaption||''});
  });
  form.querySelector('[data-insert-image]')?.addEventListener('click',()=>insertImage(form.querySelector('input[name="cover"]:checked')?.value));
  form.addEventListener('editor:insert-image',event=>insertImage(event.detail.name));

  editor.on('selectionUpdate', updateToolbar);
  editor.on('transaction', updateToolbar);
  updateToolbar();
  return editor;
}
