// Tauri command handler bridging renderer IPC channels to the Go backend and native APIs.

use std::path::PathBuf;
use std::sync::atomic::Ordering;

use base64::engine::general_purpose::STANDARD as BASE64;
use base64::Engine;
use reqwest::header::{CONTENT_DISPOSITION, CONTENT_TYPE};
use reqwest::Method;
use serde_json::{json, Value};
use tauri::{AppHandle, Manager, State};
use tauri_plugin_clipboard_manager::ClipboardExt;
use tauri_plugin_dialog::{DialogExt, FilePath};
use tauri_plugin_opener::OpenerExt;
use tokio::sync::oneshot;
use tokio_util::sync::CancellationToken;

use crate::backend::node_platform_arch;
use crate::routes::{
    as_id, as_non_empty_str, as_object, bad, base_name, check_import, route_for_channel, seg,
    suggested_name, AI_ENGINES, CLEANUP_TARGETS, IMPORT_EXTENSIONS,
};
use crate::AppState;

async fn request_export_bytes(
    state: &AppState,
    url_path: &str,
    format: &str,
    style: Option<&str>,
) -> Result<(Vec<u8>, Option<String>), Value> {
    let mut payload = json!({ "format": format });
    if let Some(s) = style.filter(|s| !s.is_empty()) {
        payload["style"] = json!(s);
    }
    let raw = state
        .backend
        .request_raw(Method::POST, url_path, Some(payload))
        .await
        .map_err(|msg| {
            json!({
                "error": {
                    "code": "unreachable",
                    "message": msg,
                    "details": Value::Null
                }
            })
        })?;

    let is_json = raw
        .headers
        .get(CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .map(|v| v.contains("application/json"))
        .unwrap_or(false);

    if (200..300).contains(&raw.status) && !is_json {
        let disposition = raw
            .headers
            .get(CONTENT_DISPOSITION)
            .and_then(|v| v.to_str().ok())
            .map(|s| s.to_string());
        return Ok((raw.bytes, disposition));
    }

    if is_json {
        if let Ok(val) = serde_json::from_slice::<Value>(&raw.bytes) {
            if val.get("error").is_some() {
                return Err(val);
            }
        }
    }

    Err(json!({
        "error": {
            "code": "internal",
            "message": format!("The backend sent an unexpected response ({})", raw.status),
            "details": Value::Null
        }
    }))
}

async fn save_export(
    app: &AppHandle,
    state: &AppState,
    url_path: &str,
    format: &str,
    style: Option<&str>,
) -> Value {
    let (filter_name, ext) = match format {
        "pdf" => ("PDF", "pdf"),
        "docx" => ("Word Document", "docx"),
        "md" => ("Markdown", "md"),
        _ => return bad("Format must be pdf, docx or md"),
    };

    let (bytes, disposition) = match request_export_bytes(state, url_path, format, style).await {
        Ok(res) => res,
        Err(err) => return err,
    };

    let fallback = format!("Resume.{ext}");
    let default_name = suggested_name(disposition.as_deref(), &fallback);

    let mut builder = app
        .dialog()
        .file()
        .set_title(format!("Export {filter_name}"))
        .set_file_name(&default_name)
        .add_filter(filter_name, &[ext]);

    if let Ok(docs) = app.path().document_dir() {
        builder = builder.set_directory(docs);
    }

    let (tx, rx) = oneshot::channel();
    builder.save_file(move |picked| {
        let _ = tx.send(picked);
    });

    let picked = match rx.await {
        Ok(Some(FilePath::Path(path))) => path,
        _ => return json!({ "saved": false }),
    };

    if let Err(err) = std::fs::write(&picked, &bytes) {
        return json!({
            "error": {
                "code": "internal",
                "message": format!("Could not save the file: {err}"),
                "details": Value::Null
            }
        });
    }

    let path_str = picked.to_string_lossy().to_string();
    *state.last_saved.lock().await = Some(picked);
    json!({ "saved": true, "path": path_str })
}

async fn record_export(state: &AppState, tailored_id: i64, file_path: &str) -> Option<i64> {
    let list = state
        .backend
        .call(Method::GET, "/v1/applications?archived=include", None)
        .await;
    let app_id = list.as_array()?.iter().find_map(|a| {
        if a.get("tailored_resume_id").and_then(|v| v.as_i64()) == Some(tailored_id) {
            a.get("id").and_then(|v| v.as_i64())
        } else {
            None
        }
    })?;
    let updated = state
        .backend
        .call(
            Method::PUT,
            &format!("/v1/applications/{app_id}"),
            Some(json!({ "exported_pdf_path": file_path })),
        )
        .await;
    if updated.get("error").is_some() {
        None
    } else {
        Some(app_id)
    }
}

async fn preview_pdf(state: &AppState, url_path: &str, style: Option<&str>) -> Value {
    match request_export_bytes(state, url_path, "pdf", style).await {
        Ok((bytes, _)) => json!({ "data_base64": BASE64.encode(bytes) }),
        Err(err) => err,
    }
}

#[tauri::command]
pub async fn ipc_call(
    app: AppHandle,
    state: State<'_, AppState>,
    channel: String,
    args: Vec<Value>,
) -> Result<Value, String> {
    if let Some(route_result) = route_for_channel(&channel, &args) {
        return Ok(match route_result {
            Ok((method, path, body)) => state.backend.call(method, &path, body).await,
            Err(err_val) => err_val,
        });
    }

    let a0 = args.first();
    let a1 = args.get(1);

    let result = match channel.as_str() {
        "connectors:link" => match as_non_empty_str(a0) {
            Some(url) => {
                state
                    .backend
                    .call(
                        Method::GET,
                        &format!("/v1/connectors/greenhouse/link?url={}", seg(url.trim())),
                        None,
                    )
                    .await
            }
            None => bad("Enter a Greenhouse, Lever, or Ashby job link."),
        },

        "connectors:posting" => match (
            a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()),
            a1.and_then(|v| v.as_str()).filter(|s| !s.is_empty()),
        ) {
            (Some(board), Some(job_id)) => {
                state
                    .backend
                    .call(
                        Method::GET,
                        &format!(
                            "/v1/connectors/greenhouse/boards/{}/jobs/{}",
                            seg(board),
                            seg(job_id)
                        ),
                        None,
                    )
                    .await
            }
            _ => bad("Missing board or job id."),
        },

        "connectors:search" => match as_object(a0) {
            Some(query) => {
                let token = CancellationToken::new();
                {
                    let mut active = state.active_search.lock().await;
                    if let Some(prev) = active.take() {
                        prev.cancel();
                    }
                    *active = Some(token.clone());
                }
                let res = state
                    .backend
                    .call_cancellable(
                        Method::POST,
                        "/v1/connectors/greenhouse/fetch",
                        Some(json!({ "query": query })),
                        Some(token.clone()),
                    )
                    .await;
                let mut active = state.active_search.lock().await;
                if active
                    .as_ref()
                    .map(|t| t.is_cancelled() == token.is_cancelled())
                    .unwrap_or(false)
                {
                    *active = None;
                }
                res
            }
            None => bad("Invalid search"),
        },

        "connectors:suggest" => match as_object(a0) {
            Some(query) => {
                let token = CancellationToken::new();
                {
                    let mut active = state.active_suggest.lock().await;
                    if let Some(prev) = active.take() {
                        prev.cancel();
                    }
                    *active = Some(token.clone());
                }
                let res = state
                    .backend
                    .call_cancellable(
                        Method::POST,
                        "/v1/connectors/greenhouse/fetch",
                        Some(json!({ "query": query })),
                        Some(token.clone()),
                    )
                    .await;
                let mut active = state.active_suggest.lock().await;
                if active
                    .as_ref()
                    .map(|t| t.is_cancelled() == token.is_cancelled())
                    .unwrap_or(false)
                {
                    *active = None;
                }
                res
            }
            None => bad("Invalid search"),
        },

        "connectors:cancel-search" => {
            if let Some(prev) = state.active_search.lock().await.take() {
                prev.cancel();
            }
            json!({ "ok": true })
        }

        "connectors:logo" => {
            let site = a0.and_then(|v| v.as_str()).unwrap_or("");
            let board = a1.and_then(|v| v.as_str()).unwrap_or("");
            if site.is_empty() && board.is_empty() {
                bad("Enter a company website link or a board")
            } else {
                let mut pairs = Vec::new();
                if !site.is_empty() {
                    pairs.push(format!("url={}", seg(site)));
                }
                if !board.is_empty() {
                    pairs.push(format!("board={}", seg(board)));
                }
                state
                    .backend
                    .call(
                        Method::GET,
                        &format!("/v1/connectors/greenhouse/logo?{}", pairs.join("&")),
                        None,
                    )
                    .await
            }
        }

        "resume:importFile" => {
            let (tx, rx) = oneshot::channel();
            app.dialog()
                .file()
                .set_title("Import a resume")
                .add_filter("Resumes", IMPORT_EXTENSIONS)
                .pick_file(move |picked| {
                    let _ = tx.send(picked);
                });

            let picked = match rx.await {
                Ok(Some(FilePath::Path(path))) => path,
                _ => return Ok(json!({ "canceled": true })),
            };

            let name = picked
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or("resume")
                .to_string();

            match std::fs::metadata(&picked) {
                Ok(meta) => {
                    if let Some(err) = check_import(&name, meta.len() as usize) {
                        err
                    } else {
                        match std::fs::read(&picked) {
                            Ok(bytes) => {
                                state
                                    .backend
                                    .call(
                                        Method::POST,
                                        "/v1/resume/import",
                                        Some(json!({
                                            "filename": base_name(&name),
                                            "data_base64": BASE64.encode(bytes),
                                        })),
                                    )
                                    .await
                            }
                            Err(_) => bad("The file is empty or could not be read."),
                        }
                    }
                }
                Err(_) => bad("The file is empty or could not be read."),
            }
        }

        "resume:importData" => match (a0.and_then(|v| v.as_str()), a1) {
            (Some(filename), Some(Value::String(b64))) => match BASE64.decode(b64) {
                Ok(bytes) => {
                    if let Some(err) = check_import(filename, bytes.len()) {
                        err
                    } else {
                        state
                            .backend
                            .call(
                                Method::POST,
                                "/v1/resume/import",
                                Some(json!({
                                    "filename": base_name(filename),
                                    "data_base64": b64,
                                })),
                            )
                            .await
                    }
                }
                Err(_) => bad("The file is empty or could not be read."),
            },
            (Some(filename), Some(Value::Array(arr))) => {
                let bytes: Option<Vec<u8>> = arr
                    .iter()
                    .map(|v| v.as_u64().and_then(|n| u8::try_from(n).ok()))
                    .collect();
                match bytes {
                    Some(bytes) => {
                        if let Some(err) = check_import(filename, bytes.len()) {
                            err
                        } else {
                            state
                                .backend
                                .call(
                                    Method::POST,
                                    "/v1/resume/import",
                                    Some(json!({
                                        "filename": base_name(filename),
                                        "data_base64": BASE64.encode(bytes),
                                    })),
                                )
                                .await
                        }
                    }
                    None => bad("The file is empty or could not be read."),
                }
            }
            _ => bad("The file is empty or could not be read."),
        },

        "resume:export" => match a0.and_then(|v| v.as_str()) {
            Some(fmt) => {
                let style = a1.and_then(|v| v.as_str());
                save_export(&app, &state, "/v1/resume/export", fmt, style).await
            }
            None => bad("Format must be pdf, docx or md"),
        },

        "tailored:export" => match (as_id(a0), a1.and_then(|v| v.as_str())) {
            (Some(id), Some(fmt)) => {
                let style = args.get(2).and_then(|v| v.as_str());
                let mut res = save_export(
                    &app,
                    &state,
                    &format!("/v1/tailored-resumes/{id}/export"),
                    fmt,
                    style,
                )
                .await;
                if res.get("saved").and_then(|v| v.as_bool()) == Some(true) && fmt == "pdf" {
                    if let Some(path) = res.get("path").and_then(|v| v.as_str()).map(str::to_string)
                    {
                        if let Some(app_id) = record_export(&state, id, &path).await {
                            if let Some(obj) = res.as_object_mut() {
                                obj.insert("application_id".into(), json!(app_id));
                            }
                        }
                    }
                }
                res
            }
            (None, _) => bad("Invalid id"),
            (_, None) => bad("Format must be pdf, docx or md"),
        },

        "resume:previewPdf" => {
            let style = a0.and_then(|v| v.as_str());
            preview_pdf(&state, "/v1/resume/export", style).await
        }
        "tailored:previewPdf" => match as_id(a0) {
            Some(id) => {
                let style = a1.and_then(|v| v.as_str());
                preview_pdf(&state, &format!("/v1/tailored-resumes/{id}/export"), style).await
            }
            None => bad("Invalid id"),
        },

        "ai:setRemote" => match as_object(a0) {
            Some(remote) => {
                let base_url = remote
                    .get("base_url")
                    .and_then(|v| v.as_str())
                    .map(str::trim)
                    .unwrap_or("");
                if base_url.is_empty() {
                    bad("Enter the provider address.")
                } else {
                    let model = remote
                        .get("model")
                        .and_then(|v| v.as_str())
                        .map(str::trim)
                        .unwrap_or("");
                    let key = remote
                        .get("api_key")
                        .and_then(|v| v.as_str())
                        .map(str::trim)
                        .unwrap_or("");
                    let mut body = json!({ "base_url": base_url, "model": model });
                    if !key.is_empty() {
                        body["api_key"] = Value::String(key.to_string());
                    }
                    let mut res = state
                        .backend
                        .call(Method::PUT, "/v1/ai/remote", Some(body))
                        .await;
                    if res.get("error").is_none() {
                        let remembered = if !key.is_empty() {
                            state.secrets.set(key)
                        } else {
                            false
                        };
                        if let Some(obj) = res.as_object_mut() {
                            obj.insert("key_remembered".into(), json!(remembered));
                        }
                    }
                    res
                }
            }
            None => bad("Enter the provider address."),
        },

        "ai:test" => match a0
            .and_then(|v| v.as_str())
            .filter(|e| AI_ENGINES.contains(e))
        {
            Some(engine) => {
                let res = state
                    .backend
                    .call(Method::POST, "/v1/ai/test", Some(json!({ "engine": engine })))
                    .await;
                if res.get("error").map(|e| e.is_object()).unwrap_or(false) {
                    res
                } else if let Some(mut obj) = res.as_object().cloned() {
                    let problem = obj.remove("error").unwrap_or(Value::Null);
                    obj.insert("problem".into(), problem);
                    Value::Object(obj)
                } else {
                    json!({ "problem": Value::Null })
                }
            }
            None => bad("Test the local or the remote engine."),
        },

        "ai:clearRemote" => {
            let res = state
                .backend
                .call(Method::DELETE, "/v1/ai/remote", None)
                .await;
            if res.get("error").is_none() {
                state.secrets.clear();
                json!({ "ok": true })
            } else {
                res
            }
        }

        "shell:showItem" => match a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
            Some(file_path) => {
                let mut known = state
                    .last_saved
                    .lock()
                    .await
                    .as_ref()
                    .map(|p| p == &PathBuf::from(file_path))
                    .unwrap_or(false);
                if !known {
                    let list = state
                        .backend
                        .call(Method::GET, "/v1/applications?archived=include", None)
                        .await;
                    known = list
                        .as_array()
                        .map(|arr| {
                            arr.iter().any(|a| {
                                a.get("exported_pdf_path").and_then(|v| v.as_str())
                                    == Some(file_path)
                            })
                        })
                        .unwrap_or(false);
                }
                if !known {
                    bad("Only a file Fistbump saved can be shown.")
                } else if let Err(err) = app.opener().reveal_item_in_dir(file_path) {
                    bad(format!("Could not show file: {err}"))
                } else {
                    json!({ "ok": true })
                }
            }
            None => bad("Only a file Fistbump saved can be shown."),
        },

        "storage:cleanup" => match as_object(a0) {
            Some(payload) => {
                let targets = payload
                    .get("targets")
                    .and_then(|v| v.as_array())
                    .filter(|arr| {
                        !arr.is_empty()
                            && arr.iter().all(|t| {
                                t.as_str()
                                    .map(|s| CLEANUP_TARGETS.contains(&s))
                                    .unwrap_or(false)
                            })
                    });
                let Some(targets) = targets else {
                    return Ok(bad("Unknown cleanup target"));
                };
                let mut body = json!({ "targets": targets });
                if let Some(days) = payload.get("older_than_days") {
                    match days.as_i64() {
                        Some(n) if n >= 1 => body["older_than_days"] = json!(n),
                        _ => return Ok(bad("older_than_days must be a whole number of days")),
                    }
                }
                if let Some(dry_run) = payload.get("dry_run") {
                    match dry_run.as_bool() {
                        Some(b) => body["dry_run"] = json!(b),
                        None => return Ok(bad("dry_run must be true or false")),
                    }
                }
                state
                    .backend
                    .call(Method::POST, "/v1/storage/cleanup", Some(body))
                    .await
            }
            None => bad("Invalid cleanup request"),
        },

        "app:info" => {
            let (platform, arch) = node_platform_arch();
            json!({
                "version": app.package_info().version.to_string(),
                "tauri": tauri::VERSION,
                "platform": platform,
                "arch": arch,
                "dataDir": state.backend.data_dir().to_string_lossy().to_string(),
            })
        }

        "app:show-data-folder" => {
            let dir = state.backend.data_dir().to_string_lossy().to_string();
            match app.opener().open_path(&dir, None::<&str>) {
                Ok(()) => json!({ "ok": true }),
                Err(err) => json!({
                    "error": {
                        "code": "internal",
                        "message": format!("Could not open data folder: {err}"),
                        "details": Value::Null
                    }
                }),
            }
        }

        "app:open-releases" => match a0.and_then(|v| v.as_str()) {
            Some(url) if url.starts_with("https://") => {
                match app.opener().open_url(url, None::<&str>) {
                    Ok(()) => json!({ "ok": true }),
                    Err(err) => bad(format!("Could not open link: {err}")),
                }
            }
            _ => bad("Invalid release link"),
        },

        //Opt-in desktop integration, see desktop.rs
        #[cfg(desktop)]
        "app:setShortcut" => crate::desktop::set_shortcut(&app, a0.and_then(|v| v.as_bool()).unwrap_or(false)),
        #[cfg(desktop)]
        "app:setTray" => {
            let enabled = a0.and_then(|v| v.as_bool()).unwrap_or(false);
            let status = a1.and_then(|v| v.get("status")).and_then(|v| v.as_str()).unwrap_or("Nothing due today");
            let count = a1.and_then(|v| v.get("count")).and_then(|v| v.as_u64()).unwrap_or(0);
            crate::desktop::set_tray(&app, enabled, status, count)
        }
        #[cfg(desktop)]
        "app:notify" => match (as_non_empty_str(a0), a1.and_then(|v| v.as_str())) {
            (Some(title), body) => crate::desktop::notify(&app, title, body.unwrap_or("")),
            _ => bad("A notification needs a title"),
        },
        "calendar:feeds" => crate::calendar::count(),
        "calendar:add-feed" => crate::calendar::add_feed(a0).await,
        "calendar:clear-feeds" => crate::calendar::clear(),
        "calendar:busy" => crate::calendar::busy().await,
        "calendar:open-ics" => crate::calendar::open_ics(&app, a0, args.get(1)),
        "relay:similar" | "relay:report" | "relay:share-boards" | "relay:boards" => crate::relay::call(channel.as_str(), a0).await,
        "app:setMenuState" => match a0 {
            Some(v) if v.is_object() => crate::menus::set_menu_state(&app, v),
            _ => bad("The menu state must be an object"),
        },
        "app:check-updates" => crate::updates::check_for_updates(&app, &state).await,
        "app:download-update" => crate::updates::download_update(&app, &state).await,
        "app:cancel-update" => crate::updates::cancel_update(&state).await,
        "app:install-update" => crate::updates::install_update(&app, &state).await,
        "app:open-update" => {
            let url = format!("{}/releases", crate::menus::GITHUB_URL);
            let _ = app.opener().open_url(&url, None::<&str>);
            json!({ "ok": true })
        }

        "error:details" => {
            let err = state.startup_error.lock().await;
            json!({
                "message": err.message,
                "stderr": err.stderr,
            })
        }

        "error:copyDetails" => {
            let text = {
                let err = state.startup_error.lock().await;
                let mut lines = vec![err.message.clone()];
                lines.extend(err.stderr.clone());
                lines.join("\n")
            };
            match app.clipboard().write_text(text) {
                Ok(()) => json!({ "ok": true }),
                Err(err) => bad(format!("Could not copy details: {err}")),
            }
        }

        "error:quit" => {
            state.explicit_quit.store(true, Ordering::SeqCst);
            app.exit(0);
            json!({ "ok": true })
        }

        _ => bad(format!("Unknown channel: {channel}")),
    };

    Ok(result)
}
