const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
class Element {
 constructor(tag='div'){this.tag=tag;this.children=[];this.dataset={};this.listeners={};this.hidden=false;this.disabled=false;}
 append(...nodes){for(const n of nodes){n.parent=this;this.children.push(n);}}
 close(){this.open=false;}
 setAttribute(name,value){this[name]=value;}
 addEventListener(name,fn){this.listeners[name]=fn;}
 async click(){if(!this.disabled)return this.listeners.click?.();}
 querySelectorAll(selector){return this.children.flatMap(n=>[...(selector==='button'&&n.tag==='button'?[n]:[]),...n.querySelectorAll(selector)]);}
 querySelector(selector){return this.children.find(n=>selector===n.tag||selector==='.'+n.className||selector===`[data-direction=${n.dataset.direction}]`)||this.children.map(n=>n.querySelector(selector)).find(Boolean);}
 insertBefore(node,before){if(node.parent){node.parent.children.splice(node.parent.children.indexOf(node),1);}node.parent=this;const index=this.children.indexOf(before);this.children.splice(index<0?this.children.length:index,0,node);}
 remove(){this.parent.children.splice(this.parent.children.indexOf(this),1);}
}
async function load(){
 const nodes=new Map(),events={},writes=[];const get=s=>{if(!nodes.has(s))nodes.set(s,new Element());return nodes.get(s);};
 let items=[{id:'first',title:'Eerste'},{id:'second',title:'Tweede'}],fail=false;
 const club={canEdit:true,revision:10,api:async(path,options)=>{if(!options)return {items:[...items],revision:club.revision};writes.push({path,...JSON.parse(options.body)});if(fail)throw new Error('De lijst is intussen gewijzigd.');items.reverse();club.revision++;return {ok:true};}};
 vm.runInNewContext(fs.readFileSync('internal/web/static/playlists.js','utf8'),{document:{querySelector:get,createElement:tag=>new Element(tag)},window:{MCH:club,addEventListener:(name,fn)=>events[name]=fn}});
 await new Promise(resolve=>setImmediate(resolve));
 return {get,club,writes,events,setFail:()=>{fail=true;}};
}
test('edit controls move playlists, enforce boundaries and keep iframe nodes on refresh',async()=>{
 const {get,club,writes,events}=await load(),grid=get('#playlist-grid'),first=grid.children[0],frame=first.querySelector('iframe');
 assert.equal(first.querySelector('.playlist-tools').hidden,true);
 await get('#playlist-edit-mode').click();
 assert.equal(first.querySelector('.playlist-tools').hidden,false);
 assert.equal(first.querySelector('[data-direction=up]').disabled,true);
 assert.equal(grid.children[1].querySelector('[data-direction=down]').disabled,true);
 await first.querySelector('[data-direction=down]').click();
 assert.deepEqual(writes,[{path:'/api/playlists/first/move',direction:'down',revision:10}]);
 assert.equal(grid.children[1],first);assert.equal(first.querySelector('iframe'),frame);
 await club.refreshLibrary();assert.equal(first.querySelector('iframe'),frame);
 club.canEdit=false;events['club-auth']();assert.equal(first.querySelector('.playlist-tools').hidden,true);
});
test('failed move shows the error and re-enables controls',async()=>{
 const {get,setFail}=await load(),first=get('#playlist-grid').children[0];setFail();await get('#playlist-edit-mode').click();
 await first.querySelector('[data-direction=down]').click();
 assert.match(get('#playlist-status').textContent,/intussen gewijzigd/);
 assert.equal(first.querySelector('[data-direction=down]').disabled,false);
});
