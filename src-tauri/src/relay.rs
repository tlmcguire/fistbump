//! The one place Fistbump talks to a server of its own: the community relay (relay/ in this repository), a small
//! Cloudflare Worker that files issue reports on GitHub and collects job boards people chose to share. It runs only
//! when the person asks for it (sending a report, sharing boards, or using shared boards in Settings).
//!
//! The page cannot reach the network itself (its content security policy allows only the app), so it asks through
//! the `relay:*` channels, and only these paths can be reached.

use std::time::Duration;

use serde_json::{json, Value};

/// The relay's address. A build can point elsewhere with FISTBUMP_RELAY_URL (at build time or when it runs).
const DEFAULT_RELAY: &str = "https://fistbump-relay.tlmcguire.workers.dev";

fn base() -> String {
    std::env::var("FISTBUMP_RELAY_URL")
        .ok()
        .or_else(|| option_env!("FISTBUMP_RELAY_URL").map(str::to_string))
        .unwrap_or_else(|| DEFAULT_RELAY.to_string())
        .trim_end_matches('/')
        .to_string()
}

fn fail(code: &str, message: impl Into<String>) -> Value {
    json!({ "error": { "code": code, "message": message.into(), "details": Value::Null } })
}

/// `relay:similar` (words) · `relay:report` (report) · `relay:share-boards` (boards) · `relay:boards` () · `relay:share-listing` (listing) · `relay:listings` ().
pub async fn call(channel: &str, a0: Option<&Value>) -> Value {
    let client = match reqwest::Client::builder()
        .timeout(Duration::from_secs(20))
        .user_agent(concat!("Fistbump/", env!("CARGO_PKG_VERSION")))
        .build()
    {
        Ok(c) => c,
        Err(err) => return fail("internal", format!("Could not start a connection: {err}")),
    };
    let root = base();
    let request = match channel {
        "relay:similar" => {
            let words = a0.and_then(Value::as_str).unwrap_or("").chars().take(200).collect::<String>();
            client.get(format!("{root}/v1/reports/similar")).query(&[("q", words)])
        }
        "relay:report" => match a0 {
            Some(body) if body.is_object() => client.post(format!("{root}/v1/reports")).json(body),
            _ => return fail("validation_failed", "A report needs a title and what happened."),
        },
        "relay:share-boards" => match a0 {
            Some(body) if body.is_object() => client.post(format!("{root}/v1/boards")).json(body),
            _ => return fail("validation_failed", "Nothing to share."),
        },
        "relay:boards" => client.get(format!("{root}/v1/boards")),
        "relay:share-listing" => match a0 {
            Some(body) if body.is_object() => client.post(format!("{root}/v1/listings")).json(body),
            _ => return fail("validation_failed", "Nothing to share."),
        },
        "relay:listings" => client.get(format!("{root}/v1/listings")),
        _ => return fail("validation_failed", format!("Unknown channel: {channel}")),
    };
    let response = match request.send().await {
        Ok(r) => r,
        Err(err) if err.is_timeout() => return fail("unavailable", "The Fistbump community service did not answer in time. Try again later."),
        Err(_) => return fail("unavailable", "Could not reach the Fistbump community service. Check your connection and try again."),
    };
    let status = response.status();
    let body: Value = response.json().await.unwrap_or(Value::Null);
    if status.is_success() {
        return body;
    }
    let message = body
        .pointer("/error/message")
        .and_then(Value::as_str)
        .map(str::to_string)
        .unwrap_or_else(|| format!("The community service answered {status}."));
    let code = if status.as_u16() == 429 { "rate_limited" } else { "unavailable" };
    fail(code, message)
}
