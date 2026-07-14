<script setup lang="ts">
import { DeleteOutlined, ReloadOutlined } from "@ant-design/icons-vue";
import type { useConfigRuntime } from "../../composables/config/useConfigRuntime";

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
}>();

const {
  runtimeSettings,
  bootstrapHealth,
  collectorHealth,
  otelHealth,
  domainForwardStatus,
  loopDetectionStatus,
  researchProcessingStatus,
  fetchCollectorHealth,
  fetchLoopDetectionStatus,
  runLoopDetectionScan,
  resetLoopDetection,
  fetchResearchProcessingStatus,
  runResearchProcessingScan,
  resetResearchProcessing,
  clearInMemoryEvents,
  clearPersistedLog,
  clearAllEvents,
} = props.runtime;

const formatLatencyMs = (value?: number) => {
  if (!value) return "0 ms";
  return `${(value / 1_000_000).toFixed(value >= 1_000_000 ? 2 : 3)} ms`;
};

const formatMaybeDate = (value?: string) => {
  if (!value) return "Not exported yet";
  return value;
};

const compactList = (items?: Array<string | number>) => {
  if (!items || items.length === 0) return "—";
  return items.join(", ");
};
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-card title="Collection Health" size="small">
        <template #extra>
          <a-button size="small" @click="fetchCollectorHealth">
            <ReloadOutlined /> Refresh Health
          </a-button>
        </template>
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag
                  :color="collectorHealth.captureHealthy ? 'green' : 'red'"
                >
                  {{
                    collectorHealth.captureHealthy
                      ? "Capture healthy"
                      : "Ringbuf drops detected"
                  }}
                </a-tag>
                <a-tag
                  :color="
                    collectorHealth.collectorMapAvailable ? 'blue' : 'default'
                  "
                >
                  {{
                    collectorHealth.collectorMapAvailable
                      ? "collector_stats available"
                      : "collector_stats unavailable"
                  }}
                </a-tag>
              </div>
              <div>
                ringbuf events:
                <strong>{{ collectorHealth.ringbufEventsTotal }}</strong>
              </div>
              <div>
                ringbuf reserve failed:
                <strong>{{ collectorHealth.ringbufReserveFailedTotal }}</strong>
              </div>
              <div>
                zero-copy decode:
                <strong>{{
                  collectorHealth.ringbufZeroCopyDecodeTotal
                }}</strong>
              </div>
              <div>
                copy fallback decode:
                <strong>{{ collectorHealth.ringbufCopyDecodeTotal }}</strong>
              </div>
              <div>
                backend queue len:
                <strong>{{ collectorHealth.backendQueueLen }}</strong>
              </div>
              <div>
                WS clients: <strong>{{ collectorHealth.wsClients }}</strong>
              </div>
              <div>
                last persisted append latency:
                <strong>{{
                  formatLatencyMs(collectorHealth.persistAppendLatencyNs)
                }}</strong>
              </div>
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag
                  :color="
                    collectorHealth.persistWriterActive
                      ? 'green'
                      : collectorHealth.persistWriterStopping
                        ? 'orange'
                        : collectorHealth.persistWriterLastError
                          ? 'red'
                          : 'default'
                  "
                >
                  persist writer:
                  {{
                    collectorHealth.persistWriterActive
                      ? "active"
                      : collectorHealth.persistWriterStopping
                        ? "stopping"
                        : collectorHealth.persistWriterLastError
                          ? "failed"
                          : "disabled"
                  }}
                </a-tag>
                <a-tag color="blue">
                  queue: {{ collectorHealth.persistQueueLen }}/{{
                    collectorHealth.persistQueueCap
                  }}
                </a-tag>
                <a-tag color="purple">
                  pending: {{ collectorHealth.persistPending }}
                </a-tag>
                <a-tag color="green">
                  persisted:
                  {{ collectorHealth.persistGenerationPersisted }}/{{
                    collectorHealth.persistGenerationEnqueued
                  }}
                </a-tag>
                <a-tag
                  :color="
                    collectorHealth.persistGenerationFailed > 0
                      ? 'red'
                      : 'default'
                  "
                >
                  failed: {{ collectorHealth.persistGenerationFailed }}
                </a-tag>
                <a-tag
                  :color="
                    collectorHealth.persistGenerationDropped > 0
                      ? 'red'
                      : 'default'
                  "
                >
                  dropped: {{ collectorHealth.persistGenerationDropped }}
                </a-tag>
              </div>
              <div>
                last persisted flush:
                <strong>{{
                  formatMaybeDate(collectorHealth.persistWriterLastFlushedAt)
                }}</strong>
              </div>
              <a-alert
                v-if="collectorHealth.persistWriterLastError"
                type="warning"
                show-icon
                :message="collectorHealth.persistWriterLastError"
              />
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 8px">
              <div style="font-weight: 600">Events by type</div>
              <a-empty
                v-if="
                  Object.keys(collectorHealth.eventsByTypeTotal || {})
                    .length === 0
                "
                description="No events yet"
                :image="false"
              />
              <div v-else style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag
                  v-for="(count, type) in collectorHealth.eventsByTypeTotal"
                  :key="type"
                  color="default"
                >
                  {{ type }}: {{ count }}
                </a-tag>
              </div>
              <a-alert
                v-if="collectorHealth.ringbufDroppedTotal > 0"
                type="warning"
                show-icon
                message="Sampling may be incomplete because the kernel ringbuf dropped events."
              />
              <a-divider style="margin: 4px 0" />
              <div style="font-weight: 600">Kernel-risk loop</div>
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag color="purple">
                  eval: {{ collectorHealth.kernelRiskEvaluationsTotal }}
                </a-tag>
                <a-tag color="orange">
                  alert: {{ collectorHealth.kernelRiskAlertsTotal }}
                </a-tag>
                <a-tag color="red">
                  block: {{ collectorHealth.kernelRiskBlocksTotal }}
                </a-tag>
                <a-tag color="geekblue">
                  feedback applied:
                  {{ collectorHealth.kernelRiskFeedbackApplied }}
                </a-tag>
                <a-tag color="default">
                  feedback dropped:
                  {{ collectorHealth.kernelRiskFeedbackDropped }}
                </a-tag>
              </div>
              <div>
                last risk eval latency:
                <strong>{{
                  formatLatencyMs(collectorHealth.kernelRiskLastEvalLatencyNs)
                }}</strong>
              </div>
              <a-alert
                v-if="collectorHealth.kernelRiskFeedbackLastError"
                type="warning"
                show-icon
                :message="collectorHealth.kernelRiskFeedbackLastError"
              />
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Dead-loop Detection Worker" size="small">
        <template #extra>
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-button size="small" @click="fetchLoopDetectionStatus">
              <ReloadOutlined /> Refresh
            </a-button>
            <a-button size="small" type="primary" @click="runLoopDetectionScan()">
              Scan Recent Events
            </a-button>
            <a-popconfirm
              title="Reset loop windows and recent findings?"
              @confirm="resetLoopDetection"
            >
              <a-button size="small" danger>Reset</a-button>
            </a-popconfirm>
          </div>
        </template>
        <div style="display: flex; flex-direction: column; gap: 12px">
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-tag :color="loopDetectionStatus.enabled ? 'blue' : 'default'">
              {{ loopDetectionStatus.enabled ? "streaming enabled" : "manual scan" }}
            </a-tag>
            <a-tag color="purple">
              consumed: {{ loopDetectionStatus.consumedTotal }}
            </a-tag>
            <a-tag color="orange">
              findings: {{ loopDetectionStatus.findingsTotal }}
            </a-tag>
            <a-tag color="blue">
              windows: {{ loopDetectionStatus.windowCount }}
            </a-tag>
            <a-tag color="geekblue">
              queue:
              {{ loopDetectionStatus.queueLen }}/{{ loopDetectionStatus.queueCap }}
            </a-tag>
            <a-tag color="default">
              dropped: {{ loopDetectionStatus.droppedTotal }}
            </a-tag>
          </div>
          <a-alert
            v-if="loopDetectionStatus.lastError"
            type="warning"
            show-icon
            :message="loopDetectionStatus.lastError"
          />
          <a-alert
            v-else
            type="info"
            show-icon
            message="Repeated-context findings are localized to run / task / tool / trace / PID."
            description="Use Scan Recent Events for offline research even when streaming detection is disabled. Enable and tune thresholds from Runtime Config."
          />
          <a-empty
            v-if="loopDetectionStatus.recentFindings.length === 0"
            description="No repeated contexts detected yet"
            :image="false"
          />
          <a-card
            v-for="finding in loopDetectionStatus.recentFindings"
            :key="finding.id"
            size="small"
            :title="`${finding.contextType}: ${finding.contextKey}`"
          >
            <div style="display: flex; flex-direction: column; gap: 8px">
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag color="red">
                  repeats: {{ finding.repeatCount }} / {{ finding.windowSeconds }}s
                </a-tag>
                <a-tag color="purple">{{ finding.target || "no target" }}</a-tag>
                <a-tag v-if="finding.agentRunId" color="blue">
                  run {{ finding.agentRunId }}
                </a-tag>
                <a-tag v-if="finding.taskId" color="geekblue">
                  task {{ finding.taskId }}
                </a-tag>
                <a-tag v-if="finding.toolCallId" color="cyan">
                  tool {{ finding.toolCallId }}
                </a-tag>
              </div>
              <div>
                <strong>reason:</strong>
                {{ finding.reason }}
              </div>
              <div>
                <strong>fingerprint:</strong>
                <code>{{ finding.fingerprint }}</code>
              </div>
              <div>
                <strong>events:</strong>
                {{ compactList(finding.eventTypes) }}
              </div>
              <div>
                <strong>pids:</strong>
                {{ compactList(finding.pids) }}
              </div>
              <div>
                <strong>comms:</strong>
                {{ compactList(finding.comms) }}
              </div>
              <div>
                <strong>paths:</strong>
                {{ compactList(finding.paths) }}
              </div>
              <a-typography-text type="secondary">
                {{ finding.suggestedAction }}
              </a-typography-text>
            </div>
          </a-card>
        </div>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Backend Research Processing" size="small">
        <template #extra>
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-button size="small" @click="fetchResearchProcessingStatus">
              <ReloadOutlined /> Refresh
            </a-button>
            <a-button
              size="small"
              type="primary"
              @click="runResearchProcessingScan()"
            >
              Scan Recent Events
            </a-button>
            <a-popconfirm
              title="Reset backend research summaries?"
              @confirm="resetResearchProcessing"
            >
              <a-button size="small" danger>Reset</a-button>
            </a-popconfirm>
          </div>
        </template>
        <div style="display: flex; flex-direction: column; gap: 12px">
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-tag
              :color="researchProcessingStatus.enabled ? 'blue' : 'default'"
            >
              {{
                researchProcessingStatus.enabled
                  ? "streaming enabled"
                  : "manual scan"
              }}
            </a-tag>
            <a-tag color="purple">
              consumed: {{ researchProcessingStatus.consumedTotal }}
            </a-tag>
            <a-tag color="blue">
              buffered: {{ researchProcessingStatus.bufferedTotal }}
            </a-tag>
            <a-tag color="orange">
              summarized: {{ researchProcessingStatus.summary.total }}
            </a-tag>
            <a-tag color="geekblue">
              queue:
              {{ researchProcessingStatus.queueLen }}/{{
                researchProcessingStatus.queueCap
              }}
            </a-tag>
            <a-tag color="default">
              dropped: {{ researchProcessingStatus.droppedTotal }}
            </a-tag>
            <a-tag color="cyan">
              retention:
              {{ researchProcessingStatus.settings.artifactRetentionDays }}d
            </a-tag>
            <a-tag color="green">
              session cap:
              {{ researchProcessingStatus.settings.maxSessionEvents }}
            </a-tag>
            <a-tag color="gold">
              exports: {{ researchProcessingStatus.settings.exportFormats }}
            </a-tag>
          </div>
          <a-alert
            v-if="researchProcessingStatus.lastError"
            type="warning"
            show-icon
            :message="researchProcessingStatus.lastError"
          />
          <a-alert
            v-else
            type="info"
            show-icon
            message="Backend mirrors the frontend AgentSight research transforms."
            description="The worker builds source/type/comm/trace counts, timeline buckets, process summaries, recent samples, and Research v2 export defaults outside the browser. Use manual scan for offline research when streaming is disabled."
          />
          <a-row :gutter="[16, 16]">
            <a-col :xs="24" :lg="12">
              <div style="font-weight: 600; margin-bottom: 8px">
                Top sources / types
              </div>
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag
                  v-for="item in researchProcessingStatus.summary.bySource"
                  :key="`source-${item.key}`"
                  color="blue"
                >
                  {{ item.key }}: {{ item.count }}
                </a-tag>
                <a-tag
                  v-for="item in researchProcessingStatus.summary.byType"
                  :key="`type-${item.key}`"
                  color="purple"
                >
                  {{ item.key }}: {{ item.count }}
                </a-tag>
              </div>
            </a-col>
            <a-col :xs="24" :lg="12">
              <div style="font-weight: 600; margin-bottom: 8px">
                Timeline buckets
              </div>
              <a-empty
                v-if="researchProcessingStatus.summary.timeline.length === 0"
                description="No timeline buckets"
                :image="false"
              />
              <div v-else style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag
                  v-for="bucket in researchProcessingStatus.summary.timeline"
                  :key="bucket.start"
                  color="geekblue"
                >
                  {{ bucket.time }}: {{ bucket.count }}
                </a-tag>
              </div>
            </a-col>
          </a-row>
          <a-row :gutter="[16, 16]">
            <a-col :xs="24" :lg="12">
              <div style="font-weight: 600; margin-bottom: 8px">
                Top processes
              </div>
              <a-empty
                v-if="
                  researchProcessingStatus.summary.topProcesses.length === 0
                "
                description="No process summaries"
                :image="false"
              />
              <template v-else>
                <a-card
                  v-for="process in researchProcessingStatus.summary.topProcesses"
                  :key="process.pid"
                  size="small"
                  style="margin-bottom: 8px"
                >
                  <div style="display: flex; gap: 8px; flex-wrap: wrap">
                    <a-tag color="blue">
                      {{ process.comm }}#{{ process.pid }}
                    </a-tag>
                    <a-tag color="purple">
                      events: {{ process.eventCount }}
                    </a-tag>
                    <a-tag v-if="process.ppid" color="default">
                      ppid: {{ process.ppid }}
                    </a-tag>
                  </div>
                  <div style="margin-top: 6px">
                    <strong>sources:</strong>
                    {{ compactList(process.sources) }}
                  </div>
                  <div>
                    <strong>children:</strong>
                    {{ compactList(process.childPids) }}
                  </div>
                </a-card>
              </template>
            </a-col>
            <a-col :xs="24" :lg="12">
              <div style="font-weight: 600; margin-bottom: 8px">
                Recent backend samples
              </div>
              <a-empty
                v-if="
                  researchProcessingStatus.summary.recentSamples.length === 0
                "
                description="No backend samples"
                :image="false"
              />
              <template v-else>
                <a-card
                  v-for="sample in researchProcessingStatus.summary.recentSamples"
                  :key="sample.id"
                  size="small"
                  style="margin-bottom: 8px"
                >
                  <div style="display: flex; gap: 8px; flex-wrap: wrap">
                    <a-tag color="blue">{{ sample.source }}</a-tag>
                    <a-tag color="purple">{{ sample.eventType }}</a-tag>
                    <a-tag v-if="sample.pid" color="default">
                      pid: {{ sample.pid }}
                    </a-tag>
                  </div>
                  <div style="margin-top: 6px">{{ sample.title }}</div>
                  <a-typography-text type="secondary">
                    {{ sample.time }}
                  </a-typography-text>
                </a-card>
              </template>
            </a-col>
          </a-row>
        </div>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="eBPF Bootstrap Health" size="small">
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag
                  :color="
                    bootstrapHealth.status === 'ready'
                      ? 'green'
                      : bootstrapHealth.status === 'partial'
                        ? 'orange'
                        : bootstrapHealth.status === 'error'
                          ? 'red'
                          : 'blue'
                  "
                >
                  {{
                    bootstrapHealth.status === "ready"
                      ? "Tracepoints ready"
                      : bootstrapHealth.status === "partial"
                        ? "Tracepoints partially attached"
                        : bootstrapHealth.status === "error"
                          ? "Tracepoint bootstrap error"
                          : "Tracepoint status pending"
                  }}
                </a-tag>
                <a-tag color="blue">{{
                  bootstrapHealth.kernelRelease || "unknown kernel"
                }}</a-tag>
              </div>
              <div>
                compiled tracepoints:
                <strong>{{ bootstrapHealth.compiledCount }}</strong>
              </div>
              <div>
                attached tracepoints:
                <strong>{{ bootstrapHealth.attachedCount }}</strong>
              </div>
              <div>
                skipped tracepoints:
                <strong>{{ bootstrapHealth.skippedCount }}</strong>
              </div>
              <div>
                last observed:
                <strong>{{
                  formatMaybeDate(bootstrapHealth.observedAt)
                }}</strong>
              </div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <a-alert
                :type="
                  bootstrapHealth.status === 'ready'
                    ? 'success'
                    : bootstrapHealth.status === 'partial'
                      ? 'warning'
                      : bootstrapHealth.status === 'error'
                        ? 'error'
                        : 'info'
                "
                show-icon
                :message="
                  bootstrapHealth.message ||
                  'No tracepoint bootstrap status available'
                "
              />
              <a-space wrap>
                <a-tag
                  v-for="tracepoint in bootstrapHealth.skippedTracepoints"
                  :key="tracepoint"
                  color="orange"
                >
                  {{ tracepoint }}
                </a-tag>
              </a-space>
              <a-typography-text
                v-if="
                  bootstrapHealth.status !== 'unknown' &&
                  bootstrapHealth.skippedTracepoints.length === 0
                "
                type="secondary"
              >
                When some tracepoints are missing on the current kernel, the
                backend skips only those hooks and keeps booting with the rest.
              </a-typography-text>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="OpenTelemetry Export Health" size="small">
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag
                  :color="
                    otelHealth.ready
                      ? 'green'
                      : otelHealth.enabled
                        ? 'orange'
                        : 'default'
                  "
                >
                  {{
                    otelHealth.ready
                      ? "OTLP ready"
                      : otelHealth.enabled
                        ? "OTLP waiting / error"
                        : "OTLP disabled"
                  }}
                </a-tag>
                <a-tag color="blue">{{
                  otelHealth.serviceName ||
                  runtimeSettings.otlpServiceName ||
                  "agent-ebpf-filter"
                }}</a-tag>
                <a-tag
                  v-if="
                    otelHealth.evictedRunSpans +
                      otelHealth.evictedTaskSpans +
                      otelHealth.evictedToolSpans >
                    0
                  "
                  color="orange"
                >
                  span capacity reclaim active
                </a-tag>
              </div>
              <div>
                endpoint:
                <strong>{{
                  otelHealth.endpoint ||
                  runtimeSettings.otlpEndpoint ||
                  "not configured"
                }}</strong>
              </div>
              <div>
                queue len:
                <strong
                  >{{ otelHealth.queueLen }} / {{ otelHealth.queueCap }}</strong
                >
              </div>
              <div>
                processed / accepted:
                <strong
                  >{{ otelHealth.processedEvents }} /
                  {{ otelHealth.enqueuedEvents }}</strong
                >
              </div>
              <div>
                exported spans: <strong>{{ otelHealth.exportedSpans }}</strong>
              </div>
              <div>
                dropped events: <strong>{{ otelHealth.droppedEvents }}</strong>
              </div>
              <div>
                last export:
                <strong>{{
                  formatMaybeDate(otelHealth.lastExportedAt)
                }}</strong>
              </div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div>
                active run spans:
                <strong
                  >{{ otelHealth.activeRunSpans }} /
                  {{ otelHealth.maxRunSpans }}</strong
                >
              </div>
              <div>
                active task spans:
                <strong
                  >{{ otelHealth.activeTaskSpans }} /
                  {{ otelHealth.maxTaskSpans }}</strong
                >
              </div>
              <div>
                active tool spans:
                <strong
                  >{{ otelHealth.activeToolSpans }} /
                  {{ otelHealth.maxToolSpans }}</strong
                >
              </div>
              <div>
                capacity evictions (run / task / tool):
                <strong
                  >{{ otelHealth.evictedRunSpans }} /
                  {{ otelHealth.evictedTaskSpans }} /
                  {{ otelHealth.evictedToolSpans }}</strong
                >
              </div>
              <a-alert
                v-if="otelHealth.lastError"
                type="warning"
                show-icon
                :message="otelHealth.lastError"
              />
              <a-typography-text v-else type="secondary">
                Export health comes from <code>/system/otel-health</code>. If
                OTLP is enabled but not ready, check the endpoint, headers, and
                collector reachability.
              </a-typography-text>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Domain Forward Proxy Health" size="small">
        <div style="display: flex; flex-direction: column; gap: 12px">
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-tag :color="domainForwardStatus.enabled ? 'blue' : 'default'">
              {{ domainForwardStatus.enabled ? "enabled" : "disabled" }}
            </a-tag>
            <a-tag
              :color="domainForwardStatus.httpRunning ? 'green' : 'default'"
            >
              HTTP {{ domainForwardStatus.httpRunning ? "running" : "stopped" }}
            </a-tag>
            <a-tag
              :color="domainForwardStatus.httpsRunning ? 'green' : 'default'"
            >
              HTTPS
              {{ domainForwardStatus.httpsRunning ? "running" : "stopped" }}
            </a-tag>
            <a-tag color="blue"
              >routes: {{ domainForwardStatus.routeCount }}</a-tag
            >
            <a-tag v-if="domainForwardStatus.dnsResolver" color="purple">
              DNS {{ domainForwardStatus.dnsResolver }}
            </a-tag>
          </div>
          <div v-if="domainForwardStatus.httpAddress">
            HTTP listener:
            <strong>{{ domainForwardStatus.httpAddress }}</strong>
          </div>
          <div v-if="domainForwardStatus.httpsAddress">
            HTTPS listener:
            <strong>{{ domainForwardStatus.httpsAddress }}</strong>
          </div>
          <a-alert
            v-if="
              domainForwardStatus.errors &&
              domainForwardStatus.errors.length > 0
            "
            type="error"
            show-icon
            :message="domainForwardStatus.errors.join('; ')"
          />
          <a-typography-text v-else type="secondary">
            Forwarder status comes from
            <code>/system/domain-forward/status</code>. Configure it from the
            Runtime Config tab.
          </a-typography-text>
        </div>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Data Cleanup" size="small">
        <template #extra>
          <DeleteOutlined />
        </template>
        <div style="display: flex; flex-direction: column; gap: 12px">
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-popconfirm
              title="Clear in-memory event buffer?"
              @confirm="clearInMemoryEvents"
            >
              <a-button size="small" danger>Clear Memory Events</a-button>
            </a-popconfirm>
            <a-popconfirm
              title="Truncate persisted event log file?"
              @confirm="clearPersistedLog"
            >
              <a-button size="small" danger>Truncate Log File</a-button>
            </a-popconfirm>
            <a-popconfirm
              title="Clear all events (memory + file)?"
              @confirm="clearAllEvents"
            >
              <a-button size="small" type="primary" danger
                >Clear All Events</a-button
              >
            </a-popconfirm>
          </div>
          <a-typography-text type="secondary">
            These actions are irreversible. Memory events and/or the JSONL log
            file will be permanently deleted.
          </a-typography-text>
        </div>
      </a-card>
    </a-col>
  </a-row>
</template>
