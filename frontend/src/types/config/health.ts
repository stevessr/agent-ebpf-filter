export interface CollectorHealthResponse {
  collectorMapAvailable: boolean;
  ringbufEventsTotal: number;
  ringbufDroppedTotal: number;
  ringbufReserveFailedTotal: number;
  ringbufZeroCopyDecodeTotal: number;
  ringbufCopyDecodeTotal: number;
  eventsByTypeTotal: Record<string, number>;
  eventsByPidTotal?: Record<string, number>;
  agentSightCountersTotal?: Record<string, number>;
  backendQueueLen: number;
  wsClients: number;
  persistAppendLatencyNs: number;
  capturedArchivedTotal: number;
  capturedPersistedTotal: number;
  capturedPersistErrorsTotal: number;
  persistWriterActive: boolean;
  persistWriterStopping: boolean;
  persistQueueLen: number;
  persistQueueCap: number;
  persistPending: number;
  persistGenerationEnqueued: number;
  persistGenerationPersisted: number;
  persistGenerationFailed: number;
  persistGenerationDropped: number;
  persistWriterLastFlushedAt?: string;
  persistWriterLastError?: string;
  kernelRiskEvaluationsTotal: number;
  kernelRiskAlertsTotal: number;
  kernelRiskBlocksTotal: number;
  kernelRiskLastEvalLatencyNs: number;
  kernelRiskFeedbackApplied: number;
  kernelRiskFeedbackDropped: number;
  kernelRiskFeedbackLastError?: string;
  captureHealthy: boolean;
}
export interface TracepointBootstrapStatus {
  kernelRelease: string;
  compiledCount: number;
  attachedCount: number;
  skippedCount: number;
  skippedTracepoints: string[];
  status: "unknown" | "ready" | "partial" | "error";
  message: string;
  observedAt?: string;
}
export interface OTelHealthResponse {
  enabled: boolean;
  ready: boolean;
  endpoint: string;
  serviceName: string;
  queueLen: number;
  queueCap: number;
  enqueuedEvents: number;
  processedEvents: number;
  activeRunSpans: number;
  activeTaskSpans: number;
  activeToolSpans: number;
  maxRunSpans: number;
  maxTaskSpans: number;
  maxToolSpans: number;
  evictedRunSpans: number;
  evictedTaskSpans: number;
  evictedToolSpans: number;
  exportedSpans: number;
  droppedEvents: number;
  lastExportedAt?: string;
  lastError?: string;
}
