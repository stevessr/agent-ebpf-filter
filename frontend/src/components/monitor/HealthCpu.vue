<script setup lang="ts">
import type { GlobalStats, StatsHistory } from '../../composables/useMonitorData';

const props = defineProps<{
  systemStats: GlobalStats;
  statsHistory: StatsHistory;
  cpuView: 'overall' | 'cores';
  getCoreTypeColor: (type: number) => string;
  getCoreTypeName: (type: number) => string;
  openHistoryChart: (title: string, datasets: { name: string; data: { time: number; value: number }[]; color?: string }[]) => void;
}>();

const emit = defineEmits<{
  'update:cpuView': [value: 'overall' | 'cores'];
}>();
</script>

<template>
  <div style="padding-top: 16px;">
    <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center;">
      <a-radio-group :value="cpuView" @change="emit('update:cpuView', ($event.target as any).value)" button-style="solid" size="small">
        <a-radio-button value="overall">Overall</a-radio-button>
        <a-radio-button value="cores">Per Core</a-radio-button>
      </a-radio-group>
      <a-button type="link" size="small" @click="openHistoryChart('CPU Usage History', [{ name: 'Total CPU', data: statsHistory.cpu, color: '#1890ff' }])">History Chart</a-button>
    </div>

    <div v-if="cpuView === 'overall'" style="background: #fafafa; padding: 24px; border-radius: 8px; text-align: center; border: 1px solid #f0f0f0;">
      <a-progress type="dashboard" :percent="Math.round(systemStats.cpuTotal)" :width="180" :stroke-color="systemStats.cpuTotal > 80 ? '#ff4d4f' : '#1890ff'" @click="openHistoryChart('CPU Usage History', [{ name: 'Total CPU', data: statsHistory.cpu, color: '#1890ff' }])" style="cursor: pointer;" />
      <div style="margin-top: 16px; font-size: 18px; font-weight: bold;">System CPU Usage: {{ systemStats.cpuTotal.toFixed(1) }}%</div>
    </div>

    <div v-else style="display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px;">
      <div v-for="core in systemStats.cpuCoresDetailed" :key="core.index"
           style="padding: 12px; border: 1px solid #f0f0f0; border-radius: 8px; text-align: center; background: #fafafa; cursor: pointer; transition: all 0.2s;"
           class="core-card"
           @click="openHistoryChart(`Core #${core.index} Usage History`, [{ name: `Core #${core.index}`, data: statsHistory.cores[core.index] || [], color: getCoreTypeColor(core.type) }])">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
          <span style="font-family: monospace; font-size: 11px; font-weight: bold;">#{{ core.index }}</span>
          <a-tag :color="getCoreTypeColor(core.type)" style="font-size: 9px; padding: 0 4px; line-height: 16px;">{{ getCoreTypeName(core.type) }}</a-tag>
        </div>
        <a-progress type="dashboard" :percent="Math.round(core.usage)" :width="70" :stroke-color="core.usage > 80 ? '#ff4d4f' : getCoreTypeColor(core.type)" />
      </div>
    </div>
  </div>
</template>
