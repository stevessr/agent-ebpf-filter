<script setup lang="ts">
import type { useConfigRuntime } from "../../../composables/config/useConfigRuntime";

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
              <strong>{{ formatMaybeDate(bootstrapHealth.observedAt) }}</strong>
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
              <strong>{{ formatMaybeDate(otelHealth.lastExportedAt) }}</strong>
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
              Export health comes from <code>/system/otel-health</code>. If OTLP
              is enabled but not ready, check the endpoint, headers, and
              collector reachability.
            </a-typography-text>
          </div>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
