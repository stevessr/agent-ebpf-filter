<script setup lang="ts">
import {
  ExportOutlined,
  ImportOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons-vue";
import type {
  VisualAction,
  VisualMapMode,
  VisualRecipe,
  VisualTrigger,
  VisualValidationIssue,
} from "./types";

defineProps<{
  recipes: VisualRecipe[];
  trigger: VisualTrigger;
  action: VisualAction;
  mapMode: VisualMapMode;
  conditionCount: number;
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
  (e: "apply-recipe", id: string): void;
  (e: "reset-workspace"): void;
  (e: "export-workspace"): void;
  (e: "import-workspace"): void;
  (e: "save-draft"): void;
  (e: "clear-draft"): void;
  (e: "undo-workspace"): void;
  (e: "redo-workspace"): void;
}>();

const issueColor = (severity: VisualValidationIssue["severity"]) => {
  if (severity === "error") return "red";
  if (severity === "warning") return "gold";
  return "blue";
};
</script>

<template>
  <div class="recipe-panel">
    <div class="panel-header">
      <ThunderboltOutlined class="panel-icon" />
      <h4>场景积木模板</h4>
    </div>
    <div class="panel-desc">
      先套用完整低代码方案，再拖拽 Palette 微调，避免从空白画布开始拼装。
    </div>

    <div class="workspace-meter">
      <div class="meter-row">
        <span>Hook</span>
        <code>{{ trigger }}</code>
      </div>
      <div class="meter-row">
        <span>Action</span>
        <a-tag
          :color="
            action === 'ALERT' ? 'gold' : action === 'KILL' ? 'red' : 'volcano'
          "
        >
          {{ action }}
        </a-tag>
      </div>
      <div class="meter-row">
        <span>Map</span>
        <a-tag :color="mapMode === 'NONE' ? 'default' : 'purple'">{{
          mapMode
        }}</a-tag>
      </div>
      <div class="meter-grid">
        <div>
          <strong>{{ conditionCount }}</strong>
          <span>条件</span>
        </div>
        <div>
          <strong>{{ treeDepth }}</strong>
          <span>层深</span>
        </div>
        <div>
          <strong>{{ codeLines }}</strong>
          <span>行 C</span>
        </div>
      </div>
      <div class="plugin-id" :title="pluginId">{{ pluginId }}</div>
    </div>

    <div class="validation-card">
      <div class="validation-head">
        <span>编译前验证</span>
        <a-tag :color="compileReady ? 'green' : 'red'">
          {{ compileReady ? "READY" : "FIX REQUIRED" }}
        </a-tag>
      </div>
      <div class="autosave-line">{{ autosaveLabel }}</div>
      <div v-if="validationIssues.length" class="issue-list">
        <div
          v-for="issue in validationIssues"
          :key="issue.id"
          class="issue-row"
        >
          <a-tag :color="issueColor(issue.severity)" class="issue-tag">
            {{ issue.severity }}
          </a-tag>
          <div class="issue-copy">
            <strong>{{ issue.title }}</strong>
            <span v-if="issue.detail">{{ issue.detail }}</span>
          </div>
        </div>
      </div>
      <div v-else class="no-issues">当前积木参数可安全进入编译流程。</div>
    </div>

    <div class="recipe-list">
      <button
        v-for="recipe in recipes"
        :key="recipe.id"
        type="button"
        class="recipe-card"
        @click="emit('apply-recipe', recipe.id)"
      >
        <div class="recipe-title">{{ recipe.name }}</div>
        <div class="recipe-desc">{{ recipe.description }}</div>
        <div class="recipe-tags">
          <a-tag
            v-for="tag in recipe.tags"
            :key="`${recipe.id}-${tag}`"
            class="recipe-tag"
          >
            {{ tag }}
          </a-tag>
        </div>
      </button>
    </div>

    <div class="workspace-actions">
      <div class="history-actions">
        <a-button
          size="small"
          :disabled="undoCount === 0"
          @click="emit('undo-workspace')"
        >
          ↶ 撤销
        </a-button>
        <a-button
          size="small"
          :disabled="redoCount === 0"
          @click="emit('redo-workspace')"
        >
          ↷ 重做
        </a-button>
      </div>
      <div class="history-line">
        历史栈 {{ undoCount }} / 重做 {{ redoCount }}，支持 Ctrl/Cmd+Z。
      </div>
      <a-button block size="small" @click="emit('export-workspace')">
        <template #icon><ExportOutlined /></template>
        导出 JSON
      </a-button>
      <a-button block size="small" @click="emit('import-workspace')">
        <template #icon><ImportOutlined /></template>
        导入 JSON
      </a-button>
      <a-button block size="small" @click="emit('save-draft')">
        保存草稿
      </a-button>
      <a-button block size="small" @click="emit('clear-draft')">
        清除草稿
      </a-button>
      <a-button block size="small" danger @click="emit('reset-workspace')">
        <template #icon><ReloadOutlined /></template>
        重置画布
      </a-button>
    </div>
  </div>
</template>

<style scoped>
.recipe-panel {
  background-color: #ffffff;
  border: 1px solid #d6e4ff;
  border-radius: 10px;
  padding: 14px;
  box-shadow: 0 8px 24px rgba(22, 119, 255, 0.08);
  color: #475569;
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid #d6e4ff;
}

.panel-icon {
  color: #1677ff;
}

.panel-header h4 {
  margin: 0;
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.panel-desc {
  color: #64748b;
  font-size: 11px;
  line-height: 1.45;
  margin-bottom: 12px;
}

.workspace-meter {
  background: #f8fbff;
  border: 1px solid #d6e4ff;
  border-radius: 8px;
  padding: 10px;
  margin-bottom: 12px;
}

.meter-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
  margin-bottom: 6px;
}

.meter-row span {
  color: #64748b;
}

.meter-row code {
  color: #1677ff;
  font-size: 10px;
}

.meter-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 6px;
  margin: 8px 0;
}

.meter-grid div {
  background: #ffffff;
  border: 1px solid #e6f4ff;
  border-radius: 6px;
  padding: 6px 4px;
  text-align: center;
}

.meter-grid strong {
  display: block;
  color: #0f172a;
  font-size: 15px;
  line-height: 1.1;
}

.meter-grid span {
  display: block;
  color: #64748b;
  font-size: 10px;
  margin-top: 2px;
}

.plugin-id {
  color: #1677ff;
  font-size: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  border-top: 1px dashed #d6e4ff;
  padding-top: 6px;
}

.validation-card {
  background: #f8fbff;
  border: 1px solid #d6e4ff;
  border-radius: 8px;
  padding: 10px;
  margin-bottom: 12px;
}

.validation-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #0f172a;
  font-size: 12px;
  font-weight: 700;
  margin-bottom: 6px;
}

.autosave-line {
  color: #64748b;
  font-size: 10px;
  margin-bottom: 8px;
}

.issue-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.issue-row {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  padding: 6px;
  background: #ffffff;
  border-radius: 6px;
}

.issue-tag {
  margin: 0;
  text-transform: uppercase;
  font-size: 9px;
}

.issue-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.issue-copy strong {
  color: #0f172a;
  font-size: 10.5px;
  line-height: 1.3;
}

.issue-copy span {
  color: #64748b;
  font-size: 10px;
  line-height: 1.35;
}

.no-issues {
  color: #237804;
  font-size: 10.5px;
}

.recipe-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.recipe-card {
  width: 100%;
  text-align: left;
  border: 1px solid #e2e8f0;
  border-left: 3px solid #1677ff;
  background: #ffffff;
  color: inherit;
  border-radius: 6px;
  padding: 9px;
  cursor: pointer;
  transition: all 0.18s ease;
}

.recipe-card:hover {
  transform: translateX(2px);
  border-color: #1677ff;
  background: #f0f7ff;
  box-shadow: 0 8px 18px rgba(22, 119, 255, 0.1);
}

.recipe-title {
  color: #0f172a;
  font-weight: 700;
  font-size: 12px;
  margin-bottom: 3px;
}

.recipe-desc {
  color: #64748b;
  font-size: 10.5px;
  line-height: 1.4;
}

.recipe-tags {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 6px;
}

.recipe-tag {
  margin: 0;
  font-size: 10px;
  background: #e6f4ff !important;
  border-color: #91caff !important;
  color: #0958d9 !important;
}

.workspace-actions {
  display: grid;
  grid-template-columns: 1fr;
  gap: 6px;
  margin-top: 12px;
}

.history-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.history-line {
  color: #64748b;
  font-size: 10px;
  line-height: 1.35;
}
</style>
