<script setup lang="ts">
import PluginsVisualCodePanel from "./PluginsVisualCodePanel.vue";

defineProps<{
  code: string;
  compiling: boolean;
  compiled: boolean;
  loading: boolean;
  log: string;
  activeFlowNode: string;
  flowSectionClass: (node: string) => Record<string, boolean>;
}>();

const emit = defineEmits<{
  load: [];
}>();
</script>

<template>
  <div
    ref="codeBlock"
    class="source-workspace-shell"
    :class="flowSectionClass('code')"
  >
    <div class="source-workspace-notice">
      <a-tag color="cyan">独立源码 Tab</a-tag>
      <span
        >动态生成的 eBPF C 语言高阶过滤器源码、Clang
        编译日志和加载入口集中在这里，主画布只负责 Dify
        风格节点编排。</span
      >
    </div>
    <PluginsVisualCodePanel
      :code="code"
      :compiling="compiling"
      :compiled="compiled"
      :loading="loading"
      :log="log"
      @load="emit('load')"
    />
  </div>
</template>

<style scoped>
.source-workspace-shell {
  padding-top: 8px;
}

.source-workspace-notice {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid var(--workflow-border);
  background: linear-gradient(135deg, #ffffff 0%, var(--workflow-primary-subtle) 100%);
  color: var(--workflow-text-secondary);
  box-shadow: 0 8px 24px rgba(22, 119, 255, 0.08);
  border-color: var(--workflow-border);
}

.source-workspace-notice span {
  margin: 0;
  color: var(--workflow-text-muted);
  font-size: 12px;
  line-height: 1.45;
}

.flow-section-active {
  outline: 2px solid rgba(22, 119, 255, 0.62);
  outline-offset: 4px;
  box-shadow: 0 0 0 1px rgba(22, 119, 255, 0.2),
    0 0 24px rgba(22, 119, 255, 0.16);
  border-radius: 10px;
  transition: outline-color 0.2s ease, box-shadow 0.2s ease;
}
</style>
