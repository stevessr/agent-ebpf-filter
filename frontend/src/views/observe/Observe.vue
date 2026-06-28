<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";
import { useRoute } from "vue-router";
import { useMonitorData } from "../../composables/monitor/useMonitorData";
import ProcessObserverPanel from "../../components/monitor/ProcessObserverPanel.vue";

const route = useRoute();

const {
  processes,
  systemStats,
  setup,
  teardown,
  sendProcessSignal,
} = useMonitorData();

onMounted(() => {
  setup();

  // If the route has a ?pid=XXX query param, pre-select it.
  // The observer composable reads from localStorage; we write it here
  // so ProcessObserverPanel picks it up on mount.
  const pidParam = route.query.pid;
  if (pidParam) {
    const parsed = parseInt(String(pidParam), 10);
    if (!isNaN(parsed) && parsed > 0) {
      try {
        localStorage.setItem("observe-selected-pid", String(parsed));
      } catch { /* ignore */ }
    }
  }
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
