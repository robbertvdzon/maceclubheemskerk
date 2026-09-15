'use strict';
const $ = (s) => document.querySelector(s);
let member = false, examples = true, category = 'all', mediaFilter = 'all', expanded = false;
let nextID = 100, photoURL = '', toastTimer;
const initial = [
 {id:1,section:'exercise',type:'video',category:'Basis',title:'De 360 swing',description:'Een voorbeeld van een basisbeweging in de bibliotheek.',tone:0},
 {id:2,section:'exercise',type:'video',category:'Techniek',title:'De 10-to-2',description:'Een plek voor uitleg, uitvoering en aandachtspunten.',tone:1},
 {id:3,section:'exercise',type:'video',category:'Flow',title:'Van beweging naar flow',description:'Bewaar combinaties die je samen wilt oefenen.',tone:2},
 {id:4,section:'exercise',type:'video',category:'Basis',title:'Grip & houding',description:'Zo ziet een volgende oefening in de verzameling eruit.',tone:2},
 {id:5,section:'training',type:'video',title:'Samen in beweging',description:'Hier komt een video van jullie eigen training.',tone:1},
 {id:6,section:'training',type:'photo',title:'Tussen de swings',description:'Ruimte voor een eigen trainingsfoto.',tone:0},
 {id:7,section:'training',type:'photo',title:'Met de club',description:'Een foto uit jullie training.',tone:2}
];
const added = [];
const icon = name => `<svg aria-hidden="true"><use href="#${name}"/></svg>`;
function illustration(tone=0) {
 const palettes=[['#d6dac9','#abb899','#3d4939'],['#d3b9a3','#bd9376','#4d4437'],['#c5cebd','#8d9e85','#35463a']];
 const [bg,accent,ink]=palettes[tone%3];
 return `<svg class="mace-illustration" viewBox="0 0 500 300" preserveAspectRatio="xMidYMid slice" aria-hidden="true"><rect width="500" height="300" fill="${bg}"/><g fill="none" stroke="${accent}" stroke-width="1"><circle cx="270" cy="150" r="124"/><circle cx="270" cy="150" r="100"/><path d="M0 150h500M270 0v300"/></g><g transform="translate(265 155) rotate(${tone===1?42:-35})"><rect x="-8" y="-62" width="16" height="212" rx="8" fill="${ink}"/><circle cy="-76" r="44" fill="${ink}"/><ellipse cx="-12" cy="-88" rx="17" ry="21" fill="${accent}" opacity=".42"/><path d="M-7 105H7M-7 115H7M-7 125H7" stroke="${bg}" stroke-width="2" opacity=".65"/></g><path d="M42 45h22M53 34v22M441 254h22M452 243v22" stroke="${ink}" opacity=".35" stroke-width="1"/></svg>`;
}
function activeItems(){return [...(examples?initial:[]),...added];}
function makeCard(item){
 const button=document.createElement('button'); button.className='media-card';button.type='button';button.setAttribute('aria-label',`${item.type==='photo'?'Bekijk foto':'Bekijk video'}: ${item.title}`);
 const visual=document.createElement('div');visual.className='card-image';
 if(item.photo){const img=document.createElement('img');img.src=item.photo;img.alt='';img.className='training-visual';visual.append(img);}
 else if(item.type==='photo'){visual.innerHTML=`<div class="photo-empty ${item.tone===2?'warm':''}"><div class="grid-lines"></div>${icon('camera')}<span>Jullie foto komt hier</span></div>`;}
 else {visual.innerHTML=illustration(item.tone);const play=document.createElement('span');play.className='play-circle';play.innerHTML=icon('play');visual.append(play);}
 const tag=document.createElement('span');tag.className='card-tag';tag.textContent=item.id<100?'ONTWERPVOORBEELD':'JOUW VOORBEELD';visual.append(tag);
 const body=document.createElement('div');body.className='card-body';
 const meta=document.createElement('div');meta.className='card-meta';
 const tag1=document.createElement('span');tag1.textContent=item.category|| (item.type==='photo'?'FOTO':'TRAININGSVIDEO');const tag2=document.createElement('span');tag2.textContent=item.type==='photo'?'Clubalbum':'YouTube-video';meta.append(tag1,tag2);
 const title=document.createElement('h3');title.textContent=item.title;const desc=document.createElement('p');desc.textContent=item.description;
 body.append(meta,title,desc);button.append(visual,body);button.addEventListener('click',()=>openMedia(item));return button;
}
function render(){
 const all=activeItems();const exercises=all.filter(i=>i.section==='exercise'&&(category==='all'||i.category===category));
 const trainings=all.filter(i=>i.section==='training'&&(mediaFilter==='all'||i.type===mediaFilter));
 $('#exercise-grid').replaceChildren(...exercises.slice(0,expanded?100:3).map(makeCard));$('#training-grid').replaceChildren(...trainings.map(makeCard));
 $('#exercise-empty').hidden=exercises.length>0;$('#training-empty').hidden=trainings.length>0;
 $('#more-exercises').hidden=exercises.length<=3;$('#more-exercises').textContent=expanded?'Minder oefeningen tonen ↑':'Alle oefeningen bekijken ↗';
 document.querySelectorAll('.member-only').forEach(el=>el.hidden=!member);
 document.querySelectorAll('.sample-note').forEach(el=>el.hidden=!examples);
}
function closeDialogs(){document.querySelectorAll('dialog[open]').forEach(d=>d.close());}
function setMember(value){member=value;closeDialogs();$('#account-button span').textContent=member?'Robbert':'Inloggen';$('#cta-add').innerHTML=(member?'Video of foto toevoegen':'Inloggen als clublid')+icon('arrow');$('#visitor-mode').classList.toggle('selected',!member);$('#member-mode').classList.toggle('selected',member);$('#visitor-mode').setAttribute('aria-pressed',String(!member));$('#member-mode').setAttribute('aria-pressed',String(member));render();}
function openLogin(){closeDialogs();$('#login-dialog').showModal();}
$('#account-button').addEventListener('click',()=>member?$('#account-dialog').showModal():openLogin());
$('#visitor-mode').addEventListener('click',()=>setMember(false));$('#member-mode').addEventListener('click',()=>setMember(true));$('#demo-login').addEventListener('click',()=>setMember(true));$('#logout').addEventListener('click',()=>setMember(false));
$('#cta-add').addEventListener('click',()=>member?openAdd('training'):openLogin());
$('#show-examples').addEventListener('change',e=>{examples=e.target.checked;render();});
$('#more-exercises').addEventListener('click',()=>{expanded=!expanded;render();});
for(const attribute of ['category','media']){document.querySelectorAll(`[data-${attribute}]`).forEach(b=>b.addEventListener('click',()=>{document.querySelectorAll(`[data-${attribute}]`).forEach(el=>{el.classList.toggle('active',el===b);el.setAttribute('aria-pressed',String(el===b));});if(attribute==='category'){category=b.dataset.category;expanded=true;}else{mediaFilter=b.dataset.media;}render();}));}
document.querySelectorAll('[data-close]').forEach(b=>b.addEventListener('click',()=>b.closest('dialog').close()));
document.querySelectorAll('[data-add]').forEach(b=>b.addEventListener('click',()=>openAdd(b.dataset.add)));
function syncMediaType(){const photo=$('#section-value').value==='training'&&$('#media-type').value==='photo';$('#url-field').hidden=photo;$('#photo-field').hidden=!photo;$('#video-url').required=!photo;$('#photo-file').required=photo;}
function openAdd(section){if(!member)return openLogin();closeDialogs();$('#content-form').reset();$('#section-value').value=section;$('#form-error').textContent='';$('#photo-preview').hidden=true;if(photoURL){URL.revokeObjectURL(photoURL);photoURL='';}$('#add-title').textContent=section==='exercise'?'OEFENING TOEVOEGEN.':'TRAININGSBEELDEN TOEVOEGEN.';$('#add-description').textContent=section==='exercise'?'Plak een YouTube-link. Geef de oefening een herkenbare naam.':'Een video of foto van jullie training, met een kort verhaal erbij.';$('#media-type-field').hidden=section==='exercise';$('#category-field').hidden=section!=='exercise';syncMediaType();$('#add-dialog').showModal();}
$('#media-type').addEventListener('change',syncMediaType);
$('#photo-file').addEventListener('change',()=>{if(photoURL){URL.revokeObjectURL(photoURL);photoURL='';}const file=$('#photo-file').files[0];$('#photo-preview').hidden=true;$('#form-error').textContent='';if(!file)return;if(!['image/jpeg','image/png','image/webp'].includes(file.type)||file.size>10*1024*1024){$('#form-error').textContent='Kies een JPG-, PNG- of WebP-foto van maximaal 10 MB.';$('#photo-file').value='';return;}photoURL=URL.createObjectURL(file);$('#photo-preview').src=photoURL;$('#photo-preview').hidden=false;});
function youtubeID(text){try{const url=new URL(text);if(!['https:','http:'].includes(url.protocol))return null;const host=url.hostname.toLowerCase();let id;if(host==='youtu.be')id=url.pathname.slice(1);else if(['youtube.com','www.youtube.com','m.youtube.com'].includes(host))id=url.searchParams.get('v')||url.pathname.match(/^\/(?:shorts|embed)\/([^/]+)/)?.[1];return /^[A-Za-z0-9_-]{11}$/.test(id||'')?id:null;}catch{return null;}}
$('#content-form').addEventListener('submit',e=>{e.preventDefault();if(!member)return;const section=$('#section-value').value;const type=section==='exercise'?'video':$('#media-type').value;const title=$('#content-title').value.trim();if(!title){$('#form-error').textContent='Geef je toevoeging een titel.';return;}const id=type==='video'?youtubeID($('#video-url').value):null;if(type==='video'&&!id){$('#form-error').textContent='Deze link herkennen we niet. Plak een volledige YouTube-videolink.';return;}if(type==='photo'&&!photoURL){$('#form-error').textContent='Kies eerst een foto.';return;}added.unshift({id:nextID++,section,type,title,description:$('#content-description').value.trim(),category:section==='exercise'?$('#content-category').value:undefined,youtube:id,photo:type==='photo'?photoURL:null,tone:0});photoURL='';category='all';mediaFilter='all';expanded=true;document.querySelectorAll('[data-category],[data-media]').forEach(el=>{const on=el.dataset.category==='all'||el.dataset.media==='all';el.classList.toggle('active',on);el.setAttribute('aria-pressed',String(on));});$('#add-dialog').close();render();document.getElementById(section==='exercise'?'oefeningen':'trainingen').scrollIntoView({behavior:'smooth'});$('#toast').textContent='Toegevoegd aan dit ontwerp. Nog niet opgeslagen op de website.';$('#toast').hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>$('#toast').hidden=true,5500);});
function openMedia(item){$('#media-title').textContent=item.title;$('#media-description').textContent=item.description;$('#media-label').textContent=item.type==='photo'?'HET CLUBALBUM':'MACE CLUB / VIDEOBIBLIOTHEEK';$('#youtube-link').hidden=!item.youtube;if(item.youtube)$('#youtube-link').href='https://www.youtube.com/watch?v='+item.youtube;const stage=$('#media-stage');stage.replaceChildren();if(item.photo){const img=document.createElement('img');img.src=item.photo;img.alt=item.title;stage.append(img);}else{const ph=document.createElement('div');ph.className='video-placeholder';ph.innerHTML=icon(item.type==='photo'?'camera':'play');const title=document.createElement('h3');title.textContent=item.type==='photo'?'Jullie trainingsfoto komt hier':'Hier speelt de YouTube-video';const p=document.createElement('p');p.textContent=item.youtube?'Je link is toegevoegd aan het ontwerp. Je kunt de video hieronder op YouTube openen.':'Dit is een ontwerpvoorbeeld. In de werkende website zie je hier de toegevoegde video of foto.';ph.append(title,p);stage.append(ph);}$('#media-dialog').showModal();}
render();
