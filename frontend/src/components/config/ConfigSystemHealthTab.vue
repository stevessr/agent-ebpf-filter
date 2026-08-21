<script setup lang="ts">
import { DeleteOutlined, ReloadOutlined } from "@ant-design/icons-vue";
import HealthCollectionSection from "./SystemHealthTab/HealthCollectionSection.vue";
import HealthExportersSection from "./SystemHealthTab/HealthExportersSection.vue";
import HealthMaintenanceSection from "./SystemHealthTab/HealthMaintenanceSection.vue";
import HealthWorkersSection from "./SystemHealthTab/HealthWorkersSection.vue";
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
    <HealthCollectionSection :runtime="runtime" />
    <HealthWorkersSection :runtime="runtime" />
    <HealthExportersSection :runtime="runtime" />
    <HealthMaintenanceSection :runtime="runtime" />
  </a-row>
</template>
