<script setup lang="ts">
import { computed, onMounted, watch } from "vue";
import { ReloadOutlined } from "@ant-design/icons-vue";
import type {
  VisualAction,
  VisualFlowNodeId,
  VisualHiddenNodeStates,
  VisualMapMode,
  VisualNodeLayout,
  VisualTrigger,
  VisualWireId,
  VisualWireStates,
} from "./types";
import {
  useCanvasLayout,
  MIN_CANVAS_WIDTH,
} from "../../composables/plugins/useCanvasLayout";
import { useCanvasInteraction } from "../../composables/plugins/useCanvasInteraction";

const props = defineProps<{
  nodeLayout: VisualNodeLayout;
  wireStates: VisualWireStates;
  selectedNodeId: VisualFlowNodeId;
  trigger: VisualTrigger;
  action: VisualAction;
  mapMode: VisualMapMode;
  conditionCount: number;
  treeDepth: number;
  codeLines: number;
  compileReady: boolean;
  hiddenNodes: VisualHiddenNodeStates;
}>();

const emit = defineEmits<{
  (e: "update:nodeLayout", value: VisualNodeLayout): void;
  (e: "update:wireStates", value: VisualWireStates): void;
  (e: "update:selectedNodeId", value: VisualFlowNodeId): void;
  (e: "reset-layout"): void;
  (e: "reset-wires"): void;
  (e: "delete-node", value: VisualFlowNodeId): void;
  (
    e: "drop-node-type",
    value: { category: string; value: string; x: number; y: number }
  ): void;
}>();

const layout = useCanvasLayout(() => ({
  nodeLayout: props.nodeLayout,
  wireStates: props.wireStates,
  trigger: props.trigger,
  action: props.action,
  mapMode: props.mapMode,
  conditionCount: props.conditionCount,
  treeDepth: props.treeDepth,
  codeLines: props.codeLines,
  compileReady: props.compileReady,
  hiddenNodes: props.hiddenNodes,
}));

const {
  theme,
  mergedLayout,
  mergedWireStates,
  canvasSize,
  snapNodeX,
  snapNodeY,
  boundedLayout,
  canvasViewBox,
  flowNodes,
  visibleFlowNodes,
  visibleWireDefinitions,
  wires,
  connectedWireCount,
  getNodePortPoint,
  getWireForEndpoints,
  hasOutgoingWire,
  hasIncomingWire,
  canDeleteNode,
} = layout;

const interaction = useCanvasInteraction(() => ({
  canvasSize: canvasSize.value,
  mergedWireStates: mergedWireStates.value,
  mergedLayout: mergedLayout.value,
  visibleFlowNodes: visibleFlowNodes.value,
  visibleWireDefinitions: visibleWireDefinitions.value,
  getNodePortPoint,
  getWireForEndpoints,
  hasOutgoingWire,
  hasIncomingWire,
  snapNodeX,
  snapNodeY,
  emit: emit as any,
}));

const {
  viewportSize,
  workspaceSize,
  connectionDrag,
  canvasResizeDrag,
  handlePointerDown,
  handleConnectionPointerDown,
  handleCanvasDragOver,
  handleCanvasDrop,
  handleCanvasResizePointerDown,
  connectionPreviewPath,
  expandWorkspace,
  resetWorkspaceSize,
  initResizeObserver,
} = interaction;

const selectedNode = computed(
  () =>
    visibleFlowNodes.value.find((node) => node.id === props.selectedNodeId) ??
    flowNodes.value.find((node) => node.id === props.selectedNodeId) ??
    flowNodes.value[0]
);

// Canvas size computation -- wired to workspace + viewport
const computedCanvasSize = () => ({
  width: Math.max(MIN_CANVAS_WIDTH, workspaceSize.value.width, Math.round(viewportSize.value.width)),
  height: workspaceSize.value.height,
});

canvasSize.value = computedCanvasSize();

watch(workspaceSize, () => { canvasSize.value = computedCanvasSize(); });
watch(viewportSize, () => { canvasSize.value = computedCanvasSize(); });

// applyWorkspaceSize already syncs canvasSize via workspaceSize watcher

// Wire toggle (enableWire handled by composable's stopConnectionDragging)
const toggleWire = (id: VisualWireId) => {
  emit("update:wireStates", {
    ...mergedWireStates.value,
    [id]: !mergedWireStates.value[id],
  });
};

const canvasStyle = () => ({
  width: `${canvasSize.value.width}px`,
  height: `${canvasSize.value.height}px`,
});

onMounted(() => {
  initResizeObserver();
});
</script>

<template>
  <div class="flow-shell">
    <div class="flow-header">
      <div>
        <h4>低代码节点拓扑画布 (Draggable Blueprint Canvas)</h4>
        <span>从左侧拖节点类型到画布会按 24px 网格吸附；拖节点端口可编辑路由，拖动底部手柄可拉开工作区。</span>
      </div>
      <a-space size="small" wrap>
        <a-tag color="blue">{{ canvasSize.width }}×{{ canvasSize.height }}</a-tag>
        <a-tag color="geekblue">24px snap</a-tag>
        <a-tag color="cyan">route edit</a-tag>
        <a-tag :color="connectedWireCount === visibleWireDefinitions.length ? 'green' : 'orange'">
          {{ connectedWireCount }}/{{ visibleWireDefinitions.length }} wires
        </a-tag>
        <a-button size="small" @click="expandWorkspace">展开画布</a-button>
        <a-button size="small" @click="resetWorkspaceSize">重置尺寸</a-button>
        <a-button size="small" @click="emit('reset-wires')">重连线缆</a-button>
        <a-button size="small" @click="emit('reset-layout')">
          <template #icon><ReloadOutlined /></template>
          自动布局
        </a-button>
      </a-space>
    </div>

    <div class="flow-inspector">
      <a-tag :color="selectedNode.color" class="inspector-tag">
        {{ selectedNode.badge }}
      </a-tag>
      <div>
        <strong>{{ selectedNode.title }}</strong>
        <span>{{ selectedNode.hint }}</span>
      </div>
    </div>

    <div ref="viewport" class="flow-canvas-viewport">
      <div
        ref="canvas"
        class="flow-canvas"
        :class="{ resizing: !!canvasResizeDrag }"
        :style="canvasStyle()"
        @dragover="handleCanvasDragOver"
        @drop="handleCanvasDrop"
      >
        <div class="drop-target-hint">
          拖入节点类型创建/恢复节点 · 节点移动自动吸附网格
        </div>
        <svg class="flow-wires" :viewBox="canvasViewBox" preserveAspectRatio="none">
        <defs>
          <linearGradient id="flow-wire-gradient" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" :stop-color="theme.primary" />
            <stop offset="48%" :stop-color="theme.primaryHover" />
            <stop offset="100%" :stop-color="theme.primary" />
          </linearGradient>
          <filter id="flow-glow">
            <feGaussianBlur stdDeviation="2" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <path
          v-for="wire in wires"
          :key="wire.id"
          :d="wire.d"
          :stroke="wire.enabled ? 'url(#flow-wire-gradient)' : 'rgba(148, 163, 184, 0.5)'"
          :stroke-width="wire.enabled ? 2 : 1.5"
          fill="none"
          :filter="wire.enabled ? 'url(#flow-glow)' : undefined"
          class="flow-wire"
          :class="{ disabled: !wire.enabled }"
        />
        <path
          v-if="connectionPreviewPath()"
          :d="connectionPreviewPath()"
          :stroke="connectionDrag?.valid ? theme.success : theme.warning"
          stroke-width="2.5"
          fill="none"
          class="flow-wire-preview"
          :class="{ invalid: connectionDrag && !connectionDrag.valid }"
        />
      </svg>

      <button
        v-for="wire in wires"
        :key="`${wire.id}-toggle`"
        type="button"
        class="wire-toggle"
        :class="{ disabled: !wire.enabled, required: wire.required }"
        :style="{ left: `${wire.labelX}px`, top: `${wire.labelY}px` }"
        :title="wire.enabled ? `断开 ${wire.id}` : `连接 ${wire.id}`"
        @pointerdown.stop
        @click.stop="toggleWire(wire.id)"
      >
        <span>{{ wire.label }}</span>
        <code>{{ wire.enabled ? "ON" : "OFF" }}</code>
      </button>

      <div v-if="connectionDrag" class="connection-hint">
        <strong>{{ connectionDrag.from }}</strong>
        <span v-if="connectionDrag.target && connectionDrag.valid">
          松开即可连接到 {{ connectionDrag.target }}
        </span>
        <span v-else-if="connectionDrag.target">
          {{ connectionDrag.from }} 不能直接连接 {{ connectionDrag.target }}
        </span>
        <span v-else>拖到另一个节点左侧输入端口</span>
      </div>

      <div
        v-for="node in visibleFlowNodes"
        :key="node.id"
        role="button"
        tabindex="0"
        class="flow-node"
        :class="{ selected: selectedNodeId === node.id }"
        :style="{
          left: `${boundedLayout[node.id].x}px`,
          top: `${boundedLayout[node.id].y}px`,
          borderColor: node.color,
          '--node-color': node.color,
        }"
        @pointerdown="handlePointerDown($event, node.id)"
      >
        <button
          v-if="canDeleteNode(node.id)"
          type="button"
          class="node-delete"
          title="删除节点"
          @pointerdown.stop
          @click.stop="emit('delete-node', node.id)"
        >
          ×
        </button>
        <span
          v-if="hasIncomingWire(node.id)"
          class="node-port port-in"
          :class="{
            candidate: connectionDrag?.target === node.id,
            valid: connectionDrag?.target === node.id && connectionDrag?.valid,
            invalid: connectionDrag?.target === node.id && !connectionDrag?.valid,
          }"
        ></span>
        <span
          v-if="hasOutgoingWire(node.id)"
          class="node-port port-out"
          title="拖拽到另一个节点输入端口以连接线缆"
          @pointerdown.stop.prevent="handleConnectionPointerDown($event, node.id)"
        ></span>
        <span class="node-badge">{{ node.badge }}</span>
        <strong>{{ node.title }}</strong>
        <code>{{ node.subtitle }}</code>
      </div>

      </div>
    </div>
    <div class="canvas-resize-footer">
      <span>画布超过可视区后可横向/纵向滚动，继续拖拽右侧手柄可拉得更开。</span>
      <button
        type="button"
        class="canvas-resize-handle"
        title="拖拽拉开画布"
        @pointerdown.stop.prevent="handleCanvasResizePointerDown"
      >
        <span>拖拽拉开</span>
      </button>
    </div>
  </div>
</template>

<style scoped src="./flow-canvas/flow-canvas.css"></style>
