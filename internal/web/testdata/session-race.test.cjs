const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync('internal/web/static/app.js', 'utf8');
const opening = source.slice(source.indexOf('let sessionGeneration=0;'), source.indexOf('function drawTabs()'));
function setup() {
  const nodes = new Map();
  const node = () => ({classList:{add(){},remove(){},toggle(){}},setAttribute(){},querySelector(){return node()},textContent:'',title:'',disabled:false});
  const state = {selectedDiff:null, view:'workspace', capabilities:{send:true}, session:null, openTabs:[], activeProject:null};
  const requests = [];
  const win = {dispatchEvent(){}};
  const context = {state, window:win, Event:class {}, detailsGeneration:0, $: selector => {if(!nodes.has(selector))nodes.set(selector,node());return nodes.get(selector)}, showSessionLoading(){}, request:path=>new Promise((resolve,reject)=>requests.push({path,resolve,reject})), drawProjects(){}, drawChat(){}, drawTabs(){}, drawDetails(){}};
  vm.createContext(context);
  vm.runInContext(opening + '\nthis.openSession = openSession; this.invalidateSession = () => sessionGeneration++;', context);
  return {state,requests,win,...context};
}
test('most recently opened session wins even if earlier request completes last', async () => {
  const {state,requests,win,openSession} = setup();
  const first = openSession({id:'p1',name:'First'}, {id:'s1'});
  const second = openSession({id:'p2',name:'Second'}, {id:'s2'});
  requests[1].resolve({id:'s2',title:'Second',worktreeId:'w2'});
  await second;
  requests[0].resolve({id:'s1',title:'First',worktreeId:'w1'});
  await first;
  assert.equal(state.session.id,'s2');
  assert.equal(state.activeProject,'p2');
  assert.equal(state.activeWorktree,'w2');
  assert.equal(win.gasaSendSession.id,'s2');
});
test('navigation invalidates pending successful and failed session loads', async () => {
  for (const fails of [false,true]) {
    const {state,requests,win,openSession,invalidateSession} = setup();
    const pending = openSession({id:'p1',name:'First'}, {id:'s1'});
    invalidateSession();
    state.session = {id:'other'};
    state.activeProject = 'other-project';
    win.gasaSendSession = state.session;
    if (fails) requests[0].reject(Error('offline')); else requests[0].resolve({id:'s1'});
    await pending;
    assert.equal(state.session.id,'other');
    assert.equal(state.activeProject,'other-project');
    assert.equal(win.gasaSendSession.id,'other');
  }
});