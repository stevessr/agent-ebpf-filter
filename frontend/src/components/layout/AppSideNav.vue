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
        <template #title>{{ group.title }}</template>
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
  background: #1677ff;
  color: #fff;
  font-weight: 700;
  font-size: 12px;
}

.app-side-nav__title {
  min-width: 0;
  color: #000;
  font-size: 16px;
  font-weight: 700;
  white-space: nowrap;
}
</style>
