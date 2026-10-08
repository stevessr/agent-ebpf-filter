<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { RobotOutlined } from "@ant-design/icons-vue";
import { resolveHarnessIcon } from "../../utils/harnessIcons";

const props = withDefaults(defineProps<{
  harness?: string | null;
  command?: string | null;
  size?: number;
  fallback?: boolean;
}>(), {
  size: 20,
  fallback: false,
});

const key = computed(() => resolveHarnessIcon(props.harness, props.command));
const failed = ref(false);
watch(key, () => { failed.value = false; });
const iconUrl = computed(() =>
  key.value ? `${import.meta.env.BASE_URL}brand-icons/${key.value}.svg` : "",
);
</script>

<template>
  <span
    v-if="(key && !failed) || fallback"
    class="harness-icon"
    :style="{ width: `${size}px`, height: `${size}px`, fontSize: `${size}px` }"
    aria-hidden="true"
  >
    <img
      v-if="key && !failed"
      :key="key"
      :src="iconUrl"
      alt=""
      :width="size"
      :height="size"
      loading="lazy"
      @error="failed = true"
    />
    <RobotOutlined v-else />
  </span>
</template>

<style scoped>
.harness-icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  vertical-align: middle;
  line-height: 1;
}
.harness-icon img { display: block; width: 100%; height: 100%; object-fit: contain; }
</style>
