<script setup lang="ts">
import FileBrowserPanel from '../../components/explorer/FileBrowserPanel.vue';
</script>

<template>
  <div class="explorer-page">
    <FileBrowserPanel route-path="/explorer" />
  </div>
</template>

<style scoped>
.explorer-page {
  background: #fff;
  padding: 24px;
  min-height: 100%;
}
</style>
