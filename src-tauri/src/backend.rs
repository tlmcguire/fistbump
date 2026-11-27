// Manages the Go backend (`fistbump-core`): spawns it, reads its port, authenticates
// requests with a per-launch token, and restarts it on unexpected exit.

use std::collections::VecDeque;
use std::future::Future;
use std::net::Ipv4Addr;
use std::path::{Path, PathBuf};
use std::pin::Pin;
use std::process::Stdio;
use std::sync::Arc;
use std::time::{Duration, Instant};

use rand::RngCore;
use reqwest::header::{HeaderMap, CONTENT_TYPE};
use reqwest::Method;
use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{ChildStdin, Command};
use tokio::sync::{oneshot, Mutex, Notify};
use tokio::time::timeout;
use tokio_util::sync::CancellationToken;

pub const STARTUP_TIMEOUT: Duration = Duration::from_secs(10);
pub const SHUTDOWN_TIMEOUT: Duration = Duration::from_secs(5);
pub const MAX_RESTARTS: usize = 3;
pub const RESTART_WINDOW: Duration = Duration::from_secs(60);
pub const STDERR_RING_CAP: usize = 200;
pub const STDERR_TAIL_LINES: usize = 40;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BackendState {
    Stopped,
    Starting,
    Ready,
    Crashed,
    Failed,
    Stopping,
}

#[derive(Debug, Clone)]
pub enum BackendEvent {
    Ready,
    Restarted,
    Failed { reason: String },
}

pub type EventCallback = Arc<dyn Fn(BackendEvent) + Send + Sync>;

struct Inner {
    state: BackendState,
    port: Option<u16>,
    token: Option<String>,
    stdin: Option<ChildStdin>,
    abort_tx: Option<oneshot::Sender<()>>,
    generation: u64,
    stderr_tail: VecDeque<String>,
    restarts: Vec<Instant>,
    quitting: bool,
    restarting: bool,
    exit_notify: Arc<Notify>,
}

pub struct BackendManager {
    bin_dir: PathBuf,
    data_dir: PathBuf,
    http: reqwest::Client,
    inner: Mutex<Inner>,
    on_event: Mutex<Option<EventCallback>>,
}

pub struct RawResponse {
    pub status: u16,
    pub headers: HeaderMap,
    pub bytes: Vec<u8>,
}

impl BackendManager {
    pub fn new(bin_dir: PathBuf, data_dir: PathBuf) -> Arc<Self> {
        let http = reqwest::Client::builder()
            .no_proxy()
            .redirect(reqwest::redirect::Policy::none())
            .tcp_nodelay(true)
            .pool_max_idle_per_host(8)
            .build()
            .expect("could not initialize the loopback HTTP client");
        Arc::new(Self {
            bin_dir,
            data_dir,
            http,
            inner: Mutex::new(Inner {
                state: BackendState::Stopped,
                port: None,
                token: None,
                stdin: None,
                abort_tx: None,
                generation: 0,
                stderr_tail: VecDeque::with_capacity(STDERR_RING_CAP),
                restarts: Vec::new(),
                quitting: false,
                restarting: false,
                exit_notify: Arc::new(Notify::new()),
            }),
            on_event: Mutex::new(None),
        })
    }

    pub async fn set_event_callback(&self, cb: EventCallback) {
        *self.on_event.lock().await = Some(cb);
    }

    async fn emit(&self, event: BackendEvent) {
        let cb = self.on_event.lock().await.clone();
        if let Some(cb) = cb {
            cb(event);
        }
    }

    pub fn binary_path(&self) -> PathBuf {
        let name = if cfg!(windows) {
            "fistbump-core.exe"
        } else {
            "fistbump-core"
        };
        self.bin_dir.join(name)
    }

    pub fn data_dir(&self) -> &Path {
        &self.data_dir
    }

    pub async fn state(&self) -> BackendState {
        self.inner.lock().await.state.clone()
    }

    pub async fn stderr_lines(&self, n: usize) -> Vec<String> {
        let inner = self.inner.lock().await;
        let count = n.min(inner.stderr_tail.len());
        inner
            .stderr_tail
            .iter()
            .skip(inner.stderr_tail.len() - count)
            .cloned()
            .collect()
    }

    async fn tail_string(&self, n: usize) -> String {
        self.stderr_lines(n).await.join("\n")
    }

    /// Starts `fistbump-core`, reads its `PORT=<n>` line, and verifies `GET /v1/health`.
    pub async fn start(self: &Arc<Self>) -> Result<(), String> {
        let bin = self.binary_path();
        if !bin.exists() {
            return Err(format!(
                "Backend binary not found at {}. Run \"npm run build:go\".",
                bin.display()
            ));
        }
        std::fs::create_dir_all(&self.data_dir).map_err(|e| {
            format!(
                "Could not create data directory {}: {e}",
                self.data_dir.display()
            )
        })?;

        let (was_restart, token, generation) = {
            let mut inner = self.inner.lock().await;
            let was_restart = inner.restarting;
            inner.restarting = false;
            inner.state = BackendState::Starting;
            let token = generate_token();
            inner.token = Some(token.clone());
            inner.port = None;
            if !was_restart {
                inner.stderr_tail.clear();
            }
            inner.generation += 1;
            (was_restart, token, inner.generation)
        };

        let mut cmd = Command::new(&bin);
        cmd.arg("--data-dir")
            .arg(&self.data_dir)
            .env("FISTBUMP_TOKEN", &token)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .kill_on_drop(true);

        let mut child = cmd
            .spawn()
            .map_err(|e| format!("Failed to spawn backend {}: {e}", bin.display()))?;

        let stdin = child.stdin.take();
        let stdout = child
            .stdout
            .take()
            .ok_or_else(|| "Failed to capture backend stdout".to_string())?;
        let stderr = child
            .stderr
            .take()
            .ok_or_else(|| "Failed to capture backend stderr".to_string())?;

        {
            let mut inner = self.inner.lock().await;
            inner.stdin = stdin;
            inner.abort_tx = None;
            inner.exit_notify = Arc::new(Notify::new());
        }

        let mgr_stderr = Arc::clone(self);
        tokio::spawn(async move {
            let mut lines = BufReader::new(stderr).lines();
            while let Ok(Some(line)) = lines.next_line().await {
                if line.is_empty() {
                    continue;
                }
                let mut inner = mgr_stderr.inner.lock().await;
                push_stderr_line(&mut inner.stderr_tail, line);
            }
        });

        let mut stdout_reader = BufReader::new(stdout).lines();
        let port_result = timeout(STARTUP_TIMEOUT, async {
            while let Ok(Some(line)) = stdout_reader.next_line().await {
                if let Some(port) = parse_port_line(&line) {
                    return Ok(port);
                }
            }
            Err("Backend exited before reporting a port.".to_string())
        })
        .await;

        let port = match port_result {
            Ok(Ok(p)) => p,
            Ok(Err(msg)) => {
                drop(child);
                self.mark_startup_failed(generation).await;
                let tail = self.tail_string(20).await;
                return Err(if tail.is_empty() {
                    msg
                } else {
                    format!("{msg}\n{tail}")
                });
            }
            Err(_) => {
                drop(child);
                self.mark_startup_failed(generation).await;
                let tail = self.tail_string(20).await;
                return Err(format!("Backend did not report a port in time.\n{tail}"));
            }
        };

        tokio::spawn(async move {
            while let Ok(Some(_)) = stdout_reader.next_line().await {}
        });

        {
            let mut inner = self.inner.lock().await;
            if inner.quitting {
                drop(inner);
                drop(child);
                let mut inner = self.inner.lock().await;
                inner.stdin = None;
                inner.state = BackendState::Stopped;
                return Ok(());
            }
            inner.port = Some(port);
        }

        if let Err(err) = self.health().await {
            drop(child);
            self.mark_startup_failed(generation).await;
            return Err(err);
        }

        let (abort_tx, mut abort_rx) = oneshot::channel::<()>();
        {
            let mut inner = self.inner.lock().await;
            inner.abort_tx = Some(abort_tx);
        }

        let mgr_watch = Arc::clone(self);
        tokio::spawn(async move {
            let code = tokio::select! {
                status = child.wait() => status.ok().and_then(|s| s.code()),
                _ = &mut abort_rx => {
                    drop(child);
                    None
                }
            };
            mgr_watch.on_exit(generation, code).await;
        });

        {
            let mut inner = self.inner.lock().await;
            if inner.quitting {
                return Ok(());
            }
            inner.state = BackendState::Ready;
        }

        self.emit(BackendEvent::Ready).await;
        if was_restart {
            self.emit(BackendEvent::Restarted).await;
        }
        Ok(())
    }

    async fn mark_startup_failed(&self, generation: u64) {
        let mut inner = self.inner.lock().await;
        if inner.generation == generation {
            inner.stdin = None;
            inner.abort_tx = None;
            inner.state = if inner.quitting {
                BackendState::Stopped
            } else {
                BackendState::Failed
            };
        }
    }

    fn on_exit<'a>(
        self: &'a Arc<Self>,
        generation: u64,
        _code: Option<i32>,
    ) -> Pin<Box<dyn Future<Output = ()> + Send + 'a>> {
        Box::pin(async move {
            let should_restart = {
                let mut inner = self.inner.lock().await;
                if inner.generation != generation {
                    return;
                }
                inner.stdin = None;
                inner.abort_tx = None;
                inner.exit_notify.notify_waiters();

                if inner.state == BackendState::Stopping || inner.quitting {
                    inner.state = BackendState::Stopped;
                    return;
                }
                if inner.state == BackendState::Starting || inner.state == BackendState::Failed {
                    return;
                }

                inner.state = BackendState::Crashed;
                if !record_restart(&mut inner.restarts, Instant::now()) {
                    inner.state = BackendState::Failed;
                    false
                } else {
                    inner.restarting = true;
                    true
                }
            };

            if !should_restart {
                self.emit(BackendEvent::Failed {
                    reason: "too many crashes".to_string(),
                })
                .await;
                return;
            }

            let mgr = Arc::clone(self);
            tokio::spawn(async move {
                if let Err(err) = mgr.start().await {
                    mgr.emit(BackendEvent::Failed { reason: err }).await;
                }
            });
        })
    }

    async fn health(&self) -> Result<Value, String> {
        let res = self.request_raw(Method::GET, "/v1/health", None).await?;
        if res.status != 200 {
            return Err(format!("Backend health check failed ({}).", res.status));
        }
        serde_json::from_slice(&res.bytes)
            .map_err(|e| format!("Backend health response was not valid JSON: {e}"))
    }

    pub async fn request_raw(
        &self,
        method: Method,
        url_path: &str,
        body: Option<Value>,
    ) -> Result<RawResponse, String> {
        self.request_raw_cancellable(method, url_path, body, None)
            .await
    }

    pub async fn request_raw_cancellable(
        &self,
        method: Method,
        url_path: &str,
        body: Option<Value>,
        cancel: Option<CancellationToken>,
    ) -> Result<RawResponse, String> {
        let (port, token) = {
            let inner = self.inner.lock().await;
            match (inner.port, inner.token.clone(), &inner.state) {
                (Some(p), Some(t), BackendState::Ready | BackendState::Starting) => (p, t),
                _ => {
                    return Err(
                        "The backend is not running. It may be restarting; try again in a moment."
                            .to_string(),
                    );
                }
            }
        };

        let url = format!("http://{}:{port}{url_path}", Ipv4Addr::LOCALHOST);
        let mut req = self.http.request(method, &url).bearer_auth(&token);

        if let Some(json_body) = body {
            req = req
                .header(CONTENT_TYPE, "application/json")
                .json(&json_body);
        }

        let send_fut = async {
            let res = req.send().await.map_err(|_| {
                "Could not reach the Fistbump backend. Restart the app and try again.".to_string()
            })?;
            let status = res.status().as_u16();
            let headers = res.headers().clone();
            let bytes = res
                .bytes()
                .await
                .map_err(|e| format!("Failed to read backend response: {e}"))?
                .to_vec();
            Ok(RawResponse {
                status,
                headers,
                bytes,
            })
        };

        match cancel {
            Some(token) => tokio::select! {
                _ = token.cancelled() => Err("canceled".to_string()),
                result = send_fut => result,
            },
            None => send_fut.await,
        }
    }

    pub async fn call(&self, method: Method, url_path: &str, body: Option<Value>) -> Value {
        self.call_cancellable(method, url_path, body, None).await
    }

    pub async fn call_cancellable(
        &self,
        method: Method,
        url_path: &str,
        body: Option<Value>,
        cancel: Option<CancellationToken>,
    ) -> Value {
        let was_canceled = cancel.as_ref().map(|c| c.is_cancelled()).unwrap_or(false);
        match self
            .request_raw_cancellable(method, url_path, body, cancel)
            .await
        {
            Ok(res) => {
                if res.status == 204 {
                    return json!({ "ok": true });
                }
                let is_json = res
                    .headers
                    .get(CONTENT_TYPE)
                    .and_then(|v| v.to_str().ok())
                    .map(|v| v.contains("application/json"))
                    .unwrap_or(false);
                if is_json && !res.bytes.is_empty() {
                    if let Ok(val) = serde_json::from_slice::<Value>(&res.bytes) {
                        return val;
                    }
                }
                json!({
                    "error": {
                        "code": "internal",
                        "message": "The backend sent an unreadable response",
                        "details": Value::Null
                    }
                })
            }
            Err(msg) if was_canceled || msg == "canceled" => json!({
                "error": {
                    "code": "canceled",
                    "message": "Search canceled",
                    "details": Value::Null
                }
            }),
            Err(msg) => json!({
                "error": {
                    "code": "unreachable",
                    "message": msg,
                    "details": Value::Null
                }
            }),
        }
    }

    /// Closes stdin (the quit signal for `fistbump-core`) and waits up to 5 seconds before stopping.
    pub async fn stop(&self) {
        let (abort_tx, notify) = {
            let mut inner = self.inner.lock().await;
            inner.quitting = true;
            if inner.abort_tx.is_none() && inner.stdin.is_none() {
                inner.state = BackendState::Stopped;
                return;
            }
            inner.state = BackendState::Stopping;
            inner.stdin = None;
            (inner.abort_tx.take(), Arc::clone(&inner.exit_notify))
        };

        if timeout(SHUTDOWN_TIMEOUT, notify.notified()).await.is_err() {
            if let Some(tx) = abort_tx {
                let _ = tx.send(());
            }
        }
    }
}

pub fn node_platform_arch() -> (&'static str, &'static str) {
    let os = match std::env::consts::OS {
        "macos" => "darwin",
        "windows" => "win32",
        "linux" => "linux",
        other => other,
    };
    let arch = match std::env::consts::ARCH {
        "aarch64" => "arm64",
        "x86_64" => "x64",
        other => other,
    };
    (os, arch)
}

pub fn resolve_data_dir(base_app_data: PathBuf) -> PathBuf {
    if let Some(override_dir) = std::env::var_os("FISTBUMP_USER_DATA") {
        if !override_dir.is_empty() {
            return PathBuf::from(override_dir);
        }
    }
    base_app_data.join("fistbump")
}

pub fn resolve_bin_dir(resource_dir: Option<PathBuf>) -> PathBuf {
    if cfg!(debug_assertions) {
        let (os, arch) = node_platform_arch();
        Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("..")
            .join("bin")
            .join(format!("{os}-{arch}"))
    } else {
        resource_dir
            .unwrap_or_else(|| PathBuf::from("."))
            .join("bin")
    }
}

fn generate_token() -> String {
    let mut bytes = [0u8; 32];
    rand::thread_rng().fill_bytes(&mut bytes);
    hex::encode(bytes)
}

fn parse_port_line(line: &str) -> Option<u16> {
    let rest = line.trim().strip_prefix("PORT=")?;
    if rest.is_empty() || !rest.chars().all(|c| c.is_ascii_digit()) {
        return None;
    }
    rest.parse::<u16>().ok()
}

fn push_stderr_line(buf: &mut VecDeque<String>, line: String) {
    if buf.len() >= STDERR_RING_CAP {
        buf.pop_front();
    }
    buf.push_back(line);
}

fn record_restart(restarts: &mut Vec<Instant>, now: Instant) -> bool {
    restarts.retain(|t| now.saturating_duration_since(*t) < RESTART_WINDOW);
    if restarts.len() >= MAX_RESTARTS {
        return false;
    }
    restarts.push(now);
    true
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_valid_port_lines() {
        assert_eq!(parse_port_line("PORT=54321"), Some(54321));
        assert_eq!(parse_port_line("PORT=8080\r"), Some(8080));
        assert_eq!(parse_port_line("INFO starting"), None);
        assert_eq!(parse_port_line("PORT=abc"), None);
        assert_eq!(parse_port_line("PORT=99999"), None);
    }

    #[test]
    fn caps_stderr_ring_at_200_lines() {
        let mut buf = VecDeque::new();
        for i in 1..=250 {
            push_stderr_line(&mut buf, format!("line {i}"));
        }
        assert_eq!(buf.len(), STDERR_RING_CAP);
        assert_eq!(buf.front().map(String::as_str), Some("line 51"));
        assert_eq!(buf.back().map(String::as_str), Some("line 250"));
    }

    #[test]
    fn enforces_max_three_restarts_per_minute() {
        let mut restarts = Vec::new();
        let t0 = Instant::now();
        assert!(record_restart(&mut restarts, t0));
        assert!(record_restart(&mut restarts, t0 + Duration::from_secs(10)));
        assert!(record_restart(&mut restarts, t0 + Duration::from_secs(20)));
        assert!(!record_restart(&mut restarts, t0 + Duration::from_secs(30)));
        assert!(record_restart(&mut restarts, t0 + Duration::from_secs(61)));
    }

    #[test]
    fn generates_64_char_hex_tokens() {
        let a = generate_token();
        let b = generate_token();
        assert_eq!(a.len(), 64);
        assert_ne!(a, b);
        assert!(a.chars().all(|c| c.is_ascii_hexdigit()));
    }
}
