<script setup lang="ts">
import type { GlobalStats, StatsHistory } from '../../composables/useMonitorData';

defineProps<{
  systemStats: GlobalStats;
  statsHistory: StatsHistory;
  formatBytesWithUnit: (bytes: number) => string;
  openHistoryChart: (title: string, datasets: { name: string; data: { time: number; value: number }[]; color?: string }[]) => void;
}>();
</script>

<template>
  <a-row :gutter="16" style="padding-top: 16px;">
    <a-col :span="12">
      <a-card title="Physical Memory" size="small" :bordered="false" style="background: #fafafa;">
        <template #extra>
          <a-button type="link" size="small" @click="openHistoryChart('Memory Usage History', [
            { name: 'Used', data: statsHistory.memUsed, color: '#1890ff' },
            { name: 'Cached', data: statsHistory.memCached, color: '#52c41a' },
            { name: 'Buffers', data: statsHistory.memBuffers, color: '#faad14' }
          ])">History (All)</a-button>
        </template>
        <a-statistic title="Overall Usage" :value="systemStats.memPercent" suffix="%" :precision="1" @click="openHistoryChart('Memory Usage History (%)', [{ name: 'Mem %', data: statsHistory.mem, color: '#52c41a' }])" style="cursor: pointer;" />
        <div style="margin-top: 16px; display: grid; gap: 8px;">
          <div style="display: flex; justify-content: space-between;"><span>Total:</span><b>{{ formatBytesWithUnit(systemStats.memTotal) }}</b></div>
          <div style="display: flex; justify-content: space-between; color: #1890ff; cursor: pointer;" @click="openHistoryChart('Used Memory Usage History', [{ name: 'Used', data: statsHistory.memUsed, color: '#1890ff' }])"><span>Used:</span><b>{{ formatBytesWithUnit(systemStats.memUsed) }}</b></div>
          <div style="display: flex; justify-content: space-between; color: #52c41a; cursor: pointer;" @click="openHistoryChart('Cached Memory Usage History', [{ name: 'Cached', data: statsHistory.memCached, color: '#52c41a' }])"><span>Cached:</span><b>{{ formatBytesWithUnit(systemStats.memCached) }}</b></div>
          <div style="display: flex; justify-content: space-between; color: #faad14; cursor: pointer;" @click="openHistoryChart('Buffers Memory Usage History', [{ name: 'Buffers', data: statsHistory.memBuffers, color: '#faad14' }])"><span>Buffers:</span><b>{{ formatBytesWithUnit(systemStats.memBuffers) }}</b></div>
        </div>
      </a-card>
    </a-col>
    <a-col :span="12">
      <a-card title="Swap / ZRAM" size="small" :bordered="false" style="background: #fafafa;">
        <template #extra>
          <a-button type="link" size="small" @click="openHistoryChart('Swap/ZRAM Usage History', [
            { name: 'Swap Used', data: statsHistory.swapUsage, color: '#722ed1' },
            { name: 'ZRAM Used', data: statsHistory.zramUsage, color: '#13c2c2' }
          ])">History</a-button>
        </template>
        <div style="display: grid; gap: 16px;">
          <div v-if="systemStats.swapTotal > 0" style="cursor: pointer;" @click="openHistoryChart('Swap Usage History', [{ name: 'Swap Used', data: statsHistory.swapUsage, color: '#722ed1' }])">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
              <span style="font-size: 12px; color: #888;">System Swap</span>
              <span style="font-weight: bold;">{{ systemStats.swapTotal > 0 ? ((systemStats.swapUsed / systemStats.swapTotal) * 100).toFixed(1) : 0 }}%</span>
            </div>
            <a-progress :percent="systemStats.swapTotal > 0 ? Math.round((systemStats.swapUsed / systemStats.swapTotal) * 100) : 0" size="small" stroke-color="#722ed1" />
            <div style="display: flex; justify-content: space-between; font-size: 11px; margin-top: 2px;">
              <span>Used: {{ formatBytesWithUnit(systemStats.swapUsed) }}</span>
              <span>Total: {{ formatBytesWithUnit(systemStats.swapTotal) }}</span>
            </div>
          </div>
          <div v-if="systemStats.zramTotal > 0" style="cursor: pointer;" @click="openHistoryChart('ZRAM Usage History', [{ name: 'ZRAM Used', data: statsHistory.zramUsage, color: '#13c2c2' }])">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
              <span style="font-size: 12px; color: #888;">ZRAM (Compressed)</span>
              <span style="font-weight: bold;">{{ systemStats.zramTotal > 0 ? ((systemStats.zramUsed / systemStats.zramTotal) * 100).toFixed(1) : 0 }}%</span>
            </div>
            <a-progress :percent="systemStats.zramTotal > 0 ? Math.round((systemStats.zramUsed / systemStats.zramTotal) * 100) : 0" size="small" stroke-color="#13c2c2" />
            <div style="display: flex; justify-content: space-between; font-size: 11px; margin-top: 2px;">
              <span>Compressed: {{ formatBytesWithUnit(systemStats.zramUsed) }}</span>
              <span>Original: {{ formatBytesWithUnit(systemStats.zramTotal) }}</span>
            </div>
          </div>
          <a-empty v-if="systemStats.swapTotal === 0 && systemStats.zramTotal === 0" description="No Swap/ZRAM detected" />
        </div>
      </a-card>
    </a-col>
  </a-row>
</template>
