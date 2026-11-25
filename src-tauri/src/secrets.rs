// Stores the optional remote AI key in the OS keychain (macOS Keychain, Windows Credential
// Manager, Linux Secret Service) via `keyring`. Ports `app/main/secrets.js`.

use reqwest::Method;
use serde_json::json;

use crate::backend::BackendManager;

const SERVICE_NAME: &str = "com.fistbump.app";
const REMOTE_KEY_USER: &str = "ai-remote-key";

pub struct SecretsStore {
    service: String,
    account: String,
}

impl Default for SecretsStore {
    fn default() -> Self {
        Self::new()
    }
}

impl SecretsStore {
    pub fn new() -> Self {
        Self {
            service: SERVICE_NAME.to_string(),
            account: REMOTE_KEY_USER.to_string(),
        }
    }

    fn entry(&self) -> Result<keyring::Entry, keyring::Error> {
        keyring::Entry::new(&self.service, &self.account)
    }

    pub fn get(&self) -> Option<String> {
        let entry = self.entry().ok()?;
        entry.get_password().ok().filter(|s| !s.is_empty())
    }

    pub fn set(&self, secret: &str) -> bool {
        let Ok(entry) = self.entry() else {
            return false;
        };
        entry.set_password(secret).is_ok()
    }

    pub fn clear(&self) {
        if let Ok(entry) = self.entry() {
            let _ = entry.delete_credential();
        }
    }
}

/// At launch (and after a backend restart): when a remote provider is configured and a key
/// was saved in the OS keychain, send the key to `fistbump-core` so it stays only in memory.
pub async fn restore_remote_key(backend: &BackendManager, secrets: &SecretsStore) -> bool {
    let status = backend.call(Method::GET, "/v1/ai/status", None).await;
    let Some(remote) = status.get("remote").and_then(|v| v.as_object()) else {
        return false;
    };
    let base_url = remote
        .get("base_url")
        .and_then(|v| v.as_str())
        .unwrap_or("");
    let has_key = remote
        .get("has_key")
        .and_then(|v| v.as_bool())
        .unwrap_or(false);
    if base_url.is_empty() || has_key {
        return false;
    }
    let Some(key) = secrets.get() else {
        return false;
    };
    let model = remote.get("model").and_then(|v| v.as_str()).unwrap_or("");
    let res = backend
        .call(
            Method::PUT,
            "/v1/ai/remote",
            Some(json!({
                "base_url": base_url,
                "model": model,
                "api_key": key,
            })),
        )
        .await;
    res.get("error").is_none()
}
