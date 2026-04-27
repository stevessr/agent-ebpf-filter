<script setup lang="ts">
import { computed } from 'vue';
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

const pCores = computed(() => props.systemStats.cpuCoresDetailed.filter(c => props.getCoreTypeName(c.type) === 'P-Core'));
const eCores = computed(() => props.systemStats.cpuCoresDetailed.filter(c => props.getCoreTypeName(c.type) === 'E-Core'));
</script>

<template>
  <div style="padding-top: 16px;">
    <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center;">
      <a-radio-group :value="cpuView" @change="(e: any) => emit('update:cpuView', e?.target?.value ?? e)" button-style="solid" size="small">
        <a-radio-button value="overall">Overall</a-radio-button>
        <a-radio-button value="cores">Per Core</a-radio-button>
      </a-radio-group>
      <a-button type="link" size="small" @click="openHistoryChart('CPU Usage History', [{ name: 'Total CPU', data: statsHistory.cpu, color: '#1890ff' }])">History Chart</a-button>
    </div>

    <div v-if="cpuView === 'overall'" style="background: #fafafa; padding: 24px; border-radius: 8px; text-align: center; border: 1px solid #f0f0f0;">
      <a-progress type="dashboard" :percent="Math.round(systemStats.cpuTotal)" :width="180" :stroke-color="systemStats.cpuTotal > 80 ? '#ff4d4f' : '#1890ff'" @click="openHistoryChart('CPU Usage History', [{ name: 'Total CPU', data: statsHistory.cpu, color: '#1890ff' }])" style="cursor: pointer;" />
      <div style="margin-top: 16px; font-size: 18px; font-weight: bold;">System CPU Usage: {{ systemStats.cpuTotal.toFixed(1) }}%</div>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 16px;">
      <!-- P-Cores Section -->
      <div v-if="pCores.length > 0">
        <div style="font-weight: 600; margin-bottom: 8px; color: #1890ff;">
          <a-tag color="#1890ff">P-Core</a-tag> Performance Cores ({{ pCores.length }})
        </div>
        <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px;">
          <div v-for="core in pCores" :key="core.index"
               style="padding: 12px; border: 1px solid #e6f0ff; border-radius: 8px; text-align: center; background: #fafcff; cursor: pointer; transition: all 0.2s;"
               class="core-card core-card--p"
               @click="openHistoryChart(`P-Core #${core.index} Usage History`, [{ name: `P-Core #${core.index}`, data: statsHistory.cores[core.index] || [], color: getCoreTypeColor(core.type) }])">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
              <span style="font-family: monospace; font-size: 11px; font-weight: bold;">#{{ core.index }}</span>
              <a-tag :color="getCoreTypeColor(core.type)" style="font-size: 9px; padding: 0 4px; line-height: 16px;">{{ getCoreTypeName(core.type) }}</a-tag>
            </div>
            <a-progress type="dashboard" :percent="Math.round(core.usage)" :width="70" :stroke-color="core.usage > 80 ? '#ff4d4f' : getCoreTypeColor(core.type)" />
          </div>
        </div>
      </div>

      <!-- E-Cores Section -->
      <div v-if="eCores.length > 0">
        <div style="font-weight: 600; margin-bottom: 8px; color: #52c41a;">
          <a-tag color="#52c41a">E-Core</a-tag> Efficiency Cores ({{ eCores.length }})
        </div>
        <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px;">
          <div v-for="core in eCores" :key="core.index"
               style="padding: 12px; border: 1px solid #e6ffe6; border-radius: 8px; text-align: center; background: #fafffa; cursor: pointer; transition: all 0.2s;"
               class="core-card core-card--e"
               @click="openHistoryChart(`E-Core #${core.index} Usage History`, [{ name: `E-Core #${core.index}`, data: statsHistory.cores[core.index] || [], color: getCoreTypeColor(core.type) }])">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
              <span style="font-family: monospace; font-size: 11px; font-weight: bold;">#{{ core.index }}</span>
              <a-tag :color="getCoreTypeColor(core.type)" style="font-size: 9px; padding: 0 4px; line-height: 16px;">{{ getCoreTypeName(core.type) }}</a-tag>
            </div>
            <a-progress type="dashboard" :percent="Math.round(core.usage)" :width="70" :stroke-color="core.usage > 80 ? '#ff4d4f' : getCoreTypeColor(core.type)" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.core-card--p:hover {
  border-color: #91caff !important;
  background: #f0f7ff !important;
  box-shadow: 0 2px 8px rgba(24, 144, 255, 0.12);
}
.core-card--e:hover {
  border-color: #95de64 !important;
  background: #f6ffed !important;
  box-shadow: 0 2px 8px rgba(82, 196, 26, 0.12);
}
</style>
