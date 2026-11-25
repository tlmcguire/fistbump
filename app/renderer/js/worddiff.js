// Word-level diff with typography-aware comparison and clean line/bullet rendering.
window.WordDiff = (() => {
  // Newlines are their own tokens so bullets keep their lines.
  const tokens = text => text.replace(/\n/g, " \n ").split(/[ \t]+/).filter(Boolean);

  // The stored resume text backslash-escapes a leading -, # or * so it is not read as Markdown.
  // People should see the line as written, so the backslash is dropped from what is shown.
  const plain = text => String(text ?? "").replace(/^([ \t]*)\\([-#*])/gm, "$1$2");

  // Fold curly apostrophes, quotes, and dashes when comparing tokens so typographic equivalents
  // like "Bachelor’s" vs "Bachelor's" are never flagged as edits.
  const foldTypo = s => s
    .replace(/[\u2018\u2019\u201a\u201b\u00b4]/g, "'")
    .replace(/[\u201c\u201d\u201e\u201f]/g, '"')
    .replace(/[\u2013\u2014\u2212]/g, "-")
    .replace(/\u00a0/g, " ");

  const eq = (a, b) => a === b || foldTypo(a) === foldTypo(b);

  const diff = (before, after) => {
    const a = tokens(plain(before));
    const b = tokens(plain(after));
    const dp = Array.from({length: a.length + 1}, () => new Array(b.length + 1).fill(0));
    for (let i = a.length - 1; i >= 0; i--) {
      for (let j = b.length - 1; j >= 0; j--) {
        dp[i][j] = eq(a[i], b[j]) ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
      }
    }
    const out = [];
    const push = (op, text) => {
      const last = out[out.length - 1];
      if (last && last.op === op) last.text += text === "\n" || last.text.endsWith("\n") ? text : ` ${text}`;
      else out.push({op, text});
    };
    let i = 0;
    let j = 0;
    while (i < a.length && j < b.length) {
      if (eq(a[i], b[j])) { push("eq", b[j]); i++; j++; }
      else if (dp[i + 1][j] >= dp[i][j + 1]) push("del", a[i++]);
      else push("add", b[j++]);
    }
    while (i < a.length) push("del", a[i++]);
    while (j < b.length) push("add", b[j++]);
    return out;
  };

  // side "before" shows eq and del; side "after" shows eq and add.
  // Renders each line in a structured .diff-line block (with .diff-bullet for bulleted items)
  // so multi-bullet suggestions scan cleanly.
  const render = (words, side) => {
    const frag = document.createDocumentFragment();
    const mark = (tag, text) => Object.assign(document.createElement(tag), {textContent: text});

    // Collect visible runs split by newline so each line gets its own container.
    const lines = [[]];
    for (const raw of words) {
      const op = raw.op;
      if (op === "del" && side !== "before") continue;
      if (op === "add" && side !== "after") continue;
      const parts = plain(raw.text).split("\n");
      parts.forEach((part, idx) => {
        if (idx > 0) lines.push([]);
        if (part) lines[lines.length - 1].push({op, text: part});
      });
    }

    for (const segments of lines) {
      const lineText = segments.map(s => s.text).join(" ").trim();
      if (!lineText && lines.length > 1) continue;
      const isBullet = /^[-*•]\s/.test(lineText);
      const hasChange = segments.some(s => s.op !== "eq");
      const row = document.createElement("div");
      row.className = `diff-line${isBullet ? " diff-bullet" : ""}${hasChange ? " diff-changed" : ""}`;
      for (let i = 0; i < segments.length; i++) {
        const seg = segments[i];
        const space = i < segments.length - 1 ? " " : "";
        if (seg.op === "eq") row.append(seg.text + space);
        else if (seg.op === "del") row.append(mark("del", seg.text), space);
        else if (seg.op === "add") row.append(mark("ins", seg.text), space);
      }
      frag.append(row);
    }
    return frag;
  };

  return {diff, render, plain};
})();
