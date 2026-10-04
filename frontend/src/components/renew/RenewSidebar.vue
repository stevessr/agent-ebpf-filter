<script setup lang="ts">
import {
  AppstoreOutlined,
  ArrowRightOutlined,
  DashboardOutlined,
  GlobalOutlined,
  NodeIndexOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
} from "@ant-design/icons-vue";

const emit = defineEmits<{
  navigate: [name: string, params?: Record<string, string>];
}>();
</script>

<template>
  <aside class="renew-sidebar">
    <div class="renew-brand">
      <div class="renew-brand__mark">R</div>
      <div>
        <strong>Renew</strong>
        <span>Agent Monitor</span>
      </div>
    </div>

    <nav class="renew-nav" aria-label="Renew navigation">
      <button class="renew-nav__item renew-nav__item--active">
        <DashboardOutlined />
        <span>概览</span>
      </button>
      <button class="renew-nav__item" @click="emit('navigate', 'Dashboard')">
        <AppstoreOutlined />
        <span>事件</span>
      </button>
      <button
        class="renew-nav__item"
        @click="emit('navigate', 'NetworkFlow', { tab: 'overview' })"
      >
        <GlobalOutlined />
        <span>网络</span>
      </button>
      <button
        class="renew-nav__item"
        @click="emit('navigate', 'Monitor', { tab: 'processes' })"
      >
        <NodeIndexOutlined />
        <span>进程</span>
      </button>
      <button
        class="renew-nav__item"
        @click="emit('navigate', 'Config', { tab: 'security' })"
      >
        <SafetyCertificateOutlined />
        <span>规则</span>
      </button>
    </nav>

    <div class="renew-sidebar__footer">
      <button
        class="renew-nav__item"
        @click="emit('navigate', 'Config', { tab: 'runtime' })"
      >
        <SettingOutlined />
        <span>设置</span>
      </button>
      <button class="renew-professional" @click="emit('navigate', 'Dashboard')">
        打开专业工作台
        <ArrowRightOutlined />
      </button>
    </div>
  </aside>
</template>
