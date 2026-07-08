<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from "vue";
import {
  DeleteOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
  LoadingOutlined,
} from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import axios from "axios";

type RedactionLevel = "none" | "basic" | "standard" | "strict";
type FieldCategory = "path" | "command" | "network" | "credential" | "identifier";

interface RedactionRule {
  id: string;
  description?: string;
  level: RedactionLevel;
  categories?: FieldCategory[];
  replaceWith?: string;
  enabled?: boolean;
}

interface RedactionPolicy {
  level: RedactionLevel;
  rules?: RedactionRule[];
  defaultPlaceholder?: string;
  preserveLengths?: boolean;
  excludeCategories?: FieldCategory[];
}

const levels: Array<{
  value: RedactionLevel;
  title: string;
  description: string;
}> = [
  {
    value: "none",
    title: "None",
    description: "不启用自动脱敏，原始内容会按现有出口配置直接输出。",
  },
  {
    value: "basic",
    title: "Basic",
    description: "仅处理明显敏感字段，例如 token、api key、password。",
  },
  {
    value: "standard",
    title: "Standard",
    description: "覆盖常见凭据、邮件、手机号、路径中的敏感片段。",
  },
  {
    value: "strict",
    title: "Strict",
    description: "尽可能对请求体、响应体和元数据进行强脱敏，优先保守输出。",
  },
];

const loading = ref(false);
const saving = ref(false);
const policy = ref<RedactionPolicy>({
  level: "standard",
  rules: [],
  defaultPlaceholder: "[REDACTED]",
  preserveLengths: false,
  excludeCategories: [],
});

const selectedLevel = computed({
  get: () => policy.value.level,
  set: (value: RedactionLevel) => {
    policy.value.level = value;
  },
});

const customRules = computed({
  get: () => policy.value.rules || [],
  set: (value: RedactionRule[]) => {
    policy.value.rules = value;
  },
});

const fetchPolicy = async () => {
  loading.value = true;
  try {
    const res = await axios.get("/config/redaction-policy");
    const data = res.data as RedactionPolicy;
    policy.value = {
      level: data.level || "standard",
      rules: Array.isArray(data.rules) ? data.rules : [],
      defaultPlaceholder: data.defaultPlaceholder || "[REDACTED]",
      preserveLengths: !!data.preserveLengths,
      excludeCategories: Array.isArray(data.excludeCategories) ? data.excludeCategories : [],
    };
  } catch (err: any) {
    message.error(err?.response?.data?.error || "加载脱敏策略失败");
  } finally {
    loading.value = false;
  }
};

const savePolicy = async () => {
  saving.value = true;
  try {
    const res = await axios.put("/config/redaction-policy", policy.value);
    const data = res.data as RedactionPolicy;
    policy.value = {
      level: data.level || "standard",
      rules: Array.isArray(data.rules) ? data.rules : [],
      defaultPlaceholder: data.defaultPlaceholder || "[REDACTED]",
      preserveLengths: !!data.preserveLengths,
      excludeCategories: Array.isArray(data.excludeCategories) ? data.excludeCategories : [],
    };
    message.success("脱敏策略已保存");
  } catch (err: any) {
    message.error(err?.response?.data?.error || "保存脱敏策略失败");
  } finally {
    saving.value = false;
  }
};

const addRule = () => {
  const rules = policy.value.rules || [];
  rules.push({
    id: `rule-${Date.now()}`,
    description: "自定义规则",
    level: policy.value.level,
    replaceWith: "[REDACTED]",
    enabled: true,
  });
  policy.value.rules = [...rules];
};

const removeRule = (id: string) => {
  policy.value.rules = (policy.value.rules || []).filter((r) => r.id !== id);
};

const toggleRule = (id: string) => {
  const rule = (policy.value.rules || []).find((r) => r.id === id);
  if (rule) rule.enabled = !rule.enabled;
};

onMounted(() => {
  fetchPolicy();
});
</script>

<template>
  <div v-if="loading" style="text-align: center; padding: 48px">
    <LoadingOutlined style="font-size: 32px" />
    <p>加载脱敏策略...</p>
  </div>
  <div v-else>
    <a-row :gutter="[24, 24]">
      <a-col :span="24">
        <a-card title="Redaction Policy" size="small">
          <template #extra><SafetyCertificateOutlined /></template>

          <a-alert
            type="info"
            show-icon
            style="margin-bottom: 16px"
            message="选择全局脱敏等级，并按出口单独控制是否输出脱敏后的内容。"
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
              <a-card size="small" title="Settings">
                <div style="display: flex; flex-direction: column; gap: 12px">
                  <label style="display: flex; align-items: center; gap: 12px">
                    <a-switch v-model:checked="policy.preserveLengths" />
                    <span>保留原始长度（用 * 替代）</span>
                  </label>
                  <label style="display: flex; align-items: center; gap: 12px">
                    <span>默认占位符：</span>
                    <a-input
                      v-model:value="policy.defaultPlaceholder"
                      style="width: 160px"
                      placeholder="[REDACTED]"
                    />
                  </label>
                </div>
              </a-card>
            </a-col>
          </a-row>

          <div style="margin-top: 16px; text-align: right">
            <a-button type="primary" :loading="saving" @click="savePolicy">
              <template #icon><SafetyCertificateOutlined /></template>
              {{ saving ? "保存中..." : "保存策略" }}
            </a-button>
          </div>
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
            message="自定义规则按顺序应用。每条规则需指定 ID、脱敏等级和替换文本。"
          />

          <a-empty v-if="(policy.rules || []).length === 0" description="No custom rules yet">
            <a-button type="primary" @click="addRule">
              <PlusOutlined /> Add First Rule
            </a-button>
          </a-empty>

          <a-space direction="vertical" style="width: 100%" :size="12">
            <a-card
              v-for="rule in policy.rules || []"
              :key="rule.id"
              size="small"
              :hoverable="true"
            >
              <a-row :gutter="[12, 12]" align="middle">
                <a-col :xs="24" :md="4">
                  <a-tag :color="rule.enabled ? 'green' : 'default'">
                    {{ rule.enabled ? "启用" : "禁用" }}
                  </a-tag>
                </a-col>
                <a-col :xs="24" :md="3">
                  <a-tag>{{ rule.level || policy.level }}</a-tag>
                </a-col>
                <a-col :xs="24" :md="5">
                  <a-typography-text ellipsis>{{ rule.id }}</a-typography-text>
                </a-col>
                <a-col :xs="24" :md="5">
                  <a-typography-text type="secondary" ellipsis>
                    {{ rule.description || "—" }}
                  </a-typography-text>
                </a-col>
                <a-col :xs="24" :md="4">
                  <a-typography-text code>{{ rule.replaceWith || "[REDACTED]" }}</a-typography-text>
                </a-col>
                <a-col :xs="24" :md="3" style="text-align: right">
                  <a-button type="link" size="small" @click="toggleRule(rule.id)">
                    {{ rule.enabled ? "禁用" : "启用" }}
                  </a-button>
                  <a-button danger type="text" @click="removeRule(rule.id)">
                    <DeleteOutlined />
                  </a-button>
                </a-col>
              </a-row>
            </a-card>
          </a-space>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>