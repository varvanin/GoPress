import {t} from './i18n.js';
// One in-flight write per editor. Version conflicts stop retries without touching local text.
export function createAutosave(form,state,signal,status) {
  let timer=null,flight=null,generation=0,blocked=false,failed=false;
  const stop=()=>{clearTimeout(timer);timer=null;};
  signal.addEventListener('abort',stop,{once:true});
  const schedule=()=>{
    generation++;failed=false;stop();
    if(!blocked&&!signal.aborted)timer=setTimeout(flush,1500);
  };
  async function flush(){
    stop();
    if(flight)return flight;
    if(blocked||signal.aborted||!state.dirty||!state.editor||state.pending||state.uploading||form.dataset.readonly==='true')return false;
    form.dispatchEvent(new Event('editor:serialize'));
    const data=new URLSearchParams(new FormData(form));
    data.set('intent','autosave');
    const sent=generation;
    status(t('Сохранение черновика…'));
    flight=(async()=>{
      let success=false;
      try {
        const response=await fetch(form.action,{
          method:'POST',credentials:'same-origin',signal,
          headers:{'Content-Type':'application/x-www-form-urlencoded','Accept':'application/json','X-CSRF-Token':form.elements.csrf.value},
          body:data,
        });
        if(!response.ok){
          if([401,403,409].includes(response.status))blocked=true;
          const error=await response.json().catch(()=>null);
          throw new Error(response.status===409?t('Конфликт версий. Автосохранение остановлено; ваши правки остались здесь. Откройте актуальную версию в другой вкладке.'):
            error?.message||t('Не удалось сохранить. Проверьте соединение или войдите заново в другой вкладке.'));
        }
        const saved=await response.json();
        if(signal.aborted)return false;
        const fresh=form.elements.id.value==='0';
        form.elements.id.value=String(saved.id);
        form.elements.version.value=String(saved.version);
        if(fresh&&form.elements.original_translation) {
          const expectedTitle=data.get('content_language')==='original'?data.get('title'):JSON.parse(data.get('original_translation')).title;
          if(!expectedTitle.trim())form.dispatchEvent(new CustomEvent('editor:original-saved',{detail:{title:saved.title,expectedTitle}}));
        } else if(fresh&&!data.get('title')?.trim()&&!form.elements.title.value.trim())form.elements.title.value=saved.title;
        if(fresh&&!data.get('slug')&&!form.elements.slug.value)form.elements.slug.value=saved.slug;
        const root=form.closest('#main')||form;
        root.querySelectorAll('input[name="version"]').forEach(input=>{input.value=String(saved.version);});
        const preview=root.querySelector('[data-saved-preview]');
        if(preview){const language=form.elements.content_language?.value;preview.href='/admin/pages/'+saved.id+'/preview'+(language&&language!=='original'?'?lang='+encodeURIComponent(language):'');preview.hidden=false;}
        const controls=root.querySelector('[data-schedule-controls]');
        if(controls)controls.hidden=false;
        const scheduleForm=root.querySelector('#schedule-form');
        if(scheduleForm)scheduleForm.action='/admin/pages/'+saved.id+'/schedule';
        if(fresh){const language=form.elements.content_language?.value;window.history.replaceState({...window.history.state},'',saved.edit_url+(language&&language!=='original'?'?lang='+encodeURIComponent(language):''));}
        state.dirty=generation!==sent;
        form.dispatchEvent(new Event('editor:autosaved'));
        success=true;
        status(state.dirty?t('Есть новые несохранённые изменения.'):t('Черновик автоматически сохранён.'));
        return true;
      }catch(error){
        if(!signal.aborted){failed=true;status(error.message);}
        return false;
      }finally{
        flight=null;
        if(success&&state.dirty&&!signal.aborted)timer=setTimeout(flush,1500);
      }
    })();
    return flight;
  }
  return {schedule,flush,get pending(){return flight;},get failed(){return failed||blocked;}};
}
