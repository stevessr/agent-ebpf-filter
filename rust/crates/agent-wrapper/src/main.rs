use agent_common::{UDS_PATH, argv_digest, env_f64, env_u32, first_env, read_frame, write_frame};
use agent_proto::pb::{WrapperRequest, WrapperResponse, wrapper_response};
use anyhow::{Context, Result};
use clap::Parser;
use nix::unistd::{User, initgroups, setgid, setuid};
use prost::Message;
use std::{
    ffi::CString,
    os::unix::{net::UnixStream, process::CommandExt},
    path::{Path, PathBuf},
    process::Command,
    time::Duration,
};

#[derive(Parser)]
#[command(name = "agent-wrapper", trailing_var_arg = true)]
struct Cli {
    #[arg(long)]
    user: Option<String>,
    #[arg(long)]
    cwd: Option<PathBuf>,
    #[arg(long)]
    observer: bool,
    #[arg(required = true)]
    command: Vec<String>,
}

fn resolve_binary(command: &str) -> String {
    if command.contains('/') {
        return command.to_owned();
    }
    std::env::var_os("PATH")
        .and_then(|paths| {
            std::env::split_paths(&paths)
                .map(|dir| dir.join(command))
                .find(|path| path.is_file())
        })
        .map(|path| path.to_string_lossy().into_owned())
        .unwrap_or_default()
}

fn exchange(req: &WrapperRequest) -> Result<WrapperResponse> {
    let stream = UnixStream::connect(UDS_PATH).context("connect backend UDS")?;
    stream.set_read_timeout(Some(Duration::from_secs(2)))?;
    stream.set_write_timeout(Some(Duration::from_secs(2)))?;
    write_frame(&stream, &req.encode_to_vec())?;
    let payload = read_frame(&stream)?;
    WrapperResponse::decode(payload.as_slice()).context("decode wrapper response")
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    let mut command = cli.command[0].trim().to_owned();
    let mut args: Vec<String> = cli.command[1..]
        .iter()
        .map(|arg| arg.trim())
        .filter(|arg| !arg.is_empty())
        .map(ToOwned::to_owned)
        .collect();

    let cwd = cli
        .cwd
        .clone()
        .or_else(|| std::env::current_dir().ok())
        .unwrap_or_else(|| PathBuf::from("."));
    let binary_path = resolve_binary(&command);

    let request = WrapperRequest {
        pid: std::process::id(),
        comm: command.clone(),
        args: args.clone(),
        user: cli.user.clone().unwrap_or_else(|| first_env(&["USER"])),
        agent_run_id: first_env(&["AGENT_EBPF_AGENT_RUN_ID", "AGENT_RUN_ID"]),
        conversation_id: first_env(&["AGENT_EBPF_CONVERSATION_ID", "AGENT_CONVERSATION_ID"]),
        turn_id: first_env(&["AGENT_EBPF_TURN_ID", "AGENT_TURN_ID"]),
        tool_call_id: first_env(&["AGENT_EBPF_TOOL_CALL_ID", "AGENT_TOOL_CALL_ID"]),
        tool_name: first_env(&["AGENT_EBPF_TOOL_NAME", "AGENT_TOOL_NAME"]),
        trace_id: first_env(&["AGENT_EBPF_TRACE_ID", "TRACE_ID"]),
        span_id: first_env(&["AGENT_EBPF_SPAN_ID", "SPAN_ID"]),
        root_agent_pid: env_u32(&["AGENT_EBPF_ROOT_AGENT_PID", "ROOT_AGENT_PID"]),
        decision: first_env(&["AGENT_EBPF_DECISION", "AGENT_DECISION"]).to_uppercase(),
        risk_score: env_f64(&["AGENT_EBPF_RISK_SCORE", "AGENT_RISK_SCORE"]),
        container_id: first_env(&["AGENT_EBPF_CONTAINER_ID", "CONTAINER_ID"]),
        argv_digest: argv_digest(
            std::iter::once(command.as_str()).chain(args.iter().map(String::as_str)),
        ),
        task_id: first_env(&["AGENT_EBPF_TASK_ID", "AGENT_TASK_ID"]),
        cwd: cwd.to_string_lossy().into_owned(),
        binary_path,
        observer: cli.observer,
    };

    if let Ok(response) = exchange(&request) {
        match wrapper_response::Action::try_from(response.action)
            .unwrap_or(wrapper_response::Action::Allow)
        {
            wrapper_response::Action::Block => {
                eprintln!("Execution blocked: {}", response.message);
                std::process::exit(1);
            }
            wrapper_response::Action::Alert => eprintln!("Security alert: {}", response.message),
            wrapper_response::Action::Rewrite if !response.rewritten_args.is_empty() => {
                command = response.rewritten_args[0].clone();
                args = response.rewritten_args[1..].to_vec();
            }
            _ => {}
        }
    }

    if let Some(user_name) = cli.user.as_deref() {
        let user =
            User::from_name(user_name)?.with_context(|| format!("unknown user {user_name}"))?;
        let c_user = CString::new(user.name.clone())?;
        initgroups(&c_user, user.gid)?;
        setgid(user.gid)?;
        setuid(user.uid)?;
    }

    let mut child = Command::new(&command);
    child.args(&args).current_dir(Path::new(&cwd));
    let error = child.exec();
    Err(error).context("exec command")
}
