<script setup lang="ts">
import type { NavMenuGroup } from "../../types/navigation";

const props = defineProps<{
  navGroups: NavMenuGroup[];
  selectedKeys: string[];
  openKeys: string[];
  collapsed: boolean;
}>();

const emit = defineEmits<{
  "update:collapsed": [value: boolean];
  "update:openKeys": [keys: string[]];
  select: [payload: { key: string }];
}>();

const handleCollapse = (value: boolean) => {
  emit("update:collapsed", value);
};

const handleOpenChange = (keys: unknown[]) => {
  emit("update:openKeys", keys.map(String));
};

const handleSelect = ({ key }: { key: string | number }) => {
  emit("select", { key: String(key) });
};
</script>

<template>
  <a-layout-sider
    class="app-side-nav"
    collapsible
    :collapsed="props.collapsed"
    @collapse="handleCollapse"
  >
    <div class="app-side-nav__brand">
      <div class="app-side-nav__logo">eBPF</div>
      <div v-if="!props.collapsed" class="app-side-nav__title">Agent eBPF</div>
    </div>

    <a-menu
      mode="inline"
      :selectedKeys="props.selectedKeys"
      :openKeys="props.collapsed ? [] : props.openKeys"
      @openChange="handleOpenChange"
      @click="handleSelect"
    >
      <a-sub-menu v-for="group in props.navGroups" :key="group.key">
        <template #title>
          <component :is="group.icon" />
          <span>{{ group.title }}</span>
        </template>
        <a-menu-item v-for="item in group.children" :key="item.key">
          <template #icon><component :is="item.icon" /></template>
          {{ item.title }}
        </a-menu-item>
      </a-sub-menu>
    </a-menu>
  </a-layout-sider>
</template>

<style scoped>
.app-side-nav {
  min-height: 100vh;
}

.app-side-nav__brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  padding: 0 16px;
  overflow: hidden;
}

.app-side-nav__logo {
  display: flex;
  flex: 0 0 32px;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--ui-accent);
  color: var(--ui-surface);
  font-weight: 700;
  font-size: 12px;
}

.app-side-nav__title {
  min-width: 0;
  color: var(--ui-text-strong);
  font-size: 16px;
  font-weight: 700;
  white-space: nowrap;
}

/* 浅色主题覆盖 — 侧边栏灰底黑字 */
.app-side-nav:deep(.ant-layout-sider-children),
.app-side-nav:deep(.ant-menu),
.app-side-nav:deep(.ant-menu-sub) {
  background: var(--ui-surface-soft) !important;
  color: var(--ui-text-strong) !important;
}

.app-side-nav:deep(.ant-menu-item),
.app-side-nav:deep(.ant-menu-submenu-title) {
  color: var(--ui-text-strong) !important;
}

.app-side-nav:deep(.ant-menu-item-selected) {
  background: var(--ui-primary-soft) !important;
  color: var(--ui-accent) !important;
}

.app-side-nav:deep(.ant-menu-item-active),
.app-side-nav:deep(.ant-menu-submenu-title:hover) {
  color: var(--ui-accent) !important;
  background: var(--ui-primary-soft) !important;
}

.app-side-nav:deep(.ant-menu-item-selected .ant-menu-item-icon) {
  color: var(--ui-accent) !important;
}

.app-side-nav:deep(.ant-menu-submenu-arrow) {
  color: var(--ui-text-strong) !important;
}
</style>
