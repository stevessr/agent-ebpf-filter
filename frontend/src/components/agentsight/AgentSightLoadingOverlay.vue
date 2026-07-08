<script setup lang="ts">
import { computed } from "vue";
import { LoadingOutlined, ExclamationCircleOutlined } from "@ant-design/icons-vue";

interface LoadingOverlayProps {
  loading: boolean;
  error?: string | null;
  isEmpty?: boolean;
  emptyMessage?: string;
}

const props = withDefaults(defineProps<LoadingOverlayProps>(), {
  error: null,
  isEmpty: false,
  emptyMessage: "No data available",
});

const showOverlay = computed(() => props.loading || props.error || props.isEmpty);
</script>

<template>
  <div v-if="showOverlay" class="agentsight-loading-overlay">
    <div v-if="loading" class="loading-spinner">
      <LoadingOutlined :style="{ fontSize: '48px', color: '#1890ff' }" spin />
      <p class="loading-text">Processing events...</p>
    </div>

    <div v-else-if="error" class="error-state">
      <ExclamationCircleOutlined :style="{ fontSize: '48px', color: '#ff4d4f' }" />
      <p class="error-text">{{ error }}</p>
      <slot name="error-actions" />
    </div>

    <div v-else-if="isEmpty" class="empty-state">
      <p class="empty-text">{{ emptyMessage }}</p>
      <slot name="empty-actions" />
    </div>
  </div>
</template>

<style scoped>
.agentsight-loading-overlay {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  background: rgba(255, 255, 255, 0.9);
  border-radius: 8px;
}

.loading-spinner,
.error-state,
.empty-state {
  text-align: center;
  padding: 40px;
}

.loading-text,
.error-text,
.empty-text {
  margin-top: 16px;
  font-size: 16px;
  color: #4a4a4a;
}

.error-text {
  color: #ff4d4f;
  font-weight: 500;
}
</style>
