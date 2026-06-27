<script setup lang="ts">
import { computed } from "vue";
import {
  CaretDownOutlined,
  CaretRightOutlined,
} from "@ant-design/icons-vue";
import type { ProcessTreeNode } from "../../composables/monitor/useProcessObserver";

const props = defineProps<{
  node: ProcessTreeNode;
  depth: number;
  highlightPid: number;
  expandedSet: Set<number>;
  sslAttachedSet?: Set<number>;
  sslLibForPid?: (pid: number) => string;
}>();

const emit = defineEmits<{
  toggle: [pid: number];
  select: [pid: number];
}>();

const expanded = computed(() => props.expandedSet.has(props.node.pid));
const hasChildren = computed(() => props.node.children.length > 0);
const isHighlighted = computed(() => props.node.pid === props.highlightPid);
</script>

<template>
  <div class="tree-node" :style="{ marginLeft: depth * 22 + 'px' }">
    <div
      class="tree-node-row"
      :class="{ highlighted: isHighlighted }"
      @click="emit('select', node.pid)"
    >
      <span
        v-if="hasChildren"
        class="tree-toggle"
        @click.stop="emit('toggle', node.pid)"
      >
        <CaretDownOutlined v-if="expanded" />
        <CaretRightOutlined v-else />
      </span>
      <span v-else class="tree-toggle" style="visibility: hidden">
        <CaretRightOutlined />
      </span>
      <code class="tree-pid">{{ node.pid }}</code>
      <strong class="tree-name">{{ node.name }}</strong>
      <span v-if="node.ppid && node.ppid !== node.pid" class="tree-ppid">
        ppid {{ node.ppid }}
      </span>
      <span
        v-if="sslAttachedSet?.has(node.pid)"
        class="ssl-dot"
        :title="'SSL: ' + (sslLibForPid?.(node.pid) || 'attached')"
      >●</span>
      <span class="tree-usage">
        CPU {{ (node.cpu ?? 0).toFixed(1) }}% |
        Mem {{ (node.mem ?? 0).toFixed(1) }}%
      </span>
    </div>
    <template v-if="expanded && hasChildren">
      <ProcessTreeNodeDisplay
        v-for="child in node.children"
        :key="child.pid"
        :node="child"
        :depth="depth + 1"
        :highlight-pid="highlightPid"
        :expanded-set="expandedSet"
        @toggle="emit('toggle', $event)"
        @select="emit('select', $event)"
      />
    </template>
  </div>
</template>

<style scoped>
.tree-node-row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: 3px;
  cursor: pointer;
  transition: background 0.1s;
}
.tree-node-row:hover {
  background: #f0f5ff;
}
.tree-node-row.highlighted {
  background: #e6f7ff;
  border: 1px solid #91d5ff;
}

.tree-toggle {
  width: 14px;
  flex-shrink: 0;
  color: #888;
  font-size: 10px;
}
.tree-pid {
  color: #1677ff;
  font-weight: 600;
  min-width: 50px;
}
.tree-name {
  color: #333;
}
.tree-ppid {
  color: #aaa;
  font-size: 11px;
}
.tree-usage {
  margin-left: auto;
  color: #888;
  font-size: 11px;
}
.ssl-dot {
  color: #10b981;
  font-size: 8px;
  margin-right: 4px;
  cursor: help;
}
</style>
