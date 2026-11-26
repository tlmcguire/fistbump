//! Operating-system integration the user turns on in Settings: a global shortcut that brings Fistbump forward with
//! the search box focused, a menu-bar (tray) item that shows what is due, native notifications, and the
//! `fistbump://` deep links. Each is driven from the renderer through `ipc_call` channels (`app:setShortcut`,
//! `app:setTray`, `app:notify`); deep links arrive as the `app:deep-link` event.

use std::sync::Mutex;

use serde_json::{json, Value};
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::{TrayIcon, TrayIconBuilder};
use tauri::{AppHandle, Emitter, Manager, Runtime};
use tauri_plugin_global_shortcut::{GlobalShortcutExt, Shortcut, ShortcutState};
use tauri_plugin_notification::NotificationExt;

/// The keys of the global shortcut. Chosen to stay clear of the system search shortcuts (Cmd/Ctrl+Space).
pub const SHORTCUT: &str = "CommandOrControl+Alt+Space";
const TRAY_ID: &str = "fistbump-tray";
const TRAY_STATUS: &str = "tray-status";
const TRAY_OPEN: &str = "tray-open";
const TRAY_SEARCH: &str = "tray-search";
const TRAY_QUIT: &str = "tray-quit";

#[derive(Default)]
pub struct DesktopState {
    tray: Mutex<Option<TrayIcon>>,
    status: Mutex<Option<MenuItem<tauri::Wry>>>,
}

/// Brings the main window forward, and optionally asks the page to focus its search box.
pub fn show_main<R: Runtime>(app: &AppHandle<R>, focus_search: bool) {
    if let Some(win) = app.get_webview_window("main") {
        let _ = win.unminimize();
        let _ = win.show();
        let _ = win.set_focus();
        if focus_search {
            let _ = win.emit("app:focus-search", ());
        }
    }
}

/// The global shortcut plugin, with its handler: a press brings Fistbump forward with the search box focused.
pub fn shortcut_plugin<R: Runtime>() -> tauri::plugin::TauriPlugin<R> {
    tauri_plugin_global_shortcut::Builder::new()
        .with_handler(|app, _shortcut, event| {
            if event.state() == ShortcutState::Pressed {
                show_main(app, true);
            }
        })
        .build()
}

fn ok() -> Value {
    json!({ "ok": true })
}

fn fail(message: impl Into<String>) -> Value {
    json!({ "error": { "code": "internal", "message": message.into(), "details": Value::Null } })
}

/// `app:setShortcut` — registers or removes the global shortcut. Another app may own the keys already; that is
/// reported so Settings can say so instead of failing silently.
pub fn set_shortcut(app: &AppHandle, enabled: bool) -> Value {
    let shortcut: Shortcut = match SHORTCUT.parse() {
        Ok(s) => s,
        Err(err) => return fail(format!("Could not read the shortcut: {err}")),
    };
    let gs = app.global_shortcut();
    let registered = gs.is_registered(shortcut);
    if enabled && !registered {
        if let Err(err) = gs.register(shortcut) {
            return fail(format!("Another app is already using this shortcut. {err}"));
        }
    } else if !enabled && registered {
        let _ = gs.unregister(shortcut);
    }
    json!({ "ok": true, "shortcut": SHORTCUT })
}

/// `app:setTray` — shows or removes the menu-bar item, and updates what it says is due.
/// `status` is the line in its menu ("2 things need you today"), `count` the number shown beside the icon on macOS.
pub fn set_tray(app: &AppHandle, enabled: bool, status: &str, count: u64) -> Value {
    let desktop = app.state::<DesktopState>();
    if !enabled {
        if let Some(tray) = desktop.tray.lock().unwrap().take() {
            let _ = tray.set_visible(false);
        }
        let _ = app.remove_tray_by_id(TRAY_ID);
        *desktop.status.lock().unwrap() = None;
        return ok();
    }

    if desktop.tray.lock().unwrap().is_none() {
        match build_tray(app, status) {
            Ok((tray, item)) => {
                *desktop.tray.lock().unwrap() = Some(tray);
                *desktop.status.lock().unwrap() = Some(item);
            }
            Err(err) => return fail(format!("Could not add the menu-bar item: {err}")),
        }
    }
    if let Some(item) = desktop.status.lock().unwrap().as_ref() {
        let _ = item.set_text(status);
    }
    if let Some(tray) = desktop.tray.lock().unwrap().as_ref() {
        let _ = tray.set_tooltip(Some(format!("Fistbump: {status}")));
        #[cfg(target_os = "macos")]
        {
            let _ = tray.set_title(if count > 0 { Some(count.to_string()) } else { None });
        }
        #[cfg(not(target_os = "macos"))]
        let _ = count;
    }
    ok()
}

fn build_tray(app: &AppHandle, status: &str) -> tauri::Result<(TrayIcon, MenuItem<tauri::Wry>)> {
    let status_item = MenuItem::with_id(app, TRAY_STATUS, status, false, None::<&str>)?;
    let open = MenuItem::with_id(app, TRAY_OPEN, "Open Fistbump", true, None::<&str>)?;
    let search = MenuItem::with_id(app, TRAY_SEARCH, "Search jobs…", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, TRAY_QUIT, "Quit Fistbump", true, None::<&str>)?;
    let menu = Menu::with_items(
        app,
        &[
            &status_item,
            &PredefinedMenuItem::separator(app)?,
            &open,
            &search,
            &PredefinedMenuItem::separator(app)?,
            &quit,
        ],
    )?;
    let mut builder = TrayIconBuilder::with_id(TRAY_ID)
        .menu(&menu)
        .show_menu_on_left_click(true)
        .tooltip("Fistbump")
        .on_menu_event(|app, event| match event.id().as_ref() {
            TRAY_OPEN => show_main(app, false),
            TRAY_SEARCH => show_main(app, true),
            TRAY_QUIT => {
                if let Some(state) = app.try_state::<crate::AppState>() {
                    state.explicit_quit.store(true, std::sync::atomic::Ordering::SeqCst);
                }
                app.exit(0);
            }
            _ => {}
        });
    if let Some(icon) = app.default_window_icon() {
        builder = builder.icon(icon.clone());
    }
    let tray = builder.build(app)?;
    Ok((tray, status_item))
}

/// `app:notify` — a native notification. The renderer decides what is worth a notification and when.
pub fn notify(app: &AppHandle, title: &str, body: &str) -> Value {
    match app.notification().builder().title(title).body(body).show() {
        Ok(()) => ok(),
        Err(err) => fail(format!("Could not show a notification: {err}")),
    }
}

/// Deep links (`fistbump://open?url=…`, `fistbump://job/12`) are passed to the page, which decides what they open.
/// A link that launched the app is held until the page is ready to receive it.
pub fn setup_deep_links(app: &tauri::App) {
    use tauri_plugin_deep_link::DeepLinkExt;
    #[cfg(any(target_os = "linux", all(debug_assertions, windows)))]
    {
        let _ = app.deep_link().register_all();
    }
    let handle = app.handle().clone();
    app.deep_link().on_open_url(move |event| {
        for url in event.urls() {
            forward_link(&handle, url.as_str());
        }
    });
    if let Ok(Some(urls)) = app.deep_link().get_current() {
        let handle = app.handle().clone();
        let links: Vec<String> = urls.iter().map(|u| u.to_string()).collect();
        //the page needs a moment to load before it can hear the link that started the app
        tauri::async_runtime::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(2500)).await;
            for link in links {
                forward_link(&handle, &link);
            }
        });
    }
}

fn forward_link<R: Runtime>(app: &AppHandle<R>, url: &str) {
    if !url.starts_with("fistbump://") {
        return;
    }
    show_main(app, false);
    if let Some(win) = app.get_webview_window("main") {
        let _ = win.emit("app:deep-link", url.to_string());
    }
}
