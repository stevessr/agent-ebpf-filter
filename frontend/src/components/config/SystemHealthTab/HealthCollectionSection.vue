<script setup lang="ts">
import { ReloadOutlined } from "@ant-design/icons-vue";
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
              <a-tag :color="collectorHealth.captureHealthy ? 'green' : 'red'">
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
              <strong>{{ collectorHealth.ringbufZeroCopyDecodeTotal }}</strong>
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
                Object.keys(collectorHealth.eventsByTypeTotal || {}).length ===
                0
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
            <div style="font-weight: 600">Semantic correlation state</div>
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <a-tag color="blue">
                entries: {{ collectorHealth.semanticStateEntries }}/{{
                  collectorHealth.semanticStateMaxEntries
                }}
              </a-tag>
              <a-tag color="default">
                expired:
                {{ collectorHealth.semanticStateExpiredEvictionsTotal }}
              </a-tag>
              <a-tag
                :color="
                  collectorHealth.semanticStateCapacityEvictionsTotal > 0
                    ? 'orange'
                    : 'default'
                "
              >
                capacity evictions:
                {{ collectorHealth.semanticStateCapacityEvictionsTotal }}
              </a-tag>
              <a-tag color="purple">
                bounded values:
                {{ collectorHealth.semanticStateTruncatedValuesTotal }}
              </a-tag>
              <a-tag
                :color="
                  collectorHealth.semanticStateIgnoredOversizedMetadataTotal > 0
                    ? 'orange'
                    : 'default'
                "
              >
                ignored metadata:
                {{ collectorHealth.semanticStateIgnoredOversizedMetadataTotal }}
              </a-tag>
            </div>
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <a-tag
                v-for="(
                  count, kind
                ) in collectorHealth.semanticStateEntriesByKind"
                :key="kind"
                color="default"
              >
                {{ kind }}: {{ count }}
              </a-tag>
            </div>
            <div>
              last semantic GC:
              <strong>{{
                formatMaybeDate(collectorHealth.semanticStateLastSweepAt)
              }}</strong>
            </div>
            <a-divider style="margin: 4px 0" />
            <div style="font-weight: 600">Tool behavior baseline</div>
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <a-tag color="blue">
                tools: {{ collectorHealth.toolBaselineTools }}/{{
                  collectorHealth.toolBaselineMaxTools
                }}
              </a-tag>
              <a-tag color="cyan">
                samples: {{ collectorHealth.toolBaselineSamples }}/{{
                  collectorHealth.toolBaselineMaxSamples
                }}
              </a-tag>
              <a-tag color="default">
                per-tool cap:
                {{ collectorHealth.toolBaselineMaxSamplesPerTool }}
              </a-tag>
              <a-tag color="purple">
                observations:
                {{ collectorHealth.toolBaselineObservationsTotal }}
              </a-tag>
              <a-tag
                :color="
                  collectorHealth.toolBaselineDriftsTotal > 0
                    ? 'orange'
                    : 'default'
                "
              >
                drifts: {{ collectorHealth.toolBaselineDriftsTotal }}
              </a-tag>
              <a-tag color="default">
                expired:
                {{ collectorHealth.toolBaselineExpiredEvictionsTotal }}
              </a-tag>
              <a-tag
                :color="
                  collectorHealth.toolBaselineCapacityEvictionsTotal > 0
                    ? 'orange'
                    : 'default'
                "
              >
                capacity evictions:
                {{ collectorHealth.toolBaselineCapacityEvictionsTotal }}
              </a-tag>
              <a-tag color="purple">
                bounded values:
                {{ collectorHealth.toolBaselineTruncatedValuesTotal }}
              </a-tag>
            </div>
            <div>
              last baseline GC:
              <strong>{{
                formatMaybeDate(collectorHealth.toolBaselineLastSweepAt)
              }}</strong>
            </div>
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
</template>
