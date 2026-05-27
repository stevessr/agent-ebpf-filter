<script setup lang="ts">
import { DeleteOutlined, ReloadOutlined } from '@ant-design/icons-vue';
import type { useConfigRuntime } from '../../composables/config/useConfigRuntime';

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
}>();

const {
  runtimeSettings, bootstrapHealth, collectorHealth, otelHealth, domainForwardStatus,
  fetchCollectorHealth, clearInMemoryEvents, clearPersistedLog, clearAllEvents,
} = props.runtime;

const formatLatencyMs = (value?: number) => {
  if (!value) return '0 ms';
  return `${(value / 1_000_000).toFixed(value >= 1_000_000 ? 2 : 3)} ms`;
};

const formatMaybeDate = (value?: string) => {
  if (!value) return 'Not exported yet';
  return value;
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
                <a-tag :color="collectorHealth.captureHealthy ? 'green' : 'red'">
                  {{ collectorHealth.captureHealthy ? 'Capture healthy' : 'Ringbuf drops detected' }}
                </a-tag>
                <a-tag :color="collectorHealth.collectorMapAvailable ? 'blue' : 'default'">
                  {{ collectorHealth.collectorMapAvailable ? 'collector_stats available' : 'collector_stats unavailable' }}
                </a-tag>
              </div>
              <div>ringbuf events: <strong>{{ collectorHealth.ringbufEventsTotal }}</strong></div>
              <div>ringbuf reserve failed: <strong>{{ collectorHealth.ringbufReserveFailedTotal }}</strong></div>
              <div>backend queue len: <strong>{{ collectorHealth.backendQueueLen }}</strong></div>
              <div>WS clients: <strong>{{ collectorHealth.wsClients }}</strong></div>
              <div>last persisted append latency: <strong>{{ formatLatencyMs(collectorHealth.persistAppendLatencyNs) }}</strong></div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 8px">
              <div style="font-weight: 600">Events by type</div>
              <a-empty v-if="Object.keys(collectorHealth.eventsByTypeTotal || {}).length === 0" description="No events yet" :image="false" />
              <div v-else style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag v-for="(count, type) in collectorHealth.eventsByTypeTotal" :key="type" color="default">
                  {{ type }}: {{ count }}
                </a-tag>
              </div>
              <a-alert
                v-if="collectorHealth.ringbufDroppedTotal > 0"
                type="warning"
                show-icon
                message="Sampling may be incomplete because the kernel ringbuf dropped events."
              />
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="eBPF Bootstrap Health" size="small">
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag :color="bootstrapHealth.status === 'ready' ? 'green' : bootstrapHealth.status === 'partial' ? 'orange' : bootstrapHealth.status === 'error' ? 'red' : 'blue'">
                  {{ bootstrapHealth.status === 'ready' ? 'Tracepoints ready' : bootstrapHealth.status === 'partial' ? 'Tracepoints partially attached' : bootstrapHealth.status === 'error' ? 'Tracepoint bootstrap error' : 'Tracepoint status pending' }}
                </a-tag>
                <a-tag color="blue">{{ bootstrapHealth.kernelRelease || 'unknown kernel' }}</a-tag>
              </div>
              <div>compiled tracepoints: <strong>{{ bootstrapHealth.compiledCount }}</strong></div>
              <div>attached tracepoints: <strong>{{ bootstrapHealth.attachedCount }}</strong></div>
              <div>skipped tracepoints: <strong>{{ bootstrapHealth.skippedCount }}</strong></div>
              <div>last observed: <strong>{{ formatMaybeDate(bootstrapHealth.observedAt) }}</strong></div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <a-alert
                :type="bootstrapHealth.status === 'ready' ? 'success' : bootstrapHealth.status === 'partial' ? 'warning' : bootstrapHealth.status === 'error' ? 'error' : 'info'"
                show-icon
                :message="bootstrapHealth.message || 'No tracepoint bootstrap status available'"
              />
              <a-space wrap>
                <a-tag v-for="tracepoint in bootstrapHealth.skippedTracepoints" :key="tracepoint" color="orange">
                  {{ tracepoint }}
                </a-tag>
              </a-space>
              <a-typography-text v-if="bootstrapHealth.status !== 'unknown' && bootstrapHealth.skippedTracepoints.length === 0" type="secondary">
                When some tracepoints are missing on the current kernel, the backend skips only those hooks and keeps booting with the rest.
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
                <a-tag :color="otelHealth.ready ? 'green' : (otelHealth.enabled ? 'orange' : 'default')">
                  {{ otelHealth.ready ? 'OTLP ready' : (otelHealth.enabled ? 'OTLP waiting / error' : 'OTLP disabled') }}
                </a-tag>
                <a-tag color="blue">{{ otelHealth.serviceName || runtimeSettings.otlpServiceName || 'agent-ebpf-filter' }}</a-tag>
              </div>
              <div>endpoint: <strong>{{ otelHealth.endpoint || runtimeSettings.otlpEndpoint || 'not configured' }}</strong></div>
              <div>queue len: <strong>{{ otelHealth.queueLen }}</strong></div>
              <div>exported spans: <strong>{{ otelHealth.exportedSpans }}</strong></div>
              <div>dropped events: <strong>{{ otelHealth.droppedEvents }}</strong></div>
              <div>last export: <strong>{{ formatMaybeDate(otelHealth.lastExportedAt) }}</strong></div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div>active run spans: <strong>{{ otelHealth.activeRunSpans }}</strong></div>
              <div>active task spans: <strong>{{ otelHealth.activeTaskSpans }}</strong></div>
              <div>active tool spans: <strong>{{ otelHealth.activeToolSpans }}</strong></div>
              <a-alert
                v-if="otelHealth.lastError"
                type="warning"
                show-icon
                :message="otelHealth.lastError"
              />
              <a-typography-text v-else type="secondary">
                Export health comes from <code>/system/otel-health</code>. If OTLP is enabled but not ready, check the endpoint, headers, and collector reachability.
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
              {{ domainForwardStatus.enabled ? 'enabled' : 'disabled' }}
            </a-tag>
            <a-tag :color="domainForwardStatus.httpRunning ? 'green' : 'default'">
              HTTP {{ domainForwardStatus.httpRunning ? 'running' : 'stopped' }}
            </a-tag>
            <a-tag :color="domainForwardStatus.httpsRunning ? 'green' : 'default'">
              HTTPS {{ domainForwardStatus.httpsRunning ? 'running' : 'stopped' }}
            </a-tag>
            <a-tag color="blue">routes: {{ domainForwardStatus.routeCount }}</a-tag>
            <a-tag v-if="domainForwardStatus.dnsResolver" color="purple">
              DNS {{ domainForwardStatus.dnsResolver }}
            </a-tag>
          </div>
          <div v-if="domainForwardStatus.httpAddress">HTTP listener: <strong>{{ domainForwardStatus.httpAddress }}</strong></div>
          <div v-if="domainForwardStatus.httpsAddress">HTTPS listener: <strong>{{ domainForwardStatus.httpsAddress }}</strong></div>
          <a-alert
            v-if="domainForwardStatus.errors && domainForwardStatus.errors.length > 0"
            type="error"
            show-icon
            :message="domainForwardStatus.errors.join('; ')"
          />
          <a-typography-text v-else type="secondary">
            Forwarder status comes from <code>/system/domain-forward/status</code>. Configure it from the Runtime Config tab.
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
            <a-popconfirm title="Clear in-memory event buffer?" @confirm="clearInMemoryEvents">
              <a-button size="small" danger>Clear Memory Events</a-button>
            </a-popconfirm>
            <a-popconfirm title="Truncate persisted event log file?" @confirm="clearPersistedLog">
              <a-button size="small" danger>Truncate Log File</a-button>
            </a-popconfirm>
            <a-popconfirm title="Clear all events (memory + file)?" @confirm="clearAllEvents">
              <a-button size="small" type="primary" danger>Clear All Events</a-button>
            </a-popconfirm>
          </div>
          <a-typography-text type="secondary">
            These actions are irreversible. Memory events and/or the JSONL log file will be permanently deleted.
          </a-typography-text>
        </div>
      </a-card>
    </a-col>
  </a-row>
</template>
