<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from "vue";
import { LoadingOutlined, PlayCircleOutlined, CodeOutlined, SettingOutlined } from "@ant-design/icons-vue";
import * as monaco from "monaco-editor";

// 配置 Monaco Web Workers
import "monaco-editor/esm/vs/language/typescript/monaco.contribution";
import "monaco-editor/esm/vs/editor/editor.all.js";

const props = defineProps<{
  code: string;
  compiling: boolean;
  compiled: boolean;
  loading: boolean;
  log: string;
  pseudoCode: string;
  usePseudoCode: boolean;
}>();

const emit = defineEmits<{
  (e: "load"): void;
  (e: "update:pseudoCode", val: string): void;
  (e: "update:usePseudoCode", val: boolean): void;
  (e: "compile-pseudo-code"): void;
}>();

const activeTab = ref<"pseudo" | "c">("pseudo");
const editorContainer = ref<HTMLElement | null>(null);
let editor: monaco.editor.IStandaloneCodeEditor | null = null;

// 初始化 Monaco Editor
const initMonaco = () => {
  if (!editorContainer.value) return;

  // 注册自定义 eBPF 自动补全
  monaco.languages.registerCompletionItemProvider("typescript", {
    provideCompletionItems: (model, position) => {
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      };

      const suggestions = [
        {
          label: "ctx.comm",
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: "当前触发事件的进程名称 (string)",
          insertText: "ctx.comm",
          range,
        },
        {
          label: "ctx.pid",
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: "当前触发事件的进程 PID (number)",
          insertText: "ctx.pid",
          range,
        },
        {
          label: "ctx.uid",
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: "当前触发事件的用户 UID (number, 0 代表 root)",
          insertText: "ctx.uid",
          range,
        },
        {
          label: "ctx.gid",
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: "当前触发事件的用户组 GID (number)",
          insertText: "ctx.gid",
          range,
        },
        {
          label: "ctx.basename",
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: "操作的目标文件名 (string)",
          insertText: "ctx.basename",
          range,
        },
        {
          label: "ctx.port",
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: "外发网络的目标端口 (number, 仅在 socket_connect 有效)",
          insertText: "ctx.port",
          range,
        },
        {
          label: "ctx.ipv4",
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: "外发网络的目标 IPv4 地址 (string, 仅在 socket_connect 有效)",
          insertText: "ctx.ipv4",
          range,
        },
        {
          label: "Action.block()",
          kind: monaco.languages.CompletionItemKind.Method,
          documentation: "执行内核级硬拦截阻断行为",
          insertText: "Action.block();",
          range,
        },
        {
          label: "Action.alert()",
          kind: monaco.languages.CompletionItemKind.Method,
          documentation: "仅触发 RingBuffer 告警审计，不阻断执行",
          insertText: "Action.alert();",
          range,
        },
        {
          label: "Action.kill()",
          kind: monaco.languages.CompletionItemKind.Method,
          documentation: "发送 SIGKILL (9) 信号强制终止该进程",
          insertText: "Action.kill();",
          range,
        },
        {
          label: "Maps.createCounter",
          kind: monaco.languages.CompletionItemKind.Method,
          documentation: "声明一个自动频控计数器 Map",
          insertText: 'Maps.createCounter({ key: "pid", limit: 3 })',
          range,
        },
        {
          label: "Maps.createBlocklist",
          kind: monaco.languages.CompletionItemKind.Method,
          documentation: "声明一个运行时查表黑名单 Map",
          insertText: 'Maps.createBlocklist({ key: "comm" })',
          range,
        },
      ];

      return { suggestions };
    },
  });

  editor = monaco.editor.create(editorContainer.value, {
    value: props.pseudoCode,
    language: "typescript",
    theme: "vs-dark",
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 12,
    lineNumbers: "on",
    roundedSelection: true,
    scrollBeyondLastLine: false,
    readOnly: !props.usePseudoCode,
  });

  editor.onDidChangeModelContent(() => {
    const value = editor?.getValue() || "";
    emit("update:pseudoCode", value);
  });
};

watch(() => props.pseudoCode, (newVal) => {
  if (editor && editor.getValue() !== newVal) {
    editor.setValue(newVal);
  }
});

watch(() => props.usePseudoCode, (newVal) => {
  if (editor) {
    editor.updateOptions({ readOnly: !newVal });
  }
});

watch(activeTab, async (newVal) => {
  if (newVal === "pseudo") {
    await nextTick();
    if (!editor) {
      initMonaco();
    } else {
      editor.layout();
    }
  }
});

onMounted(() => {
  if (activeTab.value === "pseudo") {
    initMonaco();
  }
});

onBeforeUnmount(() => {
  if (editor) {
    editor.dispose();
    editor = null;
  }
});

const toggleUsePseudoCode = (checked: boolean) => {
  emit("update:usePseudoCode", checked);
};
</script>

<template>
  <a-card title="规则源码与高级伪代码编辑器 (VSCode 同款)" size="small" class="blueprint-code-card">
    <template #extra>
      <a-space size="middle">
        <a-checkbox :checked="usePseudoCode" @update:checked="toggleUsePseudoCode">
          <span style="color: #cbd5e1; font-size: 12px">启用 TS 伪代码编译</span>
        </a-checkbox>
        <a-radio-group v-model:value="activeTab" size="small" button-style="solid">
          <a-radio-button value="pseudo">
            <CodeOutlined /> TS 伪代码
          </a-radio-button>
          <a-radio-button value="c">
            <SettingOutlined /> 生成的 eBPF C 源码
          </a-radio-button>
        </a-radio-group>
      </a-space>
    </template>

    <div v-show="activeTab === 'pseudo'">
      <div style="margin-bottom: 8px; font-size: 12px; color: #94a3b8">
        编写直观的 TS/JS 风格伪代码，支持 VSCode 同款 **智能自动补全**、**实时高亮**、与低代码积木面板**双向绑定同步**：
      </div>
      <div class="monaco-editor-wrapper">
        <div ref="editorContainer" class="monaco-container"></div>
      </div>
      <div style="margin-top: 12px; display: flex; justify-content: space-between; align-items: center">
        <span style="font-size: 11px; color: #64748b">
          * 提示：输入 <code>ctx.</code> 或 <code>Action.</code> 或 <code>Maps.</code> 即可唤起自动补全。
        </span>
        <a-button
          v-if="usePseudoCode"
          type="primary"
          size="small"
          @click="emit('compile-pseudo-code')"
          :loading="compiling"
        >
          立即编译并生成 C 代码
        </a-button>
      </div>
    </div>

    <div v-show="activeTab === 'c'">
      <div class="generated-code-box">
        <pre><code>{{ code }}</code></pre>
      </div>
    </div>

    <!-- Compilation Logger -->
    <div
      v-if="compiling || compiled || log"
      class="compilation-logger"
    >
      <div class="logger-header">
        <span>Clang LLVM 编译与内核校验审计台</span>
        <a-tag v-if="compiling" color="blue" class="compiling-tag">
          <LoadingOutlined /> 正在编译中...
        </a-tag>
        <a-tag v-else-if="compiled" color="green" class="success-tag">SUCCESS</a-tag>
      </div>
      <pre class="logger-body"><code>{{ log }}</code></pre>

      <div
        v-if="compiled"
        class="action-footer"
      >
        <a-button
          type="primary"
          @click="$emit('load')"
          :loading="loading"
          class="load-btn"
        >
          <template #icon><PlayCircleOutlined /></template>
          载入内核并立即生效插件
        </a-button>
      </div>
    </div>
  </a-card>
</template>

<style scoped>
.blueprint-code-card {
  background: rgba(13, 19, 33, 0.85) !important;
  backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.08) !important;
  border-radius: 8px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  color: #ffffff !important;
}

:deep(.ant-card-head) {
  background: linear-gradient(135deg, #1e293b, #0f172a) !important;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08) !important;
  color: #ffffff !important;
  border-top-left-radius: 8px;
  border-top-right-radius: 8px;
}

:deep(.ant-card-head-title) {
  font-weight: 600;
  letter-spacing: 0.5px;
}

.monaco-editor-wrapper {
  background: #1e1e1e;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  overflow: hidden;
}

.monaco-container {
  width: 100%;
  height: 320px;
}

.generated-code-box {
  background: #070b11;
  border-radius: 6px;
  padding: 14px;
  overflow: auto;
  max-height: 480px;
  border: 1px solid rgba(255, 255, 255, 0.05);
  box-shadow: inset 0 0 10px rgba(0, 0, 0, 0.5);
}

.generated-code-box pre {
  margin: 0;
}

.generated-code-box code {
  font-family: "Consolas", "Courier New", monospace;
  font-size: 11.5px;
  color: #9cdcfe;
  line-height: 1.5;
}

.compilation-logger {
  background: #0b0f19;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  overflow: hidden;
  margin-top: 16px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.3);
}

.logger-header {
  background: #1e293b;
  padding: 8px 12px;
  color: #f1f5f9;
  font-size: 12px;
  font-weight: bold;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid rgba(255,255,255,0.05);
}

.compiling-tag {
  background-color: rgba(24, 144, 255, 0.15) !important;
  border-color: rgba(24, 144, 255, 0.3) !important;
  color: #1890ff !important;
}

.success-tag {
  background-color: rgba(82, 196, 26, 0.15) !important;
  border-color: rgba(82, 196, 26, 0.3) !important;
  color: #52c41a !important;
}

.logger-body {
  margin: 0;
  padding: 12px;
  max-height: 180px;
  overflow: auto;
  color: #4ade80;
  background: #020408;
  font-family: "Consolas", "Courier New", monospace;
  font-size: 11.5px;
  white-space: pre-wrap;
  line-height: 1.4;
}

.action-footer {
  margin-top: 12px;
  padding: 0 12px 12px 12px;
  display: flex;
  justify-content: flex-end;
}

.load-btn {
  background: #52c41a !important;
  border-color: #52c41a !important;
  box-shadow: 0 2px 8px rgba(82, 196, 26, 0.3);
  transition: all 0.3s ease;
}

.load-btn:hover {
  background: #73d13d !important;
  border-color: #73d13d !important;
  box-shadow: 0 0 12px rgba(82, 196, 26, 0.6);
}
</style>
