<script setup lang="ts">
import AppSideNav from "./components/layout/AppSideNav.vue";
import AppWorkbenchTabs from "./components/layout/AppWorkbenchTabs.vue";
import { useWorkbenchNavigation } from "./composables/navigation/useWorkbenchNavigation";

const {
  navGroups,
  collapsed,
  openedTabs,
  activeTabKey,
  selectedMenuKeys,
  openKeys,
  handleMenuSelect,
  handleTabChange,
  handleTabEdit,
  updateOpenKeys,
} = useWorkbenchNavigation();
</script>

<template>
  <a-layout class="app-layout">
    <AppSideNav
      v-model:collapsed="collapsed"
      :nav-groups="navGroups"
      :selected-keys="selectedMenuKeys"
      :open-keys="openKeys"
      @update:open-keys="updateOpenKeys"
      @select="handleMenuSelect"
    />

    <a-layout class="app-layout__main">
      <AppWorkbenchTabs
        :tabs="openedTabs"
        :active-key="activeTabKey"
        @change="handleTabChange"
        @edit="handleTabEdit"
      />

      <a-layout-content class="app-content">
        <router-view />
      </a-layout-content>

      <a-layout-footer class="app-footer">
        Agent eBPF Tracker ©2026 Created by Stevessr
      </a-layout-footer>
    </a-layout>
  </a-layout>
</template>

<style scoped>
.app-layout {
  min-height: 100vh;
}

.app-layout__main {
  min-width: 0;
  min-height: 100vh;
}

.app-content {
  min-width: 0;
  padding: 16px 16px 20px;
  overflow: auto;
}

.app-footer {
  padding: 12px 16px;
  text-align: center;
}
</style>
