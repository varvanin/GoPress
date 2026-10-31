export function parseBlock(text){
 const match=/^\[(faq|form|gallery|testimonials) id=([1-9][0-9]{0,18})\]$/.exec(text);
 if(match)return {kind:match[1],id:match[2],code:text};
 const module=/^\[module id="([a-z][a-z0-9-]{0,47})"\]$/.exec(text);
 return module?{kind:'module',id:module[1],code:text}:null;
}
