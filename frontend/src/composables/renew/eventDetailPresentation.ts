import type { RenewTone } from "./types";

export interface RenewDetailField {
  key: string;
  label: string;
  value: string;
  mono?: boolean;
}

export interface RenewDetailSection {
  key: string;
  title: string;
  description?: string;
  fields: RenewDetailField[];
}

export interface RenewEventDetailPresentation {
  title: string;
  subtitle: string;
  category: "file" | "network" | "process" | "agent" | "security" | "system";
  categoryLabel: string;
  tone: RenewTone;
  outcome: string;
  riskScore: number;
  riskLabel: string;
  timestamp: string;
  eventType: string;
  processLabel: string;
  targetLabel: string;
  sections: RenewDetailSection[];
  envelopeFields: RenewDetailField[];
  error: string;
}

type AnyRecord = Record<string, unknown>;

const keyToken = (key: string) => key.replace(/[^a-z0-9]/gi, "").toLowerCase();

const asRecord = (value: unknown): AnyRecord | null =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as AnyRecord)
    : null;

const findValue = (record: AnyRecord | null, ...aliases: string[]): unknown => {
  if (!record) return undefined;
  for (const alias of aliases) {
    if (Object.hasOwn(record, alias)) return record[alias];
  }
  const normalized = new Map(
    Object.entries(record).map(([key, value]) => [keyToken(key), value]),
  );
  for (const alias of aliases) {
    const value = normalized.get(keyToken(alias));
    if (value !== undefined) return value;
  }
  return undefined;
};

const hasValue = (value: unknown) => {
  if (value == null) return false;
  if (typeof value === "string") return value.trim().length > 0;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === "object") return Object.keys(value as AnyRecord).length > 0;
  return true;
};

const textValue = (value: unknown) => {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return "";
};

const numberValue = (value: unknown) => {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
};

const clampRisk = (value: unknown) =>
  Math.max(0, Math.min(100, numberValue(value)));

const formatBytes = (value: unknown) => {
  const bytes = numberValue(value);
  if (bytes < 1024) return `${bytes.toLocaleString()} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let amount = bytes;
  let index = -1;
  do {
    amount /= 1024;
    index += 1;
  } while (amount >= 1024 && index < units.length - 1);
  return `${amount >= 10 ? amount.toFixed(1) : amount.toFixed(2)} ${units[index]}`;
};

const formatDurationNs = (value: unknown) => {
  const ns = numberValue(value);
  if (ns <= 0) return "0 ns";
  if (ns < 1_000) return `${ns.toLocaleString()} ns`;
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(2)} μs`;
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(2)} ms`;
  return `${(ns / 1_000_000_000).toFixed(2)} s`;
};

const formatTimestamp = (value: unknown) => {
  const numeric = numberValue(value);
  const date = numeric > 0
    ? new Date(numeric < 10_000_000_000 ? numeric * 1000 : numeric)
    : new Date(textValue(value));
  return Number.isNaN(date.getTime())
    ? textValue(value) || "—"
    : date.toLocaleString([], {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
};

const formatObject = (value: unknown) => {
  if (Array.isArray(value)) return value.map(textValue).filter(Boolean).join(" · ");
  const record = asRecord(value);
  if (!record) return textValue(value);
  return Object.entries(record)
    .filter(([, item]) => hasValue(item))
    .slice(0, 8)
    .map(([key, item]) => `${prettyKey(key)}=${textValue(item) || JSON.stringify(item)}`)
    .join(" · ");
};

const prettyKeyMap: Record<string, string> = {
  pid: "PID",
  ppid: "父 PID",
  tgid: "TGID",
  uid: "用户 ID",
  gid: "组 ID",
  type: "事件类型",
  eventtype: "事件编号",
  tag: "标签",
  comm: "进程",
  path: "路径",
  extrapath: "附加路径",
  cwd: "工作目录",
  netendpoint: "网络目标",
  netdirection: "方向",
  netfamily: "地址族",
  netbytes: "网络字节",
  domain: "域名",
  srcip: "源 IP",
  srcport: "源端口",
  dstip: "目标 IP",
  dstport: "目标端口",
  transport: "传输协议",
  appprotocol: "应用协议",
  servicename: "服务",
  dnsname: "DNS 名称",
  sni: "TLS SNI",
  httphost: "HTTP Host",
  httpmethod: "HTTP 方法",
  httppath: "HTTP 路径",
  httpstatus: "HTTP 状态",
  grpcservice: "gRPC 服务",
  grpcmethod: "gRPC 方法",
  toolname: "工具",
  agentrunid: "Agent Run",
  conversationid: "会话",
  turnid: "Turn",
  toolcallid: "Tool Call",
  taskid: "任务",
  traceid: "Trace ID",
  spanid: "Span ID",
  decision: "安全决策",
  riskscore: "风险分",
  behavior: "行为分类",
  retval: "返回值",
  durationns: "耗时",
  schemaversion: "Schema",
  capturesource: "采集来源",
  capturedelayns: "采集延迟",
  kernelsequence: "内核序号",
  kernelcpu: "采集 CPU",
  redactionlevel: "脱敏级别",
  sanitizedfields: "已脱敏字段",
  ipscope: "IP 范围",
  geocountry: "国家/地区",
  geoasn: "ASN",
};

const prettyKey = (key: string) =>
  prettyKeyMap[keyToken(key)] ||
  key
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/[_-]+/g, " ")
    .replace(/^./, (char) => char.toUpperCase());

const field = (
  record: AnyRecord | null,
  key: string,
  label: string,
  aliases: string[],
  options: { mono?: boolean; format?: (value: unknown) => string } = {},
): RenewDetailField | null => {
  const value = findValue(record, ...aliases);
  if (!hasValue(value)) return null;
  const formatted = options.format ? options.format(value) : formatObject(value);
  if (!formatted) return null;
  return { key, label, value: formatted, mono: options.mono };
};

const compact = (values: Array<RenewDetailField | null>) =>
  values.filter((value): value is RenewDetailField => Boolean(value));

const categoryForType = (type: string): RenewEventDetailPresentation["category"] => {
  const normalized = type.toLowerCase();
  if (
    normalized.includes("network") ||
    normalized.startsWith("tcp_") ||
    normalized.includes("dns") ||
    normalized.includes("socket") ||
    normalized.includes("accept") ||
    normalized.includes("http") ||
    normalized.includes("tls") ||
    normalized.includes("sse") ||
    normalized.includes("grpc")
  ) {
    return "network";
  }
  if (
    ["open", "openat", "read", "write", "unlink", "rename", "chmod", "chown", "link", "symlink", "mkdir", "mknod", "ioctl", "stdio"].includes(normalized)
  ) {
    return "file";
  }
  if (
    normalized.includes("exec") ||
    normalized.includes("fork") ||
    normalized.includes("clone") ||
    normalized.includes("exit") ||
    normalized === "wait4"
  ) {
    return "process";
  }
  if (normalized.includes("alert") || normalized.includes("security")) {
    return "security";
  }
  if (
    normalized.includes("wrapper") ||
    normalized.includes("hook") ||
    normalized.includes("agent") ||
    normalized.includes("otel")
  ) {
    return "agent";
  }
  return "system";
};

const categoryLabels: Record<RenewEventDetailPresentation["category"], string> = {
  file: "文件行为",
  network: "网络行为",
  process: "进程行为",
  agent: "Agent 行为",
  security: "安全告警",
  system: "系统事件",
};

const actionTitle = (type: string) => {
  switch (type.toLowerCase()) {
    case "open":
    case "openat":
    case "read":
      return "读取文件";
    case "write":
      return "写入文件";
    case "unlink":
      return "删除文件";
    case "rename":
      return "重命名文件";
    case "mkdir":
      return "创建目录";
    case "chmod":
    case "chown":
      return "修改文件权限";
    case "execve":
    case "process_exec":
      return "启动程序";
    case "process_fork":
    case "clone":
      return "创建子进程";
    case "network_connect":
    case "tcp_connect":
      return "建立网络连接";
    case "network_sendto":
      return "发送网络数据";
    case "network_recvfrom":
      return "接收网络数据";
    case "dns_query":
      return "查询域名";
    case "wrapper_intercept":
      return "Agent 调用工具";
    case "native_hook":
      return "Agent 生命周期事件";
    case "semantic_alert":
    case "agentsight_alert":
      return "检测到异常行为";
    default:
      return type ? `记录 ${type} 事件` : "记录系统事件";
  }
};

const toneAndOutcome = (
  decision: string,
  risk: number,
  retval: number,
  type: string,
): { tone: RenewTone; outcome: string } => {
  const normalized = decision.toUpperCase();
  if (normalized.includes("BLOCK") || normalized.includes("DENY")) {
    return { tone: "danger", outcome: "已阻断" };
  }
  if (
    normalized.includes("ALERT") ||
    type.includes("alert") ||
    risk >= 60
  ) {
    return {
      tone: risk >= 80 ? "danger" : "warning",
      outcome: normalized.includes("ALERT") ? "需要关注" : "高风险",
    };
  }
  if (retval < 0) return { tone: "warning", outcome: "操作失败" };
  if (normalized === "ALLOW") return { tone: "normal", outcome: "已允许" };
  return { tone: "normal", outcome: "已记录" };
};

const riskLabel = (risk: number) => {
  if (risk >= 80) return "严重";
  if (risk >= 60) return "高";
  if (risk >= 40) return "中";
  if (risk > 0) return "低";
  return "未发现风险";
};

const firstText = (record: AnyRecord | null, aliases: string[]) =>
  textValue(findValue(record, ...aliases));

const targetFor = (event: AnyRecord | null) => {
  const srcIP = firstText(event, ["src_ip", "srcIp"]);
  const srcPort = firstText(event, ["src_port", "srcPort"]);
  const dstIP = firstText(event, ["dst_ip", "dstIp"]);
  const dstPort = firstText(event, ["dst_port", "dstPort"]);
  const flow = [srcIP && `${srcIP}${srcPort ? `:${srcPort}` : ""}`, dstIP && `${dstIP}${dstPort ? `:${dstPort}` : ""}`]
    .filter(Boolean)
    .join(" → ");
  return (
    firstText(event, ["path"]) ||
    firstText(event, ["net_endpoint", "netEndpoint"]) ||
    firstText(event, ["domain", "dns_name", "dnsName", "sni", "http_host", "httpHost"]) ||
    flow ||
    firstText(event, ["extra_path", "extraPath"])
  );
};

const envelopeLeafFields = (envelope: AnyRecord | null) => {
  const fields: RenewDetailField[] = [];
  const walk = (value: unknown, path: string[], depth: number) => {
    if (fields.length >= 18 || depth > 3) return;
    if (!hasValue(value)) return;
    if (Array.isArray(value)) {
      if (value.every((item) => typeof item !== "object")) {
        fields.push({
          key: path.join("."),
          label: path.map(prettyKey).join(" · "),
          value: value.map(textValue).filter(Boolean).join(" · "),
          mono: path.some((part) => /id|path|endpoint/i.test(part)),
        });
      }
      return;
    }
    const record = asRecord(value);
    if (record) {
      for (const [key, item] of Object.entries(record)) {
        if (keyToken(key) === "legacyevent") continue;
        walk(item, [...path, key], depth + 1);
        if (fields.length >= 18) break;
      }
      return;
    }
    fields.push({
      key: path.join("."),
      label: path.map(prettyKey).join(" · "),
      value: formatObject(value),
      mono: path.some((part) => /id|path|endpoint|digest/i.test(part)),
    });
  };

  if (envelope) {
    for (const [key, value] of Object.entries(envelope)) {
      const token = keyToken(key);
      if (["schemaversion", "timestampns", "source", "pid", "tid", "uid", "gid", "cgroupid"].includes(token)) {
        continue;
      }
      walk(value, [key], 0);
      if (fields.length >= 18) break;
    }
  }
  return fields.filter((item) => item.value);
};

export function presentRenewEventDetail(
  detail: Record<string, unknown> | null,
): RenewEventDetailPresentation | null {
  if (!detail) return null;

  const event =
    asRecord(findValue(detail, "Event", "event")) ||
    (findValue(detail, "type", "Type") ? detail : null);
  const envelope = asRecord(findValue(detail, "Envelope", "envelope"));
  const error = textValue(findValue(detail, "error", "Error"));

  if (!event && error) {
    return {
      title: "无法读取事件详情",
      subtitle: error,
      category: "system",
      categoryLabel: "读取错误",
      tone: "warning",
      outcome: "不可用",
      riskScore: 0,
      riskLabel: "—",
      timestamp: "—",
      eventType: "—",
      processLabel: "—",
      targetLabel: "—",
      sections: [],
      envelopeFields: [],
      error,
    };
  }

  const type = firstText(event, ["type", "Type"]) || "event";
  const category = categoryForType(type);
  const decision = firstText(event, ["decision", "Decision"]);
  const risk = clampRisk(findValue(event, "risk_score", "riskScore", "RiskScore"));
  const retval = numberValue(findValue(event, "retval", "Retval"));
  const status = toneAndOutcome(decision, risk, retval, type.toLowerCase());
  const process = firstText(event, ["comm", "Comm"]) || "未知进程";
  const pid = numberValue(findValue(event, "pid", "Pid"));
  const target = targetFor(event);
  const timestamp = formatTimestamp(
    findValue(detail, "Timestamp", "timestamp") ||
      findValue(event, "received_at_ms", "receivedAtMs", "first_seen_ms", "firstSeenMs"),
  );

  const subjectFields = compact([
    { key: "time", label: "发生时间", value: timestamp },
    field(event, "comm", "进程", ["comm", "Comm"]),
    field(event, "pid", "PID", ["pid", "Pid"], { mono: true }),
    field(event, "ppid", "父 PID", ["ppid", "Ppid"], { mono: true }),
    field(event, "tgid", "TGID", ["tgid", "Tgid"], { mono: true }),
    field(event, "uid", "用户 ID", ["uid", "Uid"], { mono: true }),
    field(event, "gid", "组 ID", ["gid", "Gid"], { mono: true }),
    field(event, "cwd", "工作目录", ["cwd", "Cwd"], { mono: true }),
    field(event, "container", "容器", ["container_id", "containerId", "ContainerId"], { mono: true }),
    field(event, "cgroup", "cgroup ID", ["cgroup_id", "cgroupId", "CgroupId"], { mono: true }),
  ]);

  const targetFields = compact([
    field(event, "path", "路径", ["path", "Path"], { mono: true }),
    field(event, "extraPath", "附加路径", ["extra_path", "extraPath", "ExtraPath"], { mono: true }),
    field(event, "endpoint", "网络目标", ["net_endpoint", "netEndpoint", "NetEndpoint"], { mono: true }),
    field(event, "domain", "域名", ["domain", "Domain", "dns_name", "dnsName", "DnsName"], { mono: true }),
    field(event, "direction", "方向", ["net_direction", "netDirection", "NetDirection"]),
    field(event, "netBytes", "本次网络字节", ["net_bytes", "netBytes", "NetBytes"], { format: formatBytes }),
    field(event, "bytes", "文件字节", ["bytes", "Bytes"], { format: formatBytes }),
    field(event, "srcIp", "源 IP", ["src_ip", "srcIp", "SrcIp"], { mono: true }),
    field(event, "srcPort", "源端口", ["src_port", "srcPort", "SrcPort"], { mono: true }),
    field(event, "dstIp", "目标 IP", ["dst_ip", "dstIp", "DstIp"], { mono: true }),
    field(event, "dstPort", "目标端口", ["dst_port", "dstPort", "DstPort"], { mono: true }),
    field(event, "transport", "传输协议", ["transport", "Transport"]),
    field(event, "appProtocol", "应用协议", ["app_protocol", "appProtocol", "AppProtocol"]),
    field(event, "service", "服务", ["service_name", "serviceName", "ServiceName"]),
    field(event, "sni", "TLS SNI", ["sni", "Sni"], { mono: true }),
    field(event, "httpHost", "HTTP Host", ["http_host", "httpHost", "HttpHost"], { mono: true }),
    field(event, "httpMethod", "HTTP 方法", ["http_method", "httpMethod", "HttpMethod"]),
    field(event, "httpPath", "HTTP 路径", ["http_path", "httpPath", "HttpPath"], { mono: true }),
    field(event, "httpStatus", "HTTP 状态", ["http_status", "httpStatus", "HttpStatus"]),
    field(event, "grpc", "gRPC", ["grpc_service", "grpcService", "GrpcService"], { mono: true }),
    field(event, "mode", "模式 / 权限", ["mode", "Mode"]),
  ]);

  const agentFields = compact([
    field(event, "tag", "归属标签", ["tag", "Tag"]),
    field(event, "tool", "工具", ["tool_name", "toolName", "ToolName"]),
    field(event, "rootPid", "Agent 根 PID", ["root_agent_pid", "rootAgentPid", "RootAgentPid"], { mono: true }),
    field(event, "run", "Agent Run", ["agent_run_id", "agentRunId", "AgentRunId"], { mono: true }),
    field(event, "conversation", "会话", ["conversation_id", "conversationId", "ConversationId"], { mono: true }),
    field(event, "turn", "Turn", ["turn_id", "turnId", "TurnId"], { mono: true }),
    field(event, "toolCall", "Tool Call", ["tool_call_id", "toolCallId", "ToolCallId"], { mono: true }),
    field(event, "task", "任务", ["task_id", "taskId", "TaskId"], { mono: true }),
    field(event, "trace", "Trace ID", ["trace_id", "traceId", "TraceId"], { mono: true }),
    field(event, "span", "Span ID", ["span_id", "spanId", "SpanId"], { mono: true }),
    field(event, "apiProduct", "API 产品", ["api_product", "apiProduct", "ApiProduct"]),
    field(event, "apiOperation", "API 操作", ["api_operation", "apiOperation", "ApiOperation"]),
  ]);

  const securityFields = compact([
    decision
      ? { key: "decision", label: "安全决策", value: decision.toUpperCase() }
      : null,
    hasValue(findValue(event, "risk_score", "riskScore", "RiskScore"))
      ? { key: "risk", label: "风险评分", value: `${risk.toFixed(risk % 1 ? 1 : 0)} / 100（${riskLabel(risk)}）` }
      : null,
    field(event, "retval", "系统返回值", ["retval", "Retval"], { mono: true }),
    field(event, "behavior", "行为分类", ["behavior", "Behavior"]),
    field(event, "redaction", "脱敏级别", ["redaction_level", "redactionLevel", "RedactionLevel"]),
    field(event, "sanitized", "已脱敏字段", ["sanitized_fields", "sanitizedFields", "SanitizedFields"]),
    field(event, "ipScope", "IP 范围", ["ip_scope", "ipScope", "IpScope"]),
    field(event, "geo", "国家 / 地区", ["geo_country", "geoCountry", "GeoCountry"]),
    field(event, "asn", "ASN", ["geo_asn", "geoAsn", "GeoAsn"], { mono: true }),
    field(event, "audit", "审计标记", ["audit_flags", "auditFlags", "AuditFlags"], { mono: true }),
  ]);

  const captureFields = compact([
    { key: "type", label: "事件类型", value: type, mono: true },
    field(event, "schema", "Schema", ["schema_version", "schemaVersion", "SchemaVersion"], { mono: true }),
    field(event, "captureSource", "采集来源", ["capture_source", "captureSource", "CaptureSource"]),
    field(event, "duration", "执行耗时", ["duration_ns", "durationNs", "DurationNs"], { format: formatDurationNs }),
    field(event, "captureDelay", "采集延迟", ["capture_delay_ns", "captureDelayNs", "CaptureDelayNs"], { format: formatDurationNs }),
    field(event, "flowId", "Flow ID", ["flow_id", "flowId", "FlowId"], { mono: true }),
    field(event, "argvDigest", "参数摘要", ["argv_digest", "argvDigest", "ArgvDigest"], { mono: true }),
    field(event, "kernelSequence", "内核序号", ["kernel_sequence", "kernelSequence", "KernelSequence"], { mono: true }),
    field(event, "kernelCpu", "采集 CPU", ["kernel_cpu", "kernelCpu", "KernelCpu"], { mono: true }),
    field(event, "historic", "历史记录", ["historic", "Historic"]),
    field(event, "stale", "时效状态", ["stale_level", "staleLevel", "StaleLevel"]),
  ]);

  const sections: RenewDetailSection[] = [
    { key: "subject", title: "谁在什么时候做了什么", fields: subjectFields },
    { key: "target", title: "目标与影响", fields: targetFields },
    {
      key: "agent",
      title: "Agent 上下文",
      description: "只有后端明确关联到 Agent / Harness 的字段才会显示。",
      fields: agentFields,
    },
    { key: "security", title: "安全判断", fields: securityFields },
    { key: "capture", title: "采集信息", fields: captureFields },
  ].filter((section) => section.fields.length > 0);

  return {
    title: actionTitle(type),
    subtitle: target || `${process}${pid ? ` · PID ${pid}` : ""}`,
    category,
    categoryLabel: categoryLabels[category],
    tone: status.tone,
    outcome: status.outcome,
    riskScore: risk,
    riskLabel: riskLabel(risk),
    timestamp,
    eventType: type,
    processLabel: `${process}${pid ? ` · PID ${pid}` : ""}`,
    targetLabel: target || "未提供明确目标",
    sections,
    envelopeFields: envelopeLeafFields(envelope),
    error,
  };
}
