#![forbid(unsafe_code)]

use anyhow::{Context, Result, bail};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::io::{Read, Write};

pub const UDS_PATH: &str = "/tmp/agent-ebpf.sock";
pub const MAX_UDS_PAYLOAD: usize = 4 << 20;

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ProcessContext {
    pub pid: u32,
    #[serde(default)]
    pub root_agent_pid: u32,
    #[serde(default)]
    pub agent_run_id: String,
    #[serde(default)]
    pub task_id: String,
    #[serde(default)]
    pub conversation_id: String,
    #[serde(default)]
    pub turn_id: String,
    #[serde(default)]
    pub tool_call_id: String,
    #[serde(default)]
    pub tool_name: String,
    #[serde(default)]
    pub trace_id: String,
    #[serde(default)]
    pub span_id: String,
    #[serde(default)]
    pub decision: String,
    #[serde(default)]
    pub risk_score: f64,
    #[serde(default)]
    pub container_id: String,
    #[serde(default)]
    pub cwd: String,
    #[serde(default)]
    pub argv_digest: String,
}

pub fn first_env(keys: &[&str]) -> String {
    keys.iter()
        .filter_map(|key| std::env::var(key).ok())
        .map(|value| value.trim().to_owned())
        .find(|value| !value.is_empty())
        .unwrap_or_default()
}

pub fn env_u32(keys: &[&str]) -> u32 {
    first_env(keys).parse().unwrap_or_default()
}

pub fn env_f64(keys: &[&str]) -> f64 {
    first_env(keys).parse().unwrap_or_default()
}

pub fn argv_digest<'a>(parts: impl IntoIterator<Item = &'a str>) -> String {
    let normalized: Vec<_> = parts
        .into_iter()
        .map(str::trim)
        .filter(|p| !p.is_empty())
        .collect();
    if normalized.is_empty() {
        return String::new();
    }
    let mut hasher = Sha256::new();
    hasher.update(normalized.join("\0"));
    hex::encode(hasher.finalize())
}

pub fn write_frame(mut writer: impl Write, payload: &[u8]) -> Result<()> {
    if payload.is_empty() || payload.len() > MAX_UDS_PAYLOAD {
        bail!("invalid UDS payload size: {}", payload.len());
    }
    writer
        .write_all(&(payload.len() as u32).to_be_bytes())
        .context("write UDS header")?;
    writer.write_all(payload).context("write UDS payload")?;
    Ok(())
}

pub fn read_frame(mut reader: impl Read) -> Result<Vec<u8>> {
    let mut header = [0_u8; 4];
    reader.read_exact(&mut header).context("read UDS header")?;
    let size = u32::from_be_bytes(header) as usize;
    if size == 0 || size > MAX_UDS_PAYLOAD {
        bail!("invalid UDS payload size: {size}");
    }
    let mut payload = vec![0_u8; size];
    reader
        .read_exact(&mut payload)
        .context("read UDS payload")?;
    Ok(payload)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn framing_matches_existing_big_endian_protocol() {
        let mut wire = Vec::new();
        write_frame(&mut wire, b"hello").unwrap();
        assert_eq!(&wire[..4], &[0, 0, 0, 5]);
        assert_eq!(read_frame(wire.as_slice()).unwrap(), b"hello");
    }
}
