// Bridges `tauri-plugin-updater` to the existing Settings UI update channels
// (`app:check-updates`, `app:download-update`, `app:cancel-update`, `app:install-update`).

use std::sync::atomic::Ordering;
use std::time::Instant;

use serde_json::{json, Value};
use tauri::{AppHandle, Emitter};
use tauri_plugin_updater::{Update, UpdaterExt};
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;

use crate::menus::GITHUB_URL;
use crate::AppState;

#[derive(Default)]
pub struct UpdateState {
    available: Option<Update>,
    downloaded: Option<Vec<u8>>,
    cancel: Option<CancellationToken>,
}

pub type SharedUpdateState = Mutex<UpdateState>;

fn releases_url() -> String {
    format!("{GITHUB_URL}/releases")
}

pub async fn check_for_updates(app: &AppHandle, state: &AppState) -> Value {
    let current = app.package_info().version.to_string();
    let url = releases_url();
    let endpoint = format!("{GITHUB_URL}/releases/latest/download/latest.json");
    let updater = match tauri::Url::parse(&endpoint)
        .ok()
        .and_then(|u| app.updater_builder().endpoints(vec![u]).ok())
        .and_then(|b| b.build().ok())
    {
        Some(u) => u,
        None => {
            return json!({
                "status": "current",
                "current": current,
                "latest": current,
                "url": url,
                "publishedAt": "",
                "message": format!("You have the latest version ({current})."),
                "downloadable": false,
                "size": 0
            });
        }
    };

    match updater.check().await {
        Ok(Some(update)) => {
            let latest = update.version.clone();
            let published_at = update.date.map(|d| d.to_string()).unwrap_or_default();
            let mut guard = state.updates.lock().await;
            guard.available = Some(update);
            guard.downloaded = None;
            json!({
                "status": "available",
                "current": current,
                "latest": latest,
                "url": url,
                "publishedAt": published_at,
                "message": format!("Fistbump {latest} is available."),
                "downloadable": true,
                "size": 0
            })
        }
        Ok(None) => {
            let mut guard = state.updates.lock().await;
            guard.available = None;
            guard.downloaded = None;
            json!({
                "status": "current",
                "current": current,
                "latest": current,
                "url": url,
                "publishedAt": "",
                "message": format!("You have the latest version ({current})."),
                "downloadable": false,
                "size": 0
            })
        }
        Err(err) => json!({
            "status": "error",
            "current": current,
            "latest": "",
            "url": url,
            "publishedAt": "",
            "message": format!("Could not check for updates: {err}"),
            "downloadable": false,
            "size": 0
        }),
    }
}

pub async fn download_update(app: &AppHandle, state: &AppState) -> Value {
    let (update, token) = {
        let mut guard = state.updates.lock().await;
        if guard.cancel.is_some() {
            return json!({
                "error": {
                    "code": "conflict",
                    "message": "The update is already downloading.",
                    "details": Value::Null
                }
            });
        }
        let Some(update) = guard.available.clone() else {
            return json!({
                "error": {
                    "code": "not_found",
                    "message": "There is no update to download. Check for updates first.",
                    "details": Value::Null
                }
            });
        };
        let token = CancellationToken::new();
        guard.cancel = Some(token.clone());
        (update, token)
    };

    let app_handle = app.clone();
    let mut received: u64 = 0;
    let mut last_emit = Instant::now();

    let download_fut = update.download(
        move |chunk_len, total| {
            received += chunk_len as u64;
            let total_bytes = total.unwrap_or(0);
            let done = total_bytes > 0 && received >= total_bytes;
            if done || last_emit.elapsed().as_millis() >= 100 {
                last_emit = Instant::now();
                let _ = app_handle.emit(
                    "update:progress",
                    json!({ "received": received, "total": total_bytes }),
                );
            }
        },
        || {},
    );

    let outcome = tokio::select! {
        _ = token.cancelled() => Err("canceled".to_string()),
        res = download_fut => res.map_err(|e| e.to_string()),
    };

    let mut guard = state.updates.lock().await;
    guard.cancel = None;

    match outcome {
        Ok(bytes) => {
            guard.downloaded = Some(bytes);
            json!({
                "ok": true,
                "name": format!("FistBump-{}", update.version),
                "folder": ""
            })
        }
        Err(msg) if msg == "canceled" => json!({
            "error": {
                "code": "canceled",
                "message": "The download was canceled.",
                "details": Value::Null
            }
        }),
        Err(msg) => json!({
            "error": {
                "code": "upstream_error",
                "message": msg,
                "details": Value::Null
            }
        }),
    }
}

pub async fn cancel_update(state: &AppState) -> Value {
    if let Some(token) = state.updates.lock().await.cancel.take() {
        token.cancel();
    }
    json!({ "ok": true })
}

pub async fn install_update(app: &AppHandle, state: &AppState) -> Value {
    let (update, bytes) = {
        let mut guard = state.updates.lock().await;
        match (guard.available.clone(), guard.downloaded.take()) {
            (Some(u), Some(b)) => (u, b),
            _ => {
                return json!({
                    "error": {
                        "code": "not_found",
                        "message": "Download the update first.",
                        "details": Value::Null
                    }
                });
            }
        }
    };

    state.explicit_quit.store(true, Ordering::SeqCst);
    state.backend.stop().await;

    if let Err(err) = update.install(bytes) {
        return json!({
            "error": {
                "code": "internal",
                "message": format!("The update could not be installed: {err}"),
                "details": Value::Null
            }
        });
    }

    let handle = app.clone();
    tokio::spawn(async move {
        tokio::time::sleep(std::time::Duration::from_millis(300)).await;
        handle.restart();
    });

    json!({ "ok": true })
}
