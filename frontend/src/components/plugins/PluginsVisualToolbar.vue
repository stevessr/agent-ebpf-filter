<script setup lang="ts">
import { computed } from "vue";
import PluginsVisualNodeTypeLibrary from "./PluginsVisualNodeTypeLibrary.vue";
import PluginsVisualRecipePanel from "./PluginsVisualRecipePanel.vue";
import { useFloatingPanel } from "./useFloatingPanel";
import type {
  VisualAction,
  VisualConditionField,
  VisualMapMode,
  VisualRecipe,
  VisualTrigger,
  VisualValidationIssue,
} from "./types";

const props = defineProps<{
  designerSubtab: string;
  recipes: VisualRecipe[];
  trigger: VisualTrigger;
  action: VisualAction;
  mapMode: VisualMapMode;
  countConditions: number;
  treeDepth: number;
  pluginId: string;
  codeLines: number;
  validationIssues: VisualValidationIssue[];
  compileReady: boolean;
  autosaveLabel: string;
  undoCount: number;
  redoCount: number;
}>();

const emit = defineEmits<{
  "select-trigger": [value: VisualTrigger];
  "add-condition": [value: VisualConditionField];
  "add-group": [value: "AND" | "OR"];
  "set-map": [value: VisualMapMode];
  "set-action": [value: VisualAction];
  "focus-node": [node: string];
  "apply-recipe": [recipeId: string];
  "reset-workspace": [];
  "export-workspace": [];
  "import-workspace": [];
  "save-draft": [];
  "clear-draft": [];
  "undo-workspace": [];
  "redo-workspace": [];
}>();

const nodeLibrary = useFloatingPanel({
  initialDock: "left",
  defaultWidth: 320,
  defaultHeight: 560,
});

const recipePanel = useFloatingPanel({
  initialDock: "right",
  defaultWidth: 360,
  defaultHeight: 520,
});
</script>

<template>
  <!-- Node Library Floating Window -->
  <transition name="node-library-float">
    <div
      v-if="nodeLibrary.visible.value && designerSubtab === 'dify'"
      class="node-library-floating-window"
      :class="[
        `dock-${nodeLibrary.dock.value}`,
        { dragging: !!nodeLibrary.dragging.value },
      ]"
      :style="nodeLibrary.panelStyle.value"
    >
      <div class="node-library-floating-toolbar">
        <button
          type="button"
          class="node-library-direction-button"
          :title="
            nodeLibrary.dock.value === 'left'
              ? '贴左边缘隐藏节点类型'
              : '贴右边缘隐藏节点类型'
          "
          @pointerdown.stop
          @click="nodeLibrary.hide"
        >
          {{ nodeLibrary.hideArrow.value }}
        </button>
        <div
          class="node-library-floating-drag-handle"
          title="拖拽移动；靠近左右边缘自动吸附"
          @pointerdown.prevent="nodeLibrary.startDragging"
        >
          <a-tag color="blue">节点类型</a-tag>
          <span>拖拽移动，靠近边缘自动吸附</span>
        </div>
        <a-button
          size="small"
          class="node-library-dock-toggle"
          @pointerdown.stop
          @click="nodeLibrary.toggleDock"
        >
          {{ nodeLibrary.dockLabel.value }}
        </a-button>
      </div>
      <PluginsVisualNodeTypeLibrary
        @select-trigger="emit('select-trigger', $event)"
        @add-condition="emit('add-condition', $event)"
        @add-group="emit('add-group', $event)"
        @set-map="emit('set-map', $event)"
        @set-action="emit('set-action', $event)"
        @focus-node="emit('focus-node', $event)"
      />
    </div>
  </transition>

  <!-- Node Library Restore Trigger -->
  <transition name="node-library-trigger">
    <button
      v-if="!nodeLibrary.visible.value && designerSubtab === 'dify'"
      type="button"
      class="node-library-floating-trigger"
      :class="`dock-${nodeLibrary.dock.value}`"
      :style="nodeLibrary.triggerStyle.value"
      :title="
        nodeLibrary.dock.value === 'left'
          ? '从左边缘展开节点类型'
          : '从右边缘展开节点类型'
      "
      @click="nodeLibrary.show"
    >
      <span>{{ nodeLibrary.restoreArrow.value }}</span>
      节点类型
    </button>
  </transition>

  <!-- Recipe Floating Window -->
  <transition name="recipe-float">
    <div
      v-if="recipePanel.visible.value"
      class="recipe-floating-window"
      :class="[
        `dock-${recipePanel.dock.value}`,
        { dragging: !!recipePanel.dragging.value },
      ]"
      :style="recipePanel.panelStyle.value"
    >
      <div class="recipe-floating-toolbar">
        <button
          type="button"
          class="recipe-direction-button"
          :title="
            recipePanel.dock.value === 'left' ? '贴左边缘隐藏' : '贴右边缘隐藏'
          "
          @pointerdown.stop
          @click="recipePanel.hide"
        >
          {{ recipePanel.hideArrow.value }}
        </button>
        <div
          class="recipe-floating-drag-handle"
          title="拖拽移动；靠近左右边缘自动吸附"
          @pointerdown.prevent="recipePanel.startDragging"
        >
          <a-tag color="green">场景积木</a-tag>
          <span>拖拽移动，靠近边缘自动吸附</span>
        </div>
        <a-button
          size="small"
          class="recipe-dock-toggle"
          @pointerdown.stop
          @click="recipePanel.toggleDock"
        >
          {{ recipePanel.dockLabel.value }}
        </a-button>
      </div>
      <PluginsVisualRecipePanel
        :recipes="recipes"
        :trigger="trigger"
        :action="action"
        :map-mode="mapMode"
        :condition-count="countConditions"
        :tree-depth="treeDepth"
        :plugin-id="pluginId"
        :code-lines="codeLines"
        :validation-issues="validationIssues"
        :compile-ready="compileReady"
        :autosave-label="autosaveLabel"
        :undo-count="undoCount"
        :redo-count="redoCount"
        @apply-recipe="emit('apply-recipe', $event)"
        @reset-workspace="emit('reset-workspace')"
        @export-workspace="emit('export-workspace')"
        @import-workspace="emit('import-workspace')"
        @save-draft="emit('save-draft')"
        @clear-draft="emit('clear-draft')"
        @undo-workspace="emit('undo-workspace')"
        @redo-workspace="emit('redo-workspace')"
      />
    </div>
  </transition>

  <!-- Recipe Restore Trigger -->
  <transition name="recipe-trigger">
    <button
      v-if="!recipePanel.visible.value"
      type="button"
      class="recipe-floating-trigger"
      :class="`dock-${recipePanel.dock.value}`"
      :style="recipePanel.triggerStyle.value"
      :title="
        recipePanel.dock.value === 'left'
          ? '从左边缘展开场景积木'
          : '从右边缘展开场景积木'
      "
      @click="recipePanel.show"
    >
      <span>{{ recipePanel.restoreArrow.value }}</span>
      场景积木
    </button>
  </transition>
</template>

<style scoped>
/* Node Library Floating Window */
.node-library-floating-window {
  position: fixed;
  z-index: 34;
  display: flex;
  flex-direction: column;
  width: min(320px, calc(100vw - 32px));
  max-height: calc(100vh - 128px);
  min-height: 420px;
  overflow: hidden;
  padding: 10px;
  border: 1px solid var(--workflow-border);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: var(--workflow-shadow);
  backdrop-filter: blur(14px);
  --node-library-panel-exit-x: calc(-100% - 24px);
  --node-library-edge-button-hover: -4px;
  transition: left 0.18s ease, top 0.18s ease, opacity 0.2s ease,
    transform 0.2s ease, box-shadow 0.18s ease;
}

.node-library-floating-window.dragging {
  cursor: grabbing;
  transition: none;
  box-shadow: 0 24px 68px rgba(22, 119, 255, 0.2), 0 0 0 1px var(--workflow-primary-border);
}

.node-library-floating-window.dock-right {
  --node-library-panel-exit-x: calc(100% + 24px);
  --node-library-edge-button-hover: 4px;
}

.node-library-floating-toolbar {
  position: sticky;
  top: -10px;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex: 0 0 auto;
  margin: -10px -10px 10px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--workflow-border);
  background: rgba(248, 251, 255, 0.96);
}

.node-library-floating-window.dock-left .node-library-direction-button {
  order: 0;
}

.node-library-floating-window.dock-left .node-library-floating-drag-handle {
  order: 1;
}

.node-library-floating-window.dock-left .node-library-dock-toggle {
  order: 2;
}

.node-library-floating-window.dock-right .node-library-dock-toggle {
  order: 0;
}

.node-library-floating-window.dock-right .node-library-floating-drag-handle {
  order: 1;
}

.node-library-floating-window.dock-right .node-library-direction-button {
  order: 2;
}

.node-library-floating-drag-handle {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1 1 auto;
  min-width: 0;
  color: var(--workflow-text-muted);
  font-size: 11px;
  cursor: grab;
  user-select: none;
  touch-action: none;
}

.node-library-floating-drag-handle span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-library-floating-window.dragging .node-library-floating-drag-handle {
  cursor: grabbing;
}

.node-library-dock-toggle {
  flex: 0 0 auto;
}

.node-library-direction-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 24px;
  border: 1px solid var(--workflow-primary-border);
  border-radius: 999px;
  color: var(--workflow-primary);
  background: var(--workflow-primary-soft);
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  transition: transform 0.16s ease, border-color 0.16s ease,
    background 0.16s ease;
}

.node-library-direction-button:hover {
  transform: translateX(var(--node-library-edge-button-hover));
  border-color: var(--workflow-primary);
  background: #d6eaff;
}

.node-library-floating-window :deep(.dify-node-library) {
  flex: 1 1 auto;
  min-height: 0;
}

.node-library-floating-trigger {
  position: fixed;
  z-index: 35;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border: 1px solid var(--workflow-primary-border);
  color: var(--workflow-primary);
  background: rgba(255, 255, 255, 0.96);
  box-shadow: var(--workflow-shadow);
  cursor: pointer;
  font-size: 12px;
  transition: transform 0.18s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.18s ease,
    border-color 0.16s ease;
}

.node-library-floating-trigger:hover {
  transform: translateX(var(--node-library-trigger-hover, 4px));
  border-color: var(--workflow-primary);
}

.node-library-floating-trigger span {
  font-size: 18px;
  line-height: 0.8;
}

.node-library-floating-trigger.dock-left {
  left: 0;
  border-left: 0;
  border-radius: 0 999px 999px 0;
  --node-library-trigger-enter-x: -100%;
  --node-library-trigger-hover: 4px;
}

.node-library-floating-trigger.dock-right {
  right: 0;
  border-right: 0;
  border-radius: 999px 0 0 999px;
  --node-library-trigger-enter-x: 100%;
  --node-library-trigger-hover: -4px;
}

.node-library-float-enter-active,
.node-library-float-leave-active,
.node-library-trigger-enter-active,
.node-library-trigger-leave-active {
  transition: opacity 0.22s ease, transform 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}

.node-library-float-enter-from,
.node-library-float-leave-to {
  opacity: 0;
  transform: translateX(var(--node-library-panel-exit-x)) scale(0.98);
}

.node-library-trigger-enter-from,
.node-library-trigger-leave-to {
  opacity: 0;
  transform: translateX(var(--node-library-trigger-enter-x)) scale(0.98);
}

/* Recipe Floating Window */
.recipe-floating-window {
  position: fixed;
  z-index: 30;
  width: min(360px, calc(100vw - 32px));
  max-height: calc(100vh - 128px);
  overflow: auto;
  padding: 10px;
  border: 1px solid var(--workflow-border);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: var(--workflow-shadow);
  backdrop-filter: blur(14px);
  --recipe-panel-exit-x: calc(-100% - 24px);
  --recipe-edge-button-hover: -4px;
  transition: left 0.18s ease, top 0.18s ease, opacity 0.2s ease,
    transform 0.2s ease, box-shadow 0.18s ease;
}

.recipe-floating-window.dragging {
  cursor: grabbing;
  transition: none;
  box-shadow: 0 24px 68px rgba(22, 119, 255, 0.2), 0 0 0 1px var(--workflow-primary-border);
}

.recipe-floating-window.dock-right {
  --recipe-panel-exit-x: calc(100% + 24px);
  --recipe-edge-button-hover: 4px;
}

.recipe-floating-toolbar {
  position: sticky;
  top: -10px;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin: -10px -10px 10px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--workflow-border);
  background: rgba(248, 251, 255, 0.96);
}

.recipe-floating-window.dock-left .recipe-direction-button {
  order: 0;
}

.recipe-floating-window.dock-left .recipe-floating-drag-handle {
  order: 1;
}

.recipe-floating-window.dock-left .recipe-dock-toggle {
  order: 2;
}

.recipe-floating-window.dock-right .recipe-dock-toggle {
  order: 0;
}

.recipe-floating-window.dock-right .recipe-floating-drag-handle {
  order: 1;
}

.recipe-floating-window.dock-right .recipe-direction-button {
  order: 2;
}

.recipe-floating-drag-handle {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1 1 auto;
  min-width: 0;
  color: var(--workflow-text-muted);
  font-size: 11px;
  cursor: grab;
  user-select: none;
  touch-action: none;
}

.recipe-floating-drag-handle span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recipe-floating-window.dragging .recipe-floating-drag-handle {
  cursor: grabbing;
}

.recipe-dock-toggle {
  flex: 0 0 auto;
}

.recipe-direction-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 24px;
  border: 1px solid var(--workflow-primary-border);
  border-radius: 999px;
  color: var(--workflow-primary);
  background: var(--workflow-primary-soft);
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  transition: transform 0.16s ease, border-color 0.16s ease,
    background 0.16s ease;
}

.recipe-direction-button:hover {
  transform: translateX(var(--recipe-edge-button-hover));
  border-color: var(--workflow-primary);
  background: #d6eaff;
}

.recipe-floating-trigger {
  position: fixed;
  z-index: 31;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border: 1px solid var(--workflow-primary-border);
  border-radius: 999px;
  color: var(--workflow-primary);
  background: rgba(255, 255, 255, 0.96);
  box-shadow: var(--workflow-shadow);
  cursor: pointer;
  font-size: 12px;
  transition: transform 0.18s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.18s ease,
    border-color 0.16s ease;
}

.recipe-floating-trigger:hover {
  transform: translateX(var(--recipe-trigger-hover, 4px));
  border-color: var(--workflow-primary);
}

.recipe-floating-trigger span {
  font-size: 18px;
  line-height: 0.8;
}

.recipe-floating-trigger.dock-left {
  left: 0;
  border-left: 0;
  border-radius: 0 999px 999px 0;
  --recipe-trigger-enter-x: -100%;
  --recipe-trigger-hover: 4px;
}

.recipe-floating-trigger.dock-right {
  right: 0;
  border-right: 0;
  border-radius: 999px 0 0 999px;
  --recipe-trigger-enter-x: 100%;
  --recipe-trigger-hover: -4px;
}

.recipe-float-enter-active,
.recipe-float-leave-active,
.recipe-trigger-enter-active,
.recipe-trigger-leave-active {
  transition: opacity 0.22s ease, transform 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}

.recipe-float-enter-from,
.recipe-float-leave-to {
  opacity: 0;
  transform: translateX(var(--recipe-panel-exit-x)) scale(0.98);
}

.recipe-trigger-enter-from,
.recipe-trigger-leave-to {
  opacity: 0;
  transform: translateX(var(--recipe-trigger-enter-x)) scale(0.98);
}
</style>
