// The omnibox in the bottom bar: one field for every way a job comes in.
//  - A job link (Greenhouse, Lever, Ashby or any https page) opens it, as the Find search bar does.
//  - Posting text (several lines, or a long paste) goes to Paste a job, ready to preview.
//  - Anything else is a search: it runs in Find jobs with the words typed.
// Cmd/Ctrl+K focuses it from anywhere. Text dropped on it is treated as a paste.
(() => {
  const form = document.getElementById("omnibox");
  const input = document.getElementById("omnibox-input");
  const $ = id => document.getElementById(id);

  const isMac = window.api?.platform === "darwin";
  $("omnibox-key").textContent = isMac ? "⌘K" : "Ctrl K";

  const looksLikeLink = text => {
    const t = text.trim();
    if (!t || /\s/.test(t)) return false;
    return /^https?:\/\//i.test(t) || /^(?:job-)?boards\.greenhouse\.io\//i.test(t) || /^grnh\.se\//i.test(t)
      || /^jobs\.(?:eu\.)?lever\.co\//i.test(t) || /^jobs\.ashbyhq\.com\//i.test(t);
  };
  //A posting is long or spans lines; a search is a few words.
  const looksLikePosting = text => /\n/.test(text.trim()) || text.trim().length > 200;

  //Sets a field's value the way typing would, so the view's own listeners see it.
  const fill = (field, value) => {
    field.value = value;
    field.dispatchEvent(new Event("input", {bubbles: true}));
  };

  function runSearch(text) {
    $("tab-find").click();
    fill($("search-keywords"), text);
    $("search-form").requestSubmit();
  }

  function openPosting(text) {
    $("tab-paste").click();
    const area = $("paste-text");
    fill(area, text);
    area.focus();
    area.setSelectionRange(0, 0);
    area.scrollTop = 0;
  }

  function route(text) {
    const value = String(text || "");
    if (!value.trim()) return;
    if (looksLikePosting(value)) openPosting(value);
    else runSearch(value.trim()); //a link runs through the search bar too, which opens it
    input.value = "";
    input.blur();
  }

  form.addEventListener("submit", event => {
    event.preventDefault();
    route(input.value);
  });

  //A search field drops line breaks, so a pasted posting is caught before it is flattened.
  input.addEventListener("paste", event => {
    const text = event.clipboardData?.getData("text/plain") || "";
    if (looksLikePosting(text)) {
      event.preventDefault();
      route(text);
    }
  });

  form.addEventListener("dragover", event => {
    if ([...(event.dataTransfer?.types || [])].includes("text/plain")) form.classList.add("dropping");
  });
  form.addEventListener("dragleave", () => form.classList.remove("dropping"));
  form.addEventListener("drop", event => {
    form.classList.remove("dropping");
    const text = event.dataTransfer?.getData("text/plain") || "";
    if (text) route(text);
  });

  input.addEventListener("keydown", event => {
    if (event.key === "Escape") {
      input.value = "";
      input.blur();
    }
  });

  document.addEventListener("keydown", event => {
    if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && event.key.toLowerCase() === "k") {
      event.preventDefault();
      input.focus();
      input.select();
    }
  });

  window.Omnibox = {route, looksLikeLink, looksLikePosting, focus: () => input.focus()};
})();
