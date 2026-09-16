'use strict';
(() => {
 const $=s=>document.querySelector(s),club=window.MCH;
 const {format,parse}=window.MCHVideoTimes;
 let item,revision,duration=0,busy=false,previewing=false,timer,epoch=0,activeJob='';
 const dialog=$('#clip-dialog'),video=$('#clip-preview'),form=$('#clip-form');
 function fieldsDisabled(value){form.querySelectorAll('input,button').forEach(e=>e.disabled=value);}
 function bounds(){return {start:parse($('#clip-start').value),end:parse($('#clip-end').value),thumbnailTime:parse($('#clip-thumbnail-time').value)};}
 function validate(){const p=bounds();if(!Number.isFinite(p.start)||!Number.isFinite(p.end)||p.start<0||p.end>duration+.001||p.end-p.start<.1)throw Error('Kies een begin en einde binnen de video, met minimaal 0,1 seconde ertussen.');if(!Number.isFinite(p.thumbnailTime)||p.thumbnailTime<p.start||p.thumbnailTime>=p.end)throw Error('Kies een thumbnail binnen het fragment.');return p;}
 function changed(which){previewing=false;video.pause();const p=bounds();if(Number.isFinite(p.start)&&Number.isFinite(p.end)){
  $('#clip-duration').textContent=p.end>p.start?'Fragment: '+format(p.end-p.start):'Het einde moet na het begin liggen.';
  if(p.thumbnailTime<p.start||p.thumbnailTime>=p.end||!Number.isFinite(p.thumbnailTime))$('#clip-thumbnail-time').value=format(p.start);
  $('#clip-start-range').value=String(p.start);$('#clip-end-range').value=String(p.end);
  if(which&&Number.isFinite(p[which]))video.currentTime=Math.min(duration-.001,Math.max(0,p[which]));
 }}
 async function status(token){try{
  const data=await club.api('/api/media/'+item.id+'/clip');if(token!==epoch||!dialog.open)return;
  busy=data.status==='processing';fieldsDisabled(busy);
  if(busy){activeJob=data.id;$('#clip-status').textContent='Het fragment wordt verwerkt. Je mag dit venster sluiten; de huidige video blijft zichtbaar tot de nieuwe klaar is.';timer=setTimeout(()=>status(token),2000);}
  else if(data.status==='error'){$('#clip-status').textContent=data.error;}
  else if(data.status==='done'&&activeJob===data.id){$('#clip-status').textContent='Opgeslagen. Het fragment en de thumbnail staan op de website.';activeJob='';await club.refreshLibrary?.();revision=club.revision;}
 }catch(error){if(token===epoch&&dialog.open){$('#clip-status').textContent='De status kon niet worden opgehaald. We proberen het opnieuw.';timer=setTimeout(()=>status(token),3000);}}}
 async function openClip(value){
  item=value;revision=club.revision;const token=++epoch;clearTimeout(timer);activeJob='';busy=false;previewing=false;$('#clip-error').textContent='';$('#clip-status').textContent='Video laden…';$('#clip-thumbnail-canvas').hidden=true;fieldsDisabled(true);dialog.showModal();
  try{const data=await club.api('/api/media/'+item.id+'/editing');if(token!==epoch||!dialog.open)return;duration=data.duration;video.src=data.source;video.load();
   const p=data.playback;$('#clip-start').value=format(p.start);$('#clip-end').value=format(p.end||duration);$('#clip-thumbnail-time').value=format(p.thumbnailTime||p.start);
   for(const id of ['clip-start-range','clip-end-range'])$('#'+id).max=String(duration);changed();fieldsDisabled(false);$('#clip-status').textContent='';await status(token);
  }catch(error){if(token===epoch)$('#clip-error').textContent=error.message;}
 }
 for(const which of ['start','end']){
  $('#clip-'+which+'-range').addEventListener('input',e=>{$('#clip-'+which).value=format(Number(e.target.value));changed(which);});
  $('#clip-'+which).addEventListener('change',()=>changed(which));
  $('#clip-use-'+which).addEventListener('click',()=>{$('#clip-'+which).value=format(video.currentTime);changed(which);});
 }
 $('#clip-preview-fragment').addEventListener('click',()=>{try{const p=validate();$('#clip-error').textContent='';video.currentTime=p.start;previewing=true;video.play().catch(()=>{});}catch(e){$('#clip-error').textContent=e.message;}});
 video.addEventListener('timeupdate',()=>{if(previewing&&video.currentTime>=bounds().end){video.pause();previewing=false;}});
 function drawThumbnail(){const canvas=$('#clip-thumbnail-canvas');if(!video.videoWidth)return;canvas.width=480;canvas.height=Math.round(480*video.videoHeight/video.videoWidth);canvas.getContext('2d').drawImage(video,0,0,canvas.width,canvas.height);canvas.hidden=false;}
 $('#clip-use-thumbnail').addEventListener('click',()=>{video.pause();previewing=false;$('#clip-thumbnail-time').value=format(video.currentTime);drawThumbnail();});
 $('#clip-thumbnail-time').addEventListener('change',()=>{const t=parse($('#clip-thumbnail-time').value);if(Number.isFinite(t)&&t>=0&&t<duration){video.pause();video.currentTime=t;video.addEventListener('seeked',drawThumbnail,{once:true});}});
 form.addEventListener('submit',async e=>{e.preventDefault();if(busy||!club.canEdit)return;const token=epoch,id=item.id;try{
  const p=validate();busy=true;fieldsDisabled(true);$('#clip-error').textContent='';$('#clip-status').textContent='Verwerking starten…';
  const data=await club.api('/api/media/'+id+'/clip',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({...p,revision})});if(token!==epoch)return;activeJob=data.id;await status(token);
 }catch(error){if(token!==epoch)return;busy=false;fieldsDisabled(false);$('#clip-error').textContent=error.message;$('#clip-status').textContent='';}});
 dialog.addEventListener('close',()=>{++epoch;clearTimeout(timer);video.pause();video.removeAttribute('src');video.load();previewing=false;});
 video.addEventListener('error',()=>{if(dialog.open&&video.getAttribute('src'))$('#clip-error').textContent='De bronvideo kon niet worden geladen. Sluit het venster en probeer opnieuw.';});
 const ytDialog=$('#youtube-edit-dialog'),rows=$('#youtube-chapter-rows');let ytItem,ytRevision,ytBusy=false,ytEpoch=0;
 function chapterRow(chapter={time:0,title:''}){
  const row=document.createElement('div');row.className='chapter-edit-row';
  const time=document.createElement('input');time.type='text';time.className='chapter-time';time.value=format(chapter.time);time.placeholder='02:10';time.setAttribute('aria-label','Tijdstip oefening');time.required=true;
  const name=document.createElement('input');name.className='chapter-name';name.value=chapter.title;name.maxLength=80;name.placeholder='Naam van de oefening';name.setAttribute('aria-label','Naam van de oefening');name.required=true;
  const remove=document.createElement('button');remove.type='button';remove.className='text-button';remove.textContent='×';remove.setAttribute('aria-label','Tijdstip verwijderen');remove.addEventListener('click',()=>row.remove());row.append(time,name,remove);rows.append(row);
 }
 function openYouTube(value){++ytEpoch;ytItem=value;ytRevision=club.revision;const p=value.playback||{};$('#youtube-start').value=format(p.start||0);$('#youtube-end').value=p.end?format(p.end):'';rows.replaceChildren();(p.chapters||[]).forEach(chapterRow);$('#youtube-edit-error').textContent='';ytDialog.showModal();}
 $('#youtube-add-chapter').addEventListener('click',()=>{if(rows.children.length<40)chapterRow();});
 $('#youtube-edit-form').addEventListener('submit',async e=>{e.preventDefault();if(ytBusy||!club.canEdit)return;const token=ytEpoch,id=ytItem.id;try{
  const start=parse($('#youtube-start').value),end=$('#youtube-end').value.trim()?parse($('#youtube-end').value):0;
  const chapters=[...rows.children].map(row=>({time:parse(row.querySelector('.chapter-time').value),title:row.querySelector('.chapter-name').value.trim()})).sort((a,b)=>a.time-b.time);
  if(!Number.isFinite(start)||!Number.isFinite(end)||start<0||(end&&end<=start)||chapters.some((c,i)=>!Number.isFinite(c.time)||c.time<start||(end&&c.time>=end)||!c.title||(i&&chapters[i-1].time===c.time)))throw Error('Vul geldige tijden in en geef iedere oefening een unieke tijd en naam binnen het fragment.');
  ytBusy=true;e.target.querySelectorAll('button,input').forEach(el=>el.disabled=true);
  await club.api('/api/media/'+id+'/youtube',{method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({start,end,chapters,revision:ytRevision})});if(token===ytEpoch)ytDialog.close();await club.refreshLibrary?.();
 }catch(error){if(token===ytEpoch)$('#youtube-edit-error').textContent=error.message;}finally{ytBusy=false;e.target.querySelectorAll('button,input').forEach(el=>el.disabled=false);}});
 ytDialog.addEventListener('close',()=>{++ytEpoch;});
 window.addEventListener('club-auth',()=>{if(!club.canEdit){dialog.close();ytDialog.close();}});
 window.MCHVideoEditor={open(value){if(club.canEdit){if(value.youtube)openYouTube(value);else if(value.video)openClip(value);}},format,parse};
})();
