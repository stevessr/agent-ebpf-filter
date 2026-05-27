<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  TagOutlined,
  SafetyCertificateOutlined,
  ReloadOutlined,
  BookOutlined,
  ClusterOutlined,
  SettingOutlined,
  CodeOutlined,
} from "@ant-design/icons-vue";
import ConfigRegistryTab from "../../components/config/ConfigRegistryTab.vue";
import ConfigSecurityTab from "../../components/config/ConfigSecurityTab.vue";
import ConfigRuntimeTab from "../../components/config/ConfigRuntimeTab.vue";
import ConfigSystemHealthTab from "../../components/config/ConfigSystemHealthTab.vue";
import ConfigDocsTab from "../../components/config/ConfigDocsTab.vue";
import ConfigClusterTab from "../../components/config/ConfigClusterTab.vue";
import ConfigVisualFilterTab from "../../components/config/ConfigVisualFilterTab.vue";
import { useConfigRegistry } from "../../composables/config/useConfigRegistry";
import { useConfigSecurity } from "../../composables/config/useConfigSecurity";
import { useConfigRuntime } from "../../composables/config/useConfigRuntime";
import { useConfigCluster } from "../../composables/config/useConfigCluster";

// ── Composable Instantiations ──
const registry = useConfigRegistry();
const security = useConfigSecurity();
const runtime = useConfigRuntime();
const cluster = useConfigCluster();

const {
  fetchTags,
  fetchTrackedComms,
  fetchTrackedPaths,
  fetchTrackedPrefixes,
} = registry;
const {
  fetchRules,
  fetchDisabledEventTypes,
  fetchCgroupSandboxStatus,
  fetchLsmEnforcerStatus,
} = security;
const { fetchRuntime } = runtime;
const { updateClusterTargetFromStorage, fetchClusterState, fetchClusterNodes } =
  cluster;

// ── Routing ──
const route = useRoute();
const router = useRouter();
const configTabKeys = new Set([
  "registry",
  "security",
  "visual-filter",
  "runtime",
  "system",
  "docs",
  "cluster",
]);
const normalizeConfigTab = (tab: unknown) =>
  typeof tab === "string" && configTabKeys.has(tab) ? tab : "registry";
const activeTabKey = ref(normalizeConfigTab(route.params.tab));

watch(
  () => route.params.tab,
  (tab) => {
    if (tab === "ml") {
      const subtab = Array.isArray(route.params.subtab)
        ? route.params.subtab[0]
        : route.params.subtab;
      router.replace(
        subtab ? { name: "ML", params: { subtab } } : { name: "ML" }
      );
      return;
    }
    activeTabKey.value = normalizeConfigTab(tab);
  },
  { immediate: true }
);

watch(activeTabKey, (val) => {
  if (route.params.tab !== "ml" && val !== route.params.tab) {
    router.replace({ name: "Config", params: { tab: val } });
  }
});

// ── Initial Data Fetch ──
onMounted(async () => {
  updateClusterTargetFromStorage();
  await fetchClusterState();
  await fetchClusterNodes();
  await fetchRuntime();
  fetchTags();
  fetchTrackedComms();
  fetchTrackedPaths();
  fetchTrackedPrefixes();
  fetchRules();
  fetchDisabledEventTypes();
  fetchCgroupSandboxStatus();
  fetchLsmEnforcerStatus();
});
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%">
    <a-tabs
      v-model:activeKey="activeTabKey"
      type="card"
      size="large"
      :destroyInactiveTabPane="false"
    >
      <a-tab-pane key="registry">
        <template #tab
          ><span><TagOutlined /> eBPF Registry</span></template
        >
        <ConfigRegistryTab :registry="registry" />
      </a-tab-pane>

      <a-tab-pane key="security">
        <template #tab
          ><span
            ><SafetyCertificateOutlined /> Security Policies</span
          ></template
        >
        <ConfigSecurityTab :security="security" />
      </a-tab-pane>

      <a-tab-pane key="visual-filter">
        <template #tab
          ><span><CodeOutlined /> Visual eBPF Filter</span></template
        >
        <ConfigVisualFilterTab :security="security" />
      </a-tab-pane>

      <a-tab-pane key="runtime">
        <template #tab
          ><span><SettingOutlined /> Runtime Config</span></template
        >
        <ConfigRuntimeTab :runtime="runtime" />
      </a-tab-pane>

      <a-tab-pane key="system">
        <template #tab
          ><span><ReloadOutlined /> System Health</span></template
        >
        <ConfigSystemHealthTab :runtime="runtime" />
      </a-tab-pane>

      <a-tab-pane key="docs">
        <template #tab
          ><span><BookOutlined /> Linux 6.18 LTS</span></template
        >
        <ConfigDocsTab />
      </a-tab-pane>

      <a-tab-pane key="cluster">
        <template #tab
          ><span><ClusterOutlined /> Cluster Control</span></template
        >
        <ConfigClusterTab :cluster="cluster" />
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<style scoped>
:deep(.ant-card) {
  border-radius: 8px;
}
</style>
