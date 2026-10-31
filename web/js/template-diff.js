// A bounded contiguous hunk shows every change without a quadratic diff algorithm.
export function sourceDiff(before,after){
 if(before.length>512*1024||after.length>512*1024)throw new Error('Diff too large');
 if(before===after)return {changed:false,before:'',after:''};
 const old=before.split('\n'),next=after.split('\n');let start=0,end=0;
 while(start<old.length&&start<next.length&&old[start]===next[start])start++;
 while(end<old.length-start&&end<next.length-start&&old[old.length-1-end]===next[next.length-1-end])end++;
 const from=Math.max(0,start-3),oldEnd=Math.min(old.length,old.length-end+3),newEnd=Math.min(next.length,next.length-end+3);
 return {changed:true,start:from+1,before:old.slice(from,oldEnd).join('\n'),after:next.slice(from,newEnd).join('\n')};
}
