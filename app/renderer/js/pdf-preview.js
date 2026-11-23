// An inline PDF preview. The backend makes the same PDF the export saves; main hands it over as base64 and the
// page shows it in the system's PDF viewer inside the window. Nothing is written to disk.
//Esc closes an expanded preview. One listener serves every preview, so drawing a preview again adds none.
document.addEventListener("keydown", event => {
  const open = document.querySelector(".pdf-preview.expanded");
  if (event.key === "Escape" && open) {
    event.stopPropagation();
    open.collapse?.();
  }
}, true);

window.PdfPreview = {
  //getBase64() gives the PDF as base64 (it may throw). Returns {node, load, dispose}.
  create(getBase64, {title = "PDF preview"} = {}) {
    const {make, pico, motion, icon} = window.UI;
    const state = make("div", {class: "pdf-state", role: "status"});
    const frame = make("iframe", {class: "pdf-frame", title, hidden: true});
    const expand = make("button", {type: "button", class: "pdf-expand", "aria-pressed": "false", title: "Show the PDF larger (Esc closes)"}, icon("expand"), make("span", {text: "Expand"}));
    const bar = make("div", {class: "pdf-bar"}, make("span", {class: "pdf-title", text: title}), expand);
    const body = make("div", {class: "pdf-body"}, state, frame);
    const node = make("div", {class: "pdf-preview"}, bar, body);
    const backdrop = make("div", {class: "pdf-backdrop"}); //sits in the page, behind the preview, so it does not dim it

    //Larger: the preview fills the window over a dimmed page. Esc or the button puts it back.
    //While it is expanded the preview moves to the document root. Inside the page an animated or clipped
    //ancestor can trap a fixed element behind the backdrop, and then Close cannot be clicked.
    const home = document.createComment("pdf-preview");
    const setExpanded = on => {
      if (on === node.classList.contains("expanded")) return;
      node.classList.toggle("expanded", on);
      document.body.classList.toggle("pdf-expanded", on); //drag regions would swallow clicks on the Close button
      if (on) {
        node.before(home);
        document.body.append(backdrop, node);
      } else {
        backdrop.remove();
        if (home.parentNode) home.replaceWith(node);
        else node.remove(); //the page it came from is gone
      }
      expand.setAttribute("aria-pressed", String(on));
      expand.replaceChildren(icon(on ? "collapse" : "expand"), make("span", {text: on ? "Close" : "Expand"}));
      if (on) expand.focus(); else if (node.isConnected) expand.blur();
    };
    expand.addEventListener("click", () => setExpanded(!node.classList.contains("expanded")));
    backdrop.addEventListener("click", () => setExpanded(false));
    node.collapse = () => setExpanded(false);
    let url = null;
    let token = 0;

    const dispose = () => {
      token++;
      setExpanded(false);
      if (url) URL.revokeObjectURL(url);
      url = null;
      frame.removeAttribute("src");
    };

    const message = (text, {error = false, file = null, busy = false} = {}) => {
      frame.hidden = true;
      state.hidden = false;
      state.classList.toggle("error", error);
      state.replaceChildren(
        busy ? make("span", {class: "spinner", "aria-hidden": "true"}) : (file ? pico(file) : null),
        make("p", {text}));
    };

    //Makes the PDF again. Call it each time the preview is shown, so it matches the text next to it.
    const load = async () => {
      const mine = ++token;
      message("Making the PDF...", {busy: true});
      try {
        const base64 = await getBase64();
        if (mine !== token) return;
        const bytes = Uint8Array.from(atob(base64), c => c.charCodeAt(0));
        if (url) URL.revokeObjectURL(url);
        url = URL.createObjectURL(new Blob([bytes], {type: "application/pdf"}));
        frame.src = `${url}#toolbar=0&navpanes=0&view=FitH`;
        frame.hidden = false;
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
