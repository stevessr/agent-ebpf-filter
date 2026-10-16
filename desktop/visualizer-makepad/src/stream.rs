//! Unprivileged, bounded consumer of backend compact summaries.
//! The producer may drop events when the GPU is busy. Health and drop counts
//! are independent atomics so a full event queue cannot mask a disconnection.
use crate::model::{FeedBatch, Summary};
use std::sync::{
    atomic::{AtomicBool, AtomicU64, Ordering},
    mpsc::{self, Receiver, SyncSender, TrySendError},
    Arc,
};
use std::{thread, time::Duration};
use tungstenite::client::IntoClientRequest;
use tungstenite::http::HeaderValue;
use url::Url;

pub struct EventFeed {
    pub events: Receiver<Summary>,
    pub online: Arc<AtomicBool>,
    pub dropped: Arc<AtomicU64>,
}

/// Validate a network location before sending any tokens to it.
/// In particular, string-prefix matching of ws://127.0.0.1: is unsafe because
/// ws://127.0.0.1:8080@evil.example is a valid attacker-controlled authority.
pub fn validate_endpoint(raw: &str) -> Result<Url, &'static str> {
    let url = Url::parse(raw).map_err(|_| "invalid WebSocket endpoint")?;
    if !url.username().is_empty() || url.password().is_some() || url.fragment().is_some() {
        return Err("credentials and fragments are forbidden in endpoint URLs");
    }
    let host = url.host_str().ok_or("endpoint host is required")?;
    match url.scheme() {
        "wss" => {}
        "ws" if matches!(host, "localhost" | "127.0.0.1" | "::1") => {}
        "ws" => return Err("remote plaintext WebSocket is forbidden; use wss://"),
        _ => return Err("endpoint must start with ws:// or wss://"),
    }
    if url.path() != "/ws/event-summaries" {
        return Err("only the read-only compact event stream is supported");
    }
    if url.query().is_some() {
        // Keep API tokens out of command lines, URLs, reverse proxy logs and
        // browser histories. Use X-API-KEY on the authenticated handshake.
        return Err("query parameters are disabled; pass credentials via environment");
    }
    Ok(url)
}

/// The blocking network loop always runs off the rendering/UI thread.
/// A finite channel limits memory; drops and connection health remain visible.
pub fn subscribe() -> EventFeed {
    let (tx, rx) = mpsc::sync_channel(512);
    let online = Arc::new(AtomicBool::new(false));
    let dropped = Arc::new(AtomicU64::new(0));
    let online_worker = online.clone();
    let dropped_worker = dropped.clone();
    thread::spawn(move || {
        let raw = std::env::var("AGENT_EBPF_WS_URL")
            .unwrap_or_else(|_| "ws://127.0.0.1:8080/ws/event-summaries".into());
        let url = match validate_endpoint(&raw) {
            Ok(url) => url,
            Err(err) => { eprintln!("codecity: invalid event endpoint: {err}"); return; }
        };
        let token = std::env::var("AGENT_EBPF_API_TOKEN").unwrap_or_default();
        loop {
            let result = consume(&tx, &url, &token, &online_worker, &dropped_worker);
            online_worker.store(false, Ordering::Release);
            if result.is_err() {
                // Never log potentially sensitive request URLs or handshake details.
                eprintln!("codecity: compact event feed offline; reconnecting");
            }
            if matches!(result, Err("receiver closed")) { return; }
            thread::sleep(Duration::from_secs(3));
        }
    });
    EventFeed { events: rx, online, dropped }
}

fn consume(tx: &SyncSender<Summary>, url: &Url, token: &str,
    online: &AtomicBool, dropped: &AtomicU64) -> Result<(), &'static str> {
    let mut req = url.as_str().into_client_request().map_err(|_| "invalid request")?;
    if !token.is_empty() {
        let value = HeaderValue::from_str(token).map_err(|_| "invalid API token")?;
        req.headers_mut().insert("X-API-KEY", value);
    }
    let (mut socket, _) = tungstenite::connect(req)
        .map_err(|_| "could not connect to event stream")?;
    online.store(true, Ordering::Release);
    loop {
        let msg = socket.read().map_err(|_| "event feed closed")?;
        if !msg.is_text() { continue; }
        let text = msg.to_text().map_err(|_| "invalid text frame")?;
        if text.len() > 2 * 1024 * 1024 {
            dropped.fetch_add(1, Ordering::Relaxed);
            continue;
        }
        let batch: FeedBatch = match serde_json::from_str(text) {
            Ok(batch) => batch,
            Err(_) => continue,
        };
        for event in batch.events.into_iter().take(2048) {
            match tx.try_send(event) {
                Ok(()) => {}
                Err(TrySendError::Full(_)) => { dropped.fetch_add(1, Ordering::Relaxed); }
                Err(TrySendError::Disconnected(_)) => return Err("receiver closed"),
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn websocket_authority_checks_are_strict() {
        for valid in [
            "ws://127.0.0.1:8080/ws/event-summaries",
            "ws://localhost:8080/ws/event-summaries",
            "wss://my-codespace.example/ws/event-summaries",
        ] {
            assert!(validate_endpoint(valid).is_ok(), "rejected {valid}");
        }
        for invalid in [
            "ws://127.0.0.1:8080@evil.example/ws/event-summaries",
            "ws://127.0.0.1.evil.example/ws/event-summaries",
            "ws://my-codespace.example/ws/event-summaries",
            "wss://user:pass@example.com/ws/event-summaries",
            "wss://example.com/ws/event-summaries?key=secret",
            "wss://example.com/ws/envelopes",
            "https://example.com/ws/event-summaries",
        ] {
            assert!(validate_endpoint(invalid).is_err(), "accepted {invalid}");
        }
    }
}
