<script setup lang="ts">
import { ref, watch, onUnmounted, computed, defineAsyncComponent } from 'vue';
import { AudioMutedOutlined, SoundOutlined } from '@ant-design/icons-vue';

const VueApexCharts = defineAsyncComponent(async () => (await import('vue3-apexcharts')).default as any) as any;

const props = defineProps<{
  sensorSubTab: string;
  // Hardware
  groupedSensors: Record<string, any[]>;
  sensorVisibility: Record<string, boolean>;
  sensorHistory: Record<string, { time: number; value: number }[]>;
  sensorInterval: number;
  fanData: any[];
  sensorChartOptions: any;
  // Camera
  cameras: string[];
  selectedCamera: string | null;
  cameraLiveMode: boolean;
  cameraStreamUrl: string;
  cameraLoading: boolean;
  // Mic
  micLiveMode: boolean;
  micVolume: number;
  micListenBrowser: boolean;
  micDevices: { id: string; name: string }[];
  selectedMic: string;
  micDataBuffer: Int16Array;
}>();

const emit = defineEmits<{
  'update:sensorSubTab': [value: string];
  'update:sensorInterval': [value: number];
  'update:sensorVisibility': [checked: boolean, key: string];
  'update:selectedCamera': [value: string];
  'update:cameraLiveMode': [value: boolean];
  'update:micLiveMode': [value: boolean];
  'update:micListenBrowser': [value: boolean];
  'update:selectedMic': [value: string];
  toggleAllSensors: [visible: boolean];
  refreshCamera: [];
}>();

// Microphone waveform via ApexCharts
const micChartOptions = computed(() => ({
  chart: {
    animations: { enabled: false },
    toolbar: { show: false },
    background: '#1a1a1a',
    foreColor: '#00ff88',
    zoom: { enabled: false },
  },
  xaxis: {
    labels: { show: false },
    axisBorder: { show: false },
    axisTicks: { show: false },
  },
  yaxis: {
    min: -32768,
    max: 32767,
    labels: { show: false },
    axisBorder: { show: false },
  },
  grid: { show: true, borderColor: '#333', strokeDashArray: 3 },
  stroke: { width: 1.5, curve: 'smooth' as const },
  legend: { show: false },
  tooltip: { enabled: false },
  dataLabels: { enabled: false },
  markers: { size: 0 },
}));

const micWaveformSeries = ref<{ name: string; data: number[][] }[]>([
  { name: 'Mic', data: [] },
]);

let micChartTimer: ReturnType<typeof setInterval> | null = null;

watch(() => props.micLiveMode, (val) => {
  if (val) {
    micChartTimer = setInterval(() => {
      const buf = props.micDataBuffer;
      const data: number[][] = [];
      for (let i = 0; i < buf.length; i += 2) {
        data.push([i, buf[i]]);
      }
      micWaveformSeries.value = [{ name: 'Mic', data }];
    }, 80);
  } else {
    if (micChartTimer) { clearInterval(micChartTimer); micChartTimer = null; }
    micWaveformSeries.value = [{ name: 'Mic', data: [] }];
  }
});

onUnmounted(() => {
  if (micChartTimer) clearInterval(micChartTimer);
});
</script>

<template>
  <div style="background: #fff; padding: 16px; border-radius: 4px; border: 1px solid #f0f0f0;">
    <a-tabs :activeKey="sensorSubTab" @change="emit('update:sensorSubTab', $event as string)" size="small">
      <a-tab-pane key="hardware" tab="Hardware">
        <div style="display: flex; flex-direction: column; gap: 16px;">
          <div style="display: flex; justify-content: flex-end; margin-bottom: 8px;">
            <a-space>
              <span style="font-size:12px;color:#888;">Interval:</span>
              <a-select :value="sensorInterval" size="small" style="width:70px" @change="emit('update:sensorInterval', $event)">
                <a-select-option :value="1000">1s</a-select-option>
                <a-select-option :value="2000">2s</a-select-option>
                <a-select-option :value="5000">5s</a-select-option>
              </a-select>
              <a-button-group size="small">
                <a-button @click="emit('toggleAllSensors', true)">All</a-button>
                <a-button @click="emit('toggleAllSensors', false)">None</a-button>
              </a-button-group>
            </a-space>
          </div>
          <div v-for="(sensors, category) in groupedSensors" :key="category" style="margin-bottom: 24px;">
            <div style="font-weight: bold; font-size: 14px; color: #1890ff; border-bottom: 2px solid #e6f7ff; padding-bottom: 8px; margin-bottom: 12px;"><span>{{ category }}</span></div>
            <a-row :gutter="16">
              <a-col :span="16"><div style="height:260px; background: #fafafa; border-radius: 8px; padding: 8px;"><VueApexCharts type="line" height="240" :options="{ ...sensorChartOptions, chart: { ...sensorChartOptions.chart, id: `chart-${category.replace(/\s+/g, '-')}` } }" :series="(sensors as any[]).filter(s => sensorVisibility[s.sensorKey]).map(s => ({ name: s.label || s.sensorKey, data: (sensorHistory[s.sensorKey] || []).map(d => ({ x: d.time, y: d.value })) }))" /></div></a-col>
              <a-col :span="8">
                <div style="max-height: 260px; overflow-y: auto; padding-right: 4px;">
                  <div v-for="s in sensors" :key="s.sensorKey" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; padding: 6px 10px; background: #fff; border: 1px solid #f0f0f0; border-radius: 4px;">
                    <a-checkbox :checked="sensorVisibility[s.sensorKey]" @change="(e: any) => emit('update:sensorVisibility', e?.target?.checked ?? e, s.sensorKey)" style="display: flex; align-items: center; flex: 1; overflow: hidden;">
                      <span style="font-size: 12px; margin-left: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 150px;" :title="s.label || s.sensorKey">{{ s.label || s.sensorKey }}</span>
                    </a-checkbox>
                    <span :style="{ color: s.temperature > 75 ? 'red' : s.temperature > 60 ? 'orange' : 'green', fontWeight:'bold', fontSize: '12px', marginLeft: '8px' }">{{ s.temperature.toFixed(1) }}°C</span>
                  </div>
                </div>
              </a-col>
            </a-row>
          </div>
          <div v-if="fanData.length > 0">
            <div style="font-weight: bold; font-size: 14px; color: #52c41a; border-bottom: 2px solid #f6ffed; padding-bottom: 8px; margin-bottom: 12px;">Cooling (Fans)</div>
            <a-row :gutter="16"><a-col v-for="f in fanData" :key="f.label" :span="6"><a-card size="small" style="margin-bottom: 8px;"><a-statistic :title="f.label" :value="f.speed" suffix="RPM" /></a-card></a-col></a-row>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="camera" tab="Camera">
        <a-card title="Live Feed" size="small">
          <template #extra><a-space><a-tag v-if="cameraLiveMode" color="red">LIVE</a-tag><span>Live:</span><a-switch :checked="cameraLiveMode" @change="(checked: boolean) => emit('update:cameraLiveMode', checked)" size="small" /></a-space></template>
          <div style="display:flex;gap:16px;">
            <div style="flex:1;">
              <a-select :value="selectedCamera" style="width:100%;margin-bottom:12px;" @change="emit('update:selectedCamera', $event); emit('refreshCamera')">
                <a-select-option v-for="cam in cameras" :key="cam" :value="cam">{{ cam }}</a-select-option>
              </a-select>
              <div style="background:#000;border-radius:4px;overflow:hidden;aspect-ratio:16/9;display:flex;align-items:center;justify-content:center;">
                <img v-if="cameraStreamUrl" :src="cameraStreamUrl" style="width:100%;height:100%;object-fit:contain;" />
                <a-empty v-else description="No stream" />
              </div>
            </div>
            <div v-if="!cameraLiveMode" style="width:200px;"><a-card size="small" title="Snapshot"><a-button block size="small" @click="emit('refreshCamera')">Capture</a-button></a-card></div>
          </div>
        </a-card>
      </a-tab-pane>

      <a-tab-pane key="mic" tab="Microphone">
        <a-card title="Input Monitor" size="small">
          <template #extra><a-space><a-tag v-if="micLiveMode" color="green">ON</a-tag><a-switch :checked="micLiveMode" @change="(checked: boolean) => emit('update:micLiveMode', checked)" size="small" /></a-space></template>
          <div style="display:flex;gap:24px;align-items:center;">
            <div style="flex:1;"><div style="margin-bottom:8px;font-size:12px;color:#888;">Waveform</div><VueApexCharts type="line" height="120" :options="micChartOptions" :series="micWaveformSeries" /></div>
            <div style="width:280px;">
              <div style="margin-bottom:16px;">
                <div style="margin-bottom:8px;font-size:12px;color:#888;display:flex;justify-content:space-between;">
                  <span>Input Level</span>
                  <a-button type="link" size="small" style="padding:0;height:auto;" @click="emit('update:micListenBrowser', !micListenBrowser)">
                    <template #icon><SoundOutlined v-if="micListenBrowser" /><AudioMutedOutlined v-else /></template>
                    {{ micListenBrowser ? 'Listening' : 'Muted' }}
                  </a-button>
                </div>
                <a-progress :percent="micVolume" :show-info="false" :stroke-color="micVolume > 80 ? '#ff4d4f' : '#52c41a'" />
              </div>
              <div style="font-size: 12px; color: #888; margin-bottom: 4px;">Device</div>
              <a-select :value="selectedMic" style="width:100%" size="small" placeholder="Select Microphone" @change="emit('update:selectedMic', $event)">
                <a-select-option v-for="dev in micDevices" :key="dev.id" :value="dev.id">{{ dev.name }}</a-select-option>
              </a-select>
            </div>
          </div>
        </a-card>
      </a-tab-pane>
    </a-tabs>
  </div>
</template>
