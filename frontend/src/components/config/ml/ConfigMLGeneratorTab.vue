<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import {
  ReloadOutlined,
  PlayCircleOutlined,
  StopOutlined,
  PlusOutlined,
  SearchOutlined,
  HeartOutlined,
  InfoCircleOutlined,
} from "@ant-design/icons-vue";
import type { useConfigML } from "../../../composables/config/useConfigML";

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

interface ProcessItem {
  pid: number;
  name: string;
  cmdline: string;
  user: string;
}

interface GeneratorItem {
  pid: number;
  name: string;
  cmdline: string;
  user: string;
  registeredAt: string;
}

const processes = ref<ProcessItem[]>([]);
const generators = ref<GeneratorItem[]>([]);

const loadingProcesses = ref(false);
const loadingGenerators = ref(false);

const processSearchText = ref("");

// Manual launch form states
const spawnComm = ref("");
const spawnArgsStr = ref("");
const runningSpawn = ref(false);

// Load generators and system processes
const fetchGenerators = async () => {
  loadingGenerators.value = true;
  try {
    const res = await axios.get("/config/ml/health/generators");
    generators.value = res.data.generators || [];
  } catch (err: any) {
    message.error("获取可信健康进程列表失败: " + (err.response?.data?.error || err.message));
  } finally {
    loadingGenerators.value = false;
  }
};

const fetchProcesses = async () => {
  loadingProcesses.value = true;
  try {
    const res = await axios.get("/config/ml/health/processes");
    processes.value = res.data.processes || [];
  } catch (err: any) {
    message.error("获取系统进程列表失败: " + (err.response?.data?.error || err.message));
  } finally {
    loadingProcesses.value = false;
  }
};

const refreshAll = () => {
  fetchGenerators();
  fetchProcesses();
};

// Register a PID as a health generator
const attachProcess = async (pid: number) => {
  try {
    await axios.post("/config/ml/health/register", { pid });
    message.success(`成功附加可信进程 PID ${pid}`);
    fetchGenerators();
  } catch (err: any) {
    message.error("附加进程失败: " + (err.response?.data?.error || err.message));
  }
};

// Unregister a PID from health generator
const detachProcess = async (pid: number) => {
  try {
    await axios.post("/config/ml/health/unregister", { pid });
    message.success(`已取消附加进程 PID ${pid}`);
    fetchGenerators();
  } catch (err: any) {
    message.error("取消附加进程失败: " + (err.response?.data?.error || err.message));
  }
};

// Spawn a new wrapped command
const spawnProgram = async () => {
  if (!spawnComm.value.trim()) {
    message.warning("请输入程序名或执行路径");
    return;
  }
  runningSpawn.value = true;
  try {
    // Parse args by space
    const args = spawnArgsStr.value
      .trim()
      .split(/\s+/)
      .filter((arg) => arg.length > 0);

    const res = await axios.post("/config/ml/health/run", {
      comm: spawnComm.value.trim(),
      args: args,
    });

    message.success(`程序已成功启动，分配 PID: ${res.data.pid}. 行为已被标记为可信。`);
    spawnComm.value = "";
    spawnArgsStr.value = "";
    fetchGenerators();
  } catch (err: any) {
    message.error("手动启动程序失败: " + (err.response?.data?.error || err.message));
  } finally {
    runningSpawn.value = false;
  }
};

// Filtered running processes computed property
const filteredProcesses = computed(() => {
  const q = processSearchText.value.toLowerCase().trim();
  if (!q) return processes.value;
  return processes.value.filter(
    (p) =>
      p.pid.toString().includes(q) ||
      p.name.toLowerCase().includes(q) ||
      (p.cmdline && p.cmdline.toLowerCase().includes(q)) ||
      (p.user && p.user.toLowerCase().includes(q))
  );
});

// Setup lists on mount
onMounted(() => {
  refreshAll();
});

// Table column definitions
const processColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 80, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: "进程名称", dataIndex: "name", key: "name", width: 140, ellipsis: true },
  { title: "启动命令 (Cmdline)", dataIndex: "cmdline", key: "cmdline", ellipsis: true },
  { title: "执行用户", dataIndex: "user", key: "user", width: 100 },
  { title: "操作", key: "action", width: 90 },
];

const generatorColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 80 },
  { title: "进程名称", dataIndex: "name", key: "name", width: 110, ellipsis: true },
  { title: "用户", dataIndex: "user", key: "user", width: 80 },
  { title: "附加时间", dataIndex: "registeredAt", key: "registeredAt", width: 130 },
  { title: "操作", key: "action", width: 90 },
];
</script>

<template>
  <!-- Main Explanatory Alert -->
  <a-col :xs="24">
    <a-card size="small" class="generator-header-card">
      <div class="card-header-title">
        <HeartOutlined class="health-icon" />
        <span>可信健康数据生成器 (Healthy Dataset Generator)</span>
        <a-button type="link" size="small" @click="refreshAll" :loading="loadingGenerators || loadingProcesses" style="margin-left: auto">
          <ReloadOutlined /> 刷新列表
        </a-button>
      </div>

      <a-alert type="success" show-icon class="health-alert" style="margin-top: 10px">
        <template #message>
          <span style="font-weight: 600">利用可信运行期行为，生成零误伤 ML 健康训练基准</span>
        </template>
        <template #description>
          <div class="alert-desc">
            为防止机器学习模型在业务运行中出现误判定（如拦截正常的构建指令或包管理更新），我们需要收集可信、健康的运行数据作为 <b>ALLOW (放行类别=0)</b> 样本训练模型。
            当你在本面板中<b>附加到现有进程</b>或<b>手动运行新程序</b>后，系统会：
            <ul style="margin: 4px 0 0 16px; padding: 0">
              <li>通过内核 eBPF 全程追踪该进程及其派生的所有子命令树。</li>
              <li>在探针级直接做 <b>ALLOW (安全放行)</b> 裁决，免去任何策略拦截。</li>
              <li>自动在训练集中以标签 <code>ALLOW</code> 且来源标记 <code>health-generator</code> 进行流式序列化落盘保存。</li>
            </ul>
          </div>
        </template>
      </a-alert>
    </a-card>
  </a-col>

  <!-- Split Panel Layout -->
  <a-col :xs="24" :lg="10">
    <a-space direction="vertical" style="width: 100%" :size="16">
      
      <!-- Launch New Program Card -->
      <a-card title="手动启动新可信程序" size="small">
        <a-form layout="vertical" @submit.prevent="spawnProgram">
          <a-form-item label="程序名字 / 可执行文件路径" required>
            <a-input 
              v-model:value="spawnComm" 
              placeholder="例如: bun, go, make, /usr/bin/python3" 
            />
          </a-form-item>
          <a-form-item label="执行参数 (空格分隔)">
            <a-input 
              v-model:value="spawnArgsStr" 
              placeholder="例如: run build, test, install" 
            />
          </a-form-item>
          <a-button 
            type="primary" 
            html-type="submit" 
            block 
            class="spawn-btn"
            :loading="runningSpawn"
          >
            <PlayCircleOutlined /> 启动程序并开始录制
          </a-button>
        </a-form>
      </a-card>

      <!-- Active Attached Generators Card -->
      <a-card size="small">
        <template #title>
          <span>已附加的可信进程 (录制中)</span>
          <a-tag color="success" style="margin-left: 8px">{{ generators.length }}</a-tag>
        </template>
        
        <a-table
          size="small"
          :columns="generatorColumns"
          :data-source="generators"
          :loading="loadingGenerators"
          :pagination="{ pageSize: 5 }"
          row-key="pid"
          class="generators-table"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'registeredAt'">
              <span>{{ new Date(record.registeredAt).toLocaleTimeString() }}</span>
            </template>
            <template v-else-if="column.key === 'action'">
              <a-popconfirm
                title="确定要取消追踪该进程吗？"
                ok-text="是"
                cancel-text="否"
                @confirm="detachProcess(record.pid)"
              >
                <a-button type="link" danger size="small" class="detach-action-btn">
                  <StopOutlined /> 停止录制
                </a-button>
              </a-popconfirm>
            </template>
            <template v-else-if="column.key === 'name'">
              <a-tooltip :title="record.cmdline || record.name">
                <code style="color: #096dd9">{{ record.name }}</code>
              </a-tooltip>
            </template>
          </template>
        </a-table>
      </a-card>
    </a-space>
  </a-col>

  <!-- Process List (Right Column) -->
  <a-col :xs="24" :lg="14">
    <a-card size="small">
      <template #title>
        <span>附加到已有系统进程</span>
      </template>
      <template #extra>
        <a-input-search
          v-model:value="processSearchText"
          placeholder="搜索 PID/程序名/命令行/用户..."
          style="width: 240px"
          size="small"
          allow-clear
        >
          <template #enterButton>
            <SearchOutlined />
          </template>
        </a-input-search>
      </template>

      <a-table
        size="small"
        :columns="processColumns"
        :data-source="filteredProcesses"
        :loading="loadingProcesses"
        :pagination="{ pageSize: 8, showSizeChanger: false }"
        row-key="pid"
        class="processes-table"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'pid'">
            <a-tag color="default" style="font-family: monospace">{{ record.pid }}</a-tag>
          </template>
          
          <template v-else-if="column.key === 'name'">
            <code style="font-weight: 600">{{ record.name }}</code>
          </template>

          <template v-else-if="column.key === 'cmdline'">
            <a-tooltip :title="record.cmdline">
              <span class="cmdline-cell">{{ record.cmdline }}</span>
            </a-tooltip>
          </template>

          <template v-else-if="column.key === 'user'">
            <span style="color: #595959">{{ record.user || '—' }}</span>
          </template>

          <template v-else-if="column.key === 'action'">
            <a-button 
              type="link" 
              size="small" 
              :disabled="generators.some((g) => g.pid === record.pid)"
              @click="attachProcess(record.pid)"
              class="attach-btn"
            >
              <PlusOutlined /> 附加
            </a-button>
          </template>
        </template>
      </a-table>
    </a-card>
  </a-col>
</template>

<style scoped>
.generator-header-card {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.03);
}

.card-header-title {
  display: flex;
  align-items: center;
  font-size: 15px;
  font-weight: 600;
}

.health-icon {
  margin-right: 8px;
  color: #52c41a;
}

.health-alert {
  background: rgba(82, 196, 26, 0.04);
  border: 1px dashed rgba(82, 196, 26, 0.2);
}

.alert-desc {
  font-size: 12px;
  color: #555;
  line-height: 1.6;
}

.spawn-btn {
  background: #52c41a;
  border-color: #52c41a;
}

.spawn-btn:hover, .spawn-btn:focus {
  background: #73d13d;
  border-color: #73d13d;
}

.cmdline-cell {
  font-family: monospace;
  font-size: 11px;
  color: #595959;
  display: block;
  max-width: 320px;
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
}

.attach-btn {
  color: #52c41a;
}

.attach-btn:hover {
  color: #73d13d;
}

.detach-action-btn {
  color: #ff4d4f;
}

.detach-action-btn:hover {
  color: #ff7875;
}

.generators-table :deep(.ant-table-cell),
.processes-table :deep(.ant-table-cell) {
  padding: 8px 8px !important;
}

code {
  font-family: Consolas, "Liberation Mono", Courier, monospace;
}
</style>
