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
  background: #ffffff !important;
  backdrop-filter: blur(8px);
  border: 1px solid #d6e4ff !important;
  border-radius: 10px;
  box-shadow: 0 8px 24px rgba(22, 119, 255, 0.08);
  color: #0f172a !important;
}

:deep(.ant-card-head) {
  background: linear-gradient(135deg, #ffffff, #f0f7ff) !important;
  border-bottom: 1px solid #d6e4ff !important;
  color: #0f172a !important;
  border-top-left-radius: 10px;
  border-top-right-radius: 10px;
}

:deep(.ant-card-head-title) {
  font-weight: 600;
  letter-spacing: 0.5px;
}

.source-hint {
  margin-bottom: 12px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.generated-code-box {
  background: #f8fbff;
  border-radius: 6px;
  border: 1px solid #d6e4ff;
  overflow: auto;
  max-height: 560px;
}

.generated-code-box pre {
  margin: 0;
  padding: 16px;
}

.generated-code-box code {
  color: #0f172a;
  font-size: 11.5px;
  line-height: 1.6;
  font-family: "JetBrains Mono", Menlo, Consolas, monospace;
}

.compilation-logger {
  margin-top: 20px;
  border: 1px solid #d6e4ff;
  border-radius: 6px;
  overflow: hidden;
  background: #ffffff;
}

.logger-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: #f0f7ff;
  font-size: 13px;
  font-weight: 600;
}

.logger-body {
  margin: 0;
  padding: 14px;
  color: #0f172a;
  font-size: 12px;
  line-height: 1.6;
  max-height: 260px;
  overflow: auto;
}

.action-footer {
  display: flex;
  justify-content: flex-end;
  padding: 12px 14px;
  border-top: 1px solid #d6e4ff;
}

.load-btn {
  box-shadow: 0 8px 18px rgba(22, 119, 255, 0.14);
}

.compiling-tag,
.success-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
</style>
