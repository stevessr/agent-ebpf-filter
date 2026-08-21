<script setup lang="ts">
import { DeleteOutlined } from "@ant-design/icons-vue";
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
    <a-card title="Domain Forward Proxy Health" size="small">
      <div style="display: flex; flex-direction: column; gap: 12px">
        <div style="display: flex; gap: 8px; flex-wrap: wrap">
          <a-tag :color="domainForwardStatus.enabled ? 'blue' : 'default'">
            {{ domainForwardStatus.enabled ? "enabled" : "disabled" }}
          </a-tag>
          <a-tag :color="domainForwardStatus.httpRunning ? 'green' : 'default'">
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
            domainForwardStatus.errors && domainForwardStatus.errors.length > 0
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
</template>
