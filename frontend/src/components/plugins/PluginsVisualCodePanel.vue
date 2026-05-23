<script setup lang="ts">
import { ref, watch } from "vue";
import { LoadingOutlined, PlayCircleOutlined, CodeOutlined, SettingOutlined } from "@ant-design/icons-vue";

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

const localPseudoCode = ref(props.pseudoCode);
watch(() => props.pseudoCode, (newVal) => {
  localPseudoCode.value = newVal;
});

const onPseudoCodeChange = () => {
  emit("update:pseudoCode", localPseudoCode.value);
};

const toggleUsePseudoCode = (checked: boolean) => {
  emit("update:usePseudoCode", checked);
};
</script>

<template>
  <a-card title="规则源码与伪代码编译器" size="small" class="blueprint-code-card">
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
        通过编写直观的 TS/JS 风格伪代码，一键编译为底层的 eBPF 过滤规则：
      </div>
      <div class="generated-code-box">
        <textarea
          v-model="localPseudoCode"
          @input="onPseudoCodeChange"
          class="pseudo-code-editor"
          placeholder="// 编写您的 TS 风格 eBPF 过滤规则"
          rows="15"
        ></textarea>
      </div>
      <div style="margin-top: 12px; display: flex; justify-content: space-between; align-items: center">
        <span style="font-size: 11px; color: #64748b">
          * 提示：支持 ctx.comm, ctx.port, ctx.pid, ctx.uid 等常见匹配，修改后将双向同步至积木面板。
        </span>
        <a-button
          v-if="usePseudoCode"
          type="primary"
          size="small"
          @click="emit('compile-pseudo-code')"
          :loading="compiling"
        >
          立即编译伪代码
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

.pseudo-code-editor {
  width: 100%;
  background: transparent;
  border: none;
  color: #a8ffb2;
  font-family: "Consolas", "Courier New", monospace;
  font-size: 12px;
  line-height: 1.6;
  resize: vertical;
  outline: none;
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
