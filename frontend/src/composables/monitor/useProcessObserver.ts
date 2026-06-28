import { ref, computed, watch, onUnmounted } from "vue";
import axios from "axios";
import { buildWebSocketUrl } from "../../utils/requestContext";
import { pb } from "../../pb/tracker_pb.js";

// ── Types ───────────────────────────────────────────────────────────────────

export interface ProcessInfo {
  pid: number;
  ppid: number;
  name: string;
  cpu: number;
  mem: number;
  user: string;
  gpuMem: number;
  gpuId: number;
  gpuUtil: number;
  cmdline: string;
  createTime: number;
  minorFaults: number;
  majorFaults: number;
}

export interface ObserverEvent {
  key: string;
  pid: number;
  ppid: number;
  type: string;
  eventType: number;
  tag: string;
  comm: string;
  path: string;
  extraInfo: string;
  bytes: number;
  retval: number;
  time: string;
  timestamp: number;
}

export interface ObserverTLSEvent {
  key: string;
  timestamp: string;
  pid: number;
  tgid: number;
  comm: string;
  direction: string;
  lib: string;
  function: string;
  captured_len: number;
  original_len: number;
  type: string;
  method: string;
  url: string;
  host: string;
  status: number;
  raw_hex_dump?: string;
  raw_available?: boolean;
  body_size?: number;
  truncated?: boolean;
  // Decrypted context (auto-context decryption)
  body?: string;
  headers?: Record<string, string>;
  content_type?: string;
  redaction_state?: string;
  // Agent context
  vendor?: string;
  message_role?: string;
  prompt_digest?: string;
  agent_run_id?: string;
  task_id?: string;
  tool_call_id?: string;
  tool_name?: string;
  // SSE
  sse_event?: string;
  sse_data_digest?: string;
  sse_data_count?: number;
}

export interface ProcessTreeNode {
  pid: number;
  ppid: number;
  name: string;
  cpu: number;
  mem: number;
  user: string;
  cmdline: string;
  children: ProcessTreeNode[];
  dead: boolean;
  createTime: number;
}

// ── Timeline ignore rule types ─────────────────────────────────────────
export interface EventIgnoreRule {
  id: string;
  name: string;
  enabled: boolean;
  types?: string[];
  comms?: string[];
  tags?: string[];
  pathPrefixes?: string[];
  categoryOverride?: string;
  description?: string;
}

export const IGNORE_RULES_KEY = "agent-ebpf.timeline.ignoreRules";

export const loadIgnoreRulesFromStorage = (): EventIgnoreRule[] => {
  try {
    const raw = localStorage.getItem(IGNORE_RULES_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
};

export const saveIgnoreRulesToStorage = (rules: EventIgnoreRule[]) => {
  try {
    localStorage.setItem(IGNORE_RULES_KEY, JSON.stringify(rules));
  } catch {
    localStorage.removeItem(IGNORE_RULES_KEY);
  }
};

export const DEFAULT_IGNORE_RULES: EventIgnoreRule[] = [
  { id: "ignore-wrapper-intercept", name: "Wrapper Intercept", enabled: true, types: ["wrapper_intercept"], description: "Hide wrapper_intercept noise" },
  { id: "ignore-semantic-alert", name: "Semantic Alerts", enabled: true, types: ["semantic_alert"], description: "Hide semantic loop alerts" },
  { id: "ignore-stdio", name: "Stdio Events", enabled: true, types: ["stdio", "read", "write"], tags: ["Stdio"], description: "Low-signal stdio read/write" },
  { id: "ignore-system-metrics", name: "System Metrics", enabled: true, types: ["system_metric"], description: "Periodic system metric snapshots" },
  { id: "ignore-tcp-close", name: "TCP Close Events", enabled: true, types: ["tcp_close"], description: "High-frequency TCP teardown" },
  { id: "ignore-agent-sight", name: "AgentSight Alerts", enabled: true, types: ["agentsight_alert"], description: "AgentSight diagnosis events" },
  { id: "ignore-accept", name: "Accept / Accept4", enabled: true, types: ["accept", "accept4"], description: "TCP accept noise" },
];

export function isEventIgnored(event: ObserverEvent, rules: EventIgnoreRule[]): boolean {
  for (const rule of rules) {
    if (!rule.enabled) continue;
    if (rule.types && rule.types.some((t) => event.type?.toLowerCase().includes(t.toLowerCase()))) return true;
    if (rule.comms && rule.comms.some((c) => event.comm?.toLowerCase().includes(c.toLowerCase()))) return true;
    if (rule.tags && rule.tags.some((t) => event.tag?.toLowerCase().includes(t.toLowerCase()))) return true;
    if (rule.pathPrefixes && rule.pathPrefixes.some((p) => event.path?.toLowerCase().startsWith(p.toLowerCase()))) return true;
  }
  return false;
}

export interface NetworkFlow {
  flowId: string;
  protocol: string;
  srcIp: string;
  dstIp: string;
  dstPort: number;
  dstService: string;
  processPids: number[];
  processComms: string[];
  bytesIn: number;
  bytesOut: number;
  riskScore: number;
}

export interface TCPConnection {
  pid: number;
  comm: string;
  state: string;
  srcIp: string;
  dstIp: string;
  srcPort: number;
  dstPort: number;
}

// ── Constants ───────────────────────────────────────────────────────────────

// ── Scrollback caps (persisted, 0 = unlimited) ──────────────────────────────
const STORAGE_KEY_TLS_CAP = "observe-tls-cap";
const STORAGE_KEY_EVENT_CAP = "observe-event-cap";

const readStoredCap = (key: string, fallback: number): number => {
  try {
    const v = localStorage.getItem(key);
    if (v === null) return fallback;
    const n = parseInt(v, 10);
    return Number.isFinite(n) && n >= 0 ? n : fallback;
  } catch { return fallback; }
};

const writeStoredCap = (key: string, value: number): void => {
  try { localStorage.setItem(key, String(value)); } catch { /* ignore */ }
};

const maxTLS = ref(readStoredCap(STORAGE_KEY_TLS_CAP, 50_000));
const maxEvents = ref(readStoredCap(STORAGE_KEY_EVENT_CAP, 100_000));
const MAX_FLOWS = 5_000;

// capSlice: slice to cap, 0 = unlimited (no slicing)
const capSlice = <T>(arr: T[], cap: number): T[] =>
  cap > 0 ? arr.slice(0, cap) : arr;

// Batching: flush events in batches to avoid excessive reactive updates
let pendingEvents: ObserverEvent[] = [];
let pendingTLSEvents: ObserverTLSEvent[] = [];
let eventFlushTimer: ReturnType<typeof setTimeout> | null = null;
let tlsFlushTimer: ReturnType<typeof setTimeout> | null = null;
const FLUSH_INTERVAL = 200; // ms

let _counter = 0;
const nextKey = () => _counter++;

// Network event types
const NETWORK_EVENT_TYPES = new Set<number>([
  pb.EventType.NETWORK_CONNECT,   // 2
  pb.EventType.NETWORK_BIND,      // 6
  pb.EventType.NETWORK_SENDTO,    // 7
  pb.EventType.NETWORK_RECVFROM,  // 8
  pb.EventType.SOCKET,            // 20
  pb.EventType.ACCEPT,            // 21
  pb.EventType.ACCEPT4,           // 22
  pb.EventType.TCP_CONNECT,       // 31
  pb.EventType.TCP_CLOSE,         // 32
  pb.EventType.TCP_STATE_CHANGE,  // 33
  pb.EventType.DNS_QUERY,         // 34
]);

// Syscall / execution event types
const SYSCALL_EVENT_TYPES = new Set<number>([
  pb.EventType.EXECVE,            // 0
  pb.EventType.IOCTL,             // 5
  pb.EventType.GENERIC_SYSCALL,   // 25
  pb.EventType.CLONE,             // 18
  pb.EventType.EXIT,              // 19
  pb.EventType.SCHED_PROCESS_FORK,   // 26
  pb.EventType.SCHED_PROCESS_EXEC,   // 27
  pb.EventType.SCHED_PROCESS_EXIT,   // 28
  pb.EventType.WAIT4,             // 29
]);

// File access event types
const FILE_EVENT_TYPES = new Set<number>([
  pb.EventType.OPENAT,            // 1
  pb.EventType.OPEN,              // 11
  pb.EventType.READ,              // 9
  pb.EventType.WRITE,             // 10
  pb.EventType.CHMOD,             // 12
  pb.EventType.CHOWN,             // 13
  pb.EventType.RENAME,            // 14
  pb.EventType.LINK,              // 15
  pb.EventType.SYMLINK,           // 16
  pb.EventType.MKDIR,             // 3
  pb.EventType.UNLINK,            // 4
  pb.EventType.MKNOD,             // 17
]);

// ── Helpers ─────────────────────────────────────────────────────────────────

// Reverse map: type name string → EventType number (for fallback)
const TYPE_NAME_MAP: Record<string, number> = {
  execve: 0, openat: 1, network_connect: 2, mkdir: 3, unlink: 4,
  ioctl: 5, network_bind: 6, network_sendto: 7, network_recvfrom: 8,
  read: 9, write: 10, open: 11, chmod: 12, chown: 13, rename: 14,
  link: 15, symlink: 16, mknod: 17, clone: 18, exit: 19,
  socket: 20, accept: 21, accept4: 22, syscall: 25,
  process_fork: 26, process_exec: 27, process_exit: 28, wait4: 29,
  tcp_connect: 31, tcp_close: 32, tcp_state_change: 33, dns_query: 34,
};

const extractEventType = (event: pb.IEvent): number | undefined => {
  // Path 1: explicit eventType field (binary proto → number, JSON → string name)
  if (
    Object.prototype.hasOwnProperty.call(event, "eventType") &&
    event.eventType !== null &&
    event.eventType !== undefined
  ) {
    const val = event.eventType as any;
    // Binary protobuf: numeric enum value (e.g., 0, 1, 2...)
    if (typeof val === "number") return Number.isFinite(val) ? val : undefined;
    // JSON: string enum name (e.g., "EXECVE", "OPENAT"...) — map to number
    if (typeof val === "string") {
      const num = (pb.EventType as any)[val];
      if (typeof num === "number") return num;
      const parsed = parseInt(val, 10);
      if (!isNaN(parsed)) return parsed;
    }
  }
  // Path 2: fallback via type string field (handles proto3 default-value omission
  // where eventType=0/EXECVE is not serialised, or when event_type is unset)
  if (event.type && typeof event.type === "string") {
    const t = event.type.toLowerCase();
    const mapped = TYPE_NAME_MAP[t];
    if (typeof mapped === "number") return mapped;
    // Also try via pb.EventType reverse lookup (e.g. "EXECVE" → 0)
    const rev = (pb.EventType as any)[t.toUpperCase()] ?? (pb.EventType as any)[t];
    if (typeof rev === "number") return rev;
  }
  return undefined;
};

// ── Main composable ─────────────────────────────────────────────────────────

export function useProcessObserver() {
  // ---- Selection state (multi-select) ----
  const selectedPids = ref<Set<number>>(new Set());
  const showPicker = ref(false);

  const addPid = (pid: number) => {
    if (pid <= 0) return;
    selectedPids.value = new Set([...selectedPids.value, pid]);
  };
  const removePid = (pid: number) => {
    const s = new Set(selectedPids.value);
    s.delete(pid);
    selectedPids.value = s;
  };
  const togglePid = (pid: number) => {
    if (pid <= 0) return;
    const s = new Set(selectedPids.value);
    if (s.has(pid)) s.delete(pid);
    else s.add(pid);
    selectedPids.value = s;
  };
  const clearPids = () => { selectedPids.value = new Set(); };
  const hasPid = (pid: number): boolean => selectedPids.value.has(pid);

  // ---- Input: processes fed from parent ----
  const processes = ref<ProcessInfo[]>([]);

  // ---- Event buffers ----
  const allEvents = ref<ObserverEvent[]>([]);
  const tlsEvents = ref<ObserverTLSEvent[]>([]);
  const networkFlows = ref<NetworkFlow[]>([]);
  const tcpConns = ref<TCPConnection[]>([]);

  // ---- WebSocket state ----
  let eventWs: WebSocket | null = null;
  let tlsWs: WebSocket | null = null;
  let shouldReconnect = true;
  let eventReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let tlsReconnectTimer: ReturnType<typeof setTimeout> | null = null;

  // ── Process tree ────────────────────────────────────────────────────────

  /** Retain snapshots of all PIDs ever seen so dead processes stay in the tree (grayed out). */
  const staleProcessMap = new Map<number, ProcessInfo>();

  /** Maps ppid → children, built from live + stale process data */
  const processTree = computed<ProcessTreeNode[]>(() => {
    const map = new Map<number, ProcessTreeNode>();
    const roots: ProcessTreeNode[] = [];
    const livePids = new Set(processes.value.map((p) => p.pid));

    // Seed from stale map (includes live processes, which get overwritten below)
    for (const p of staleProcessMap.values()) {
      map.set(p.pid, {
        pid: p.pid,
        ppid: p.ppid,
        name: p.name,
        cpu: livePids.has(p.pid) ? (p.cpu ?? 0) : 0,
        mem: livePids.has(p.pid) ? (p.mem ?? 0) : 0,
        user: p.user,
        cmdline: p.cmdline,
        children: [],
        dead: !livePids.has(p.pid),
        createTime: p.createTime || 0,
      });
    }

    // Refresh live processes with current data (overwrites stale entry)
    for (const p of processes.value) {
      map.set(p.pid, {
        pid: p.pid,
        ppid: p.ppid,
        name: p.name,
        cpu: p.cpu ?? 0,
        mem: p.mem ?? 0,
        user: p.user,
        cmdline: p.cmdline,
        children: [], // rebuilt below
        dead: false,
        createTime: p.createTime || 0,
      });
    }

    // Wire up parent-child relationships
    for (const node of map.values()) {
      if (node.ppid && node.ppid !== node.pid && map.has(node.ppid)) {
        map.get(node.ppid)!.children.push(node);
      } else {
        roots.push(node);
      }
    }
    return roots;
  });

  /** Find the subtrees rooted at each selected PID */
  const selectedProcessTree = computed<ProcessTreeNode[]>(() => {
    if (selectedPids.value.size === 0) return [];
    const findSubtree = (
      nodes: ProcessTreeNode[],
      targetPid: number,
    ): ProcessTreeNode | null => {
      for (const node of nodes) {
        if (node.pid === targetPid) return node;
        const found = findSubtree(node.children, targetPid);
        if (found) return found;
      }
      return null;
    };
    const result: ProcessTreeNode[] = [];
    for (const pid of selectedPids.value) {
      const subtree = findSubtree(processTree.value, pid);
      if (subtree) result.push(subtree);
    }
    return result;
  });

  /** Recursively collect all PIDs in a list of tree nodes */
  const collectTreePids = (nodes: ProcessTreeNode[]): number[] =>
    nodes.flatMap((n) => [n.pid, ...collectTreePids(n.children)]);

  /** Set of all PIDs in the selected process subtree */
  const treePids = computed<Set<number>>(
    () => new Set(collectTreePids(selectedProcessTree.value)),
  );

  /** Flat list of all processes in the selected tree (live + dead) */
  const treeProcessList = computed<ProcessInfo[]>(() => {
    const result: ProcessInfo[] = [];
    const livePids = new Set(processes.value.map((p) => p.pid));
    for (const pid of treePids.value) {
      const live = processes.value.find((p) => p.pid === pid);
      if (live) {
        result.push(live);
      } else {
        const stale = staleProcessMap.get(pid);
        if (stale) result.push(stale);
      }
    }
    return result;
  });

  // ── Filtered event views ─────────────────────────────────────────────────

  const treeNetworkEvents = computed<ObserverEvent[]>(() =>
    allEvents.value.filter(
      (e) => treePids.value.has(e.pid) && NETWORK_EVENT_TYPES.has(e.eventType),
    ),
  );

  const treeSyscallEvents = computed<ObserverEvent[]>(() =>
    allEvents.value.filter(
      (e) => treePids.value.has(e.pid) && SYSCALL_EVENT_TYPES.has(e.eventType),
    ),
  );

  const treeFileAccessEvents = computed<ObserverEvent[]>(() =>
    allEvents.value.filter(
      (e) => treePids.value.has(e.pid) && FILE_EVENT_TYPES.has(e.eventType),
    ),
  );

  const treeTLSEvents = computed<ObserverTLSEvent[]>(() =>
    tlsEvents.value.filter((e) => treePids.value.has(e.pid)),
  );

  const treeNetworkFlows = computed<NetworkFlow[]>(() =>
    networkFlows.value.filter((f) =>
      f.processPids?.some((pid) => treePids.value.has(pid)),
    ),
  );

  const treeTCPConns = computed<TCPConnection[]>(() =>
    tcpConns.value.filter((c) => treePids.value.has(c.pid)),
  );

  // ── Event WebSocket (/ws — protobuf eBPF events) ─────────────────────────

  const connectEventWS = () => {
    if (!shouldReconnect) return;
    if (eventWs) eventWs.close();
    const socket = new WebSocket(buildWebSocketUrl("/ws"));
    eventWs = socket;
    socket.binaryType = "arraybuffer";

    const flushEvents = () => {
      if (pendingEvents.length === 0) return;
      allEvents.value = capSlice([...pendingEvents.reverse(), ...allEvents.value], maxEvents.value);
      pendingEvents = [];
    };

    socket.onmessage = (me) => {
      try {
        const raw = new Uint8Array(me.data);
        const incoming: pb.IEvent[] =
          raw[0] === 0x0a
            ? pb.EventBatch.decode(raw).events || []
            : [pb.Event.decode(raw)];

        const now = Date.now();
        for (const d of incoming) {
          const et = extractEventType(d);
          if (et === undefined) continue;

          // ── Auto-navigate to observe page when backend sends OBSERVE_NAVIGATE ──
          if (et === pb.EventType.OBSERVE_NAVIGATE && d.pid) {
            try {
              localStorage.setItem("observe-selected-pid", String(d.pid));
            } catch { /* ignore */ }
            if (router.currentRoute.value.name !== "Observe") {
              router.push({ name: "Observe", query: { pid: String(d.pid) } });
            } else {
              selectedPid.value = d.pid;
            }
            continue; // don't display as a regular event
          }

          pendingEvents.push({
            key: `ev-${Date.now()}-${nextKey()}`,
            pid: d.pid || 0,
            ppid: d.ppid || 0,
            type: d.type || "",
            eventType: et,
            tag: d.tag || "",
            comm: d.comm || "",
            path: d.path || "",
            extraInfo: d.extraInfo || "",
            bytes: Number(d.bytes || 0),
            retval: Number(d.retval || 0),
            time: new Date(now).toLocaleTimeString(),
            timestamp: now,
          });
        }
        if (!eventFlushTimer) {
          eventFlushTimer = setTimeout(() => {
            eventFlushTimer = null;
            flushEvents();
          }, FLUSH_INTERVAL);
        }
      } catch {
        /* skip malformed message */
      }
    };

    socket.onclose = () => {
      if (shouldReconnect)
        eventReconnectTimer = setTimeout(connectEventWS, 3000);
    };
  };

  // ── TLS WebSocket (/ws/tls-capture — JSON) ───────────────────────────────

  const connectTLSSocket = () => {
    if (!shouldReconnect) return;
    if (tlsWs) tlsWs.close();
    const socket = new WebSocket(buildWebSocketUrl("/ws/tls-capture"));
    tlsWs = socket;

    const flushTLSEvents = () => {
      if (pendingTLSEvents.length === 0) return;
      tlsEvents.value = capSlice([...pendingTLSEvents.reverse(), ...tlsEvents.value], maxTLS.value);
      pendingTLSEvents = [];
    };

    socket.onmessage = (event) => {
      try {
        const payload = JSON.parse(String(event.data));
        const items: any[] = Array.isArray(payload)
          ? payload
          : payload.events
            ? payload.events
            : [payload];
        for (const ev of items) {
          pendingTLSEvents.push({
            key: `tls-${Date.now()}-${nextKey()}`,
            timestamp: ev.timestamp || new Date().toISOString(),
            pid: ev.pid || ev.tgid || 0,
            tgid: ev.tgid || ev.pid || 0,
            comm: ev.comm || "",
            direction: ev.direction || "",
            lib: ev.lib || "",
            function: ev.function || "",
            captured_len: ev.captured_len || 0,
            original_len: ev.original_len || 0,
            type: ev.type || "",
            method: ev.method || "",
            url: ev.url || "",
            host: ev.host || "",
            status: ev.status || 0,
            raw_hex_dump: ev.raw_hex_dump || "",
            raw_available: ev.raw_available || false,
            body_size: ev.body_size || 0,
            truncated: ev.truncated || false,
            // Decrypted context
            body: ev.body || "",
            headers: ev.headers || undefined,
            content_type: ev.content_type || ev.contentType || "",
            redaction_state: ev.redaction_state || ev.redactionState || "",
            // Agent context
            vendor: ev.vendor || "",
            message_role: ev.message_role || ev.messageRole || "",
            prompt_digest: ev.prompt_digest || ev.promptDigest || "",
            agent_run_id: ev.agent_run_id || ev.agentRunId || "",
            task_id: ev.task_id || ev.taskId || "",
            tool_call_id: ev.tool_call_id || ev.toolCallId || "",
            tool_name: ev.tool_name || ev.toolName || "",
            // SSE
            sse_event: ev.sse_event || ev.sseEvent || "",
            sse_data_digest: ev.sse_data_digest || ev.sseDataDigest || "",
            sse_data_count: ev.sse_data_count || ev.sseDataCount || 0,
          });
        }
        if (!tlsFlushTimer) {
          tlsFlushTimer = setTimeout(() => {
            tlsFlushTimer = null;
            flushTLSEvents();
          }, FLUSH_INTERVAL);
        }
      } catch {
        /* skip malformed message */
      }
    };

    socket.onclose = () => {
      if (shouldReconnect)
        tlsReconnectTimer = setTimeout(connectTLSSocket, 3000);
    };
  };

  // ── HTTP fetchers ────────────────────────────────────────────────────────

  const fetchRecentEvents = async () => {
    try {
      const res = await axios.get("/events/recent?limit=400");
      const loaded: ObserverEvent[] = (res.data.events || [])
        .map((r: any) => {
          const d = r.Event || r;
          const et = extractEventType(d);
          return {
            key: `ev-init-${Date.now()}-${nextKey()}`,
            pid: d.pid || 0,
            ppid: d.ppid || 0,
            type: d.type || "",
            eventType: et ?? -1,
            tag: d.tag || "",
            comm: d.comm || "",
            path: d.path || "",
            extraInfo: d.extraInfo || "",
            bytes: Number(d.bytes || 0),
            retval: Number(d.retval || 0),
            time: new Date(r.Timestamp || Date.now()).toLocaleTimeString(),
            timestamp: r.Timestamp || Date.now(),
          };
        })
        .filter((e: ObserverEvent) => e.eventType >= 0);
      allEvents.value = loaded.reverse();
    } catch {
      /* silent */
    }
  };

  const fetchRecentTLSEvents = async () => {
    try {
      const res = await axios.get("/tls-capture/recent?limit=300");
      const items: any[] =
        res.data?.events || res.data?.events || [];
      tlsEvents.value = items
        .map((ev: any) => ({
          key: `tls-init-${Date.now()}-${nextKey()}`,
          timestamp: ev.timestamp || new Date().toISOString(),
          pid: ev.pid || ev.tgid || 0,
          tgid: ev.tgid || ev.pid || 0,
          comm: ev.comm || "",
          direction: ev.direction || "",
          lib: ev.lib || "",
          function: ev.function || "",
          captured_len: ev.captured_len || 0,
          original_len: ev.original_len || 0,
          type: ev.type || "",
          method: ev.method || "",
          url: ev.url || "",
          host: ev.host || "",
          status: ev.status || 0,
          raw_hex_dump: ev.raw_hex_dump || "",
          raw_available: ev.raw_available || false,
          body_size: ev.body_size || 0,
          truncated: ev.truncated || false,
          // Decrypted context
          body: ev.body || "",
          headers: ev.headers || undefined,
          content_type: ev.content_type || ev.contentType || "",
          redaction_state: ev.redaction_state || ev.redactionState || "",
          // Agent context
          vendor: ev.vendor || "",
          message_role: ev.message_role || ev.messageRole || "",
          prompt_digest: ev.prompt_digest || ev.promptDigest || "",
          agent_run_id: ev.agent_run_id || ev.agentRunId || "",
          task_id: ev.task_id || ev.taskId || "",
          tool_call_id: ev.tool_call_id || ev.toolCallId || "",
          tool_name: ev.tool_name || ev.toolName || "",
          // SSE
          sse_event: ev.sse_event || ev.sseEvent || "",
          sse_data_digest: ev.sse_data_digest || ev.sseDataDigest || "",
          sse_data_count: ev.sse_data_count || ev.sseDataCount || 0,
        } as ObserverTLSEvent))
        .slice(0, maxTLS.value || 50000);
    } catch {
      /* silent */
    }
  };

  const fetchNetworkFlows = async () => {
    try {
      const params: Record<string, any> = {};
      if (selectedPids.value.size > 0) {
        params.pids = [...selectedPids.value].join(",");
      }
      const res = await axios.get("/network/flows", { params });
      networkFlows.value = (res.data.flows || []).slice(0, MAX_FLOWS);
    } catch {
      /* silent */
    }
  };

  const fetchTCPState = async () => {
    try {
      const res = await axios.get("/network/tcp-state");
      tcpConns.value = (res.data.connections || []).slice(0, MAX_FLOWS);
    } catch {
      /* silent */
    }
  };


  const clearEvents = () => { allEvents.value = []; };
  const clearTLSEvents = () => { tlsEvents.value = []; };
  const clearNetworkFlows = () => { networkFlows.value = []; };
  const clearTCPConns = () => { tcpConns.value = []; };

  // SSL attachment state
  interface AttachedPID { pid: number; binary_path: string; library_name: string; }
  const attachedPIDs = ref<AttachedPID[]>([]);
  const fetchAttachedPIDs = async () => {
    try {
      const res = await axios.get("/tls-capture/attached-pids");
      attachedPIDs.value = Array.isArray(res.data) ? res.data : [];
    } catch { attachedPIDs.value = []; }
  };

  // ── Lifecycle ────────────────────────────────────────────────────────────

  const connectAll = () => {
    shouldReconnect = true;
    connectEventWS();
    connectTLSSocket();
  };

  const disconnectAll = () => {
    shouldReconnect = false;
    if (eventWs) {
      eventWs.close();
      eventWs = null;
    }
    if (tlsWs) {
      tlsWs.close();
      tlsWs = null;
    }
    if (eventReconnectTimer) {
      clearTimeout(eventReconnectTimer);
      eventReconnectTimer = null;
    }
    if (tlsReconnectTimer) {
      clearTimeout(tlsReconnectTimer);
      tlsReconnectTimer = null;
    }
  };

  const loadAllInitial = () => {
    fetchRecentEvents();
    fetchRecentTLSEvents();
    fetchNetworkFlows();
    fetchTCPState();
  };

  // Refresh network data when selected PIDs change
  watch(selectedPids, (pids) => {
    if (pids.size > 0) {
      fetchNetworkFlows();
      fetchTCPState();
    }
  }, { deep: true });

  // ── Timeline ignore rules ──────────────────────────────────────────────
  const ignoreRules = ref<EventIgnoreRule[]>(loadIgnoreRulesFromStorage());
  const ignoreRulesInitialized = ref(false);

  if (!ignoreRulesInitialized.value) {
    ignoreRulesInitialized.value = true;
    if (ignoreRules.value.length === 0) {
      ignoreRules.value = [...DEFAULT_IGNORE_RULES];
      saveIgnoreRulesToStorage(ignoreRules.value);
    }
  }

  const addTimelineIgnoreRule = (rule: EventIgnoreRule) => {
    const updated = [...ignoreRules.value, rule];
    ignoreRules.value = updated;
    saveIgnoreRulesToStorage(updated);
  };

  const removeTimelineIgnoreRule = (id: string) => {
    const updated = ignoreRules.value.filter((r) => r.id !== id);
    ignoreRules.value = updated;
    saveIgnoreRulesToStorage(updated);
  };

  const toggleTimelineIgnoreRule = (id: string) => {
    const updated = ignoreRules.value.map((r) =>
      r.id === id ? { ...r, enabled: !r.enabled } : r,
    );
    ignoreRules.value = updated;
    saveIgnoreRulesToStorage(updated);
  };

  const resetTimelineIgnoreRules = () => {
    const updated = DEFAULT_IGNORE_RULES.map((r) => ({ ...r }));
    ignoreRules.value = updated;
    saveIgnoreRulesToStorage(updated);
  };

  // ── Public API ───────────────────────────────────────────────────────────

  return {
    // State
    selectedPids,
    showPicker,
    // Multi-select helpers
    addPid,
    removePid,
    togglePid,
    clearPids,
    hasPid,
    // Tree
    processTree,
    selectedProcessTree,
    treePids,
    treeProcessList,
    // Filtered views
    treeNetworkEvents,
    treeSyscallEvents,
    treeFileAccessEvents,
    treeNetworkFlows,
    treeTCPConns,
    treeTLSEvents,
    // Raw data
    allEvents,
    tlsEvents,
    networkFlows,
    tcpConns,
    // Methods
    setProcesses: (p: ProcessInfo[]) => {
      // Upsert into stale map so dead processes persist in the tree
      for (const proc of p) {
        staleProcessMap.set(proc.pid, { ...proc });
      }
      // Also remember PIDs that exist in the stale map but are NOT in the incoming list
      // (they stay in staleProcessMap as-is so the tree can show them grayed out)
      processes.value = p;
    },
    connectAll,
    disconnectAll,
    loadAllInitial,
    fetchNetworkFlows,
    fetchTCPState,
    // Clear functions
    clearEvents,
    clearTLSEvents,
    clearNetworkFlows,
    clearTCPConns,
    // SSL attachment
    attachedPIDs,
    fetchAttachedPIDs,
    // Timeline ignore rules
    ignoreRules,
    addTimelineIgnoreRule,
    removeTimelineIgnoreRule,
    toggleTimelineIgnoreRule,
    resetTimelineIgnoreRules,
    isEventIgnored,
    // Scrollback caps (0 = unlimited)
    maxTLS,
    maxEvents,
    setTLSCap: (n: number) => {
      maxTLS.value = n;
      writeStoredCap(STORAGE_KEY_TLS_CAP, n);
    },
    setEventCap: (n: number) => {
      maxEvents.value = n;
      writeStoredCap(STORAGE_KEY_EVENT_CAP, n);
    },
  };
}
