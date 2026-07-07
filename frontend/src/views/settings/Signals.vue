<script setup lang="ts">
import { onMounted } from "vue";
import { ThunderboltOutlined } from "@ant-design/icons-vue";
import ConfigSignalsTab from "../../components/config/ConfigSignalsTab.vue";
import { useConfigRuntime } from "../../composables/config/useConfigRuntime";
import { useConfigRegistry } from "../../composables/config/useConfigRegistry";

const runtime = useConfigRuntime();
const registry = useConfigRegistry();

const {
  fetchRuntime,
  fetchSignalProcessingStatus,
  fetchSignalProgramLogs,
} = runtime;

const { fetchTags, fetchTrackedComms, fetchTrackedPaths, fetchTrackedPrefixes } =
  registry;

onMounted(() => {
  void Promise.allSettled([
    fetchTags(),
    fetchTrackedComms(),
    fetchTrackedPaths(),
    fetchTrackedPrefixes(),
    fetchRuntime(),
    fetchSignalProcessingStatus(),
    fetchSignalProgramLogs(),
  ]);
});
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%">
    <a-page-header
      title="Signal Processing"
      sub-title="Lazy, TTL-decayed runtime signal management"
      style="padding: 0; margin-bottom: 24px"
    >
      <template #backIcon>
        <ThunderboltOutlined />
      </template>
    </a-page-header>
    <ConfigSignalsTab :runtime="runtime" :registry="registry" />
  </div>
</template>