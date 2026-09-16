'use strict';
((root)=>{
 const format=t=>{const ms=Math.round(t*1000);const seconds=((ms%60000)/1000).toFixed(3).padStart(6,'0').replace(/\.?0+$/,'').padEnd(2,'0');return Math.floor(ms/60000)+':'+seconds;};
 const parse=value=>{const text=value.trim();if(!/^\d+(?::[0-5]?\d){0,2}(?:\.\d{1,3})?$/.test(text))return NaN;return text.split(':').reduce((n,v)=>n*60+Number(v),0);};
 const api={format,parse};if(typeof module==='object'&&module.exports)module.exports=api;else root.MCHVideoTimes=api;
})(typeof window==='undefined'?globalThis:window);
