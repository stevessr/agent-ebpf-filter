<script setup lang="ts">
import { SearchOutlined } from "@ant-design/icons-vue";

defineProps<{
  search: string;
  onlyAgents: boolean;
  netRecv: number;
  netSent: number;
  formatRate: (bytes: number) => string;
}>();

const emit = defineEmits<{
  "update:search": [value: string];
  "update:onlyAgents": [value: boolean];
}>();
</script>

<template>
  <section class="renew-toolbar">
    <a-input
      :value="search"
      allow-clear
      placeholder="搜索 Agent、动作、文件或网络目标"
      class="renew-search"
      @update:value="emit('update:search', $event)"
    >
      <template #prefix><SearchOutlined /></template>
    </a-input>
    <label class="renew-filter">
      <span>只看 Agent</span>
      <a-switch
        :checked="onlyAgents"
        size="small"
        @update:checked="emit('update:onlyAgents', $event)"
      />
    </label>
    <div class="renew-throughput">
      ↓ {{ formatRate(netRecv) }}
      <span>↑ {{ formatRate(netSent) }}</span>
    </div>
  </section>
</template>
