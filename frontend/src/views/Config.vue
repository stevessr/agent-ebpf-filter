<script setup lang="ts">
import { ref, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  TagOutlined, SafetyCertificateOutlined, ReloadOutlined,
  ThunderboltOutlined, BookOutlined, ClusterOutlined,
} from '@ant-design/icons-vue';
import ConfigRegistryTab from '../components/config/ConfigRegistryTab.vue';
import ConfigSecurityTab from '../components/config/ConfigSecurityTab.vue';
import ConfigRuntimeTab from '../components/config/ConfigRuntimeTab.vue';
import ConfigMLTab from '../components/config/ConfigMLTab.vue';
import ConfigDocsTab from '../components/config/ConfigDocsTab.vue';
import ConfigClusterTab from '../components/config/ConfigClusterTab.vue';
import { useConfigRegistry } from '../composables/useConfigRegistry';
import { useConfigSecurity } from '../composables/useConfigSecurity';
import { useConfigRuntime } from '../composables/useConfigRuntime';
import { useConfigML } from '../composables/useConfigML';
import { useConfigCluster } from '../composables/useConfigCluster';

// ── Composable Instantiations ──
const registry = useConfigRegistry();
const security = useConfigSecurity();
const runtime = useConfigRuntime();
const ml = useConfigML();
const cluster = useConfigCluster();

const {
  fetchTags, fetchTrackedComms, fetchTrackedPaths, fetchTrackedPrefixes,
} = registry;
const { fetchRules, fetchDisabledEventTypes } = security;
const { fetchRuntime } = runtime;
const { fetchMLStatus, fetchAllSamples, fetchExistingCommandData } = ml;
const {
  updateClusterTargetFromStorage, fetchClusterState, fetchClusterNodes,
} = cluster;

// ── Routing ──
const route = useRoute();
const router = useRouter();
const activeTabKey = ref((route.params.tab as string) || 'registry');

watch(() => route.params.tab, (tab) => {
  if (tab) activeTabKey.value = tab as string;
});

watch(activeTabKey, (val) => {
  if (val !== route.params.tab) {
    router.replace({ name: 'Config', params: { tab: val } });
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
  await fetchMLStatus();
  fetchAllSamples();
  fetchExistingCommandData(true);
});
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%">
    <a-tabs v-model:activeKey="activeTabKey" type="card" size="large" :destroyInactiveTabPane="false">
      <a-tab-pane key="registry">
        <template #tab><span><TagOutlined /> eBPF Registry</span></template>
        <ConfigRegistryTab :registry="registry" />
      </a-tab-pane>

      <a-tab-pane key="security">
        <template #tab><span><SafetyCertificateOutlined /> Security Policies</span></template>
        <ConfigSecurityTab :security="security" />
      </a-tab-pane>

      <a-tab-pane key="system">
        <template #tab><span><ReloadOutlined /> System & Runtime</span></template>
        <ConfigRuntimeTab :runtime="runtime" />
      </a-tab-pane>

      <a-tab-pane key="ml">
        <template #tab><span><ThunderboltOutlined /> ML Classification</span></template>
        <ConfigMLTab :ml="ml" />
      </a-tab-pane>

      <a-tab-pane key="docs">
        <template #tab><span><BookOutlined /> Linux 6.18 LTS</span></template>
        <ConfigDocsTab />
      </a-tab-pane>

      <a-tab-pane key="cluster">
        <template #tab><span><ClusterOutlined /> Cluster Control</span></template>
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
