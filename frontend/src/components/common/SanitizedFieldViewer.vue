<script setup lang="ts">
import { computed, ref } from 'vue'

const props = defineProps<{
  value: string
  isSanitized: boolean
  fieldName: string
}>()

const copied = ref(false)
let copyTimer: number | undefined

const displayValue = computed(() => props.value || (props.isSanitized ? '***' : ''))
const placeholderText = computed(() =>
  props.isSanitized ? `${props.fieldName} 已脱敏` : `暂无 ${props.fieldName} 内容`,
)

async function copyValue() {
  if (!displayValue.value) return

  try {
    await navigator.clipboard.writeText(displayValue.value)
    copied.value = true
    window.clearTimeout(copyTimer)
    copyTimer = window.setTimeout(() => {
      copied.value = false
    }, 1200)
  } catch {
    copied.value = false
  }
}
</script>

<template>
  <div class="sanitized-field-viewer">
    <div class="viewer-header">
      <span class="field-name">{{ fieldName }}</span>
      <span v-if="isSanitized" class="sanitized-badge">已脱敏</span>
      <button class="copy-btn" type="button" :disabled="!displayValue" @click="copyValue">
        {{ copied ? '已复制' : '复制' }}
      </button>
    </div>

    <div class="viewer-value" :class="{ sanitized: isSanitized }">
      {{ displayValue || placeholderText }}
    </div>

    <p v-if="isSanitized" class="viewer-hint">此字段已进行脱敏处理，展示的是脱敏后的值。</p>
    <p v-else-if="!value" class="viewer-hint muted">{{ placeholderText }}</p>
  </div>
</template>

<style scoped>
.sanitized-field-viewer {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.75rem;
  border: 1px solid var(--color-border, #e5e7eb);
  border-radius: 0.75rem;
  background: var(--color-surface, #fff);
}

.viewer-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  justify-content: space-between;
}

.field-name {
  font-weight: 600;
  color: var(--color-text, #111827);
}

.sanitized-badge {
  margin-left: auto;
  padding: 0.125rem 0.5rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  color: var(--color-warning-foreground, #92400e);
  background: var(--color-warning, #fef3c7);
}

.copy-btn {
  margin-left: auto;
  padding: 0.35rem 0.75rem;
  border: 1px solid var(--color-border, #d1d5db);
  border-radius: 0.5rem;
  background: var(--color-surface, #fff);
  color: var(--color-text, #111827);
  cursor: pointer;
}

.copy-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.viewer-value {
  padding: 0.65rem 0.75rem;
  border-radius: 0.5rem;
  background: var(--color-muted, #f9fafb);
  color: var(--color-text, #111827);
  word-break: break-word;
  white-space: pre-wrap;
}

.viewer-value.sanitized {
  letter-spacing: 0.02em;
}

.viewer-hint {
  margin: 0;
  font-size: 0.875rem;
  color: var(--color-warning-foreground, #92400e);
}

.viewer-hint.muted {
  color: var(--color-text-muted, #6b7280);
}
</style>
