'use strict';
(() => {
 const $ = s => document.querySelector(s);
 const club = window.MCH;
 let items=[],category='all',mediaFilter='all',expanded=false,photoURL='',videoURL='',activeUpload=null,saving=false,loading=false,toastTimer;
 const icon = name => `<svg aria-hidden="true"><use href="#${name}"/></svg>`;
 function makeCard(item) {
  const button=document.createElement('button');button.type='button';button.className='media-card';button.setAttribute('aria-label',`${item.type==='photo'?'Bekijk foto':'Bekijk video'}: ${item.title}`);
  const visual=document.createElement('div');visual.className='card-image';
  if(!item.video){const img=document.createElement('img');img.className='training-visual';img.alt='';img.loading='lazy';img.decoding='async';img.src=item.photo||`https://i.ytimg.com/vi/${item.youtube}/hqdefault.jpg`;visual.append(img);}else{visual.classList.add("uploaded-video");const label=document.createElement("span");label.textContent="MACE CLUB / VIDEO";visual.append(label);}
  if(item.type==='video'){const play=document.createElement('span');play.className='play-circle';play.innerHTML=icon('play');visual.append(play);}
  const body=document.createElement('div');body.className='card-body';const meta=document.createElement('div');meta.className='card-meta';
  const tag=document.createElement('span');tag.textContent=item.category||(item.type==='photo'?'FOTO':'TRAININGSVIDEO');const date=document.createElement('span');date.textContent=new Date(item.createdAt).toLocaleDateString('nl-NL',{day:'numeric',month:'short',year:'numeric'});meta.append(tag,date);
  const h=document.createElement('h3');h.textContent=item.title;const p=document.createElement('p');p.textContent=item.description;body.append(meta,h,p);button.append(visual,body);button.addEventListener('click',()=>openMedia(item));return button;
 }
 function render(){
  const ex=items.filter(i=>i.section==='exercise'&&(category==='all'||i.category===category));const tr=items.filter(i=>i.section==='training'&&(mediaFilter==='all'||i.type===mediaFilter));
  $('#exercise-grid').replaceChildren(...ex.slice(0,expanded?ex.length:3).map(makeCard));$('#training-grid').replaceChildren(...tr.map(makeCard));
  $('#exercise-empty').hidden=ex.length>0;$('#training-empty').hidden=tr.length>0;
  $('#more-exercises').hidden=ex.length<=3;$('#more-exercises').textContent=expanded?'Minder oefeningen tonen ↑':'Alle oefeningen bekijken ↗';
  document.querySelectorAll('.member-only').forEach(el=>el.hidden=!club.canEdit);
  $('#cta-add').innerHTML=(club.canEdit?'Video of foto toevoegen':club.user?'Mijn account':'Inloggen als clublid')+icon('arrow');
 }
 async function refreshLibrary(){if(loading)return;loading=true;try{const data=await club.api('/api/media');items=data.items;club.revision=data.revision;$('#library-status').textContent='';render();}catch{ $('#library-status').textContent='De bibliotheek kon niet worden geladen. We proberen het zo opnieuw.';}finally{loading=false;}}
 club.refreshLibrary=refreshLibrary;
 window.addEventListener('club-auth',()=>{if(!club.canEdit&&$('#add-dialog').open&&!saving)$('#add-dialog').close();render();});
 for(const attr of ['category','media'])document.querySelectorAll(`[data-${attr}]`).forEach(b=>b.addEventListener('click',()=>{document.querySelectorAll(`[data-${attr}]`).forEach(el=>{el.classList.toggle('active',el===b);el.setAttribute('aria-pressed',String(el===b));});if(attr==='category'){category=b.dataset.category;expanded=true;}else mediaFilter=b.dataset.media;render();}));
 $('#more-exercises').addEventListener('click',()=>{expanded=!expanded;render();});
 $('#cta-add').addEventListener('click',()=>club.canEdit?openAdd('training'):club.user?club.showAccount():club.openLogin());
 document.querySelectorAll('[data-add]').forEach(b=>b.addEventListener('click',()=>openAdd(b.dataset.add)));
 // Auth code owns the shared close buttons; form cancel needs the same guard.
 $('[data-close].text-button')?.addEventListener('click',()=>{if(!saving)$('#add-dialog').close();});
 function syncMediaType(){
  const kind=$('#media-type').value;const photo=kind==='photo',upload=kind==='upload';
  $('#url-field').hidden=photo||upload;$('#photo-field').hidden=!photo;$('#video-file-field').hidden=!upload;
  $('#video-url').required=!photo&&!upload;$('#photo-file').required=photo;$('#video-file').required=upload;
  if(!upload)$('#video-preview').pause();
 }
 function clearVideoPreview(){const v=$('#video-preview');v.pause();v.removeAttribute('src');v.load();v.hidden=true;if(videoURL)URL.revokeObjectURL(videoURL);videoURL='';}
 function openAdd(section){
  if(!club.canEdit)return;document.querySelectorAll('dialog[open]').forEach(d=>d.close());$('#content-form').reset();$('#section-value').value=section;$('#form-error').textContent='';$('#photo-preview').hidden=true;clearVideoPreview();$('#upload-status').hidden=true;
  if(photoURL){URL.revokeObjectURL(photoURL);photoURL='';}
  $('#add-title').textContent=section==='exercise'?'OEFENING TOEVOEGEN.':'TRAININGSBEELDEN TOEVOEGEN.';$('#add-description').textContent=section==='exercise'?'Deel een YouTube-link of upload je eigen oefenvideo.':'Deel een YouTube-video, eigen video of foto van jullie training.';
  $('#media-type-field').hidden=false;$('#media-type option[value=photo]').hidden=section==='exercise';$('#media-type option[value=photo]').disabled=section==='exercise';$('#category-field').hidden=section!=='exercise';syncMediaType();$('#add-dialog').showModal();
 }
 $('#media-type').addEventListener('change',syncMediaType);
 $('#photo-file').addEventListener('change',()=>{
  if(photoURL)URL.revokeObjectURL(photoURL);photoURL='';$('#photo-preview').hidden=true;$('#form-error').textContent='';const file=$('#photo-file').files[0];if(!file)return;
  if(!['image/jpeg','image/png','image/webp'].includes(file.type)||file.size>10*1024*1024){$('#form-error').textContent='Kies een JPG-, PNG- of WebP-foto van maximaal 10 MB.';$('#photo-file').value='';return;}
  photoURL=URL.createObjectURL(file);$('#photo-preview').src=photoURL;$('#photo-preview').hidden=false;
 });
 $('#video-file').addEventListener('change',()=>{
  clearVideoPreview();$('#form-error').textContent='';const file=$('#video-file').files[0];if(!file)return;
  if(!/\.mp4$/i.test(file.name)||file.size>90000000||file.size===0){$('#form-error').textContent='Kies een MP4-video van maximaal 90 MB.';$('#video-file').value='';return;}
  videoURL=URL.createObjectURL(file);$('#video-preview').src=videoURL;$('#video-preview').hidden=false;
 });
 function uploadVideo(body){return new Promise((resolve,reject)=>{
  const xhr=new XMLHttpRequest();activeUpload=xhr;xhr.open('POST','/api/videos');xhr.timeout=600000;
  $('#upload-status').hidden=false;$('#upload-progress').value=0;$('#upload-message').textContent='Upload starten…';$('#cancel-upload').disabled=false;
  xhr.upload.onprogress=e=>{if(e.lengthComputable){const percent=Math.min(100,Math.round(e.loaded/e.total*100));$('#upload-progress').value=percent;$('#upload-message').textContent=percent===100?'Controleren en opslaan…':`Uploaden: ${percent}%`;}};
  xhr.onload=()=>{let data;try{data=JSON.parse(xhr.responseText);}catch{}if(xhr.status>=200&&xhr.status<300)resolve(data);else reject(new Error(data?.error||(xhr.status===413?'De video is te groot. Maximaal 90 MB.':'Uploaden is niet gelukt. Probeer opnieuw.')));};
  xhr.onerror=()=>reject(new Error('De verbinding is onderbroken. Probeer opnieuw.'));
  xhr.ontimeout=()=>reject(new Error('De upload duurde te lang. Probeer een kleiner bestand of een snellere verbinding.'));
  xhr.onabort=()=>reject(new Error('Upload geannuleerd.'));xhr.send(body);
 });}
 $('#cancel-upload').addEventListener('click',()=>activeUpload?.abort());
 $('#content-form').addEventListener('submit',async e=>{
  e.preventDefault();if(saving||!club.canEdit)return;
  const section=$('#section-value').value;const photo=section==='training'&&$('#media-type').value==='photo',upload=$('#media-type').value==='upload';let body,headers={};
  const data={section,title:$('#content-title').value.trim(),description:$('#content-description').value.trim()};
  if(photo||upload){body=new FormData();for(const [k,v] of Object.entries(data))body.append(k,v);if(upload){body.append('category',section==='exercise'?$('#content-category').value:'');body.append('video',$('#video-file').files[0]);}else body.append('photo',$('#photo-file').files[0]);}
  else{body=JSON.stringify({...data,category:section==='exercise'?$('#content-category').value:'',url:$('#video-url').value.trim()});headers={'Content-Type':'application/json'};}
  saving=true;$('#form-error').textContent='';$('#content-form button[type=submit]').textContent='Opslaan…';$('#add-dialog').querySelectorAll('button,input,select,textarea').forEach(el=>el.disabled=true);
  try{if(upload)await uploadVideo(body);else await club.api('/api/media',{method:'POST',headers,body});$('#add-dialog').close();category='all';mediaFilter='all';expanded=true;document.querySelectorAll('[data-category],[data-media]').forEach(el=>{const on=el.dataset.category==='all'||el.dataset.media==='all';el.classList.toggle('active',on);el.setAttribute('aria-pressed',String(on));});await refreshLibrary();document.getElementById(section==='exercise'?'oefeningen':'trainingen').scrollIntoView({behavior:'smooth'});toast('Opgeslagen. Je toevoeging staat op de website.');}
  catch(error){$('#form-error').textContent=error.message;await club.refreshSession();}
  finally{activeUpload=null;saving=false;$('#upload-status').hidden=true;$('#add-dialog').querySelectorAll('button,input,select,textarea').forEach(el=>el.disabled=false);$('#media-type option[value=photo]').disabled=$('#section-value').value==='exercise';$('#content-form button[type=submit]').textContent='Toevoegen ↗';}
 });
 $('#add-dialog').addEventListener('cancel',e=>{if(saving)e.preventDefault();});
 $('#add-dialog').addEventListener('close',()=>{clearVideoPreview();if(photoURL){URL.revokeObjectURL(photoURL);photoURL='';} $('#photo-preview').hidden=true;});
 function toast(message){$('#toast').textContent=message;$('#toast').hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>$('#toast').hidden=true,5000);}
 function openMedia(item){
  const stage=$('#media-stage');stage.replaceChildren();$('#media-title').textContent=item.title;$('#media-description').textContent=item.description;$('#media-label').textContent=item.type==='photo'?'HET CLUBALBUM':'MACE CLUB / VIDEOBIBLIOTHEEK';$('#youtube-link').hidden=!item.youtube;
  if(item.youtube){$('#youtube-link').href='https://www.youtube.com/watch?v='+item.youtube;const frame=document.createElement('iframe');frame.src='https://www.youtube-nocookie.com/embed/'+item.youtube;frame.title=item.title;frame.allow='accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture';frame.allowFullscreen=true;frame.referrerPolicy='strict-origin-when-cross-origin';stage.append(frame);}
  else if(item.video){const video=document.createElement('video');video.controls=true;video.playsInline=true;video.preload='metadata';video.src=item.video;video.setAttribute('aria-label',item.title);video.addEventListener('error',()=>{$('#media-description').textContent='Deze video kan niet worden afgespeeld. Controleer of het bestand H.264-video en AAC-geluid gebruikt.';});stage.append(video);}
  else{const img=document.createElement('img');img.src=item.photo;img.alt=item.title;stage.append(img);}
  $('#media-dialog').showModal();
 }
 $('#media-dialog').addEventListener('close',()=>{const video=$('#media-stage video');if(video){video.pause();video.removeAttribute('src');video.load();}$('#media-stage').replaceChildren();});
 render();refreshLibrary();
})();
