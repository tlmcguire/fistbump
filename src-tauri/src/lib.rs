pub mod backend;
pub mod commands;
pub mod routes;
pub mod secrets;

use std::path::PathBuf;
use std::sync::Arc;

use tauri::{Emitter, Manager, RunEvent};
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;

use crate::backend::{
    resolve_bin_dir, resolve_data_dir, BackendEvent, BackendManager, STDERR_TAIL_LINES,
};
use crate::secrets::{restore_remote_key, SecretsStore};

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
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .invoke_handler(tauri::generate_handler![commands::ipc_call])
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

            // Notify windows and restore the remote AI key whenever the backend restarts after a crash.
            let handle = app.handle().clone();
            let backend_for_cb = Arc::clone(&backend);
            let secrets_for_cb = Arc::clone(&secrets);
            let error_for_cb = Arc::clone(&startup_error);

            let backend_for_setup = Arc::clone(&backend);
            let secrets_for_setup = Arc::clone(&secrets);
            let error_for_setup = Arc::clone(&startup_error);

            tauri::async_runtime::block_on(async move {
                backend_for_setup
                    .set_event_callback(Arc::new(move |event| match event {
                        BackendEvent::Restarted => {
                            let h = handle.clone();
                            let b = Arc::clone(&backend_for_cb);
                            let s = Arc::clone(&secrets_for_cb);
                            tauri::async_runtime::spawn(async move {
                                let _ = restore_remote_key(&b, &s).await;
                                let _ = h.emit("backend:restarted", ());
                            });
                        }
                        BackendEvent::Failed { reason } => {
                            eprintln!("Fistbump backend failed: {reason}");
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
                            });
                        }
                        BackendEvent::Ready => {}
                    }))
                    .await;

                match backend_for_setup.start().await {
                    Ok(()) => {
                        let _ = restore_remote_key(&backend_for_setup, &secrets_for_setup).await;
                    }
                    Err(err) => {
                        let stderr = backend_for_setup.stderr_lines(STDERR_TAIL_LINES).await;
                        *error_for_setup.lock().await = StartupError {
                            message: format!("The backend could not start. {err}"),
                            stderr,
                        };
                        eprintln!("Fistbump backend failed to start: {err}");
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
            });

            // Show the main window once initialized so it avoids a white flash on startup.
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
            }

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building Fistbump Tauri application");

    app.run(|app_handle, event| {
        if let RunEvent::Exit = event {
            if let Some(state) = app_handle.try_state::<AppState>() {
                tauri::async_runtime::block_on(state.backend.stop());
            }
        }
    });
}
