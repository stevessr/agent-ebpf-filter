<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import type { ProcessInfo } from "../../composables/monitor/useProcessObserver";

const props = defineProps<{
  open: boolean;
  process: ProcessInfo | null;
  processList: ProcessInfo[];
}>();

const emit = defineEmits<{
  (e: "update:open", value: boolean): void;
  (e: "selectPid", pid: number): void;
  (e: "signal", pid: number, signal: string): void;
}>();

// ── Live duration counter ───────────────────────────────────────────────

const now = ref(Date.now());
let timer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  timer = setInterval(() => { now.value = Date.now(); }, 1000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});

const isDead = computed(() => {
  if (!props.process) return false;
  return !props.processList.some((p) => p.pid === props.process!.pid);
});

const formatDuration = (seconds: number): string => {
  if (seconds < 0) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
};

const durationLabel = computed(() => {
  if (!props.process?.createTime) return "—";
  const elapsed = (now.value / 1000) - props.process.createTime;
  if (isDead.value) return `lived ${formatDuration(elapsed)}`;
  return formatDuration(elapsed) + " (running)";
});

const formatBytes = (bytes: number): string => {
  if (!bytes || bytes === 0) return "—";
  const u = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), u.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};

const formatTime = (ts?: number): string => {
  if (!ts || ts <= 0) return "—";
  const d = new Date(ts * 1000);
  return d.toLocaleString();
};

const childrenCount = computed(() => {
  if (!props.process) return 0;
  return props.processList.filter((p) => p.ppid === props.process!.pid).length;
});

const ancestorChain = computed<string>(() => {
  if (!props.process) return "";
  const chain: string[] = [];
  let currentPid = props.process.ppid;
  let guard = 0;
  while (currentPid > 0 && currentPid !== props.process!.pid && guard < 20) {
    const parent = props.processList.find((p) => p.pid === currentPid);
    if (parent) {
      chain.push(`[${parent.pid}] ${parent.name}`);
      currentPid = parent.ppid;
    } else {
      chain.push(`[${currentPid}] ?`);
      break;
    }
    guard++;
  }
  return chain.length > 0 ? chain.join(" ← ") : "—";
});
</script>

<template>
  <a-modal
    :open="open"
    title="Process Details"
    :footer="null"
    width="560px"
    @update:open="emit('update:open', $event)"
  >
    <template v-if="process">
      <!-- Header: PID + Name -->
      <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 16px">
        <a-avatar
          :style="{
            backgroundColor: process.cpu > 50 ? '#ff4d4f' : process.cpu > 20 ? '#faad14' : '#1677ff',
            verticalAlign: 'middle',
          }"
          size="small"
        >
          {{ process.name.charAt(0).toUpperCase() }}
        </a-avatar>
        <span style="font-size: 16px; font-weight: 600; color: #1f2937">
          {{ process.name }}
        </span>
        <code style="font-size: 14px; color: #1677ff; font-weight: 700">
          PID {{ process.pid }}
        </code>
      </div>

      <!-- Detail fields -->
      <a-descriptions bordered :column="2" size="small">
        <a-descriptions-item label="PPID" :span="1">
          <code>{{ process.ppid || "—" }}</code>
        </a-descriptions-item>
        <a-descriptions-item label="User" :span="1">
          <span>{{ process.user || "—" }}</span>
        </a-descriptions-item>

        <a-descriptions-item label="CPU" :span="1">
          <a-progress
            :percent="Number((process.cpu ?? 0).toFixed(1))"
            :show-info="true"
            size="small"
            :stroke-color="process.cpu > 50 ? '#ff4d4f' : process.cpu > 20 ? '#faad14' : '#1677ff'"
            style="width: 120px"
          />
        </a-descriptions-item>
        <a-descriptions-item label="Memory" :span="1">
          <a-progress
            :percent="Number((process.mem ?? 0).toFixed(1))"
            :show-info="true"
            size="small"
            :stroke-color="process.mem > 50 ? '#ff4d4f' : process.mem > 20 ? '#faad14' : '#52c41a'"
            style="width: 120px"
          />
        </a-descriptions-item>

        <a-descriptions-item label="GPU Memory" :span="1">
          <span>{{ process.gpuMem ? formatBytes(process.gpuMem) : "—" }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="GPU Util" :span="1">
          <span v-if="process.gpuUtil != null && process.gpuUtil >= 0">
            {{ process.gpuUtil.toFixed(1) }}%
          </span>
          <span v-else style="color: #6b7280">—</span>
        </a-descriptions-item>

        <a-descriptions-item label="Children" :span="1">
          <a-tag color="processing">{{ childrenCount }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Created" :span="1">
          <span :title="String(process.createTime || '')">
            {{ formatTime(process.createTime) }}
          </span>
        </a-descriptions-item>
        <a-descriptions-item label="Duration" :span="1">
          <span :class="isDead ? 'duration-dead' : ''">
            {{ durationLabel }}
          </span>
        </a-descriptions-item>

        <a-descriptions-item label="Minor Faults" :span="1">
          <code>{{ process.minorFaults?.toLocaleString() || "0" }}</code>
        </a-descriptions-item>
        <a-descriptions-item label="Major Faults" :span="1">
          <code>{{ process.majorFaults?.toLocaleString() || "0" }}</code>
        </a-descriptions-item>

        <a-descriptions-item label="Command Line" :span="2">
          <a-typography-text code style="word-break: break-all; font-size: 11px">
            {{ process.cmdline || "—" }}
          </a-typography-text>
        </a-descriptions-item>

        <a-descriptions-item label="Ancestor Chain" :span="2">
          <span style="font-family: ui-monospace, monospace; font-size: 11px">
            {{ ancestorChain }}
          </span>
        </a-descriptions-item>
      </a-descriptions>

      <!-- Actions footer -->
      <div style="margin-top: 16px; display: flex; gap: 8px; justify-content: flex-end">
        <a-button size="small" @click="emit('selectPid', process.pid)">
          Focus in Tree
        </a-button>
        <a-dropdown>
          <a-button size="small" danger>
            Send Signal
          </a-button>
          <template #overlay>
            <a-menu @click="({ key }: { key: string }) => { if (process) emit('signal', (process as NonNullable<typeof process>).pid, key); }">
              <a-menu-item key="SIGTERM">SIGTERM (graceful)</a-menu-item>
              <a-menu-item key="SIGKILL">SIGKILL (force)</a-menu-item>
              <a-menu-item key="SIGSTOP">SIGSTOP (pause)</a-menu-item>
              <a-menu-item key="SIGCONT">SIGCONT (resume)</a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
      </div>
    </template>
    <a-empty v-else description="No process selected" />
  </a-modal>
</template>

<style scoped>
.duration-dead {
  color: #6b7280;
  font-style: italic;
}
</style>
