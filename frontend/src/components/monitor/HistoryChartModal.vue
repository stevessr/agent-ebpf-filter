<script setup lang="ts">
import { computed, defineAsyncComponent } from "vue";
import { systemIsDark } from "../../composables/useSystemTheme";

const VueApexCharts = defineAsyncComponent(
  async () => (await import("vue3-apexcharts")).default as any,
) as any;

const props = defineProps<{
  show: boolean;
  title: string;
  series: any[];
  chartOptions: any;
}>();

const themedOptions = computed(() => ({
  ...props.chartOptions,
  theme: { ...props.chartOptions?.theme, mode: systemIsDark.value ? "dark" : "light" },
  chart: { ...props.chartOptions?.chart, foreColor: systemIsDark.value ? "#bdc4d1" : "#475569" },
  grid: { ...props.chartOptions?.grid, borderColor: systemIsDark.value ? "#334155" : "#e2e8f0" },
  tooltip: { ...props.chartOptions?.tooltip, theme: systemIsDark.value ? "dark" : "light" },
}));

const emit = defineEmits<{
  "update:show": [value: boolean];
}>();
</script>

<template>
  <a-modal
    :open="show"
    :title="title"
    width="800px"
    :footer="null"
    @update:open="emit('update:show', $event)"
  >
    <div style="height: 400px; padding: 12px">
      <VueApexCharts
        type="line"
        height="380"
        :options="themedOptions"
        :series="series"
      />
    </div>
  </a-modal>
</template>
