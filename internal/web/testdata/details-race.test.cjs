const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync('internal/web/static/app.js', 'utf8');
const details = source.slice(source.indexOf('function section('), source.indexOf('async function refresh()'));
function setup() {
  const root = {children: [], replaceChildren(...children) { this.children = children; }, append(...children) { this.children.push(...children); }};
  const state = {session: {id:'s', agent:'Codex'}, activeTab:'changes', activeWorktree:'one'};
  const queue = [];
  const context = {state, $: () => root, document: {querySelectorAll: () => []}, el: (tag, className, text) => ({tag, className, text, children: [], append(...nodes) { this.children.push(...nodes); }}), request: () => new Promise((resolve, reject) => queue.push({resolve, reject}))};
  vm.createContext(context);
  vm.runInContext(details + '\nthis.drawDetails = drawDetails; this.invalidateDetails = () => detailsGeneration++;', context);
  return {root, state, queue, ...context};
}
test('late changes response and error do not overwrite checks', async () => {
  for (const fails of [false, true]) {
    const {root, state, queue, drawDetails} = setup();
    const pending = drawDetails();
    state.activeTab = 'checks';
    await drawDetails();
    if (fails) queue[0].reject(Error('offline')); else queue[0].resolve({status:'', numstat:''});
    await pending;
    assert.deepEqual(root.children.map(child => child.text), ['Checks unavailable']);
  }
});
test('only latest changes request renders and invalidation suppresses old responses', async () => {
  const {root, queue, drawDetails, invalidateDetails} = setup();
  const first = drawDetails();
  const second = drawDetails();
  queue[1].resolve({status:'', numstat:''});
  await second;
  const rendered = root.children.slice();
  queue[0].resolve({status:'', numstat:''});
  await first;
  assert.deepEqual(root.children, rendered);
  const third = drawDetails();
  invalidateDetails();
  root.replaceChildren({text:'project overview'});
  queue[2].resolve({status:'', numstat:''});
  await third;
  assert.deepEqual(root.children.map(child => child.text), ['project overview']);
});