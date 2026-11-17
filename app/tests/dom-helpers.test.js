// el and fill build every view. A list passed inside a list must still end up as elements, not as text.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// Just enough of the DOM for el and fill.
class Node {
  constructor() { this.childNodes = []; this.attrs = {}; }
  append(...kids) { this.childNodes.push(...kids); }
  replaceChildren(...kids) { this.childNodes = kids; }
  setAttribute(k, v) { this.attrs[k] = v; }
  addEventListener() {}
}
const document = {
  createElement: () => new Node(),
  createTextNode: text => Object.assign(new Node(), { text }),
};

function loadHelpers() {
  const file = path.join(__dirname, '..', 'renderer', 'views', 'resume-builder', 'resume-builder.js');
  const window = {};
  vm.runInNewContext(fs.readFileSync(file, 'utf8'), { window, document, Node, console, localStorage: {} });
  return window.ResumeBuilder;
}

test('fill flattens nested lists into elements', () => {
  const { el, fill } = loadHelpers();
  const node = new Node();
  const cards = [el('section'), el('section')];
  fill(node, el('div'), false, [cards, el('footer')]);
  assert.equal(node.childNodes.length, 4);
  assert.ok(node.childNodes.every(child => child instanceof Node), 'every child is a node, none is an array');
});

test('el flattens nested lists of children', () => {
  const { el } = loadHelpers();
  const parent = el('div', {}, [[el('a'), el('b')], 'text']);
  assert.equal(parent.childNodes.length, 3);
  assert.ok(parent.childNodes.every(child => child instanceof Node));
});
