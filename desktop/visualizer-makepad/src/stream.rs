//! Unprivileged consumer of backend compact summaries; no privileged eBPF access.
use crate::model::{FeedBatch, Summary};
use std::sync::mpsc::{self, Receiver, SyncSender, TrySendError};
use std::thread;
use std::time::Duration;
use tungstenite::client::IntoClientRequest;
use tungstenite::http::HeaderValue;

pub enum FeedMessage {
    Connected(bool),
    Event(Summary),
}

/// Blocking read runs on its own OS thread, never on the GPU/UI thread.
/// Bounded queues are intentionally lossy to protect the renderer under bursts.
pub fn subscribe() -> Receiver<FeedMessage> {
    let (tx, rx) = mpsc::sync_channel(512);
    thread::spawn(move || {
        let url = std::env::var("AGENT_EBPF_WS_URL")
            .unwrap_or_else(|_| "ws://127.0.0.1:8080/ws/event-summaries".into());
        let token = std::env::var("AGENT_EBPF_API_TOKEN").unwrap_or_default();
        loop {
            if let Err(err) = consume(&tx, &url, &token) {
                eprintln!("codecity: event feed disconnected: {err}");
            }
            if matches!(tx.try_send(FeedMessage::Connected(false)),
                Err(TrySendError::Disconnected(_))) { break; }
            thread::sleep(Duration::from_secs(3));
        }
    });
    rx
}

fn consume(tx: &SyncSender<FeedMessage>, url: &str, token: &str) -> Result<(), String> {
    if !url.starts_with("ws://127.0.0.1:") && !url.starts_with("ws://localhost:")
        && !url.starts_with("wss://") {
        return Err("cleartext remote WS is forbidden; use WSS".into());
    }
    let mut req = url.into_client_request().map_err(|_| "invalid WS URL")?;
    if !token.is_empty() {
        let value = HeaderValue::from_str(token).map_err(|_| "invalid API token")?;
        req.headers_mut().insert("X-API-KEY", value);
    }
    let (mut socket, _) = tungstenite::connect(req).map_err(|e| format!("{e}"))?;
    if tx.send(FeedMessage::Connected(true)).is_err() { return Ok(()); }
    loop {
        let msg = socket.read().map_err(|e| format!("{e}"))?;
        if !msg.is_text() { continue; }
        let batch: FeedBatch = match serde_json::from_slice(msg.as_bytes()) {
            Ok(batch) => batch,
            Err(_) => continue,
        };
        for event in batch.events.into_iter().take(2048) {
            match tx.try_send(FeedMessage::Event(event)) {
                Ok(()) => {}
                Err(TrySendError::Full(_)) => {} // Drop on pressure, keep UI responsive.
                Err(TrySendError::Disconnected(_)) => return Ok(()),
            }
        }
    }
}
