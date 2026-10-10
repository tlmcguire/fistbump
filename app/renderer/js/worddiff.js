// Word-level diff for suggestion cards. Mirrors backend/internal/diff.Words.
import { h } from './dom.js';

export function wordDiff(before, after) {
  // Newlines are their own tokens so bullets keep their lines.
  const tok = (s) => s.replace(/\n/g, ' \n ').split(/[ \t]+/).filter(Boolean);
  const a = tok(before);
  const b = tok(after);
  const dp = Array.from({ length: a.length + 1 }, () => new Array(b.length + 1).fill(0));
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const out = [];
  const push = (op, text) => {
    const last = out[out.length - 1];
    if (last && last.op === op) last.text += text === '\n' || last.text.endsWith('\n') ? text : ` ${text}`;
    else out.push({ op, text });
  };
  let i = 0;
  let j = 0;
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) { push('eq', a[i]); i++; j++; }
    else if (dp[i + 1][j] >= dp[i][j + 1]) push('del', a[i++]);
    else push('add', b[j++]);
  }
  while (i < a.length) push('del', a[i++]);
  while (j < b.length) push('add', b[j++]);
  return out;
}

// Renders ops as spans; side 'before' shows eq+del, 'after' shows eq+add.
export function renderWords(words, side) {
  const frag = document.createDocumentFragment();
  const sp = (t) => (t.endsWith('\n') ? '' : ' ');
  for (const w of words) {
    if (w.op === 'eq') frag.append(w.text + sp(w.text));
    else if (w.op === 'del' && side === 'before') frag.append(h('del', { class: 'w' }, w.text), sp(w.text));
    else if (w.op === 'add' && side === 'after') frag.append(h('ins', { class: 'w' }, w.text), sp(w.text));
  }
  return frag;
}
