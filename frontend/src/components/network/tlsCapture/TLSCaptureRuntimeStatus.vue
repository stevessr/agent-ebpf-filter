<script setup lang="ts">
import type { TLSCaptureStatus } from "../../../types/tls";
import { TLS_CAPTURE_DEFAULT_QUEUE_CAPACITY } from "../../../views/network/tlsCapture/constants";

defineProps<{
  captureStatus: TLSCaptureStatus;
  captureStatusText: string;
  captureStatusColor: string;
  isConnected: boolean;
  attachedLibraries: number;
  attachLoading: boolean;
}>();

defineEmits<{
  attach: [];
  refresh: [];
}>();
</script>

<template>
  <a-card size="small" class="tls-runtime-card">
    <a-space wrap>
      <a-tag :color="captureStatusColor">{{ captureStatusText }}</a-tag>
      <a-tag :color="isConnected ? 'green' : 'red'">
        WebSocket {{ isConnected ? "live" : "offline" }}
      </a-tag>
      <a-tag color="blue">{{ attachedLibraries }} attached libraries</a-tag>
      <a-tag color="cyan">
        {{ captureStatus.broadcast?.activeClients ?? 0 }} broadcast clients
      </a-tag>
      <a-tag color="geekblue">
        queue {{ captureStatus.broadcast?.queuedEvents ?? 0 }}/{{
          (captureStatus.broadcast?.queueCapacity ??
            TLS_CAPTURE_DEFAULT_QUEUE_CAPACITY) *
          (captureStatus.broadcast?.activeClients ?? 0)
        }}
      </a-tag>
      <a-tag
        v-if="(captureStatus.broadcast?.queueFullDropsTotal ?? 0) > 0"
        color="orange"
      >
        queue drops {{ captureStatus.broadcast?.queueFullDropsTotal }}
      </a-tag>
      <a-tag
        v-if="
          (captureStatus.broadcast?.writeFailuresTotal ?? 0) +
            (captureStatus.broadcast?.writeDeadlineFailuresTotal ?? 0) >
          0
        "
        color="red"
      >
        write failures
        {{
          (captureStatus.broadcast?.writeFailuresTotal ?? 0) +
          (captureStatus.broadcast?.writeDeadlineFailuresTotal ?? 0)
        }}
      </a-tag>
      <a-button
        type="primary"
        size="small"
        :loading="attachLoading"
        @click="$emit('attach')"
      >
        Start / Attach SSL hooks
      </a-button>
      <a-button
        size="small"
        :loading="attachLoading"
        @click="$emit('refresh')"
      >
        Refresh status
      </a-button>
    </a-space>
    <a-alert
      v-if="captureStatus.error"
      type="warning"
      show-icon
      class="tls-runtime-error"
      :message="captureStatus.error"
    />
  </a-card>
</template>

<style scoped>
.tls-runtime-card {
  margin-bottom: 16px;
}

.tls-runtime-error {
  margin-top: 10px;
}
</style>
