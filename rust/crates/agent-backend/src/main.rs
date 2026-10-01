#![forbid(unsafe_code)]

use std::{
    collections::VecDeque,
    os::unix::fs::PermissionsExt,
    path::{Path, PathBuf},
    sync::{Arc, Mutex},
    time::{SystemTime, UNIX_EPOCH},
};

use agent_common::{ProcessContext, UDS_PATH, read_frame, write_frame};
use agent_proto::pb::{
    CapturedEventRecord, Event, EventBatch, EventHistoryResponse, WrapperRequest, WrapperResponse,
    wrapper_response,
};
use anyhow::{Context, Result};
use axum::{
    Json, Router,
    extract::{
        State,
        ws::{Message as WsMessage, WebSocket, WebSocketUpgrade},
    },
    http::{StatusCode, header},
    response::{IntoResponse, Response},
    routing::{get, post},
};
use dashmap::DashMap;
use prost::Message;
use serde::{Deserialize, Serialize};
use tokio::{net::UnixListener, sync::broadcast, task};
use tower_http::services::{ServeDir, ServeFile};

const MAX_RECENT_EVENTS: usize = 1_500;
const EVENT_BROADCAST_CAPACITY: usize = 1_024;

#[derive(Clone)]
struct AppState {
    processes: Arc<DashMap<u32, ProcessContext>>,
    recent_events: Arc<Mutex<VecDeque<CapturedEventRecord>>>,
    event_tx: broadcast::Sender<Event>,
}

impl AppState {
    fn new() -> Self {
        let (event_tx, _) = broadcast::channel(EVENT_BROADCAST_CAPACITY);
        Self {
            processes: Arc::new(DashMap::new()),
            recent_events: Arc::new(Mutex::new(VecDeque::with_capacity(MAX_RECENT_EVENTS))),
            event_tx,
        }
    }

    fn publish_event(&self, event: Event) {
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis()
            .min(i64::MAX as u128) as i64;

        {
            let mut recent = self
                .recent_events
                .lock()
                .expect("recent event mutex poisoned");
            recent.push_back(CapturedEventRecord {
                event: Some(event.clone()),
                timestamp,
                envelope: None,
            });
            while recent.len() > MAX_RECENT_EVENTS {
                recent.pop_front();
            }
        }

        let _ = self.event_tx.send(event);
    }
}

#[derive(Deserialize)]
struct Unregister {
    pid: u32,
}

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
    backend: &'static str,
    registered_processes: usize,
    retained_events: usize,
    policy_engine: &'static str,
}

async fn health(State(state): State<AppState>) -> Json<HealthResponse> {
    let retained_events = state
        .recent_events
        .lock()
        .expect("recent event mutex poisoned")
        .len();
    Json(HealthResponse {
        status: "ok",
        backend: "rust",
        registered_processes: state.processes.len(),
        retained_events,
        policy_engine: "migration-bootstrap",
    })
}

async fn register(
    State(state): State<AppState>,
    Json(context): Json<ProcessContext>,
) -> (StatusCode, Json<serde_json::Value>) {
    state.processes.insert(context.pid, context);
    (StatusCode::OK, Json(serde_json::json!({"success": true})))
}

async fn unregister(
    State(state): State<AppState>,
    Json(req): Json<Unregister>,
) -> (StatusCode, Json<serde_json::Value>) {
    state.processes.remove(&req.pid);
    (StatusCode::OK, Json(serde_json::json!({"success": true})))
}

async fn recent_events(State(state): State<AppState>) -> Response {
    let records = state
        .recent_events
        .lock()
        .expect("recent event mutex poisoned")
        .iter()
        .cloned()
        .collect();

    let response = EventHistoryResponse {
        events: records,
        source: "rust-memory".into(),
    };
    (
        [(header::CONTENT_TYPE, "application/x-protobuf")],
        response.encode_to_vec(),
    )
        .into_response()
}

async fn events_ws(State(state): State<AppState>, ws: WebSocketUpgrade) -> Response {
    ws.on_upgrade(move |socket| stream_events(socket, state))
}

async fn stream_events(mut socket: WebSocket, state: AppState) {
    let mut rx = state.event_tx.subscribe();
    loop {
        match rx.recv().await {
            Ok(event) => {
                let payload = EventBatch {
                    events: vec![event],
                }
                .encode_to_vec();
                if socket
                    .send(WsMessage::Binary(payload.into()))
                    .await
                    .is_err()
                {
                    break;
                }
            }
            Err(broadcast::error::RecvError::Lagged(_)) => continue,
            Err(broadcast::error::RecvError::Closed) => break,
        }
    }
}

async fn run_uds(state: AppState) -> Result<()> {
    if Path::new(UDS_PATH).exists() {
        std::fs::remove_file(UDS_PATH).context("remove stale wrapper UDS")?;
    }

    let listener = UnixListener::bind(UDS_PATH).context("bind wrapper UDS")?;
    std::fs::set_permissions(UDS_PATH, std::fs::Permissions::from_mode(0o600))
        .context("restrict wrapper UDS permissions")?;

    loop {
        let (stream, _) = listener.accept().await?;
        let state = state.clone();
        let _connection_task = task::spawn_blocking(move || -> Result<()> {
            let stream = stream.into_std()?;
            stream.set_nonblocking(false)?;
            let frame = read_frame(&stream)?;
            let req = WrapperRequest::decode(frame.as_slice()).context("decode wrapper request")?;

            state.processes.insert(
                req.pid,
                ProcessContext {
                    pid: req.pid,
                    root_agent_pid: req.root_agent_pid,
                    agent_run_id: req.agent_run_id.clone(),
                    task_id: req.task_id.clone(),
                    conversation_id: req.conversation_id.clone(),
                    turn_id: req.turn_id.clone(),
                    tool_call_id: req.tool_call_id.clone(),
                    tool_name: req.tool_name.clone(),
                    trace_id: req.trace_id.clone(),
                    span_id: req.span_id.clone(),
                    decision: req.decision.clone(),
                    risk_score: req.risk_score,
                    container_id: req.container_id.clone(),
                    cwd: req.cwd.clone(),
                    argv_digest: req.argv_digest.clone(),
                },
            );

            let effective_decision = if req.decision.is_empty() {
                "ALERT".to_owned()
            } else {
                req.decision.clone()
            };
            state.publish_event(Event {
                pid: req.pid,
                tgid: req.pid,
                r#type: "wrapper_intercept".into(),
                comm: req.comm.clone(),
                path: req.binary_path.clone(),
                agent_run_id: req.agent_run_id.clone(),
                conversation_id: req.conversation_id.clone(),
                turn_id: req.turn_id.clone(),
                tool_call_id: req.tool_call_id.clone(),
                tool_name: req.tool_name.clone(),
                trace_id: req.trace_id.clone(),
                span_id: req.span_id.clone(),
                root_agent_pid: req.root_agent_pid,
                decision: effective_decision,
                risk_score: req.risk_score,
                container_id: req.container_id.clone(),
                argv_digest: req.argv_digest.clone(),
                task_id: req.task_id.clone(),
                cwd: req.cwd.clone(),
                ..Default::default()
            });

            let response = WrapperResponse {
                action: wrapper_response::Action::Alert as i32,
                message:
                    "Rust backend bootstrap: policy engine is not ported yet; execution continues"
                        .into(),
                rewritten_args: Vec::new(),
                classification: None,
                ml_score: 0.0,
                anomaly_score: 0.0,
                ml_action: String::new(),
                ml_reasoning: String::new(),
            };
            write_frame(&stream, &response.encode_to_vec())?;
            Ok(())
        });
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    let state = AppState::new();
    let uds_state = state.clone();
    let _uds_task = tokio::spawn(async move {
        if let Err(error) = run_uds(uds_state).await {
            tracing::error!(%error, "wrapper UDS server stopped");
        }
    });

    let static_dir = [
        PathBuf::from("webui/dist"),
        PathBuf::from("../webui/dist"),
        PathBuf::from("frontend/dist"),
        PathBuf::from("../frontend/dist"),
    ]
    .into_iter()
    .find(|path| path.join("index.html").is_file());

    let mut app = Router::new()
        .route("/health", get(health))
        .route("/api/v1/health", get(health))
        .route("/register", post(register))
        .route("/unregister", post(unregister))
        .route("/events/recent", get(recent_events))
        .route("/ws", get(events_ws))
        .with_state(state);

    if let Some(static_dir) = static_dir {
        let index = static_dir.join("index.html");
        app = app
            .fallback_service(ServeDir::new(static_dir).not_found_service(ServeFile::new(index)));
    }

    let addr = std::env::var("AGENT_BACKEND_ADDR").unwrap_or_else(|_| "127.0.0.1:8080".into());
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    tracing::info!(%addr, "Rust compatibility backend listening");
    axum::serve(listener, app).await?;
    Ok(())
}
