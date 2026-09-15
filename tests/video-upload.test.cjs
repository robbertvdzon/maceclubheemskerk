const test=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');const {webcrypto}=require('node:crypto');
const source=fs.readFileSync('internal/web/static/video-upload.js','utf8');
function load(extra={}){const context=vm.createContext({console,crypto:webcrypto,setTimeout:(f)=>setTimeout(f,0),Uint8Array,AbortSignal,fetch:async()=>({}),...extra});vm.runInContext(source,context);return context.MCHVideoUpload;}
test('phone video of 223.8 MB is accepted and oversize error states actual size',()=>{const api=load();assert.equal(api.validationError({name:'phone.mp4',size:223777680}),'');assert.equal(api.validationError({name:'phone.MOV',size:223777680}),'');assert.match(api.validationError({name:'phone.mp4',size:1100000000}),/1\.100 MB.*1 GB/);assert.match(api.validationError({name:'empty.mp4',size:0}),/leeg/);});
test('large file is split into bounded requests, retries lost chunk response and waits for conversion',async()=>{
 const chunks=[];let offset=0,lost=false,polls=0;
 class XHR{constructor(){this.upload={};this.headers={};}open(m,u){this.method=m;this.url=u;}setRequestHeader(k,v){this.headers[k]=v;}send(body){queueMicrotask(()=>{let data={};this.status=200;
 if(this.url==='/api/video-uploads'){data={id:'ignored',offset:0,chunkSize:8*1024*1024};}
 else if(this.method==='PUT'){const start=Number(this.headers['Upload-Offset']);chunks.push(body.size);assert.ok(body.size<=8*1024*1024);offset=Math.max(offset,start+body.size);if(!lost){lost=true;this.onerror();return;}data={offset};this.upload.onprogress?.({lengthComputable:true,loaded:body.size});}
 else if(this.url.endsWith('/complete'))data={state:'processing'};
 else {polls++;data={state:'complete',item:{id:1}};}
 this.responseText=JSON.stringify(data);this.onload();});}abort(){this.onabort?.();}}
 const api=load({XMLHttpRequest:XHR});const file={name:'phone.mp4',size:223777680,slice(start,end){return {size:end-start}}};const messages=[];const result=await api.create().start(file,{title:'test'},(p,m)=>messages.push(m));assert.equal(result.id,1);assert.equal(offset,file.size);assert.ok(chunks.length>26);assert.equal(polls,1);assert.ok(messages.some(m=>m.includes('omgezet')));
});
