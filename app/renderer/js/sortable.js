// A list people can reorder three ways: drag a row by its grip, press the row's up and down buttons, or focus a
// row and press Alt+Up / Alt+Down. Dragging uses pointer events, not HTML drag and drop, so it behaves the same in
// every desktop web view and with a mouse, a trackpad or a pen. Esc while dragging puts the row back.
//
//   const list = window.Sortable.list(items, {label: item => "Summary", onChange: newItems => save(newItems)});
//   container.append(list);
window.Sortable = (() => {
  const make = (tag, cls, text) => {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined) node.textContent = text;
    return node;
  };
  const icon = id => {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS(ns, "use");
    use.setAttribute("href", `#i-${id}`);
    svg.append(use);
    return svg;
  };
  const reduced = () => document.documentElement.classList.contains("reduce-motion");

  function list(items, {label, onChange, name = "Order"}) {
    let order = [...items];
    const root = make("ol", "sortlist");
    root.setAttribute("aria-label", name);
    const live = make("p", "visually-hidden");
    live.setAttribute("aria-live", "polite");

    //Rows slide to their new place: each row's old position is noted, then it animates from there (FLIP).
    const slide = mutate => {
      const before = new Map([...root.children].map(n => [n, n.getBoundingClientRect().top]));
      mutate();
      if (reduced()) return;
      for (const node of root.children) {
        const from = before.get(node);
        if (from === undefined || node.classList.contains("dragging")) continue;
        const dy = from - node.getBoundingClientRect().top;
        if (!dy) continue;
        node.animate([{transform: `translateY(${dy}px)`}, {transform: "translateY(0)"}], {duration: 220, easing: "cubic-bezier(0.2, 0.9, 0.2, 1)"});
      }
    };

    const commit = (moved, announce = true) => {
      const next = [...root.children].map(n => n.item);
      const changed = next.some((x, i) => x !== order[i]);
      order = next;
      renumber();
      if (!changed) return;
      if (announce && moved) live.textContent = `${label(moved)} moved to position ${order.indexOf(moved) + 1} of ${order.length}.`;
      onChange?.(order);
    };

    const renumber = () => {
      [...root.children].forEach((row, i) => {
        row.querySelector(".sortlist-n").textContent = String(i + 1);
        row.querySelector(".sortlist-up").disabled = i === 0;
        row.querySelector(".sortlist-down").disabled = i === root.children.length - 1;
      });
    };

    const move = (row, delta) => {
      const rows = [...root.children];
      const at = rows.indexOf(row);
      const to = at + delta;
      if (to < 0 || to >= rows.length) return;
      slide(() => (delta < 0 ? root.insertBefore(row, rows[to]) : root.insertBefore(row, rows[to].nextSibling)));
      commit(row.item);
    };

    const row = item => {
      const node = make("li", "sortlist-row");
      node.item = item;
      node.tabIndex = 0;
      node.setAttribute("aria-label", `${label(item)}. Press Alt and an arrow key to move it.`);
      const grip = make("span", "sortlist-grip");
      grip.title = "Drag to reorder";
      grip.setAttribute("aria-hidden", "true");
      grip.append(icon("grip"));
      const up = make("button", "sortlist-up");
      up.type = "button";
      up.append(icon("chevron"));
      up.title = up.ariaLabel = `Move ${label(item)} up`;
      up.setAttribute("aria-label", `Move ${label(item)} up`);
      up.addEventListener("click", () => { move(node, -1); up.disabled || up.focus(); });
      const down = make("button", "sortlist-down");
      down.type = "button";
      down.append(icon("chevron"));
      down.title = `Move ${label(item)} down`;
      down.setAttribute("aria-label", `Move ${label(item)} down`);
      down.addEventListener("click", () => { move(node, 1); down.disabled || down.focus(); });
      node.append(grip, make("span", "sortlist-n"), make("span", "sortlist-label", label(item)), make("span", "sortlist-moves"));
      node.lastChild.append(up, down);

      node.addEventListener("keydown", event => {
        if (!event.altKey || event.target !== node) return;
        if (event.key === "ArrowUp" || event.key === "ArrowDown") {
          event.preventDefault();
          move(node, event.key === "ArrowUp" ? -1 : 1);
          node.focus();
        }
      });

      // ---- dragging by the grip (or anywhere on the row that is not a button) ----
      node.addEventListener("pointerdown", event => {
        if (event.button !== 0 || event.target.closest("button")) return;
        event.preventDefault();
        const startOrder = [...root.children];
        const startY = event.clientY;
        let lastY = startY;
        let dragging = false;
        node.setPointerCapture(event.pointerId);

        const onMove = e => {
          lastY = e.clientY;
          if (!dragging && Math.abs(lastY - startY) < 4) return; //a click, not a drag
          if (!dragging) {
            dragging = true;
            node.classList.add("dragging");
            root.classList.add("sorting");
          }
          const offset = lastY - startY + (node._shift || 0);
          node.style.transform = `translateY(${offset}px)`;
          //where the dragged row's middle now sits among the other rows
          const rect = node.getBoundingClientRect();
          const mid = rect.top + rect.height / 2;
          const others = [...root.children].filter(n => n !== node);
          let index = others.findIndex(n => {
            const r = n.getBoundingClientRect();
            return mid < r.top + r.height / 2;
          });
          if (index === -1) index = others.length;
          const current = [...root.children].indexOf(node);
          if (index !== current) {
            const beforeTop = node.getBoundingClientRect().top - offset;
            slide(() => root.insertBefore(node, others[index] || null));
            //keep the row under the pointer after it changed place in the list
            const afterTop = node.getBoundingClientRect().top - offset;
            node._shift = (node._shift || 0) + (beforeTop - afterTop);
            node.style.transform = `translateY(${lastY - startY + node._shift}px)`;
          }
        };
        const finish = cancel => {
          node.removeEventListener("pointermove", onMove);
          node.removeEventListener("pointerup", onUp);
          node.removeEventListener("pointercancel", onCancel);
          window.removeEventListener("keydown", onKey, true);
          node.classList.remove("dragging");
          root.classList.remove("sorting");
          node.style.transform = "";
          node._shift = 0;
          if (!dragging) return;
          if (cancel) slide(() => startOrder.forEach(n => root.append(n)));
          commit(node.item);
        };
        const onUp = () => finish(false);
        const onCancel = () => finish(true);
        const onKey = e => {
          if (e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            finish(true);
          }
        };
        node.addEventListener("pointermove", onMove);
        node.addEventListener("pointerup", onUp);
        node.addEventListener("pointercancel", onCancel);
        window.addEventListener("keydown", onKey, true);
      });
      return node;
    };

    order.forEach(item => root.append(row(item)));
    renumber();
    const wrap = make("div", "sortlist-wrap");
    const hint = make("p", "sortlist-hint", "Drag a row by its handle, or use the arrows. Alt with the up or down arrow key moves the row you are on.");
    wrap.append(hint, root, live);
    return wrap;
  }

  return {list};
})();
