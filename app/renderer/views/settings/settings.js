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
    bind("pref-check-updates", "checkUpdatesOnLaunch", ...checked);

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

    // ---------- History ----------

    const showHistory = () => {
      const count = window.JobParser?.historyCount?.() ?? 0;
      $("history-summary").textContent = count
        ? `${count} recent ${count === 1 ? "search" : "searches"}, and the suggestions each search box remembers.`
        : "No recent searches.";
      $("settings-clear-history").disabled = count === 0;
    };
    confirmFirst($("settings-clear-history"), "Clear all?", async () => {
      window.JobParser?.clearHistory?.();
      showHistory();
      setStatus("history-status", "Recent searches and suggestions cleared.", {cleaned: true});
    });
    confirmFirst($("settings-reset-form"), "Reset?", async () => {
      window.JobParser?.resetForm?.();
      setStatus("history-status", "Search form cleared. Fistbump will open with it blank.", {cleaned: true});
    });

    // ---------- Storage ----------

    const storageStatus = (text, options) => setStatus("storage-status", text, options);

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
      $("data-dir").textContent = report.data_dir;
      $("clear-cache").disabled = report.cache_bytes === 0;
    };

    confirmFirst($("clear-cache"), "Clear cache?", async () => {
      if (!api?.storage) return;
      const result = await api.storage.cleanup(["cache"]);
      if (result?.error) storageStatus(result.error.message);
      else storageStatus(`Cleared ${formatBytes(result.freed_bytes)}. The next search will take a little longer.`, {cleaned: true});
      await showStorage();
    });

    $("show-data-folder").addEventListener("click", async () => {
      const result = await api?.app?.showDataFolder?.();
      if (result?.error) storageStatus(result.error.message);
    });
    if (api?.platform === "darwin") $("show-data-folder").textContent = "Show in Finder";
    if (api?.platform === "win32") $("show-data-folder").textContent = "Show in Explorer";

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
      aiStatus("Testing...");
      try {
        const r = await ai.test(engine);
        aiStatus(r.ok ? `The ${engine} engine answered in ${(r.latency_ms / 1000).toFixed(1)} seconds.` : `The ${engine} engine did not answer: ${r.problem}`, !r.ok);
      } catch (error) {
        aiStatus(error.message, true);
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
    const showUpdate = result => {
      $("update-status").textContent = result.message || "";
      releaseUrl = result.status === "available" ? result.url : "";
      $("open-release").hidden = !releaseUrl;
      $("update-dot").hidden = !releaseUrl;
    };

    const checkForUpdates = async () => {
      if (!api?.app?.checkForUpdates) {
        showUpdate({message: "Updates can be checked in the app."});
        return;
      }
      $("check-updates").disabled = true;
      $("update-status").textContent = "Checking for updates...";
      try {
        const result = await api.app.checkForUpdates();
        showUpdate(result?.error ? {message: result.error.message} : result);
      } finally {
        $("check-updates").disabled = false;
      }
    };
    $("check-updates").addEventListener("click", checkForUpdates);
    $("open-release").addEventListener("click", () => api?.app?.openReleases?.(releaseUrl));

    // ---------- About ----------

    const showAbout = async () => {
      const info = await api?.app?.info?.();
      if (!info || info.error) return;
      $("app-version").textContent = info.version;
      const platform = {darwin: "macOS", win32: "Windows", linux: "Linux"}[info.platform] || info.platform;
      $("about-details").textContent = `Version ${info.version} for ${platform} (${info.arch}). Electron ${info.electron}, Chromium ${info.chrome.split(".")[0]}.`;
    };

    //Shown every time the view opens, so sizes and counts are current.
    window.Settings.refresh = () => {
      showHistory();
      showStorage();
      showAi();
      showModels();
    };

    showAbout();
    if (window.Prefs.get("checkUpdatesOnLaunch")) checkForUpdates();
  },
  refresh() {},
};
