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

const isNvidia = (gpu: GPUStatus) => gpu.name.toLowerCase().includes('nvidia');

const gpuColor = (pct: number) => pct > 80 ? '#ff4d4f' : '#722ed1';
const memColor = (pct: number) => pct > 80 ? '#ff4d4f' : '#13c2c2';
const tempColor = (t: number) => t > 80 ? '#ff4d4f' : t > 65 ? '#faad14' : '#52c41a';
const encColor = (pct: number) => pct > 80 ? '#ff4d4f' : '#eb2f96';
const decColor = (pct: number) => pct > 80 ? '#ff4d4f' : '#fa8c16';

const fmtPct = (val: number) => () => `${Math.round(val ?? 0)}%`;
</script>

<template>
  <div style="padding-top: 16px;">
    <a-empty v-if="gpus.length === 0" description="No GPU detected" />
    <div v-else style="display: flex; flex-direction: column; gap: 20px;">
      <a-card v-for="gpu in gpus" :key="gpu.index" size="small" :bordered="false" style="background: #fafafa;">
        <!-- Header -->
        <template #title>
          <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
            <span style="font-weight: bold;">GPU #{{ gpu.index }}</span>
            <span style="font-family: monospace; color: #888;">{{ gpu.name }}</span>
            <a-tag v-if="isIntegrated(gpu)" color="purple" style="font-size: 10px;">iGPU</a-tag>
            <a-tag v-if="isNvidia(gpu) && (gpu.pcieGen ?? 0) > 0" color="geekblue" style="font-size: 10px;">PCIe {{ gpu.pcieGen }}.0 x{{ gpu.pcieWidth }}</a-tag>
            <a-tag v-if="(gpu.temp ?? 0) > 0" :color="tempColor(gpu.temp)">{{ gpu.temp }}°C</a-tag>
          </div>
        </template>

        <!-- Main dashboard row -->
        <a-row :gutter="24">
          <!-- GPU Utilization -->
          <a-col :span="8" style="text-align: center;">
            <a-progress type="dashboard" :percent="Math.round(gpu.utilGpu ?? 0)" :width="120"
              :stroke-color="gpuColor(gpu.utilGpu ?? 0)" :format="fmtPct(gpu.utilGpu ?? 0)"
              style="cursor: pointer;"
              @click="openHistoryChart(`GPU #${gpu.index} Utilization`, [
                { name: 'GPU Util', data: statsHistory.gpus[gpu.index]?.util || [], color: '#722ed1' }
              ])" />
            <div style="margin-top: 8px; font-size: 13px; color: #888; cursor: pointer;"
              @click="openHistoryChart(`GPU #${gpu.index} Utilization`, [
                { name: 'GPU Util', data: statsHistory.gpus[gpu.index]?.util || [], color: '#722ed1' }
              ])">GPU Utilization</div>
          </a-col>

          <!-- VRAM or Shared Memory -->
          <a-col v-if="!isIntegrated(gpu)" :span="8" style="text-align: center;">
            <a-progress type="dashboard" :percent="Math.round(gpu.utilMem ?? 0)" :width="120"
              :stroke-color="memColor(gpu.utilMem ?? 0)" :format="fmtPct(gpu.utilMem ?? 0)"
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

          <!-- Stats column -->
          <a-col :span="8">
            <div style="display: grid; gap: 8px; padding-top: 4px;">
              <!-- VRAM stats -->
              <template v-if="!isIntegrated(gpu) && (gpu.memTotal ?? 0) > 0">
                <div style="font-size: 12px; color: #888;">VRAM: {{ ((gpu.memUsed ?? 0) / 1024).toFixed(1) }} / {{ ((gpu.memTotal ?? 0) / 1024).toFixed(1) }} GB</div>
                <a-progress :percent="Math.round((gpu.memUsed ?? 0) / (gpu.memTotal || 1) * 100)" size="small"
                  :stroke-color="(gpu.memUsed ?? 0) / (gpu.memTotal || 1) > 0.9 ? '#ff4d4f' : '#1890ff'"
                  :show-info="false" style="margin-bottom: 4px;" />
              </template>
              <template v-else-if="isIntegrated(gpu) && (gpu.memUsed ?? 0) > 0">
                <div style="font-size: 12px; color: #888;">Shared Mem: {{ ((gpu.memUsed ?? 0) / 1024).toFixed(2) }} GB</div>
              </template>

              <!-- Temperature or unavailable -->
              <div v-if="(gpu.temp ?? 0) > 0" style="font-size: 12px;" :style="{ color: tempColor(gpu.temp ?? 0) }">
                Temperature: {{ gpu.temp }}°C
              </div>
              <div v-else style="font-size: 12px; color: #ccc;">
                Temp sensor unavailable
              </div>

              <!-- Power -->
              <div v-if="(gpu.powerW ?? 0) > 0" style="font-size: 12px; color: #888;">
                Power: {{ gpu.powerW }}W
                <template v-if="(gpu.powerLimitW ?? 0) > 0"> / {{ gpu.powerLimitW }}W limit</template>
                <a-progress :percent="(gpu.powerLimitW ?? 0) > 0 ? Math.round((gpu.powerW ?? 0) / (gpu.powerLimitW || 1) * 100) : 0"
                  size="small" :show-info="false" :stroke-color="(gpu.powerLimitW ?? 0) > 0 && (gpu.powerW ?? 0) / (gpu.powerLimitW || 1) > 0.8 ? '#faad14' : '#52c41a'" />
              </div>

              <!-- Fan -->
              <div v-if="(gpu.fanSpeed ?? 0) > 0" style="font-size: 12px; color: #888;">
                Fan: {{ gpu.fanSpeed }}%
                <a-progress :percent="gpu.fanSpeed ?? 0" size="small" :show-info="false"
                  :stroke-color="(gpu.fanSpeed ?? 0) > 80 ? '#ff4d4f' : '#1890ff'" />
              </div>
            </div>
          </a-col>
        </a-row>

        <!-- NVIDIA detailed engine section -->
        <a-divider v-if="isNvidia(gpu) && ((gpu.encUtil ?? 0) > 0 || (gpu.decUtil ?? 0) > 0 || (gpu.smClockMhz ?? 0) > 0 || (gpu.memClockMhz ?? 0) > 0)" style="margin: 12px 0 8px; font-size: 11px; color: #888;">
          Engine Details
        </a-divider>

        <a-row v-if="isNvidia(gpu)" :gutter="16">
          <!-- Encoder -->
          <a-col v-if="(gpu.encUtil ?? 0) >= 0" :span="6" style="text-align: center;">
            <a-progress type="circle" :percent="Math.round(gpu.encUtil ?? 0)" :width="64" :stroke-color="encColor(gpu.encUtil ?? 0)" :format="fmtPct(gpu.encUtil ?? 0)" />
            <div style="font-size: 11px; color: #888; margin-top: 4px;">Encoder (NVENC)</div>
          </a-col>

          <!-- Decoder -->
          <a-col v-if="(gpu.decUtil ?? 0) >= 0" :span="6" style="text-align: center;">
            <a-progress type="circle" :percent="Math.round(gpu.decUtil ?? 0)" :width="64" :stroke-color="decColor(gpu.decUtil ?? 0)" :format="fmtPct(gpu.decUtil ?? 0)" />
            <div style="font-size: 11px; color: #888; margin-top: 4px;">Decoder (NVDEC)</div>
          </a-col>

          <!-- Clock speeds -->
          <a-col :span="12">
            <div style="display: grid; gap: 6px;">
              <div v-if="(gpu.gfxClockMhz ?? 0) > 0" style="display: flex; justify-content: space-between; font-size: 12px;">
                <span style="color: #888;">Graphics Clock</span>
                <span style="font-weight: bold; font-family: monospace;">{{ gpu.gfxClockMhz }} MHz</span>
              </div>
              <div v-if="(gpu.smClockMhz ?? 0) > 0" style="display: flex; justify-content: space-between; font-size: 12px;">
                <span style="color: #888;">SM Clock</span>
                <span style="font-weight: bold; font-family: monospace;">{{ gpu.smClockMhz }} MHz</span>
              </div>
              <div v-if="(gpu.memClockMhz ?? 0) > 0" style="display: flex; justify-content: space-between; font-size: 12px;">
                <span style="color: #888;">Memory Clock</span>
                <span style="font-weight: bold; font-family: monospace;">{{ gpu.memClockMhz }} MHz</span>
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </div>
  </div>
</template>
