use std::{borrow::Cow, cell::RefCell, collections::VecDeque};

use futures_util::StreamExt;
use gloo_net::{
    http::Request,
    websocket::{Message as WsMessage, futures::WebSocket},
};
use gpui::prelude::*;
use gpui::{
    App, ApplicationHandle, Bounds, Context, ElementId, IntoElement, Render, SharedString, Task,
    Window, WindowBounds, WindowOptions, div, px, rgb, size,
};
use prost::Message;

use agent_proto::pb::{Event, EventBatch, EventHistoryResponse};

const API_TOKEN_KEY: &str = "agent-ebpf.apiToken";
const CLUSTER_TARGET_KEY: &str = "agent-ebpf.clusterTarget";
const LOCAL_CLUSTER_TARGET: &str = "local";
const MAX_EVENTS: usize = 500;

const FONT_URLS: &[&str] = &[
    "https://fonts.gstatic.com/s/notosansdisplay/v20/RLplK4fy6r6tOBEJg0IAKzqdFZVZxokvfn_BDLxR.ttf",
    "https://fonts.gstatic.com/s/notosansdevanagari/v30/TuGoUUFzXI5FBtUq5a8bjKYTZjtRU6Sgv3NaV_SNmI0b8QQCQmHn6B2OHjbL_08AlXQly-A.ttf",
];

const BG: u32 = 0x0b0f18;
const SIDEBAR: u32 = 0x101725;
const SURFACE: u32 = 0x151e2f;
const SURFACE_ALT: u32 = 0x1b273b;
const BORDER: u32 = 0x26354e;
const TEXT: u32 = 0xe7edf7;
const MUTED: u32 = 0x8fa2bd;
const ACCENT: u32 = 0x6aa7ff;
const GREEN: u32 = 0x67d391;
const YELLOW: u32 = 0xf1c75b;
const RED: u32 = 0xff7c87;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Page {
    Dashboard,
    Monitor,
    Observe,
    Network,
    ExecutionGraph,
    Research,
    Explorer,
    Executor,
    Hooks,
    Ml,
    Plugins,
    Config,
    Signals,
    Recording,
}

impl Page {
    const ALL: [Page; 14] = [
        Page::Dashboard,
        Page::Monitor,
        Page::Observe,
        Page::Network,
        Page::ExecutionGraph,
        Page::Research,
        Page::Explorer,
        Page::Executor,
        Page::Hooks,
        Page::Ml,
        Page::Plugins,
        Page::Config,
        Page::Signals,
        Page::Recording,
    ];

    fn label(self) -> &'static str {
        match self {
            Page::Dashboard => "Dashboard",
            Page::Monitor => "Monitor",
            Page::Observe => "Observe",
            Page::Network => "Network",
            Page::ExecutionGraph => "Execution Graph",
            Page::Research => "Research",
            Page::Explorer => "Explorer",
            Page::Executor => "Executor",
            Page::Hooks => "Hooks",
            Page::Ml => "ML",
            Page::Plugins => "Plugins",
            Page::Config => "Configuration",
            Page::Signals => "Signals",
            Page::Recording => "Recording / Replay",
        }
    }

    fn route(self) -> &'static str {
        match self {
            Page::Dashboard => "/dashboard",
            Page::Monitor => "/monitor",
            Page::Observe => "/observe",
            Page::Network => "/network",
            Page::ExecutionGraph => "/execution-graph",
            Page::Research => "/research",
            Page::Explorer => "/explorer",
            Page::Executor => "/executor",
            Page::Hooks => "/hooks",
            Page::Ml => "/ml",
            Page::Plugins => "/plugins",
            Page::Config => "/config",
            Page::Signals => "/settings/signals",
            Page::Recording => "/recording",
        }
    }

    fn from_path(path: &str) -> Self {
        if path.starts_with("/monitor") {
            Self::Monitor
        } else if path.starts_with("/observe") {
            Self::Observe
        } else if path.starts_with("/network") || path.starts_with("/tls-capture") {
            Self::Network
        } else if path.starts_with("/execution-graph") || path.starts_with("/agentsight") {
            Self::ExecutionGraph
        } else if path.starts_with("/research") {
            Self::Research
        } else if path.starts_with("/explorer") {
            Self::Explorer
        } else if path.starts_with("/executor") {
            Self::Executor
        } else if path.starts_with("/hooks") {
            Self::Hooks
        } else if path.starts_with("/ml") || path.starts_with("/config/ml") {
            Self::Ml
        } else if path.starts_with("/plugins") {
            Self::Plugins
        } else if path.starts_with("/config") {
            Self::Config
        } else if path.starts_with("/settings/signals") {
            Self::Signals
        } else if path.starts_with("/recording") {
            Self::Recording
        } else {
            Self::Dashboard
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum StreamStatus {
    Connecting,
    Connected,
    Disconnected,
    Failed,
}

impl StreamStatus {
    fn label(self) -> &'static str {
        match self {
            Self::Connecting => "connecting",
            Self::Connected => "live",
            Self::Disconnected => "disconnected",
            Self::Failed => "error",
        }
    }

    fn color(self) -> u32 {
        match self {
            Self::Connected => GREEN,
            Self::Connecting => YELLOW,
            Self::Disconnected | Self::Failed => RED,
        }
    }
}

#[derive(Clone)]
struct EventRow {
    pid: u32,
    ppid: u32,
    comm: SharedString,
    kind: SharedString,
    path: SharedString,
    endpoint: SharedString,
    decision: SharedString,
    risk_score: f64,
}

impl From<Event> for EventRow {
    fn from(event: Event) -> Self {
        Self {
            pid: event.pid,
            ppid: event.ppid,
            comm: event.comm.into(),
            kind: event.r#type.into(),
            path: event.path.into(),
            endpoint: event.net_endpoint.into(),
            decision: event.decision.into(),
            risk_score: event.risk_score,
        }
    }
}

struct Dashboard {
    page: Page,
    stream_status: StreamStatus,
    status_message: SharedString,
    events: VecDeque<EventRow>,
    stream_task: Option<Task<()>>,
    history_task: Option<Task<()>>,
}

impl Dashboard {
    fn new(cx: &mut Context<Self>) -> Self {
        let path = browser_path();
        let mut this = Self {
            page: Page::from_path(&path),
            stream_status: StreamStatus::Connecting,
            status_message: "Starting protobuf event stream…".into(),
            events: VecDeque::new(),
            stream_task: None,
            history_task: None,
        };
        this.load_history(cx);
        this.connect_stream(cx);
        this
    }

    fn push_events(&mut self, events: impl IntoIterator<Item = Event>) {
        for event in events {
            self.events.push_front(event.into());
        }
        while self.events.len() > MAX_EVENTS {
            self.events.pop_back();
        }
    }

    fn load_history(&mut self, cx: &mut Context<Self>) {
        let token = stored(API_TOKEN_KEY);
        let cluster = stored(CLUSTER_TARGET_KEY);
        self.history_task = Some(cx.spawn(async move |this, cx| {
            let result = async {
                let mut request = Request::get("/events/recent?limit=200")
                    .header("Accept", "application/x-protobuf, application/json;q=0.9");
                let bearer;
                if !token.is_empty() {
                    bearer = format!("Bearer {token}");
                    request = request
                        .header("X-API-KEY", &token)
                        .header("Authorization", &bearer);
                }
                if !cluster.is_empty() && cluster != LOCAL_CLUSTER_TARGET {
                    request = request.header("X-Cluster-Target", &cluster);
                }
                let response = request.send().await.map_err(|error| error.to_string())?;
                if !response.ok() {
                    return Err(format!("history HTTP {}", response.status()));
                }
                let bytes = response.binary().await.map_err(|error| error.to_string())?;
                EventHistoryResponse::decode(bytes.as_slice()).map_err(|error| error.to_string())
            }
            .await;

            let _ = this.update(cx, |this, cx| {
                match result {
                    Ok(history) => {
                        let events = history
                            .events
                            .into_iter()
                            .filter_map(|record| record.event)
                            .collect::<Vec<_>>();
                        this.push_events(events);
                        this.status_message = "History loaded; waiting for live events".into();
                    }
                    Err(error) => {
                        this.status_message =
                            format!("History unavailable ({error}); live stream still enabled")
                                .into();
                    }
                }
                cx.notify();
            });
        }));
    }

    fn connect_stream(&mut self, cx: &mut Context<Self>) {
        let url = websocket_url("/ws");
        self.stream_status = StreamStatus::Connecting;
        self.stream_task = Some(cx.spawn(async move |this, cx| {
            let mut socket = match WebSocket::open(&url) {
                Ok(socket) => socket,
                Err(error) => {
                    let _ = this.update(cx, |this, cx| {
                        this.stream_status = StreamStatus::Failed;
                        this.status_message = format!("WebSocket open failed: {error}").into();
                        cx.notify();
                    });
                    return;
                }
            };

            let _ = this.update(cx, |this, cx| {
                this.stream_status = StreamStatus::Connected;
                this.status_message = "Connected to /ws protobuf stream".into();
                cx.notify();
            });

            while let Some(message) = socket.next().await {
                match message {
                    Ok(WsMessage::Bytes(bytes)) => {
                        if let Ok(batch) = EventBatch::decode(bytes.as_slice()) {
                            let _ = this.update(cx, |this, cx| {
                                this.push_events(batch.events);
                                cx.notify();
                            });
                        }
                    }
                    Ok(WsMessage::Text(_)) => {}
                    Err(error) => {
                        let _ = this.update(cx, |this, cx| {
                            this.stream_status = StreamStatus::Failed;
                            this.status_message = format!("WebSocket error: {error}").into();
                            cx.notify();
                        });
                        return;
                    }
                }
            }

            let _ = this.update(cx, |this, cx| {
                this.stream_status = StreamStatus::Disconnected;
                this.status_message = "WebSocket closed; reload to reconnect".into();
                cx.notify();
            });
        }));
    }

    fn select_page(&mut self, page: Page, cx: &mut Context<Self>) {
        self.page = page;
        push_route(page.route());
        cx.notify();
    }

    fn metric_card(
        &self,
        title: &'static str,
        value: impl Into<SharedString>,
        accent: u32,
    ) -> impl IntoElement {
        div()
            .flex()
            .flex_col()
            .gap_1()
            .p_3()
            .w(px(190.))
            .rounded_md()
            .bg(rgb(SURFACE))
            .border_1()
            .border_color(rgb(BORDER))
            .child(div().text_sm().text_color(rgb(MUTED)).child(title))
            .child(div().text_xl().text_color(rgb(accent)).child(value.into()))
    }

    fn event_row(&self, event: &EventRow, index: usize) -> impl IntoElement {
        let target: SharedString = if !event.path.is_empty() {
            event.path.clone()
        } else if !event.endpoint.is_empty() {
            event.endpoint.clone()
        } else {
            "—".into()
        };
        let decision_color = if event.decision.as_ref() == "BLOCK" {
            RED
        } else if event.decision.as_ref() == "ALERT" {
            YELLOW
        } else {
            MUTED
        };

        div()
            .id(ElementId::NamedInteger("event-row".into(), index as u64))
            .flex()
            .flex_row()
            .items_center()
            .gap_3()
            .px_3()
            .py_2()
            .when(index % 2 == 0, |row| row.bg(rgb(SURFACE)))
            .child(
                div()
                    .w(px(72.))
                    .text_sm()
                    .text_color(rgb(MUTED))
                    .child(event.pid.to_string()),
            )
            .child(
                div()
                    .w(px(150.))
                    .text_sm()
                    .text_color(rgb(TEXT))
                    .child(event.comm.clone()),
            )
            .child(
                div()
                    .w(px(150.))
                    .text_sm()
                    .text_color(rgb(ACCENT))
                    .child(event.kind.clone()),
            )
            .child(
                div()
                    .w(px(440.))
                    .text_sm()
                    .text_color(rgb(MUTED))
                    .child(target),
            )
            .child(
                div()
                    .w(px(90.))
                    .text_sm()
                    .text_color(rgb(decision_color))
                    .child(if event.decision.is_empty() {
                        "—".into()
                    } else {
                        event.decision.clone()
                    }),
            )
    }

    fn event_table<'a>(
        &'a self,
        rows: impl Iterator<Item = &'a EventRow>,
        empty: &'static str,
    ) -> impl IntoElement {
        let selected = rows.take(24).collect::<Vec<_>>();
        let mut table = div()
            .flex()
            .flex_col()
            .w_full()
            .rounded_md()
            .border_1()
            .border_color(rgb(BORDER))
            .child(
                div()
                    .flex()
                    .flex_row()
                    .gap_3()
                    .px_3()
                    .py_2()
                    .bg(rgb(SURFACE_ALT))
                    .text_sm()
                    .text_color(rgb(MUTED))
                    .child(div().w(px(72.)).child("PID"))
                    .child(div().w(px(150.)).child("Command"))
                    .child(div().w(px(150.)).child("Event"))
                    .child(div().w(px(440.)).child("Target"))
                    .child(div().w(px(90.)).child("Decision")),
            );

        if selected.is_empty() {
            table = table.child(div().p_4().text_color(rgb(MUTED)).child(empty));
        } else {
            for (index, event) in selected.into_iter().enumerate() {
                table = table.child(self.event_row(event, index));
            }
        }
        table
    }

    fn page_dashboard(&self) -> impl IntoElement {
        let blocked = self
            .events
            .iter()
            .filter(|event| event.decision.as_ref() == "BLOCK")
            .count();
        let risky = self
            .events
            .iter()
            .filter(|event| event.risk_score >= 60.0)
            .count();
        let network = self
            .events
            .iter()
            .filter(|event| !event.endpoint.is_empty())
            .count();

        div()
            .flex()
            .flex_col()
            .gap_4()
            .child(
                div()
                    .flex()
                    .flex_row()
                    .gap_3()
                    .child(self.metric_card(
                        "Buffered events",
                        self.events.len().to_string(),
                        ACCENT,
                    ))
                    .child(self.metric_card("Network events", network.to_string(), GREEN))
                    .child(self.metric_card("Risk >= 60", risky.to_string(), YELLOW))
                    .child(self.metric_card("Blocked", blocked.to_string(), RED)),
            )
            .child(
                div()
                    .text_lg()
                    .text_color(rgb(TEXT))
                    .child("Live kernel and agent activity"),
            )
            .child(self.event_table(self.events.iter(), "No events received yet."))
    }

    fn page_network(&self) -> impl IntoElement {
        div()
            .flex()
            .flex_col()
            .gap_4()
            .child(
                div()
                    .text_lg()
                    .text_color(rgb(TEXT))
                    .child("Network activity"),
            )
            .child(
                self.event_table(
                    self.events
                        .iter()
                        .filter(|event| !event.endpoint.is_empty()),
                    "No network events in the retained window.",
                ),
            )
    }

    fn page_execution_graph(&self) -> impl IntoElement {
        let mut pids = self
            .events
            .iter()
            .map(|event| event.pid)
            .collect::<Vec<_>>();
        pids.sort_unstable();
        pids.dedup();

        let mut parents = self
            .events
            .iter()
            .filter(|event| event.ppid > 0)
            .map(|event| (event.ppid, event.pid))
            .collect::<Vec<_>>();
        parents.sort_unstable();
        parents.dedup();

        div()
            .flex()
            .flex_col()
            .gap_4()
            .child(
                div()
                    .flex()
                    .flex_row()
                    .gap_3()
                    .child(self.metric_card("Processes", pids.len().to_string(), ACCENT))
                    .child(self.metric_card("Observed edges", parents.len().to_string(), GREEN)),
            )
            .child(
                div()
                    .p_4()
                    .rounded_md()
                    .bg(rgb(SURFACE))
                    .border_1()
                    .border_color(rgb(BORDER))
                    .text_color(rgb(MUTED))
                    .child(
                        "GPUI execution graph data path is live. Spatial graph rendering is the next parity layer; process/parent relations already come from the same event stream.",
                    ),
            )
    }

    fn page_configuration(&self) -> impl IntoElement {
        let token = stored(API_TOKEN_KEY);
        let cluster = stored(CLUSTER_TARGET_KEY);
        div()
            .flex()
            .flex_col()
            .gap_3()
            .child(
                div()
                    .p_4()
                    .rounded_md()
                    .bg(rgb(SURFACE))
                    .border_1()
                    .border_color(rgb(BORDER))
                    .child(
                        div()
                            .text_lg()
                            .text_color(rgb(TEXT))
                            .child("Runtime request context"),
                    )
                    .child(
                        div()
                            .mt_2()
                            .text_color(rgb(MUTED))
                            .child(format!(
                                "API token: {}",
                                if token.is_empty() { "not stored" } else { "stored" }
                            )),
                    )
                    .child(
                        div()
                            .text_color(rgb(MUTED))
                            .child(format!(
                                "Cluster target: {}",
                                if cluster.is_empty() { LOCAL_CLUSTER_TARGET } else { &cluster }
                            )),
                    ),
            )
            .child(
                div()
                    .p_4()
                    .rounded_md()
                    .bg(rgb(SURFACE))
                    .border_1()
                    .border_color(rgb(BORDER))
                    .text_color(rgb(MUTED))
                    .child(
                        "The GPUI client reuses the legacy localStorage keys and sends the same X-API-KEY, Authorization and X-Cluster-Target headers. Runtime mutation controls are being ported endpoint-by-endpoint.",
                    ),
            )
    }

    fn page_hooks(&self) -> impl IntoElement {
        let hooks = [
            "Claude Code",
            "Gemini CLI",
            "Codex",
            "DeepSeek Harness",
            "Pi / Oh My Pi",
            "GitHub Copilot CLI",
            "Kiro",
            "Augment / Auggie",
            "Antigravity",
            "ZCode",
            "MiniMax Code",
            "Cursor",
        ];
        let mut list = div().flex().flex_col().gap_1();
        for hook in hooks {
            list = list.child(
                div()
                    .px_3()
                    .py_2()
                    .rounded_sm()
                    .bg(rgb(SURFACE))
                    .text_color(rgb(TEXT))
                    .child(hook),
            );
        }
        div()
            .flex()
            .flex_col()
            .gap_3()
            .child(
                div()
                    .text_lg()
                    .text_color(rgb(TEXT))
                    .child("Hook integrations"),
            )
            .child(list)
    }

    fn page_recording(&self) -> impl IntoElement {
        let endpoints = [
            "GET /events/recording",
            "POST /events/recording/start",
            "POST /events/recording/stop",
            "POST /events/recording/replay",
            "POST /events/recording/browser/save",
        ];
        let mut list = div().flex().flex_col().gap_1();
        for endpoint in endpoints {
            list = list.child(
                div()
                    .px_3()
                    .py_2()
                    .rounded_sm()
                    .bg(rgb(SURFACE))
                    .text_color(rgb(ACCENT))
                    .child(endpoint),
            );
        }
        div()
            .flex()
            .flex_col()
            .gap_3()
            .child(
                div()
                    .text_lg()
                    .text_color(rgb(TEXT))
                    .child("Recording and replay"),
            )
            .child(list)
            .child(
                div()
                    .text_color(rgb(MUTED))
                    .child("Control widgets will bind to these unchanged backend endpoints."),
            )
    }

    fn page_placeholder(&self, title: &'static str, detail: &'static str) -> impl IntoElement {
        div()
            .flex()
            .flex_col()
            .gap_3()
            .child(div().text_lg().text_color(rgb(TEXT)).child(title))
            .child(
                div()
                    .p_4()
                    .rounded_md()
                    .bg(rgb(SURFACE))
                    .border_1()
                    .border_color(rgb(BORDER))
                    .text_color(rgb(MUTED))
                    .child(detail),
            )
            .child(self.event_table(self.events.iter(), "No events received yet."))
    }

    fn page_content(&self) -> impl IntoElement {
        match self.page {
            Page::Dashboard => self.page_dashboard().into_any_element(),
            Page::Network => self.page_network().into_any_element(),
            Page::ExecutionGraph => self.page_execution_graph().into_any_element(),
            Page::Config => self.page_configuration().into_any_element(),
            Page::Hooks => self.page_hooks().into_any_element(),
            Page::Recording => self.page_recording().into_any_element(),
            Page::Monitor => self
                .page_placeholder(
                    "System monitor",
                    "The GPUI monitor consumes the same event stream now; CPU, memory, interface and sensor WebSocket panels are next.",
                )
                .into_any_element(),
            Page::Observe => self
                .page_placeholder(
                    "Process observe",
                    "PID / trace focused observation is being moved from Vue composables into Rust filters over the shared event model.",
                )
                .into_any_element(),
            Page::Research => self
                .page_placeholder(
                    "Research sessions",
                    "Research REST APIs remain unchanged. Session creation, persisted summaries and experiment controls are being ported to GPUI.",
                )
                .into_any_element(),
            Page::Explorer => self
                .page_placeholder(
                    "Event explorer",
                    "The explorer shares the retained protobuf events and will add Rust-side query/filter controls.",
                )
                .into_any_element(),
            Page::Executor => self
                .page_placeholder(
                    "Executor",
                    "Shell-session and remote-wrapper terminal APIs remain available; terminal rendering is the remaining GPUI parity item.",
                )
                .into_any_element(),
            Page::Ml => self
                .page_placeholder(
                    "Machine learning",
                    "ML status/configuration endpoints remain backend-owned and are being surfaced directly in GPUI.",
                )
                .into_any_element(),
            Page::Plugins => self
                .page_placeholder(
                    "Plugins",
                    "Plugin catalog, enablement and visual builder API compatibility is retained while the controls move to Rust.",
                )
                .into_any_element(),
            Page::Signals => self
                .page_placeholder(
                    "Signals",
                    "Program-signal settings and findings continue to use the existing backend contracts.",
                )
                .into_any_element(),
        }
    }
}

impl Render for Dashboard {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let mut nav = div()
            .flex()
            .flex_col()
            .gap_1()
            .w(px(210.))
            .h_full()
            .p_3()
            .bg(rgb(SIDEBAR))
            .border_color(rgb(BORDER));

        nav = nav.child(
            div()
                .px_2()
                .py_3()
                .text_lg()
                .text_color(rgb(TEXT))
                .child("Agent eBPF Filter"),
        );

        for (index, page) in Page::ALL.into_iter().enumerate() {
            let selected = self.page == page;
            nav = nav.child(
                div()
                    .id(ElementId::NamedInteger("nav".into(), index as u64))
                    .px_3()
                    .py_2()
                    .rounded_md()
                    .cursor_pointer()
                    .when(selected, |item| {
                        item.bg(rgb(SURFACE_ALT)).text_color(rgb(TEXT))
                    })
                    .when(!selected, |item| item.text_color(rgb(MUTED)))
                    .on_click(cx.listener(move |this, _event, _window, cx| {
                        this.select_page(page, cx);
                    }))
                    .child(page.label()),
            );
        }

        let status = div()
            .flex()
            .flex_row()
            .items_center()
            .gap_2()
            .child(
                div()
                    .size(px(9.))
                    .rounded_full()
                    .bg(rgb(self.stream_status.color())),
            )
            .child(div().text_sm().text_color(rgb(MUTED)).child(format!(
                "{} · {} events",
                self.stream_status.label(),
                self.events.len()
            )));

        div()
            .size_full()
            .flex()
            .flex_row()
            .bg(rgb(BG))
            .font_family("Noto Sans Display")
            .child(nav)
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap_4()
                    .h_full()
                    .w_full()
                    .p_4()
                    .child(
                        div()
                            .flex()
                            .flex_row()
                            .items_center()
                            .gap_3()
                            .child(
                                div()
                                    .text_xl()
                                    .text_color(rgb(TEXT))
                                    .child(self.page.label()),
                            )
                            .child(status),
                    )
                    .child(
                        div()
                            .text_sm()
                            .text_color(rgb(MUTED))
                            .child(self.status_message.clone()),
                    )
                    .child(self.page_content()),
            )
    }
}

fn stored(key: &str) -> String {
    web_sys::window()
        .and_then(|window| window.local_storage().ok().flatten())
        .and_then(|storage| storage.get_item(key).ok().flatten())
        .map(|value| value.trim().to_owned())
        .unwrap_or_default()
}

fn browser_path() -> String {
    web_sys::window()
        .and_then(|window| window.location().pathname().ok())
        .unwrap_or_else(|| "/dashboard".into())
}

fn push_route(route: &str) {
    if let Some(window) = web_sys::window() {
        if let Ok(history) = window.history() {
            let _ = history.push_state_with_url(&wasm_bindgen::JsValue::NULL, "", Some(route));
        }
    }
}

fn websocket_url(path: &str) -> String {
    let Some(window) = web_sys::window() else {
        return path.into();
    };
    let location = window.location();
    let protocol = location.protocol().unwrap_or_else(|_| "http:".into());
    let host = location.host().unwrap_or_else(|_| "127.0.0.1:8080".into());
    let scheme = if protocol == "https:" { "wss:" } else { "ws:" };

    let token = stored(API_TOKEN_KEY);
    let cluster = stored(CLUSTER_TARGET_KEY);
    let mut query = Vec::new();
    if !token.is_empty() {
        query.push(format!("key={token}"));
    }
    if !cluster.is_empty() && cluster != LOCAL_CLUSTER_TARGET {
        query.push(format!("cluster={cluster}"));
    }

    if query.is_empty() {
        format!("{scheme}//{host}{path}")
    } else {
        format!("{scheme}//{host}{path}?{}", query.join("&"))
    }
}

fn requested_backend() -> gpui_platform::WebBackendPreference {
    let search = web_sys::window()
        .and_then(|window| window.location().search().ok())
        .unwrap_or_default();
    if search
        .split('&')
        .any(|part| part.contains("backend=webgpu"))
    {
        gpui_platform::WebBackendPreference::WebGpu
    } else if search.split('&').any(|part| part.contains("backend=webgl")) {
        gpui_platform::WebBackendPreference::WebGl
    } else {
        gpui_platform::WebBackendPreference::Auto
    }
}

thread_local! {
    static APPLICATION: RefCell<Option<ApplicationHandle>> = const { RefCell::new(None) };
}

fn start_application(font: Vec<u8>) {
    let handle = gpui_platform::application_with_web_backend(requested_backend()).run_embedded(
        move |cx: &mut App| {
            if let Err(error) = cx.text_system().add_fonts(vec![Cow::Owned(font)]) {
                web_sys::console::error_1(&format!("failed to load GPUI font: {error:#}").into());
                return;
            }
            let bounds = Bounds::centered(None, size(px(1440.), px(900.)), cx);
            if let Err(error) = cx.open_window(
                WindowOptions {
                    window_bounds: Some(WindowBounds::Windowed(bounds)),
                    ..Default::default()
                },
                |_, cx| cx.new(Dashboard::new),
            ) {
                web_sys::console::error_1(&format!("failed to open GPUI window: {error:#}").into());
                return;
            }
            cx.activate(true);
        },
    );
    APPLICATION.with(|cell| *cell.borrow_mut() = Some(handle));
}

async fn load_boot_font() -> Result<Vec<u8>, String> {
    let mut last_error = String::new();
    for url in FONT_URLS {
        match Request::get(url).send().await {
            Ok(response) if response.ok() => match response.binary().await {
                Ok(bytes) if !bytes.is_empty() => return Ok(bytes),
                Ok(_) => last_error = "font response was empty".into(),
                Err(error) => last_error = error.to_string(),
            },
            Ok(response) => last_error = format!("font HTTP {}", response.status()),
            Err(error) => last_error = error.to_string(),
        }
    }
    Err(last_error)
}

fn main() {
    gpui_platform::web_init();
    wasm_bindgen_futures::spawn_local(async {
        match load_boot_font().await {
            Ok(font) => start_application(font),
            Err(error) => {
                web_sys::console::error_1(&format!("GPUI boot font failed: {error}").into());
            }
        }
    });
}
