//new pahse 5 script for world level diff. based on the one found in the full build experiment branch
window.WordDiff = (() => {
    // Newlines are their own tokens so bullets keep their lines.
    const tokens = text => text.replace(/\n/g, " \n ").split(/[ \t]+/).filter(Boolean);
  
    const diff = (before, after) => {
      const a = tokens(before);
      const b = tokens(after);
      const dp = Array.from({length: a.length + 1}, () => new Array(b.length + 1).fill(0));
      for (let i = a.length - 1; i >= 0; i--) {
        for (let j = b.length - 1; j >= 0; j--) {
          dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
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
        if (a[i] === b[j]) { push("eq", a[i]); i++; j++; }
        else if (dp[i + 1][j] >= dp[i][j + 1]) push("del", a[i++]);
        else push("add", b[j++]);
      }
      while (i < a.length) push("del", a[i++]);
      while (j < b.length) push("add", b[j++]);
      return out;
    };
  
    // side "before" shows eq and del; side "after" shows eq and add
    const render = (words, side) => {
      const frag = document.createDocumentFragment();
      const mark = (tag, text) => Object.assign(document.createElement(tag), {textContent: text});
      for (const w of words) {
        const space = w.text.endsWith("\n") ? "" : " ";
        if (w.op === "eq") frag.append(w.text + space);
        else if (w.op === "del" && side === "before") frag.append(mark("del", w.text), space);
        else if (w.op === "add" && side === "after") frag.append(mark("ins", w.text), space);
      }
      return frag;
    };
  
    return {diff, render};
  })();