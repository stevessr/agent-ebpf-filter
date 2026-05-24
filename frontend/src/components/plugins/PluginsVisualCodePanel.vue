<script setup lang="ts">
import { LoadingOutlined, PlayCircleOutlined } from "@ant-design/icons-vue";

const props = defineProps<{
  code: string;
  compiling: boolean;
  compiled: boolean;
  loading: boolean;
  log: string;
}>();

const emit = defineEmits<{
  (e: "load"): void;
}>();
</script>

<template>
  <a-card
    title="生成的 eBPF C 源码与编译控制台"
    size="small"
    class="blueprint-code-card"
  >
    <div class="source-hint">
      这里仅展示由低代码画布生成的 libbpf C 源码；TS 伪代码已拆到独立工作区，不再与画布互相转换或共享草稿槽。
    </div>

    <div class="generated-code-box">
      <pre><code>{{ props.code }}</code></pre>
    </div>

    <div v-if="props.compiling || props.compiled || props.log" class="compilation-logger">
      <div class="logger-header">
        <span>Clang LLVM 编译与内核校验审计台</span>
        <a-space>
          <a-tag v-if="props.compiling" color="blue" class="compiling-tag">
            <LoadingOutlined /> 正在编译中...
          </a-tag>
          <a-tag v-else-if="props.compiled" color="green" class="success-tag">
            SUCCESS
          </a-tag>
        </a-space>
      </div>
      <pre class="logger-body"><code>{{ props.log }}</code></pre>

      <div v-if="props.compiled" class="action-footer">
        <a-button
          type="primary"
          @click="emit('load')"
          :loading="props.loading"
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

.source-hint {
  margin-bottom: 12px;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.6;
}

.generated-code-box {
  background: #070b11;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  overflow: auto;
  max-height: 560px;
}

.generated-code-box pre {
  margin: 0;
  padding: 16px;
}

.generated-code-box code {
  color: #b7f7c9;
  font-size: 11.5px;
  line-height: 1.6;
  font-family: "JetBrains Mono", Menlo, Consolas, monospace;
}

.compilation-logger {
  margin-top: 20px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 6px;
  overflow: hidden;
  background: #0f172a;
}

.logger-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: rgba(30, 41, 59, 0.85);
  font-size: 13px;
  font-weight: 600;
}

.logger-body {
  margin: 0;
  padding: 14px;
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
  max-height: 260px;
  overflow: auto;
}

.action-footer {
  display: flex;
  justify-content: flex-end;
  padding: 12px 14px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.load-btn {
  box-shadow: 0 0 18px rgba(34, 197, 94, 0.25);
}

.compiling-tag,
.success-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
</style>
