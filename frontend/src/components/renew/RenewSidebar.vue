<script setup lang="ts">
import {
  AppstoreOutlined,
  ArrowRightOutlined,
  DashboardOutlined,
  GlobalOutlined,
  NodeIndexOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
} from "@ant-design/icons-vue";

defineProps<{
  activeSection:
    "overview" | "monitoring" | "events" | "network" | "processes" | "rules";
}>();

const emit = defineEmits<{
  navigate: [name: string, params?: Record<string, string>];
  section: [
    section:
      "overview" | "monitoring" | "events" | "network" | "processes" | "rules",
  ];
}>();
</script>

<template>
  <aside class="renew-sidebar">
    <div class="renew-brand">
      <div class="renew-brand__mark">R</div>
      <div>
        <strong>Renew</strong>
        <span>日常监控</span>
      </div>
    </div>

    <nav class="renew-nav" aria-label="Renew 导航">
      <button
        class="renew-nav__item"
        :class="{ 'renew-nav__item--active': activeSection === 'overview' }"
        @click="emit('section', 'overview')"
      >
        <DashboardOutlined />
        <span>概览</span>
      </button>
      <button
        class="renew-nav__item"
        :class="{ 'renew-nav__item--active': activeSection === 'monitoring' }"
        @click="emit('section', 'monitoring')"
      >
        <RadarChartOutlined />
        <span>监控中心</span>
      </button>
      <button
        class="renew-nav__item"
        :class="{ 'renew-nav__item--active': activeSection === 'events' }"
        @click="emit('section', 'events')"
      >
        <AppstoreOutlined />
        <span>事件</span>
      </button>
      <button
        class="renew-nav__item"
        :class="{ 'renew-nav__item--active': activeSection === 'network' }"
        @click="emit('section', 'network')"
      >
        <GlobalOutlined />
        <span>网络</span>
      </button>
      <button
        class="renew-nav__item"
        :class="{ 'renew-nav__item--active': activeSection === 'processes' }"
        @click="emit('section', 'processes')"
      >
        <NodeIndexOutlined />
        <span>进程</span>
      </button>
      <button
        class="renew-nav__item"
        :class="{ 'renew-nav__item--active': activeSection === 'rules' }"
        @click="emit('section', 'rules')"
      >
        <SafetyCertificateOutlined />
        <span>规则</span>
      </button>
    </nav>

    <div class="renew-sidebar__footer">
      <button class="renew-nav__item" @click="emit('section', 'monitoring')">
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
