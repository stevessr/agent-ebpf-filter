<script setup lang="ts">
import { computed, ref } from "vue";
import {
  DeleteOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
} from "@ant-design/icons-vue";

type RedactionLevel = "None" | "Basic" | "Standard" | "Strict";

type RedactionScope = "ws" | "jsonl" | "mcp";

type RedactionRule = {
  id: string;
  name: string;
  pattern: string;
  replacement: string;
};

const levels: Array<{
  value: RedactionLevel;
  title: string;
  description: string;
}> = [
  {
    value: "None",
    title: "None",
    description: "不启用自动脱敏，原始内容会按现有出口配置直接输出。",
  },
  {
    value: "Basic",
    title: "Basic",
    description: "仅处理明显敏感字段，例如 token、api key、password。",
  },
  {
    value: "Standard",
    title: "Standard",
    description: "覆盖常见凭据、邮件、手机号、路径中的敏感片段。",
  },
  {
    value: "Strict",
    title: "Strict",
    description: "尽可能对请求体、响应体和元数据进行强脱敏，优先保守输出。",
  },
];

const props = withDefaults(
  defineProps<{
    modelValue?: {
      level?: RedactionLevel;
      outputs?: Record<RedactionScope, boolean>;
      rules?: RedactionRule[];
    };
  }>(),
  {
    modelValue: () => ({
      level: "Standard",
      outputs: { ws: true, jsonl: true, mcp: false },
      rules: [],
    }),
  },
);

const emit = defineEmits<{
  (e: "update:modelValue", value: {
    level: RedactionLevel;
    outputs: Record<RedactionScope, boolean>;
    rules: RedactionRule[];
  }): void;
}>();

const state = ref({
  level: props.modelValue.level ?? "Standard",
  outputs: {
    ws: props.modelValue.outputs?.ws ?? true,
    jsonl: props.modelValue.outputs?.jsonl ?? true,
    mcp: props.modelValue.outputs?.mcp ?? false,
  },
  rules: (props.modelValue.rules ?? []).map((rule, index) => ({
    id: rule.id || `rule-${index}`,
    name: rule.name || `Rule ${index + 1}`,
    pattern: rule.pattern,
    replacement: rule.replacement,
  })),
});

const emitChange = () => {
  emit("update:modelValue", {
    level: state.value.level,
    outputs: { ...state.value.outputs },
    rules: state.value.rules.map((rule) => ({ ...rule })),
  });
};

const selectedLevel = computed({
  get: () => state.value.level,
  set: (value: RedactionLevel) => {
    state.value.level = value;
    emitChange();
  },
});

const addRule = () => {
  state.value.rules.push({
    id: `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
    name: "Custom rule",
    pattern: "",
    replacement: "[REDACTED]",
  });
  emitChange();
};

const removeRule = (id: string) => {
  state.value.rules = state.value.rules.filter((rule) => rule.id !== id);
  emitChange();
};

const onRuleUpdate = () => emitChange();
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-card title="Redaction Policy" size="small">
        <template #extra><SafetyCertificateOutlined /></template>

        <a-alert
          type="info"
          show-icon
          style="margin-bottom: 16px"
          message="选择全局脱敏等级，并按出口单独控制是否输出脱敏后的内容。自定义规则支持按正则或关键字扩展。"
        />

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="12">
            <a-card size="small" title="Redaction Level">
              <a-select v-model:value="selectedLevel" style="width: 100%">
                <a-select-option
                  v-for="level in levels"
                  :key="level.value"
                  :value="level.value"
                >
                  {{ level.title }}
                </a-select-option>
              </a-select>
              <div style="margin-top: 12px">
                <a-typography-paragraph
                  v-for="level in levels"
                  :key="level.value"
                  :style="{
                    marginBottom: '8px',
                    color: selectedLevel === level.value ? '#1677ff' : '#666',
                  }"
                >
                  <strong>{{ level.title }}</strong> — {{ level.description }}
                </a-typography-paragraph>
              </div>
            </a-card>
          </a-col>

          <a-col :xs="24" :lg="12">
            <a-card size="small" title="Output Gates">
              <div style="display: flex; flex-direction: column; gap: 12px">
                <label style="display: flex; align-items: center; gap: 12px">
                  <a-switch v-model:checked="state.outputs.ws" @change="emitChange" />
                  <span>WebSocket output</span>
                </label>
                <label style="display: flex; align-items: center; gap: 12px">
                  <a-switch v-model:checked="state.outputs.jsonl" @change="emitChange" />
                  <span>JSONL persistence</span>
                </label>
                <label style="display: flex; align-items: center; gap: 12px">
                  <a-switch v-model:checked="state.outputs.mcp" @change="emitChange" />
                  <span>MCP output</span>
                </label>
              </div>
            </a-card>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Custom Redaction Rules" size="small">
        <template #extra>
          <a-button type="primary" size="small" @click="addRule">
            <PlusOutlined /> Add Rule
          </a-button>
        </template>

        <a-alert
          type="warning"
          show-icon
          style="margin-bottom: 16px"
          message="规则按顺序应用。建议将更具体的规则放在前面，避免被通用规则过早匹配。"
        />

        <a-space direction="vertical" style="width: 100%" :size="12">
          <a-card
            v-for="rule in state.rules"
            :key="rule.id"
            size="small"
            :hoverable="true"
          >
            <a-row :gutter="[12, 12]" align="middle">
              <a-col :xs="24" :md="5">
                <a-input v-model:value="rule.name" placeholder="Rule name" @change="onRuleUpdate" />
              </a-col>
              <a-col :xs="24" :md="7">
                <a-input v-model:value="rule.pattern" placeholder="Pattern / regex" @change="onRuleUpdate" />
              </a-col>
              <a-col :xs="24" :md="9">
                <a-input v-model:value="rule.replacement" placeholder="Replacement" @change="onRuleUpdate" />
              </a-col>
              <a-col :xs="24" :md="3" style="text-align: right">
                <a-button danger type="text" @click="removeRule(rule.id)">
                  <DeleteOutlined /> Delete
                </a-button>
              </a-col>
            </a-row>
          </a-card>

          <a-empty v-if="state.rules.length === 0" description="No custom rules yet">
            <a-button type="primary" @click="addRule">
              <PlusOutlined /> Add First Rule
            </a-button>
          </a-empty>
        </a-space>
      </a-card>
    </a-col>
  </a-row>
</template>
