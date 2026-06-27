<script setup lang="ts">
import {
  PlayCircleOutlined,
  StopOutlined,
  DeleteOutlined,
  InfoCircleOutlined,
} from "@ant-design/icons-vue";

defineProps<{
  security: any;
  plugins: any[];
  loadBpf: (id: string) => Promise<void>;
  unloadBpf: (id: string) => Promise<void>;
  fetchPlugins: () => Promise<void>;
}>();

const emit = defineEmits<{
  (e: "remove-lsm-path", path: string): void;
  (e: "remove-lsm-name", name: string): void;
  (e: "remove-lsm-file", file: string): void;
  (e: "remove-cgroup-ip", ip: string): void;
  (e: "remove-cgroup-port", port: number): void;
}>();
</script>

<template>
  <a-card
    title="活跃内核拦截状态与自定义插件监控"
    size="small"
    style="margin-top: 24px"
  >
    <a-tabs default-active-key="core-rules" size="small">
      <a-tab-pane key="core-rules" tab="内核核心阻断列表 (Core Lists)">
        <div class="monitor-stats">
          <a-row :gutter="16" style="margin-bottom: 16px">
            <a-col :span="6">
              <a-card size="small" class="stat-card">
                <a-statistic
                  title="LSM 检查总数"
                  :value="
                    security.lsmEnforcerStatus.value.stats.execChecked +
                    security.lsmEnforcerStatus.value.stats.fileChecked
                  "
                />
              </a-card>
            </a-col>
            <a-col :span="6">
              <a-card size="small" class="stat-card">
                <a-statistic
                  title="LSM 执行阻断数"
                  :value="security.lsmEnforcerStatus.value.stats.execBlocked"
                />
              </a-card>
            </a-col>
            <a-col :span="6">
              <a-card size="small" class="stat-card">
                <a-statistic
                  title="LSM 文件阻断数"
                  :value="security.lsmEnforcerStatus.value.stats.fileBlocked"
                />
              </a-card>
            </a-col>
            <a-col :span="6">
              <a-card size="small" class="stat-card">
                <a-statistic
                  title="cgroup2 出站拦截"
                  :value="security.cgroupSandboxStatus.value.stats.blocked"
                />
              </a-card>
            </a-col>
          </a-row>
        </div>

        <div style="margin-top: 12px">
          <a-list bordered size="small" header="当前激活的拦截项">
            <a-list-item
              v-for="path in security.lsmEnforcerStatus.value.blockedExecPaths"
              :key="`path-${path}`"
            >
              <div class="rule-item">
                <a-tag color="red">LSM 路径执行拦截</a-tag>
                <code>{{ path }}</code>
              </div>
              <template #actions>
                <a-button
                  size="small"
                  danger
                  type="text"
                  @click="emit('remove-lsm-path', path)"
                >
                  <template #icon><DeleteOutlined /></template>解封
                </a-button>
              </template>
            </a-list-item>

            <a-list-item
              v-for="name in security.lsmEnforcerStatus.value.blockedExecNames"
              :key="`name-${name}`"
            >
              <div class="rule-item">
                <a-tag color="volcano">LSM Basename 执行拦截</a-tag>
                <code>{{ name }}</code>
              </div>
              <template #actions>
                <a-button
                  size="small"
                  danger
                  type="text"
                  @click="emit('remove-lsm-name', name)"
                >
                  <template #icon><DeleteOutlined /></template>解封
                </a-button>
              </template>
            </a-list-item>

            <a-list-item
              v-for="file in security.lsmEnforcerStatus.value.blockedFileNames"
              :key="`file-${file}`"
            >
              <div class="rule-item">
                <a-tag color="orange">LSM 属性阻断 (创建/打开/软链/删除)</a-tag>
                <code>{{ file }}</code>
              </div>
              <template #actions>
                <a-button
                  size="small"
                  danger
                  type="text"
                  @click="emit('remove-lsm-file', file)"
                >
                  <template #icon><DeleteOutlined /></template>解封
                </a-button>
              </template>
            </a-list-item>

            <a-list-item
              v-for="ip in security.cgroupSandboxStatus.value.blockedIPs"
              :key="`ip-${ip}`"
            >
              <div class="rule-item">
                <a-tag color="blue">网络精确 IP 拦截</a-tag>
                <code>{{ ip }}</code>
              </div>
              <template #actions>
                <a-button
                  size="small"
                  danger
                  type="text"
                  @click="emit('remove-cgroup-ip', ip)"
                >
                  <template #icon><DeleteOutlined /></template>解封
                </a-button>
              </template>
            </a-list-item>

            <a-list-item
              v-for="port in security.cgroupSandboxStatus.value.blockedPorts"
              :key="`port-${port}`"
            >
              <div class="rule-item">
                <a-tag color="purple">目的端口拦截</a-tag>
                <code>dst_port == {{ port }}</code>
              </div>
              <template #actions>
                <a-button
                  size="small"
                  danger
                  type="text"
                  @click="emit('remove-cgroup-port', port)"
                >
                  <template #icon><DeleteOutlined /></template>解封
                </a-button>
              </template>
            </a-list-item>

            <div
              v-if="
                !security.lsmEnforcerStatus.value.blockedExecPaths?.length &&
                !security.lsmEnforcerStatus.value.blockedExecNames?.length &&
                !security.lsmEnforcerStatus.value.blockedFileNames?.length &&
                !security.cgroupSandboxStatus.value.blockedIPs?.length &&
                !security.cgroupSandboxStatus.value.blockedPorts?.length
              "
              style="padding: 32px; text-align: center; color: #6b7280"
            >
              <InfoCircleOutlined /> 暂无活跃内核阻断规则。
            </div>
          </a-list>
        </div>
      </a-tab-pane>

      <a-tab-pane
        key="plugins"
        tab="自生成的自定义 eBPF 过滤插件 (Block Plugins)"
      >
        <a-list
          bordered
          size="small"
          :data-source="plugins.filter((p) => p.id.startsWith('visual-'))"
        >
          <template #renderItem="{ item }">
            <a-list-item :key="item.id">
              <div class="plugin-item-info">
                <a-tag color="purple">eBPF 插件</a-tag>
                <strong style="margin-right: 12px">{{ item.name }}</strong>
                <code style="font-size: 11px; margin-right: 12px">{{
                  item.id
                }}</code>
                <a-tag :color="item.loaded ? 'green' : 'orange'">{{
                  item.loaded ? "挂载拦截中" : "未装载"
                }}</a-tag>
              </div>
              <template #actions>
                <a-button
                  v-if="item.loaded"
                  size="small"
                  danger
                  @click="unloadBpf(item.id).then(() => fetchPlugins())"
                >
                  <template #icon><StopOutlined /></template>卸载
                </a-button>
                <a-button
                  v-else
                  size="small"
                  type="primary"
                  @click="loadBpf(item.id).then(() => fetchPlugins())"
                >
                  <template #icon><PlayCircleOutlined /></template>装载
                </a-button>
              </template>
            </a-list-item>
          </template>

          <div
            v-if="!plugins.filter((p) => p.id.startsWith('visual-')).length"
            style="padding: 32px; text-align: center; color: #6b7280"
          >
            <InfoCircleOutlined /> 暂无自编译的高级过滤器插件。
          </div>
        </a-list>
      </a-tab-pane>
    </a-tabs>
  </a-card>
</template>

<style scoped>
.stat-card {
  border-radius: 6px;
  background: #fafafa;
}
.rule-item {
  display: flex;
  align-items: center;
  gap: 12px;
}
.plugin-item-info {
  display: flex;
  align-items: center;
}
</style>
