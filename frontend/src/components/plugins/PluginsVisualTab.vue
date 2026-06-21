<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, h } from "vue";
import { message, Modal } from "ant-design-vue";
import { usePlugins } from "../../composables/plugins/usePlugins";

import PluginsVisualAiPanel from "./PluginsVisualAiPanel.vue";
import PluginsVisualFlowCanvas from "./PluginsVisualFlowCanvas.vue";
import PluginsVisualNodeInspector from "./PluginsVisualNodeInspector.vue";
import PluginsVisualToolbar from "./PluginsVisualToolbar.vue";
import PluginsVisualDesigner from "./PluginsVisualDesigner.vue";
import PluginsVisualPreview from "./PluginsVisualPreview.vue";

import type {
  VisualAction,
  VisualConditionField,
  VisualFlowNodeId,
  VisualLogicGroup,
  VisualMapMode,
  VisualMapKey,
  VisualTrigger,
} from "./types";

import { generateBpfCode } from "./transpiler";
import { VISUAL_PROGRAM_NAME } from "./trigger-runtime";
import { validateWorkspace, isVisualConditionField } from "./validation";
import { useVisualWorkspace } from "./useVisualWorkspace";
import {
  visualWireIds,
  visualWireLabels,
  visualWireEndpoints,
  visualFlowNodeIds,
  flowNodeDetails,
  createDefaultNodeLayout,
  createDefaultWireStates,
  createDefaultHiddenNodes,
  mergeWireStates,
  useFlowNodeManager,
} from "./useFlowNodeManager";
import { usePluginCompiler } from "./usePluginCompiler";
import { visualRecipes, useRecipeOperations } from "./visualRecipes";
import { triggerOptions } from "./constants";

const { fetchPlugins } = usePlugins();

// --- Workspace state ---
const {
  trigger,
  logicRoot,
  action,
  mapMode,
  mapKey,
  mapLimit,
  aiPrompt,
  pluginId,
  pluginName,
  description,
  isCompiled,
  autosaveLabel,
  undoStack,
  redoStack,
  nodeLayout,
  wireStates,
  hiddenFlowNodes,
  activeFlowNode,
  designerSubtab,
  countConditions,
  treeDepth,
  onDeleteNode,
  onAddRule,
  onAddGroup,
  onUpdateRule,
  onUpdateGroupType,
  createWorkspaceSnapshot,
  applyWorkspaceSnapshot,
  saveWorkspaceDraft,
  clearWorkspaceDraft,
  restoreWorkspaceDraft,
  syncHistoryBaseline,
  undoWorkspace,
  redoWorkspace,
} = useVisualWorkspace();

// --- Flow node manager ---
const {
  selectedFlowNodeDetail,
  resetNodeLayout,
  resetWireStates,
  restoreFlowNode,
  handleDeleteFlowNode,
  selectFlowNode,
  focusFlowNode,
  flowSectionClass,
  moveFlowNodeTo,
} = useFlowNodeManager({
  wireStates,
  hiddenFlowNodes,
  activeFlowNode,
  designerSubtab,
  nodeLayout,
  trigger,
  action,
});

// --- Compiler ---
const validationIssues = computed(() => {
  const snapshot = createWorkspaceSnapshot();
  return validateWorkspace(
    snapshot,
    flowNodeDetails,
    visualWireLabels,
    visualWireEndpoints,
    visualFlowNodeIds,
    visualWireIds,
  );
});

const validationErrors = computed(() =>
  validationIssues.value.filter((issue) => issue.severity === "error"),
);

const isWorkspaceValid = computed(() => validationErrors.value.length === 0);

const generatedBpfCode = computed(() => {
  const snapshot = createWorkspaceSnapshot();
  return generateBpfCode(snapshot, VISUAL_PROGRAM_NAME);
});

const generatedLineCount = computed(
  () => generatedBpfCode.value.split(/\r?\n/).length,
);

const {
  compiling,
  loadingAction,
  compileLogLocal,
  handleCompileAndRegister,
  handleLoad,
} = usePluginCompiler({
  pluginId: () => pluginId.value,
  pluginName: () => pluginName.value,
  description: () => description.value,
  trigger: () => trigger.value,
  generatedBpfCode: () => generatedBpfCode.value,
  isWorkspaceValid: () => isWorkspaceValid.value,
  validationErrors: () => validationErrors.value,
  isCompiled,
});

// --- Recipe operations ---
const { applyRecipe, resetWorkspace, exportWorkspace, importWorkspace } =
  useRecipeOperations({
    createWorkspaceSnapshot,
    applyWorkspaceSnapshot,
  });

// --- Node type library handlers ---
const selectTriggerNodeType = (value: VisualTrigger) => {
  trigger.value = value;
  void focusFlowNode("trigger");
  message.success(`已从节点类型库选择入口：${value}`);
};

const addConditionNodeType = (value: VisualConditionField) => {
  onAddRule("root", value);
  void focusFlowNode("condition");
  message.success(`已从节点类型库添加条件：${value}`);
};

const addLogicNodeType = (value: "AND" | "OR") => {
  onAddGroup("root", value);
  void focusFlowNode("condition");
  message.success(`已从节点类型库添加逻辑组：${value}`);
};

const handleAddGroup = (groupIdOrType: string, type?: "AND" | "OR") => {
  if (type) {
    onAddGroup(groupIdOrType, type);
  } else if (groupIdOrType === "AND" || groupIdOrType === "OR") {
    onAddGroup("root", groupIdOrType);
  }
};

const setMapNodeType = (value: VisualMapMode) => {
  mapMode.value = value;
  void focusFlowNode("map");
  message.success(`已从节点类型库设置状态节点：${value}`);
};

const setActionNodeType = (value: VisualAction) => {
  if (trigger.value === "unlink" && value === "BLOCK") {
    message.error(
      "unlink (Kprobe) 挂载点不支持 BLOCK 动作，请选择 ALERT 或 KILL",
    );
    return;
  }
  action.value = value;
  void focusFlowNode("action");
  message.success(`已从节点类型库设置动作：${value}`);
};

// --- AI translate handler ---
const handleAiTranslate = (payload: {
  trigger: VisualTrigger;
  action: VisualAction;
  conditions: VisualLogicGroup;
  mapMode: VisualMapMode;
  mapKey: VisualMapKey;
  mapLimit: number;
}) => {
  trigger.value = payload.trigger;
  action.value = payload.action;
  logicRoot.value = payload.conditions;
  mapMode.value = payload.mapMode;
  mapKey.value = payload.mapKey;
  mapLimit.value = payload.mapLimit;
  hiddenFlowNodes.value = createDefaultHiddenNodes();
  wireStates.value = createDefaultWireStates();
  designerSubtab.value = "dify";
  activeFlowNode.value = "condition";
};

// --- Canvas drop handler ---
interface CanvasNodeTypeDropPayload {
  category: string;
  value: string;
  x: number;
  y: number;
}

const handleCanvasNodeTypeDrop = (payload: CanvasNodeTypeDropPayload) => {
  applyNodeTypeDrop(payload.category, payload.value, {
    x: payload.x,
    y: payload.y,
  });
};

const applyNodeTypeDrop = (
  category: string,
  value: string,
  position?: { x: number; y: number },
) => {
  let targetNode: VisualFlowNodeId | null = null;
  let statusText = "";

  const visualMapModeSet = new Set<VisualMapMode>([
    "NONE",
    "COUNTER",
    "BLOCKLIST",
  ]);
  const visualActionSet = new Set<VisualAction>(["BLOCK", "ALERT", "KILL"]);

  if (category === "trigger") {
    if (!triggerOptions.some((item) => item.value === value)) return;
    targetNode = "trigger";
    restoreFlowNode(targetNode);
    trigger.value = value as VisualTrigger;
    statusText = `已切换事件挂载点为：${value}`;
  } else if (category === "condition") {
    if (!isVisualConditionField(value)) return;
    targetNode = "condition";
    restoreFlowNode(targetNode);
    onAddRule("root", value);
    statusText = `已拖动添加匹配过滤：${value}`;
  } else if (category === "logic_group") {
    if (value !== "AND" && value !== "OR") return;
    targetNode = "condition";
    restoreFlowNode(targetNode);
    onAddGroup("root", value);
    statusText = `已拖动添加逻辑运算组：${value}`;
  } else if (category === "map") {
    if (!visualMapModeSet.has(value as VisualMapMode)) return;
    targetNode = "map";
    restoreFlowNode(targetNode);
    mapMode.value = value as VisualMapMode;
    statusText = `已配置 Map 状态存储为：${value}`;
  } else if (category === "action") {
    if (!visualActionSet.has(value as VisualAction)) return;
    if (trigger.value === "unlink" && value === "BLOCK") {
      message.error(
        "unlink (Kprobe) 挂载点不支持 BLOCK 动作，请选择 ALERT 或 KILL",
      );
      return;
    }
    targetNode = "action";
    restoreFlowNode(targetNode);
    action.value = value as VisualAction;
    statusText = `已更新拦截响应动作为：${value}`;
  } else if (category === "focus") {
    if (!visualFlowNodeIds.includes(value as VisualFlowNodeId)) return;
    targetNode = value as VisualFlowNodeId;
    restoreFlowNode(targetNode);
    statusText = `已拖入并聚焦节点：${flowNodeDetails[targetNode].label}`;
  }

  if (!targetNode) return;
  if (position) {
    moveFlowNodeTo(targetNode, position.x, position.y);
  }
  activeFlowNode.value = targetNode;
  designerSubtab.value = "dify";
  message.success(position ? `${statusText}，已吸附到画布网格` : statusText);
};

const handleFocusNode = (node: string) => {
  if (!visualFlowNodeIds.includes(node as VisualFlowNodeId)) return;
  void focusFlowNode(node as VisualFlowNodeId);
};

const flowSectionClassForAnyNode = (node: string) => {
  if (!visualFlowNodeIds.includes(node as VisualFlowNodeId)) return {};
  return flowSectionClass(node as VisualFlowNodeId);
};

const handleWorkspaceDrop = (event: DragEvent) => {
  event.preventDefault();
  if (!event.dataTransfer) return;
  try {
    const rawData = event.dataTransfer.getData("text/plain");
    if (!rawData) return;
    const { category, value } = JSON.parse(rawData) as {
      category: string;
      value: string;
    };
    applyNodeTypeDrop(category, value);
  } catch (e) {
    console.error("Drop parsing failed:", e);
  }
};

// --- Keyboard shortcuts ---
const isTextEditingTarget = (target: EventTarget | null) => {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  return (
    tag === "input" ||
    tag === "textarea" ||
    target.isContentEditable ||
    !!target.closest(".ant-select")
  );
};

const handleHistoryShortcut = (event: KeyboardEvent) => {
  const key = event.key.toLowerCase();
  const isModifier = event.ctrlKey || event.metaKey;
  if (!isModifier || isTextEditingTarget(event.target)) return;
  if (key === "z" && !event.shiftKey) {
    event.preventDefault();
    void undoWorkspace();
  } else if ((key === "z" && event.shiftKey) || key === "y") {
    event.preventDefault();
    void redoWorkspace();
  }
};

// --- Lifecycle ---
onMounted(async () => {
  restoreWorkspaceDraft();
  syncHistoryBaseline();
  window.addEventListener("keydown", handleHistoryShortcut);
  await fetchPlugins();
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleHistoryShortcut);
});
</script>

<template>
  <div class="plugins-visual-tab">
    <a-row :gutter="16">
      <a-col :span="24">
        <div
          class="graphical-workspace"
          @dragover.prevent
          @drop="handleWorkspaceDrop"
        >
          <div class="workspace-title">
            <h3>流程图高级规则拼接控制台 (Advanced Flow Designer)</h3>
            <span class="sub"
              >通过拼接多重高级匹配字段与触发点，在系统内核深层执行精密入侵侦测。</span
            >
          </div>

          <a-tabs
            v-model:active-key="designerSubtab"
            class="dify-workspace-tabs"
          >
            <!-- Tab: Workflow -->
            <a-tab-pane key="dify" tab="Workflow">
              <div class="dify-workflow-shell">
                <div class="dify-workflow-hero">
                  <div>
                    <a-tag color="blue">Style</a-tag>
                    <h4>节点工作流编排</h4>
                    <p>
                      主视图只保留悬浮节点类型库、拖线画布和节点
                      Inspector；可从悬浮窗拖拽节点类型到画布，自动吸附到网格，并通过端口拖线/线缆开关编辑路由。
                    </p>
                  </div>
                  <a-space size="small" wrap>
                    <a-tag :color="isWorkspaceValid ? 'green' : 'red'">
                      {{ isWorkspaceValid ? "READY" : "FIX REQUIRED" }}
                    </a-tag>
                    <a-tag color="purple">{{ countConditions }} filters</a-tag>
                    <a-tag color="cyan">{{ generatedLineCount }} C lines</a-tag>
                  </a-space>
                </div>
                <PluginsVisualFlowCanvas
                  v-model:node-layout="nodeLayout"
                  v-model:wire-states="wireStates"
                  :selected-node-id="activeFlowNode"
                  :trigger="trigger"
                  :action="action"
                  :map-mode="mapMode"
                  :condition-count="countConditions"
                  :tree-depth="treeDepth"
                  :code-lines="generatedLineCount"
                  :compile-ready="isWorkspaceValid"
                  :hidden-nodes="hiddenFlowNodes"
                  @update:selected-node-id="selectFlowNode"
                  @reset-layout="resetNodeLayout"
                  @reset-wires="resetWireStates"
                  @delete-node="handleDeleteFlowNode"
                  @drop-node-type="handleCanvasNodeTypeDrop"
                />

                <div class="selected-flow-panel">
                  <a-tag color="blue" class="selected-flow-tag">
                    {{ selectedFlowNodeDetail.label }}
                  </a-tag>
                  <span>{{ selectedFlowNodeDetail.focus }}</span>
                  <a-space size="small" wrap>
                    <a-button size="small" @click="focusFlowNode('trigger')"
                      >Trigger</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('condition')"
                      >Condition</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('map')"
                      >Map</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('action')"
                      >Action</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('code')"
                      >Code</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('compile')"
                      >Compile</a-button
                    >
                  </a-space>
                </div>

                <PluginsVisualNodeInspector
                  :selected-node-id="activeFlowNode"
                  v-model:trigger="trigger"
                  v-model:action="action"
                  v-model:map-mode="mapMode"
                  v-model:map-key="mapKey"
                  v-model:map-limit="mapLimit"
                  v-model:plugin-id="pluginId"
                  v-model:plugin-name="pluginName"
                  v-model:description="description"
                  :condition-count="countConditions"
                  :tree-depth="treeDepth"
                  :code-lines="generatedLineCount"
                  :compile-ready="isWorkspaceValid"
                  :compiling="compiling"
                  :validation-issues="validationIssues"
                  @add-condition="onAddRule('root', $event)"
                  @add-group="handleAddGroup"
                  @compile="handleCompileAndRegister"
                />
              </div>
            </a-tab-pane>

            <!-- Tab: Blueprint Details -->
            <a-tab-pane key="map" tab="Map / Blueprint Details">
              <PluginsVisualDesigner
                :trigger="trigger"
                :action="action"
                :logic-root="logicRoot"
                :map-mode="mapMode"
                :map-key="mapKey"
                :map-limit="mapLimit"
                :plugin-id="pluginId"
                :plugin-name="pluginName"
                :description="description"
                :compiling="compiling"
                :is-workspace-valid="isWorkspaceValid"
                :validation-issues="validationIssues"
                :active-flow-node="activeFlowNode"
                :count-conditions="countConditions"
                :tree-depth="treeDepth"
                :on-delete-node="onDeleteNode"
                :on-add-rule="onAddRule"
                :on-add-group="onAddGroup"
                :on-update-rule="onUpdateRule"
                :on-update-group-type="onUpdateGroupType"
                :flow-section-class="flowSectionClassForAnyNode"
                @update:trigger="trigger = $event"
                @update:action="action = $event"
                @update:map-mode="mapMode = $event"
                @update:map-key="mapKey = $event"
                @update:map-limit="mapLimit = $event"
                @update:plugin-id="pluginId = $event"
                @update:plugin-name="pluginName = $event"
                @update:description="description = $event"
                @add-condition="onAddRule('root', $event)"
                @add-group="handleAddGroup"
                @compile="handleCompileAndRegister"
              />
            </a-tab-pane>

            <!-- Tab: NLP -->
            <a-tab-pane key="nlp" tab="NLP Blocks Compiler">
              <div class="nlp-workspace-shell">
                <div class="nlp-workspace-notice">
                  <a-tag color="purple">LLM Blocks Compiler</a-tag>
                  <span
                    >用后端 OpenAI 兼容 LLM 将自然语言内核防御意图编译成 Trigger
                    / Condition / Map / Action 积木流；生成后会自动回到 Dify
                    Workflow，继续拖线、删节点、调 Inspector。</span
                  >
                </div>
                <PluginsVisualAiPanel
                  v-model="aiPrompt"
                  @translate="handleAiTranslate"
                />
              </div>
            </a-tab-pane>

            <!-- Tab: Generated C -->
            <a-tab-pane key="source" tab="Generated eBPF C">
              <PluginsVisualPreview
                :code="generatedBpfCode"
                :compiling="compiling"
                :compiled="isCompiled"
                :loading="loadingAction"
                :log="compileLogLocal"
                :active-flow-node="activeFlowNode"
                :flow-section-class="flowSectionClassForAnyNode"
                @load="handleLoad"
              />
            </a-tab-pane>
          </a-tabs>
        </div>
      </a-col>
    </a-row>

    <!-- Floating toolbars -->
    <PluginsVisualToolbar
      :designer-subtab="designerSubtab"
      :recipes="visualRecipes"
      :trigger="trigger"
      :action="action"
      :map-mode="mapMode"
      :count-conditions="countConditions"
      :tree-depth="treeDepth"
      :plugin-id="pluginId"
      :code-lines="generatedLineCount"
      :validation-issues="validationIssues"
      :compile-ready="isWorkspaceValid"
      :autosave-label="autosaveLabel"
      :undo-count="undoStack.length"
      :redo-count="redoStack.length"
      @select-trigger="selectTriggerNodeType"
      @add-condition="addConditionNodeType"
      @add-group="addLogicNodeType"
      @set-map="setMapNodeType"
      @set-action="setActionNodeType"
      @focus-node="handleFocusNode"
      @apply-recipe="applyRecipe"
      @reset-workspace="resetWorkspace"
      @export-workspace="exportWorkspace"
      @import-workspace="importWorkspace"
      @save-draft="() => saveWorkspaceDraft(false)"
      @clear-draft="clearWorkspaceDraft"
      @undo-workspace="undoWorkspace"
      @redo-workspace="redoWorkspace"
    />
  </div>
</template>

<style scoped>
.plugins-visual-tab {
  min-height: 600px;
  --workflow-primary: #1677ff;
  --workflow-primary-hover: #4096ff;
  --workflow-primary-soft: #e6f4ff;
  --workflow-primary-subtle: #f0f7ff;
  --workflow-primary-border: #91caff;
  --workflow-text: #0f172a;
  --workflow-text-secondary: #475569;
  --workflow-text-muted: #64748b;
  --workflow-border: #d6e4ff;
  --workflow-surface: #ffffff;
  --workflow-surface-soft: #f8fbff;
  --workflow-shadow: 0 14px 34px rgba(22, 119, 255, 0.12);
}

.dify-workspace-tabs {
  margin-top: 8px;
}

.dify-workflow-shell,
.nlp-workspace-shell {
  padding-top: 8px;
}

.dify-workflow-hero,
.nlp-workspace-notice {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid var(--workflow-border);
  background: linear-gradient(
    135deg,
    #ffffff 0%,
    var(--workflow-primary-subtle) 100%
  );
  color: var(--workflow-text-secondary);
  box-shadow: 0 8px 24px rgba(22, 119, 255, 0.08);
}

.dify-workflow-hero h4 {
  margin: 8px 0 4px;
  color: var(--workflow-text);
  font-size: 15px;
}

.dify-workflow-hero p,
.nlp-workspace-notice span {
  margin: 0;
  color: var(--workflow-text-muted);
  font-size: 12px;
  line-height: 1.45;
}

.nlp-workspace-notice {
  align-items: center;
  justify-content: flex-start;
  border-color: var(--workflow-border);
}

:deep(.dify-workspace-tabs .ant-tabs-nav) {
  margin-bottom: 12px;
}

:deep(.dify-workspace-tabs .ant-tabs-tab) {
  padding: 8px 14px;
  border-radius: 999px;
  color: var(--workflow-text-muted);
  background: var(--workflow-surface-soft);
  border: 1px solid transparent;
}

:deep(.dify-workspace-tabs .ant-tabs-tab-active) {
  background: var(--workflow-primary-soft);
  border-color: var(--workflow-primary-border);
}

:deep(.dify-workspace-tabs .ant-tabs-tab-active .ant-tabs-tab-btn) {
  color: var(--workflow-primary);
}

:deep(.dify-workspace-tabs .ant-tabs-ink-bar) {
  background: var(--workflow-primary);
}

.selected-flow-panel {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 16px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--workflow-border);
  background: var(--workflow-primary-subtle);
  color: var(--workflow-text-secondary);
  font-size: 12px;
}

.selected-flow-tag {
  margin: 0;
}

.graphical-workspace {
  background-color: #ffffff;
  background-image:
    linear-gradient(to right, rgba(22, 119, 255, 0.06) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(22, 119, 255, 0.06) 1px, transparent 1px),
    linear-gradient(to right, rgba(22, 119, 255, 0.035) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(22, 119, 255, 0.035) 1px, transparent 1px);
  background-size:
    40px 40px,
    40px 40px,
    10px 10px,
    10px 10px;
  border: 1px solid var(--workflow-border);
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 12px 32px rgba(22, 119, 255, 0.08);
  position: relative;
}

.workspace-title {
  margin-bottom: 20px;
  border-left: 4px solid var(--workflow-primary);
  padding-left: 10px;
}

.workspace-title h3 {
  margin: 0;
  font-weight: 600;
  color: var(--workflow-text);
}

.workspace-title .sub {
  font-size: 12px;
  color: var(--workflow-text-muted);
}

.flow-section-active {
  outline: 2px solid rgba(22, 119, 255, 0.62);
  outline-offset: 4px;
  box-shadow:
    0 0 0 1px rgba(22, 119, 255, 0.2),
    0 0 24px rgba(22, 119, 255, 0.16);
  border-radius: 10px;
  transition:
    outline-color 0.2s ease,
    box-shadow 0.2s ease;
}
</style>
