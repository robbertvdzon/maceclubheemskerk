'use strict';
(()=>{
 const $=s=>document.querySelector(s);if(!$('#bingo-grid'))return;
 const club=window.MCH;let cells=[],editing=false,busy=false,selected;
 function render(){
  $('#bingo-grid').replaceChildren(...cells.map(cell=>{
   const button=document.createElement('button');button.className='bingo-cell'+(cell.checked?' crossed':'')+(cell.id===13?' free-cell':'');button.type='button';button.disabled=!club.canEdit||busy;button.setAttribute('aria-pressed',String(cell.checked));button.setAttribute('aria-label',`${cell.text}, ${cell.checked?'doorgestreept':'niet doorgestreept'}${editing?', tekst aanpassen':''}`);
   const label=document.createElement('span');label.textContent=cell.text;button.append(label);
   if(cell.id===13){const star=document.createElement('span');star.className='bingo-star';star.textContent='★';star.setAttribute('aria-hidden','true');button.prepend(star);}
   button.addEventListener('click',()=>{if(editing){selected=cell;$('#bingo-text').value=cell.text;$('#bingo-checked').checked=cell.checked;$('#bingo-error').textContent='';$('#bingo-dialog').showModal();}else save(cell,{checked:!cell.checked});});return button;
  }));
  const rows=Array.from({length:5},(_,r)=>Array.from({length:5},(_,c)=>r*5+c));const cols=Array.from({length:5},(_,c)=>Array.from({length:5},(_,r)=>r*5+c));const lines=[...rows,...cols,[0,6,12,18,24],[4,8,12,16,20]];const wins=lines.filter(line=>line.every(i=>cells[i]?.checked)).length;
  $('#bingo-score').textContent=`${cells.filter(c=>c.checked).length} van 25 afgestreept${wins?' · BINGO! '+wins+' volle '+(wins===1?'rij':'rijen'):''}`;
  $('#bingo-mark-mode').setAttribute('aria-pressed',String(!editing));$('#bingo-edit-mode').setAttribute('aria-pressed',String(editing));$('#bingo-mark-mode').classList.toggle('active',!editing);$('#bingo-edit-mode').classList.toggle('active',editing);
  $('#bingo-help').textContent=editing?'Tik op een vakje om de tekst aan te passen.':'Tik op een vakje om het af te strepen of weer vrij te maken.';
 }
 async function refresh(){try{const data=await club.api('/api/bingo');cells=data.cells;club.revision=data.revision;$('#bingo-status').textContent='';render();}catch{$('#bingo-status').textContent='De bingo kon niet worden geladen. Probeer het zo opnieuw.';}}
 async function save(cell,changes){if(busy||!club.canEdit)return;busy=true;render();$('#bingo-error').textContent='';try{await club.api('/api/bingo/'+cell.id,{method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({text:cell.text,checked:cell.checked,version:cell.version,...changes})});if($('#bingo-dialog').open)$('#bingo-dialog').close();await refresh();}catch(error){if($('#bingo-dialog').open)$('#bingo-error').textContent=error.message;else{$('#bingo-status').textContent=error.message;await refresh();$('#bingo-status').textContent=error.message;}}finally{busy=false;render();}}
 $('#bingo-mark-mode').addEventListener('click',()=>{editing=false;render();});$('#bingo-edit-mode').addEventListener('click',()=>{editing=true;render();});
 $('#bingo-form').addEventListener('submit',e=>{e.preventDefault();save(selected,{text:$('#bingo-text').value,checked:$('#bingo-checked').checked});});
 window.addEventListener('club-auth',()=>{if(!club.canEdit&&$('#bingo-dialog').open)$('#bingo-dialog').close();render();});
 club.refreshLibrary=refresh;refresh();
})();
