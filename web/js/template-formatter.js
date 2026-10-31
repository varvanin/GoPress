import beautify from 'js-beautify/js/lib/beautifier.js';

// Go actions may contain quoted closing delimiters or multiline comments.
export function templateActions(text){
 const actions=[];
 for(let start=text.indexOf('{{');start!==-1;){
  let quote='',comment=false,end=-1;
  for(let i=start+2;i<text.length;i++){
   const ch=text[i],next=text[i+1];
   if(comment){if(ch==='*'&&next==='/'){comment=false;i++;}continue;}
   if(quote){if(ch==='\\'&&quote!=='`')i++;else if(ch===quote)quote='';continue;}
   if(ch==='/'&&next==='*'){comment=true;i++;continue;}
   if(ch==='"'||ch==="'"||ch==='`'){quote=ch;continue;}
   if(ch==='}'&&next==='}'){end=i+2;break;}
  }
  if(end===-1)throw new Error('Unclosed Go template action');
  actions.push(text.slice(start,end));start=text.indexOf('{{',end);
 }
 return actions;
}

export function formatTemplateSource(text,filename){
 if(text.length>512*1024)throw new Error('File too large');
 const options={indent_size:2,end_with_newline:true,max_preserve_newlines:2};
 if(filename.endsWith('.css'))return beautify.css(text,options);
 if(filename.endsWith('.js'))return beautify.js(text,options);
 const actions=templateActions(text);
 const formatted=beautify.html(text,options),after=templateActions(formatted);
 if(actions.length!==after.length||actions.some((action,i)=>action!==after[i]))throw new Error('Go template actions changed');
 return formatted;
}
