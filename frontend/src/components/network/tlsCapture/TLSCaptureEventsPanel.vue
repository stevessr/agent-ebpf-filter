<script setup lang="ts">
import { computed } from "vue";
import {
  CloseCircleOutlined,
  CopyOutlined,
  PauseOutlined,
  PlayCircleOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
} from "@ant-design/icons-vue";

import type {
  TLSLibraryStatus,
  TLSPlaintextEvent,
} from "../../../types/tls";
import { TLS_DIRECTION_OPTIONS } from "../../../views/network/tlsCapture/constants";
import type { TLSCaptureSummaryStats } from "../../../views/network/tlsCapture/types";
import {
  formatTLSBytes,
  formatTLSTimestamp,
  tlsDirectionColor,
  tlsDirectionLabel,
  tlsPacketTypeColor,
  tlsPacketTypeLabel,
} from "../../../views/network/tlsCapture/utils";

const props = defineProps<{
  summaryStats: TLSCaptureSummaryStats;
  eventsCount: number;
  filteredEvents: TLSPlaintextEvent[];
  libraries: TLSLibraryStatus[];
}>();

const isPaused = defineModel<boolean>("isPaused", { required: true });
const searchQuery = defineModel<string>("searchQuery", { required: true });
const commFilter = defineModel<string>("commFilter", { required: true });
const hostFilter = defineModel<string>("hostFilter", { required: true });
const selectedLib = defineModel<string>("selectedLib", { required: true });
const selectedDirection = defineModel<string>("selectedDirection", {
  required: true,
});
const sslFilterExpr = defineModel<string>("sslFilterExpr", { required: true });
const ignoreFilter = defineModel<string>("ignoreFilter", { required: true });

const emit = defineEmits<{
  clearFilters: [];
  openDetails: [event: TLSPlaintextEvent];
  copyBody: [event: TLSPlaintextEvent];
}>();

const libraryOptions = computed(() => [
  { label: "All libraries", value: "all" },
  ...props.libraries.map((library) => ({
    label: library.name,
    value: library.name,
  })),
]);

const formatTimestamp = formatTLSTimestamp;
const formatBytes = formatTLSBytes;
const directionColor = tlsDirectionColor;
const directionLabel = tlsDirectionLabel;
const packetTypeColor = tlsPacketTypeColor;
const packetTypeLabel = tlsPacketTypeLabel;
</script>

<template>
  <a-row :gutter="16" class="tls-stats">
        <a-col :xs="12" :sm="6">
          <a-statistic title="Total" :value="summaryStats.total" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Requests" :value="summaryStats.sends" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Responses" :value="summaryStats.recvs" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic
            title="Attached Libraries"
            :value="summaryStats.attachedLibs"
          />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="HTTP" :value="summaryStats.http" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="SSE" :value="summaryStats.sse" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="LLM Metadata" :value="summaryStats.llm" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Sanitized" :value="summaryStats.redacted" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Handshake" :value="summaryStats.handshakes" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="HTTP Req" :value="summaryStats.httpRequests" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="JSON" :value="summaryStats.jsonData" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="SSE" :value="summaryStats.sseData" />
        </a-col>
      </a-row>

      <a-space wrap class="tls-toolbar">
        <a-button
          @click="isPaused = !isPaused"
          :type="isPaused ? 'primary' : 'default'"
          danger
          size="small"
        >
          <template #icon
            ><PauseOutlined v-if="isPaused" /><PlayCircleOutlined v-else
          /></template>
          {{ isPaused ? "Resume" : "Pause" }}
        </a-button>
        <a-input
          v-model:value="searchQuery"
          size="small"
          placeholder="Search URL, headers, body"
          allow-clear
          style="width: 220px"
        >
          <template #prefix><SearchOutlined /></template>
        </a-input>
        <a-input
          v-model:value="commFilter"
          size="small"
          placeholder="Command filter"
          allow-clear
          style="width: 180px"
        />
        <a-input
          v-model:value="hostFilter"
          size="small"
          placeholder="Host filter"
          allow-clear
          style="width: 180px"
        />
        <a-select
          v-model:value="selectedLib"
          size="small"
          style="width: 160px"
          :options="libraryOptions"
        />
        <a-select
          v-model:value="selectedDirection"
          size="small"
          style="width: 120px"
          :options="TLS_DIRECTION_OPTIONS"
        />
        <a-input
          v-model:value="sslFilterExpr"
          size="small"
          placeholder="SSL filter: len>100&data_type=http_request"
          allow-clear
          style="width: 280px"
        >
          <template #prefix><SafetyCertificateOutlined /></template>
        </a-input>
        <a-input
          v-model:value="ignoreFilter"
          size="small"
          placeholder="Ignore: comm,host,url (反向排除)"
          allow-clear
          style="width: 240px"
        >
          <template #prefix><CloseCircleOutlined /></template>
        </a-input>
        <a-badge
          v-if="ignoreFilter.trim()"
          :count="`Ignore: ${ignoreFilter.split(',').filter(Boolean).length}`"
          :overflow-count="99"
          size="small"
        >
          <a-tag color="red">Active</a-tag>
        </a-badge>
        <a-button size="small" @click="emit('clearFilters')"
          >Clear Filters</a-button
        >
      </a-space>

      <a-empty
        v-if="eventsCount === 0"
        description="暂无完整 HTTP 请求/返回包 — 请确保后端已启动且 eBPF TLS 探针已挂载"
      />
      <a-empty
        v-else-if="filteredEvents.length === 0"
        description="无匹配请求/返回包，请调整过滤条件"
      />

      <a-table
        :data-source="filteredEvents"
        row-key="key"
        size="small"
        :pagination="{ pageSize: 20, showSizeChanger: true }"
        :scroll="{ x: 1200 }"
      >
        <a-table-column
          title="Time"
          data-index="timestamp"
          key="timestamp"
          width="180"
        >
          <template #default="{ text }">{{ formatTimestamp(text) }}</template>
        </a-table-column>
        <a-table-column
          title="Packet"
          data-index="direction"
          key="direction"
          width="110"
        >
          <template #default="{ text }">
            <a-tag :color="directionColor(text)">{{
              directionLabel(text)
            }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column
          title="Library"
          data-index="lib"
          key="lib"
          width="120"
        />
        <a-table-column
          title="Command"
          data-index="comm"
          key="comm"
          width="140"
          ellipsis
        />
        <a-table-column
          title="Host"
          data-index="host"
          key="host"
          width="180"
          ellipsis
        />
        <a-table-column title="Type" key="type" width="140">
          <template #default="{ record }">
            <a-tag :color="packetTypeColor(record)">{{
              packetTypeLabel(record)
            }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column
          title="Method"
          data-index="method"
          key="method"
          width="90"
        />
        <a-table-column
          title="Status"
          data-index="status"
          key="status"
          width="90"
        />
        <a-table-column title="URL" data-index="url" key="url" ellipsis />
        <a-table-column
          title="Redaction"
          data-index="redaction_state"
          key="redaction_state"
          width="110"
        >
          <template #default="{ text }">
            <a-tag :color="text === 'sanitized' ? 'green' : 'default'">{{
              text || "raw"
            }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column
          title="Body Size"
          data-index="body_size"
          key="body_size"
          width="110"
          align="right"
        >
          <template #default="{ text }">{{ formatBytes(text) }}</template>
        </a-table-column>
        <a-table-column title="" key="action" width="160" fixed="right">
          <template #default="{ record }">
            <a-space :size="4">
              <a-button
                type="link"
                size="small"
                @click="emit('openDetails', record)"
                >Detail</a-button
              >
              <a-button
                type="link"
                size="small"
                @click="emit('copyBody', record)"
              >
                <template #icon><CopyOutlined /></template>
              </a-button>
            </a-space>
          </template>
        </a-table-column>
  </a-table>
</template>
