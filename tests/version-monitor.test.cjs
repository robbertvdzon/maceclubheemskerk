const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const source = fs.readFileSync('internal/web/static/monitor.js', 'utf8');
const current = 'a'.repeat(64);
function setup() {
  const elements = new Map();
  const listeners = {};
  const intervals = [];
  const reloads = [];
  const calls = [];
  let next = current;
  let failed = false;
  let dialogOpen = false;
  let contentRefreshes = 0;
  let revision = 0;
  const document = { hidden: false,
    querySelector(selector) {
      if (selector === 'dialog[open]') return dialogOpen ? {} : null;
      if (!elements.has(selector)) elements.set(selector, { content: current, addEventListener() {}, open: false, textContent: '' });
      return elements.get(selector);
    },
    querySelectorAll() { return []; },
    addEventListener(type, fn) { listeners[type] = fn; },
  };
  vm.runInNewContext(source, { document,
    window: { MCH: {revision: 0, async refreshLibrary(){contentRefreshes++; this.revision=revision;}}, addEventListener(type, fn) { listeners[type] = fn; } },
    location: { href: 'https://club.example/', replace(url) { reloads.push(url); } },
    URL, console,
    setInterval(fn, delay) { intervals.push({fn, delay}); },
    async fetch(path, options) {
      calls.push({path,options});
      if (failed) throw new Error('offline');
      return {ok: true, json: async () => path === '/api/version' ? {version:next,revision} : {user:null} };
    },
  });
  return { document, listeners, calls, reloads,
    check: intervals.find(i => i.delay === 3000).fn,
    update(value) { next=value; }, offline(value) { failed=value; },
    dialog(value) {dialogOpen=value;}, content(value) {revision=value;}, refreshes(){return contentRefreshes;},
  };
}
test('new backend version reloads once with cache-busting URL', async () => {
  const s=setup(); await s.check(); assert.equal(s.reloads.length,0);
  s.update('b'.repeat(64)); await s.check(); await s.check();
  assert.equal(s.reloads.length,1);
  assert.equal(new URL(s.reloads[0]).searchParams.get('v'),'b'.repeat(64));
  for(const call of s.calls) { assert.equal(call.options.cache,'no-store'); assert.equal(call.options.credentials,'same-origin'); }
});
test('offline, malformed versions and hidden tabs keep the current page', async () => {
  const s=setup(); s.offline(true); await s.check();
  s.offline(false); s.update('invalid'); await s.check();
  s.update('b'.repeat(64)); s.document.hidden=true; await s.check();
  assert.equal(s.reloads.length,0);
  s.document.hidden=false; await s.check(); assert.equal(s.reloads.length,1);
});

test('forms postpone updates and content revisions refresh without a page reload', async () => {
 const s=setup();s.content(1);s.dialog(true);await s.check();assert.equal(s.refreshes(),0);
 s.dialog(false);await s.check();assert.equal(s.refreshes(),1);assert.equal(s.reloads.length,0);
 await s.check();assert.equal(s.refreshes(),1);
 s.update('b'.repeat(64));s.dialog(true);await s.check();assert.equal(s.reloads.length,0);
 s.dialog(false);await s.check();assert.equal(s.reloads.length,1);
});
