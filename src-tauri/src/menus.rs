//! The native menu bar. On macOS it follows the platform's layout (Fistbump, File, Edit, View, Go, Window, Help),
//! with the app's own actions in their usual places and their usual keys; on Windows and Linux the same menus, with
//! Settings and Quit under File.
//!
//! Items that act on the page have ids starting with `do:`; they are passed to the page as the `app:menu` event,
//! which runs the same code as the matching button. Ticks (day or night, bar at the top) follow Settings through the
//! `app:setMenuState` channel, so the menu and the page never disagree.

use std::sync::atomic::Ordering;
use std::sync::Mutex;

use serde_json::Value;
use tauri::menu::{CheckMenuItem, CheckMenuItemBuilder, Menu, MenuBuilder, MenuItemBuilder, PredefinedMenuItem, SubmenuBuilder};
use tauri::{AppHandle, Emitter, Manager, Wry};
use tauri_plugin_opener::OpenerExt;

use crate::AppState;

pub const MENU_SETTINGS: &str = "app-settings";
pub const MENU_QUIT: &str = "app-quit";
pub const MENU_RELOAD: &str = "view-reload";
pub const MENU_DEVTOOLS: &str = "view-devtools";
pub const MENU_GITHUB: &str = "help-github";
pub const MENU_HELP: &str = "help-guide";

pub const GITHUB_URL: &str = "https://github.com/tlmcguire/fistbump";
pub const GUIDE_URL: &str = "https://github.com/tlmcguire/fistbump/blob/main/docs/user-guide.md";

/// The menu's tick marks, kept so the page can update them.
#[derive(Default)]
pub struct MenuState {
    night: Mutex<Option<CheckMenuItem<Wry>>>,
    darker: Mutex<Option<CheckMenuItem<Wry>>>,
    day: Mutex<Option<CheckMenuItem<Wry>>>,
    system: Mutex<Option<CheckMenuItem<Wry>>>,
    bar_top: Mutex<Option<CheckMenuItem<Wry>>>,
}

pub fn build_app_menu(app: &AppHandle) -> tauri::Result<Menu<Wry>> {
    let mac = cfg!(target_os = "macos");
    let item = |id: &str, text: &str, keys: Option<&str>| {
        let mut b = MenuItemBuilder::with_id(id, text);
        if let Some(k) = keys {
            b = b.accelerator(k);
        }
        b.build(app)
    };

    let settings_item = item(MENU_SETTINGS, if mac { "Settings…" } else { "Settings" }, Some("CmdOrCtrl+,"))?;
    let quit_item = item(MENU_QUIT, if mac { "Quit Fistbump" } else { "Quit" }, Some("CmdOrCtrl+Q"))?;
    let updates_item = item("do:check-updates", "Check for Updates…", None)?;

    let mut menu = MenuBuilder::new(app);

    #[cfg(target_os = "macos")]
    {
        let app_submenu = SubmenuBuilder::new(app, "Fistbump")
            .about(None)
            .item(&updates_item)
            .separator()
            .item(&settings_item)
            .separator()
            .services()
            .separator()
            .hide()
            .hide_others()
            .show_all()
            .separator()
            .item(&quit_item)
            .build()?;
        menu = menu.item(&app_submenu);
    }

    // ---- File: start something, bring something in, take something out ----
    let new_search = item("do:new-search", "New Search…", Some("CmdOrCtrl+N"))?;
    let paste_job = item("do:paste-job", "Paste a Job Posting…", Some("CmdOrCtrl+Shift+N"))?;
    let import_resume = item("do:import-resume", "Import Resume…", Some("CmdOrCtrl+O"))?;
    let export_csv = item("do:export-csv", "Export Applications as CSV…", Some("CmdOrCtrl+Shift+E"))?;
    let mut file = SubmenuBuilder::new(app, "File")
        .item(&new_search)
        .item(&paste_job)
        .separator()
        .item(&import_resume)
        .item(&export_csv)
        .separator();
    file = if mac {
        file.close_window()
    } else {
        file.item(&settings_item).item(&updates_item).separator().item(&quit_item)
    };
    let file_submenu = file.build()?;

    // ---- Edit: the system's own items, then Find ----
    let find = item("do:find", "Find…", Some("CmdOrCtrl+F"))?;
    let edit_submenu = SubmenuBuilder::new(app, "Edit")
        .undo()
        .redo()
        .separator()
        .cut()
        .copy()
        .paste()
        .select_all()
        .separator()
        .item(&find)
        .build()?;

    // ---- View: how the app looks ----
    let night = CheckMenuItemBuilder::with_id("do:mode-night", "Night").build(app)?;
    let darker = CheckMenuItemBuilder::with_id("do:mode-darker", "Darker").build(app)?;
    let day = CheckMenuItemBuilder::with_id("do:mode-day", "Day").build(app)?;
    let system = CheckMenuItemBuilder::with_id("do:mode-system", "Match System").build(app)?;
    let appearance = SubmenuBuilder::new(app, "Day or Night").item(&night).item(&darker).item(&day).item(&system).build()?;
    let bar_top = CheckMenuItemBuilder::with_id("do:bar-top", "Navigation Bar at Top").build(app)?;
    let bigger = item("do:text-bigger", "Bigger Text", Some("CmdOrCtrl+="))?;
    let smaller = item("do:text-smaller", "Smaller Text", Some("CmdOrCtrl+-"))?;
    let actual = item("do:text-actual", "Actual Size", Some("CmdOrCtrl+0"))?;
    let mut view_builder = SubmenuBuilder::new(app, "View")
        .item(&appearance)
        .item(&bar_top)
        .separator()
        .item(&bigger)
        .item(&smaller)
        .item(&actual)
        .separator();
    if cfg!(debug_assertions) {
        let reload = item(MENU_RELOAD, "Reload", Some("CmdOrCtrl+R"))?;
        let devtools = item(MENU_DEVTOOLS, "Toggle Developer Tools", Some(if mac { "Alt+Cmd+I" } else { "Ctrl+Shift+I" }))?;
        view_builder = view_builder.item(&reload).item(&devtools).separator();
    }
    let view_submenu = view_builder.fullscreen().build()?;

    // ---- Go: the places in the app, with the same keys as the bar ----
    let back = item("do:back", "Back", Some("CmdOrCtrl+["))?;
    let forward = item("do:forward", "Forward", Some("CmdOrCtrl+]"))?;
    let home = item("do:go-home", "Home", Some("CmdOrCtrl+1"))?;
    let jobs = item("do:go-jobs", "Your Jobs", Some("CmdOrCtrl+2"))?;
    let resume = item("do:go-resume", "Resume", Some("CmdOrCtrl+3"))?;
    let tracker = item("do:go-tracker", "Tracker", Some("CmdOrCtrl+4"))?;
    let tailor = item("do:go-tailor", "Tailor", Some("CmdOrCtrl+5"))?;
    let calendar = item("do:go-calendar", "Calendar", Some("CmdOrCtrl+6"))?;
    let search = item("do:search", "Search…", Some("CmdOrCtrl+K"))?;
    let go_submenu = SubmenuBuilder::new(app, "Go")
        .item(&back)
        .item(&forward)
        .separator()
        .item(&home)
        .item(&jobs)
        .item(&resume)
        .item(&tracker)
        .item(&tailor)
        .item(&calendar)
        .separator()
        .item(&search)
        .build()?;

    // ---- Window ----
    let mut window = SubmenuBuilder::new(app, "Window").minimize().item(&PredefinedMenuItem::maximize(app, None)?);
    window = if mac { window.separator().bring_all_to_front() } else { window.close_window() };
    let window_submenu = window.build()?;

    // ---- Help ----
    let guide = item(MENU_HELP, "Fistbump Help", None)?;
    let shortcuts = item("do:shortcuts", "Keyboard Shortcuts", Some("CmdOrCtrl+/"))?;
    let report = item("do:report", "Report an Issue…", None)?;
    let github_item = item(MENU_GITHUB, "Fistbump on GitHub", None)?;
    let help_submenu = SubmenuBuilder::new(app, "Help")
        .item(&guide)
        .item(&shortcuts)
        .separator()
        .item(&report)
        .item(&github_item)
        .build()?;

    #[cfg(target_os = "macos")]
    {
        let _ = window_submenu.set_as_windows_menu_for_nsapp();
        let _ = help_submenu.set_as_help_menu_for_nsapp();
    }

    if let Some(state) = app.try_state::<MenuState>() {
        *state.night.lock().unwrap() = Some(night);
        *state.darker.lock().unwrap() = Some(darker);
        *state.day.lock().unwrap() = Some(day);
        *state.system.lock().unwrap() = Some(system);
        *state.bar_top.lock().unwrap() = Some(bar_top);
    }

    menu.item(&file_submenu)
        .item(&edit_submenu)
        .item(&view_submenu)
        .item(&go_submenu)
        .item(&window_submenu)
        .item(&help_submenu)
        .build()
}

/// `app:setMenuState` — the page says what is chosen, and the ticks follow: `{ "mode": "night"|"darker"|"day"|"system", "barTop": bool }`.
pub fn set_menu_state(app: &AppHandle, state: &Value) -> Value {
    if let Some(menu) = app.try_state::<MenuState>() {
        if let Some(mode) = state.get("mode").and_then(Value::as_str) {
            for (slot, name) in [(&menu.night, "night"), (&menu.darker, "darker"), (&menu.day, "day"), (&menu.system, "system")] {
                if let Some(item) = slot.lock().unwrap().as_ref() {
                    let _ = item.set_checked(mode == name);
                }
            }
        }
        if let Some(top) = state.get("barTop").and_then(Value::as_bool) {
            if let Some(item) = menu.bar_top.lock().unwrap().as_ref() {
                let _ = item.set_checked(top);
            }
        }
    }
    serde_json::json!({ "ok": true })
}

pub fn handle_menu_event(app: &AppHandle, id: &str) {
    if let Some(action) = id.strip_prefix("do:") {
        if let Some(win) = app.get_webview_window("main") {
            let _ = win.unminimize();
            let _ = win.show();
            let _ = win.set_focus();
            let _ = win.emit("app:menu", action.to_string());
        }
        return;
    }
    match id {
        MENU_SETTINGS => {
            if let Some(win) = app.get_webview_window("main") {
                let _ = win.unminimize();
                let _ = win.show();
                let _ = win.set_focus();
                let _ = win.emit("app:open-settings", ());
            }
        }
        MENU_QUIT => {
            if let Some(state) = app.try_state::<AppState>() {
                state.explicit_quit.store(true, Ordering::SeqCst);
            }
            app.exit(0);
        }
        MENU_RELOAD => {
            if let Some(win) = app.get_webview_window("main") {
                let _ = win.eval("window.location.reload()");
            }
        }
        MENU_DEVTOOLS => {
            #[cfg(debug_assertions)]
            if let Some(win) = app.get_webview_window("main") {
                if win.is_devtools_open() {
                    win.close_devtools();
                } else {
                    win.open_devtools();
                }
            }
        }
        MENU_GITHUB => {
            let _ = app.opener().open_url(GITHUB_URL, None::<&str>);
        }
        MENU_HELP => {
            let _ = app.opener().open_url(GUIDE_URL, None::<&str>);
        }
        _ => {}
    }
}
