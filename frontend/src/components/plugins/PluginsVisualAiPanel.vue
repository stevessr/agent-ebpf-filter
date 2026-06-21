<script setup lang="ts">
import { ref, watch } from "vue";
import { ThunderboltOutlined } from "@ant-design/icons-vue";
import { useNlpCompiler } from "../../composables/plugins/useNlpCompiler";
import type { VisualBlocksPayload } from "../../composables/plugins/useNlpCompiler";

const props = defineProps<{
  modelValue: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", val: string): void;
  (e: "translate", payload: VisualBlocksPayload): void;
}>();

const localPrompt = ref(props.modelValue);
watch(
  () => props.modelValue,
  (val) => {
    localPrompt.value = val;
  },
);
watch(localPrompt, (val) => {
  emit("update:modelValue", val);
});

const {
  aiGenerating,
  lastCompileResult,
  lastCompileMessage,
  lastCompileDescription,
  handleAiGenerate,
} = useNlpCompiler((payload) => emit("translate", payload));

const applyExample = (text: string) => {
  localPrompt.value = text;
};
const generateFromPrompt = () => handleAiGenerate(localPrompt.value);
</script>

<template>
  <div class="block-card ai-copilot-card">
    <div class="block-header">
      <span class="block-badge">LLM Copilot</span>
      <strong style="color: #fff"
        >AI 智能内核防御助手 (NLP Blocks Compiler)</strong
      >
    </div>
    <div class="block-body">
      <div class="desc-line">
        使用后端 OpenAI 兼容 LLM 配置把自然语言防御意图编译成 Trigger /
        Condition / Map / Action 积木流；LLM 不可用时自动降级到本地 NLP 规则。
      </div>
      <a-textarea
        v-model:value="localPrompt"
        placeholder="例如：当有人使用 python 运行网络连接，且外连端口为 4444 时，直接强杀该进程，并启用计数器限制其最大触发频率为 3 次。"
        :rows="4"
        class="ai-textarea"
      />

      <a-alert
        v-if="lastCompileResult"
        class="llm-result-alert"
        :type="lastCompileResult.mode === 'llm' ? 'success' : 'warning'"
        :message="lastCompileMessage"
        :description="lastCompileDescription"
        show-icon
      />

      <div class="control-footer">
        <div class="ai-prompts-examples">
          快捷指令示例：
          <a-tag
            @click="applyExample('阻止 nc 进程运行，并且直接杀死进程')"
            class="example-tag"
            >阻断并杀死 nc</a-tag
          >
          <a-tag
            @click="
              applyExample('当外连端口为 4444 时强杀进程，并限频计数最多 5 次')
            "
            class="example-tag"
            >外连 4444 强杀限频 5 次</a-tag
          >
          <a-tag
            @click="applyExample('拦截对 shadow 文件的重命名操作并发出警告')"
            class="example-tag"
            >勒索 shadow 重命名保护</a-tag
          >
        </div>
        <a-button
          type="primary"
          :loading="aiGenerating"
          @click="generateFromPrompt"
          class="ai-btn"
        >
          <template #icon><ThunderboltOutlined /></template>
          调用 LLM 生成积木
        </a-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.block-card {
  border-radius: 8px;
  overflow: visible;
  box-shadow: 0 8px 24px rgba(22, 119, 255, 0.08);
  background: #ffffff;
  backdrop-filter: blur(8px);
  transition: all 0.3s ease;
  border: 1px solid #d6e4ff;
}
.block-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 14px 38px rgba(22, 119, 255, 0.16);
}
.ai-copilot-card {
  margin-bottom: 20px;
  border-color: #91caff !important;
}
.ai-copilot-card:hover {
  border-color: #1677ff !important;
  box-shadow: 0 0 15px rgba(22, 119, 255, 0.18);
}
.block-header {
  padding: 10px 14px;
  display: flex;
  align-items: center;
  border-bottom: 1px solid #d6e4ff;
  background: linear-gradient(135deg, #1677ff, #4096ff);
}
.block-badge {
  background: rgba(255, 255, 255, 0.22);
  color: #ffffff;
  padding: 2px 8px;
  font-size: 11px;
  border-radius: 4px;
  margin-right: 12px;
  font-weight: bold;
}
.block-body {
  background: #ffffff;
  padding: 18px;
  color: #94a3b8;
}
.desc-line {
  font-size: 13px;
  color: #0958d9;
  font-weight: 500;
  margin-bottom: 12px;
}
.llm-result-alert {
  margin-top: 12px;
  white-space: pre-line;
}
.control-footer {
  margin-top: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.ai-prompts-examples {
  font-size: 11px;
  color: #64748b;
}
.example-tag {
  cursor: pointer;
  margin-right: 4px;
  background-color: #e6f4ff !important;
  border-color: #91caff !important;
  color: #d3adf7 !important;
  transition: all 0.2s ease;
}
.example-tag:hover {
  background-color: #91caff !important;
  border-color: #1677ff !important;
  color: #0958d9 !important;
}
.ai-btn {
  background: #1677ff !important;
  border-color: #1677ff !important;
  transition: all 0.3s ease;
}
.ai-btn:hover {
  background: #4096ff !important;
  box-shadow: 0 0 8px #1677ff;
}
:deep(.ai-textarea) {
  background-color: #ffffff !important;
  border-color: #d9d9d9 !important;
  color: #0f172a !important;
  border-radius: 6px;
}
:deep(.ai-textarea::placeholder) {
  color: #94a3b8 !important;
}
:deep(.ai-textarea:focus) {
  border-color: #1677ff !important;
  box-shadow: 0 0 0 2px rgba(22, 119, 255, 0.16) !important;
}
</style>
