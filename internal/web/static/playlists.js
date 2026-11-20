'use strict';
(() => {
 const $=s=>document.querySelector(s),club=window.MCH;
 if(!$('#playlists-page'))return;
 let items=[],editMode=false,busy=false,loading=false,removing=null,removeRevision=0;
 const cards=new Map(),grid=$('#playlist-grid');
 const titleOf=item=>item.title||'Spotify-playlist';
 function updateTools(){
  if(!club.canEdit)editMode=false;
  $('#playlist-edit-mode').hidden=!club.canEdit;
  $('#playlist-edit-mode').textContent=editMode?'✓ Klaar met bewerken':'✎ Bewerken';
  $('#playlist-edit-mode').setAttribute('aria-pressed',String(editMode));
  $('#add-playlist').hidden=!club.canEdit;
  items.forEach((item,index)=>{const card=cards.get(item.id);if(!card)return;card.querySelector('.playlist-tools').hidden=!club.canEdit||!editMode;for(const button of card.querySelectorAll('button'))button.disabled=busy;card.querySelector('[data-direction=up]').disabled=busy||index===0;card.querySelector('[data-direction=down]').disabled=busy||index===items.length-1;});
 }
 function render(){
  const ids=new Set(items.map(item=>item.id));
  for(const [id,card] of cards){if(!ids.has(id)){card.remove();cards.delete(id);}}
  items.forEach((item,index)=>{
   let card=cards.get(item.id);
   if(!card){
    card=document.createElement('article');card.className='playlist-card';
    const h=document.createElement('h2');
    const frame=document.createElement('iframe');frame.src='https://open.spotify.com/embed/playlist/'+item.id+'?theme=0';frame.loading=index<2?'eager':'lazy';frame.height='352';frame.allow='autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture';frame.allowFullscreen=true;frame.referrerPolicy='strict-origin-when-cross-origin';
    const link=document.createElement('a');link.className='text-button';link.href='https://open.spotify.com/playlist/'+item.id;link.target='_blank';link.rel='noopener noreferrer';link.textContent='Openen in Spotify ↗';
    const controls=document.createElement('div');controls.className='playlist-tools';controls.hidden=true;
    for(const [direction,label] of [['up','↑ Omhoog'],['down','↓ Omlaag']]){const move=document.createElement('button');move.type='button';move.className='text-button';move.dataset.direction=direction;move.textContent=label;move.setAttribute('aria-label',titleOf(item)+' '+(direction==='up'?'omhoog':'omlaag')+' verplaatsen');move.addEventListener('click',()=>movePlaylist(item.id,direction));controls.append(move);}
    const remove=document.createElement('button');remove.type='button';remove.className='text-button';remove.textContent='Verwijderen';remove.addEventListener('click',()=>{removing=item;removeRevision=club.revision;$('#playlist-delete-name').textContent=titleOf(item);$('#playlist-delete-error').textContent='';$('#playlist-delete-dialog').showModal();});controls.append(remove);
    card.append(h,frame,link,controls);cards.set(item.id,card);
   }
   card.querySelector('h2').textContent=titleOf(item);
   card.querySelector('iframe').title='Spotify-playlist: '+titleOf(item);
   // Keep existing iframe nodes in place so polling and edit mode do not restart music.
   if(grid.children[index]!==card)grid.insertBefore(card,grid.children[index]||null);
  });
  $('#playlists-empty').hidden=items.length>0;updateTools();
 }
 async function refresh(){if(loading)return;loading=true;try{const data=await club.api('/api/playlists');items=data.items;club.revision=data.revision;render();$('#playlist-status').textContent='';}catch{$('#playlist-status').textContent='De playlists konden niet worden geladen. Probeer het zo opnieuw.';}finally{loading=false;}}
 async function movePlaylist(id,direction){
  if(busy||!club.canEdit)return;busy=true;updateTools();
  try{await club.api('/api/playlists/'+id+'/move',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({direction,revision:club.revision})});await refresh();}
  catch(error){await refresh();$('#playlist-status').textContent=error.message;}
  finally{busy=false;updateTools();}
 }
 $('#playlist-edit-mode').addEventListener('click',()=>{if(!club.canEdit||busy)return;editMode=!editMode;updateTools();});
 $('#add-playlist').addEventListener('click',()=>{if(!club.canEdit)return;$('#playlist-form').reset();$('#playlist-error').textContent='';$('#playlist-dialog').showModal();});
 $('#playlist-form').addEventListener('submit',async e=>{
  e.preventDefault();if(busy||!club.canEdit)return;busy=true;const button=e.target.querySelector('button[type=submit]');button.disabled=true;$('#playlist-error').textContent='';
  try{await club.api('/api/playlists',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({url:$('#playlist-url').value,title:$('#playlist-name').value})});$('#playlist-dialog').close();await refresh();}
  catch(error){$('#playlist-error').textContent=error.message;}
  finally{busy=false;button.disabled=false;updateTools();}
 });
 $('#playlist-confirm-delete').addEventListener('click',async()=>{
  if(busy||!club.canEdit||!removing)return;busy=true;$('#playlist-confirm-delete').disabled=true;
  try{await club.api('/api/playlists/'+removing.id,{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({revision:removeRevision})});$('#playlist-delete-dialog').close();await refresh();}
  catch(error){$('#playlist-delete-error').textContent=error.message;await refresh();removeRevision=club.revision;}
  finally{busy=false;$('#playlist-confirm-delete').disabled=false;updateTools();}
 });
 window.addEventListener('club-auth',()=>{if(!club.canEdit){$('#playlist-dialog').close();$('#playlist-delete-dialog').close();}updateTools();});
 club.refreshLibrary=refresh;updateTools();refresh();
})();
