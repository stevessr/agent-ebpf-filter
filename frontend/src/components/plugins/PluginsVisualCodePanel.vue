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
  <a-card title="动态生成的 eBPF C 语言高阶过滤器源码" size="small" style="box-shadow: 0 4px 10px rgba(0, 0, 0, 0.02);">
    <template #extra>
      <a-tag color="purple">Pure C / Libbpf</a-tag>
    </template>

    <div class="generated-code-box">
      <pre><code>{{ code }}</code></pre>
    </div>

    <!-- Compilation Logger -->
    <div
      v-if="compiling || compiled || log"
      class="compilation-logger"
      style="margin-top: 16px"
    >
      <div class="logger-header">
        <span>Clang LLVM 编译与内核校验审计台</span>
        <a-tag v-if="compiling" color="blue">
          <LoadingOutlined /> 正在编译中...
        </a-tag>
        <a-tag v-else-if="compiled" color="green">SUCCESS</a-tag>
      </div>
      <pre class="logger-body"><code>{{ log }}</code></pre>

      <div
        v-if="compiled"
        style="margin-top: 12px; display: flex; justify-content: flex-end"
      >
        <a-button
          type="primary"
          color="green"
          @click="$emit('load')"
          :loading="loading"
        >
          <template #icon><PlayCircleOutlined /></template>
          载入内核并立即生效插件
        </a-button>
      </div>
    </div>
  </a-card>
</template>

<style scoped>
.generated-code-box {
  background: #1e1e1e;
  border-radius: 6px;
  padding: 12px;
  overflow: auto;
  max-height: 480px;
  border: 1px solid #333;
}
.generated-code-box pre {
  margin: 0;
}
.generated-code-box code {
  font-family: "Consolas", monospace;
  font-size: 12px;
  color: #9cdcfe;
}
.compilation-logger {
  background: #141414;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  overflow: hidden;
}
.logger-header {
  background: #262626;
  padding: 6px 12px;
  color: #fafafa;
  font-size: 13px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.logger-body {
  margin: 0;
  padding: 12px;
  max-height: 180px;
  overflow: auto;
  color: #52c41a;
  background: #000;
  font-family: "Consolas", monospace;
  font-size: 12px;
  white-space: pre-wrap;
}
</style>
