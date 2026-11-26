pub mod backend;
pub mod calendar;
pub mod commands;
pub mod desktop;
pub mod menus;
pub mod relay;
pub mod routes;
pub mod secrets;
pub mod updates;

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use tauri::webview::NewWindowResponse;
use tauri::window::Color;
#[cfg(any(target_os = "macos", target_os = "windows"))]
use tauri::window::{Effect, EffectState, EffectsBuilder};
#[cfg(target_os = "macos")]
use tauri::{LogicalPosition, TitleBarStyle};
use tauri::{AppHandle, Emitter, Manager, RunEvent, Theme, WebviewUrl, WebviewWindowBuilder, WindowEvent};
use tauri_plugin_opener::OpenerExt;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;

use crate::backend::{
    resolve_bin_dir, resolve_data_dir, BackendEvent, BackendManager, STDERR_TAIL_LINES,
};
use crate::menus::{build_app_menu, handle_menu_event};
use crate::secrets::{restore_remote_key, SecretsStore};
use crate::updates::{SharedUpdateState, UpdateState};

#[derive(Default, Clone)]
pub struct StartupError {
    pub message: String,
    pub stderr: Vec<String>,
}

pub struct AppState {
    pub backend: Arc<BackendManager>,
    pub secrets: Arc<SecretsStore>,
    pub active_search: Mutex<Option<CancellationToken>>,
    pub active_suggest: Mutex<Option<CancellationToken>>,
    pub last_saved: Mutex<Option<PathBuf>>,
    pub startup_error: Arc<Mutex<StartupError>>,
    pub updates: SharedUpdateState,
    pub explicit_quit: AtomicBool,
    pub error_screen_shown: AtomicBool,
}

fn is_internal_url(url: &tauri::Url) -> bool {
    matches!(url.scheme(), "tauri" | "about" | "blob" | "data" | "asset")
        || matches!(
            (url.scheme(), url.host_str()),
            ("http" | "https", Some("tauri.localhost" | "localhost" | "127.0.0.1"))
        )
}

fn open_https_externally(app: &AppHandle, url: &tauri::Url) {
    if url.scheme() == "https" {
        let _ = app.opener().open_url(url.as_str(), None::<&str>);
    }
}

pub fn create_main_window(app: &AppHandle) -> tauri::Result<()> {
    if let Some(existing) = app.get_webview_window("main") {
        let _ = existing.unminimize();
        let _ = existing.show();
        let _ = existing.set_focus();
        return Ok(());
    }

    let nav_app = app.clone();
    let popup_app = app.clone();

    let mut builder = WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
        .title("Fistbump")
        .inner_size(1240.0, 820.0)
        .min_inner_size(760.0, 520.0)
        .visible(false)
        .theme(Some(Theme::Dark))
        .disable_drag_drop_handler()
        .zoom_hotkeys_enabled(false)
        .on_navigation(move |url| {
            if is_internal_url(url) {
                true
            } else {
                open_https_externally(&nav_app, url);
                false
            }
        })
        .on_new_window(move |url, _features| {
            open_https_externally(&popup_app, &url);
            NewWindowResponse::Deny
        });

    #[cfg(target_os = "macos")]
    {
        builder = builder
            .transparent(true)
            .title_bar_style(TitleBarStyle::Overlay)
            .hidden_title(true)
            .traffic_light_position(LogicalPosition::new(16.0, 19.0))
            .effects(
                EffectsBuilder::new()
                    .effect(Effect::Sidebar)
                    .state(EffectState::FollowsWindowActiveState)
                    .build(),
            );
    }

    #[cfg(target_os = "windows")]
    {
        builder = builder
            .decorations(false)
            .background_color(Color(11, 36, 87, 255))
            .effects(EffectsBuilder::new().effect(Effect::Mica).build());
    }

    #[cfg(target_os = "linux")]
    {
        builder = builder.background_color(Color(11, 36, 87, 255));
    }

    let window = builder.build()?;
    let _ = window.show();
    Ok(())
}

pub fn show_error_screen(app: &AppHandle, quit_on_close: bool) {
    if let Some(state) = app.try_state::<AppState>() {
        if state.error_screen_shown.swap(true, Ordering::SeqCst) {
            if let Some(existing) = app.get_webview_window("error") {
                let _ = existing.show();
                let _ = existing.set_focus();
            }
            return;
        }
    }

    let builder = WebviewWindowBuilder::new(app, "error", WebviewUrl::App("error.html".into()))
        .title("Fistbump could not start")
        .inner_size(720.0, 600.0)
        .theme(Some(Theme::Dark))
        .background_color(Color(11, 36, 87, 255))
        .visible(true);

    if let Ok(win) = builder.build() {
        if quit_on_close {
            let app_handle = app.clone();
            win.on_window_event(move |event| {
                if let WindowEvent::Destroyed = event {
                    if let Some(state) = app_handle.try_state::<AppState>() {
                        state.explicit_quit.store(true, Ordering::SeqCst);
                    }
                    app_handle.exit(0);
                }
            });
        }
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let mut builder = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_process::init())
        .manage(menus::MenuState::default())
        .invoke_handler(tauri::generate_handler![commands::ipc_call])
        .on_menu_event(|app, event| {
            handle_menu_event(app, event.id().as_ref());
        });

    #[cfg(desktop)]
    {
        builder = builder
            .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
                if let Some(win) = app.get_webview_window("main") {
                    let _ = win.unminimize();
                    let _ = win.show();
                    let _ = win.set_focus();
                } else {
                    let _ = create_main_window(app);
                }
            }))
            .plugin(tauri_plugin_window_state::Builder::default().build())
            .plugin(tauri_plugin_updater::Builder::new().build())
            .plugin(desktop::shortcut_plugin())
            .plugin(tauri_plugin_notification::init())
            .plugin(tauri_plugin_deep_link::init())
            .manage(desktop::DesktopState::default());
    }

    let app = builder
        .setup(|app| {
            let base_data = app
                .path()
                .data_dir()
                .map_err(|e| format!("Could not resolve OS data directory: {e}"))?;
            let data_dir = resolve_data_dir(base_data);
            let resource_dir = app.path().resource_dir().ok();
            let bin_dir = resolve_bin_dir(resource_dir);

            let backend = BackendManager::new(bin_dir, data_dir);
            let secrets = Arc::new(SecretsStore::new());
            let startup_error = Arc::new(Mutex::new(StartupError::default()));

            if let Ok(menu) = build_app_menu(app.handle()) {
                let _ = app.set_menu(menu);
            }
            #[cfg(desktop)]
            desktop::setup_deep_links(app);

            let handle_for_cb = app.handle().clone();
            let backend_for_cb = Arc::clone(&backend);
            let secrets_for_cb = Arc::clone(&secrets);
            let error_for_cb = Arc::clone(&startup_error);

            let backend_for_setup = Arc::clone(&backend);
            let secrets_for_setup = Arc::clone(&secrets);
            let error_for_setup = Arc::clone(&startup_error);

            let started_ok = tauri::async_runtime::block_on(async move {
                backend_for_setup
                    .set_event_callback(Arc::new(move |event| match event {
                        BackendEvent::Restarted => {
                            let h = handle_for_cb.clone();
                            let b = Arc::clone(&backend_for_cb);
                            let s = Arc::clone(&secrets_for_cb);
                            tauri::async_runtime::spawn(async move {
                                let _ = restore_remote_key(&b, &s).await;
                                let _ = h.emit("backend:restarted", ());
                            });
                        }
                        BackendEvent::Failed { reason } => {
                            eprintln!("Fistbump backend failed: {reason}");
                            let h = handle_for_cb.clone();
                            let b = Arc::clone(&backend_for_cb);
                            let err_state = Arc::clone(&error_for_cb);
                            tauri::async_runtime::spawn(async move {
                                let msg = if reason == "too many crashes" {
                                    "The backend crashed too many times in a minute and was not restarted."
                                        .to_string()
                                } else {
                                    "The backend stopped and could not be restarted.".to_string()
                                };
                                let stderr = b.stderr_lines(STDERR_TAIL_LINES).await;
                                *err_state.lock().await = StartupError {
                                    message: msg,
                                    stderr,
                                };
                                show_error_screen(&h, false);
                            });
                        }
                        BackendEvent::Ready => {}
                    }))
                    .await;

                match backend_for_setup.start().await {
                    Ok(()) => {
                        let _ = restore_remote_key(&backend_for_setup, &secrets_for_setup).await;
                        true
                    }
                    Err(err) => {
                        let stderr = backend_for_setup.stderr_lines(STDERR_TAIL_LINES).await;
                        *error_for_setup.lock().await = StartupError {
                            message: format!("The backend could not start. {err}"),
                            stderr,
                        };
                        eprintln!("Fistbump backend failed to start: {err}");
                        false
                    }
                }
            });

            app.manage(AppState {
                backend,
                secrets,
                active_search: Mutex::new(None),
                active_suggest: Mutex::new(None),
                last_saved: Mutex::new(None),
                startup_error,
                updates: Mutex::new(UpdateState::default()),
                explicit_quit: AtomicBool::new(false),
                error_screen_shown: AtomicBool::new(false),
            });

            if started_ok {
                create_main_window(app.handle())?;
            } else {
                show_error_screen(app.handle(), true);
            }

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building Fistbump Tauri application");

    app.run(|app_handle, event| match event {
        #[cfg(target_os = "macos")]
        RunEvent::ExitRequested { api, .. } => {
            let should_quit = app_handle
                .try_state::<AppState>()
                .map(|s| s.explicit_quit.load(Ordering::SeqCst))
                .unwrap_or(true);
            if !should_quit {
                api.prevent_exit();
            }
        }
        #[cfg(target_os = "macos")]
        RunEvent::Reopen {
            has_visible_windows,
            ..
        } => {
            if !has_visible_windows {
                let _ = create_main_window(app_handle);
            }
        }
        RunEvent::Exit => {
            if let Some(state) = app_handle.try_state::<AppState>() {
                tauri::async_runtime::block_on(state.backend.stop());
            }
        }
        _ => {}
    });
}

#[cfg(test)]
mod tests {
    use super::is_internal_url;
    use std::net::Ipv4Addr;

    #[test]
    fn internal_url_guard_allows_app_origins_and_blocks_external_https() {
        let loopback = format!("http://{}:8080/index.html", Ipv4Addr::LOCALHOST);
        for allowed in [
            "tauri://localhost/index.html",
            "http://tauri.localhost/index.html",
            "https://tauri.localhost/index.html",
            "http://localhost:1420/",
            loopback.as_str(),
            "about:blank",
            "blob:tauri://localhost/preview-id",
            "data:image/png;base64,AAAA",
        ] {
            let url = tauri::Url::parse(allowed).expect("valid test url");
            assert!(is_internal_url(&url), "expected internal URL to be allowed: {allowed}");
        }

        for blocked in [
            "https://github.com/tlmcguire/fistbump",
            "https://boards.greenhouse.io/acme",
            "http://example.com/",
        ] {
            let url = tauri::Url::parse(blocked).expect("valid test url");
            assert!(!is_internal_url(&url), "expected external URL to be blocked: {blocked}");
        }
    }
}
