//! A bounded, read-only projection of *observed* CodeSpace paths; never reads file contents.
use serde::Deserialize;
use std::collections::HashMap;
use std::path::{Component, Path, PathBuf};

pub const MAX_BUILDINGS: usize = 2048;
pub const MAX_EVENTS_PER_FRAME: usize = 128;

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Summary {
    #[serde(default)]
    pub event_id: String,
    #[serde(default)]
    pub event_type: i32,
    #[serde(default, rename = "type")]
    pub kind: String,
    #[serde(default)]
    pub target: String,
    #[serde(default)]
    pub network: bool,
    #[serde(default)]
    pub decision: String,
    #[serde(default)]
    pub tag: String,
    #[serde(default)]
    pub comm: String,
    #[serde(default)]
    pub agent_run_id: String,
    #[serde(default)]
    pub root_agent_pid: i32,
    #[serde(default)]
    pub received_at_ms: i64,
}

#[derive(Debug, Deserialize)]
pub struct FeedBatch {
    #[serde(default)]
    pub events: Vec<Summary>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Activity { Read, Write, Blocked, Alert, Access }

impl Summary {
    pub fn classify(&self) -> Option<Activity> {
        if self.network { return None; }
        let name = self.kind.trim().to_ascii_lowercase();
        let name = name.strip_prefix("file_").unwrap_or(&name);
        let activity = match name {
            "write" | "writev" | "pwrite" | "pwrite64" | "pwritev" | "pwritev2" |
            "rename" | "renameat" | "renameat2" | "unlink" | "unlinkat" |
            "truncate" | "ftruncate" | "creat" | "mkdir" | "mkdirat" |
            "rmdir" | "chmod" | "fchmod" | "fchmodat" | "chown" | "fchown" |
            "fchownat" | "lchown" | "link" | "linkat" | "symlink" | "symlinkat" |
            "mknod" | "mknodat" | "setxattr" | "removexattr" => Activity::Write,
            "read" | "readv" | "pread" | "pread64" | "preadv" | "preadv2" => Activity::Read,
            "open" | "openat" | "openat2" | "stat" | "lstat" | "fstat" |
            "access" | "faccessat" => Activity::Access,
            _ => return None, // Do not turn arbitrary tool/HTTP events into file writes.
        };
        match self.decision.trim().to_ascii_uppercase().as_str() {
            "BLOCK" | "DENY" | "BLOCKED" | "DENIED" => Some(Activity::Blocked),
            "ALERT" => Some(Activity::Alert),
            _ => Some(activity),
        }
    }

    pub fn known_agent(&self) -> bool {
        self.root_agent_pid > 0 || !self.agent_run_id.is_empty() || matches!(
            self.tag.to_ascii_lowercase().as_str(),
            "codex" | "claude" | "claude-code" | "gemini" | "gemini-cli" |
            "cursor" | "cursor-agent" | "opencode" | "copilot" | "kiro" |
            "deepseek-harness" | "pi" | "omp" | "antigravity"
        )
    }
}

/// Reject lexical traversal and non-file placeholders. Default scope is /workspaces/<repo>.
/// An explicit root is needed to view other local paths. Never inspect paths outside it.
pub fn workspace_path(target: &str, root: Option<&Path>) -> Option<String> {
    let path = Path::new(target.trim());
    if !path.is_absolute() || path.components().any(|c| matches!(c, Component::ParentDir)) {
        return None;
    }
    let relative = if let Some(root) = root {
        path.strip_prefix(root).ok()?
    } else {
        let prefix = Path::new("/workspaces");
        let rest = path.strip_prefix(prefix).ok()?;
        let mut parts = rest.components();
        let repo = parts.next()?;
        if !matches!(repo, Component::Normal(_)) { return None; }
        let relative = parts.as_path();
        if relative.as_os_str().is_empty() { return None; }
        relative
    };
    let components = relative.components()
        .filter_map(|c| match c { Component::Normal(s) => s.to_str(), _ => None })
        .collect::<Vec<_>>();
    if components.is_empty() || components.iter().any(|s| *s == ".git") { return None; }
    let path = components.join("/");
    if path.len() > 360 || path.chars().any(|c| c.is_control()) { return None; }
    Some(path)
}

pub fn workspace_root() -> Option<PathBuf> {
    let raw = std::env::var_os("AGENT_EBPF_WORKSPACE_ROOT")?;
    let root = PathBuf::from(raw);
    // Explicit user-supplied root must exist; we never follow a supplied root
    // through a file event target or independently read source contents.
    root.canonicalize().ok().filter(|p| p.is_dir())
}

fn hash_path(path: &str) -> u32 {
    path.bytes().fold(2166136261_u32, |h, b| (h ^ u32::from(b)).wrapping_mul(16777619))
}

#[derive(Clone)]
pub struct Building {
    pub path: String,
    pub x: f32,
    pub z: f32,
    pub height: f32,
    pub pulse: f32,
    pub activity: Activity,
    pub owned_by_agent: bool,
}

#[derive(Default)]
pub struct City {
    pub buildings: Vec<Building>,
    lookup: HashMap<String, usize>,
    district_lookup: HashMap<String, usize>,
    district_counts: Vec<usize>,
    pub total_events: u64,
    pub agent_events: u64,
    pub dropped_events: u64,
    pub connected: bool,
}

impl City {
    pub fn seed(&mut self, path: String) {
        if self.lookup.contains_key(&path) || self.buildings.len() >= MAX_BUILDINGS { return; }
        let district = path.split('/').next().unwrap_or("root").to_owned();
        let district_id = match self.district_lookup.get(&district) {
            Some(id) => *id,
            None => {
                let id = self.district_counts.len();
                self.district_counts.push(0);
                self.district_lookup.insert(district, id);
                id
            }
        };
        let local_id = self.district_counts[district_id];
        self.district_counts[district_id] += 1;
        // Each directory forms its own 12x12 block. Coordinates are stable
        // within a running snapshot and do not depend on file contents.
        let dx = (district_id % 4) as f32 * 16.0 - 24.0;
        let dz = (district_id / 4) as f32 * 16.0 - 18.0;
        let x = dx + (local_id % 12) as f32 * 1.18;
        let z = dz + ((local_id / 12) % 12) as f32 * 1.18;
        let height = 0.25 + (hash_path(&path) % 350) as f32 / 125.0;
        self.lookup.insert(path.clone(), self.buildings.len());
        self.buildings.push(Building { path, x, z, height, pulse: 0.0,
            activity: Activity::Access, owned_by_agent: false });
    }

    /// Optional local metadata-only index for a populated city before the
    /// first syscall. Never follows symlinks or opens source file contents.
    pub fn seed_workspace(&mut self, root: &Path) {
        let mut dirs = vec![(root.to_path_buf(), 0usize)];
        let mut inspected = 0usize;
        while let Some((dir, depth)) = dirs.pop() {
            if depth > 7 || inspected >= 30000 || self.buildings.len() >= MAX_BUILDINGS { break; }
            let Ok(entries) = std::fs::read_dir(dir) else { continue };
            for entry in entries.flatten() {
                if inspected >= 30000 || self.buildings.len() >= MAX_BUILDINGS { return; }
                inspected += 1;
                let name = entry.file_name();
                let name = name.to_string_lossy();
                if name.starts_with('.') || matches!(name.as_ref(),
                    "node_modules" | "target" | "dist" | "build" | "__pycache__" |
                    "vendor" | ".git" | ".next" | ".cache") { continue; }
                let Ok(kind) = entry.file_type() else { continue };
                // file_type does not follow symlinks.
                if kind.is_symlink() { continue; }
                if kind.is_dir() {
                    if depth < 7 { dirs.push((entry.path(), depth+1)); }
                } else if kind.is_file() {
                    if let Ok(relative) = entry.path().strip_prefix(root) {
                        if let Some(path) = relative.to_str() {
                            if path.len() <= 360 { self.seed(path.replace('\\', "/")); }
                        }
                    }
                }
            }
        }
    }

    pub fn apply(&mut self, event: Summary, root: Option<&Path>) -> bool {
        let Some(activity) = event.classify() else { return false };
        let Some(path) = workspace_path(&event.target, root) else { return false };
        self.seed(path.clone());
        let Some(&index) = self.lookup.get(&path) else { self.dropped_events += 1; return false };
        let building = &mut self.buildings[index];
        building.activity = activity;
        building.pulse = 1.0;
        building.owned_by_agent = event.known_agent();
        self.total_events += 1;
        if building.owned_by_agent { self.agent_events += 1; }
        true
    }

    pub fn step(&mut self) {
        for b in &mut self.buildings { b.pulse = (b.pulse - 0.018).max(0.0); }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn e(kind: &str, path: &str) -> Summary {
        serde_json::from_value(serde_json::json!({"type":kind,"target":path})).unwrap()
    }
    #[test]
    fn observed_file_events_are_not_invented() {
        assert_eq!(e("file_write","/workspaces/repo/src/main.rs").classify(), Some(Activity::Write));
        assert_eq!(e("read","/workspaces/repo/src/main.rs").classify(), Some(Activity::Read));
        assert_eq!(e("openai_request","/workspaces/repo/src/main.rs").classify(), None);
        assert_eq!(e("write","socket write").classify(), Some(Activity::Write));
        let mut socket = e("write","/workspaces/repo/src/main.rs");
        socket.network = true;
        assert_eq!(socket.classify(), None);
    }
    #[test]
    fn traversal_scope_and_redacted_paths_are_rejected() {
        assert_eq!(workspace_path("/workspaces/repo/../secret", None), None);
        assert_eq!(workspace_path("/etc/passwd", None), None);
        assert_eq!(workspace_path("[redacted]", None), None);
        assert_eq!(workspace_path("/workspaces/repo/src/main.rs", None).as_deref(), Some("src/main.rs"));
        let root = Path::new("/workspaces/repo");
        assert_eq!(workspace_path("/workspaces/repo-old/src/a.rs", Some(root)), None);
    }
    #[test]
    fn city_is_bounded_and_counts_only_observed_agent_evidence() {
        let mut city = City::default();
        let shell = e("file_write","/workspaces/repo/src/main.rs");
        assert!(city.apply(shell, None));
        assert_eq!(city.total_events, 1);
        assert_eq!(city.agent_events, 0);
        let mut agent = e("file_write","/workspaces/repo/src/main.rs");
        agent.root_agent_pid = 141;
        assert!(city.apply(agent, None));
        assert_eq!(city.agent_events, 1);
        assert_eq!(city.buildings.len(), 1);
        assert!(city.buildings[0].pulse > 0.0);
    }
}
