<script setup lang="ts">
import { LoadingOutlined, PlayCircleOutlined } from "@ant-design/icons-vue";

defineProps<{
  code: string;
  compiling: boolean;
  compiled: boolean;
  loading: boolean;
  log: string;
}>();

defineEmits<{
  (e: "load"): void;
}>();
</script>

<template>
  <a-card title="动态生成的 eBPF C 语言高阶过滤器源码" size="small" class="blueprint-code-card">
    <template #extra>
      <a-tag color="purple" class="c-tag">Pure C / Libbpf</a-tag>
    </template>

    <div class="generated-code-box">
      <pre><code>{{ code }}</code></pre>
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

.c-tag {
  background-color: rgba(114, 46, 209, 0.15) !important;
  border-color: rgba(114, 46, 209, 0.3) !important;
  color: #d3adf7 !important;
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
