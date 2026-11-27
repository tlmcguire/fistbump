//! Busy times from the person's own calendars, so a new interview never lands on top of something else. Off until
//! they paste a calendar's private address (an "iCal" or "ICS" link from Google Calendar, Outlook or iCloud) in
//! Settings. The addresses work like passwords, so they are kept in the system keychain, never in the database, and
//! the page only ever learns how many there are. The page reads the calendars it gets back and keeps only the times.

use std::io::Write;
use std::time::Duration;

use serde_json::{json, Value};

const SERVICE_NAME: &str = "com.fistbump.desktop";
const FEEDS_USER: &str = "calendar-feeds";
const MAX_FEEDS: usize = 5;
const MAX_BYTES: usize = 4 * 1024 * 1024;

fn entry() -> Option<keyring::Entry> {
    keyring::Entry::new(SERVICE_NAME, FEEDS_USER).ok()
}

fn read_feeds() -> Vec<String> {
    entry()
        .and_then(|e| e.get_password().ok())
        .and_then(|s| serde_json::from_str::<Vec<String>>(&s).ok())
        .unwrap_or_default()
}

fn fail(code: &str, message: impl Into<String>) -> Value {
    json!({ "error": { "code": code, "message": message.into(), "details": Value::Null } })
}

/// A calendar address is an https (or webcal) link; webcal is the same thing over https.
fn normalize(url: &str) -> Option<String> {
    let url = url.trim();
    let url = url
        .strip_prefix("webcal://")
        .map(|rest| format!("https://{rest}"))
        .unwrap_or_else(|| url.to_string());
    let parsed = reqwest::Url::parse(&url).ok()?;
    (parsed.scheme() == "https"
        && parsed.host_str().is_some()
        && parsed.username().is_empty()
        && parsed.password().is_none()
        && parsed.fragment().is_none())
    .then(|| parsed.to_string())
}

/// `calendar:feeds` — how many calendars are connected (never their addresses).
pub fn count() -> Value {
    json!({ "count": read_feeds().len() })
}

/// `calendar:add-feed` (address) — checks it answers as a calendar, then keeps it in the keychain.
pub async fn add_feed(url: Option<&Value>) -> Value {
    let Some(url) = url.and_then(Value::as_str).and_then(normalize) else {
        return fail(
            "validation_failed",
            "Paste the calendar's private address. It starts with https:// or webcal://.",
        );
    };
    let mut feeds = read_feeds();
    if feeds.contains(&url) {
        return json!({ "count": feeds.len() });
    }
    if feeds.len() >= MAX_FEEDS {
        return fail(
            "validation_failed",
            "Five calendars is the most Fistbump checks.",
        );
    }
    match fetch(&url).await {
        Ok(text) if text.contains("BEGIN:VCALENDAR") => {}
        Ok(_) => return fail(
            "validation_failed",
            "That address did not answer with a calendar. Copy the private iCal or ICS address.",
        ),
        Err(message) => return fail("unavailable", message),
    }
    feeds.push(url);
    let saved = entry()
        .map(|e| {
            e.set_password(&serde_json::to_string(&feeds).unwrap_or_default())
                .is_ok()
        })
        .unwrap_or(false);
    if !saved {
        return fail(
            "internal",
            "The system keychain would not keep the address.",
        );
    }
    json!({ "count": feeds.len() })
}

/// `calendar:clear-feeds` — forgets every connected calendar.
pub fn clear() -> Value {
    if let Some(e) = entry() {
        let _ = e.delete_credential();
    }
    json!({ "count": 0 })
}

/// `calendar:busy` — the connected calendars' text, for the page to read busy times from.
pub async fn busy() -> Value {
    let feeds = read_feeds();
    let mut calendars = Vec::new();
    let mut failed = 0;
    for url in feeds {
        match fetch(&url).await {
            Ok(text) => calendars.push(Value::String(text)),
            Err(_) => failed += 1,
        }
    }
    json!({ "calendars": calendars, "failed": failed })
}

async fn fetch(url: &str) -> Result<String, String> {
    let url = normalize(url).ok_or_else(|| "That calendar address is invalid.".to_string())?;
    let client = reqwest::Client::builder()
        .redirect(reqwest::redirect::Policy::custom(|attempt| {
            let next = attempt.url();
            if attempt.previous().len() <= 3
                && next.origin() == attempt.previous()[0].origin()
                && next.username().is_empty()
                && next.password().is_none()
            {
                attempt.follow()
            } else {
                attempt.stop()
            }
        }))
        .timeout(Duration::from_secs(15))
        .user_agent(concat!("Fistbump/", env!("CARGO_PKG_VERSION")))
        .build()
        .map_err(|e| format!("Could not start a connection: {e}"))?;
    let mut response = client.get(url).send().await.map_err(|_| {
        "Could not reach that calendar. Check the address and your connection.".to_string()
    })?;
    if !response.status().is_success() {
        return Err(format!(
            "The calendar answered {}. The address may have been reset.",
            response.status()
        ));
    }
    let mut bytes = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|_| "The calendar stopped answering.".to_string())?
    {
        if chunk.len() > MAX_BYTES - bytes.len() {
            return Err("That calendar is too large to check.".into());
        }
        bytes.extend_from_slice(&chunk);
    }
    Ok(String::from_utf8_lossy(&bytes).into_owned())
}

#[cfg(test)]
mod tests {
    use super::normalize;

    #[test]
    fn calendar_addresses_are_https_only() {
        assert_eq!(
            normalize("webcal://p01-caldav.icloud.com/published/2/abc").as_deref(),
            Some("https://p01-caldav.icloud.com/published/2/abc")
        );
        assert!(
            normalize("https://calendar.google.com/calendar/ical/x/private-y/basic.ics").is_some()
        );
        assert!(normalize("http://example.com/cal.ics").is_none());
        assert!(normalize("file:///etc/passwd").is_none());
        assert!(normalize("not a url").is_none());
    }
}

/// `calendar:open-ics` (name, text) — writes the event to a file and opens it, so the computer's own calendar app
/// (Apple Calendar, Outlook, whichever the person set as default) offers to add it.
pub fn open_ics(app: &tauri::AppHandle, name: Option<&Value>, text: Option<&Value>) -> Value {
    use tauri_plugin_opener::OpenerExt;
    let Some(text) = text
        .and_then(Value::as_str)
        .filter(|t| t.starts_with("BEGIN:VCALENDAR") && t.len() < 512 * 1024)
    else {
        return fail("validation_failed", "That is not a calendar event.");
    };
    let stem: String = name
        .and_then(Value::as_str)
        .unwrap_or("Fistbump plan")
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || c == '-' || c == ' ' {
                c
            } else {
                '-'
            }
        })
        .take(60)
        .collect();
    let Ok(mut file) = tempfile::Builder::new()
        .prefix(stem.trim())
        .suffix(".ics")
        .tempfile()
    else {
        return fail("internal", "Could not write the calendar file.");
    };
    if file.write_all(text.as_bytes()).is_err() {
        return fail("internal", "Could not write the calendar file.");
    }
    match app
        .opener()
        .open_path(file.path().to_string_lossy(), None::<&str>)
    {
        Ok(()) => {
            // The calendar application may read the file asynchronously.
            file.disable_cleanup(true);
            json!({ "ok": true })
        }
        Err(err) => fail(
            "internal",
            format!("No calendar app opened the file: {err}"),
        ),
    }
}
