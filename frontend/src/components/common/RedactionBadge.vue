<script setup lang="ts">
import { computed } from "vue";
import {
  SafetyOutlined,
  LockOutlined,
  EyeOutlined,
} from "@ant-design/icons-vue";

const props = withDefaults(
  defineProps<{
    level: string | number;
    size?: "small" | "large";
    showIcon?: boolean;
  }>(),
  {
    size: "small",
    showIcon: true,
  },
);

const levelLabelMap: Record<string, string> = {
  none: "未脱敏",
  low: "低脱敏",
  medium: "中脱敏",
  high: "高脱敏",
  strict: "严格脱敏",
};

const colorMap: Record<string, string> = {
  none: "default",
  low: "blue",
  medium: "gold",
  high: "orange",
  strict: "red",
};

const iconMap: Record<string, any> = {
  none: EyeOutlined,
  low: SafetyOutlined,
  medium: LockOutlined,
  high: LockOutlined,
  strict: LockOutlined,
};

const normalizedLevel = computed(() => String(props.level).toLowerCase());
const levelLabel = computed(
  () => levelLabelMap[normalizedLevel.value] ?? String(props.level),
);
const tagColor = computed(
  () => colorMap[normalizedLevel.value] ?? "default",
);
const iconComponent = computed(
  () => iconMap[normalizedLevel.value] ?? SafetyOutlined,
);
const isLarge = computed(() => props.size === "large");
</script>

<template>
  <a-tag :color="tagColor" :style="isLarge ? largeStyle : smallStyle">
    <template v-if="showIcon" #icon>
      <component :is="iconComponent" />
    </template>
    {{ levelLabel }}
  </a-tag>
</template>

<script lang="ts">
const smallStyle = {
  borderRadius: "999px",
  fontSize: "12px",
  lineHeight: "20px",
  padding: "0 8px",
  margin: 0,
};

const largeStyle = {
  borderRadius: "999px",
  fontSize: "14px",
  lineHeight: "28px",
  padding: "0 12px",
  margin: 0,
  fontWeight: 600,
};
</script>
