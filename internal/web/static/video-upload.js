'use strict';
((root)=>{
 const maxBytes=1000000000;
 const mb=n=>(n/1000000).toLocaleString('nl-NL',{maximumFractionDigits:1})+' MB';
 function validationError(file){
  if(!file)return 'Kies een video.';
  if(file.size===0)return 'Dit bestand is leeg. Kies een andere video.';
  if(file.size>maxBytes)return `Dit bestand is ${mb(file.size)}. De maximale grootte is 1 GB.`;
  if(!/\.(mp4|mov)$/i.test(file.name))return 'Kies een MP4- of MOV-video. Dit bestand heeft een ander formaat.';
  return '';
 }
 function create(){
  let cancelled=false,xhr=null;
  const abort=()=>{cancelled=true;xhr?.abort();};
  function request(method,url,body,headers={},progress){return new Promise((resolve,reject)=>{
   if(cancelled){reject(new Error('Upload geannuleerd.'));return;}
   xhr=new XMLHttpRequest();xhr.open(method,url);xhr.timeout=120000;
   for(const [name,value]of Object.entries(headers))xhr.setRequestHeader(name,value);
   if(progress)xhr.upload.onprogress=e=>{if(e.lengthComputable)progress(e.loaded);};
   const fail=(message,status=0)=>reject(Object.assign(new Error(message),{status}));
   xhr.onload=()=>{let data;try{data=JSON.parse(xhr.responseText);}catch{}
    if(xhr.status>=200&&xhr.status<300)resolve(data);else fail(data?.error||'Uploaden is niet gelukt. Probeer opnieuw.',xhr.status);
   };
   xhr.onerror=()=>fail('De verbinding is onderbroken. Probeer opnieuw.');
   xhr.ontimeout=()=>fail('De verbinding is te traag of onderbroken. Probeer opnieuw.');
   xhr.onabort=()=>fail('Upload geannuleerd.');xhr.send(body);
  });}
  async function retry(action){for(let attempt=0;;attempt++){try{return await action();}catch(error){if(cancelled||attempt>=2||(error.status&&error.status<500))throw error;await new Promise(resolve=>setTimeout(resolve,1000*(attempt+1)));}}}
  async function start(file,metadata,progress){
   const error=validationError(file);if(error)throw new Error(error);
   const id=Array.from(crypto.getRandomValues(new Uint8Array(16)),b=>b.toString(16).padStart(2,'0')).join('');
   const url='/api/video-uploads/'+id;
   try{
    progress(0,'Upload voorbereiden…',false);
    const upload=await retry(()=>request('POST','/api/video-uploads',JSON.stringify({...metadata,filename:file.name,size:file.size}),{'Content-Type':'application/json','Upload-ID':id}));
    let offset=upload.offset;
    if(!Number.isSafeInteger(offset)||offset<0||offset>file.size||!Number.isSafeInteger(upload.chunkSize)||upload.chunkSize<1||upload.chunkSize>8*1024*1024)throw new Error('Ongeldig antwoord van de uploadserver.');
    while(offset<file.size){
     const chunk=file.slice(offset,Math.min(file.size,offset+upload.chunkSize));
     const update=loaded=>{const sent=Math.min(file.size,offset+loaded);progress(Math.floor(sent/file.size*100),`${mb(sent)} van ${mb(file.size)} geüpload`,false);};
     const response=await retry(()=>request('PUT',url,chunk,{'Content-Type':'application/octet-stream','Upload-Offset':String(offset)},update));
     if(response.offset!==offset+chunk.size)throw new Error('Het uploaddeel is niet volledig bevestigd. Probeer opnieuw.');
     offset=response.offset;update(0);
    }
    progress(100,'Controleren en opslaan…',true);
    let result=await retry(()=>request('POST',url+'/complete',null));
    const deadline=Date.now()+35*60*1000;
    while(result.state==='processing'){
     if(Date.now()>deadline)throw new Error('Het verwerken duurt te lang. Probeer een kortere video.');
     progress(100,'Video wordt omgezet voor de website. Dit kan enkele minuten duren…',false);
     await new Promise(resolve=>setTimeout(resolve,2000));result=await retry(()=>request('GET',url,null));
    }
    if(result.state!=='complete')throw new Error(result.error||'De video kon niet worden verwerkt.');
    return result.item;
   }catch(error){
    // A lost connection is cleaned up by the server timeout as well.
    fetch(url,{method:'DELETE',signal:AbortSignal.timeout(5000)}).catch(()=>{});
    throw error;
   }
  }
  return {start,abort};
 }
 root.MCHVideoUpload={create,validationError,maxBytes,mb};
})(typeof window==='undefined'?globalThis:window);
