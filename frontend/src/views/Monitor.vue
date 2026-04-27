<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  SearchOutlined, DeploymentUnitOutlined,
  DashboardOutlined, AppstoreOutlined,
  ApiOutlined,
} from '@ant-design/icons-vue';
import { useMonitorData } from '../composables/useMonitorData';
import { useSensors } from '../composables/useSensors';
import { useSystemd } from '../composables/useSystemd';
import HealthCpu from '../components/monitor/HealthCpu.vue';
import HealthMemory from '../components/monitor/HealthMemory.vue';
import HealthIO from '../components/monitor/HealthIO.vue';
import HealthFaults from '../components/monitor/HealthFaults.vue';
import HealthGpu from '../components/monitor/HealthGpu.vue';
import ProcessTable from '../components/monitor/ProcessTable.vue';
import SystemdPanel from '../components/monitor/SystemdPanel.vue';
import SensorsPanel from '../components/monitor/SensorsPanel.vue';
import TracingPanel from '../components/monitor/TracingPanel.vue';
import HistoryChartModal from '../components/monitor/HistoryChartModal.vue';

// ── Composable instances ──
const {
  processes, gpus, systemStats, statsHistory,
  trackedCommsNames,
  faultTopN, mergeFaultProcesses, cpuView,
  groupedNetInterfaces, groupedDiskDevices, topFaultProcesses, trackedProcesses,
  formatBytesWithUnit, getCoreTypeColor, getCoreTypeName,
  fetchTrackedComms, sendProcessSignal,
  openHistoryChart, showProcessDetails,
  showHistoryModal, historyModalTitle, historySeries, historyChartOptions,
  showProcessMapsModal, selectedProcessMaps, selectedProcessDetails, processMapsLoading,
  setup, teardown
} = useMonitorData();

const {
  sensorInterval,
  sensorHistory, sensorVisibility, fanData,
  connectSensorsWS, fetchSensors, closeSensorWS,
  sensorChartOptions, groupedSensors, toggleAllSensors,
  cameras, selectedCamera, cameraLiveMode, cameraLoading,
  cameraStreamUrl,
  fetchCameras, stopCameraWS, refreshCamera,
  micDevices, selectedMic, micLiveMode, micListenBrowser, micVolume, micDataBuffer,
  fetchMicrophones, connectMicWS, stopMicWS,
} = useSensors();

const {
  systemdServices, systemdLoading, systemdSearch, systemdScope,
  showLogsModal, activeLogUnit, serviceLogs, logsLoading,
  filteredSystemdServices, systemdColumns,
  fetchSystemdServices, controlSystemdService, fetchSystemdLogs,
} = useSystemd();

// ── Route-driven tab state ──
const route = useRoute();
const router = useRouter();

const activeTab = ref((route.params.tab as string) || 'dashboard');
const sensorSubTab = ref('hardware');
const healthTab = ref('cpu');

// Parse subtab from URL on init; redirect to default if missing
if (route.params.subtab) {
  const subtab = route.params.subtab as string;
  if (activeTab.value === 'dashboard' && ['cpu', 'mem', 'io', 'faults', 'gpu'].includes(subtab)) {
    healthTab.value = subtab;
  } else if (activeTab.value === 'sensors' && ['hardware', 'camera', 'mic'].includes(subtab)) {
    sensorSubTab.value = subtab;
  }
} else if (activeTab.value === 'dashboard') {
  void router.replace({ name: 'Monitor', params: { tab: 'dashboard', subtab: healthTab.value } });
} else if (activeTab.value === 'sensors') {
  void router.replace({ name: 'Monitor', params: { tab: 'sensors', subtab: sensorSubTab.value } });
}

const navigate = (tab: string, subtab?: string) => {
  const params: Record<string, string> = { tab };
  if (subtab) params.subtab = subtab;
  void router.replace({ name: 'Monitor', params });
};

const handleTabChange = (key: string) => {
  activeTab.value = key;
  if (key === 'dashboard') navigate(key, healthTab.value);
  else if (key === 'sensors') navigate(key, sensorSubTab.value);
  else navigate(key);
};

const handleHealthSubTabChange = (key: string) => {
  healthTab.value = key;
  navigate('dashboard', key);
};

const handleSensorSubTabChange = (key: string) => {
  sensorSubTab.value = key;
  navigate('sensors', key);
};

const onSendProcessSignal = async (pid: number, signal: string) => {
  const { message } = await import('ant-design-vue');
  const ok = await sendProcessSignal(pid, signal);
  if (ok) message.success(`Signal ${signal.toUpperCase()} sent to PID ${pid}`);
  else message.error(`Failed to send ${signal}`);
};

// ── Tab activation effects ──
watch(activeTab, (newTab) => {
  if (newTab === 'systemd' && systemdServices.value.length === 0) void fetchSystemdServices();
  else if (newTab === 'sensors') {
    if (sensorSubTab.value === 'hardware') { void fetchSensors(); connectSensorsWS(); }
    else if (sensorSubTab.value === 'camera') void fetchCameras();
    else if (sensorSubTab.value === 'mic') void fetchMicrophones();
  } else if (newTab === 'tracing') void fetchTrackedComms();
  else {
    closeSensorWS();
    cameraLiveMode.value = false;
    micLiveMode.value = false;
  }
});

watch(sensorSubTab, (newSub) => {
  if (activeTab.value !== 'sensors') return;
  if (newSub === 'hardware') { void fetchSensors(); connectSensorsWS(); }
  else {
    closeSensorWS();
    if (newSub === 'camera') void fetchCameras();
    else if (newSub === 'mic') void fetchMicrophones();
  }
});

watch(sensorInterval, () => {
  if (activeTab.value === 'sensors' && sensorSubTab.value === 'hardware') connectSensorsWS();
});

// Mic live mode: connect/disconnect WebSocket
watch(micLiveMode, (val) => {
  if (val) connectMicWS();
  else stopMicWS();
});

watch(selectedMic, () => {
  if (micLiveMode.value) connectMicWS();
});

// ── Lifecycle ──
onMounted(() => {
  setup();
  if (activeTab.value === 'systemd') void fetchSystemdServices();
  else if (activeTab.value === 'sensors') {
    if (sensorSubTab.value === 'hardware') { void fetchSensors(); connectSensorsWS(); }
    else if (sensorSubTab.value === 'camera') void fetchCameras();
    else if (sensorSubTab.value === 'mic') void fetchMicrophones();
  } else if (activeTab.value === 'tracing') void fetchTrackedComms();
});

onUnmounted(() => {
  teardown();
  stopCameraWS();
  stopMicWS();
  closeSensorWS();
});
</script>

<template>
  <div style="background: #f0f2f5; padding: 20px; min-height: 100%;">
    <a-tabs :activeKey="activeTab" @change="handleTabChange" type="card" class="monitor-tabs">
      <!-- ── Health / Dashboard ── -->
      <a-tab-pane key="dashboard">
        <template #tab><span><DashboardOutlined /> Health</span></template>
        <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">
          <a-tabs :activeKey="healthTab" @change="handleHealthSubTabChange" size="small" type="line" style="margin-top: -12px;">
            <a-tab-pane key="cpu" tab="CPU">
              <HealthCpu
                :systemStats="systemStats" :statsHistory="statsHistory" :cpuView="cpuView"
                :getCoreTypeColor="getCoreTypeColor" :getCoreTypeName="getCoreTypeName"
                :openHistoryChart="openHistoryChart"
                @update:cpuView="cpuView = $event" />
            </a-tab-pane>
            <a-tab-pane key="mem" tab="Memory">
              <HealthMemory
                :systemStats="systemStats" :statsHistory="statsHistory"
                :formatBytesWithUnit="formatBytesWithUnit" :openHistoryChart="openHistoryChart" />
            </a-tab-pane>
            <a-tab-pane key="io" tab="I/O">
              <HealthIO
                :systemStats="systemStats" :statsHistory="statsHistory"
                :groupedNetInterfaces="groupedNetInterfaces" :groupedDiskDevices="groupedDiskDevices"
                :formatBytesWithUnit="formatBytesWithUnit" :openHistoryChart="openHistoryChart" />
            </a-tab-pane>
            <a-tab-pane key="faults" tab="Faults">
              <HealthFaults
                :faults="systemStats.faults" :topFaultProcesses="topFaultProcesses"
                :faultTopN="faultTopN" :mergeFaultProcesses="mergeFaultProcesses"
                :statsHistory="{ faults: statsHistory.faults, swapIn: statsHistory.swapIn, swapOut: statsHistory.swapOut }"
                :openHistoryChart="openHistoryChart"
                @update:faultTopN="faultTopN = $event"
                @update:mergeFaultProcesses="mergeFaultProcesses = $event" />
            </a-tab-pane>
            <a-tab-pane key="gpu" tab="GPU">
              <HealthGpu :gpus="gpus" />
            </a-tab-pane>
          </a-tabs>
        </div>
      </a-tab-pane>

      <!-- ── Processes ── -->
      <a-tab-pane key="processes">
        <template #tab><span><AppstoreOutlined /> Processes</span></template>
        <ProcessTable
          :processes="processes" :showProcessDetails="showProcessDetails"
          :sendProcessSignal="onSendProcessSignal" />
      </a-tab-pane>

      <!-- ── Systemd ── -->
      <a-tab-pane key="systemd">
        <template #tab><span><DeploymentUnitOutlined /> Systemd</span></template>
        <SystemdPanel
          :systemdServices="systemdServices" :systemdLoading="systemdLoading"
          :filteredSystemdServices="filteredSystemdServices"
          :systemdScope="systemdScope" :systemdSearch="systemdSearch"
          :systemdColumns="systemdColumns"
          :showLogsModal="showLogsModal" :activeLogUnit="activeLogUnit"
          :serviceLogs="serviceLogs" :logsLoading="logsLoading"
          @update:systemdScope="systemdScope = $event"
          @update:systemdSearch="systemdSearch = $event"
          @update:showLogsModal="showLogsModal = $event"
          @refresh="fetchSystemdServices"
          @control="(unit, action) => controlSystemdService(unit, action)"
          @fetchLogs="fetchSystemdLogs" />
      </a-tab-pane>

      <!-- ── Sensors ── -->
      <a-tab-pane key="sensors">
        <template #tab><span><ApiOutlined /> Sensors</span></template>
        <SensorsPanel
          :sensorSubTab="sensorSubTab"
          :groupedSensors="groupedSensors" :sensorVisibility="sensorVisibility"
          :sensorHistory="sensorHistory" :sensorInterval="sensorInterval"
          :fanData="fanData" :sensorChartOptions="sensorChartOptions"
          :cameras="cameras" :selectedCamera="selectedCamera"
          :cameraLiveMode="cameraLiveMode" :cameraStreamUrl="cameraStreamUrl"
          :cameraLoading="cameraLoading"
          :micLiveMode="micLiveMode" :micVolume="micVolume"
          :micListenBrowser="micListenBrowser" :micDevices="micDevices"
          :selectedMic="selectedMic" :micDataBuffer="micDataBuffer"
          @update:sensorSubTab="handleSensorSubTabChange"
          @update:sensorInterval="sensorInterval = $event"
          @update:sensorVisibility="(checked, key) => sensorVisibility[key] = checked"
          @update:selectedCamera="selectedCamera = $event"
          @update:cameraLiveMode="cameraLiveMode = $event"
          @update:micLiveMode="micLiveMode = $event"
          @update:micListenBrowser="micListenBrowser = $event"
          @update:selectedMic="selectedMic = $event"
          @toggleAllSensors="toggleAllSensors"
          @refreshCamera="refreshCamera" />
      </a-tab-pane>

      <!-- ── Tracing ── -->
      <a-tab-pane key="tracing">
        <template #tab><span><SearchOutlined /> Tracing</span></template>
        <TracingPanel
          :trackedProcesses="trackedProcesses" :trackedCommsNames="trackedCommsNames"
          :sendProcessSignal="onSendProcessSignal"
          @refresh="fetchTrackedComms" />
      </a-tab-pane>
    </a-tabs>

    <!-- ── Shared modals ── -->
    <HistoryChartModal
      :show="showHistoryModal" :title="historyModalTitle"
      :series="historySeries" :chartOptions="historyChartOptions"
      @update:show="showHistoryModal = $event" />

    <a-modal v-model:open="showProcessMapsModal"
      :title="`Process Details: ${selectedProcessDetails?.name} (PID: ${selectedProcessDetails?.pid})`"
      width="1000px" :footer="null">
      <div style="display: flex; flex-direction: column; gap: 16px;">
        <div v-if="selectedProcessDetails" style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; background: #fafafa; padding: 12px; border-radius: 4px;">
          <div><span style="color:#888">User:</span> <b>{{ selectedProcessDetails.user }}</b></div>
          <div><span style="color:#888">CPU:</span> <b>{{ (selectedProcessDetails.cpu ?? 0).toFixed(1) }}%</b></div>
          <div><span style="color:#888">Mem:</span> <b>{{ (selectedProcessDetails.mem ?? 0).toFixed(1) }}%</b></div>
          <div style="grid-column: span 3;"><span style="color:#888">Command:</span> <code style="font-size: 11px;">{{ selectedProcessDetails.cmdline }}</code></div>
        </div>
        <div style="height: 500px; overflow-y: auto; background: #1e1e1e; color: #d4d4d4; padding: 12px; border-radius: 4px; font-family: 'JetBrains Mono', monospace; font-size: 12px;">
          <a-spin :spinning="processMapsLoading">
            <pre v-if="selectedProcessMaps" style="margin: 0; white-space: pre-wrap; word-break: break-all;">{{ selectedProcessMaps }}</pre>
            <a-empty v-else-if="!processMapsLoading" description="No map data available" />
          </a-spin>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.monitor-tabs :deep(.ant-tabs-nav) { margin-bottom: 0; }
</style>
