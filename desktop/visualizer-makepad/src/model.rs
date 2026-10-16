//! A bounded, read-only projection of *observed* CodeSpace paths; never reads file contents.
use serde::Deserialize;
use crate::timeline::{self, Lane, LaneSelection, TimeWindow};
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
    #[serde(default)]
    pub pid: i32,
    #[serde(default)]
    pub ppid: i32,
    // Not currently populated by the compact backend. Only future explicit
    // trusted identifiers can establish a logical SDK Subagent lane.
    #[serde(default)]
    pub subagent_id: String,
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
    // A remote CodeSpace root generally does NOT exist on the graphics
    // workstation. Still preserve it as an exact logical event-path filter.
    if !root.is_absolute() || root.components()
        .any(|c| matches!(c, Component::ParentDir | Component::CurDir)) {
        return None;
    }
    Some(root)
}

/// Only locally accessible physical roots may seed metadata. Never follow a
/// symbolic link supplied as the root of the metadata traversal.
pub fn local_scan_root(root: &Path) -> bool {
    std::fs::symlink_metadata(root).ok()
        .is_some_and(|meta| meta.is_dir() && !meta.file_type().is_symlink())
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
    pub visible: bool,
    pub last_lane: Option<Lane>,
}

#[derive(Debug, Clone)]
pub struct HistoryEvent {
    pub path: String,
    pub activity: Activity,
    pub agent: bool,
    pub timestamp_ms: i64,
    pub lane: Lane,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SubagentSignalKind { Start, Stop }

#[derive(Clone, Debug)]
pub struct SubagentSignal {
    pub event_id: String,
    pub timestamp_ms: i64,
    pub kind: SubagentSignalKind,
    pub lane: Lane,
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
    // Explicit native-hook lifecycle signals are kept separate from file I/O.
    pub subagent_signals: VecDeque<SubagentSignal>,
    pub visible_subagent_starts: usize,
    pub visible_subagent_stops: usize,
    replay_cursor: Option<usize>,
    seen_ids: HashSet<String>,
    seen_order: VecDeque<String>,
    agent_run_filter: Option<String>,
    agent_root_filter: Option<i32>,
    agent_only: bool,
    pub time_window: TimeWindow,
    pub lane_selection: LaneSelection,
    pub visible_events: u64,
    pub visible_agent_events: u64,
    dirty_view: bool,
    last_view_second: i64,
}

impl City {
    pub fn set_agent_scope(&mut self, run_id: Option<String>, root_pid: Option<i32>, only_agent: bool) {
        self.agent_run_filter = run_id.filter(|s| !s.trim().is_empty() && s.len() <= 256);
        self.agent_root_filter = root_pid.filter(|pid| *pid > 0);
        self.agent_only = only_agent;
    }

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
            activity: Activity::Access, owned_by_agent: false, visible: false,
            last_lane: None });
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

    /// Populate the CodeSpace layout from an explicitly imported file-name-only
    /// JSON manifest. This also works when the CodeSpace is remote and the
    /// renderer has no filesystem mount. No files mentioned are opened.
    pub fn seed_manifest(&mut self, bytes: &[u8]) -> Result<usize, &'static str> {
        if bytes.len() > 1024 * 1024 {
            return Err("metadata manifest exceeds 1 MiB");
        }
        #[derive(Deserialize)]
        struct Manifest { version: u32, files: Vec<String> }
        let manifest: Manifest = serde_json::from_slice(bytes)
            .map_err(|_| "malformed metadata manifest")?;
        if manifest.version != 1 {
            return Err("unsupported metadata manifest version");
        }
        let before = self.buildings.len();
        for candidate in manifest.files.into_iter().take(8_000) {
            let path = Path::new(candidate.as_str());
            if path.is_absolute() || path.components()
                .any(|c| !matches!(c, Component::Normal(_))) { continue; }
            if let Some(normalized) = workspace_relative(path) {
                self.seed(normalized);
            }
            if self.buildings.len() >= MAX_BUILDINGS { break; }
        }
        Ok(self.buildings.len() - before)
    }

    pub fn observe(&mut self, event: Summary, root: Option<&Path>) -> bool {
        // Native lifecycle hooks are not file operations and never light towers.
        if event.kind == "native_hook" {
            let name = event.comm.split(':').next().unwrap_or("").trim();
            let signal = match name.to_ascii_lowercase().as_str() {
                "subagentstart" | "agentspawn" => Some(SubagentSignalKind::Start),
                "subagentstop" => Some(SubagentSignalKind::Stop),
                _ => None,
            };
            if let Some(kind) = signal {
                if event.received_at_ms <= 0 { return false; }
                if self.agent_only && !event.known_agent() { return false; }
                if self.agent_run_filter.as_deref()
                    .is_some_and(|run| event.agent_run_id != run) { return false; }
                if self.agent_root_filter.is_some_and(|root_pid| event.root_agent_pid != root_pid) {
                    return false;
                }
                if !event.event_id.is_empty() && self.subagent_signals.iter()
                    .any(|entry| entry.event_id == event.event_id) { return false; }
                let lane = Lane::from_fields(&event.agent_run_id, event.root_agent_pid,
                    event.pid, &event.subagent_id);
                if self.subagent_signals.len() >= MAX_HISTORY {
                    self.subagent_signals.pop_front();
                }
                self.subagent_signals.push_back(SubagentSignal {
                    event_id: event.event_id, timestamp_ms: event.received_at_ms, kind, lane,
                });
                self.dirty_view = true;
                return true;
            }
        }
        self.apply(event, root)
    }

    pub fn apply(&mut self, event: Summary, root: Option<&Path>) -> bool {
        // Filters require exact recorded context, never process-name guesses
        // or substring matching of session/tool/target fields.
        if self.agent_only && !event.known_agent() { return false; }
        if self.agent_run_filter.as_deref().is_some_and(|run| event.agent_run_id != run) {
            return false;
        }
        if self.agent_root_filter.is_some_and(|root| event.root_agent_pid != root) {
            return false;
        }
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
            self.seen_order.push_back(event.event_id.clone());
            if self.seen_order.len() > MAX_SEEN_IDS {
                if let Some(oldest) = self.seen_order.pop_front() {
                    self.seen_ids.remove(&oldest);
                }
            }
        }
        let agent = event.known_agent();
        let lane = Lane::from_fields(&event.agent_run_id, event.root_agent_pid, event.pid,
            &event.subagent_id);
        self.total_events += 1;
        if agent { self.agent_events += 1; }
        self.history.push_back(HistoryEvent {
            path, activity, agent, timestamp_ms: event.received_at_ms, lane: lane.clone(),
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
            building.last_lane = Some(lane);
            building.visible = true;
        }
        self.dirty_view = true;
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
            if cursor + 1 >= self.history.len().saturating_sub(1) { self.resume_live(); }
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
        self.dirty_view = true;
        self.refresh_view(system_time_ms());
    }

    pub fn set_time_span(&mut self, minutes: u32) {
        self.time_window.set_minutes(minutes);
        self.dirty_view = true;
        self.refresh_view(system_time_ms());
    }

    pub fn resize_time_span(&mut self, longer: bool) {
        self.time_window.adjust_span(longer);
        self.dirty_view = true;
        self.refresh_view(system_time_ms());
    }

    pub fn shift_time_range(&mut self, older: bool) {
        self.time_window.shift(older);
        self.dirty_view = true;
        self.refresh_view(system_time_ms());
    }

    pub fn resume_time_live(&mut self) {
        self.time_window.live();
        self.replay_cursor = None;
        self.dirty_view = true;
        self.refresh_view(system_time_ms());
    }

    pub fn set_lane_selection(&mut self, chosen: LaneSelection) {
        self.lane_selection = chosen;
        self.dirty_view = true;
        self.refresh_view(system_time_ms());
    }

    pub fn cycle_actor(&mut self, forward: bool) {
        let now = self.anchor_ms(system_time_ms());
        let snapshot = self.history.iter()
            .filter(|event| self.time_window.contains(event.timestamp_ms, now))
            .map(|event| event.lane.clone())
            .chain(self.subagent_signals.iter()
                .filter(|signal| self.time_window.contains(signal.timestamp_ms, now))
                .map(|signal| signal.lane.clone()));
        self.lane_selection = timeline::cycle_lane(&self.lane_selection, snapshot, forward);
        self.dirty_view = true;
        self.refresh_view(system_time_ms());
    }

    fn anchor_ms(&self, now_ms: i64) -> i64 {
        self.replay_cursor
            .and_then(|pos| self.history.get(pos).map(|e| e.timestamp_ms))
            .filter(|ts| *ts > 0)
            .unwrap_or(now_ms)
    }

    fn event_visible(&self, event: &HistoryEvent, index: usize, anchor: i64) -> bool {
        self.replay_cursor.is_none_or(|cursor| index <= cursor)
            && self.time_window.contains(event.timestamp_ms, anchor)
            && self.lane_selection.includes(&event.lane)
    }

    /// Refresh the visible city only on source changes, user selections, or a
    /// second boundary; all counters and highlights use the same interval.
    pub fn refresh_view(&mut self, now_ms: i64) {
        let second = now_ms / 1000;
        if !self.dirty_view && (self.replay_cursor.is_some() || self.last_view_second == second) {
            return;
        }
        self.dirty_view = false;
        self.last_view_second = second;
        let anchor = self.anchor_ms(now_ms);
        let visible = self.history.iter().enumerate()
            .filter(|(index, event)| self.event_visible(event, *index, anchor))
            .map(|(_, event)| event.clone())
            .collect::<Vec<_>>();
        for building in &mut self.buildings {
            building.visible = false;
            building.last_lane = None;
            building.activity = Activity::Access;
            building.owned_by_agent = false;
            building.pulse = 0.0;
        }
        self.visible_events = visible.len() as u64;
        self.visible_agent_events = visible.iter().filter(|e| e.agent).count() as u64;
        self.visible_subagent_starts = 0;
        self.visible_subagent_stops = 0;
        for signal in &self.subagent_signals {
            if self.time_window.contains(signal.timestamp_ms, anchor) &&
                (self.lane_selection.includes(&signal.lane) ||
                 matches!(self.lane_selection, LaneSelection::Verified)) {
                match signal.kind {
                    SubagentSignalKind::Start => self.visible_subagent_starts += 1,
                    SubagentSignalKind::Stop => self.visible_subagent_stops += 1,
                }
            }
        }
        for event in visible {
            if let Some(&index) = self.lookup.get(&event.path) {
                let building = &mut self.buildings[index];
                building.visible = true;
                building.activity = event.activity;
                building.owned_by_agent = event.agent;
                building.last_lane = Some(event.lane.clone());
                // Time-based intensity reflects the actual event age, not UI FPS.
                let age = anchor.saturating_sub(event.timestamp_ms);
                if (0..=6_000).contains(&age) {
                    building.pulse = ((6_000 - age) as f32 / 6_000.0).max(building.pulse);
                }
            }
        }
    }

    /// Directory name/position pairs for small, camera-projected district
    /// labels. Labels are UI metadata, not a dependency or syscall trace.
    pub fn district_labels(&self, limit: usize) -> Vec<(String, f32, f32)> {
        let mut districts: Vec<_> = self.district_lookup.iter()
            .filter_map(|(name, id)| {
                let tile = *self.district_tiles.get(&(*id, 0))?;
                let x = (tile % 4) as f32 * 16.0 - 17.5;
                let z = (tile / 4) as f32 * 16.0 - 17.5;
                Some((tile, name.clone(), x, z))
            }).collect();
        districts.sort_by_key(|d| d.0);
        districts.into_iter().take(limit.min(24))
            .map(|(_, name, x, z)| (name, x, z)).collect()
    }

    pub fn status_line(&self) -> String {
        let state = if self.connected { "● ONLINE" } else { "● OFFLINE" };
        let replay = match self.replay_cursor {
            Some(index) => format!("  |  REPLAY {}/{}", index + 1, self.history.len()),
            None => "  |  LIVE".into(),
        };
        let filtered = if self.agent_only || self.agent_run_filter.is_some() || self.agent_root_filter.is_some() {
            "  |  AGENT SCOPE"
        } else { "" };
        format!("{state}{replay}{filtered}  |  SPAN {}  |  {}  |  FILE {} (AGENT {})  |  HOOKS +{}/-{}  |  LOST {}",
            self.time_window.caption(), self.lane_selection.label(),
            self.visible_events, self.visible_agent_events,
            self.visible_subagent_starts, self.visible_subagent_stops,
            self.dropped_events + self.transport_dropped)
    }

    pub fn latest_note(&self) -> String {
        let anchor = self.anchor_ms(system_time_ms());
        let Some((_,event)) = self.history.iter().enumerate().rev()
            .find(|(index,event)| self.event_visible(event,*index,anchor)) else {
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
        format!("{verb} • {agent} • {} • {filename}", event.lane.title())
    }

    pub fn step(&mut self) {
        self.refresh_view(system_time_ms());
    }
}

pub fn system_time_ms() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now().duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis().min(i64::MAX as u128) as i64)
        .unwrap_or(0)
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

    #[test]
    fn exact_agent_scoping_never_attributes_generic_shells() {
        let mut city = City::default();
        city.set_agent_scope(Some("run-1".into()), Some(127), true);
        let mut match_event = e("file_write", "/workspaces/repo/src/agent.rs");
        match_event.agent_run_id = "run-1".into();
        match_event.root_agent_pid = 127;
        assert!(city.apply(match_event, None));
        let mut other = e("file_write", "/workspaces/repo/src/other.rs");
        other.agent_run_id = "run-10".into();
        other.root_agent_pid = 127;
        assert!(!city.apply(other, None));
        let mut shell = e("file_write", "/workspaces/repo/src/shell.rs");
        shell.tag = "Shell".into();
        assert!(!city.apply(shell, None));
        assert_eq!(city.agent_events, 1);
        assert_eq!(city.history.len(), 1);
    }


    #[test]
    fn remote_manifest_is_metadata_only_bounded_and_path_safe() {
        let mut city = City::default();
        let bytes = serde_json::to_vec(&serde_json::json!({
            "version":1, "files":[
                "src/main.rs", "src/main.rs", "../outside.txt", "/etc/passwd",
                "src/../bad.rs", ".git/config", "assets/logo.svg"
            ]
        })).unwrap();
        assert_eq!(city.seed_manifest(&bytes).unwrap(), 2);
        assert_eq!(city.buildings.len(), 2);
        assert!(city.buildings.iter().any(|b| b.path == "src/main.rs"));
        assert!(city.buildings.iter().any(|b| b.path == "assets/logo.svg"));
        assert!(city.seed_manifest(&vec![b' '; 1_048_577]).is_err());
        assert!(city.seed_manifest(br#"{"version":2,"files":[]}"#).is_err());
    }

    #[test]
    fn visible_counters_and_highlights_follow_time_span_and_actor() {
        let now = system_time_ms();
        let mut city = City::default();
        let mut root = e("file_write","/workspaces/repo/src/a.rs");
        root.pid = 10; root.root_agent_pid = 10;
        root.agent_run_id = "test-run".into();
        root.received_at_ms = now - 15_000;
        assert!(city.apply(root, None));
        let mut child = e("file_read","/workspaces/repo/src/b.rs");
        child.pid = 11; child.root_agent_pid = 10;
        child.agent_run_id = "test-run".into();
        child.received_at_ms = now - 120_000;
        assert!(city.apply(child, None));
        city.set_time_span(1);
        city.refresh_view(now);
        assert_eq!(city.visible_events, 1);
        assert!(city.buildings[0].visible);
        assert!(!city.buildings[1].visible);
        city.set_time_span(5);
        city.set_lane_selection(LaneSelection::Delegated);
        city.refresh_view(now);
        assert_eq!(city.visible_events, 1);
        assert!(!city.buildings[0].visible);
        assert!(city.buildings[1].visible);
        city.set_lane_selection(LaneSelection::Verified);
        assert_eq!(city.visible_events, 0);
        // The current stream does not provide verifiable SDK Subagent IDs.
        assert_eq!(city.agent_events, 2);
    }

    #[test]
    fn native_subagent_hooks_are_separate_from_file_activity() {
        let now = system_time_ms();
        let mut city = City::default();
        let mut started = e("native_hook", "");
        started.comm = "SubagentStart:Explore".into();
        started.event_id = "start-1".into();
        started.root_agent_pid = 100;
        started.pid = 100;
        started.agent_run_id = "run-1".into();
        started.subagent_id = "sha256:0123456789abcdef0123".into();
        started.received_at_ms = now - 5_000;
        assert!(city.observe(started.clone(), None));
        assert!(!city.observe(started, None));
        city.refresh_view(now);
        assert_eq!(city.visible_subagent_starts, 1);
        assert_eq!(city.visible_subagent_stops, 0);
        assert_eq!(city.visible_events, 0);
        assert!(city.buildings.is_empty());
        city.set_lane_selection(LaneSelection::Verified);
        assert_eq!(city.visible_subagent_starts, 1);
        assert_eq!(city.visible_events, 0);
        assert!(matches!(city.subagent_signals[0].lane,
            Lane::VerifiedSubagent { .. }));
        let mut second = e("native_hook", "");
        second.comm = "SubagentStart:Plan".into();
        second.event_id = "start-2".into();
        second.root_agent_pid = 100;
        second.pid = 100;
        second.agent_run_id = "run-1".into();
        second.subagent_id = "sha256:abcdef0123456789abcd".into();
        second.received_at_ms = now - 3_000;
        assert!(city.observe(second, None));
        city.refresh_view(now);
        assert_eq!(city.visible_subagent_starts, 2);
        city.cycle_actor(true);
        assert_eq!(city.visible_subagent_starts, 1);
        assert!(matches!(city.lane_selection, LaneSelection::One(_)));
        city.shift_time_range(true);
        assert_eq!(city.visible_subagent_starts, 0);
    }

}
