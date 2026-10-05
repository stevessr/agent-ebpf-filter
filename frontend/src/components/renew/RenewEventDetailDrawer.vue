<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  open: boolean;
  loading: boolean;
  eventId: string;
  detail: Record<string, unknown> | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

const prettyDetail = computed(() =>
  props.detail ? JSON.stringify(props.detail, null, 2) : "",
);
</script>

<template>
  <a-drawer
    :open="open"
    width="min(680px, 92vw)"
    title="完整事件"
    placement="right"
    @close="emit('close')"
  >
    <div class="renew-detail-meta">
      <span>Event ID</span>
      <code>{{ eventId || "—" }}</code>
    </div>
    <a-spin :spinning="loading">
      <div v-if="detail" class="renew-detail-body">
        <p>
          完整内容按需从后端本地事件库读取；关闭此面板后 Renew 不再保留这份详情。
        </p>
        <pre>{{ prettyDetail }}</pre>
      </div>
      <a-empty v-else description="选择一条事件查看完整内容" />
    </a-spin>
  </a-drawer>
</template>