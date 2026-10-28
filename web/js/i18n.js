export function t(text) {
 const key=text.trim(),translated=globalThis.GoPressTranslations?.[key];
 if(translated===undefined)return text;
 const start=text.indexOf(key);return text.slice(0,start)+translated+text.slice(start+key.length);
}
