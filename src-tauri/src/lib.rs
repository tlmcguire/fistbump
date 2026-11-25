pub mod backend;

use std::sync::Arc;
use tauri::{Emitter, Manager, RunEvent};

use crate::backend::{resolve_bin_dir, resolve_data_dir, BackendEvent, BackendManager};

pub struct AppState {
    pub backend: Arc<BackendManager>,
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .setup(|app| {
            let base_data = app
                .path()
                .data_dir()
                .map_err(|e| format!("Could not resolve OS data directory: {e}"))?;
            let data_dir = resolve_data_dir(base_data);
            let resource_dir = app.path().resource_dir().ok();
            let bin_dir = resolve_bin_dir(resource_dir);

            let backend = BackendManager::new(bin_dir, data_dir);

            // Notify all windows when the backend restarts after a crash so views reload.
            let handle = app.handle().clone();
            let backend_for_setup = Arc::clone(&backend);
            tauri::async_runtime::block_on(async move {
                backend_for_setup
                    .set_event_callback(Arc::new(move |event| match event {
                        BackendEvent::Restarted => {
                            let _ = handle.emit("backend:restarted", ());
                        }
                        BackendEvent::Failed { reason } => {
                            eprintln!("Fistbump backend failed: {reason}");
                        }
                        BackendEvent::Ready => {}
                    }))
                    .await;

                if let Err(err) = backend_for_setup.start().await {
                    eprintln!("Fistbump backend failed to start: {err}");
                }
            });

            app.manage(AppState { backend });

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
