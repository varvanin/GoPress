import {t} from './i18n.js';
import {toast} from './feedback.js';

export function mediaDrop(event) {
  const form=event.target.closest?.('#media-upload-form');
  if(!form||!Array.from(event.dataTransfer?.types||[]).includes('Files'))return;
  event.preventDefault();
  if(event.type==='dragleave'){
    if(!form.contains(event.relatedTarget))form.classList.remove('is-dragging');
    return;
  }
  if(event.type!=='drop'){
    form.classList.add('is-dragging');
    if(event.dataTransfer)event.dataTransfer.dropEffect='copy';
    return;
  }
  form.classList.remove('is-dragging');
  if(form.classList.contains('htmx-request'))return;
  const files=event.dataTransfer.files;
  if(files.length!==1){toast(t('Перетащите одно изображение.'),'error');return;}
  if(!['image/png','image/jpeg'].includes(files[0].type)||files[0].size>8*1024*1024){
    toast(t('Не удалось загрузить изображение. Нужен PNG или JPEG до 8 MiB и 16 миллионов пикселей.'),'error');return;
  }
  const input=form.querySelector('input[type="file"]');
  input.files=files;
  form.requestSubmit();
}

export function mediaChoose(event) {
  if(event.type==='click'){
    const button=event.target.closest?.('[data-media-choose]');
    if(button&&!button.form.classList.contains('htmx-request'))button.form.querySelector('input[type="file"]').click();
  }else if(event.target.id==='library-file'&&event.target.files?.length){
    const form=event.target.form;
    if(form&&!form.classList.contains('htmx-request'))form.requestSubmit();
  }
}
