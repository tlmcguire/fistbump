// Stores the optional remote AI key in the OS keychain (macOS Keychain, Windows Credential
// Manager, Linux Secret Service) via `keyring`. Ports `app/main/secrets.js`.

use reqwest::Method;
use serde::{Deserialize, Serialize};
use serde_json::json;
use tokio::sync::Mutex;

use crate::backend::BackendManager;

const SERVICE_NAME: &str = "com.fistbump.desktop";
const REMOTE_KEY_USER: &str = "ai-remote-key";

#[derive(Deserialize, Serialize)]
struct RemoteCredential {
    origin: String,
    key: String,
}

fn provider_origin(base_url: &str) -> Option<String> {
    let url = reqwest::Url::parse(base_url).ok()?;
    let origin = url.origin();
    matches!(url.scheme(), "http" | "https").then(|| origin.ascii_serialization())
}

pub struct SecretsStore {
    pub(crate) remote_change: Mutex<()>,
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
            remote_change: Mutex::new(()),
            service: SERVICE_NAME.to_string(),
            account: REMOTE_KEY_USER.to_string(),
        }
    }

    fn entry(&self) -> Result<keyring::Entry, keyring::Error> {
        keyring::Entry::new(&self.service, &self.account)
    }

    fn get(&self, base_url: &str) -> Option<String> {
        let credential = self.entry().ok()?.get_password().ok()?;
        key_for_provider(&credential, base_url)
    }

    pub fn set(&self, base_url: &str, key: &str) -> bool {
        let (Ok(entry), Some(origin)) = (self.entry(), provider_origin(base_url)) else {
            return false;
        };
        let credential = RemoteCredential {
            origin,
            key: key.to_owned(),
        };
        serde_json::to_string(&credential).is_ok_and(|value| entry.set_password(&value).is_ok())
    }

    pub fn clear(&self) {
        if let Ok(entry) = self.entry() {
            let _ = entry.delete_credential();
        }
    }
}

fn key_for_provider(value: &str, base_url: &str) -> Option<String> {
    let credential: RemoteCredential = serde_json::from_str(value).ok()?;
    (credential.origin == provider_origin(base_url)? && !credential.key.is_empty())
        .then_some(credential.key)
}

/// At launch (and after a backend restart): when a remote provider is configured and a key
/// was saved in the OS keychain, send the key to `fistbump-core` so it stays only in memory.
pub async fn restore_remote_key(backend: &BackendManager, secrets: &SecretsStore) -> bool {
    let _guard = secrets.remote_change.lock().await;
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
    let Some(key) = secrets.get(base_url) else {
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

#[cfg(test)]
mod tests {
    use super::key_for_provider;

    #[test]
    fn only_credentials_bound_to_the_current_origin_are_restored() {
        let value = r#"{"origin":"https://provider.example","key":"saved"}"#;
        assert_eq!(
            key_for_provider(value, "https://provider.example:443/v1").as_deref(),
            Some("saved")
        );
        assert!(key_for_provider(value, "https://other.example/v1").is_none());
        assert!(key_for_provider(value, "https://provider.example:8443/v1").is_none());
        assert!(key_for_provider("legacy-unbound-key", "https://provider.example/v1").is_none());
    }
}
