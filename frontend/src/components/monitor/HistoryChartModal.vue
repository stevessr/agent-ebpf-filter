<script setup lang="ts">
import { defineAsyncComponent } from 'vue';

const VueApexCharts = defineAsyncComponent(async () => (await import('vue3-apexcharts')).default as any) as any;

defineProps<{
  show: boolean;
  title: string;
  series: any[];
  chartOptions: any;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
}>();
</script>

<template>
  <a-modal :open="show" :title="title" width="800px" :footer="null" @update:open="emit('update:show', $event)">
    <div style="height: 400px; padding: 12px;">
      <VueApexCharts type="line" height="380" :options="chartOptions" :series="series" />
    </div>
  </a-modal>
</template>
