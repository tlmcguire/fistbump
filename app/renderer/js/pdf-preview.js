// An inline PDF preview. The backend makes the same PDF the export saves; main hands it over as base64
// and the page renders it locally with PDF.js onto <canvas> (identical across macOS WKWebView, Windows
// WebView2, and Linux WebKitGTK), falling back to an <iframe> only if PDF.js is not synced yet.
// Esc closes an expanded preview. One listener serves every preview, so drawing a preview again adds none.
document.addEventListener("keydown", event => {
  const open = document.querySelector(".pdf-preview.expanded");
  if (event.key === "Escape" && open) {
    event.stopPropagation();
    open.collapse?.();
  }
}, true);

let pdfjsPromise = null;
function loadPdfJs() {
  if (!pdfjsPromise) {
    pdfjsPromise = (async () => {
      try {
        const modUrl = new URL("vendor/pdfjs/pdf.min.mjs", window.location.href).href;
        const workerUrl = new URL("vendor/pdfjs/pdf.worker.min.mjs", window.location.href).href;
        const lib = await import(modUrl);
        if (lib?.GlobalWorkerOptions) lib.GlobalWorkerOptions.workerSrc = workerUrl;
        return lib;
      } catch {
        return null;
      }
    })();
  }
  return pdfjsPromise;
}

// Tauri v2 drag regions use the `data-tauri-drag-region` HTML attribute rather than CSS
// `-webkit-app-region`, so expanding a PDF overlay must toggle the attribute off while open.
function setDragRegionsEnabled(enabled) {
  for (const el of document.querySelectorAll(".drag")) {
    if (!enabled) {
      if (el.hasAttribute("data-tauri-drag-region")) {
        el.dataset.hadTauriDrag = "1";
        el.removeAttribute("data-tauri-drag-region");
      }
    } else if (el.dataset.hadTauriDrag === "1") {
      delete el.dataset.hadTauriDrag;
      el.setAttribute("data-tauri-drag-region", "");
    }
  }
}

window.PdfPreview = {
  // getBase64() gives the PDF as base64 (it may throw). Returns {node, load, dispose}.
  create(getBase64, {title = "PDF preview"} = {}) {
    const {make, pico, motion, icon} = window.UI;
    const state = make("div", {class: "pdf-state", role: "status"});
    const pages = make("div", {class: "pdf-pages", hidden: true});
    const frame = make("iframe", {class: "pdf-frame", title, hidden: true});
    const expand = make("button", {type: "button", class: "pdf-expand", "aria-pressed": "false", title: "Show the PDF larger (Esc closes)"}, icon("expand"), make("span", {text: "Expand"}));
    const bar = make("div", {class: "pdf-bar"}, make("span", {class: "pdf-title", text: title}), expand);
    const body = make("div", {class: "pdf-body"}, state, pages, frame);
    const node = make("div", {class: "pdf-preview"}, bar, body);
    const backdrop = make("div", {class: "pdf-backdrop"}); // sits in the page, behind the preview, so it does not dim it

    let url = null;
    let token = 0;
    let lastBytes = null;

    const renderCanvasPages = async (bytes, mine) => {
      try {
      const pdfjs = await loadPdfJs();
      if (!pdfjs?.getDocument || mine !== token) return false;
      const loadingTask = pdfjs.getDocument({data: bytes.slice()});
      const pdf = await loadingTask.promise;
      if (mine !== token) {
        pdf.destroy?.();
        return true;
      }
      pages.hidden = false;
      frame.hidden = true;
      state.hidden = true;
      const containerWidth = Math.max(240, (pages.clientWidth || body.clientWidth || 480) - 24);
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      const frag = document.createDocumentFragment();
      for (let num = 1; num <= pdf.numPages; num++) {
        if (mine !== token) {
          pdf.destroy?.();
          return true;
        }
        const page = await pdf.getPage(num);
        const baseViewport = page.getViewport({scale: 1});
        const scale = containerWidth / baseViewport.width;
        const cssViewport = page.getViewport({scale});
        const renderViewport = page.getViewport({scale: scale * dpr});
        const canvas = document.createElement("canvas");
        canvas.className = "pdf-page-canvas";
        canvas.width = Math.floor(renderViewport.width);
        canvas.height = Math.floor(renderViewport.height);
        canvas.style.width = `${Math.floor(cssViewport.width)}px`;
        const ctx = canvas.getContext("2d");
        await page.render({canvasContext: ctx, viewport: renderViewport}).promise;
        frag.appendChild(canvas);
      }
      if (mine === token) {
        pages.replaceChildren(frag);
        if (motion()) {
          pages.classList.remove("reveal");
          void pages.offsetWidth;
          pages.classList.add("reveal");
        }
      }
      pdf.destroy?.();
      return true;
      } catch {
        return false;
      }
    };

    // Larger: the preview fills the window over a dimmed page. Esc or the button puts it back.
    // While it is expanded the preview moves to the document root. Inside the page an animated or clipped
    // ancestor can trap a fixed element behind the backdrop, and then Close cannot be clicked.
    const home = document.createComment("pdf-preview");
    const setExpanded = on => {
      if (on === node.classList.contains("expanded")) return;
      node.classList.toggle("expanded", on);
      document.body.classList.toggle("pdf-expanded", on); // drag regions would swallow clicks on the Close button
      setDragRegionsEnabled(!on);
      if (on) {
        node.before(home);
        document.body.append(backdrop, node);
      } else {
        backdrop.remove();
        if (home.parentNode) home.replaceWith(node);
        else node.remove(); // the page it came from is gone
      }
      expand.setAttribute("aria-pressed", String(on));
      expand.replaceChildren(icon(on ? "collapse" : "expand"), make("span", {text: on ? "Close" : "Expand"}));
      if (on) expand.focus(); else if (node.isConnected) expand.blur();
      // Refit canvas pages to the new width when toggling expanded/collapsed mode.
      if (lastBytes && !pages.hidden) {
        const mine = ++token;
        requestAnimationFrame(() => {
          renderCanvasPages(lastBytes, mine).catch(() => {});
        });
      }
    };
    expand.addEventListener("click", () => setExpanded(!node.classList.contains("expanded")));
    backdrop.addEventListener("click", () => setExpanded(false));
    node.collapse = () => setExpanded(false);

    const dispose = () => {
      token++;
      lastBytes = null;
      setExpanded(false);
      if (url) URL.revokeObjectURL(url);
      url = null;
      pages.replaceChildren();
      pages.hidden = true;
      frame.removeAttribute("src");
    };

    const message = (text, {error = false, file = null, busy = false} = {}) => {
      frame.hidden = true;
      pages.hidden = true;
      state.hidden = false;
      state.classList.toggle("error", error);
      state.replaceChildren(
        busy ? make("span", {class: "spinner", "aria-hidden": "true"}) : (file ? pico(file) : null),
        make("p", {text}));
    };

    // Makes the PDF again. Call it each time the preview is shown, so it matches the text next to it.
    const load = async () => {
      const mine = ++token;
      message("Making the PDF...", {busy: true});
      try {
        const base64 = await getBase64();
        if (mine !== token) return;
        const bytes = Uint8Array.from(atob(base64), c => c.charCodeAt(0));
        lastBytes = bytes;
        const renderedWithPdfJs = await renderCanvasPages(bytes, mine);
        if (renderedWithPdfJs || mine !== token) return;

        // Fallback when PDF.js vendor files are not synced yet (e.g. before npm install).
        if (url) URL.revokeObjectURL(url);
        url = URL.createObjectURL(new Blob([bytes], {type: "application/pdf"}));
        frame.src = `${url}#toolbar=0&navpanes=0&view=FitH`;
        frame.hidden = false;
        pages.hidden = true;
        state.hidden = true;
        if (motion()) {
          frame.classList.remove("reveal");
          void frame.offsetWidth;
          frame.classList.add("reveal");
        }
      } catch (error) {
        if (mine !== token) return;
        message(error.details?.fallback === "md"
          ? `The PDF could not be made (${error.message}). You can still export Markdown.`
          : error.message, {error: true, file: "29-something-went-wrong.png"});
      }
    };

    return {node, load, dispose};
  },
};
