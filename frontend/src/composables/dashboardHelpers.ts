import { pb } from "../pb/tracker_pb.js";
import { networkEventTypes, eventTypeLabelMap, eventTypeColorMap, type AgentEvent } from "./dashboardConstants";

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
    time: new Date(receivedAtMs).toLocaleTimeString(),
    receivedAtMs,
  };
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


