import { pb } from "../../pb/tracker_pb.js";

export type RenewMonitoringCost = "low" | "medium" | "high";
export type RenewMonitoringProfileKey = "lite" | "daily" | "deep";

export interface RenewMonitoringModule {
  key: string;
  title: string;
  description: string;
  coverage: string;
  cost: RenewMonitoringCost;
  weight: number;
  eventTypes: number[];
}

export interface RenewMonitoringProfile {
  key: RenewMonitoringProfileKey;
  title: string;
  description: string;
  modules: string[];
  statsIntervalMs: number;
  loopDetection: boolean;
  signalProcessing: boolean;
  researchProcessing: boolean;
}

export const RENEW_MONITORING_MODULES: RenewMonitoringModule[] = [
  {
    key: "process",
    title: "进程行为",
    description: "程序启动、派生、退出与进程生命周期。日常监控建议常开。",
    coverage: "执行与生命周期：fork / clone / exit / wait", 
    cost: "low",
    weight: 1,
    eventTypes: [
      pb.EventType.EXECVE,
      pb.EventType.CLONE,
      pb.EventType.EXIT,
      pb.EventType.SCHED_PROCESS_FORK,
      pb.EventType.SCHED_PROCESS_EXEC,
      pb.EventType.SCHED_PROCESS_EXIT,
      pb.EventType.WAIT4,
    ],
  },
  {
    key: "file-changes",
    title: "文件改动",
    description: "关注创建、写入、删除、改名、权限与链接变化，噪声通常低于文件访问监控。",
    coverage: "文件改动：write / unlink / rename / chmod / chown / link", 
    cost: "low",
    weight: 1,
    eventTypes: [
      pb.EventType.MKDIR,
      pb.EventType.UNLINK,
      pb.EventType.WRITE,
      pb.EventType.CHMOD,
      pb.EventType.CHOWN,
      pb.EventType.RENAME,
      pb.EventType.LINK,
      pb.EventType.SYMLINK,
      pb.EventType.MKNOD,
    ],
  },
  {
    key: "network",
    title: "基础联网",
    description: "记录连接、监听与 DNS 行为，适合发现 Agent 的外联目标。",
    coverage: "联网与域名：connect / bind / TCP 生命周期 / DNS", 
    cost: "low",
    weight: 1,
    eventTypes: [
      pb.EventType.NETWORK_CONNECT,
      pb.EventType.NETWORK_BIND,
      pb.EventType.TCP_CONNECT,
      pb.EventType.TCP_CLOSE,
      pb.EventType.DNS_QUERY,
    ],
  },
  {
    key: "file-access",
    title: "文件访问",
    description: "记录 open/read/ioctl 等高频访问。排查敏感文件读取时再开启更合适。",
    coverage: "文件访问：open / openat / read / ioctl", 
    cost: "high",
    weight: 3,
    eventTypes: [
      pb.EventType.OPENAT,
      pb.EventType.IOCTL,
      pb.EventType.READ,
      pb.EventType.OPEN,
    ],
  },
  {
    key: "network-detail",
    title: "网络细节",
    description: "记录 send/recv/socket/accept 与 TCP 状态细节，适合短时网络调查。",
    coverage: "网络细节：sendto / recvfrom / socket / accept / TCP 状态", 
    cost: "high",
    weight: 3,
    eventTypes: [
      pb.EventType.NETWORK_SENDTO,
      pb.EventType.NETWORK_RECVFROM,
      pb.EventType.SOCKET,
      pb.EventType.ACCEPT,
      pb.EventType.ACCEPT4,
      pb.EventType.TCP_STATE_CHANGE,
    ],
  },
  {
    key: "deep-syscall",
    title: "深度系统调用",
    description: "补充通用 syscall 轨迹，用于专业诊断；日常常驻通常不需要。",
    coverage: "通用系统调用（generic syscall）", 
    cost: "high",
    weight: 4,
    eventTypes: [pb.EventType.GENERIC_SYSCALL],
  },
];

export const RENEW_MONITORING_PROFILES: RenewMonitoringProfile[] = [
  {
    key: "lite",
    title: "轻量",
    description: "进程 + 文件改动，10 秒系统采样；适合长期后台常驻。",
    modules: ["process", "file-changes"],
    statsIntervalMs: 10_000,
    loopDetection: false,
    signalProcessing: false,
    researchProcessing: false,
  },
  {
    key: "daily",
    title: "日常推荐",
    description: "加入基础联网与轻量行为检测，不采集高频文件读取和网络包级细节。",
    modules: ["process", "file-changes", "network"],
    statsIntervalMs: 5_000,
    loopDetection: true,
    signalProcessing: true,
    researchProcessing: false,
  },
  {
    key: "deep",
    title: "深度",
    description: "开启全部内核事件组和研究处理，适合短时排查而非长期常驻。",
    modules: RENEW_MONITORING_MODULES.map((module) => module.key),
    statsIntervalMs: 2_000,
    loopDetection: true,
    signalProcessing: true,
    researchProcessing: true,
  },
];

export const RENEW_KERNEL_MONITOR_EVENT_TYPES = Array.from(
  new Set(RENEW_MONITORING_MODULES.flatMap((module) => module.eventTypes)),
).sort((a, b) => a - b);

export function enabledEventTypesForModules(moduleKeys: Iterable<string>) {
  const enabled = new Set(moduleKeys);
  return new Set(
    RENEW_MONITORING_MODULES.filter((module) => enabled.has(module.key)).flatMap(
      (module) => module.eventTypes,
    ),
  );
}

export function disabledEventTypesForModules(moduleKeys: Iterable<string>) {
  const enabled = enabledEventTypesForModules(moduleKeys);
  return RENEW_KERNEL_MONITOR_EVENT_TYPES.filter((type) => !enabled.has(type));
}

export function profileByKey(key: RenewMonitoringProfileKey) {
  return RENEW_MONITORING_PROFILES.find((profile) => profile.key === key)!;
}

export function estimateMonitoringWeight(
  moduleKeys: Iterable<string>,
  options: {
    statsIntervalMs: number;
    loopDetection: boolean;
    signalProcessing: boolean;
    researchProcessing: boolean;
    tlsCapture: boolean;
    persistence: boolean;
  },
) {
  const enabled = new Set(moduleKeys);
  let weight = RENEW_MONITORING_MODULES.reduce(
    (sum, module) => sum + (enabled.has(module.key) ? module.weight : 0),
    0,
  );
  if (options.statsIntervalMs <= 2_000) weight += 2;
  else if (options.statsIntervalMs <= 5_000) weight += 1;
  if (options.loopDetection) weight += 1;
  if (options.signalProcessing) weight += 1;
  if (options.researchProcessing) weight += 3;
  if (options.tlsCapture) weight += 5;
  if (options.persistence) weight += 1;
  return weight;
}

export function monitoringWeightLabel(weight: number): {
  label: string;
  tone: RenewMonitoringCost;
} {
  if (weight <= 4) return { label: "低", tone: "low" };
  if (weight <= 9) return { label: "中", tone: "medium" };
  return { label: "高", tone: "high" };
}
