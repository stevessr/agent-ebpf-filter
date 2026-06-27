<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";
import { useMonitorData } from "../../composables/monitor/useMonitorData";
import ProcessObserverPanel from "../../components/monitor/ProcessObserverPanel.vue";

const {
  processes,
  systemStats,
  setup,
  teardown,
  sendProcessSignal,
} = useMonitorData();

onMounted(() => {
  setup();
});

onUnmounted(() => {
  teardown();
});

const onSendProcessSignal = async (pid: number, signal: string) => {
  const { message } = await import("ant-design-vue");
  const ok = await sendProcessSignal(pid, signal);
  if (ok) message.success(`Signal ${signal.toUpperCase()} sent to PID ${pid}`);
  else message.error(`Failed to send ${signal}`);
};
</script>

<template>
  <div style="background: #f0f2f5; padding: 20px; min-height: 100%">
    <ProcessObserverPanel
      :processes="processes"
      :sendProcessSignal="onSendProcessSignal"
      :isActive="true"
      :mem-total="systemStats.memTotal"
    />
  </div>
</template>
