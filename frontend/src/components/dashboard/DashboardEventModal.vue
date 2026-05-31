<script setup lang="ts">
import { EyeOutlined, FolderOpenOutlined } from "@ant-design/icons-vue";

defineProps<{
  showDetails: boolean;
  selectedEvent: any;
  selectedTraceSummary: string;
  getTagColor: (eventType?: number, type?: string) => string;
  getCategoryColor: (tag: string) => string;
  formatDetailValue: (value: any) => string;
  canInteractWithPath: (record: any) => boolean;
  previewRecordPath: (record: any) => void;
  openInExplorer: (record: any) => void;
}>();

const emit = defineEmits<{
  (e: "update:showDetails", value: boolean): void;
}>();
</script>

<template>
  <a-modal
    :open="showDetails"
    title="Event Details"
    :footer="null"
    width="600px"
    @update:open="emit('update:showDetails', $event)"
  >
    <a-descriptions bordered :column="1" size="small" v-if="selectedEvent">
      <a-descriptions-item label="Time">{{
        selectedEvent.time
      }}</a-descriptions-item>
      <a-descriptions-item v-if="selectedTraceSummary" label="Trace Summary">
        <a-typography-text code style="word-break: break-all">{{
          selectedTraceSummary
        }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item label="Event Type">
        <a-tag
          :color="getTagColor(selectedEvent.eventType, selectedEvent.type)"
          >{{ selectedEvent.type.toUpperCase() }}</a-tag
        >
      </a-descriptions-item>
      <a-descriptions-item label="Tag">
        <a-tag :color="getCategoryColor(selectedEvent.tag)">{{
          selectedEvent.tag
        }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item label="Command"
        ><a-typography-text strong>{{
          selectedEvent.comm
        }}</a-typography-text></a-descriptions-item
      >
      <a-descriptions-item label="PID"
        ><code>{{
          formatDetailValue(selectedEvent.pid)
        }}</code></a-descriptions-item
      >
      <a-descriptions-item label="Parent PID (PPID)"
        ><code>{{
          formatDetailValue(selectedEvent.ppid)
        }}</code></a-descriptions-item
      >
      <a-descriptions-item label="User ID (UID)"
        ><code>{{
          formatDetailValue(selectedEvent.uid)
        }}</code></a-descriptions-item
      >
      <a-descriptions-item
        v-if="selectedEvent.netDirection"
        label="Network Direction"
      >
        <a-tag color="blue">{{ selectedEvent.netDirection }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.netEndpoint"
        label="Network Endpoint"
      >
        <a-typography-text code style="word-break: break-all">{{
          selectedEvent.netEndpoint
        }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.netFamily"
        label="Network Family"
      >
        <a-tag color="purple">{{ selectedEvent.netFamily }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.netBytes !== undefined"
        label="Network Bytes"
      >
        <a-typography-text code>{{ selectedEvent.netBytes }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.retval !== undefined"
        label="Return Value"
      >
        <a-typography-text
          :type="selectedEvent.retval < 0 ? 'danger' : undefined"
          code
          >{{ selectedEvent.retval }}</a-typography-text
        >
      </a-descriptions-item>
      <a-descriptions-item
        v-if="(selectedEvent.occurrenceCount ?? 1) > 1"
        label="Occurrences"
      >
        <a-tag color="blue">×{{ selectedEvent.occurrenceCount ?? 1 }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item v-if="selectedEvent.extraInfo" label="Extra Info">
        <a-typography-text code>{{
          selectedEvent.extraInfo
        }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item v-if="selectedEvent.extraPath" label="Extra Path">
        <code style="word-break: break-all">{{ selectedEvent.extraPath }}</code>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.bytes !== undefined"
        label="Bytes"
      >
        <a-typography-text code>{{ selectedEvent.bytes }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item v-if="selectedEvent.mode" label="Mode">
        <a-typography-text code>{{ selectedEvent.mode }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item v-if="selectedEvent.domain" label="Domain">
        <a-tag>{{ selectedEvent.domain }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item v-if="selectedEvent.sockType" label="Socket Type">
        <a-tag>{{ selectedEvent.sockType }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.protocol !== undefined"
        label="Protocol"
      >
        <a-typography-text code>{{ selectedEvent.protocol }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.uidArg !== undefined"
        label="Chown UID"
      >
        <a-typography-text code>{{ selectedEvent.uidArg }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item
        v-if="selectedEvent.gidArg !== undefined"
        label="Chown GID"
      >
        <a-typography-text code>{{ selectedEvent.gidArg }}</a-typography-text>
      </a-descriptions-item>
      <a-descriptions-item label="Resource Path / Info">
        <div
          style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap"
        >
          <code style="word-break: break-all">{{
            formatDetailValue(selectedEvent.path)
          }}</code>
          <a-button
            v-if="canInteractWithPath(selectedEvent)"
            type="link"
            size="small"
            @click="previewRecordPath(selectedEvent)"
          >
            <template #icon><EyeOutlined /></template>
            Preview
          </a-button>
          <a-button
            v-if="canInteractWithPath(selectedEvent)"
            type="link"
            size="small"
            @click="openInExplorer(selectedEvent)"
          >
            <template #icon><FolderOpenOutlined /></template>
            Open in Explorer
          </a-button>
        </div>
      </a-descriptions-item>
    </a-descriptions>
  </a-modal>
</template>
