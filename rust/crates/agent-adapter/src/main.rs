#![forbid(unsafe_code)]

use agent_common::{argv_digest, env_f64, env_u32, first_env, ProcessContext};
use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use reqwest::Client;

#[derive(Parser)]
#[command(name = "agent-adapter")]
struct Cli {
    #[arg(long, env = "AGENT_BACKEND_URL", default_value = "http://127.0.0.1:8080")]
    backend: String,
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    Register,
    Unregister,
}

fn context_from_env() -> ProcessContext {
    let tool_name = first_env(&["AGENT_EBPF_TOOL_NAME", "AGENT_TOOL_NAME"]);
    let tool_call_id = first_env(&["AGENT_EBPF_TOOL_CALL_ID", "AGENT_TOOL_CALL_ID"]);
    let agent_run_id = first_env(&["AGENT_EBPF_AGENT_RUN_ID", "AGENT_RUN_ID"]);
    ProcessContext {
        pid: std::process::id(),
        root_agent_pid: env_u32(&["AGENT_EBPF_ROOT_AGENT_PID", "ROOT_AGENT_PID"]),
        agent_run_id: agent_run_id.clone(),
        task_id: first_env(&["AGENT_EBPF_TASK_ID", "AGENT_TASK_ID"]),
        conversation_id: first_env(&["AGENT_EBPF_CONVERSATION_ID", "AGENT_CONVERSATION_ID"]),
        turn_id: first_env(&["AGENT_EBPF_TURN_ID", "AGENT_TURN_ID"]),
        tool_call_id: tool_call_id.clone(),
        tool_name: tool_name.clone(),
        trace_id: first_env(&["AGENT_EBPF_TRACE_ID", "TRACE_ID"]),
        span_id: first_env(&["AGENT_EBPF_SPAN_ID", "SPAN_ID"]),
        decision: first_env(&["AGENT_EBPF_DECISION", "AGENT_DECISION"]),
        risk_score: env_f64(&["AGENT_EBPF_RISK_SCORE", "AGENT_RISK_SCORE"]),
        container_id: first_env(&["AGENT_EBPF_CONTAINER_ID", "CONTAINER_ID"]),
        cwd: first_env(&["AGENT_EBPF_CWD", "PWD"]),
        argv_digest: argv_digest([tool_name.as_str(), tool_call_id.as_str(), agent_run_id.as_str()]),
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();
    let client = Client::new();
    let token = first_env(&["AGENT_API_KEY", "AGENT_EBPF_ACCESS_TOKEN"]);
    let endpoint = |path: &str| format!("{}{}", cli.backend.trim_end_matches('/'), path);

    match cli.command {
        Command::Register => {
            let mut request = client.post(endpoint("/register")).json(&context_from_env());
            if !token.is_empty() {
                request = request.header("X-API-KEY", &token).bearer_auth(&token);
            }
            let response = request.send().await.context("register PID")?;
            anyhow::ensure!(response.status().is_success(), "register failed: {}", response.status());
        }
        Command::Unregister => {
            let mut request = client
                .post(endpoint("/unregister"))
                .json(&serde_json::json!({ "pid": std::process::id() }));
            if !token.is_empty() {
                request = request.header("X-API-KEY", &token).bearer_auth(&token);
            }
            let response = request.send().await.context("unregister PID")?;
            anyhow::ensure!(response.status().is_success(), "unregister failed: {}", response.status());
        }
    }
    Ok(())
}
