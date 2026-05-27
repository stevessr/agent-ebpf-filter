<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  CodeOutlined,
  CopyOutlined,
  DeleteOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SaveOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import * as monaco from "monaco-editor";
import "monaco-editor/esm/vs/language/typescript/monaco.contribution";
import "monaco-editor/esm/vs/editor/editor.all.js";

import { usePlugins } from "../../composables/plugins/usePlugins";
import { configureMonacoTypesAndCompletion } from "./monaco-config";
import {
  createPseudoSeedSnapshot,
  pseudoCodeToBpfSnapshot,
} from "./pseudo-compiler";
import { generateBpfCode } from "./transpiler";
import { usePseudoDraft } from "./usePseudoDraft";
import { usePseudoValidation } from "./usePseudoValidation";

const { compileBpf, loadBpf, upsertPlugin, fetchPlugins, compileLog } =
  usePlugins();

const pluginId = ref("ts-pseudocode-filter");
const pluginName = ref("TS 伪代码过滤插件");
const description = ref(
  "由独立 TS 伪代码工作区生成的 eBPF 过滤审计插件。"
);
const pseudoCode = ref("");
const compiling = ref(false);
const compiled = ref(false);
const loadingAction = ref(false);
const compileLogLocal = ref("");
const editorContainer = ref<HTMLElement | null>(null);
let editor: monaco.editor.IStandaloneCodeEditor | null = null;

// Draft persistence
const {
  autosaveLabel,
  pseudoStorageKey,
  defaultPseudoCode,
  saveDraft,
  restoreDraft,
  clearDraft,
  resetDraft: resetDraftBase,
} = usePseudoDraft(pluginId, pluginName, description, pseudoCode, compiled, compileLogLocal, { value: null });

// Initialize pseudoCode with default
pseudoCode.value = defaultPseudoCode;

const parsedSnapshot = computed(() =>
  pseudoCodeToBpfSnapshot(
    pseudoCode.value,
    createPseudoSeedSnapshot(pluginId.value, pluginName.value, description.value)
  )
);

const generatedBpfCode = computed(() =>
  generateBpfCode(parsedSnapshot.value, PSEUDO_PROGRAM_NAME)
);

const generatedLineCount = computed(
  () => generatedBpfCode.value.split(/\r?\n/).length
);

// Validation
const {
  attachKind,
  attachTarget,
  validationIssues,
  validationErrors,
  compileReady,
  PSEUDO_PROGRAM_NAME,
} = usePseudoValidation(pluginId, pseudoCode, parsedSnapshot);

const conditionCount = computed(() => validationIssues.value.length);

const resetDraft = () => {
  resetDraftBase();
  void nextTick(() => editor?.layout());
};

const initMonaco = () => {
  if (!editorContainer.value || editor) return;
  configureMonacoTypesAndCompletion();
  editor = monaco.editor.create(editorContainer.value, {
    value: pseudoCode.value,
    language: "typescript",
    theme: "vs-dark",
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 12,
    lineNumbers: "on",
    roundedSelection: true,
    scrollBeyondLastLine: false,
  });
  editor.onDidChangeModelContent(() => {
    const nextCode = editor?.getValue() || "";
    if (nextCode !== pseudoCode.value) pseudoCode.value = nextCode;
  });
};

watch(pseudoCode, (nextCode) => {
  if (editor && editor.getValue() !== nextCode) {
    editor.setValue(nextCode);
  }
});

watch(
  [pluginId, pluginName, description, pseudoCode],
  () => {
    compiled.value = false;
    saveDraft(true);
  },
  { deep: false }
);

onMounted(() => {
  restoreDraft();
  void nextTick(initMonaco);
});

onBeforeUnmount(() => {
  editor?.dispose();
  editor = null;
});

const appendCompilerLog = () => {
  if (compileLog.value) {
    compileLogLocal.value += `\n\n--- clang 输出 ---\n${compileLog.value}`;
  }
};

const handleCompileAndRegister = async () => {
  if (!compileReady.value) {
    compileLogLocal.value = [
      "已阻止编译：独立 TS 伪代码工作区存在错误。",
      ...validationErrors.value.map((issue) => `[ERROR] ${issue.text}`),
    ].join("\n");
    message.error("请先修复 TS 伪代码工作区中的错误");
    return;
  }

  compiling.value = true;
  compiled.value = false;
  compileLogLocal.value = [
    "正在解析独立 TS 伪代码，并生成内部 eBPF 转译输入...",
    `Trigger: ${parsedSnapshot.value.trigger}`,
    `Attach: ${attachKind.value} / ${attachTarget.value} / program=${PSEUDO_PROGRAM_NAME}`,
    `Conditions: generated C lines: ${generatedLineCount.value}`,
  ].join("\n");

  try {
    compileLogLocal.value += `\n正在注册伪代码插件 Manifest [${pluginId.value}] 至本地仓库...`;
    await upsertPlugin({
      id: pluginId.value,
      name: pluginName.value,
      description: description.value,
      kind: "ebpf",
      enabled: false,
      attachKind: attachKind.value,
      attachTarget: attachTarget.value,
      programName: PSEUDO_PROGRAM_NAME,
      source: generatedBpfCode.value,
    });

    compileLogLocal.value += "\n正在调用 LLVM/Clang 编译生成的 eBPF C 源码...";
    const success = await compileBpf(pluginId.value, generatedBpfCode.value);
    appendCompilerLog();
    if (success) {
      compiled.value = true;
      compileLogLocal.value +=
        "\n\n[SUCCESS] 独立 TS 伪代码插件编译成功，可立即加载。";
      message.success("TS 伪代码插件编译成功");
      await fetchPlugins();
    } else {
      compileLogLocal.value += "\n\n[ERROR] Clang 编译失败，请查看输出。";
    }
  } catch (err: any) {
    compileLogLocal.value += `\n[ERROR] 错误: ${err?.message || err}`;
  } finally {
    compiling.value = false;
  }
};

const handleLoad = async () => {
  loadingAction.value = true;
  try {
    await loadBpf(pluginId.value);
    await fetchPlugins();
  } finally {
    loadingAction.value = false;
  }
};

const copyGeneratedSource = async () => {
  try {
    await navigator.clipboard.writeText(generatedBpfCode.value);
    message.success("已复制当前 TS 伪代码生成的 eBPF C 源码");
  } catch {
    message.warning("当前浏览器不允许写入剪贴板");
  }
};
</script>

<template>
  <div class="pseudo-workspace">
    <a-alert
      type="info"
      show-icon
      class="workspace-alert"
      message="TS 伪代码已与画布解耦"
      description="此页拥有独立组件状态、独立编译/注册流程、独立 localStorage 存储槽；不会把伪代码反向同步到低代码画布，也不会从画布实时生成伪代码。"
    />

    <a-row :gutter="16">
      <a-col :span="7">
        <a-card title="TS 伪代码插件元数据" size="small">
          <a-form layout="vertical">
            <a-form-item label="插件 ID">
              <a-input v-model:value="pluginId" />
            </a-form-item>
            <a-form-item label="显示名称">
              <a-input v-model:value="pluginName" />
            </a-form-item>
            <a-form-item label="说明">
              <a-textarea v-model:value="description" :rows="3" />
            </a-form-item>
          </a-form>
          <a-space wrap>
            <a-button size="small" @click="saveDraft(false)">
              <template #icon><SaveOutlined /></template>
              保存草稿
            </a-button>
            <a-button size="small" @click="restoreDraft">
              <template #icon><ReloadOutlined /></template>
              恢复
            </a-button>
            <a-button size="small" @click="resetDraft">重置示例</a-button>
            <a-button size="small" danger @click="clearDraft">
              <template #icon><DeleteOutlined /></template>
              清空槽
            </a-button>
          </a-space>
          <div class="storage-meta">
            <div>{{ autosaveLabel }}</div>
            <code>{{ pseudoStorageKey }}</code>
          </div>
        </a-card>

        <a-card title="独立解析结果" size="small" style="margin-top: 12px">
          <a-descriptions :column="1" size="small">
            <a-descriptions-item label="Trigger">
              <code>{{ parsedSnapshot.trigger }}</code>
            </a-descriptions-item>
            <a-descriptions-item label="Attach">
              <code>{{ attachKind }} / {{ attachTarget }}</code>
            </a-descriptions-item>
            <a-descriptions-item label="Action">
              <code>{{ parsedSnapshot.action }}</code>
            </a-descriptions-item>
            <a-descriptions-item label="Map">
              <code>{{ parsedSnapshot.mapMode }} / {{ parsedSnapshot.mapKey }}</code>
            </a-descriptions-item>
            <a-descriptions-item label="Conditions">
              <code>{{ conditionCount }}</code>
            </a-descriptions-item>
          </a-descriptions>
          <a-list
            v-if="validationIssues.length"
            :data-source="validationIssues"
            size="small"
            class="validation-list"
          >
            <template #renderItem="{ item }">
              <a-list-item>
                <a-tag :color="item.severity === 'error' ? 'red' : 'orange'">
                  {{ item.severity.toUpperCase() }}
                </a-tag>
                <span>{{ item.text }}</span>
              </a-list-item>
            </template>
          </a-list>
          <a-empty v-else description="当前 TS 伪代码可编译" />
        </a-card>
      </a-col>

      <a-col :span="17">
        <a-card size="small" class="pseudo-editor-card">
          <template #title>
            <span><CodeOutlined /> 独立 TS 伪代码编辑器</span>
          </template>
          <template #extra>
            <a-space>
              <a-tag :color="compileReady ? 'green' : 'red'">
                {{ compileReady ? "READY" : "FIX REQUIRED" }}
              </a-tag>
              <a-button size="small" @click="copyGeneratedSource">
                <template #icon><CopyOutlined /></template>
                复制生成 C
              </a-button>
              <a-button
                type="primary"
                :disabled="!compileReady"
                :loading="compiling"
                @click="handleCompileAndRegister"
              >
                <template #icon><ThunderboltOutlined /></template>
                编译并注册
              </a-button>
              <a-button
                v-if="compiled"
                :loading="loadingAction"
                @click="handleLoad"
              >
                <template #icon><PoweroffOutlined /></template>
                立即加载
              </a-button>
            </a-space>
          </template>

          <div ref="editorContainer" class="monaco-container"></div>

          <a-card
            title="由 TS 伪代码生成的 eBPF C 预览"
            size="small"
            class="generated-preview-card"
          >
            <template #extra>
              <a-tag color="cyan">{{ generatedLineCount }} lines</a-tag>
            </template>
            <pre class="generated-code"><code>{{ generatedBpfCode }}</code></pre>
          </a-card>

          <a-card
            v-if="compiling || compiled || compileLogLocal"
            title="独立 TS 编译日志"
            size="small"
            class="compile-log-card"
          >
            <pre class="compile-log"><code>{{ compileLogLocal }}</code></pre>
          </a-card>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
.pseudo-workspace {
  min-height: 640px;
}

.workspace-alert {
  margin-bottom: 16px;
}

.storage-meta {
  margin-top: 12px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.7;
}

.storage-meta code {
  word-break: break-all;
}

.validation-list {
  margin-top: 12px;
}

.validation-list :deep(.ant-list-item) {
  align-items: flex-start;
  gap: 8px;
}

.pseudo-editor-card {
  min-height: 680px;
}

.monaco-container {
  width: 100%;
  height: 360px;
  border: 1px solid #1f2937;
  border-radius: 8px;
  overflow: hidden;
}

.generated-preview-card,
.compile-log-card {
  margin-top: 12px;
}

.generated-code,
.compile-log {
  margin: 0;
  padding: 14px;
  border-radius: 6px;
  background: #070b11;
  color: #b7f7c9;
  font-size: 11.5px;
  line-height: 1.6;
  max-height: 340px;
  overflow: auto;
}

.compile-log {
  color: #e2e8f0;
}
</style>
