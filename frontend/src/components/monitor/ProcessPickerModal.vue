<script setup lang="ts">
import { computed, ref } from "vue";
import type { ProcessInfo } from "../../composables/monitor/useMonitorData";

type ProcessRow = ProcessInfo & {
  key: number;
  children?: ProcessRow[];
};

const props = withDefaults(
  defineProps<{
    open: boolean;
    processes: ProcessInfo[];
    selectedPids?: number[];
    loading?: boolean;
    title?: string;
  }>(),
  {
    selectedPids: () => [],
    loading: false,
    title: "选择进程",
  },
);

const emit = defineEmits<{
  (event: "update:open", value: boolean): void;
  (event: "select", processes: ProcessInfo[]): void;
}>();

const processSearch = ref("");
const viewMode = ref<"flat" | "tree">("flat");

const columns = [
  {
    title: "PID",
    dataIndex: "pid",
    key: "pid",
    width: 92,
    sorter: (a: ProcessRow, b: ProcessRow) => a.pid - b.pid,
  },
  {
    title: "Name",
    dataIndex: "name",
    key: "name",
    sorter: (a: ProcessRow, b: ProcessRow) => a.name.localeCompare(b.name),
  },
  {
    title: "PPID",
    dataIndex: "ppid",
    key: "ppid",
    width: 92,
    sorter: (a: ProcessRow, b: ProcessRow) => a.ppid - b.ppid,
  },
  {
    title: "CPU",
    dataIndex: "cpu",
    key: "cpu",
    width: 88,
    align: "right" as const,
    sorter: (a: ProcessRow, b: ProcessRow) => (a.cpu ?? 0) - (b.cpu ?? 0),
    defaultSortOrder: "descend" as const,
  },
  {
    title: "Mem",
    dataIndex: "mem",
    key: "mem",
    width: 88,
    align: "right" as const,
    sorter: (a: ProcessRow, b: ProcessRow) => (a.mem ?? 0) - (b.mem ?? 0),
  },
  {
    title: "User",
    dataIndex: "user",
    key: "user",
    width: 120,
    sorter: (a: ProcessRow, b: ProcessRow) =>
      (a.user || "").localeCompare(b.user || ""),
  },
  { title: "Command Line", dataIndex: "cmdline", key: "cmdline" },
];

const baseRows = computed<ProcessRow[]>(() =>
  props.processes.map((process) => ({
    ...process,
    cpu: process.cpu ?? 0,
    mem: process.mem ?? 0,
    key: process.pid,
  })),
);

const filteredRows = computed(() => {
  const query = processSearch.value.trim().toLowerCase();
  const list = query
    ? baseRows.value.filter(
        (process) =>
          process.name.toLowerCase().includes(query) ||
          String(process.pid).includes(query) ||
          String(process.ppid).includes(query) ||
          (process.cmdline ?? "").toLowerCase().includes(query) ||
          (process.user ?? "").toLowerCase().includes(query),
      )
    : baseRows.value;
  return [...list].sort((a, b) => (b.cpu ?? 0) - (a.cpu ?? 0) || a.pid - b.pid);
});

const tableRows = computed<ProcessRow[]>(() => {
  if (viewMode.value !== "tree") {
    return filteredRows.value;
  }

  const map: Record<number, ProcessRow> = {};
  filteredRows.value.forEach((process) => {
    map[process.pid] = { ...process, children: [] };
  });

  const roots: ProcessRow[] = [];
  filteredRows.value.forEach((process) => {
    const row = map[process.pid];
    const parent = map[process.ppid];
    if (parent && process.ppid !== process.pid) {
      parent.children!.push(row);
    } else {
      roots.push(row);
    }
  });
  return roots;
});

// Row selection state (local copy of selected PIDs)
const selectedRowKeys = ref<number[]>([...props.selectedPids]);

// Sync from props when modal opens
const syncSelectedFromProps = () => {
  selectedRowKeys.value = [...props.selectedPids];
};

// Row selection config for a-table
const rowSelection = computed(() => ({
  selectedRowKeys: selectedRowKeys.value,
  onChange: (keys: any[]) => {
    selectedRowKeys.value = keys.map(Number);
  },
}));

const close = () => {
  emit("update:open", false);
};

const confirmSelection = () => {
  // Find ProcessInfo for each selected key
  const selected = selectedRowKeys.value
    .map((pid) => props.processes.find((p) => p.pid === pid))
    .filter((p): p is ProcessInfo => p !== undefined);
  emit("select", selected);
  close();
};
</script>

<template>
  <a-modal
    :open="open"
    :title="title"
    width="1080px"
    :footer="null"
    destroy-on-close
    @cancel="close"
    @after-open-change="(visible: boolean) => { if (visible) syncSelectedFromProps() }"
  >
    <a-space direction="vertical" size="middle" style="width: 100%">
      <div class="process-picker-toolbar">
        <a-space wrap>
          <a-radio-group
            v-model:value="viewMode"
            button-style="solid"
            size="small"
          >
            <a-radio-button value="flat">Flat</a-radio-button>
            <a-radio-button value="tree">Tree</a-radio-button>
          </a-radio-group>
          <a-input-search
            v-model:value="processSearch"
            allow-clear
            placeholder="搜索 name / PID / PPID / user / cmdline"
            style="width: 360px"
          />
        </a-space>
        <a-space>
          <a-tag color="processing"
            >已选 {{ selectedRowKeys.length }} 个 PID</a-tag
          >
          <a-tag
            >{{ filteredRows.length }} / {{ processes.length }} processes</a-tag
          >
        </a-space>
      </div>

      <a-table
        :data-source="tableRows"
        :columns="columns"
        :loading="loading"
        :row-selection="rowSelection"
        size="small"
        row-key="pid"
        :pagination="{
          pageSize: 12,
          showSizeChanger: true,
          pageSizeOptions: ['12', '25', '50', '100'],
        }"
        :scroll="{ y: 520, x: 980 }"
      >
        <template #bodyCell="{ column, record, text }">
          <template v-if="column.key === 'pid'">
            <code>{{ text }}</code>
          </template>
          <template v-else-if="column.key === 'name'">
            <a-button
              type="link"
              size="small"
              style="padding: 0"
            >
              {{ text || "process" }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'cpu' || column.key === 'mem'">
            {{ Number(text ?? 0).toFixed(1) }}%
          </template>
          <template v-else-if="column.key === 'cmdline'">
            <a-typography-text
              class="process-picker-cmdline"
              :title="text || ''"
            >
              {{ text || "—" }}
            </a-typography-text>
          </template>
        </template>
      </a-table>

      <div class="process-picker-footer">
        <span class="footer-hint">
          勾选需要观察的进程，点击确认
        </span>
        <a-space>
          <a-button @click="close">取消</a-button>
          <a-button type="primary" @click="confirmSelection">
            确认 ({{ selectedRowKeys.length }})
          </a-button>
        </a-space>
      </div>
    </a-space>
  </a-modal>
</template>

<style scoped>
.process-picker-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  align-items: center;
}

.process-picker-cmdline {
  display: inline-block;
  max-width: 420px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.process-picker-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 8px;
  border-top: 1px solid #f0f0f0;
}

.footer-hint {
  font-size: 12px;
  color: #6b7280;
}
</style>
