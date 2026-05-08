import { pb } from "../pb/tracker_pb.js";
import { networkEventTypes, eventTypeLabelMap, eventTypeColorMap, syscallDisplayName, type AgentEvent } from "./dashboardConstants";

export const decodeIncomingEvents = (payload: Uint8Array): pb.IEvent[] => {
  if (payload[0] === 10) {
    return pb.EventBatch.decode(payload).events || [];
  }
  return [pb.Event.decode(payload)];
};


export const extractEventType = (event: pb.IEvent) =>
  Object.prototype.hasOwnProperty.call(event, 'eventType') && event.eventType !== null && event.eventType !== undefined
    ? Number(event.eventType)
    : undefined;


export const isNetworkEvent = (eventType: number | undefined, type?: string) => {
  if (eventType !== undefined && networkEventTypes.has(eventType)) {
    return true;
  }
  return type === 'accept' || type === 'accept4' || Boolean(type?.startsWith('network_'));
};

export const getTagColor = (eventType?: number, type?: string) => {
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


export const getCategoryColor = (tag: string) => {
  const colors: Record<string, string> = {
    'AI Agent': 'magenta', 'Git': 'orange', 'Build Tool': 'cyan',
    'Package Manager': 'green', 'Runtime': 'blue', 'System Tool': 'geekblue', 'Network Tool': 'purple',
    'Security': 'red'
  };
  return colors[tag] || 'default';
};


export const toOptionalNumber = (value: unknown): number | undefined => {
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


export const toText = (value: unknown): string => {
  if (value === undefined || value === null) {
    return '';
  }
  return typeof value === 'string' ? value : String(value);
};


export const buildAgentEvent = (data: Record<string, unknown>, receivedAtMs: number): AgentEvent => {
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
    durationNs: toOptionalNumber(data.durationNs ?? data.duration_ns ?? data.DurationNs),
    time: new Date(receivedAtMs).toLocaleTimeString(),
    receivedAtMs,
  };
};

const formatDurationNs = (durationNs?: number): string => {
  if (durationNs === undefined || !Number.isFinite(durationNs) || durationNs < 0) {
    return '';
  }
  if (durationNs < 1_000) {
    return `${durationNs} ns`;
  }
  if (durationNs < 1_000_000) {
    const value = durationNs / 1_000;
    return `${value.toFixed(value < 10 ? 1 : 0)} µs`;
  }
  if (durationNs < 1_000_000_000) {
    const value = durationNs / 1_000_000;
    return `${value.toFixed(value < 10 ? 2 : 1)} ms`;
  }
  const value = durationNs / 1_000_000_000;
  return `${value.toFixed(2)} s`;
};

const compactExtraInfo = (value?: string): string => {
  const trimmed = value?.trim();
  if (!trimmed) {
    return '';
  }
  return trimmed.replace(/\s+/g, ', ');
};

const formatEventArgs = (event: AgentEvent): string[] => {
  const args: string[] = [];

  switch (event.type) {
    case 'execve':
    case 'execveat':
    case 'open':
    case 'openat':
    case 'openat2':
    case 'access':
    case 'truncate':
    case 'chdir':
    case 'mkdir':
    case 'mkdirat':
    case 'rmdir':
    case 'creat':
    case 'unlink':
    case 'unlinkat':
    case 'readlink':
    case 'readlinkat':
    case 'chroot':
    case 'umount2':
    case 'swapon':
    case 'swapoff':
    case 'sethostname':
    case 'setdomainname':
    case 'setxattr':
    case 'lsetxattr':
    case 'getxattr':
    case 'lgetxattr':
    case 'listxattr':
    case 'llistxattr':
    case 'removexattr':
    case 'lremovexattr':
    case 'fsopen':
    case 'memfd_create':
    case 'open_tree':
      if (event.path) args.push(JSON.stringify(event.path));
      if (event.extraPath) args.push(JSON.stringify(event.extraPath));
      if (event.extraInfo) args.push(compactExtraInfo(event.extraInfo));
      break;
    case 'rename':
    case 'renameat':
    case 'renameat2':
    case 'link':
    case 'linkat':
    case 'symlink':
    case 'symlinkat':
    case 'move_mount':
    case 'pivot_root':
      if (event.path) args.push(JSON.stringify(event.path));
      if (event.extraPath) args.push(JSON.stringify(event.extraPath));
      break;
    case 'read':
    case 'write':
      if (event.extraInfo) args.push(compactExtraInfo(event.extraInfo));
      if (event.bytes !== undefined) args.push(`bytes=${event.bytes}`);
      break;
    case 'chmod':
    case 'fchmodat':
    case 'fchmodat2':
      if (event.path) args.push(JSON.stringify(event.path));
      if (event.mode) args.push(`mode=${event.mode}`);
      break;
    case 'mknod':
    case 'mknodat':
      if (event.path) args.push(JSON.stringify(event.path));
      if (event.extraInfo) args.push(compactExtraInfo(event.extraInfo));
      break;
    case 'chown':
    case 'fchownat':
      if (event.path) args.push(JSON.stringify(event.path));
      if (event.uidArg !== undefined) args.push(`uid=${event.uidArg}`);
      if (event.gidArg !== undefined) args.push(`gid=${event.gidArg}`);
      break;
    case 'socket':
      if (event.domain) args.push(`domain=${event.domain}`);
      if (event.sockType) args.push(`type=${event.sockType}`);
      if (event.protocol !== undefined) args.push(`protocol=${event.protocol}`);
      break;
    case 'connect':
    case 'bind':
    case 'sendto':
    case 'recvfrom':
    case 'accept':
    case 'accept4':
    case 'network_connect':
    case 'network_bind':
    case 'network_sendto':
    case 'network_recvfrom':
      if (event.path) {
        args.push(JSON.stringify(event.path));
      } else {
        if (event.netDirection) args.push(`direction=${event.netDirection}`);
        if (event.netEndpoint) args.push(`endpoint=${JSON.stringify(event.netEndpoint)}`);
        if (event.netBytes !== undefined) args.push(`bytes=${event.netBytes}`);
      }
      break;
    case 'syscall': {
      const syscallMatch = event.extraInfo?.match(/^(\w+)\((\d+)\)\s*(.*)$/);
      if (syscallMatch) {
        args.push(`nr=${syscallMatch[2]}`);
        const tail = syscallMatch[3].trim();
        if (tail) {
          args.push(...tail.split(/\s+/).map((part) => part.trim()).filter(Boolean));
        }
      } else if (event.extraInfo) {
        args.push(compactExtraInfo(event.extraInfo));
      }
      break;
    }
    default:
      if (event.path) args.push(JSON.stringify(event.path));
      if (event.extraPath) args.push(JSON.stringify(event.extraPath));
      if (event.extraInfo) args.push(compactExtraInfo(event.extraInfo));
      break;
  }

  return args.filter((part) => part !== '');
};

export const formatTraceSummary = (event: AgentEvent | null | undefined): string => {
  if (!event) {
    return '';
  }

  const displayName = event.type === 'syscall'
    ? (syscallDisplayName(event.extraInfo) || 'syscall')
    : (event.type || 'event');
  const args = formatEventArgs(event);
  const call = `${displayName}(${args.join(', ')})`;
  const retval = event.retval !== undefined ? ` = ${event.retval}` : '';
  const duration = formatDurationNs(event.durationNs);
  const durationSuffix = duration ? ` [${duration}]` : '';
  return `${call}${retval}${durationSuffix}`;
};


export const extractHistoryTimestampMs = (record: any): number => {
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


export const normalizeHistoryRecord = (record: any): AgentEvent | null => {
  const event = record?.event ?? record?.Event;
  if (!event) {
    return null;
  }
  return buildAgentEvent(event as Record<string, unknown>, extractHistoryTimestampMs(record));
};
