// Pure channel-to-route mapping and input validation for IPC commands.
// Ports the `ROUTES` table and validators from `app/main/ipc-handlers.js`.

use reqwest::Method;
use serde_json::{json, Value};

pub const MAX_IMPORT_BYTES: usize = 5 * 1024 * 1024;
pub const IMPORT_EXTENSIONS: &[&str] = &["pdf", "docx", "odt", "txt", "md"];
pub const AI_MODES: &[&str] = &["auto", "local", "remote", "rules"];
pub const AI_ENGINES: &[&str] = &["local", "remote"];
pub const CLEANUP_TARGETS: &[&str] = &[
    "cache",
    "partial_downloads",
    "old_jobs",
    "trash",
    "unused_models",
];

pub fn bad(message: impl Into<String>) -> Value {
    json!({
        "error": {
            "code": "validation_failed",
            "message": message.into(),
            "details": Value::Null
        }
    })
}

pub fn seg(s: &str) -> String {
    urlencoding::encode(s).into_owned()
}

pub fn as_id(v: Option<&Value>) -> Option<i64> {
    v.and_then(|x| x.as_i64()).filter(|&id| id > 0)
}

pub fn as_object(v: Option<&Value>) -> Option<&Value> {
    v.filter(|x| x.is_object())
}

pub fn as_non_empty_str(v: Option<&Value>) -> Option<&str> {
    v.and_then(|x| x.as_str()).filter(|s| !s.trim().is_empty())
}

pub fn as_id_list(v: Option<&Value>) -> Option<Vec<i64>> {
    let arr = v?.as_array()?;
    if arr.is_empty() || arr.len() > 1000 {
        return None;
    }
    let mut ids = Vec::with_capacity(arr.len());
    for item in arr {
        ids.push(as_id(Some(item))?);
    }
    Some(ids)
}

pub fn build_query(values: Option<&Value>, allowed: &[&str]) -> String {
    let Some(obj) = values.and_then(|v| v.as_object()) else {
        return String::new();
    };
    let mut pairs = Vec::new();
    for &key in allowed {
        if let Some(s) = obj.get(key).and_then(|v| v.as_str()) {
            if !s.is_empty() {
                pairs.push(format!("{}={}", seg(key), seg(s)));
            }
        }
    }
    if pairs.is_empty() {
        String::new()
    } else {
        format!("?{}", pairs.join("&"))
    }
}

pub fn base_name(name: &str) -> &str {
    name.rsplit(['/', '\\']).next().unwrap_or(name)
}

fn extension_of(name: &str) -> String {
    let base = base_name(name);
    match base.rsplit_once('.') {
        Some((_, ext)) if !ext.is_empty() => ext.to_ascii_lowercase(),
        _ => String::new(),
    }
}

pub fn check_import(name: &str, size: usize) -> Option<Value> {
    let ext = extension_of(name);
    if !IMPORT_EXTENSIONS.contains(&ext.as_str()) {
        return Some(bad(format!(
            "{} is not a PDF, .docx, .odt, .txt or .md file.",
            base_name(name)
        )));
    }
    if size == 0 {
        return Some(bad("The file is empty or could not be read."));
    }
    if size > MAX_IMPORT_BYTES {
        return Some(bad(
            "The file is larger than 5 MB. Export it again without images, or paste the text instead.",
        ));
    }
    None
}

/// Parses `Content-Disposition` (`filename*` or `filename`) and strips path segments/control chars.
pub fn suggested_name(header: Option<&str>, fallback: &str) -> String {
    let h = header.unwrap_or("");
    let mut name = String::new();

    if let Some(idx) = h.to_ascii_lowercase().find("filename*") {
        let rest = &h[idx..];
        if let Some(eq) = rest.find('=') {
            let mut val = rest[eq + 1..].split(';').next().unwrap_or("").trim();
            if let Some(pos) = val.to_ascii_lowercase().find("utf-8''") {
                val = &val[pos + 7..];
            }
            if let Ok(decoded) = urlencoding::decode(val) {
                name = decoded.into_owned();
            }
        }
    }

    if name.is_empty() {
        for part in h.split(';') {
            let trimmed = part.trim();
            let lower = trimmed.to_ascii_lowercase();
            if lower.starts_with("filename=") {
                let raw = trimmed["filename=".len()..].trim();
                name = raw.trim_matches('"').to_string();
                break;
            }
        }
    }

    let cleaned: String = base_name(&name)
        .chars()
        .filter(|&c| (c as u32) >= 0x20)
        .collect();
    let cleaned = cleaned.trim();
    if cleaned.is_empty() || cleaned == "." || cleaned == ".." {
        fallback.to_string()
    } else {
        cleaned.to_string()
    }
}

/// Maps direct REST channels to `(Method, path, optional JSON body)` or returns a validation error.
pub fn route_for_channel(
    channel: &str,
    args: &[Value],
) -> Option<Result<(Method, String, Option<Value>), Value>> {
    let a0 = args.first();
    let a1 = args.get(1);
    let a2 = args.get(2);

    let res = match channel {
        "resume:get" => Ok((Method::GET, "/v1/resume".into(), None)),
        "resume:putProfile" => match as_object(a0) {
            Some(p) => Ok((Method::PUT, "/v1/resume/profile".into(), Some(p.clone()))),
            None => Err(bad("Invalid profile")),
        },
        "resume:preview" => Ok((Method::GET, "/v1/resume/preview".into(), None)),
        "resume:audit" => {
            let qs = match a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
                Some(style) => format!("?style={}", seg(style)),
                None => String::new(),
            };
            Ok((Method::GET, format!("/v1/resume/audit{qs}"), None))
        }
        "ai:status" => Ok((Method::GET, "/v1/ai/status".into(), None)),
        "ai:setMode" => match a0.and_then(|v| v.as_str()).filter(|m| AI_MODES.contains(m)) {
            Some(mode) => Ok((
                Method::PUT,
                "/v1/ai/mode".into(),
                Some(json!({ "mode": mode })),
            )),
            None => Err(bad("Choose automatic, local, remote or rules.")),
        },
        "resume:importText" => match as_non_empty_str(a0) {
            Some(text) => Ok((
                Method::POST,
                "/v1/resume/import".into(),
                Some(json!({ "text": text })),
            )),
            None => Err(bad("Paste the text of your resume.")),
        },
        "resume:importApply" => match as_object(a0)
            .filter(|b| b.get("draft").map(|d| d.is_object()).unwrap_or(false))
        {
            Some(body) => Ok((
                Method::POST,
                "/v1/resume/import/apply".into(),
                Some(body.clone()),
            )),
            None => Err(bad("Invalid import")),
        },

        "experiences:create" => match as_object(a0) {
            Some(e) => Ok((
                Method::POST,
                "/v1/resume/experiences".into(),
                Some(e.clone()),
            )),
            None => Err(bad("Invalid experience")),
        },
        "experiences:update" => match (as_id(a0), as_object(a1)) {
            (Some(id), Some(e)) => Ok((
                Method::PUT,
                format!("/v1/resume/experiences/{id}"),
                Some(e.clone()),
            )),
            _ => Err(bad("Invalid experience")),
        },
        "experiences:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/resume/experiences/{id}"), None)),
            None => Err(bad("Invalid experience id")),
        },

        "education:create" => match as_object(a0) {
            Some(e) => Ok((
                Method::POST,
                "/v1/resume/education".into(),
                Some(e.clone()),
            )),
            None => Err(bad("Invalid education entry")),
        },
        "education:update" => match (as_id(a0), as_object(a1)) {
            (Some(id), Some(e)) => Ok((
                Method::PUT,
                format!("/v1/resume/education/{id}"),
                Some(e.clone()),
            )),
            _ => Err(bad("Invalid education entry")),
        },
        "education:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/resume/education/{id}"), None)),
            None => Err(bad("Invalid education id")),
        },

        "resumes:list" => Ok((Method::GET, "/v1/resumes".into(), None)),
        "resumes:get" => match as_id(a0) {
            Some(id) => Ok((Method::GET, format!("/v1/resumes/{id}"), None)),
            None => Err(bad("Invalid resume id")),
        },
        "resumes:create" => match as_object(a0) {
            Some(b) => Ok((Method::POST, "/v1/resumes".into(), Some(b.clone()))),
            None => Err(bad("Invalid resume")),
        },
        "resumes:update" => match (as_id(a0), as_object(a1)) {
            (Some(id), Some(b)) => Ok((Method::PUT, format!("/v1/resumes/{id}"), Some(b.clone()))),
            _ => Err(bad("Invalid resume")),
        },
        "resumes:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/resumes/{id}"), None)),
            None => Err(bad("Invalid resume id")),
        },

        "revisions:create" => match as_object(a0).filter(|b| as_id(b.get("job_id")).is_some()) {
            Some(b) => Ok((Method::POST, "/v1/revisions".into(), Some(b.clone()))),
            None => Err(bad("Choose a job first.")),
        },
        "revisions:list" => {
            let path = match as_id(a0) {
                Some(id) => format!("/v1/revisions?job_id={id}"),
                None => "/v1/revisions".into(),
            };
            Ok((Method::GET, path, None))
        }
        "revisions:get" => match as_id(a0) {
            Some(id) => Ok((Method::GET, format!("/v1/revisions/{id}"), None)),
            None => Err(bad("Invalid revision id")),
        },
        "revisions:cancel" => match as_id(a0) {
            Some(id) => Ok((Method::POST, format!("/v1/revisions/{id}/cancel"), None)),
            None => Err(bad("Invalid revision id")),
        },
        "revisions:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/revisions/{id}"), None)),
            None => Err(bad("Invalid revision id")),
        },
        "suggestions:accept" => match (as_id(a0), as_id(a1)) {
            (Some(rid), Some(sid)) => Ok((
                Method::POST,
                format!("/v1/revisions/{rid}/suggestions/{sid}/accept"),
                None,
            )),
            _ => Err(bad("Invalid suggestion")),
        },
        "suggestions:reject" => match (as_id(a0), as_id(a1)) {
            (Some(rid), Some(sid)) => Ok((
                Method::POST,
                format!("/v1/revisions/{rid}/suggestions/{sid}/reject"),
                None,
            )),
            _ => Err(bad("Invalid suggestion")),
        },
        "suggestions:edit" => match (as_id(a0), as_id(a1), as_non_empty_str(a2)) {
            (Some(rid), Some(sid), Some(text)) => Ok((
                Method::POST,
                format!("/v1/revisions/{rid}/suggestions/{sid}/edit"),
                Some(json!({ "edited_text": text })),
            )),
            _ => Err(bad("Write the edited text.")),
        },

        "tailored:create" => match as_id(a0) {
            Some(rid) => Ok((
                Method::POST,
                "/v1/tailored-resumes".into(),
                Some(json!({ "revision_id": rid })),
            )),
            None => Err(bad("Invalid revision id")),
        },
        "tailored:createBlank" => match as_id(a0) {
            Some(job_id) => {
                let body = match as_id(a1) {
                    Some(base_id) => json!({ "job_id": job_id, "base_resume_id": base_id }),
                    None => json!({ "job_id": job_id }),
                };
                Ok((Method::POST, "/v1/tailored-resumes".into(), Some(body)))
            }
            None => Err(bad("Choose a job first.")),
        },
        "tailored:list" => {
            let path = match as_id(a0) {
                Some(id) => format!("/v1/tailored-resumes?job_id={id}"),
                None => "/v1/tailored-resumes".into(),
            };
            Ok((Method::GET, path, None))
        }
        "tailored:get" => match as_id(a0) {
            Some(id) => Ok((Method::GET, format!("/v1/tailored-resumes/{id}"), None)),
            None => Err(bad("Invalid id")),
        },
        "tailored:update" => match (as_id(a0), as_non_empty_str(a1)) {
            (Some(id), Some(content)) => Ok((
                Method::PUT,
                format!("/v1/tailored-resumes/{id}"),
                Some(json!({ "content": content })),
            )),
            _ => Err(bad("The resume cannot be empty.")),
        },
        "tailored:diff" => match as_id(a0) {
            Some(id) => Ok((Method::GET, format!("/v1/tailored-resumes/{id}/diff"), None)),
            None => Err(bad("Invalid id")),
        },
        "tailored:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/tailored-resumes/{id}"), None)),
            None => Err(bad("Invalid id")),
        },

        "models:list" => Ok((Method::GET, "/v1/models".into(), None)),
        "models:get" => match a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
            Some(id) => Ok((Method::GET, format!("/v1/models/{}", seg(id)), None)),
            None => Err(bad("Invalid model id")),
        },
        "models:download" => match a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
            Some(id) => Ok((
                Method::POST,
                format!("/v1/models/{}/download", seg(id)),
                None,
            )),
            None => Err(bad("Invalid model id")),
        },
        "models:downloadStatus" => match a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
            Some(id) => Ok((
                Method::GET,
                format!("/v1/models/downloads/{}", seg(id)),
                None,
            )),
            None => Err(bad("Invalid download id")),
        },
        "models:cancelDownload" => match a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
            Some(id) => Ok((
                Method::DELETE,
                format!("/v1/models/downloads/{}", seg(id)),
                None,
            )),
            None => Err(bad("Invalid download id")),
        },
        "models:delete" => match a0.and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
            Some(id) => Ok((Method::DELETE, format!("/v1/models/{}", seg(id)), None)),
            None => Err(bad("Invalid model id")),
        },

        "settings:get" => Ok((Method::GET, "/v1/settings".into(), None)),
        "settings:put" => match as_object(a0) {
            Some(v) => Ok((Method::PUT, "/v1/settings".into(), Some(v.clone()))),
            None => Err(bad("Invalid settings")),
        },

        "jobs:parse" => match as_non_empty_str(a0) {
            Some(raw_text) => Ok((
                Method::POST,
                "/v1/jobs/parse".into(),
                Some(json!({ "raw_text": raw_text })),
            )),
            None => Err(bad("Paste the job posting.")),
        },
        "jobs:create" => match as_object(a0) {
            Some(b) => Ok((Method::POST, "/v1/jobs".into(), Some(b.clone()))),
            None => Err(bad("Invalid job")),
        },
        "jobs:list" => {
            let mut merged = a1.and_then(|v| v.as_object()).cloned().unwrap_or_default();
            if let Some(q) = a0.and_then(|v| v.as_str()) {
                if !q.is_empty() {
                    merged.insert("q".into(), Value::String(q.to_string()));
                }
            }
            let qs = build_query(Some(&Value::Object(merged)), &["q", "archived", "trashed"]);
            Ok((Method::GET, format!("/v1/jobs{qs}"), None))
        }
        "jobs:get" => match as_id(a0) {
            Some(id) => Ok((Method::GET, format!("/v1/jobs/{id}"), None)),
            None => Err(bad("Invalid job id")),
        },
        "jobs:update" => match (as_id(a0), as_object(a1)) {
            (Some(id), Some(b)) => Ok((Method::PUT, format!("/v1/jobs/{id}"), Some(b.clone()))),
            _ => Err(bad("Invalid job")),
        },
        "jobs:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/jobs/{id}"), None)),
            None => Err(bad("Invalid job id")),
        },
        "jobs:analyzeText" => match as_non_empty_str(a0) {
            Some(raw_text) => Ok((
                Method::POST,
                "/v1/jobs/analyze".into(),
                Some(json!({ "raw_text": raw_text })),
            )),
            None => Err(bad("No posting text.")),
        },
        "jobs:analyze" => match as_id(a0) {
            Some(id) => Ok((Method::POST, format!("/v1/jobs/{id}/analyze"), None)),
            None => Err(bad("Invalid job id")),
        },
        "jobs:archive" => match as_id(a0) {
            Some(id) => {
                let archive_app = a1
                    .and_then(|v| v.get("archive_application"))
                    .and_then(|v| v.as_bool())
                    .unwrap_or(false);
                Ok((
                    Method::POST,
                    format!("/v1/jobs/{id}/archive"),
                    Some(json!({ "archive_application": archive_app })),
                ))
            }
            None => Err(bad("Invalid job id")),
        },
        "jobs:unarchive" => match as_id(a0) {
            Some(id) => Ok((Method::POST, format!("/v1/jobs/{id}/unarchive"), None)),
            None => Err(bad("Invalid job id")),
        },
        "jobs:trash" => match as_id(a0) {
            Some(id) => Ok((Method::POST, format!("/v1/jobs/{id}/trash"), None)),
            None => Err(bad("Invalid job id")),
        },
        "jobs:restore" => match as_id(a0) {
            Some(id) => Ok((Method::POST, format!("/v1/jobs/{id}/restore"), None)),
            None => Err(bad("Invalid job id")),
        },

        "cost:analyze" => match as_object(a0) {
            Some(b) => Ok((Method::POST, "/v1/cost".into(), Some(b.clone()))),
            None => Err(bad("Invalid posting")),
        },
        "cost:compare" => match as_object(a0) {
            Some(b) => Ok((Method::POST, "/v1/cost/compare".into(), Some(b.clone()))),
            None => Err(bad("Invalid comparison")),
        },

        "applications:list" => {
            let qs = build_query(a0, &["status", "q", "archived"]);
            Ok((Method::GET, format!("/v1/applications{qs}"), None))
        }
        "applications:stats" => Ok((Method::GET, "/v1/applications/stats".into(), None)),
        "applications:create" => match as_object(a0).filter(|b| as_id(b.get("job_id")).is_some()) {
            Some(b) => Ok((Method::POST, "/v1/applications".into(), Some(b.clone()))),
            None => Err(bad("Choose a job first.")),
        },
        "applications:get" => match as_id(a0) {
            Some(id) => Ok((Method::GET, format!("/v1/applications/{id}"), None)),
            None => Err(bad("Invalid application id")),
        },
        "applications:update" => match (as_id(a0), as_object(a1)) {
            (Some(id), Some(p)) => Ok((
                Method::PUT,
                format!("/v1/applications/{id}"),
                Some(p.clone()),
            )),
            _ => Err(bad("Invalid application")),
        },
        "applications:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/applications/{id}"), None)),
            None => Err(bad("Invalid application id")),
        },
        "applications:bulkStatus" => match (as_id_list(a0), a1.and_then(|v| v.as_str())) {
            (Some(ids), Some(status)) => Ok((
                Method::POST,
                "/v1/applications/bulk-status".into(),
                Some(json!({ "ids": ids, "status": status })),
            )),
            _ => Err(bad("Choose 1 to 1000 applications and a status.")),
        },
        "applications:archive" => match as_id(a0) {
            Some(id) => Ok((
                Method::POST,
                format!("/v1/applications/{id}/archive"),
                None,
            )),
            None => Err(bad("Invalid application id")),
        },
        "applications:unarchive" => match as_id(a0) {
            Some(id) => Ok((
                Method::POST,
                format!("/v1/applications/{id}/unarchive"),
                None,
            )),
            None => Err(bad("Invalid application id")),
        },
        "applications:bulkArchive" => match (as_id_list(a0), a1.and_then(|v| v.as_bool())) {
            (Some(ids), Some(archived)) => Ok((
                Method::POST,
                "/v1/applications/bulk-archive".into(),
                Some(json!({ "ids": ids, "archived": archived })),
            )),
            _ => Err(bad("Choose 1 to 1000 applications.")),
        },

        "connectors:import" => {
            let valid = a0.and_then(|v| v.as_array()).filter(|arr| {
                !arr.is_empty()
                    && arr.iter().all(|p| {
                        p.is_object()
                            && p.get("board").and_then(|x| x.as_str()).is_some()
                            && p.get("external_id").and_then(|x| x.as_str()).is_some()
                    })
            });
            match valid {
                Some(postings) => Ok((
                    Method::POST,
                    "/v1/connectors/greenhouse/import".into(),
                    Some(json!({ "postings": postings })),
                )),
                None => Err(bad("Invalid posting")),
            }
        }

        "connectors:categories" => Ok((
            Method::GET,
            "/v1/connectors/greenhouse/categories".into(),
            None,
        )),
        "connectors:list" => Ok((Method::GET, "/v1/connectors".into(), None)),
        "storage:report" => Ok((Method::GET, "/v1/storage".into(), None)),

        "savedSearches:list" => Ok((Method::GET, "/v1/saved-searches".into(), None)),
        "savedSearches:create" => match as_object(a0) {
            Some(b) => Ok((Method::POST, "/v1/saved-searches".into(), Some(b.clone()))),
            None => Err(bad("Invalid saved search")),
        },
        "savedSearches:markSeen" => match (as_id(a0), a1.and_then(|v| v.as_array())) {
            (Some(id), Some(keys)) => Ok((
                Method::POST,
                format!("/v1/saved-searches/{id}/mark-seen"),
                Some(json!({ "seen_keys": keys })),
            )),
            _ => Err(bad("Invalid saved search")),
        },
        "savedSearches:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/saved-searches/{id}"), None)),
            None => Err(bad("Invalid saved search id")),
        },

        "drafts:list" => match as_id(a0) {
            Some(job_id) => Ok((Method::GET, format!("/v1/jobs/{job_id}/drafts"), None)),
            None => Err(bad("Invalid job id")),
        },
        "drafts:create" => match (as_id(a0), as_object(a1)) {
            (Some(job_id), Some(body)) => Ok((
                Method::POST,
                format!("/v1/jobs/{job_id}/drafts"),
                Some(body.clone()),
            )),
            _ => Err(bad("Invalid draft")),
        },
        "drafts:update" => match (as_id(a0), as_id(a1), as_object(a2)) {
            (Some(job_id), Some(did), Some(body)) => Ok((
                Method::PUT,
                format!("/v1/jobs/{job_id}/drafts/{did}"),
                Some(body.clone()),
            )),
            _ => Err(bad("Invalid draft")),
        },
        "drafts:delete" => match (as_id(a0), as_id(a1)) {
            (Some(job_id), Some(did)) => Ok((
                Method::DELETE,
                format!("/v1/jobs/{job_id}/drafts/{did}"),
                None,
            )),
            _ => Err(bad("Invalid draft id")),
        },

        "events:list" => {
            let qs = build_query(a0, &["from", "to"]);
            Ok((Method::GET, format!("/v1/events{qs}"), None))
        }
        "events:create" => match as_object(a0) {
            Some(body) => Ok((Method::POST, "/v1/events".into(), Some(body.clone()))),
            None => Err(bad("Invalid plan")),
        },
        "events:update" => match (as_id(a0), as_object(a1)) {
            (Some(id), Some(body)) => Ok((Method::PUT, format!("/v1/events/{id}"), Some(body.clone()))),
            _ => Err(bad("Invalid plan")),
        },
        "events:delete" => match as_id(a0) {
            Some(id) => Ok((Method::DELETE, format!("/v1/events/{id}"), None)),
            None => Err(bad("Invalid plan id")),
        },

        _ => return None,
    };

    Some(res)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_content_disposition_names_safely() {
        assert_eq!(
            suggested_name(Some("attachment; filename=\"Jane_Doe.pdf\""), "Resume.pdf"),
            "Jane_Doe.pdf"
        );
        assert_eq!(
            suggested_name(
                Some("attachment; filename*=UTF-8''Ren%C3%A9e%20CV.pdf"),
                "Resume.pdf"
            ),
            "Renée CV.pdf"
        );
        assert_eq!(
            suggested_name(
                Some("attachment; filename=\"../../folder/notes.pdf\""),
                "Resume.pdf"
            ),
            "notes.pdf"
        );
        assert_eq!(
            suggested_name(Some("attachment; filename=\"..\""), "Resume.pdf"),
            "Resume.pdf"
        );
    }

    #[test]
    fn validates_resume_import_files() {
        assert!(check_import("resume.pdf", 1024).is_none());
        assert!(check_import("resume.MD", 100).is_none());
        assert!(check_import("resume.docx", 100).is_none());
        assert!(check_import("resume.odt", 100).is_none());
        assert!(check_import("resume.png", 100).is_some());
        assert!(check_import("resume.pdf", 0).is_some());
        assert!(check_import("resume.pdf", MAX_IMPORT_BYTES + 1).is_some());
    }

    #[test]
    fn validates_route_ids_and_builds_queries() {
        assert!(matches!(
            route_for_channel("jobs:get", &[json!(42)]),
            Some(Ok((Method::GET, path, None))) if path == "/v1/jobs/42"
        ));
        assert!(matches!(
            route_for_channel("jobs:get", &[json!(-1)]),
            Some(Err(_))
        ));
        assert!(matches!(
            route_for_channel("jobs:get", &[json!("42/../other")]),
            Some(Err(_))
        ));
        assert_eq!(
            build_query(
                Some(&json!({ "status": "Applied", "q": "data analyst", "extra": "ignored" })),
                &["status", "q", "archived"]
            ),
            "?status=Applied&q=data%20analyst"
        );
    }

    #[test]
    fn validates_resume_and_tailored_routes() {
        assert!(matches!(
            route_for_channel("resume:importText", &[json!("   ")]),
            Some(Err(_))
        ));
        assert!(matches!(
            route_for_channel("resume:importText", &[json!("Jane Doe\nEngineer")]),
            Some(Ok((Method::POST, path, Some(_)))) if path == "/v1/resume/import"
        ));
        assert!(matches!(
            route_for_channel("resume:importApply", &[json!({ "replace": true })]),
            Some(Err(_))
        ));
        assert!(matches!(
            route_for_channel("tailored:createBlank", &[json!(5), json!(2)]),
            Some(Ok((Method::POST, path, Some(body))))
                if path == "/v1/tailored-resumes"
                    && body.get("job_id").and_then(|v| v.as_i64()) == Some(5)
                    && body.get("base_resume_id").and_then(|v| v.as_i64()) == Some(2)
        ));
        assert!(matches!(
            route_for_channel("tailored:update", &[json!(3), json!("")]),
            Some(Err(_))
        ));
    }
}
