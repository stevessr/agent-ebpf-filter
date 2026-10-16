//! A bounded, read-only projection of *observed* CodeSpace paths; never reads file contents.
use serde::Deserialize;
use std::collections::{HashMap, HashSet, VecDeque};
use std::path::{Component, Path, PathBuf};

pub const MAX_BUILDINGS: usize = 2048;
pub const MAX_EVENTS_PER_FRAME: usize = 128;
pub const MAX_HISTORY: usize = 2048;
pub const MAX_SEEN_IDS: usize = 4096;
const BLOCK_WIDTH: usize = 12;
const BLOCK_CAPACITY: usize = BLOCK_WIDTH * BLOCK_WIDTH;


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
        // Include repository identity when the view is not explicitly scoped.
        // Otherwise two CodeSpaces with "src/main.rs" collapse into one building.
        let repo_path = Path::new(repo.as_os_str());
        let relative = parts.as_path();
        if relative.as_os_str().is_empty() { return None; }
        // Preserve the repository name in the normalized path.
        return workspace_relative(&repo_path.join(relative));
    };
    workspace_relative(relative)
}

/// Never include absolute host paths or special directory entries in UI data.
fn workspace_relative(relative: &Path) -> Option<String> {
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

#[derive(Debug, Clone)]
pub struct HistoryEvent {
    pub path: String,
    pub activity: Activity,
    pub agent: bool,
    pub timestamp_ms: i64,
}

#[derive(Default)]
pub struct City {
    pub buildings: Vec<Building>,
    lookup: HashMap<String, usize>,
    district_lookup: HashMap<String, usize>,
    district_counts: Vec<usize>,
    district_tiles: HashMap<(usize, usize), usize>,
    tile_count: usize,
    pub total_events: u64,
    pub agent_events: u64,
    pub dropped_events: u64,
    pub transport_dropped: u64,
    pub connected: bool,
    pub history: VecDeque<HistoryEvent>,
    replay_cursor: Option<usize>,
    seen_ids: HashSet<String>,
    seen_order: VecDeque<String>,
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
        // Allocate additional tiles on overflow instead of wrapping every 144
        // buildings onto their earlier neighbours. All district tile IDs are
        // unique across the complete bounded city.
        let page = local_id / BLOCK_CAPACITY;
        let tile = *self.district_tiles.entry((district_id, page)).or_insert_with(|| {
            let assigned = self.tile_count;
            self.tile_count += 1;
            assigned
        });
        let slot = local_id % BLOCK_CAPACITY;
        let dx = (tile % 4) as f32 * 16.0 - 24.0;
        let dz = (tile / 4) as f32 * 16.0 - 24.0;
        let x = dx + (slot % BLOCK_WIDTH) as f32 * 1.18;
        let z = dz + (slot / BLOCK_WIDTH) as f32 * 1.18;
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
            let mut entries: Vec<_> = entries.flatten().collect();
            entries.sort_by_key(|entry| entry.file_name());
            for entry in entries {
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
        // The backend can resend the same compact event after reconnect or
        // snapshot recovery. Never double-count its stable ID.
        if !event.event_id.is_empty() && self.seen_ids.contains(&event.event_id) {
            return false;
        }
        self.seed(path.clone());
        let Some(&index) = self.lookup.get(&path) else { self.dropped_events += 1; return false };
        if !event.event_id.is_empty() {
            self.seen_ids.insert(event.event_id.clone());
            self.seen_order.push_back(event.event_id);
            if self.seen_order.len() > MAX_SEEN_IDS {
                if let Some(oldest) = self.seen_order.pop_front() {
                    self.seen_ids.remove(&oldest);
                }
            }
        }
        let agent = event.known_agent();
        self.total_events += 1;
        if agent { self.agent_events += 1; }
        self.history.push_back(HistoryEvent {
            path, activity, agent, timestamp_ms: event.received_at_ms,
        });
        if self.history.len() > MAX_HISTORY {
            self.history.pop_front();
            if let Some(cursor) = &mut self.replay_cursor {
                *cursor = cursor.saturating_sub(1);
            }
        }
        if self.replay_cursor.is_none() {
            let building = &mut self.buildings[index];
            building.activity = activity;
            building.pulse = 1.0;
            building.owned_by_agent = agent;
        }
        true
    }

    pub fn replay_back(&mut self) {
        if self.history.is_empty() { return; }
        let end = self.history.len() - 1;
        self.replay_cursor = Some(self.replay_cursor.unwrap_or(end).saturating_sub(1));
        self.rebuild_replay();
    }

    pub fn replay_forward(&mut self) {
        if let Some(cursor) = self.replay_cursor {
            if cursor + 1 >= self.history.len() { self.resume_live(); }
            else {
                self.replay_cursor = Some(cursor + 1);
                self.rebuild_replay();
            }
        }
    }

    pub fn resume_live(&mut self) {
        self.replay_cursor = None;
        self.rebuild_replay();
    }

    fn rebuild_replay(&mut self) {
        for building in &mut self.buildings {
            building.activity = Activity::Access;
            building.owned_by_agent = false;
            building.pulse = 0.0;
        }
        let limit = self.replay_cursor.unwrap_or_else(|| self.history.len().saturating_sub(1));
        for event in self.history.iter().take(limit + 1) {
            if let Some(&index) = self.lookup.get(&event.path) {
                let building = &mut self.buildings[index];
                building.activity = event.activity;
                building.owned_by_agent = event.agent;
                building.pulse = 0.0;
            }
        }
        if let Some(last) = self.history.get(limit) {
            if let Some(&index) = self.lookup.get(&last.path) {
                self.buildings[index].pulse = 1.0;
            }
        }
    }

    pub fn status_line(&self) -> String {
        let state = if self.connected { "● ONLINE" } else { "● OFFLINE" };
        let replay = match self.replay_cursor {
            Some(index) => format!("  |  REPLAY {}/{}", index + 1, self.history.len()),
            None => "  |  LIVE".into(),
        };
        format!("{state}{replay}  |  FILES {}  |  EVENTS {}  |  AGENT {}  |  LOST {}",
            self.buildings.len(), self.total_events, self.agent_events,
            self.dropped_events + self.transport_dropped)
    }

    pub fn latest_note(&self) -> String {
        let index = self.replay_cursor.unwrap_or_else(|| self.history.len().saturating_sub(1));
        let Some(event) = self.history.get(index) else {
            return "No observed file activity yet • left/right: timeline, Esc: live".into();
        };
        let verb = match event.activity {
            Activity::Read => "READ",
            Activity::Write => "WRITE",
            Activity::Blocked => "BLOCK",
            Activity::Alert => "ALERT",
            Activity::Access => "ACCESS",
        };
        let agent = if event.agent { "AGENT" } else { "UNATTRIBUTED" };
        let filename = if event.path.len() > 100 {
            format!("…{}", &event.path[event.path.char_indices()
                .nth(event.path.chars().count().saturating_sub(90))
                .map(|(i,_)| i).unwrap_or(0)..])
        } else { event.path.clone() };
        format!("{verb} • {agent} • {filename}")
    }

    pub fn step(&mut self) {
        if self.replay_cursor.is_none() {
            for b in &mut self.buildings { b.pulse = (b.pulse - 0.018).max(0.0); }
        }
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
        assert_eq!(workspace_path("/workspaces/repo/src/main.rs", None).as_deref(), Some("repo/src/main.rs"));
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

    #[test]
    fn adjacent_workspaces_never_share_building_identity() {
        let mut city = City::default();
        assert!(city.apply(e("file_write","/workspaces/alpha/src/main.rs"), None));
        assert!(city.apply(e("file_write","/workspaces/beta/src/main.rs"), None));
        assert_eq!(city.buildings.len(), 2);
    }
    #[test]
    fn district_overflow_never_stacks_two_files_on_same_coordinates() {
        let mut city = City::default();
        for i in 0..310 {
            city.seed(format!("src/file-{i}.rs"));
        }
        let positions: HashSet<(i32,i32)> = city.buildings.iter()
            .map(|b| ((b.x*100.0) as i32, (b.z*100.0) as i32)).collect();
        assert_eq!(positions.len(), 310);
        assert_eq!(city.district_tiles.len(), 3);
    }
    #[test]
    fn replay_and_stable_id_deduplication_work_while_live_events_continue() {
        let mut city = City::default();
        let mut first = e("file_write","/workspaces/repo/src/a.rs");
        first.event_id = "id-1".into();
        assert!(city.apply(first.clone(), None));
        assert!(!city.apply(first, None));
        assert_eq!(city.total_events, 1);
        let mut second = e("file_read","/workspaces/repo/src/b.rs");
        second.event_id = "id-2".into();
        assert!(city.apply(second, None));
        city.replay_back();
        assert!(city.status_line().contains("REPLAY 1/2"));
        let mut third = e("file_write","/workspaces/repo/src/c.rs");
        third.event_id = "id-3".into();
        assert!(city.apply(third, None));
        assert!(city.status_line().contains("REPLAY 1/3"));
        assert_eq!(city.total_events, 3);
        city.replay_forward();
        city.replay_forward();
        assert!(city.status_line().contains("LIVE"));
        assert_eq!(city.history.len(), 3);
    }

}
