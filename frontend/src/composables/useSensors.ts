import { ref, computed, watch } from 'vue';
import axios from 'axios';
import { buildWebSocketUrl } from '../utils/requestContext';
import { pb } from '../pb/tracker_pb.js';

export function useSensors() {
  // Sensor state
  const sensorData = ref<any[]>([]);
  const fanData = ref<any[]>([]);
  const sensorsLoading = ref(false);
  const sensorInterval = ref(2000);
  const sensorHistory = ref<Record<string, { time: number; value: number }[]>>({});
  const sensorVisibility = ref<Record<string, boolean>>({});

  let sensorWs: WebSocket | null = null;

  const getSensorCategory = (key: string, label: string) => {
    const s = (key + label).toLowerCase();
    if (s.includes('nvme')) return 'Storage (NVMe)';
    if (s.includes('acpi')) return 'System (ACPI)';
    if (s.includes('coretemp') || s.includes('package_id') || s.includes('cpu') || s.includes('k10temp')) return 'Processor (CPU)';
    if (s.includes('gpu') || s.includes('amdgpu') || s.includes('nvidia')) return 'Graphics (GPU)';
    if (s.includes('bat')) return 'Power (Battery)';
    if (s.includes('wifi') || s.includes('iwl') || s.includes('ath')) return 'Network (Wi-Fi)';
    if (s.includes('fan')) return 'Cooling (Fan)';
    if (s.includes('sata') || s.includes('sda') || s.includes('sdb')) return 'Storage (SATA)';
    return 'Other Sensors';
  };

  const connectSensorsWS = () => {
    if (sensorWs) sensorWs.close();
    const wsUrl = buildWebSocketUrl(`/ws/sensors?interval=${sensorInterval.value}`);
    sensorWs = new WebSocket(wsUrl);
    sensorWs.binaryType = 'arraybuffer';
    sensorWs.onmessage = (e) => {
      let temps: any[] = [];
      let fans: string[] = [];
      if (e.data instanceof ArrayBuffer) {
        const snap = pb.SensorsSnapshot.decode(new Uint8Array(e.data));
        temps = (snap.temperatures || []).map(t => ({ sensorKey: t.key, temperature: t.value }));
        fans = snap.fans || [];
      }
      fanData.value = fans;
      sensorData.value = temps.map((s: any) => ({ ...s, sensorKey: s.sensorKey || s.label, category: getSensorCategory(s.sensorKey || '', s.label || '') }));
      const now = Date.now();
      sensorData.value.forEach(s => {
        const key = s.sensorKey;
        if (!sensorHistory.value[key]) {
          sensorHistory.value[key] = [];
          if (sensorVisibility.value[key] === undefined) sensorVisibility.value[key] = true;
        }
        sensorHistory.value[key].push({ time: now, value: s.temperature });
        if (sensorHistory.value[key].length > 60) sensorHistory.value[key].shift();
      });
    };
  };

  const fetchSensors = async () => {
    sensorsLoading.value = true;
    try {
      const res = await axios.get('/system/sensors');
      sensorData.value = (res.data.temperatures || []).map((s: any) => ({ ...s, sensorKey: s.sensorKey || s.label, category: getSensorCategory(s.sensorKey || '', s.label || '') }));
      fanData.value = res.data.fans || [];
    } catch (err) {} finally { sensorsLoading.value = false; }
  };

  const sensorChartOptions = computed(() => ({
    chart: { id: 'sensor-chart', animations: { enabled: false }, toolbar: { show: false }, background: 'transparent' },
    xaxis: { type: 'datetime' as const, labels: { show: true, style: { fontSize: '10px' }, datetimeUTC: false }, axisBorder: { show: false } },
    yaxis: { title: { text: 'Temp (°C)', style: { fontSize: '12px' } }, min: 0, max: (maxVal: number) => Math.max(70, maxVal * 1.1), tickAmount: 5 },
    stroke: { width: 2, curve: 'smooth' as const },
    colors: ['#1890ff', '#52c41a', '#faad14', '#ff4d4f', '#722ed1', '#13c2c2', '#eb2f96'],
    legend: { show: false },
    grid: { borderColor: '#f0f0f0' },
    tooltip: { x: { format: 'HH:mm:ss' } }
  }));

  const groupedSensors = computed(() => {
    const groups: Record<string, any[]> = {};
    sensorData.value.forEach(s => {
      if (!groups[s.category]) groups[s.category] = [];
      groups[s.category].push(s);
    });
    return groups;
  });

  const toggleAllSensors = (visible: boolean) => {
    Object.keys(sensorVisibility.value).forEach(k => sensorVisibility.value[k] = visible);
  };

  // Camera state
  const cameras = ref<string[]>([]);
  const selectedCamera = ref<string | null>(null);
  const cameraLiveMode = ref(false);
  const cameraLoading = ref(false);
  const cameraFrameUrl = ref('');
  const cameraSnapshotUrl = ref('');

  let cameraWs: WebSocket | null = null;

  const fetchCameras = async () => {
    try {
      const res = await axios.get('/system/cameras');
      cameras.value = res.data;
      if (res.data.length > 0 && !selectedCamera.value) selectedCamera.value = res.data[0];
    } catch (err) {}
  };

  const connectCameraWS = () => {
    if (!selectedCamera.value) return;
    if (cameraWs) cameraWs.close();
    cameraLoading.value = true;
    const wsUrl = buildWebSocketUrl(`/ws/camera?device=${encodeURIComponent(selectedCamera.value)}`);
    cameraWs = new WebSocket(wsUrl);
    cameraWs.binaryType = 'arraybuffer';
    cameraWs.onopen = () => cameraLoading.value = false;
    cameraWs.onmessage = (e) => {
      if (typeof e.data !== 'string') {
        const blob = new Blob([e.data], { type: 'image/jpeg' });
        const url = URL.createObjectURL(blob);
        if (cameraFrameUrl.value) URL.revokeObjectURL(cameraFrameUrl.value);
        cameraFrameUrl.value = url;
      }
    };
    cameraWs.onclose = () => { if (cameraLiveMode.value) setTimeout(connectCameraWS, 2000); };
  };

  const stopCameraWS = () => {
    if (cameraWs) { cameraWs.onclose = null; cameraWs.close(); cameraWs = null; }
    if (cameraFrameUrl.value) { URL.revokeObjectURL(cameraFrameUrl.value); cameraFrameUrl.value = ''; }
  };

  const refreshCamera = async () => {
    if (!selectedCamera.value) return;
    if (cameraLiveMode.value) { connectCameraWS(); return; }
    cameraLoading.value = true;
    try { cameraSnapshotUrl.value = `/system/camera/snapshot?device=${encodeURIComponent(selectedCamera.value)}&t=${Date.now()}`; } catch (err) {} finally { cameraLoading.value = false; }
  };

  const cameraStreamUrl = computed(() => {
    if (!selectedCamera.value) return '';
    return cameraLiveMode.value ? cameraFrameUrl.value : cameraSnapshotUrl.value;
  });

  watch(cameraLiveMode, (val) => { if (val) connectCameraWS(); else stopCameraWS(); });

  // Microphone state
  const micDevices = ref<{ id: string; name: string }[]>([]);
  const selectedMic = ref('default');
  const micLiveMode = ref(false);
  const micListenBrowser = ref(false);
  const micVolume = ref(0);
  const micDataBuffer = ref(new Int16Array(1024));

  let micWs: WebSocket | null = null;

  const fetchMicrophones = async () => {
    try {
      const res = await axios.get('/system/microphones');
      micDevices.value = res.data;
      if (res.data.length > 0 && selectedMic.value === 'default') selectedMic.value = res.data[0].id;
    } catch (err) {}
  };

  const connectMicWS = () => {
    if (micWs) { micWs.onclose = null; micWs.onerror = null; micWs.close(); }
    const wsUrl = buildWebSocketUrl(`/ws/microphone?device=${encodeURIComponent(selectedMic.value)}`);
    micWs = new WebSocket(wsUrl);
    micWs.binaryType = 'arraybuffer';
    micWs.onopen = () => { micVolume.value = 0; };
    micWs.onmessage = async (e) => {
      const samples = new Int16Array(e.data);
      let sum = 0;
      for (let i = 0; i < samples.length; i++) {
        sum += Math.abs(samples[i]);
        if (i < micDataBuffer.value.length) micDataBuffer.value[i] = samples[i];
      }
      micVolume.value = Math.min(100, (sum / samples.length) / 327.68 * 2.5);
    };
    micWs.onerror = () => { /* auto-retry via onclose */ };
    micWs.onclose = () => {
      micVolume.value = 0;
      if (micLiveMode.value) {
        setTimeout(() => { if (micLiveMode.value) connectMicWS(); }, 2000);
      }
    };
    return micWs;
  };

  const stopMicWS = () => {
    if (micWs) { micWs.close(); micWs = null; }
    micVolume.value = 0;
  };

  const closeSensorWS = () => {
    if (sensorWs) { sensorWs.close(); sensorWs = null; }
  };

  watch(selectedMic, () => { if (micLiveMode.value) connectMicWS(); });

  return {
    // Sensors
    sensorData, fanData, sensorsLoading, sensorInterval,
    sensorHistory, sensorVisibility,
    connectSensorsWS, fetchSensors, closeSensorWS,
    sensorChartOptions, groupedSensors, toggleAllSensors,
    // Camera
    cameras, selectedCamera, cameraLiveMode, cameraLoading,
    cameraFrameUrl, cameraSnapshotUrl, cameraStreamUrl,
    fetchCameras, connectCameraWS, stopCameraWS, refreshCamera,
    // Mic
    micDevices, selectedMic, micLiveMode, micListenBrowser, micVolume, micDataBuffer,
    fetchMicrophones, connectMicWS, stopMicWS,
  };
}
