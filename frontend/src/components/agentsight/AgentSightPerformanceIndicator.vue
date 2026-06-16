<script setup lang="ts">
import { computed } from "vue";
import { InfoCircleOutlined } from "@ant-design/icons-vue";

interface PerformanceIndicatorProps {
  eventCount: number;
  processingTime?: number;
  memoryUsage?: number;
}

const props = defineProps<PerformanceIndicatorProps>();

const performanceLevel = computed(() => {
  if (props.eventCount < 1000) return "good";
  if (props.eventCount < 5000) return "moderate";
  return "heavy";
});

const performanceColor = computed(() => {
  switch (performanceLevel.value) {
    case "good":
      return "green";
    case "moderate":
      return "gold";
    case "heavy":
      return "orange";
    default:
      return "default";
  }
});

const performanceText = computed(() => {
  switch (performanceLevel.value) {
    case "good":
      return "Optimal performance";
    case "moderate":
      return "Good performance";
    case "heavy":
      return "Large dataset - may be slow";
    default:
      return "";
  }
});

const formattedTime = computed(() => {
  if (!props.processingTime) return null;
  return props.processingTime < 1000
    ? `${Math.round(props.processingTime)}ms`
    : `${(props.processingTime / 1000).toFixed(2)}s`;
});

const formattedMemory = computed(() => {
  if (!props.memoryUsage) return null;
  return `${(props.memoryUsage / 1024 / 1024).toFixed(1)}MB`;
});
</script>

<template>
  <a-tooltip>
    <template #title>
      <div class="performance-tooltip">
        <div v-if="formattedTime">Processing time: {{ formattedTime }}</div>
        <div v-if="formattedMemory">Memory usage: {{ formattedMemory }}</div>
        <div>{{ performanceText }}</div>
      </div>
    </template>
    <a-tag :color="performanceColor" style="cursor: help">
      <InfoCircleOutlined /> {{ eventCount.toLocaleString() }} events
    </a-tag>
  </a-tooltip>
</template>

<style scoped>
.performance-tooltip {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
</style>
