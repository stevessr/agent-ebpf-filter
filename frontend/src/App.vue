<script setup lang="ts">
import { ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { DashboardOutlined, SettingOutlined, BarChartOutlined, FolderOpenOutlined, PlaySquareOutlined, LinkOutlined, GlobalOutlined, DeploymentUnitOutlined, ClusterOutlined, SafetyCertificateOutlined } from '@ant-design/icons-vue';

const route = useRoute();
const router = useRouter();
const selectedKeys = ref<string[]>(['/dashboard']);

watch(() => route.path, (path) => {
  if (path.startsWith('/tls-capture')) {
    selectedKeys.value = ['/tls-capture'];
  } else if (path.startsWith('/execution-graph')) {
    selectedKeys.value = ['/execution-graph'];
  } else if (path.startsWith('/executor')) {
    selectedKeys.value = ['/executor'];
  } else if (path.startsWith('/network-flow')) {
    selectedKeys.value = ['/network-flow'];
  } else if (path.startsWith('/network')) {
    selectedKeys.value = ['/network'];
  } else if (path.startsWith('/config')) {
    selectedKeys.value = ['/config'];
  } else if (path.startsWith('/dashboard')) {
    selectedKeys.value = ['/dashboard'];
  } else {
    selectedKeys.value = [path];
  }
}, { immediate: true });

const handleMenuClick = ({ key }: { key: string }) => {
  router.push(key);
};
</script>

<template>
  <a-layout class="layout">
    <a-layout-header class="header">
      <div class="logo" style="flex-shrink: 0; min-width: 150px;">
        <a-typography-title :level="3" style="color: white; margin: 0; line-height: 64px; margin-right: 24px;">
          Agent eBPF
        </a-typography-title>
      </div>
      <a-menu
        v-model:selectedKeys="selectedKeys"
        theme="dark"
        mode="horizontal"
        :style="{ lineHeight: '64px', flex: 1, minWidth: 0 }"
        @click="handleMenuClick"
      >
        <a-menu-item key="/dashboard">
          <template #icon><DashboardOutlined /></template>
          Dashboard
        </a-menu-item>
        <a-menu-item key="/monitor">
          <template #icon><BarChartOutlined /></template>
          Monitor
        </a-menu-item>
        <a-menu-item key="/network">
          <template #icon><GlobalOutlined /></template>
          Network
        </a-menu-item>
        <a-menu-item key="/network-flow">
          <template #icon><DeploymentUnitOutlined /></template>
          Traffic
        </a-menu-item>
        <a-menu-item key="/tls-capture">
          <template #icon><SafetyCertificateOutlined /></template>
          TLS 捕获
        </a-menu-item>
        <a-menu-item key="/execution-graph">
          <template #icon><ClusterOutlined /></template>
          Execution Graph
        </a-menu-item>
        <a-menu-item key="/explorer">
          <template #icon><FolderOpenOutlined /></template>
          Explorer
        </a-menu-item>
        <a-menu-item key="/executor">
          <template #icon><PlaySquareOutlined /></template>
          Executor
        </a-menu-item>
        <a-menu-item key="/hooks">
          <template #icon><LinkOutlined /></template>
          Hooks
        </a-menu-item>
        <a-menu-item key="/config">
          <template #icon><SettingOutlined /></template>
          Configuration
        </a-menu-item>
      </a-menu>
    </a-layout-header>
    <a-layout-content class="app-content">
      <router-view></router-view>
    </a-layout-content>
    <a-layout-footer style="text-align: center">
      Agent eBPF Tracker ©2026 Created by Stevessr
    </a-layout-footer>
  </a-layout>
</template>

<style scoped>
.layout {
  min-height: 100vh;
}
.header {
  display: flex;
  align-items: center;
}
.logo {
  float: left;
}

.app-content {
  padding: 16px 16px 20px;
  min-width: 0;
}
</style>
