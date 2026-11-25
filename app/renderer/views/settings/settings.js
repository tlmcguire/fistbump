// The Settings view: accessibility and search preferences, history, storage, saved data, updates and
// about. Preferences are kept by window.Prefs; storage comes from the backend; updates and app details
// come from the main process.
window.Settings = {
  mount() {
    const $ = id => document.getElementById(id);
    const api = window.api;

    //"1.4 MB"; sizes are decimal, as the system's file manager shows them
    const formatBytes = bytes => {
      if (!Number.isFinite(bytes) || bytes <= 0) return "0 KB";
      const units = ["bytes", "KB", "MB", "GB"];
      let value = bytes;
      let unit = 0;
      while (value >= 1000 && unit < units.length - 1) {
        value /= 1000;
        unit++;
      }
      return unit === 0 ? `${value} bytes` : `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
    };

    //A button for something that cannot be undone asks to be pressed again within a few seconds.
    const confirmFirst = (button, label, action) => {
      let original = button.textContent;
      let timer = null;
      const reset = () => {
        clearTimeout(timer);
        button.classList.remove("confirming");
        button.textContent = original;
      };
      button.addEventListener("click", async () => {
        if (!button.classList.contains("confirming")) {
          original = button.textContent;
          button.classList.add("confirming");
          button.textContent = label;
          timer = setTimeout(reset, 4000);
          return;
        }
        reset();
        button.disabled = true;
        try {
          await action();
        } finally {
          button.disabled = false;
        }
      });
      button.addEventListener("blur", reset);
    };

    //Each preference control is bound to its key: it shows the saved value and saves changes at once.
    const bind = (id, key, read = control => control.value, write = (control, value) => { control.value = String(value); }) => {
      const control = $(id);
      write(control, window.Prefs.get(key));
      control.addEventListener("change", () => window.Prefs.set(key, read(control)));
    };
    const checked = [control => control.checked, (control, value) => { control.checked = value === true; }];
    //the shortcut list names this computer's modifier key
    if (api?.platform !== "darwin") document.querySelectorAll(".mod-key").forEach(k => { k.textContent = "Ctrl"; });
    bind("pref-theme", "theme");
    bind("pref-language", "language");
    //the page changes language in place, crossfading from the old words to the new ones
    $("pref-language").addEventListener("change", () => {
      window.UI.transition(() => window.I18n.change($("pref-language").value), "language");
    });
    bind("pref-text-size", "textSize");
    bind("pref-reduce-motion", "reduceMotion");
    bind("pref-high-contrast", "highContrast", ...checked);
    bind("pref-underline-links", "underlineLinks", ...checked);
    bind("pref-page-size", "pageSize", control => Number(control.value));
    bind("pref-default-sort", "defaultSort");
    bind("pref-show-logos", "showLogos", ...checked);
    bind("pref-check-updates", "notifyUpdates", ...checked);

    //A status line under a card. After a cleanup it shows the mascot tidying up beside what was cleared.
    const setStatus = (id, text, {cleaned = false} = {}) => {
      const line = $(id);
      line.classList.toggle("cleaned", cleaned);
      if (!cleaned) {
        line.textContent = text;
        return;
      }
      const mascot = document.createElement("img");
      mascot.className = "mascot";
      mascot.src = "views/graphics/mascots/28-cleaning-up.png";
      mascot.alt = "";
      const message = document.createElement("span");
      message.textContent = text;
      line.replaceChildren(mascot, message);
    };

    // ---------- Company watchlist ----------

    const parseBoardEntry = raw => {
      const s = String(raw || "").trim();
      if (!s) return null;
      const gh = s.match(/(?:job-)?boards\.greenhouse\.io\/([a-z0-9_-]+)/i);
      if (gh) return `greenhouse:${gh[1].toLowerCase()}`;
      const lv = s.match(/jobs\.(?:eu\.)?lever\.co\/([a-z0-9_.-]+)/i);
      if (lv) return `lever:${lv[1].toLowerCase()}`;
      const ash = s.match(/jobs\.ashbyhq\.com\/([a-z0-9_.-]+)/i);
      if (ash) return `ashby:${ash[1].toLowerCase()}`;
      const prefixed = s.match(/^(greenhouse|lever|ashby):([a-z0-9_.-]{1,64})$/i);
      if (prefixed) return `${prefixed[1].toLowerCase()}:${prefixed[2].toLowerCase()}`;
      if (/^[a-z0-9_-]{1,64}$/i.test(s)) return `greenhouse:${s.toLowerCase()}`;
      return null;
    };

    let customBoards = [];
    const renderWatchlist = () => {
      const list = $("watchlist-list");
      if (!list) return;
      list.replaceChildren(...customBoards.map(entry => {
        const li = document.createElement("li");
        li.className = "chip";
        li.append(entry + " ");
        const rm = document.createElement("button");
        rm.type = "button";
        rm.className = "filter-chip-x";
        rm.textContent = "×";
        rm.setAttribute("aria-label", `Remove ${entry}`);
        rm.addEventListener("click", async () => {
          const next = customBoards.filter(x => x !== entry);
          try {
            const res = await api?.settings?.put?.({"connectors.custom_boards": next});
            if (res?.error) throw new Error(res.error.message);
            customBoards = next;
            renderWatchlist();
            setStatus("watchlist-status", `Removed ${entry}.`);
          } catch (err) {
            setStatus("watchlist-status", err.message);
          }
        });
        li.append(rm);
        return li;
      }));
    };
    const showWatchlist = async () => {
      if (!api?.settings?.get) return;
      try {
        const vals = await api.settings.get();
        customBoards = Array.isArray(vals?.["connectors.custom_boards"]) ? vals["connectors.custom_boards"] : [];
        renderWatchlist();
      } catch {
        // settings unavailable
      }
    };
    $("watchlist-add")?.addEventListener("click", async () => {
      const input = $("watchlist-input");
      const parsed = parseBoardEntry(input?.value);
      if (!parsed) {
        setStatus("watchlist-status", "Enter a valid Greenhouse, Lever, or Ashby board token or URL.");
        return;
      }
      if (customBoards.includes(parsed)) {
        setStatus("watchlist-status", `${parsed} is already on your watchlist.`);
        return;
      }
      const next = [...customBoards, parsed];
      try {
        const res = await api?.settings?.put?.({"connectors.custom_boards": next});
        if (res?.error) throw new Error(res.error.message);
        customBoards = next;
        if (input) input.value = "";
        renderWatchlist();
        setStatus("watchlist-status", `Added ${parsed} to your watchlist.`);
      } catch (err) {
        setStatus("watchlist-status", err.message);
      }
    });

    // ---------- History ----------

    const readHistoryCount = () => {
      if (typeof window.JobParser?.historyCount === "function") return window.JobParser.historyCount();
      try {
        const list = JSON.parse(localStorage.getItem("fistbump.search-history") || "[]");
        return Array.isArray(list) ? list.length : 0;
      } catch {
        return 0;
      }
    };
    const hasLastListing = () => {
      try { return Boolean(localStorage.getItem("fistbump.last-listing")); } catch { return false; }
    };
    const showHistory = () => {
      const count = readHistoryCount();
      const hasListing = hasLastListing();
      $("history-summary").textContent = count
        ? `${count} recent ${count === 1 ? "search" : "searches"}, and the suggestions each search box remembers.`
        : hasListing
          ? "No recent searches, but your last opened listing is still remembered on Home."
          : "No recent searches.";
      $("settings-clear-history").disabled = count === 0 && !hasListing;
    };
    confirmFirst($("settings-clear-history"), "Clear all?", async () => {
      if (typeof window.JobParser?.clearHistory === "function") {
        window.JobParser.clearHistory();
      } else {
        try { localStorage.removeItem("fistbump.search-history"); } catch {}
        try { localStorage.removeItem("fistbump.last-listing"); } catch {}
        window.Suggest?.clearRecent?.();
        window.dispatchEvent(new Event("fistbump:history"));
      }
      showHistory();
      setStatus("history-status", "Recent searches and suggestions cleared.", {cleaned: true});
    });
    confirmFirst($("settings-reset-form"), "Reset?", async () => {
      window.JobParser?.resetForm?.();
      setStatus("history-status", "Search form cleared. Fistbump will open with it blank.", {cleaned: true});
    });

    // ---------- Storage ----------

    const storageStatus = (text, options) => setStatus("storage-status", text, options);
    const cleanupStatus = (text, options) => setStatus("cleanup-status", text, options);

    // The cleanup targets, each with the checkbox that turns it on.
    const CLEANUP_TARGETS = [
      { box: "cleanup-cache", target: "cache" },
      { box: "cleanup-partial-downloads", target: "partial_downloads" },
      { box: "cleanup-old-jobs", target: "old_jobs" },
      { box: "cleanup-trash", target: "trash" },
      { box: "cleanup-unused-models", target: "unused_models" },
    ];

    const showStorage = async () => {
      if (!api?.storage) {
        $("cache-size").textContent = "";
        $("data-dir").textContent = "Available in the app.";
        return;
      }
      const report = await api.storage.report();
      if (report?.error) {
        storageStatus(report.error.message);
        return;
      }
      $("cache-size").textContent = `Using ${formatBytes(report.cache_bytes)}.`;
      $("db-size").textContent = `Using ${formatBytes(report.db_bytes)}.`;
      $("models-size").textContent = report.models_bytes ? `Using ${formatBytes(report.models_bytes)}.` : "";
      $("partial-size").textContent = report.partial_downloads_bytes ? `Using ${formatBytes(report.partial_downloads_bytes)}.` : "";
      $("total-size").textContent = `Using ${formatBytes(report.total_bytes)}.`;
      $("data-dir").textContent = report.data_dir;
    };

    // The payload the backend takes. The days a job must be older than are left out when the
    // field is empty, so the backend uses the app's own setting.
    const cleanupPayload = dryRun => {
      const payload = {
        targets: CLEANUP_TARGETS.filter(t => $(t.box)?.checked).map(t => t.target),
        dry_run: dryRun,
      };
      const days = Number($("cleanup-days")?.value);
      if (Number.isInteger(days) && days >= 1) payload.older_than_days = days;
      return payload;
    };

    // The answer says what would go, or what went. The job targets free no space on their own:
    // the database file only shrinks after the cleanup that follows.
    const describeCleanup = (result, dryRun) => {
      const removed = result.removed || {};
      const bits = [];
      if (removed.old_jobs) bits.push(`${removed.old_jobs} old ${removed.old_jobs === 1 ? "job" : "jobs"}`);
      if (removed.trash) bits.push(`${removed.trash} ${removed.trash === 1 ? "job" : "jobs"} in the Trash`);
      if (removed.files) bits.push(`${removed.files} ${removed.files === 1 ? "file" : "files"}`);
      const what = bits.length ? bits.join(", ") : (result.freed_bytes ? `${formatBytes(result.freed_bytes)} of cached files` : "nothing");
      const space = result.freed_bytes ? ` That ${dryRun ? "frees" : "freed"} ${formatBytes(result.freed_bytes)}.` : "";
      return `${dryRun ? "Would remove" : "Removed"} ${what}.${space}`;
    };

    // The preview says what would go without touching anything. The cleanup does it for real.
    const runCleanup = async dryRun => {
      if (!api?.storage) return;
      const payload = cleanupPayload(dryRun);
      if (!payload.targets.length) {
        cleanupStatus("Choose what to remove first.");
        return;
      }
      const result = await api.storage.cleanup(payload);
      if (result?.error) {
        cleanupStatus(result.error.message);
        return;
      }
      cleanupStatus(describeCleanup(result, dryRun), {cleaned: !dryRun});
      if (!dryRun) {
        await showStorage();
        const clearListing = payload.targets.includes("old_jobs") || payload.targets.includes("trash");
        window.dispatchEvent(new CustomEvent("fistbump:storage-cleaned", {
          detail: {targets: payload.targets, clearListing, invalidateMatches: true},
        }));
      }
    };

    $("cleanup-preview").addEventListener("click", () => runCleanup(true));
    // Removing cannot be undone, so the button asks to be pressed again.
    confirmFirst($("cleanup-run"), "Remove this?", () => runCleanup(false));

    $("show-data-folder").addEventListener("click", async () => {
      const result = await api?.app?.showDataFolder?.();
      if (result?.error) storageStatus(result.error.message);
    });
    if (api?.platform === "darwin") $("show-data-folder").textContent = "Show in Finder";
    if (api?.platform === "win32") $("show-data-folder").textContent = "Show in Explorer";

    // ---------- Local JSON backup & restore ----------
    $("backup-export")?.addEventListener("click", async () => {
      storageStatus("Creating local backup\u2026");
      try {
        const fb = window.FistBumpAPI;
        const [master, resumes, jobs, apps, savedSearches, settings] = await Promise.all([
          fb.resume.get().catch(() => null),
          fb.resumes.list().catch(() => []),
          fb.jobs.list("", {archived: "include"}).catch(() => []),
          fb.applications.list({archived: "include"}).catch(() => []),
          api?.savedSearches?.list ? fb.savedSearches.list().catch(() => []) : [],
          api?.settings?.get ? fb.request(() => api.settings.get()).catch(() => ({})) : {},
        ]);
        const bundle = {
          schema: "fistbump-backup-v1",
          exported_at: new Date().toISOString(),
          master,
          resumes,
          jobs,
          applications: apps,
          saved_searches: savedSearches,
          settings,
        };
        const stamp = new Date().toISOString().slice(0, 10);
        const blob = new Blob([JSON.stringify(bundle, null, 2)], {type: "application/json;charset=utf-8"});
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `Fistbump-Backup-${stamp}.json`;
        a.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        storageStatus(`Backup exported (${jobs.length} jobs, ${apps.length} applications).`);
      } catch (err) {
        storageStatus(err.message);
      }
    });

    $("backup-restore")?.addEventListener("click", () => $("backup-file")?.click());
    $("backup-file")?.addEventListener("change", async event => {
      const file = event.target.files?.[0];
      event.target.value = "";
      if (!file) return;
      storageStatus("Restoring backup\u2026");
      try {
        const text = await file.text();
        const data = JSON.parse(text);
        if (!data || data.schema !== "fistbump-backup-v1") {
          throw new Error("That file is not a valid Fistbump backup.");
        }
        const fb = window.FistBumpAPI;
        if (data.master?.profile) {
          await fb.resume.importApply({
            draft: {
              profile: data.master.profile,
              experiences: data.master.experiences || [],
              education: data.master.education || [],
              skills: data.master.profile.skills || [],
            },
            replace: false,
          });
        }
        if (data.settings && typeof data.settings === "object" && api?.settings?.put) {
          const safe = {};
          for (const k of ["connectors.custom_boards", "resume.section_order", "resume.custom_sections", "jobs.retention_days", "jobs.trash_days"]) {
            if (k in data.settings) safe[k] = data.settings[k];
          }
          if (Object.keys(safe).length) await fb.request(() => api.settings.put(safe));
        }
        if (Array.isArray(data.saved_searches) && api?.savedSearches?.create) {
          for (const ss of data.saved_searches) {
            if (ss.name && ss.query_json) {
              await fb.savedSearches.create({name: ss.name, query_json: ss.query_json, seen_keys: ss.seen_keys || []}).catch(() => {});
            }
          }
        }
        showWatchlist();
        window.dispatchEvent(new CustomEvent("fistbump:storage-cleaned", {detail: {clearListing: false, invalidateMatches: true}}));
        storageStatus("Backup restored (resume profile, custom sections, watchlist, and saved searches).");
      } catch (err) {
        storageStatus(err.message || "Could not restore backup.");
      }
    });

    // ---------- AI suggestions ----------

    const ai = window.FistBumpAPI.ai;
    const aiStatus = (text, isError = false) => {
      const line = $("ai-status");
      line.textContent = text;
      line.classList.toggle("error", isError);
    };
    const aiLocalText = s => {
      if (!s.local.available) return "The local model is not included in this build, so suggestions use your remote provider or built-in rules.";
      if (!s.local.installed) return "Not downloaded yet. Pick a model below and download it.";
      return s.local.running ? "Ready and running on this computer." : "Installed on this computer. It starts when you generate suggestions.";
    };
    const showAi = async () => {
      if (!api?.ai) return;
      try {
        const s = await ai.status();
        $("ai-mode").value = s.mode;
        $("ai-local-status").textContent = aiLocalText(s);
        $("ai-test-local").disabled = !s.local.available;
        $("ai-base-url").value = s.remote.base_url || "";
        $("ai-model").value = s.remote.model || "";
        $("ai-key").value = "";
        $("ai-key").placeholder = s.remote.has_key ? "A key is saved. Type to replace it." : "Optional. Leave empty for Ollama or another provider on this computer";
        $("ai-test-remote").disabled = !s.remote.configured;
        //"Remote provider only" cannot run until there is a provider to run on
        const remoteOnly = $("ai-mode").querySelector('option[value="remote"]');
        remoteOnly.disabled = !s.remote.configured && s.mode !== "remote";
        remoteOnly.title = remoteOnly.disabled ? "Add a remote provider first" : "";
        $("ai-clear-remote").disabled = !s.remote.configured;
        window.dispatchEvent(new CustomEvent("fistbump:ai", {detail: s}));
      } catch (error) {
        aiStatus(error.message, true);
      }
    };
    $("ai-mode").addEventListener("change", async () => {
      try {
        await ai.setMode($("ai-mode").value);
        aiStatus("Saved.");
        showAi();
      } catch (error) {
        aiStatus(error.message, true);
        showAi();
      }
    });
    $("ai-save-remote").addEventListener("click", async () => {
      try {
        const result = await ai.setRemote({base_url: $("ai-base-url").value, model: $("ai-model").value, api_key: $("ai-key").value});
        aiStatus($("ai-key").value && !result.key_remembered
          ? "Saved for this session. This computer cannot store the key securely, so enter it again after restarting."
          : "Remote provider saved. Suggestions will use it when the mode allows.");
        showAi();
      } catch (error) {
        aiStatus(error.message, true);
      }
    });
    const runTest = async engine => {
      const started = Date.now();
      aiStatus(`Testing ${engine} engine… 0 s`);
      const timer = setInterval(() => {
        const sec = Math.round((Date.now() - started) / 1000);
        aiStatus(sec >= 15
          ? `Testing ${engine} engine… ${sec} s (taking longer than usual — model may be loading or stalled)`
          : `Testing ${engine} engine… ${sec} s`);
      }, 1000);
      let timeoutId = null;
      try {
        const r = await Promise.race([
          ai.test(engine),
          new Promise((_, reject) => {
            timeoutId = setTimeout(() => reject(new Error(`The ${engine} engine stalled after 45 s without responding.`)), 45000);
          }),
        ]);
        const sec = r.latency_ms ? (r.latency_ms / 1000).toFixed(1) : ((Date.now() - started) / 1000).toFixed(1);
        aiStatus(r.ok ? `The ${engine} engine answered in ${sec} s.` : `The ${engine} engine did not answer (${sec} s): ${r.problem}`, !r.ok);
      } catch (error) {
        aiStatus(error.message, true);
      } finally {
        clearInterval(timer);
        clearTimeout(timeoutId);
      }
    };
    $("ai-test-local").addEventListener("click", () => runTest("local"));
    $("ai-test-remote").addEventListener("click", () => runTest("remote"));
    confirmFirst($("ai-clear-remote"), "Remove provider?", async () => {
      try {
        await ai.clearRemote();
        aiStatus("Remote provider and its saved key removed.");
      } catch (error) {
        aiStatus(error.message, true);
      }
      showAi();
    });

    // ---------- Local models ----------

    const models = window.FistBumpAPI.models;
    const modelList = $("model-list");
    let downloadTimer = null;
    let currentDownload = null; // {modelId, downloadId, percent}

    //The catalog says what a model needs, so the choice is made before the download starts.
    const modelRow = model => {
      const row = document.createElement("div");
      row.className = "model-row";

      const info = document.createElement("div");
      info.className = "model-info";
      const name = document.createElement("div");
      name.className = "model-name";
      name.textContent = model.name;
      const meta = document.createElement("p");
      meta.className = "model-meta";
      //the parts are joined with a middle dot, which is how the language files split a line
      meta.textContent = [
        `${(model.size_bytes / 1e9).toFixed(1)} GB`,
        `Memory needed: ${model.min_ram_gb} GB`,
        model.license,
        model.fits_this_machine ? "" : "More memory than this computer has",
      ].filter(Boolean).join(" \u00b7 ");
      const state = document.createElement("p");
      state.className = "model-state";
      if (model.selected) state.textContent = "This is the model in use.";
      else if (model.downloading) state.textContent = `Downloading… ${currentDownload?.modelId === model.id ? currentDownload.percent : 0}%`;
      else if (model.installed) state.textContent = "Downloaded on this computer.";
      else state.textContent = "Not downloaded yet.";
      info.append(name, meta, state);

      const actions = document.createElement("div");
      actions.className = "setting-actions";
      const add = (text, className, run) => {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = text;
        if (className) button.className = className;
        button.addEventListener("click", run);
        actions.append(button);
        return button;
      };

      if (!model.installed && !model.downloading) add("Download", "primary", () => startDownload(model));
      if (model.downloading) add("Cancel", "", () => cancelDownload());
      if (model.installed) {
        if (!model.selected) add("Use this model", "", () => useModel(model));
        const remove = add("Delete", "", () => {});
        confirmFirst(remove, "Delete?", async () => {
          try {
            await models.delete(model.id);
            aiStatus("Model deleted from this computer.");
          } catch (error) {
            aiStatus(error.message, true);
          }
          await showModels();
          showAi();
        });
      }
      row.append(info, actions);

      //The bar of a download in flight, filled in as the bytes arrive.
      if (model.downloading && currentDownload?.modelId === model.id) {
        const bar = document.createElement("progress");
        bar.className = "model-progress";
        bar.max = 100;
        bar.value = currentDownload.percent;
        row.append(bar);
      }
      return row;
    };

    const showModels = async () => {
      if (!models || !modelList) return;
      let list = [];
      try {
        list = await models.list();
      } catch (error) {
        aiStatus(error.message, true);
        return;
      }
      modelList.replaceChildren(...list.map(modelRow));
    };

    //The download runs in the backend, so the page only asks how far it has come.
    const startDownload = async model => {
      try {
        const result = await models.download(model.id);
        currentDownload = {modelId: model.id, downloadId: result.download_id, percent: 0};
        aiStatus(`Downloading ${model.name}. It stays on this computer.`);
        pollDownload();
      } catch (error) {
        aiStatus(error.message, true);
      }
    };

    const pollDownload = async () => {
      if (!currentDownload) return;
      try {
        const status = await models.downloadStatus(currentDownload.downloadId);
        const total = status.total || 0;
        currentDownload.percent = total ? Math.min(100, Math.round((status.bytes || 0) / total * 100)) : 0;
        if (status.state === "running" || status.state === "verifying") {
          await showModels();
          downloadTimer = setTimeout(pollDownload, 1000);
          return;
        }
        clearTimeout(downloadTimer);
        downloadTimer = null;
        currentDownload = null;
        if (status.state === "done") aiStatus("Model downloaded. It is ready to use.");
        else if (status.state === "failed") aiStatus(status.error || "The download failed.", true);
        else aiStatus("Download cancelled.");
        await showModels();
        showAi();
      } catch (error) {
        clearTimeout(downloadTimer);
        downloadTimer = null;
        currentDownload = null;
        aiStatus(error.message, true);
        await showModels();
      }
    };

    const cancelDownload = async () => {
      if (!currentDownload) return;
      try {
        await models.cancelDownload(currentDownload.downloadId);
        aiStatus("Download cancelled. The part already downloaded is kept, so the next one continues from there.");
      } catch (error) {
        aiStatus(error.message, true);
      }
    };

    //The model in use is a setting, so it is saved the same way as the other ones.
    const useModel = async model => {
      try {
        const result = await api.settings.put({"ai.selected_model": model.id});
        if (result?.error) throw new Error(result.error.message);
        aiStatus(`${model.name} is the model in use now.`);
      } catch (error) {
        aiStatus(error.message, true);
      }
      await showModels();
      showAi();
    };

    // ---------- Updates ----------

    let releaseUrl = "";
    let downloading = false;
    const megabytes = bytes => `${(bytes / 1048576).toFixed(bytes < 10485760 ? 1 : 0)} MB`;

    //one place decides which update buttons show: Download (a download is possible), Cancel (one is running),
    //Install (it is saved), or just the release page when no installer is published for this computer
    const updateButtons = ({available, downloadable, saved}) => {
      $("update-dot").hidden = !available;
      $("open-release").hidden = !available;
      $("download-update").hidden = !(available && downloadable && !downloading && !saved);
      $("cancel-update").hidden = !downloading;
      $("install-update").hidden = !saved;
    };

    const showUpdate = result => {
      downloading = false;
      const available = result.status === "available";
      releaseUrl = available ? result.url : "";
      $("update-status").textContent = available && result.downloadable && result.size
        ? `${result.message} The installer is ${megabytes(result.size)}.`
        : result.message || "";
      updateButtons({available, downloadable: available && result.downloadable});
    };

    //the status line in Settings, and the popup that started the update when there is one
    let mirror = null; //{text, bar, close}
    const setUpdateStatus = (text, fraction) => {
      $("update-status").textContent = text;
      if (!mirror) return;
      mirror.text.textContent = text;
      if (fraction === undefined) return;
      if (fraction === null) mirror.bar.removeAttribute("value");
      else mirror.bar.value = fraction;
    };

    window.api?.app?.onUpdateProgress?.((received, total) => {
      if (!downloading) return;
      setUpdateStatus(total
        ? `Downloading the update... ${Math.floor(received * 100 / total)}% (${megabytes(received)} of ${megabytes(total)})`
        : `Downloading the update... ${megabytes(received)}`, total ? received / total : null);
    });

    //Update and restart: download the installer, then install it over this app and start the new version.
    //Fistbump quits as the install begins. When it cannot install itself (say it runs from the disk image),
    //the status says why and Open installer opens the download instead.
    const downloadUpdate = async () => {
      downloading = true;
      updateButtons({available: true, downloadable: true});
      setUpdateStatus("Downloading the update...");
      const result = await api.app.downloadUpdate();
      downloading = false;
      if (result?.error) {
        setUpdateStatus(result.error.code === "canceled" ? "The download was canceled." : result.error.message);
        updateButtons({available: true, downloadable: true});
        return;
      }
      setUpdateStatus("Installing the update. Fistbump will close and open again in a moment.");
      updateButtons({available: true, downloadable: true, saved: true});
      $("install-update").hidden = true;
      const installed = await api.app.installUpdate();
      if (installed?.error) {
        setUpdateStatus(`${installed.error.message} The installer, ${result.name}, is in your Downloads folder.`);
        $("install-update").hidden = false;
      }
    };

    const checkForUpdates = async () => {
      if (downloading) return; //a check would forget the installer being downloaded
      if (!api?.app?.checkForUpdates) {
        showUpdate({message: "Updates can be checked in the app."});
        return;
      }
      $("check-updates").disabled = true;
      $("update-status").textContent = "Checking for updates...";
      try {
        const result = await api.app.checkForUpdates();
        showUpdate(result?.error ? {message: result.error.message} : result);
        return result;
      } finally {
        $("check-updates").disabled = false;
      }
    };

    //When Fistbump opens it looks for a new version and, if there is one the user has not skipped, says so in
    //a dialog: Update and restart, or Not now. "Don't remind me about this version" skips that version only;
    //a later one is announced again. The Settings switch turns the whole thing off.
    const notifyOnLaunch = async () => {
      const result = await checkForUpdates();
      if (result?.status !== "available" || !result.latest || result.latest === window.Prefs.get("skippedUpdate")) return;
      const {make} = window.UI;
      const skip = make("input", {type: "checkbox", id: "skip-update"});
      const dialog = make("dialog", {class: "ask update-notice", "aria-label": "Update available"});
      const close = () => {
        if (skip.checked) window.Prefs.set("skippedUpdate", result.latest);
        dialog.close();
        dialog.remove();
      };
      const later = make("button", {type: "button", text: "Not now", onclick: close});
      const update = make("button", {type: "button", class: "primary", text: result.downloadable ? "Update and restart" : "See what's new", onclick: async () => {
        if (!result.downloadable) { close(); api.app.openReleases(result.url); return; }
        //the popup turns into the progress view, so the user sees the download where they clicked
        const text = make("p", {role: "status"});
        const bar = make("progress", {max: "1", "aria-label": "Update download"});
        const cancel = make("button", {type: "button", text: "Cancel", onclick: () => api?.app?.cancelUpdate?.()});
        const done = make("button", {type: "button", text: "Close", hidden: true, onclick: () => { mirror = null; dialog.close(); dialog.remove(); }});
        dialog.replaceChildren(make("h2", {text: `Updating to Fistbump ${result.latest}`}), text, bar, make("div", {class: "ask-actions"}, cancel, done));
        dialog.addEventListener("cancel", event => event.preventDefault());
        mirror = {text, bar};
        await downloadUpdate();
        //reaching here means it was canceled or failed (a successful install quits the app)
        cancel.hidden = true;
        done.hidden = false;
        done.focus();
      }});
      dialog.append(
        make("h2", {text: `Fistbump ${result.latest} is available`}),
        make("p", {text: result.downloadable
          ? `You have ${result.current}. Fistbump can download the new version, install it and open again.`
          : `You have ${result.current}. No installer is published for this computer, so the release page has it.`}),
        make("label", {class: "update-skip"}, skip, " Don't remind me about this version"),
        make("div", {class: "ask-actions"}, later, update));
      window.UI.wireDialogClose(dialog, close);
      dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
      document.body.append(dialog);
      dialog.showModal();
      update.focus();
    };
    $("check-updates").addEventListener("click", checkForUpdates);
    $("open-release").addEventListener("click", () => api?.app?.openReleases?.(releaseUrl));
    $("download-update").addEventListener("click", downloadUpdate);
    $("cancel-update").addEventListener("click", () => api?.app?.cancelUpdate?.());
    $("install-update").addEventListener("click", async () => {
      const result = await api.app.openUpdate();
      if (result?.error) $("update-status").textContent = result.error.message;
    });

    // ---------- About ----------

    const showAbout = async () => {
      const info = await api?.app?.info?.();
      if (!info || info.error) return;
      $("app-version").textContent = info.version;
      const platform = {darwin: "macOS", win32: "Windows", linux: "Linux"}[info.platform] || info.platform;
      $("about-details").textContent = `Version ${info.version} for ${platform} (${info.arch}). Tauri ${info.tauri}.`;
    };

    //Shown every time the view opens, so sizes and counts are current.
    window.Settings.refresh = () => {
      showWatchlist();
      showHistory();
      showStorage();
      showAi();
      showModels();
    };

    //The backend came back after a crash. It has a new port and token, so the page asks it again.
    window.api?.onBackendRestarted?.(() => window.Settings.refresh());

    showAbout();
    if (window.Prefs.get("notifyUpdates")) setTimeout(notifyOnLaunch, 2500); //after the first view has settled
  },
  refresh() {},
};
