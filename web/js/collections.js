export function chooseCollectionImage(event){
 const choice=event.target.closest?.('[data-collection-choice],[data-collection-clear]');
 const row=choice?.closest('.collection-item');if(!row)return;
 const image=choice.dataset.collectionChoice||'';
 row.querySelector('input[name="image"]').value=image;
 const preview=row.querySelector('.collection-image-preview');
 preview.hidden=!image;if(image)preview.src='/uploads/'+image;else preview.removeAttribute('src');
 const details=row.querySelector('details');if(details)details.open=false;
 const form=row.closest('[data-change-form]');if(form)form.dataset.formDirty='true';
 row.dispatchEvent(new Event('input',{bubbles:true}));
}
if(typeof document!=='undefined')document.addEventListener('click',chooseCollectionImage);
export function searchCollectionMedia(event){
 if(event.key!=='Enter'||!event.target.matches?.('.collection-picker input[type="search"]'))return;
 event.preventDefault();globalThis.htmx?.trigger(event.target,'search');
}
if(typeof document!=='undefined')document.addEventListener('keydown',searchCollectionMedia);
