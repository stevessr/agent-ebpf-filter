//! Time-window and actor-lane classification for the bounded CodeCity history.
//! A child OS process is NOT a verified logical Subagent. Only an explicit
//! stable subagentId supplied by a trusted future collector qualifies as one.
use std::collections::HashSet;

pub const MIN_SPAN_MS: i64 = 10_000;
pub const MAX_SPAN_MS: i64 = 24 * 60 * 60 * 1_000;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Span { All, Millis(i64) }

#[derive(Clone, Debug)]
pub struct TimeWindow {
    pub span: Span,
    pub offset_ms: i64,
}

impl Default for TimeWindow {
    fn default() -> Self { Self { span: Span::Millis(5 * 60_000), offset_ms: 0 } }
}

impl TimeWindow {
    pub fn set_minutes(&mut self, minutes: u32) {
        self.span = if minutes == 0 { Span::All }
            else { Span::Millis((minutes as i64 * 60_000).clamp(MIN_SPAN_MS, MAX_SPAN_MS)) };
        self.offset_ms = 0;
    }

    pub fn adjust_span(&mut self, longer: bool) {
        let previous = match self.span {
            Span::All => if longer { 60 * 60_000 } else { 5 * 60_000 },
            Span::Millis(span) => span,
        };
        let changed = if longer { previous.saturating_mul(2) } else { previous / 2 };
        self.span = Span::Millis(changed.clamp(MIN_SPAN_MS, MAX_SPAN_MS));
    }

    pub fn shift(&mut self, backwards: bool) {
        let step = match self.span { Span::Millis(span) => span, Span::All => 5 * 60_000 };
        self.offset_ms = if backwards {
            self.offset_ms.saturating_add(step).min(7 * 24 * 60 * 60_000)
        } else {
            self.offset_ms.saturating_sub(step).max(0)
        };
    }

    pub fn live(&mut self) { self.offset_ms = 0; }

    pub fn bounds(&self, now_ms: i64) -> (i64, i64) {
        let end = now_ms.saturating_sub(self.offset_ms);
        let start = match self.span {
            Span::All => i64::MIN,
            Span::Millis(span) => end.saturating_sub(span),
        };
        (start, end)
    }

    pub fn contains(&self, event_ms: i64, now_ms: i64) -> bool {
        let (start, end) = self.bounds(now_ms);
        event_ms > 0 && event_ms >= start && event_ms <= end
    }

    pub fn caption(&self) -> String {
        let span = match self.span {
            Span::All => "ALL (bounded history)".to_owned(),
            Span::Millis(ms) if ms % 3_600_000 == 0 => format!("{}h", ms / 3_600_000),
            Span::Millis(ms) if ms % 60_000 == 0 => format!("{}m", ms / 60_000),
            Span::Millis(ms) => format!("{}s", ms / 1_000),
        };
        if self.offset_ms == 0 { span }
        else { format!("{span} • -{}m", self.offset_ms / 60_000) }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum Lane {
    Root { run: String, pid: i32 },
    // Process descendants are only candidates; PID/PPID is not an SDK call graph.
    Delegated { run: String, root: i32, pid: i32 },
    // The current backend does NOT yet populate subagentId on file summaries.
    VerifiedSubagent { run: String, root: i32, id: String },
    RunProcess { run: String, pid: i32 },
    Unattributed,
}

impl Lane {
    pub fn from_fields(run: &str, root: i32, pid: i32, subagent_id: &str) -> Self {
        let run = run.trim().chars().take(96).collect::<String>();
        let valid_id = !subagent_id.is_empty() && subagent_id.len() <= 96
            && !subagent_id.chars().any(char::is_control);
        if valid_id && (!run.is_empty() || root > 0) {
            return Self::VerifiedSubagent { run, root, id: subagent_id.into() };
        }
        if root > 0 && pid > 0 && pid == root { Self::Root { run, pid } }
        else if root > 0 && pid > 0 {
            Self::Delegated { run, root, pid }
        } else if !run.is_empty() {
            Self::RunProcess { run, pid }
        } else {
            Self::Unattributed
        }
    }

    pub fn title(&self) -> String {
        match self {
            Self::Root { run, pid } => format!("MAIN Agent PID {pid}{}", short_run(run)),
            Self::Delegated { run, root, pid } =>
                format!("CHILD PROCESS PID {pid} ← {root}{} (not confirmed Subagent)", short_run(run)),
            Self::VerifiedSubagent { run, id, .. } =>
                format!("SUBAGENT {id}{} (explicit ID)", short_run(run)),
            Self::RunProcess { run, pid } =>
                format!("RUN / PID {pid}{}", short_run(run)),
            Self::Unattributed => "UNATTRIBUTED".into(),
        }
    }

    pub fn is_verified_subagent(&self) -> bool { matches!(self, Self::VerifiedSubagent { .. }) }
}

fn short_run(run: &str) -> String {
    if run.is_empty() { return String::new(); }
    let truncated = run.chars().take(12).collect::<String>();
    format!(" • run {truncated}")
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum LaneSelection {
    All,
    Main,
    Delegated,
    Verified,
    One(Lane),
}

impl Default for LaneSelection {
    fn default() -> Self { Self::All }
}

impl LaneSelection {
    pub fn includes(&self, lane: &Lane) -> bool {
        match self {
            Self::All => true,
            Self::Main => matches!(lane, Lane::Root { .. } | Lane::RunProcess { .. }),
            Self::Delegated => matches!(lane, Lane::Delegated { .. }),
            Self::Verified => lane.is_verified_subagent(),
            Self::One(chosen) => lane == chosen,
        }
    }

    pub fn label(&self) -> String {
        match self {
            Self::All => "ALL ACTORS".into(),
            Self::Main => "MAIN AGENTS".into(),
            Self::Delegated => "CHILD PROCESSES (candidates)".into(),
            Self::Verified => "VERIFIED SUBAGENTS".into(),
            Self::One(lane) => lane.title(),
        }
    }
}

/// A deterministic, finite distinct-lane browser; no dynamic widget explosion.
pub fn cycle_lane(current: &LaneSelection, events: impl Iterator<Item = Lane>,
    forward: bool) -> LaneSelection {
    let mut seen = HashSet::new();
    let mut lanes = events.filter(|lane| seen.insert(lane.clone())).collect::<Vec<_>>();
    lanes.sort();
    if lanes.is_empty() { return LaneSelection::All; }
    let index = match current {
        LaneSelection::One(id) => lanes.iter().position(|lane| lane == id),
        _ => None,
    };
    let next = match (index, forward) {
        (None, true) => 0,
        (None, false) => lanes.len() - 1,
        (Some(i), true) => (i + 1) % lanes.len(),
        (Some(i), false) => (i + lanes.len() - 1) % lanes.len(),
    };
    LaneSelection::One(lanes[next].clone())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn span_boundaries_navigation_and_live() {
        let mut w = TimeWindow::default();
        assert!(w.contains(900_000, 1_000_000));
        assert!(!w.contains(600_000, 1_000_000));
        assert!(!w.contains(0, 1_000_000));
        w.shift(true);
        assert!(w.contains(600_000, 1_000_000));
        assert!(!w.contains(900_000, 1_000_000));
        w.shift(false);
        assert_eq!(w.offset_ms, 0);
        w.set_minutes(0);
        assert!(w.contains(1, 1_000_000));
        w.set_minutes(60);
        assert_eq!(w.caption(), "1h");
        w.adjust_span(false);
        assert_eq!(w.caption(), "30m");
        for _ in 0..30 { w.adjust_span(false); }
        assert_eq!(w.span, Span::Millis(MIN_SPAN_MS));
        for _ in 0..30 { w.adjust_span(true); }
        assert_eq!(w.span, Span::Millis(MAX_SPAN_MS));
    }

    #[test]
    fn no_spurious_subagent_from_descendant_process() {
        let root = Lane::from_fields("run-a", 100, 100, "");
        let child = Lane::from_fields("run-a", 100, 101, "");
        assert!(matches!(root, Lane::Root {..}));
        assert!(matches!(child, Lane::Delegated {..}));
        assert!(!child.is_verified_subagent());
        assert!(Lane::from_fields("run-a", 100, 101, "sdk-child-id").is_verified_subagent());
        assert!(!Lane::from_fields("", 0, 120, "unanchored-id").is_verified_subagent());
    }

    #[test]
    fn actor_isolation_respects_run_root_pid_and_explicit_subagent() {
        let one = Lane::from_fields("run-1", 42, 73, "");
        let two = Lane::from_fields("run-2", 42, 73, "");
        let three = Lane::from_fields("run-1", 42, 74, "");
        assert_ne!(one, two);
        assert_ne!(one, three);
        let chosen = LaneSelection::One(one.clone());
        assert!(chosen.includes(&one));
        assert!(!chosen.includes(&two));
        assert!(!chosen.includes(&three));
        assert!(LaneSelection::Delegated.includes(&one));
        assert!(!LaneSelection::Verified.includes(&one));
        assert!(LaneSelection::Verified.includes(
            &Lane::from_fields("run-1", 42, 74, "explicit-id")));
    }

    #[test]
    fn cycle_scopes_only_to_actual_lanes() {
        let main = Lane::from_fields("abc", 10, 10, "");
        let child = Lane::from_fields("abc", 10, 11, "");
        let one = cycle_lane(&LaneSelection::All,
            [child.clone(), main.clone(), child.clone()].into_iter(), true);
        let two = cycle_lane(&one, [main.clone(), child.clone()].into_iter(), true);
        assert_ne!(one, two);
    }
}
