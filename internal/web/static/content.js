 'use strict';
(() => {
 const $=s=>document.querySelector(s),club=window.MCH;
 let items=[],photoURL='',videoURL='',activeUpload=null,saving=false,loading=false,toastTimer,managing=false;
 const librarySection=$('[data-library]')?.dataset.library;
 const icon=name=>`<svg aria-hidden="true"><use href="#${name}"/></svg>`;
 const titleOf=item=>item.title||(item.type==='photo'?'Foto':'Filmpje');
 function makeCard(item,index){
  const article=document.createElement('article');article.className='media-item';
  const button=document.createElement('button');button.type='button';button.className='media-card';button.setAttribute('aria-label',`Bekijk ${item.type==='photo'?'foto':'filmpje'}: ${titleOf(item)}`);
  const visual=document.createElement('div');visual.className='card-image';
  if(!item.video){const img=document.createElement('img');img.className='training-visual';img.alt='';img.loading='lazy';img.decoding='async';img.src=item.photo||`https://i.ytimg.com/vi/${item.youtube}/hqdefault.jpg`;visual.append(img);}
  else{visual.classList.add('uploaded-video');const label=document.createElement('span');label.textContent='MACE CLUB / VIDEO';visual.append(label);}
  if(item.type==='video'){const play=document.createElement('span');play.className='play-circle';play.innerHTML=icon('play');visual.append(play);}
  const body=document.createElement('div');body.className='card-body';const meta=document.createElement('div');meta.className='card-meta';meta.textContent=(item.type==='photo'?'FOTO · ':'FILMPJE · ')+new Date(item.createdAt).toLocaleDateString('nl-NL',{day:'numeric',month:'short',year:'numeric'});
  const h=document.createElement('h2');h.textContent=titleOf(item);body.append(meta,h);if(item.description){const p=document.createElement('p');p.textContent=item.description;body.append(p);}button.append(visual,body);button.addEventListener('click',()=>openMedia(item));article.append(button);
  if(club.canEdit){const tools=document.createElement('div');tools.className='media-tools';
   const action=(label,aria,callback,disabled=false)=>{const b=document.createElement('button');b.type='button';b.className='text-button';b.textContent=label;b.setAttribute('aria-label',aria);b.disabled=disabled||managing;b.addEventListener('click',callback);tools.append(b);};
   action('Tekst',`Tekst aanpassen: ${titleOf(item)}`,()=>openEdit(item));
   action('Verwijderen',`Verwijderen: ${titleOf(item)}`,()=>openDelete(item));
   action('↑',`Omhoog: ${titleOf(item)}`,()=>move(item,'up'),index===0);
   action('↓',`Omlaag: ${titleOf(item)}`,()=>move(item,'down'),index===items.length-1);article.append(tools);
  }return article;
 }
 function render(){if($('#media-grid')){$('#media-grid').replaceChildren(...items.map(makeCard));$('#media-empty').hidden=items.length>0;}document.querySelectorAll('.member-only').forEach(el=>el.hidden=!club.canEdit);}
 async function refreshLibrary(){if(loading)return;loading=true;try{const data=await club.api('/api/media');items=data.items.filter(item=>!librarySection||item.section===librarySection);club.revision=data.revision;if($('#library-status'))$('#library-status').textContent='';render();}catch{if($('#library-status'))$('#library-status').textContent='Laden is niet gelukt. We proberen het zo opnieuw.';}finally{loading=false;}}
 club.refreshLibrary=refreshLibrary;
 window.addEventListener('club-auth',()=>{if(!club.canEdit&&!saving){for(const id of ['add-dialog','edit-dialog','delete-dialog','trash-dialog'])if($('#'+id).open)$('#'+id).close();}render();});
 document.querySelectorAll('[data-add]').forEach(b=>b.addEventListener('click',()=>openAdd(b.dataset.add)));
 $('[data-close].text-button')?.addEventListener('click',()=>{if(!saving)$('#add-dialog').close();});
 let editItem,deleteItem,editRevision,deleteRevision;
 function openEdit(item){editItem=item;editRevision=club.revision;$('#edit-media-title').value=item.title;$('#edit-description').value=item.description;$('#edit-section').value=item.section;$('#edit-section-field').hidden=item.type==='photo';$('#edit-error').textContent='';$('#edit-dialog').showModal();}
 function openDelete(item){deleteItem=item;deleteRevision=club.revision;$('#delete-name').textContent=titleOf(item);$('#delete-error').textContent='';$('#delete-dialog').showModal();}
 async function mutate(item,method,suffix,data={},revision=club.revision){return club.api('/api/media/'+item.id+suffix,{method,headers:{'Content-Type':'application/json'},body:JSON.stringify({...data,revision})});}
 $('#edit-form').addEventListener('submit',async e=>{e.preventDefault();if(managing||!club.canEdit)return;managing=true;const button=e.target.querySelector('button[type=submit]');button.disabled=true;try{await mutate(editItem,'PATCH','',{title:$('#edit-media-title').value,description:$('#edit-description').value,section:editItem.type==='video'?$('#edit-section').value:'training'},editRevision);$('#edit-dialog').close();await refreshLibrary();toast('Tekst opgeslagen.');}catch(error){$('#edit-error').textContent=error.message;}finally{managing=false;button.disabled=false;render();}});
 $('#confirm-delete').addEventListener('click',async()=>{if(managing||!club.canEdit)return;managing=true;$('#confirm-delete').disabled=true;try{await mutate(deleteItem,'DELETE','',{},deleteRevision);$('#delete-dialog').close();await refreshLibrary();toast('Verplaatst naar de prullenbak.');}catch(error){$('#delete-error').textContent=error.message;}finally{managing=false;$('#confirm-delete').disabled=false;render();}});
 async function move(item,direction){if(managing||!club.canEdit)return;managing=true;render();try{await mutate(item,'POST','/move',{direction});await refreshLibrary();}catch(error){toast(error.message);await refreshLibrary();}finally{managing=false;render();const index=items.findIndex(i=>i.id===item.id);$('#media-grid')?.children[index]?.querySelector(direction==='up'?'button[aria-label^="Omhoog"]':'button[aria-label^="Omlaag"]')?.focus();}}
 async function loadTrash(){try{await refreshLibrary();const data=await club.api('/api/media/trash');$('#trash-items').replaceChildren();if(!data.items.some(item=>!librarySection||item.section===librarySection))$('#trash-items').textContent='De prullenbak is leeg.';for(const item of data.items.filter(item=>!librarySection||item.section===librarySection)){const row=document.createElement('div');row.className='trash-row';const text=document.createElement('span');text.textContent=titleOf(item);const b=document.createElement('button');b.className='button outline';b.textContent='Terugzetten';b.addEventListener('click',async()=>{b.disabled=true;try{await mutate(item,'POST','/restore');await loadTrash();}catch(error){$('#trash-error').textContent=error.message;b.disabled=false;}});row.append(text,b);$('#trash-items').append(row);}}catch(error){$('#trash-error').textContent=error.message;}}
 $('#open-trash')?.addEventListener('click',()=>{$('#trash-error').textContent='';$('#trash-dialog').showModal();loadTrash();});
 function syncMediaType(){
  const kind=$('#media-type').value;const photo=kind==='photo',upload=kind==='upload';
  $('#url-field').hidden=photo||upload;$('#photo-field').hidden=!photo;$('#video-file-field').hidden=!upload;
  $('#video-url').required=!photo&&!upload;$('#photo-file').required=photo;$('#video-file').required=upload;$('#video-file').disabled=!upload;$('#photo-file').disabled=!photo;$('#video-url').disabled=photo||upload;
  if(!upload)$('#video-preview').pause();
 }
 function clearVideoPreview(){const v=$('#video-preview');v.pause();v.removeAttribute('src');v.load();v.hidden=true;if(videoURL)URL.revokeObjectURL(videoURL);videoURL='';}
 function openAdd(section){
  if(!club.canEdit)return;document.querySelectorAll('dialog[open]').forEach(d=>d.close());$('#content-form').reset();$('#section-value').value=section;$('#form-error').textContent='';$('#photo-preview').hidden=true;clearVideoPreview();$('#video-file-info').textContent='';$('#video-file-error').textContent='';$('#video-file').setCustomValidity('');$('#video-file').setAttribute('aria-invalid','false');$('#upload-status').hidden=true;
  if(photoURL){URL.revokeObjectURL(photoURL);photoURL='';}
  const exercise=section==='exercise';$('#media-type').querySelector('option[value=photo]').disabled=exercise;$('#media-type').querySelector('option[value=photo]').hidden=exercise;$('#add-title').textContent=exercise?'OEFENING TOEVOEGEN.':'FOTO OF FILMPJE TOEVOEGEN.';$('#add-description').textContent=exercise?'Voeg een YouTube-video of een eigen filmpje met een oefening toe. Tekst is optioneel.':'Kies een video of foto. Tekst toevoegen mag, maar hoeft niet.';$('#content-title').placeholder=exercise?'Bijvoorbeeld: 360 swing':'Bijvoorbeeld: onze training samen';
  $('#media-type-field').hidden=false;syncMediaType();$('#add-dialog').showModal();
 }
 $('#media-type').addEventListener('change',syncMediaType);
 $('#photo-file').addEventListener('change',()=>{
  if(photoURL)URL.revokeObjectURL(photoURL);photoURL='';$('#photo-preview').hidden=true;$('#form-error').textContent='';const file=$('#photo-file').files[0];if(!file)return;
  if(!['image/jpeg','image/png','image/webp'].includes(file.type)||file.size>10*1024*1024){$('#form-error').textContent='Kies een JPG-, PNG- of WebP-foto van maximaal 10 MB.';$('#photo-file').value='';return;}
  photoURL=URL.createObjectURL(file);$('#photo-preview').src=photoURL;$('#photo-preview').hidden=false;
 });
 $('#video-file').addEventListener('change',()=>{
  clearVideoPreview();$('#form-error').textContent='';const input=$('#video-file'),file=input.files[0];
  const error=file?window.MCHVideoUpload.validationError(file):'';
  input.setCustomValidity(error);input.setAttribute('aria-invalid',String(Boolean(error)));
  $('#video-file-error').textContent=error;$('#video-file-info').textContent=file?`${file.name} · ${window.MCHVideoUpload.mb(file.size)}`:'';
  if(!file||error)return;
  videoURL=URL.createObjectURL(file);$('#video-preview').src=videoURL;$('#video-preview').hidden=false;
 });
 async function uploadVideo(file,data){
  activeUpload=window.MCHVideoUpload.create();$('#upload-status').hidden=false;$('#cancel-upload').disabled=false;
  return activeUpload.start(file,data,(percent,message,finishing)=>{
   $('#upload-progress').value=percent;$('#upload-message').textContent=message;$('#cancel-upload').disabled=finishing;
  });
 }
 $('#video-preview').addEventListener('error',()=>{if($('#video-file').files[0]&&!$('#video-file').validationMessage){$('#video-file-info').textContent+=' · Voorvertoning niet beschikbaar; je kunt de video wel uploaden en laten omzetten.';$('#video-preview').hidden=true;}});
 $('#cancel-upload').addEventListener('click',()=>activeUpload?.abort());
 $('#content-form').addEventListener('submit',async e=>{
  e.preventDefault();if(saving||!club.canEdit)return;
  const section=$('#section-value').value;const photo=section==='training'&&$('#media-type').value==='photo',upload=$('#media-type').value==='upload';let body,headers={};
  const data={section,title:$('#content-title').value.trim(),description:$('#content-description').value.trim()};
  if(photo||upload){body=new FormData();for(const [k,v] of Object.entries(data))body.append(k,v);if(upload){body.append('category','');body.append('video',$('#video-file').files[0]);}else body.append('photo',$('#photo-file').files[0]);}
  else{body=JSON.stringify({...data,category:'',url:$('#video-url').value.trim()});headers={'Content-Type':'application/json'};}
  saving=true;$('#form-error').textContent='';$('#content-form button[type=submit]').textContent='Opslaan…';$('#add-dialog').querySelectorAll('button,input,select,textarea').forEach(el=>el.disabled=true);
  try{if(upload)await uploadVideo($('#video-file').files[0],{...data,category:''});else await club.api('/api/media',{method:'POST',headers,body});$('#add-dialog').close();await refreshLibrary();if(librarySection!==section){location.assign(section==='exercise'?'/oefeningen':'/fotos-en-filmpjes');return;}toast('Opgeslagen. Je toevoeging staat op de website.');}
  catch(error){$('#form-error').textContent=error.message;await club.refreshSession();}
  finally{activeUpload=null;saving=false;$('#upload-status').hidden=true;$('#add-dialog').querySelectorAll('button,input,select,textarea').forEach(el=>el.disabled=false);$('#content-form button[type=submit]').textContent='Toevoegen ↗';syncMediaType();}
 });
 $('#add-dialog').addEventListener('cancel',e=>{if(saving)e.preventDefault();});
 $('#add-dialog').addEventListener('close',()=>{clearVideoPreview();if(photoURL){URL.revokeObjectURL(photoURL);photoURL='';} $('#photo-preview').hidden=true;});
 function toast(message){$('#toast').textContent=message;$('#toast').hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>$('#toast').hidden=true,5000);}
 function openMedia(item){
  const stage=$('#media-stage');stage.replaceChildren();$('#media-title').textContent=titleOf(item);$('#media-description').textContent=item.description;$('#media-label').textContent=item.type==='photo'?'HET CLUBALBUM':'MACE CLUB / VIDEOBIBLIOTHEEK';$('#youtube-link').hidden=!item.youtube;
  if(item.youtube){$('#youtube-link').href='https://www.youtube.com/watch?v='+item.youtube;const frame=document.createElement('iframe');frame.src='https://www.youtube-nocookie.com/embed/'+item.youtube;frame.title=titleOf(item);frame.allow='accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture';frame.allowFullscreen=true;frame.referrerPolicy='strict-origin-when-cross-origin';stage.append(frame);}
  else if(item.video){const video=document.createElement('video');video.controls=true;video.playsInline=true;video.preload='metadata';video.src=item.video;video.setAttribute('aria-label',titleOf(item));video.addEventListener('error',()=>{$('#media-description').textContent='Deze video kan niet worden afgespeeld. Controleer of het bestand H.264-video en AAC-geluid gebruikt.';});stage.append(video);}
  else{const img=document.createElement('img');img.src=item.photo;img.alt=titleOf(item);stage.append(img);}
  $('#media-dialog').showModal();
 }
 $('#media-dialog').addEventListener('close',()=>{const video=$('#media-stage video');if(video){video.pause();video.removeAttribute('src');video.load();}$('#media-stage').replaceChildren();});
 render();refreshLibrary();
})();
