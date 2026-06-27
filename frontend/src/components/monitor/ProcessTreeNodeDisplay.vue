<script setup lang="ts">
import { computed } from "vue";
import {
  CaretDownOutlined,
  CaretRightOutlined,
  AimOutlined,
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
const isDead = computed(() => props.node.dead === true);
</script>

<template>
  <div class="tree-node" :style="{ marginLeft: depth * 22 + 'px' }">
    <div
      class="tree-node-row"
      :class="{ highlighted: isHighlighted, dead: isDead }"
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
      <span v-if="isDead" class="tree-dead-tag">exited</span>
      <span
        v-if="!isDead && !isHighlighted"
        class="tree-focus-btn"
        title="Focus on this process"
        @click.stop="emit('select', node.pid)"
      >
        <AimOutlined />
      </span>
      <span
        v-if="sslAttachedSet?.has(node.pid)"
        class="ssl-dot"
        :title="'SSL: ' + (sslLibForPid?.(node.pid) || 'attached')"
      >●</span>
      <span class="tree-usage" v-if="!isDead">
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
        :ssl-attached-set="sslAttachedSet"
        :ssl-lib-for-pid="sslLibForPid"
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
  color: #4b5563;
  font-size: 10px;
  cursor: pointer;
}
.tree-pid {
  color: #1677ff;
  font-weight: 600;
  min-width: 50px;
}
.tree-name {
  color: #1f2937;
}
.tree-ppid {
  color: #6b7280;
  font-size: 11px;
}
.tree-focus-btn {
  color: #1677ff;
  font-size: 12px;
  cursor: pointer;
  padding: 0 2px;
  opacity: 0;
  transition: opacity 0.15s;
}
.tree-node-row:hover .tree-focus-btn {
  opacity: 0.7;
}
.tree-focus-btn:hover {
  opacity: 1 !important;
  color: #0958d9;
}
.tree-usage {
  margin-left: auto;
  color: #4b5563;
  font-size: 11px;
}
.ssl-dot {
  color: #10b981;
  font-size: 8px;
  margin-right: 4px;
  cursor: help;
}

/* Dead / exited process */
.tree-node-row.dead {
  opacity: 0.55;
}
.tree-node-row.dead:hover {
  background: #f5f5f5;
}
.tree-node-row.dead .tree-pid {
  color: #6b7280;
}
.tree-node-row.dead .tree-name {
  color: #6b7280;
  font-style: italic;
}
.tree-node-row.dead .tree-ppid {
  color: #9ca3af;
}
.tree-dead-tag {
  font-size: 9px;
  color: #4b5563;
  background: #e5e7eb;
  border: 1px solid #e0e0e0;
  border-radius: 3px;
  padding: 0 4px;
  font-weight: 500;
  font-family: ui-monospace, monospace;
}
</style>
