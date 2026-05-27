<script setup lang="ts">
import { onMounted, watch } from "vue";
import { InfoCircleOutlined } from "@ant-design/icons-vue";
import { useConfigVisualFilter } from "../../composables/config/useConfigVisualFilter";
import { usePlugins } from "../../composables/plugins/usePlugins";
import PluginsVisualTab from "../plugins/PluginsVisualTab.vue";
import QuickCoreRulePanel from "./QuickCoreRulePanel.vue";
import CoreRuleMonitorBoard from "./CoreRuleMonitorBoard.vue";

const props = defineProps<{
  security: any;
}>();

const { currentConfig, generatedCode, updateMetadata } = useConfigVisualFilter();
const { compileBpf, loadBpf, unloadBpf, upsertPlugin, fetchPlugins, plugins } = usePlugins();

// Auto update metadata on state changes
watch(
  [
    () => currentConfig.value.type,
    () => currentConfig.value.value,
    () => currentConfig.value.operator,
    () => currentConfig.value.action,
  ],
  () => {
    updateMetadata();
  },
  { immediate: true }
);

onMounted(async () => {
  await fetchPlugins();
});

// Rule removal handlers
const handleRemoveLsmPath = async (path: string) => {
  props.security.lsmExecPath.value = path;
  await props.security.unblockLsmExecPath();
};

const handleRemoveLsmName = async (name: string) => {
  props.security.lsmExecName.value = name;
  await props.security.unblockLsmExecName();
};

const handleRemoveLsmFile = async (file: string) => {
  props.security.lsmFileName.value = file;
  await props.security.unblockLsmFileName();
};

const handleRemoveCgroupIP = async (ip: string) => {
  props.security.cgroupTargetIP.value = ip;
  await props.security.unblockCgroupIP();
};

const handleRemoveCgroupPort = async (port: number) => {
  props.security.cgroupTargetPort.value = port;
  await props.security.unblockCgroupPort();
};
</script>

<template>
  <div class="visual-filter-tab">
    <a-tabs v-model:activeKey="currentConfig.value.visualEditorMode" type="card">
      <a-tab-pane key="blocks">
        <template #tab>低代码多积木工作台</template>
        <a-alert
          type="info"
          show-icon
          style="margin-bottom: 16px"
          message="可视化 eBPF 编辑已升级为多积木画布"
          description="通过 Trigger / Condition / Map / Action 积木组合生成 eBPF C 源码，可一键注册、编译并加载为插件；下方"快速核心规则"仍保留对内置 cgroup/LSM map 的直接写入。"
        />
        <PluginsVisualTab />
      </a-tab-pane>

      <a-tab-pane key="quick">
        <template #tab>快速核心规则与监控</template>
        <QuickCoreRulePanel
          :current-config="currentConfig"
          :generated-code="generatedCode"
          :security="security"
          :update-metadata="updateMetadata"
          :compile-bpf="compileBpf"
          :load-bpf="loadBpf"
          :fetch-plugins="fetchPlugins"
        />

        <CoreRuleMonitorBoard
          :security="security"
          :plugins="plugins"
          :load-bpf="loadBpf"
          :unload-bpf="unloadBpf"
          :fetch-plugins="fetchPlugins"
          @remove-lsm-path="handleRemoveLsmPath"
          @remove-lsm-name="handleRemoveLsmName"
          @remove-lsm-file="handleRemoveLsmFile"
          @remove-cgroup-ip="handleRemoveCgroupIP"
          @remove-cgroup-port="handleRemoveCgroupPort"
        />
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<style scoped>
.visual-filter-tab {
  min-height: 500px;
}
</style>
