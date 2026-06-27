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
  method: string;
  url: string;
  host: string;
  status: number;
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

const MAX_EVENTS = 10000;
const MAX_FLOWS = 500;
const MAX_TLS = 2000;

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

const extractEventType = (event: pb.IEvent): number | undefined => {
  if (
    Object.prototype.hasOwnProperty.call(event, "eventType") &&
    event.eventType !== null &&
    event.eventType !== undefined
  ) {
    return Number(event.eventType);
  }
  return undefined;
};

// ── Main composable ─────────────────────────────────────────────────────────

export function useProcessObserver() {
  // ---- Selection state ----
  const selectedPid = ref<number | null>(null);
  const showPicker = ref(false);

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

  /** Maps ppid → children, built once from process list */
  const processTree = computed<ProcessTreeNode[]>(() => {
    const map = new Map<number, ProcessTreeNode>();
    const roots: ProcessTreeNode[] = [];

    for (const p of processes.value) {
      map.set(p.pid, {
        pid: p.pid,
        ppid: p.ppid,
        name: p.name,
        cpu: p.cpu ?? 0,
        mem: p.mem ?? 0,
        user: p.user,
        cmdline: p.cmdline,
        children: [],
      });
    }

    for (const node of map.values()) {
      if (node.ppid && node.ppid !== node.pid && map.has(node.ppid)) {
        map.get(node.ppid)!.children.push(node);
      } else {
        roots.push(node);
      }
    }
    return roots;
  });

  /** Find the subtree rooted at selectedPid */
  const selectedProcessTree = computed<ProcessTreeNode[]>(() => {
    if (selectedPid.value === null) return [];
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
    const subtree = findSubtree(processTree.value, selectedPid.value);
    return subtree ? [subtree] : [];
  });

  /** Recursively collect all PIDs in a list of tree nodes */
  const collectTreePids = (nodes: ProcessTreeNode[]): number[] =>
    nodes.flatMap((n) => [n.pid, ...collectTreePids(n.children)]);

  /** Set of all PIDs in the selected process subtree */
  const treePids = computed<Set<number>>(
    () => new Set(collectTreePids(selectedProcessTree.value)),
  );

  /** Flat list of all processes in the selected tree */
  const treeProcessList = computed<ProcessInfo[]>(() =>
    processes.value.filter((p) => treePids.value.has(p.pid)),
  );

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
      allEvents.value = [...pendingEvents.reverse(), ...allEvents.value].slice(0, MAX_EVENTS);
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
      tlsEvents.value = [...pendingTLSEvents.reverse(), ...tlsEvents.value].slice(0, MAX_TLS);
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
            method: ev.method || "",
            url: ev.url || "",
            host: ev.host || "",
            status: ev.status || 0,
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
          method: ev.method || "",
          url: ev.url || "",
          host: ev.host || "",
          status: ev.status || 0,
        } as ObserverTLSEvent))
        .slice(0, MAX_TLS);
    } catch {
      /* silent */
    }
  };

  const fetchNetworkFlows = async () => {
    try {
      const params: Record<string, number> = {};
      if (selectedPid.value !== null) params.pid = selectedPid.value;
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

  // Refresh network data when selected PID changes
  watch(selectedPid, (pid) => {
    if (pid !== null) {
      fetchNetworkFlows();
      fetchTCPState();
    }
  });

  // ── Public API ───────────────────────────────────────────────────────────

  return {
    // State
    selectedPid,
    showPicker,
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
  };
}
