import type {
  DomainForwardRoute,
  DomainForwardProxySettings,
  KernelRiskFeedbackSettings,
  LoopDetectionSettings,
  LoopDetectionStatus,
  ResearchProcessingSettings,
  ResearchProcessingStatus,
  SignalCondition,
  SignalProcessingSettings,
  SignalProcessingStatus,
  SignalRule,
  TracepointBootstrapStatus,
} from "../../types/config";

export interface EditableHeaderRow {
  id: string;
  key: string;
  value: string;
}

export interface EditableDomainForwardRoute extends DomainForwardRoute {
  id: string;
}

let editableRowSequence = 0;
export const nextEditableRowId = (prefix: string) =>
  `${prefix}-${Date.now()}-${editableRowSequence++}`;

export const defaultDomainForwardProxy = (): DomainForwardProxySettings => ({
  enabled: false,
  httpPort: 80,
  httpsPort: 443,
  defaultScheme: "https",
  allowAnyHost: false,
  dnsResolver: "",
  dialTimeoutSeconds: 10,
  certFile: "",
  keyFile: "",
  routes: [],
});

export const defaultKernelRiskFeedback = (): KernelRiskFeedbackSettings => ({
  enabled: false,
  minRiskScore: 85,
  enforceNetwork: true,
  enforceFileNames: true,
  enforceExec: true,
  maxActionsPerMinute: 30,
});

export const defaultLoopDetection = (): LoopDetectionSettings => ({
  enabled: false,
  windowSeconds: 30,
  repeatThreshold: 5,
  maxContexts: 512,
  queueSize: 2048,
  emitSemanticAlerts: true,
});

export const defaultLoopDetectionStatus = (): LoopDetectionStatus => ({
  enabled: false,
  settings: defaultLoopDetection(),
  queueLen: 0,
  queueCap: 0,
  consumedTotal: 0,
  findingsTotal: 0,
  droppedTotal: 0,
  windowCount: 0,
  recentFindings: [],
  updatedAt: "",
});

export const defaultResearchProcessing = (): ResearchProcessingSettings => ({
  enabled: false,
  maxEvents: 5000,
  queueSize: 2048,
  timelineBucketSeconds: 60,
  topK: 20,
  recentSamples: 25,
  artifactRetentionDays: 14,
  maxSessionEvents: 50000,
  exportFormats: "jsonl,csv,bundle",
});

export const defaultResearchProcessingStatus = (): ResearchProcessingStatus => ({
  enabled: false,
  settings: defaultResearchProcessing(),
  queueLen: 0,
  queueCap: 0,
  consumedTotal: 0,
  droppedTotal: 0,
  bufferedTotal: 0,
  updatedAt: "",
  summary: {
    total: 0,
    bySource: [],
    byType: [],
    byComm: [],
    byPid: [],
    byTrace: [],
    timeline: [],
    topProcesses: [],
    topTraces: [],
    recentSamples: [],
    generatedTimestamp: 0,
    generatedTime: "",
  },
});

export const defaultSignalRules = (): SignalRule[] => [
  {
    id: "path_access",
    name: "Path / file access",
    enabled: true,
    kind: "path_access",
    ttlSeconds: 300,
    weight: 1,
    conditions: [{ field: "path", operator: "exists", value: "" }],
  },
  {
    id: "child_process",
    name: "Child process command",
    enabled: true,
    kind: "child_process",
    ttlSeconds: 300,
    weight: 2,
    conditions: [
      {
        field: "eventType",
        operator: "regex",
        value: "(EXECVE|SCHED_PROCESS_EXEC|SCHED_PROCESS_FORK|CLONE|exec|fork|clone)",
      },
    ],
  },
  {
    id: "repeated_read",
    name: "Repeated read",
    enabled: true,
    kind: "repeated_read",
    ttlSeconds: 300,
    weight: 1.5,
    conditions: [
      {
        field: "eventType",
        operator: "regex",
        value: "(READ|OPEN|OPENAT|read|open)",
      },
    ],
  },
];

export const defaultSignalProcessing = (): SignalProcessingSettings => ({
  enabled: false,
  queueSize: 2048,
  cronIntervalSeconds: 30,
  defaultTTLSeconds: 300,
  maxStates: 4096,
  protoLogCompression: "gzip",
  selectedPrograms: [],
  rules: defaultSignalRules(),
});

export const defaultSignalProcessingStatus = (): SignalProcessingStatus => ({
  enabled: false,
  settings: defaultSignalProcessing(),
  queueLen: 0,
  queueCap: 0,
  consumedTotal: 0,
  updatedTotal: 0,
  droppedTotal: 0,
  expiredTotal: 0,
  activeStates: 0,
  recentStates: [],
  availableKinds: [
    {
      kind: "path_access",
      label: "Path / file access",
      description: "Triggers when captured file/path fields match custom predicates.",
    },
    {
      kind: "child_process",
      label: "Child process command",
      description: "Triggers on exec/fork/clone style events and command/path predicates.",
    },
    {
      kind: "repeated_read",
      label: "Repeated read",
      description: "Tracks repeated READ/OPEN/OPENAT access to the same stable target.",
    },
    {
      kind: "custom",
      label: "Custom",
      description: "Uses only user-defined conditions and a TTL-weighted state key.",
    },
  ],
  updatedAt: "",
});

export const defaultTracepointBootstrapStatus = (): TracepointBootstrapStatus => ({
  kernelRelease: "unknown",
  compiledCount: 0,
  attachedCount: 0,
  skippedCount: 0,
  skippedTracepoints: [],
  status: "unknown",
  message: "Tracepoint bootstrap has not been observed yet.",
});

export const toFiniteNumber = (value: unknown, fallback: number) => {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : fallback;
};

export const normalizeTracepointBootstrapStatus = (
  value?: Partial<TracepointBootstrapStatus>,
): TracepointBootstrapStatus => {
  const defaults = defaultTracepointBootstrapStatus();
  const skippedTracepoints = Array.isArray(value?.skippedTracepoints)
    ? value.skippedTracepoints.filter(
        (tracepoint): tracepoint is string => typeof tracepoint === "string",
      )
    : [];
  const validStatuses: TracepointBootstrapStatus["status"][] = [
    "unknown",
    "ready",
    "partial",
    "error",
  ];
  const status = validStatuses.includes(
    value?.status as TracepointBootstrapStatus["status"],
  )
    ? (value?.status as TracepointBootstrapStatus["status"])
    : defaults.status;

  return {
    ...defaults,
    ...value,
    kernelRelease: value?.kernelRelease || defaults.kernelRelease,
    compiledCount: toFiniteNumber(value?.compiledCount, defaults.compiledCount),
    attachedCount: toFiniteNumber(value?.attachedCount, defaults.attachedCount),
    skippedCount: toFiniteNumber(value?.skippedCount, skippedTracepoints.length),
    skippedTracepoints,
    status,
    message: value?.message || defaults.message,
  };
};

export const normalizeDomainForwardProxy = (
  value?: Partial<DomainForwardProxySettings>,
): DomainForwardProxySettings => {
  const defaults = defaultDomainForwardProxy();
  const scheme = value?.defaultScheme === "http" ? "http" : "https";
  return {
    ...defaults,
    ...value,
    defaultScheme: scheme,
    httpPort: Number(value?.httpPort || defaults.httpPort),
    httpsPort: Number(value?.httpsPort || defaults.httpsPort),
    dialTimeoutSeconds: Number(
      value?.dialTimeoutSeconds || defaults.dialTimeoutSeconds,
    ),
    routes: Array.isArray(value?.routes) ? value.routes : [],
  };
};

export const normalizeKernelRiskFeedback = (
  value?: Partial<KernelRiskFeedbackSettings>,
): KernelRiskFeedbackSettings => {
  const defaults = defaultKernelRiskFeedback();
  return {
    ...defaults,
    ...value,
    minRiskScore: Number(value?.minRiskScore || defaults.minRiskScore),
    maxActionsPerMinute: Number(
      value?.maxActionsPerMinute || defaults.maxActionsPerMinute,
    ),
  };
};

export const normalizeLoopDetection = (
  value?: Partial<LoopDetectionSettings>,
): LoopDetectionSettings => {
  const defaults = defaultLoopDetection();
  return {
    ...defaults,
    ...value,
    windowSeconds: Number(value?.windowSeconds || defaults.windowSeconds),
    repeatThreshold: Number(
      value?.repeatThreshold || defaults.repeatThreshold,
    ),
    maxContexts: Number(value?.maxContexts || defaults.maxContexts),
    queueSize: Number(value?.queueSize || defaults.queueSize),
    emitSemanticAlerts:
      value?.emitSemanticAlerts ?? defaults.emitSemanticAlerts,
  };
};

export const normalizeLoopDetectionStatus = (
  value?: Partial<LoopDetectionStatus>,
): LoopDetectionStatus => {
  const defaults = defaultLoopDetectionStatus();
  return {
    ...defaults,
    ...value,
    settings: normalizeLoopDetection(value?.settings),
    recentFindings: Array.isArray(value?.recentFindings)
      ? value.recentFindings
      : [],
  };
};

export const normalizeResearchProcessing = (
  value?: Partial<ResearchProcessingSettings>,
): ResearchProcessingSettings => {
  const defaults = defaultResearchProcessing();
  return {
    ...defaults,
    ...value,
    maxEvents: Number(value?.maxEvents || defaults.maxEvents),
    queueSize: Number(value?.queueSize || defaults.queueSize),
    timelineBucketSeconds: Number(
      value?.timelineBucketSeconds || defaults.timelineBucketSeconds,
    ),
    topK: Number(value?.topK || defaults.topK),
    recentSamples: Number(value?.recentSamples || defaults.recentSamples),
    artifactRetentionDays: Number(
      value?.artifactRetentionDays || defaults.artifactRetentionDays,
    ),
    maxSessionEvents: Number(
      value?.maxSessionEvents || defaults.maxSessionEvents,
    ),
    exportFormats: String(value?.exportFormats || defaults.exportFormats),
  };
};

export const normalizeResearchProcessingStatus = (
  value?: Partial<ResearchProcessingStatus>,
): ResearchProcessingStatus => {
  const defaults = defaultResearchProcessingStatus();
  const summary = value?.summary || defaults.summary;
  return {
    ...defaults,
    ...value,
    settings: normalizeResearchProcessing(value?.settings),
    summary: {
      ...defaults.summary,
      ...summary,
      bySource: Array.isArray(summary.bySource) ? summary.bySource : [],
      byType: Array.isArray(summary.byType) ? summary.byType : [],
      byComm: Array.isArray(summary.byComm) ? summary.byComm : [],
      byPid: Array.isArray(summary.byPid) ? summary.byPid : [],
      byTrace: Array.isArray(summary.byTrace) ? summary.byTrace : [],
      timeline: Array.isArray(summary.timeline) ? summary.timeline : [],
      topProcesses: Array.isArray(summary.topProcesses)
        ? summary.topProcesses
        : [],
      topTraces: Array.isArray(summary.topTraces) ? summary.topTraces : [],
      recentSamples: Array.isArray(summary.recentSamples)
        ? summary.recentSamples
        : [],
    },
  };
};

export const normalizeSignalCondition = (
  value?: Partial<SignalCondition>,
): SignalCondition => ({
  field: String(value?.field || "path"),
  operator: String(value?.operator || (value?.value ? "contains" : "exists")),
  value: String(value?.value || ""),
});

export const normalizeSignalRule = (
  value: Partial<SignalRule> | undefined,
  index: number,
): SignalRule => {
  const defaults = defaultSignalRules()[index] || {
    id: `signal_rule_${index + 1}`,
    name: `Signal rule ${index + 1}`,
    enabled: true,
    kind: "custom",
    ttlSeconds: 300,
    weight: 1,
    conditions: [],
  };
  const conditions = Array.isArray(value?.conditions)
    ? value.conditions.map((condition) => normalizeSignalCondition(condition))
    : defaults.conditions;
  return {
    ...defaults,
    ...value,
    id: String(value?.id || defaults.id),
    name: String(value?.name || defaults.name),
    enabled: value?.enabled ?? defaults.enabled,
    kind: String(value?.kind || defaults.kind),
    ttlSeconds: Number(value?.ttlSeconds || defaults.ttlSeconds),
    weight: Number(value?.weight || defaults.weight),
    conditions,
  };
};

export const normalizeSignalProcessing = (
  value?: Partial<SignalProcessingSettings>,
): SignalProcessingSettings => {
  const defaults = defaultSignalProcessing();
  return {
    ...defaults,
    ...value,
    queueSize: Number(value?.queueSize || defaults.queueSize),
    cronIntervalSeconds: Number(
      value?.cronIntervalSeconds || defaults.cronIntervalSeconds,
    ),
    defaultTTLSeconds: Number(
      value?.defaultTTLSeconds || defaults.defaultTTLSeconds,
    ),
    maxStates: Number(value?.maxStates || defaults.maxStates),
    protoLogCompression: String(
      value?.protoLogCompression || defaults.protoLogCompression,
    ),
    selectedPrograms: Array.isArray(value?.selectedPrograms)
      ? value.selectedPrograms.map((program) => ({
          program: String(program.program || ""),
          enabled: program.enabled ?? true,
          path: String(program.path || ""),
        }))
      : defaults.selectedPrograms,
    rules: Array.isArray(value?.rules)
      ? value.rules.map((rule, index) => normalizeSignalRule(rule, index))
      : defaults.rules,
  };
};

export const normalizeSignalProcessingStatus = (
  value?: Partial<SignalProcessingStatus>,
): SignalProcessingStatus => {
  const defaults = defaultSignalProcessingStatus();
  return {
    ...defaults,
    ...value,
    settings: normalizeSignalProcessing(value?.settings),
    recentStates: Array.isArray(value?.recentStates) ? value.recentStates : [],
    availableKinds: Array.isArray(value?.availableKinds)
      ? value.availableKinds
      : defaults.availableKinds,
  };
};
