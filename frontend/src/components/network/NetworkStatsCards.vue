<script setup lang="ts">
interface Stats {
  outgoing: number;
  incoming: number;
  listening: number;
  uniquePids: number;
  uniqueEndpoints: number;
  totalBytes: number;
}

defineProps<{
  totalEvents: number;
  stats: Stats;
  formatBytes: (bytes: number) => string;
}>();
</script>

<template>
  <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
    <a-col :xs="12" :lg="4">
      <a-card size="small" :bordered="false">
        <a-statistic title="Packets" :value="totalEvents" />
      </a-card>
    </a-col>
    <a-col :xs="12" :lg="4">
      <a-card size="small" :bordered="false">
        <a-statistic title="Outgoing" :value="stats.outgoing" />
      </a-card>
    </a-col>
    <a-col :xs="12" :lg="4">
      <a-card size="small" :bordered="false">
        <a-statistic title="Incoming" :value="stats.incoming" />
      </a-card>
    </a-col>
    <a-col :xs="12" :lg="4">
      <a-card size="small" :bordered="false">
        <a-statistic title="Listening" :value="stats.listening" />
      </a-card>
    </a-col>
    <a-col :xs="12" :lg="4">
      <a-card size="small" :bordered="false">
        <a-statistic title="Endpoints" :value="stats.uniqueEndpoints" />
      </a-card>
    </a-col>
    <a-col :xs="12" :lg="4">
      <a-card size="small" :bordered="false">
        <a-statistic title="Traffic" :value="formatBytes(stats.totalBytes)" />
      </a-card>
    </a-col>
  </a-row>
</template>
