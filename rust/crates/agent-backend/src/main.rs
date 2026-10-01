#![forbid(unsafe_code)]

use agent_common::{ProcessContext, UDS_PATH, read_frame, write_frame};
use agent_proto::pb::{WrapperRequest, WrapperResponse, wrapper_response};
use anyhow::{Context, Result};
use axum::{
    Json, Router,
    extract::State,
    http::StatusCode,
    routing::{get, post},
};
use dashmap::DashMap;
use prost::Message;
use serde::{Deserialize, Serialize};
use std::{
    os::unix::fs::PermissionsExt,
    path::{Path, PathBuf},
    sync::Arc,
};
use tokio::{net::UnixListener, task};
use tower_http::services::{ServeDir, ServeFile};

#[derive(Clone, Default)]
struct AppState {
    processes: Arc<DashMap<u32, ProcessContext>>,
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
    policy_engine: &'static str,
}

async fn health(State(state): State<AppState>) -> Json<HealthResponse> {
    Json(HealthResponse {
        status: "ok",
        backend: "rust",
        registered_processes: state.processes.len(),
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
                    agent_run_id: req.agent_run_id,
                    task_id: req.task_id,
                    conversation_id: req.conversation_id,
                    turn_id: req.turn_id,
                    tool_call_id: req.tool_call_id,
                    tool_name: req.tool_name,
                    trace_id: req.trace_id,
                    span_id: req.span_id,
                    decision: req.decision,
                    risk_score: req.risk_score,
                    container_id: req.container_id,
                    cwd: req.cwd,
                    argv_digest: req.argv_digest,
                },
            );

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

    let state = AppState::default();
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
