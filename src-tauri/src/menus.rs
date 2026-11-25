// Native application menu matching `app/main/menus.js` across macOS, Windows, and Linux.

use std::sync::atomic::Ordering;

use tauri::menu::{Menu, MenuBuilder, MenuItemBuilder, PredefinedMenuItem, SubmenuBuilder};
use tauri::{AppHandle, Emitter, Manager, Wry};
use tauri_plugin_opener::OpenerExt;

use crate::AppState;

pub const MENU_SETTINGS: &str = "app-settings";
pub const MENU_QUIT: &str = "app-quit";
pub const MENU_RELOAD: &str = "view-reload";
pub const MENU_DEVTOOLS: &str = "view-devtools";
pub const MENU_GITHUB: &str = "help-github";

pub const GITHUB_URL: &str = "https://github.com/tlmcguire/fistbump";

pub fn build_app_menu(app: &AppHandle) -> tauri::Result<Menu<Wry>> {
    let settings_item = MenuItemBuilder::with_id(
        MENU_SETTINGS,
        if cfg!(target_os = "macos") {
            "Settings…"
        } else {
            "Settings"
        },
    )
    .accelerator("CmdOrCtrl+,")
    .build(app)?;

    let quit_item = MenuItemBuilder::with_id(
        MENU_QUIT,
        if cfg!(target_os = "macos") {
            "Quit Fistbump"
        } else {
            "Quit"
        },
    )
    .accelerator("CmdOrCtrl+Q")
    .build(app)?;

    let mut menu = MenuBuilder::new(app);

    #[cfg(target_os = "macos")]
    {
        let app_submenu = SubmenuBuilder::new(app, "Fistbump")
            .about(None)
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

    let file_submenu = if cfg!(target_os = "macos") {
        SubmenuBuilder::new(app, "File").close_window().build()?
    } else {
        SubmenuBuilder::new(app, "File")
            .item(&settings_item)
            .separator()
            .item(&quit_item)
            .build()?
    };

    let edit_submenu = SubmenuBuilder::new(app, "Edit")
        .undo()
        .redo()
        .separator()
        .cut()
        .copy()
        .paste()
        .select_all()
        .build()?;

    let mut view_builder = SubmenuBuilder::new(app, "View");
    if cfg!(debug_assertions) {
        let reload = MenuItemBuilder::with_id(MENU_RELOAD, "Reload")
            .accelerator("CmdOrCtrl+R")
            .build(app)?;
        let devtools = MenuItemBuilder::with_id(MENU_DEVTOOLS, "Toggle Developer Tools")
            .accelerator(if cfg!(target_os = "macos") {
                "Alt+Cmd+I"
            } else {
                "Ctrl+Shift+I"
            })
            .build(app)?;
        view_builder = view_builder.item(&reload).item(&devtools).separator();
    }
    let view_submenu = view_builder.fullscreen().build()?;

    let window_submenu = SubmenuBuilder::new(app, "Window")
        .minimize()
        .item(&PredefinedMenuItem::maximize(app, None)?)
        .close_window()
        .build()?;

    let github_item = MenuItemBuilder::with_id(MENU_GITHUB, "Fistbump on GitHub").build(app)?;
    let help_submenu = SubmenuBuilder::new(app, "Help")
        .item(&github_item)
        .build()?;

    menu.item(&file_submenu)
        .item(&edit_submenu)
        .item(&view_submenu)
        .item(&window_submenu)
        .item(&help_submenu)
        .build()
}

pub fn handle_menu_event(app: &AppHandle, id: &str) {
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
        _ => {}
    }
}
