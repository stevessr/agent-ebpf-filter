<script setup lang="ts">
defineProps<{ checked: boolean; disabled?: boolean; label: string }>();
const emit = defineEmits<{ "update:checked": [value: boolean] }>();
</script>
<template>
  <button
    type="button"
    role="switch"
    class="renew-switch"
    :class="{ 'is-checked': checked }"
    :aria-checked="checked"
    :aria-label="label"
    :disabled="disabled"
    @click="emit('update:checked', !checked)"
  >
    <span class="renew-switch__thumb" />
    <span class="renew-switch__state">{{ checked ? "开启" : "关闭" }}</span>
  </button>
</template>
