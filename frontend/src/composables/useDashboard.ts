import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import axios from 'axios';
import { message } from 'ant-design-vue';

import { pb } from '../pb/tracker_pb.js';
import { canPreviewEventPath, type FilePreviewResponse } from '../types/filePreview';
import { buildWebSocketUrl, fetchProto } from '../utils/requestContext';

interface AgentEvent {
  key: string;
  pid: number;
  ppid: number;
  uid: number;
  type: string;
  eventType?: number;
  tag: string;
  comm: string;
  path: string;
  netDirection?: string;
  netEndpoint?: string;
  netFamily?: string;
  netBytes?: number;
  retval?: number;
  extraInfo?: string;
  extraPath?: string;
  bytes?: number;
  mode?: string;
  domain?: string;
  sockType?: string;
  protocol?: number;
  uidArg?: number;
  gidArg?: number;
  time: string;
  receivedAtMs?: number;
  occurrenceCount?: number;
}

type DisplayedAgentEvent = AgentEvent & {
  mergeSignature?: string;
  lastReceivedAtMs?: number;
};

interface BuiltinFilterRule {
  id: string;
  label: string;
  test: (event: AgentEvent) => boolean;
}

type BuiltinFilterState = Record<string, boolean>;

type ResizableColumnKey = 'time' | 'tag' | 'pid' | 'comm' | 'type' | 'path' | 'action';

export function useDashboard() {

const events = ref<AgentEvent[]>([]);
const isConnected = ref(false);
const isPaused = ref(false);
const showDetails = ref(false);
const selectedEvent = ref<AgentEvent | null>(null);
const showPreview = ref(false);
const previewLoading = ref(false);
const previewData = ref<FilePreviewResponse | null>(null);
const selectedTags = ref<string[]>([]);
const selectedTypes = ref<number[]>([]);
const timeFilter = ref('');
const pidFilter = ref('');
const commandFilter = ref('');
const pathFilter = ref('');
const isDeduplicated = ref(false);
const hideUnknown = ref(true);
const activeHeaderFilter = ref<string | null>(null);
const tags = ref<string[]>([]);
const currentPage = ref(1);
const pageSize = ref(20);
const tableWrapperRef = ref<HTMLElement | null>(null);
const tableContentWidth = ref(0);
const router = useRouter();
const route = useRoute();
let ws: WebSocket | null = null;
let reconnectTimer: number | null = null;
let shouldReconnect = true;
let resizeObserver: ResizeObserver | null = null;
let cleanupColumnResize: (() => void) | null = null;
let recentRowTimer: number | null = null;
const eventBuffer: AgentEvent[] = [];
let flushTimer: number | null = null;
const EVENT_BATCH_WINDOW_MS = 80;
const EVENT_MERGE_WINDOW_MS = 5000;
const HISTORY_LOAD_LIMIT = 200;
const HISTORY_LOAD_BATCH_SIZE = 24;
const HISTORY_LOAD_BATCH_DELAY_MS = 24;
let historyLoadTimer: number | null = null;
let historyLoadToken = 0;
const pendingLiveEvents: AgentEvent[] = [];
const historyLoaded = ref(false);

const maxEvents = ref(5000);
const maxEventsOptions = ['2000', '5000', '10000', '20000', '50000'];

const flushEventBuffer = () => {
  if (eventBuffer.length === 0) return;

  const bufferedEvents = [...eventBuffer];
  const newEvents = [...bufferedEvents.reverse(), ...events.value];
  if (newEvents.length > maxEvents.value) {
    newEvents.length = maxEvents.value;
  }
  events.value = newEvents;
  eventBuffer.length = 0;

  markRecentRows(newEvents.slice(0, bufferedEvents.length).map((event) => event.key));
};

const scheduleEventBufferFlush = () => {
  if (flushTimer !== null) return;
  flushTimer = window.setTimeout(() => {
    flushTimer = null;
    flushEventBuffer();
  }, EVENT_BATCH_WINDOW_MS);
};

const STREAM_DIRECTION_STORAGE_KEY = 'dashboard.streamDirection';
const SHOW_ALL_ROWS_STORAGE_KEY = 'dashboard.showAllRows';
const BUILTIN_FILTER_STATE_STORAGE_KEY = 'dashboard.builtinFilters';
const streamDirection = ref<'top' | 'bottom'>(getStoredStreamDirection());
const showAllRows = ref(getStoredShowAllRows());

function getStoredStreamDirection(): 'top' | 'bottom' {
  if (typeof window === 'undefined') return 'top';
  return window.localStorage.getItem(STREAM_DIRECTION_STORAGE_KEY) === 'bottom' ? 'bottom' : 'top';
}

function getStoredShowAllRows(): boolean {
  if (typeof window === 'undefined') return false;
  return window.localStorage.getItem(SHOW_ALL_ROWS_STORAGE_KEY) === 'true';
}

const columnWidths = ref<Record<ResizableColumnKey, number>>({
  time: 120,
  tag: 120,
  pid: 96,
  comm: 150,
  type: 140,
  path: 180,
  action: 80,
});

const minColumnWidths: Record<ResizableColumnKey, number> = {
  time: 100,
  tag: 100,
  pid: 88,
  comm: 120,
  type: 120,
  path: 160,
  action: 72,
};

const eventTypes = [
  'execve',
  'openat',
  'network_connect',
  'network_bind',
  'network_sendto',
  'network_recvfrom',
  'mkdir',
  'unlink',
  'ioctl',
  'read',
  'write',
  'open',
  'chmod',
  'chown',
  'rename',
  'link',
  'symlink',
  'mknod',
  'clone',
  'exit',
  'socket',
  'accept',
  'accept4',
  'syscall',
  'wrapper_intercept',
  'native_hook',
];
const pageSizeOptions = ['20', '50', '100', '200'];
const eventTypeLabelMap: Record<number, string> = {
  [pb.EventType.EXECVE]: 'execve',
  [pb.EventType.OPENAT]: 'openat',
  [pb.EventType.NETWORK_CONNECT]: 'network_connect',
  [pb.EventType.MKDIR]: 'mkdir',
  [pb.EventType.UNLINK]: 'unlink',
  [pb.EventType.IOCTL]: 'ioctl',
  [pb.EventType.NETWORK_BIND]: 'network_bind',
  [pb.EventType.NETWORK_SENDTO]: 'network_sendto',
  [pb.EventType.NETWORK_RECVFROM]: 'network_recvfrom',
  [pb.EventType.READ]: 'read',
  [pb.EventType.WRITE]: 'write',
  [pb.EventType.OPEN]: 'open',
  [pb.EventType.CHMOD]: 'chmod',
  [pb.EventType.CHOWN]: 'chown',
  [pb.EventType.RENAME]: 'rename',
  [pb.EventType.LINK]: 'link',
  [pb.EventType.SYMLINK]: 'symlink',
  [pb.EventType.MKNOD]: 'mknod',
  [pb.EventType.CLONE]: 'clone',
  [pb.EventType.EXIT]: 'exit',
  [pb.EventType.SOCKET]: 'socket',
  [pb.EventType.ACCEPT]: 'accept',
  [pb.EventType.ACCEPT4]: 'accept4',
  25: 'syscall',
  [pb.EventType.WRAPPER_INTERCEPT]: 'wrapper_intercept',
  [pb.EventType.NATIVE_HOOK]: 'native_hook',
};
const eventTypeColorMap: Record<number, string> = {
  [pb.EventType.EXECVE]: 'blue',
  [pb.EventType.OPENAT]: 'green',
  [pb.EventType.NETWORK_CONNECT]: 'orange',
  [pb.EventType.MKDIR]: 'cyan',
  [pb.EventType.UNLINK]: 'red',
  [pb.EventType.IOCTL]: 'purple',
  [pb.EventType.NETWORK_BIND]: 'volcano',
  [pb.EventType.NETWORK_SENDTO]: 'cyan',
  [pb.EventType.NETWORK_RECVFROM]: 'geekblue',
  [pb.EventType.READ]: 'cyan',
  [pb.EventType.WRITE]: 'cyan',
  [pb.EventType.OPEN]: 'green',
  [pb.EventType.CHMOD]: 'gold',
  [pb.EventType.CHOWN]: 'gold',
  [pb.EventType.RENAME]: 'orange',
  [pb.EventType.LINK]: 'orange',
  [pb.EventType.SYMLINK]: 'orange',
  [pb.EventType.MKNOD]: 'purple',
  [pb.EventType.CLONE]: 'blue',
  [pb.EventType.EXIT]: 'red',
  [pb.EventType.SOCKET]: 'orange',
  [pb.EventType.ACCEPT]: 'volcano',
  [pb.EventType.ACCEPT4]: 'volcano',
  25: 'geekblue',
};
const selectableEventTypes = eventTypes
  .map((label) => {
    const entry = Object.entries(eventTypeLabelMap).find(([, mappedLabel]) => mappedLabel === label);
    return entry ? Number(entry[0]) : undefined;
  })
  .filter((value): value is number => value !== undefined);
const networkEventTypes = new Set<number>([
  pb.EventType.NETWORK_CONNECT,
  pb.EventType.NETWORK_BIND,
  pb.EventType.NETWORK_SENDTO,
  pb.EventType.NETWORK_RECVFROM,
  pb.EventType.ACCEPT,
  pb.EventType.ACCEPT4,
  pb.EventType.SOCKET,
]);

// Event category sets for tab filtering
const eventCategories: Record<string, Set<number>> = {
  network: new Set([
    pb.EventType.NETWORK_CONNECT,
    pb.EventType.NETWORK_BIND,
    pb.EventType.NETWORK_SENDTO,
    pb.EventType.NETWORK_RECVFROM,
    pb.EventType.SOCKET,
    pb.EventType.ACCEPT,
    pb.EventType.ACCEPT4,
  ]),
  file: new Set([
    pb.EventType.OPENAT,
    pb.EventType.IOCTL,
    pb.EventType.READ,
    pb.EventType.WRITE,
    pb.EventType.OPEN,
    pb.EventType.CHMOD,
    pb.EventType.CHOWN,
    pb.EventType.RENAME,
    pb.EventType.LINK,
    pb.EventType.SYMLINK,
    pb.EventType.MKNOD,
    pb.EventType.MKDIR,
    pb.EventType.UNLINK,
  ]),
  process: new Set([
    pb.EventType.EXECVE,
    pb.EventType.CLONE,
    pb.EventType.EXIT,
  ]),
  hook: new Set([
    pb.EventType.WRAPPER_INTERCEPT,
    pb.EventType.NATIVE_HOOK,
  ]),
};

const categoryTabs = [
  { key: 'all', label: '全部' },
  { key: 'network', label: '网络' },
  { key: 'file', label: '文件' },
  { key: 'process', label: '进程' },
  { key: 'hook', label: '钩子' },
  { key: 'syscall', label: '系统调用' },
] as const;

const activeTab = ref<string>('all');
const netDirFilter = ref<string>('all');
const syscallCatFilter = ref<string>('all');

const parseSyscallNr = (info?: string): number => {
  const m = info?.match(/\((\d+)\)/);
  return m ? Number(m[1]) : 0;
};

const syscallCategory = (nr: number): string => {
  if (nr >= 0 && nr <= 40) return 'io';       // read,write,open,close,stat,poll,mmap,brk,dup,pipe,nanosleep...
  if (nr >= 41 && nr <= 55) return 'net';      // socket,connect,accept,sendto,recvfrom,bind,listen...
  if (nr >= 56 && nr <= 61) return 'proc';     // clone,fork,vfork,execve,exit
  if (nr >= 62 && nr <= 63) return 'sig';      // kill,uname
  if (nr >= 64 && nr <= 71) return 'ipc';      // System V IPC
  if (nr >= 72 && nr <= 100) return 'io';      // fcntl,flock,fsync,truncate,chdir,mkdir,chmod,chown...
  if (nr >= 101 && nr <= 200) return 'sec';    // ptrace,prctl,capget,setuid,mount,pivot_root,chroot...
  if (nr >= 201 && nr <= 256) return 'misc';   // futex,epoll,timer,clock,mbind,inotify,migrate...
  if (nr >= 257 && nr <= 334) return 'io';     // openat,mkdirat,unlinkat,renameat,execveat,memfd_create...
  if (nr >= 424 && nr <= 453) return 'sec';    // pidfd,io_uring,open_tree,mount,landlock,clone3...
  return 'other';
};

const syscallCatLabels: Record<string, string> = {
  io: 'I/O & FS',
  net: 'Network',
  proc: 'Process',
  sig: 'Signal',
  ipc: 'SysV IPC',
  sec: 'Security',
  misc: 'Misc',
  other: 'Other',
};

const syscallCatColors: Record<string, string> = {
  io: 'cyan', net: 'purple', proc: 'orange', sig: 'red',
  ipc: 'gold', sec: 'red', misc: 'default', other: 'default',
};

const syscallDisplayName = (info?: string): string => {
  if (!info) return '';
  const m = info.match(/^(\w+)\(\d+\)/);
  return m ? m[1] : '';
};

const syncTabFromRoute = () => {
  const tab = route.params.tab as string | undefined;
  const resolved = tab && categoryTabs.some(t => t.key === tab) ? tab : 'all';
  if (activeTab.value !== resolved) {
    activeTab.value = resolved;
  }
};

syncTabFromRoute();

const onTabChange = (key: string) => {
  activeTab.value = key;
  router.push(key === 'all' ? '/dashboard' : `/dashboard/${key}`);
};

const decodeIncomingEvents = (payload: Uint8Array): pb.IEvent[] => {
  if (payload[0] === 10) {
    return pb.EventBatch.decode(payload).events || [];
  }
  return [pb.Event.decode(payload)];
};

const extractEventType = (event: pb.IEvent) =>
  Object.prototype.hasOwnProperty.call(event, 'eventType') && event.eventType !== null && event.eventType !== undefined
    ? Number(event.eventType)
    : undefined;

const isNetworkEvent = (eventType: number | undefined, type?: string) => {
  if (eventType !== undefined && networkEventTypes.has(eventType)) {
    return true;
  }
  return type === 'accept' || type === 'accept4' || Boolean(type?.startsWith('network_'));
};

const builtinFilterRules: BuiltinFilterRule[] = [
  {
    id: 'tty',
    label: 'TTY / PTY',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`.toLowerCase();
      const comm = event.comm.toLowerCase();
      return comm === 'tty'
        || path.includes('/dev/tty')
        || path.includes('/dev/pts/');
    },
  },
  {
    id: 'git',
    label: '.git metadata',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`;
      return /(^|\/)\.git(\/|$)/.test(path);
    },
  },
  {
    id: 'temp',
    label: 'Temp / cache',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`;
      return /(^|\/)(?:\.cache|__pycache__)(\/|$)/.test(path)
        || /(?:\.swp|\.tmp|~)$/i.test(path);
    },
  },
  {
    id: 'builddirs',
    label: '.venv / node_modules / target',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`;
      return /(^|\/)(?:\.venv|node_modules|target)(\/|$)/.test(path);
    },
  },
];

function createDefaultBuiltinFilterState(): BuiltinFilterState {
  return Object.fromEntries(builtinFilterRules.map((rule) => [rule.id, true])) as BuiltinFilterState;
}

function getStoredBuiltinFilterState(): BuiltinFilterState {
  const defaults = createDefaultBuiltinFilterState();
  if (typeof window === 'undefined') return defaults;
  try {
    const raw = window.localStorage.getItem(BUILTIN_FILTER_STATE_STORAGE_KEY);
    if (!raw) return defaults;
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    return builtinFilterRules.reduce((state, rule) => {
      state[rule.id] = typeof parsed[rule.id] === 'boolean' ? parsed[rule.id] as boolean : defaults[rule.id];
      return state;
    }, { ...defaults } as BuiltinFilterState);
  } catch {
    return defaults;
  }
}

const builtinFilterState = ref<BuiltinFilterState>(getStoredBuiltinFilterState());

watch(builtinFilterState, (state) => {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(BUILTIN_FILTER_STATE_STORAGE_KEY, JSON.stringify(state));
}, { deep: true });

const activeBuiltinFilterRules = computed(() => builtinFilterRules.filter((rule) => builtinFilterState.value[rule.id] !== false));

const builtinFilterSummary = computed(() => {
  const labels = activeBuiltinFilterRules.value.map((rule) => rule.label);
  return labels.length > 0 ? labels.join(' · ') : 'No built-in filters enabled';
});

const shouldKeepBuiltinEvent = (event: AgentEvent) => !activeBuiltinFilterRules.value.some((rule) => rule.test(event));

const setBuiltinFiltersEnabled = (enabled: boolean) => {
  builtinFilterState.value = Object.fromEntries(
    builtinFilterRules.map((rule) => [rule.id, enabled]),
  ) as BuiltinFilterState;
};

const baseColumns = [
  { title: 'Time', dataIndex: 'time', key: 'time' },
  { title: 'Tag', dataIndex: 'tag', key: 'tag' },
  { title: 'PID', dataIndex: 'pid', key: 'pid' },
  { title: 'Command', dataIndex: 'comm', key: 'comm' },
  { title: 'Event Type', dataIndex: 'type', key: 'type' },
  { title: 'Path', dataIndex: 'path', key: 'path', ellipsis: true },
  { title: 'Action', key: 'action', fixed: 'right' as const },
] as const;

const tagOptions = computed(() =>
  tags.value.map((tag) => ({
    label: tag,
    value: tag,
  })),
);

const eventTypeOptions = computed(() =>
  selectableEventTypes.map((eventType) => ({
    label: (eventTypeLabelMap[eventType] || String(eventType)).toUpperCase(),
    value: eventType,
  })),
);

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
  } catch (err) {
    console.error('Failed to fetch tags', err);
  }
};

// Built-in filters are always applied first and can be toggled per rule.
const builtinFilteredEvents = computed(() => events.value.filter((event) => shouldKeepBuiltinEvent(event)));

// Events with built-in + user filters only (used for stats bars)
const tabFilteredEvents = computed(() => {
  let result = builtinFilteredEvents.value;
  if (selectedTags.value.length) {
    const activeTags = new Set(selectedTags.value);
    result = result.filter(e => activeTags.has(e.tag));
  }
  if (selectedTypes.value.length) {
    const activeTypes = new Set(selectedTypes.value);
    result = result.filter((e) => e.eventType !== undefined && activeTypes.has(e.eventType));
  }
  const timeQuery = timeFilter.value.trim().toLowerCase();
  if (timeQuery) result = result.filter(e => e.time.toLowerCase().includes(timeQuery));
  const pidQuery = pidFilter.value.trim();
  const commQuery = commandFilter.value.trim().toLowerCase();
  const pathQuery = pathFilter.value.trim().toLowerCase();
  if (pidQuery) result = result.filter(e => String(e.pid).includes(pidQuery));
  if (commQuery) result = result.filter(e => e.comm.toLowerCase().includes(commQuery));
  if (pathQuery) result = result.filter(e => e.path.toLowerCase().includes(pathQuery));
  if (isDeduplicated.value) {
    const seen = new Set();
    result = result.filter(e => {
      const id = `${e.type}-${e.comm}-${e.path}`;
      if (seen.has(id)) return false;
      seen.add(id);
      return true;
    });
  }
  if (activeTab.value !== 'all') {
    const categorySet = eventCategories[activeTab.value];
    if (categorySet) {
      result = result.filter(e => e.eventType !== undefined && categorySet.has(e.eventType));
    }
  }
  if (hideUnknown.value) result = result.filter(e => e.tag !== 'Unknown');
  return result;
});

// Full filtered events including sub-filters
const filteredEvents = computed(() => {
  let result = tabFilteredEvents.value;
  if (activeTab.value === 'network' && netDirFilter.value !== 'all') {
    result = result.filter(e => (e.netDirection || 'unknown') === netDirFilter.value);
  }
  if (activeTab.value === 'syscall' && syscallCatFilter.value !== 'all') {
    result = result.filter(e => syscallCategory(parseSyscallNr(e.extraInfo)) === syscallCatFilter.value);
  }
  return streamDirection.value === 'bottom' ? [...result].reverse() : result;
});

const createEventMergeSignature = (event: AgentEvent) =>
  [
    event.eventType ?? '',
    event.type,
    event.tag,
    event.pid,
    event.ppid,
    event.uid,
    event.comm,
    event.path,
    event.netDirection ?? '',
    event.netEndpoint ?? '',
    event.netFamily ?? '',
    event.netBytes ?? '',
    event.retval ?? '',
    event.extraInfo ?? '',
    event.extraPath ?? '',
    event.bytes ?? '',
    event.mode ?? '',
    event.domain ?? '',
    event.sockType ?? '',
    event.protocol ?? '',
    event.uidArg ?? '',
    event.gidArg ?? '',
  ].map((value) => String(value)).join('\u001f');

const mergeEventsWithinWindow = (list: AgentEvent[]) => {
  const merged: DisplayedAgentEvent[] = [];
  const groupsBySignature = new Map<string, DisplayedAgentEvent>();

  for (const event of list) {
    const signature = createEventMergeSignature(event);
    const eventReceivedAtMs = event.receivedAtMs ?? 0;
    const currentGroup = groupsBySignature.get(signature);

    if (
      currentGroup
      && currentGroup.lastReceivedAtMs !== undefined
      && Math.abs(eventReceivedAtMs - currentGroup.lastReceivedAtMs) <= EVENT_MERGE_WINDOW_MS
    ) {
      currentGroup.occurrenceCount = (currentGroup.occurrenceCount ?? 1) + 1;
      currentGroup.lastReceivedAtMs = eventReceivedAtMs;
      continue;
    }

    const nextGroup: DisplayedAgentEvent = {
      ...event,
      occurrenceCount: 1,
      mergeSignature: signature,
      lastReceivedAtMs: eventReceivedAtMs,
    };
    merged.push(nextGroup);
    groupsBySignature.set(signature, nextGroup);
  }

  return merged.map(({ mergeSignature, lastReceivedAtMs, ...event }) => event);
};

const displayedEvents = computed(() => mergeEventsWithinWindow(filteredEvents.value));

// Stats use tabFilteredEvents (pre-sub-filter) to avoid zeroing out
const networkDirStats = computed(() => {
  const list = activeTab.value === 'network' ? tabFilteredEvents.value : [];
  const dirs = { outgoing: 0, incoming: 0, listening: 0, unknown: 0 };
  for (const e of list) {
    const d = e.netDirection || 'unknown';
    if (d in dirs) (dirs as any)[d]++; else dirs.unknown++;
  }
  return dirs;
});

const syscallCatStats = computed(() => {
  const list = activeTab.value === 'syscall' ? tabFilteredEvents.value : [];
  const cats: Record<string, number> = {};
  for (const e of list) {
    const cat = syscallCategory(parseSyscallNr(e.extraInfo));
    cats[cat] = (cats[cat] || 0) + 1;
  }
  return cats;
});

const tablePagination = computed(() => {
  if (showAllRows.value) {
    return false;
  }
  return {
    current: currentPage.value,
    pageSize: pageSize.value,
    total: displayedEvents.value.length,
    showSizeChanger: true,
    pageSizeOptions,
    showTotal: (total: number, range: [number, number]) => `${range[0]}-${range[1]} / ${total}`,
  };
});

const handleTableChange = (pagination: { current?: number; pageSize?: number }) => {
  if (showAllRows.value) return;
  currentPage.value = pagination.current ?? 1;
  pageSize.value = pagination.pageSize ?? pageSize.value;
};

const recentRowKeys = ref<Set<string>>(new Set());

const markRecentRows = (keys: string[]) => {
  if (keys.length === 0) return;

  const nextKeys = new Set(recentRowKeys.value);
  for (const key of keys) {
    nextKeys.add(key);
  }
  recentRowKeys.value = nextKeys;

  if (recentRowTimer !== null) {
    window.clearTimeout(recentRowTimer);
  }
  recentRowTimer = window.setTimeout(() => {
    recentRowKeys.value = new Set();
    recentRowTimer = null;
  }, 320);
};

const getRowClassName = (record: AgentEvent, index: number) => {
  const classes = [index % 2 === 0 ? 'excel-row-even' : 'excel-row-odd'];
  if (recentRowKeys.value.has(record.key)) {
    classes.push(streamDirection.value === 'bottom' ? 'excel-row-enter-bottom' : 'excel-row-enter-top');
  }
  return classes.join(' ');
};

const hasHeaderFilter = (key: string | number | symbol) => ['time', 'tag', 'pid', 'comm', 'type', 'path'].includes(String(key));

const isResizableColumn = (key: string | number | symbol) => (['time', 'tag', 'pid', 'comm', 'type', 'path', 'action'] as const).includes(String(key) as ResizableColumnKey);

const getFilterPopupContainer = (triggerNode: HTMLElement) =>
  (triggerNode.closest('.excel-filter-popover') as HTMLElement | null) ?? document.body;

const computePathWidth = () => {
  const fixedWidth = (['time', 'tag', 'pid', 'comm', 'type', 'action'] as const)
    .reduce((total, key) => total + columnWidths.value[key], 0);
  const availableWidth = tableContentWidth.value > 0 ? tableContentWidth.value : 0;
  const remainingWidth = availableWidth > 0 ? Math.max(minColumnWidths.path, availableWidth - fixedWidth - 12) : columnWidths.value.path;
  return Math.max(minColumnWidths.path, columnWidths.value.path, remainingWidth);
};

const tableColumns = computed(() => baseColumns.map((column) => {
  if (column.key === 'path') {
    return { ...column, width: computePathWidth() };
  }
  if (column.key in columnWidths.value) {
    return { ...column, width: columnWidths.value[column.key as ResizableColumnKey] };
  }
  return column;
}));

const handleTableResize = (entries: ResizeObserverEntry[]) => {
  const entry = entries[0];
  if (!entry) return;
  tableContentWidth.value = entry.contentRect.width;
};

const startColumnResize = (key: string, event: MouseEvent) => {
  if (!isResizableColumn(key)) return;
  event.preventDefault();

  const resizeKey = key as ResizableColumnKey;

  const startX = event.clientX;
  const startWidth = columnWidths.value[resizeKey];
  const minWidth = minColumnWidths[resizeKey];

  const onMouseMove = (moveEvent: MouseEvent) => {
    const nextWidth = Math.max(minWidth, startWidth + moveEvent.clientX - startX);
    columnWidths.value[resizeKey] = nextWidth;
  };

  const stopResize = () => {
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', stopResize);
    document.documentElement.classList.remove('excel-resizing');
    cleanupColumnResize = null;
  };

  cleanupColumnResize?.();
  document.documentElement.classList.add('excel-resizing');
  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', stopResize);
  cleanupColumnResize = stopResize;
};

const toggleHeaderFilter = (key: string | number | symbol) => {
  const filterKey = String(key);
  activeHeaderFilter.value = activeHeaderFilter.value === filterKey ? null : filterKey;
};

const closeHeaderFilter = () => {
  activeHeaderFilter.value = null;
};

const handleDocumentClick = (event: MouseEvent) => {
  if (!activeHeaderFilter.value) return;
  const target = event.target;
  if (!(target instanceof Element)) return;
  if (target.closest('.excel-filter-popover') || target.closest('.excel-header-filter-trigger')) {
    return;
  }
  closeHeaderFilter();
};

const isHeaderFilterActive = (key: string | number | symbol) => {
  switch (String(key)) {
    case 'time':
      return Boolean(timeFilter.value.trim());
    case 'tag':
      return selectedTags.value.length > 0;
    case 'pid':
      return Boolean(pidFilter.value.trim());
    case 'comm':
      return Boolean(commandFilter.value.trim());
    case 'type':
      return selectedTypes.value.length > 0;
    case 'path':
      return Boolean(pathFilter.value.trim());
    default:
      return false;
  }
};

const clearHeaderFilter = (key: string | number | symbol) => {
  switch (String(key)) {
    case 'time':
      timeFilter.value = '';
      break;
    case 'tag':
      selectedTags.value = [];
      break;
    case 'pid':
      pidFilter.value = '';
      break;
    case 'comm':
      commandFilter.value = '';
      break;
    case 'type':
      selectedTypes.value = [];
      break;
    case 'path':
      pathFilter.value = '';
      break;
  }
};

watch([selectedTags, selectedTypes, timeFilter, pidFilter, commandFilter, pathFilter, isDeduplicated, hideUnknown], () => {
  if (showAllRows.value) return;
  currentPage.value = 1;
});

watch(() => route.params.tab, () => {
  syncTabFromRoute();
  if (!showAllRows.value) {
    currentPage.value = 1;
  }
});

watch(streamDirection, (direction) => {
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(STREAM_DIRECTION_STORAGE_KEY, direction);
  }
  if (showAllRows.value) return;
  currentPage.value = direction === 'bottom'
    ? Math.max(1, Math.ceil(displayedEvents.value.length / pageSize.value))
    : 1;
});

watch(showAllRows, (enabled) => {
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(SHOW_ALL_ROWS_STORAGE_KEY, enabled ? 'true' : 'false');
  }
  if (enabled) {
    currentPage.value = 1;
    return;
  }
  const maxPage = Math.max(1, Math.ceil(displayedEvents.value.length / pageSize.value));
  currentPage.value = streamDirection.value === 'bottom' ? maxPage : 1;
});

watch([() => displayedEvents.value.length, pageSize, streamDirection], ([total]) => {
  if (showAllRows.value) return;
  const maxPage = Math.max(1, Math.ceil(total / pageSize.value));
  if (streamDirection.value === 'bottom') {
    currentPage.value = maxPage;
    return;
  }
  if (currentPage.value > maxPage) {
    currentPage.value = maxPage;
  }
});

const openDetails = (record: AgentEvent) => {
  selectedEvent.value = { ...record };
  showDetails.value = true;
};

const formatDetailValue = (value: number | string | undefined | null) => {
  if (value === undefined || value === null || value === '') {
    return '—';
  }
  return String(value);
};

const canInteractWithPath = (record: AgentEvent) => canPreviewEventPath(record);

const previewPath = async (path: string) => {
  previewLoading.value = true;
  try {
    const res = await axios.get(`/system/file-preview?path=${encodeURIComponent(path)}`);
    previewData.value = res.data as FilePreviewResponse;
    showPreview.value = true;
  } catch (err: any) {
    message.error(err?.response?.data?.error || 'Failed to preview file');
  } finally {
    previewLoading.value = false;
  }
};

const previewRecordPath = (record: AgentEvent) => {
  if (!canInteractWithPath(record)) return;
  void previewPath(record.path);
};

const openInExplorer = (record: AgentEvent) => {
  if (!canInteractWithPath(record)) return;
  void router.push({
    path: '/explorer',
    query: {
      path: record.path,
      preview: '1',
    },
  });
};

const getTagColor = (eventType?: number, type?: string) => {
  if (eventType !== undefined && eventTypeColorMap[eventType]) {
    return eventTypeColorMap[eventType];
  }
  const fallback = Object.entries(eventTypeLabelMap)
    .find(([, label]) => label === type)
    ?.at(0);
  if (fallback) {
    return eventTypeColorMap[Number(fallback)] || 'default';
  }
  return 'default';
};

const getCategoryColor = (tag: string) => {
  const colors: Record<string, string> = {
    'AI Agent': 'magenta', 'Git': 'orange', 'Build Tool': 'cyan',
    'Package Manager': 'green', 'Runtime': 'blue', 'System Tool': 'geekblue', 'Network Tool': 'purple',
    'Security': 'red'
  };
  return colors[tag] || 'default';
};

const toOptionalNumber = (value: unknown): number | undefined => {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  if (typeof value === 'object' && value !== null && 'toNumber' in value && typeof (value as { toNumber?: () => number }).toNumber === 'function') {
    const parsed = (value as { toNumber: () => number }).toNumber();
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
};

const toText = (value: unknown): string => {
  if (value === undefined || value === null) {
    return '';
  }
  return typeof value === 'string' ? value : String(value);
};

const buildAgentEvent = (data: Record<string, unknown>, receivedAtMs: number): AgentEvent => {
  const type = toText(data.type ?? data.Type);
  const path = toText(data.path ?? data.Path);
  const pid = toOptionalNumber(data.pid ?? data.Pid) ?? 0;
  const ppid = toOptionalNumber(data.ppid ?? data.Ppid) ?? 0;
  const uid = toOptionalNumber(data.uid ?? data.Uid) ?? 0;
  const tag = toText(data.tag ?? data.Tag);
  const comm = toText(data.comm ?? data.Comm);
  const eventType = extractEventType(data as pb.IEvent);
  const networkEvent = isNetworkEvent(eventType, type);

  return {
    key: `${pid}-${path}-${receivedAtMs}-${Math.random()}`,
    pid,
    ppid,
    uid,
    type,
    eventType,
    tag,
    comm,
    path,
    netDirection: networkEvent ? (toText(data.netDirection ?? data.net_direction) || undefined) : undefined,
    netEndpoint: networkEvent ? (toText(data.netEndpoint ?? data.net_endpoint) || undefined) : undefined,
    netFamily: networkEvent ? (toText(data.netFamily ?? data.net_family) || undefined) : undefined,
    netBytes: networkEvent ? toOptionalNumber(data.netBytes ?? data.net_bytes) : undefined,
    retval: toOptionalNumber(data.retval ?? data.Retval),
    extraInfo: toText(data.extraInfo ?? data.extra_info) || undefined,
    extraPath: toText(data.extraPath ?? data.extra_path) || undefined,
    bytes: toOptionalNumber(data.bytes ?? data.Bytes),
    mode: toText(data.mode ?? data.Mode) || undefined,
    domain: toText(data.domain ?? data.Domain) || undefined,
    sockType: toText(data.sockType ?? data.sock_type) || undefined,
    protocol: toOptionalNumber(data.protocol ?? data.Protocol),
    uidArg: toOptionalNumber(data.uidArg ?? data.uid_arg),
    gidArg: toOptionalNumber(data.gidArg ?? data.gid_arg),
    time: new Date(receivedAtMs).toLocaleTimeString(),
    receivedAtMs,
  };
};

const extractHistoryTimestampMs = (record: any): number => {
  const rawValue = record?.timestamp ?? record?.Timestamp ?? record?.receivedAt ?? record?.ReceivedAt;
  const parsed = toOptionalNumber(rawValue);
  if (parsed !== undefined) {
    return parsed;
  }
  if (typeof rawValue === 'string') {
    const dateParsed = Date.parse(rawValue);
    if (Number.isFinite(dateParsed)) {
      return dateParsed;
    }
  }
  return Date.now();
};

const normalizeHistoryRecord = (record: any): AgentEvent | null => {
  const event = record?.event ?? record?.Event;
  if (!event) {
    return null;
  }
  return buildAgentEvent(event as Record<string, unknown>, extractHistoryTimestampMs(record));
};

const clearHistoryLoadTimer = () => {
  if (historyLoadTimer !== null) {
    window.clearTimeout(historyLoadTimer);
    historyLoadTimer = null;
  }
};

const clearPendingLiveEvents = () => {
  pendingLiveEvents.length = 0;
};

const resetDashboardRuntimeState = () => {
  events.value = [];
  isConnected.value = false;
  isPaused.value = false;
  showDetails.value = false;
  selectedEvent.value = null;
  showPreview.value = false;
  previewLoading.value = false;
  previewData.value = null;
  selectedTags.value = [];
  selectedTypes.value = [];
  timeFilter.value = '';
  pidFilter.value = '';
  commandFilter.value = '';
  pathFilter.value = '';
  isDeduplicated.value = false;
  hideUnknown.value = true;
  activeHeaderFilter.value = null;
  tags.value = [];
  currentPage.value = 1;
  pageSize.value = 20;
  maxEvents.value = 5000;
  netDirFilter.value = 'all';
  syscallCatFilter.value = 'all';
  tableContentWidth.value = 0;
  columnWidths.value = {
    time: 120,
    tag: 120,
    pid: 96,
    comm: 150,
    type: 140,
    path: 180,
    action: 80,
  };
  recentRowKeys.value = new Set();
  eventBuffer.length = 0;
  clearHistoryLoadTimer();
  clearPendingLiveEvents();
  historyLoadToken += 1;
  historyLoaded.value = false;
  if (recentRowTimer !== null) {
    window.clearTimeout(recentRowTimer);
    recentRowTimer = null;
  }
  if (flushTimer !== null) {
    window.clearTimeout(flushTimer);
    flushTimer = null;
  }
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  resizeObserver?.disconnect();
  resizeObserver = null;
  cleanupColumnResize?.();
  cleanupColumnResize = null;
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
    ws = null;
  }
};

const flushPendingLiveEvents = () => {
  if (pendingLiveEvents.length === 0) {
    return;
  }
  eventBuffer.push(...pendingLiveEvents);
  clearPendingLiveEvents();
  flushEventBuffer();
};

const animateHistoryRecords = (records: AgentEvent[], token: number) => new Promise<void>((resolve) => {
  if (records.length === 0 || token !== historyLoadToken) {
    resolve();
    return;
  }

  let index = 0;
  const pump = () => {
    if (token !== historyLoadToken) {
      resolve();
      return;
    }

    const chunk = records.slice(index, index + HISTORY_LOAD_BATCH_SIZE);
    if (chunk.length === 0) {
      resolve();
      return;
    }

    eventBuffer.push(...chunk);
    flushEventBuffer();
    index += chunk.length;

    if (index < records.length) {
      historyLoadTimer = window.setTimeout(pump, HISTORY_LOAD_BATCH_DELAY_MS);
      return;
    }

    historyLoadTimer = null;
    resolve();
  };

  clearHistoryLoadTimer();
  pump();
});

const loadRecentEvents = async () => {
  const token = ++historyLoadToken;
  historyLoaded.value = false;
  clearPendingLiveEvents();
  clearHistoryLoadTimer();

  try {
    const response = await fetchProto(`/events/recent?limit=${HISTORY_LOAD_LIMIT}`, pb.EventHistoryResponse.decode);
    if (token !== historyLoadToken) {
      return;
    }

    const rawEvents = ((response as any).events ?? (response as any).Events ?? []) as any[];
    const records = rawEvents
      .map((record) => normalizeHistoryRecord(record))
      .filter((record): record is AgentEvent => record !== null);

    await animateHistoryRecords(records, token);
  } catch (err) {
    if (token === historyLoadToken) {
      console.error('Failed to load recent dashboard events', err);
    }
  } finally {
    if (token === historyLoadToken) {
      historyLoaded.value = true;
      flushPendingLiveEvents();
      clearHistoryLoadTimer();
    }
  }
};

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
  }
  const socket = new WebSocket(buildWebSocketUrl('/ws'));
  ws = socket;
  socket.binaryType = 'arraybuffer';

  socket.onopen = () => {
    if (ws !== socket) return;
    isConnected.value = true;
  };

  socket.onmessage = (message) => {
    if (ws !== socket) return;
    if (isPaused.value) return;
    try {
      const incomingEvents = decodeIncomingEvents(new Uint8Array(message.data));
      const normalizedEvents = incomingEvents.map((data) => buildAgentEvent(data as Record<string, unknown>, Date.now()));
      if (!historyLoaded.value) {
        pendingLiveEvents.push(...normalizedEvents);
      } else {
        eventBuffer.push(...normalizedEvents);
        scheduleEventBufferFlush();
      }
    } catch (e) {
      console.error('Failed to parse message', e);
    }
  };

  socket.onclose = () => {
    if (ws !== socket) return;
    isConnected.value = false;
    ws = null;
    if (!shouldReconnect) return;
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer);
    }
    reconnectTimer = window.setTimeout(() => {
      connectWebSocket();
    }, 3000);
  };
};

const clearEvents = async () => {
  try {
    await axios.post('/data/clear-events-memory');
    events.value = [];
    eventBuffer.length = 0;
    clearPendingLiveEvents();
    clearHistoryLoadTimer();
    historyLoadToken += 1;
    historyLoaded.value = true;
    if (flushTimer !== null) {
      window.clearTimeout(flushTimer);
      flushTimer = null;
    }
    recentRowKeys.value = new Set();
    currentPage.value = 1;
    message.success('Event buffer cleared on backend');
  } catch (err: any) {
    message.error(err?.response?.data?.error || 'Failed to clear events');
    events.value = [];
    eventBuffer.length = 0;
    clearPendingLiveEvents();
    clearHistoryLoadTimer();
    historyLoadToken += 1;
    historyLoaded.value = true;
    if (flushTimer !== null) {
      window.clearTimeout(flushTimer);
      flushTimer = null;
    }
    recentRowKeys.value = new Set();
    currentPage.value = 1;
  }
};

const exportEvents = () => {
  try {
    const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(events.value, null, 2));
    const downloadAnchorNode = document.createElement('a');
    downloadAnchorNode.setAttribute("href", dataStr);
    downloadAnchorNode.setAttribute("download", `ebpf-events-${new Date().toISOString()}.json`);
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    downloadAnchorNode.remove();
    message.success('Events exported as JSON');
  } catch (err) {
    message.error('Failed to export events');
  }
};

const exportEventsCSV = () => {
  try {
    const headers = ['Time', 'Tag', 'PID', 'PPID', 'UID', 'Command', 'Event Type', 'Path', 'Net Direction', 'Net Endpoint', 'Net Bytes'];
    const rows = filteredEvents.value.map(e => [
      e.time,
      e.tag,
      e.pid,
      e.ppid,
      e.uid,
      e.comm,
      e.type,
      e.path,
      e.netDirection || '',
      e.netEndpoint || '',
      e.netBytes || 0,
    ]);
    const csvContent = [headers, ...rows].map(r => r.map(c => `"${String(c).replace(/"/g, '""')}"`).join(',')).join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.setAttribute("href", url);
    link.setAttribute("download", `ebpf-events-${new Date().toISOString()}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    message.success('Events exported as CSV');
  } catch (err) {
    message.error('Failed to export CSV');
  }
};

onMounted(() => {
  resetDashboardRuntimeState();
  shouldReconnect = true;
  streamDirection.value = getStoredStreamDirection();
  showAllRows.value = getStoredShowAllRows();
  builtinFilterState.value = getStoredBuiltinFilterState();
  connectWebSocket();
  void loadRecentEvents();
  fetchTags();
  document.addEventListener('click', handleDocumentClick);
  if (tableWrapperRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(handleTableResize);
    resizeObserver.observe(tableWrapperRef.value);
  }
});

onUnmounted(() => {
  shouldReconnect = false;
  document.removeEventListener('click', handleDocumentClick);
  resetDashboardRuntimeState();
});

  return {
    events,
    isConnected,
    isPaused,
    showDetails,
    selectedEvent,
    showPreview,
    previewLoading,
    previewData,
    selectedTags,
    selectedTypes,
    timeFilter,
    pidFilter,
    commandFilter,
    pathFilter,
    isDeduplicated,
    hideUnknown,
    activeHeaderFilter,
    tags,
    currentPage,
    pageSize,
    tableWrapperRef,
    streamDirection,
    showAllRows,
    builtinFilterRules,
    builtinFilterState,
    builtinFilterSummary,
    setBuiltinFiltersEnabled,
    maxEvents,
    maxEventsOptions,
    activeTab,
    netDirFilter,
    syscallCatFilter,
    categoryTabs,
    networkDirStats,
    syscallCatStats,
    syscallCatLabels,
    syscallCatColors,
    tableColumns,
    tablePagination,
    pageSizeOptions,
    eventTypeOptions,
    tagOptions,
    displayedEvents,
    openDetails,
    formatDetailValue,
    canInteractWithPath,
    previewRecordPath,
    openInExplorer,
    getTagColor,
    getCategoryColor,
    getRowClassName,
    onTabChange,
    handleTableChange,
    toggleHeaderFilter,
    clearHeaderFilter,
    isHeaderFilterActive,
    hasHeaderFilter,
    isResizableColumn,
    startColumnResize,
    getFilterPopupContainer,
    clearEvents,
    exportEvents,
    exportEventsCSV,
    syscallDisplayName,
  };
}
