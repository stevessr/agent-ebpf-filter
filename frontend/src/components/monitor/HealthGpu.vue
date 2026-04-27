<script setup lang="ts">
import type { GPUStatus, StatsHistory } from '../../composables/useMonitorData';

const props = defineProps<{
  gpus: GPUStatus[];
  statsHistory: StatsHistory;
  openHistoryChart: (title: string, datasets: { name: string; data: { time: number; value: number }[]; color?: string }[]) => void;
}>();

const isIntegrated = (gpu: GPUStatus) => {
  const name = gpu.name.toLowerCase();
  if (name.includes('intel') && (name.includes('uhd') || name.includes('hd graphics') || name.includes('iris') || name.includes('arc graphics'))) return true;
  if (gpu.memTotal === 0) return true;
  return false;
};

const gpuColor = (pct: number) => pct > 80 ? '#ff4d4f' : '#722ed1';
const memColor = (pct: number) => pct > 80 ? '#ff4d4f' : '#13c2c2';
const tempColor = (t: number) => t > 80 ? '#ff4d4f' : t > 65 ? '#faad14' : '#52c41a';

// Suppress the built-in 100% success checkmark
const fmtPct = (val: number) => () => `${Math.round(val ?? 0)}%`;
</script>

<template>
  <div style="padding-top: 16px;">
    <a-empty v-if="gpus.length === 0" description="No GPU detected" />
    <div v-else style="display: flex; flex-direction: column; gap: 20px;">
      <a-card v-for="gpu in gpus" :key="gpu.index" size="small" :bordered="false" style="background: #fafafa;">
        <template #title>
          <div style="display: flex; align-items: center; gap: 8px;">
            <span style="font-weight: bold;">GPU #{{ gpu.index }}</span>
            <span style="font-family: monospace; color: #888;">{{ gpu.name }}</span>
            <a-tag v-if="isIntegrated(gpu)" color="purple" style="font-size: 10px;">iGPU</a-tag>
            <a-tag v-if="(gpu.temp ?? 0) > 0" :color="tempColor(gpu.temp)">{{ gpu.temp }}°C</a-tag>
          </div>
        </template>
        <a-row :gutter="24">
          <a-col :span="8" style="text-align: center;">
            <a-progress
              type="dashboard"
              :percent="Math.round(gpu.utilGpu ?? 0)"
              :width="120"
              :stroke-color="gpuColor(gpu.utilGpu ?? 0)"
              :format="fmtPct(gpu.utilGpu ?? 0)"
              style="cursor: pointer;"
              @click="openHistoryChart(`GPU #${gpu.index} Utilization`, [
                { name: 'GPU Util', data: statsHistory.gpus[gpu.index]?.util || [], color: '#722ed1' }
              ])" />
            <div style="margin-top: 8px; font-size: 13px; color: #888; cursor: pointer;"
              @click="openHistoryChart(`GPU #${gpu.index} Utilization`, [
                { name: 'GPU Util', data: statsHistory.gpus[gpu.index]?.util || [], color: '#722ed1' }
              ])">GPU Utilization</div>
          </a-col>
          <a-col v-if="!isIntegrated(gpu)" :span="8" style="text-align: center;">
            <a-progress
              type="dashboard"
              :percent="Math.round(gpu.utilMem ?? 0)"
              :width="120"
              :stroke-color="memColor(gpu.utilMem ?? 0)"
              :format="fmtPct(gpu.utilMem ?? 0)"
              style="cursor: pointer;"
              @click="openHistoryChart(`GPU #${gpu.index} VRAM`, [
                { name: 'VRAM Util', data: statsHistory.gpus[gpu.index]?.mem || [], color: '#13c2c2' }
              ])" />
            <div style="margin-top: 8px; font-size: 13px; color: #888; cursor: pointer;"
              @click="openHistoryChart(`GPU #${gpu.index} VRAM`, [
                { name: 'VRAM Util', data: statsHistory.gpus[gpu.index]?.mem || [], color: '#13c2c2' }
              ])">VRAM Utilization</div>
          </a-col>
          <a-col v-else :span="8" style="text-align: center;">
            <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 140px;">
              <span style="font-size: 36px; color: #d9d9d9;">⊕</span>
              <span style="font-size: 12px; color: #888; margin-top: 8px;">Shared System Memory</span>
            </div>
          </a-col>
          <a-col :span="8">
            <div style="display: grid; gap: 12px; padding-top: 8px;">
              <template v-if="!isIntegrated(gpu) && (gpu.memTotal ?? 0) > 0">
                <a-statistic title="Total VRAM" :value="((gpu.memTotal ?? 0) / 1024).toFixed(1)" suffix="GB" />
                <a-statistic title="Used VRAM" :value="((gpu.memUsed ?? 0) / 1024).toFixed(1)" suffix="GB"
                  :value-style="{ color: (gpu.memUsed ?? 0) / (gpu.memTotal || 1) > 0.9 ? '#ff4d4f' : '#1890ff' }" />
              </template>
              <template v-else-if="isIntegrated(gpu)">
                <a-statistic title="Shared GPU Memory" :value="((gpu.memUsed ?? 0) / 1024).toFixed(2)" suffix="GB"
                  :value-style="{ color: '#1890ff' }" />
                <span style="font-size: 11px; color: #888;">Allocated from system RAM</span>
              </template>
              <a-statistic v-if="(gpu.temp ?? 0) > 0" title="Temperature" :value="gpu.temp" suffix="°C"
                :value-style="{ color: tempColor(gpu.temp ?? 0) }" />
              <div v-else style="font-size: 12px; color: #ccc; padding: 8px 0;">
                Temperature sensor not available
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </div>
  </div>
</template>
