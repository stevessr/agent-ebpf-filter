<script setup lang="ts">
import { computed } from "vue";
import { CaretDownOutlined, EyeOutlined } from "@ant-design/icons-vue";
import {
  bestTLSPreview,
  classifySSLLib,
  createExpandedTLSRowRender,
  formatBytes,
  sslAttachmentColumns,
  tlsColumns,
} from "./processObserverViewHelpers";
import type {
  ObserverTLSEvent,
  ProcessInfo,
} from "../../composables/monitor/useProcessObserver";

interface AttachedPIDRow {
  pid: number;
  binary_path: string;
  library_name: string;
  comm: string;
}

const props = defineProps<{
  selectedPids: Set<number>;
  visibleTLSEvents: ObserverTLSEvent[];
  treeSSLAttachments: AttachedPIDRow[];
  treeSSLPending: ProcessInfo[];
  attachingPids: Set<number>;
  attachErrors: Record<number, string>;
  autoAttach: boolean;
  skipSSL: boolean;
  openTLSDetail: (event: ObserverTLSEvent) => void;
  clearTLSEvents: () => void | Promise<void>;
  fetchAttachedPIDs: () => void | Promise<void>;
  doAttachBuiltins: (pid: number) => void | Promise<void>;
  doAttachGo: (pid: number) => void | Promise<void>;
  doAttachLibrary: (pid: number, library: string) => void | Promise<void>;
  doAttachAllBuiltins: () => void | Promise<void>;
}>();

const emit = defineEmits<{
  "update:autoAttach": [value: boolean];
  "update:skipSSL": [value: boolean];
}>();

const expandedTLSRowRender = createExpandedTLSRowRender(props.openTLSDetail);
const setAutoAttach = (value: boolean) => emit("update:autoAttach", value);
const setSkipSSL = (value: boolean) => emit("update:skipSSL", value);
</script>

<template>
  <a-empty
    v-if="selectedPids.size === 0"
    description="Select a PID to view SSL/TLS data"
  />
  <template v-else>
    <!-- Section 1: Uprobe captured TLS events -->
    <div class="sub-section">
      <div class="sub-title">
        Decrypted TLS Events
        <span class="sub-count">{{ visibleTLSEvents.length }}</span>
      </div>
      <a-table
        :dataSource="visibleTLSEvents"
        :columns="tlsColumns"
        row-key="key"
        size="small"
        :pagination="{ pageSize: 20, size: 'small' }"
        :expandable="{
          expandedRowRender: (r: ObserverTLSEvent) => expandedTLSRowRender(r),
          rowExpandable: (r: ObserverTLSEvent) =>
            !!(r.body || (r.headers && Object.keys(r.headers).length > 0)),
        }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'evType'">
            <a-tag
              v-if="record.type === 'http_request'"
              color="blue"
              size="small"
              >REQ</a-tag
            >
            <a-tag
              v-else-if="record.type === 'http_response'"
              color="green"
              size="small"
              >RESP</a-tag
            >
            <a-tag
              v-else-if="record.type === 'sse_message'"
              color="purple"
              size="small"
              >SSE</a-tag
            >
            <a-tag v-else color="default" size="small">{{
              record.type || "raw"
            }}</a-tag>
          </template>
          <template
            v-else-if="column.key === 'url' && record.type === 'tls_plaintext'"
          >
            <a-tooltip :title="record.raw_hex_dump?.slice(0, 200)">
              <span class="tls-hex-preview"
                >{{ record.raw_hex_dump?.slice(0, 40) }}…</span
              >
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'bodyPreview'">
            <span
              v-if="record.body || record.raw_hex_dump"
              class="tls-body-preview"
              >{{ bestTLSPreview(record, 80) }}</span
            >
            <span v-else style="color: #6b7280; font-size: 11px">—</span>
          </template>
          <template v-else-if="column.key === 'size'">
            <span>{{ formatBytes(record.captured_len) }}</span>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-button
              v-if="
                record.body ||
                (record.headers && Object.keys(record.headers).length > 0)
              "
              size="small"
              type="link"
              style="padding: 0"
              @click="openTLSDetail(record)"
            >
              <EyeOutlined />
            </a-button>
          </template>
        </template>
      </a-table>
    </div>

    <a-divider style="margin: 16px 0 12px; font-size: 12px; color: #4b5563">
      SSL Probe Attachment
    </a-divider>

    <!-- Section 2: Attached probes -->
    <div class="sub-section">
      <div class="sub-title">
        Active Probes
        <a-tag color="green" size="small">{{
          treeSSLAttachments.length
        }}</a-tag>
      </div>
      <a-empty
        v-if="treeSSLAttachments.length === 0"
        description="No SSL probes attached to tree processes"
        style="padding: 12px"
      />
      <a-table
        v-else
        :dataSource="treeSSLAttachments"
        :columns="sslAttachmentColumns"
        row-key="pid"
        size="small"
        :pagination="false"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'comm'">
            <span class="ssl-attach-comm">{{ record.comm || "—" }}</span>
          </template>
          <template v-else-if="column.key === 'libType'">
            <a-tag
              :color="classifySSLLib(record.library_name || '').tagColor"
              size="small"
            >
              {{ classifySSLLib(record.library_name || "").type }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-badge status="processing" color="green" text="Active" />
          </template>
        </template>
      </a-table>
    </div>

    <!-- Section 3: Pending (not attached) processes -->
    <div v-if="treeSSLPending.length > 0" class="sub-section">
      <div class="sub-title">
        Not Attached
        <a-tag color="default" size="small">{{ treeSSLPending.length }}</a-tag>
      </div>
      <div v-if="Object.keys(attachErrors).length > 0" class="attach-errors">
        <div v-for="(err, pid) in attachErrors" :key="pid" class="attach-error">
          <code>[{{ pid }}]</code> {{ err }}
          <a-button size="small" type="link" @click="delete attachErrors[pid]"
            >Dismiss</a-button
          >
        </div>
      </div>
      <div style="margin-bottom: 8px">
        <a-button
          size="small"
          type="primary"
          ghost
          :loading="attachingPids.size > 0"
          @click="doAttachAllBuiltins()"
          >Attach All ({{ treeSSLPending.length }})</a-button
        >
      </div>
      <div class="ssl-pending-list">
        <div v-for="p in treeSSLPending" :key="p.pid" class="ssl-pending-row">
          <code class="ssl-pending-pid">{{ p.pid }}</code>
          <span class="ssl-pending-name">{{ p.name }}</span>
          <span class="ssl-pending-cmd" v-if="p.cmdline" :title="p.cmdline">
            {{
              p.cmdline.split(/\s+/)[0]?.split("/").pop() ||
              p.cmdline.slice(0, 40)
            }}
          </span>
          <a-dropdown :trigger="['click']" placement="bottomRight">
            <a-button
              size="small"
              type="dashed"
              :loading="attachingPids.has(p.pid)"
              style="margin-left: auto; font-size: 11px"
            >
              Attach <CaretDownOutlined />
            </a-button>
            <template #overlay>
              <a-menu
                @click="
                  ({ key }: { key: string }) => {
                    if (key === 'builtins') doAttachBuiltins(p.pid);
                    else if (key === 'go') doAttachGo(p.pid);
                    else if (key.startsWith('lib:'))
                      doAttachLibrary(p.pid, key.slice(4));
                  }
                "
              >
                <a-menu-item key="builtins">
                  <span class="attach-menu-item"
                    >🔍 Auto-detect (builtins)</span
                  >
                </a-menu-item>
                <a-menu-divider />
                <a-menu-item key="go">
                  <span class="attach-menu-item">🔷 Go crypto/tls</span>
                </a-menu-item>
                <a-menu-item key="lib:openssl">
                  <span class="attach-menu-item">🔒 OpenSSL</span>
                </a-menu-item>
                <a-menu-item key="lib:gnutls">
                  <span class="attach-menu-item">🛡️ GnuTLS</span>
                </a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
        </div>
      </div>
    </div>
  </template>
</template>
