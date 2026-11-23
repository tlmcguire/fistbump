const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const window = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'renderer', 'js', 'places.js'), 'utf8'), { window });
const { summary } = window.Places;

test('a short place is left alone', () => {
  assert.deepStrictEqual({ ...summary('Austin, TX') }, { short: 'Austin, TX', full: 'Austin, TX', shortened: false });
  assert.strictEqual(summary('').short, '');
  assert.strictEqual(summary(null).short, '');
});

test('a remote list for many states becomes one phrase', () => {
  const states = ['Alabama', 'Arizona', 'California', 'Colorado', 'Connecticut', 'Delaware', 'Florida', 'Georgia'];
  const s = summary(states.map(x => `${x} - Remote`).join('; '));
  assert.strictEqual(s.short, 'Remote in 8 places');
  assert.ok(s.full.startsWith('Alabama - Remote') && s.shortened);
});

test('a long list of cities shows the first few and the count', () => {
  const s = summary('San Francisco, CA; New York, NY; Austin, TX; Seattle, WA; Boston, MA; Denver, CO; Chicago, IL');
  assert.match(s.short, /^San Francisco, CA; ?|^San Francisco, CA, /);
  assert.match(s.short, /\+\d+ more$/);
  assert.ok(s.short.length <= 60);
});

test('one very long place is cut at a word', () => {
  const s = summary('A very long single location description that goes on and on and on without any separators at all');
  assert.ok(s.short.endsWith('…') && s.short.length <= 60 && s.shortened);
});
