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
      const original = button.textContent;
      let timer = null;
      const reset = () => {
        clearTimeout(timer);
        button.classList.remove("confirming");
        button.textContent = original;
      };
      button.addEventListener("click", async () => {
        if (!button.classList.contains("confirming")) {
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

    //The saved-data buttons stay off until their features exist. Their targets are already named so the
    //backend's cleanup route can take them as soon as it supports them.
    for (const button of document.querySelectorAll(".setting.future button.planned")) {
      button.title = "Available when this feature is added";
    }

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
    };

    showAbout();
    if (window.Prefs.get("checkUpdatesOnLaunch")) checkForUpdates();
  },
  refresh() {},
};
