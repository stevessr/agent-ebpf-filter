<script setup lang="ts">
import type { GPUStatus } from '../../composables/useMonitorData';

defineProps<{
  gpus: GPUStatus[];
}>();
</script>

<template>
  <div style="padding-top: 16px;">
    <a-empty v-if="gpus.length === 0" description="No GPU detected">
      <template #image>
        <span style="font-size: 48px; opacity: 0.3;">🎮</span>
      </template>
    </a-empty>
    <div v-else style="display: flex; flex-direction: column; gap: 20px;">
      <a-card v-for="gpu in gpus" :key="gpu.index" size="small" :bordered="false" style="background: #fafafa;">
        <template #title>
          <div style="display: flex; align-items: center; gap: 8px;">
            <span style="font-weight: bold;">GPU #{{ gpu.index }}</span>
            <span style="font-family: monospace; color: #888;">{{ gpu.name }}</span>
            <a-tag v-if="gpu.temp > 0" :color="gpu.temp > 80 ? 'red' : gpu.temp > 65 ? 'orange' : 'green'">{{ gpu.temp }}°C</a-tag>
          </div>
        </template>
        <a-row :gutter="24">
          <a-col :span="8" style="text-align: center;">
            <a-progress type="dashboard" :percent="Math.round(gpu.utilGpu)" :width="120" :stroke-color="gpu.utilGpu > 80 ? '#ff4d4f' : '#722ed1'" />
            <div style="margin-top: 8px; font-size: 13px; color: #888;">GPU Utilization</div>
          </a-col>
          <a-col :span="8" style="text-align: center;">
            <a-progress type="dashboard" :percent="Math.round(gpu.utilMem)" :width="120" :stroke-color="gpu.utilMem > 80 ? '#ff4d4f' : '#13c2c2'" />
            <div style="margin-top: 8px; font-size: 13px; color: #888;">Memory Utilization</div>
          </a-col>
          <a-col :span="8">
            <div style="display: grid; gap: 12px; padding-top: 8px;">
              <a-statistic title="Total VRAM" :value="(gpu.memTotal / 1024).toFixed(1)" suffix="GB" />
              <a-statistic title="Used VRAM" :value="(gpu.memUsed / 1024).toFixed(1)" suffix="GB" :value-style="{ color: gpu.memUsed / gpu.memTotal > 0.9 ? '#ff4d4f' : '#1890ff' }" />
              <a-statistic title="Temperature" :value="gpu.temp" suffix="°C" :value-style="{ color: gpu.temp > 80 ? '#ff4d4f' : gpu.temp > 65 ? '#faad14' : '#52c41a' }" />
            </div>
          </a-col>
        </a-row>
      </a-card>
    </div>
  </div>
</template>
