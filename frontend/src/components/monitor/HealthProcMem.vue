<script setup lang="ts">
import { computed, ref } from 'vue';
import type { ProcessInfo } from '../../composables/useMonitorData';

const props = defineProps<{
  processes: ProcessInfo[];
  systemMemTotal: number;
  formatBytesWithUnit: (bytes: number) => string;
}>();

interface ProcGroup {
  name: string;
  count: number;
  totalMemPercent: number;
  approxBytes: number;
  pids: number[];
  instances: ProcessInfo[];
}

interface ProcTile extends ProcGroup {
  rank: number;
  normalized: number;
  width: number;
  height: number;
  shareOfListed: number;
}

const detailOpen = ref(false);
const selectedGroupName = ref<string | null>(null);

const toBytes = (memPercent: number) => {
  if (props.systemMemTotal > 0) {
    return (memPercent / 100) * props.systemMemTotal;
  }
  return memPercent;
};

const groupedProcesses = computed<ProcGroup[]>(() => {
  const groups: Record<string, ProcGroup> = {};
  for (const p of props.processes) {
    const name = p.name || 'unknown';
    if (!groups[name]) {
      groups[name] = { name, count: 0, totalMemPercent: 0, approxBytes: 0, pids: [], instances: [] };
    }
    const memPercent = Number(p.mem || 0);
    groups[name].count++;
    groups[name].totalMemPercent += memPercent;
    groups[name].approxBytes += toBytes(memPercent);
    groups[name].pids.push(p.pid);
    groups[name].instances.push(p);
  }
  return Object.values(groups)
    .map(group => ({
      ...group,
      instances: [...group.instances].sort((a, b) => (b.mem ?? 0) - (a.mem ?? 0) || (b.cpu ?? 0) - (a.cpu ?? 0) || a.pid - b.pid),
    }))
    .sort((a, b) => b.approxBytes - a.approxBytes || b.totalMemPercent - a.totalMemPercent);
});

const totalMemPercent = computed(() => groupedProcesses.value.reduce((sum, g) => sum + g.totalMemPercent, 0));
const totalApproxBytes = computed(() => groupedProcesses.value.reduce((sum, g) => sum + g.approxBytes, 0));
const maxLogWeight = computed(() => {
  const maxBytes = groupedProcesses.value[0]?.approxBytes || 0;
  const weight = Math.log1p(Math.max(maxBytes, 0) / (1024 * 1024));
  return weight > 0 ? weight : 1;
});

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

const tiles = computed<ProcTile[]>(() => groupedProcesses.value.map((group, index) => {
  const bytes = Math.max(group.approxBytes, 0);
  const logWeight = Math.log1p(bytes / (1024 * 1024));
  const normalized = maxLogWeight.value > 0 ? logWeight / maxLogWeight.value : 0;
  const shareOfListed = totalApproxBytes.value > 0 ? (bytes / totalApproxBytes.value) * 100 : 0;

  return {
    ...group,
    rank: index + 1,
    normalized,
    width: clamp(Math.round(180 + normalized * 280), 180, 460),
    height: clamp(Math.round(96 + normalized * 88), 96, 184),
    shareOfListed,
  };
}));

const selectedGroup = computed(() => groupedProcesses.value.find(g => g.name === selectedGroupName.value) ?? null);

const detailColumns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 88, sorter: (a: ProcessInfo, b: ProcessInfo) => a.pid - b.pid },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 90, align: 'right' as const, sorter: (a: ProcessInfo, b: ProcessInfo) => (a.cpu ?? 0) - (b.cpu ?? 0), defaultSortOrder: 'descend' as const },
  { title: 'Mem %', dataIndex: 'mem', key: 'mem', width: 90, align: 'right' as const, sorter: (a: ProcessInfo, b: ProcessInfo) => (a.mem ?? 0) - (b.mem ?? 0) },
  { title: 'User', dataIndex: 'user', key: 'user', width: 120, sorter: (a: ProcessInfo, b: ProcessInfo) => (a.user || '').localeCompare(b.user || '') },
  { title: 'Started', dataIndex: 'createTime', key: 'createTime', width: 180, sorter: (a: ProcessInfo, b: ProcessInfo) => (a.createTime ?? 0) - (b.createTime ?? 0) },
  { title: 'Command', dataIndex: 'cmdline', key: 'cmdline', ellipsis: true },
];

const formatPercent = (value: number) => `${value.toFixed(1)}%`;
const formatDateTime = (ts: number) => {
  if (!ts) return '-';
  const ms = ts > 1e12 ? ts : ts * 1000;
  const d = new Date(ms);
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString();
};

const memColor = (normalized: number) => {
  const t = Math.min(1, Math.max(0, normalized));
  const hue = 120 - t * 120; // green -> yellow -> red
  const lightness = 52 - t * 10;
  const saturation = 84;
  return {
    background: `linear-gradient(145deg, hsla(${hue}, ${saturation}%, ${lightness + 6}%, 0.96), hsla(${hue - 8}, ${saturation}%, ${lightness - 12}%, 0.96))`,
    borderColor: `hsla(${hue}, ${Math.max(50, saturation - 10)}%, ${Math.max(28, lightness - 16)}%, 0.55)`,
    glowColor: `hsla(${hue}, ${saturation}%, ${Math.min(70, lightness + 14)}%, 0.28)`,
    meterColor: `hsla(${hue}, 100%, 96%, 0.92)`,
  };
};

const openGroupDetail = (group: ProcTile) => {
  selectedGroupName.value = group.name;
  detailOpen.value = true;
};

const closeGroupDetail = () => {
  detailOpen.value = false;
  selectedGroupName.value = null;
};

const tileStyle = (tile: ProcTile) => ({
  width: `${tile.width}px`,
  height: `${tile.height}px`,
  background: memColor(tile.normalized).background,
  borderColor: memColor(tile.normalized).borderColor,
  boxShadow: `0 10px 26px ${memColor(tile.normalized).glowColor}`,
});

const tileTitle = (tile: ProcTile) => [
  `${tile.name}`,
  `估算 RSS: ${props.formatBytesWithUnit(tile.approxBytes)}`,
  `占已列出进程内存: ${formatPercent(tile.shareOfListed)}`,
  `分组内存占比: ${formatPercent(tile.totalMemPercent)}`,
  `PID: ${tile.pids.join(', ')}`,
].join('\n');
</script>

<template>
  <div style="padding-top: 16px;">
    <div style="margin-bottom: 12px; display: flex; justify-content: space-between; align-items: flex-end; gap: 16px; flex-wrap: wrap;">
      <div>
        <div style="font-weight: 600;">Process Memory Blocks ({{ tiles.length }} unique)</div>
        <div style="color: #888; font-size: 12px; margin-top: 4px;">
          Size is log-scaled by estimated RSS from process mem% × total RAM.
        </div>
      </div>
      <div style="display: grid; gap: 4px; text-align: right; font-size: 12px; color: #666;">
        <div>Total RAM: <b>{{ formatBytesWithUnit(systemMemTotal) }}</b></div>
        <div>Summed RSS: <b>{{ formatBytesWithUnit(totalApproxBytes) }}</b> · <b>{{ totalMemPercent.toFixed(1) }}%</b></div>
      </div>
    </div>

    <a-empty v-if="tiles.length === 0" description="No process memory data" />

    <div
      v-else
      style="display: flex; flex-wrap: wrap; gap: 12px; align-items: stretch; max-height: calc(100vh - 320px); overflow-y: auto; padding-right: 4px;"
    >
      <a-tooltip v-for="tile in tiles" :key="tile.name" placement="topLeft">
        <template #title>
          <pre style="margin: 0; white-space: pre-wrap; font-family: inherit;">{{ tileTitle(tile) }}</pre>
        </template>

        <div
          class="proc-mem-tile"
          :style="tileStyle(tile)"
          role="button"
          tabindex="0"
          :aria-label="`查看 ${tile.name} 的详细信息`"
          title="点击查看详细"
          @click="openGroupDetail(tile)"
          @keydown.enter.prevent="openGroupDetail(tile)"
          @keydown.space.prevent="openGroupDetail(tile)"
        >
          <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 8px;">
            <div style="min-width: 0; flex: 1;">
              <div class="proc-mem-tile__name">{{ tile.name }}</div>
              <div class="proc-mem-tile__meta">
                <span>{{ tile.count }} proc{{ tile.count > 1 ? 's' : '' }}</span>
                <span>#{{ tile.rank }}</span>
              </div>
            </div>
            <a-tag color="blue" style="margin-right: 0;">{{ formatPercent(tile.totalMemPercent) }}</a-tag>
          </div>

          <div style="display: flex; flex-direction: column; gap: 6px; margin-top: 12px;">
            <div class="proc-mem-tile__value">{{ formatBytesWithUnit(tile.approxBytes) }}</div>
            <div class="proc-mem-tile__subvalue">{{ formatPercent(tile.shareOfListed) }} of listed RSS</div>
          </div>

          <div class="proc-mem-tile__pidline">
            {{ tile.pids.slice(0, 6).join(', ') }}<span v-if="tile.pids.length > 6">…</span>
          </div>

          <div class="proc-mem-tile__hint">
            点击查看详细
          </div>

          <div class="proc-mem-tile__meter" aria-hidden="true">
            <span
              :style="{
                width: `${Math.min(100, Math.max(0, tile.shareOfListed))}%`,
                background: memColor(tile.normalized).meterColor,
              }"
            />
          </div>
        </div>
      </a-tooltip>
    </div>

    <a-modal
      v-model:open="detailOpen"
      :title="selectedGroup ? `${selectedGroup.name} 详细信息` : 'Process Memory Details'"
      width="1120px"
      :footer="null"
      @cancel="closeGroupDetail"
    >
      <template v-if="selectedGroup">
        <a-row :gutter="12" style="margin-bottom: 16px;">
          <a-col :span="6">
            <a-statistic title="Instances" :value="selectedGroup.count" />
          </a-col>
          <a-col :span="6">
            <a-statistic title="Est. RSS" :value="formatBytesWithUnit(selectedGroup.approxBytes)" />
          </a-col>
          <a-col :span="6">
            <a-statistic title="Sum Mem %" :value="selectedGroup.totalMemPercent" :precision="1" suffix="%" />
          </a-col>
          <a-col :span="6">
            <a-statistic title="Share of Listed" :value="selectedGroup.approxBytes > 0 ? (selectedGroup.approxBytes / totalApproxBytes) * 100 : 0" :precision="1" suffix="%" />
          </a-col>
        </a-row>

        <a-card size="small" :bordered="false" style="background: #fafafa; margin-bottom: 16px;">
          <a-space direction="vertical" size="small" style="width: 100%;">
            <div><span style="color:#888;">PIDs:</span> <span style="font-family: monospace;">{{ selectedGroup.pids.join(', ') }}</span></div>
            <div><span style="color:#888;">Approx RSS:</span> <b>{{ formatBytesWithUnit(selectedGroup.approxBytes) }}</b></div>
            <div><span style="color:#888;">Peak instance mem:</span> <b>{{ formatPercent(selectedGroup.instances[0]?.mem ?? 0) }}</b></div>
          </a-space>
        </a-card>

        <a-table
          :dataSource="selectedGroup.instances"
          :columns="detailColumns"
          row-key="pid"
          size="small"
          :pagination="{ pageSize: 10, showSizeChanger: true, pageSizeOptions: ['10', '20', '50'] }"
          :scroll="{ x: 860, y: 420 }"
        >
          <template #bodyCell="{ column, record, text }">
            <template v-if="column.key === 'pid'">
              <span style="font-family: monospace;">{{ text }}</span>
            </template>
            <template v-if="column.key === 'cpu'">
              <span>{{ (text ?? 0).toFixed(1) }}%</span>
            </template>
            <template v-if="column.key === 'mem'">
              <span style="font-weight: 600;">{{ (text ?? 0).toFixed(1) }}%</span>
            </template>
            <template v-if="column.key === 'cmdline'">
              <code
                style="font-size: 11px; white-space: pre-wrap; word-break: break-word;"
                :title="record.cmdline || ''"
              >{{ text || '-' }}</code>
            </template>
            <template v-if="column.key === 'createTime'">
              <span style="font-size: 12px; color: #666;">{{ formatDateTime(text ?? 0) }}</span>
            </template>
          </template>
        </a-table>

        <div style="margin-top: 12px; color: #888; font-size: 12px;">
          Started times are shown in local time when available.
        </div>
      </template>
      <a-empty v-else description="No group selected" />
    </a-modal>
  </div>
</template>

<style scoped>
.proc-mem-tile {
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 14px 16px 18px;
  border-radius: 14px;
  color: #fff;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.16);
  transition: transform 0.18s ease, box-shadow 0.18s ease, filter 0.18s ease;
  cursor: pointer;
  outline: none;
}

.proc-mem-tile:hover {
  transform: translateY(-2px);
  filter: saturate(1.05);
}

.proc-mem-tile:focus-visible {
  box-shadow: 0 0 0 3px rgba(24, 144, 255, 0.28), 0 10px 26px rgba(24, 144, 255, 0.18);
}

.proc-mem-tile__name {
  font-size: 14px;
  font-weight: 700;
  line-height: 1.25;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.proc-mem-tile__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
  font-size: 12px;
  opacity: 0.82;
}

.proc-mem-tile__value {
  font-size: 22px;
  font-weight: 800;
  line-height: 1;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
}

.proc-mem-tile__subvalue {
  font-size: 12px;
  opacity: 0.86;
}

.proc-mem-tile__pidline {
  margin-top: 10px;
  font-size: 11px;
  opacity: 0.78;
  line-height: 1.35;
  min-height: 16px;
  word-break: break-all;
}

.proc-mem-tile__hint {
  margin-top: 10px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  opacity: 0.82;
}

.proc-mem-tile__meter {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 4px;
  background: rgba(255, 255, 255, 0.12);
}

.proc-mem-tile__meter > span {
  display: block;
  height: 100%;
  background: rgba(255, 255, 255, 0.9);
  border-radius: 0 999px 999px 0;
}
</style>
