<script setup lang="ts">
import { ReloadOutlined, SafetyCertificateOutlined, CopyOutlined, DeleteOutlined } from '@ant-design/icons-vue';
import type { useConfigRuntime } from '../../composables/useConfigRuntime';

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
}>();

const {
  runtimeSettings, mcpEndpoint,
  persistedEventLogPath, persistedEventLogAlive,
  collectorHealth, otelHealth, otlpHeadersText,
  saveRuntime, rotateAccessToken, clearInMemoryEvents, clearPersistedLog, clearAllEvents,
  copyText, mcpQueryEndpoint, mcpQueryEndpointTemplate,
} = props.runtime;

const formatLatencyMs = (value?: number) => {
  if (!value) return '0 ms';
  return `${(value / 1_000_000).toFixed(value >= 1_000_000 ? 2 : 3)} ms`;
};

const formatMaybeDate = (value?: string) => {
  if (!value) return 'Not exported yet';
  return value;
};
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-card title="Runtime & MCP Access" size="small">
        <template #extra>
          <SafetyCertificateOutlined />
        </template>
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.logPersistenceEnabled" />
                <span>Persist captured logs to file</span>
              </div>
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.shellSessionsEnabled" />
                <span>Enable PTY / shell sessions</span>
              </div>
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.systemRunEnabled" />
                <span>Enable /system/run command launch</span>
              </div>
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.hookManagementEnabled" />
                <span>Enable hook injection / config editing</span>
              </div>
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.policyManagementEnabled" />
                <span>Enable policy mutations (tags / comms / paths / rules)</span>
              </div>
              <a-input v-model:value="runtimeSettings.logFilePath"
                placeholder="Log file path (defaults to ~/.config/agent-ebpf-filter/events.jsonl)" />
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.otlpEnabled" />
                <span>Enable OTLP trace export</span>
              </div>
              <a-input
                v-model:value="runtimeSettings.otlpEndpoint"
                placeholder="OTLP endpoint, e.g. http://127.0.0.1:4318 or https://collector.example.com/v1/traces"
              />
              <a-input
                v-model:value="runtimeSettings.otlpServiceName"
                placeholder="OTLP service name (defaults to agent-ebpf-filter)"
              />
              <a-textarea
                v-model:value="otlpHeadersText"
                :rows="4"
                placeholder='OTLP headers JSON, e.g. { "Authorization": "Bearer ..." }'
              />
              <div style="display: flex; gap: 8px; flex-wrap: wrap; align-items: center">
                <a-button type="primary" @click="saveRuntime">
                  <ReloadOutlined /> Save Runtime
                </a-button>
                <a-tag :color="persistedEventLogAlive ? 'green' : 'red'">
                  {{ persistedEventLogAlive ? 'Log file ready' : 'Log file inactive' }}
                </a-tag>
                <a-tag color="blue">{{ persistedEventLogPath || 'No log path' }}</a-tag>
              </div>
              <a-typography-text type="secondary">
                When enabled, new events are appended as JSONL and can be exported or tailed through MCP.
              </a-typography-text>
              <a-typography-text type="secondary">
                OTLP export emits <code>agent.run</code>, <code>codex.task</code>, <code>tool.call</code>, <code>mcp.call</code>, process, file, network, and policy spans derived from the normalized EventEnvelope stream.
              </a-typography-text>
              <a-alert
                type="warning"
                show-icon
                message="PTY sessions, /system/run, hook injection, and policy mutations are high-risk features and stay disabled until you explicitly enable them."
              />
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div>
                <div style="margin-bottom: 6px; font-weight: 600">Access Token</div>
                <a-input :value="runtimeSettings.accessToken" readonly
                  placeholder="Generate a token to access /config and /mcp" />
                <div style="display: flex; gap: 8px; flex-wrap: wrap; margin-top: 8px">
                  <a-button @click="rotateAccessToken">
                    <ReloadOutlined /> Generate / Rotate
                  </a-button>
                  <a-button @click="copyText(runtimeSettings.accessToken, 'Access token copied')">
                    <CopyOutlined /> Copy Token
                  </a-button>
                </div>
              </div>
              <div style="display: flex; flex-direction: column; gap: 8px">
                <div style="margin-bottom: 2px; font-weight: 600">MCP Endpoint</div>
                <a-input :value="mcpEndpoint" readonly />
                <div style="display: flex; gap: 8px; flex-wrap: wrap">
                  <a-button @click="copyText(mcpEndpoint, 'MCP endpoint copied')">
                    <CopyOutlined /> Copy Base URL
                  </a-button>
                </div>
                <div style="margin-top: 4px; font-weight: 600">MCP Query URL</div>
                <a-input :value="mcpQueryEndpoint" readonly />
                <div style="display: flex; gap: 8px; flex-wrap: wrap">
                  <a-button @click="copyText(mcpQueryEndpoint, 'MCP query URL copied')">
                    <CopyOutlined /> Copy Query URL
                  </a-button>
                  <a-button @click="copyText(mcpQueryEndpointTemplate, 'MCP query template copied')">
                    <CopyOutlined /> Copy Template
                  </a-button>
                </div>
                <a-alert type="success" show-icon
                  :message="'Query URL is generated live from the current token and updates when you rotate it.'"
                  style="margin-top: 4px" />
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Collection Health" size="small">
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag :color="collectorHealth.captureHealthy ? 'green' : 'red'">
                  {{ collectorHealth.captureHealthy ? 'Capture healthy' : 'Ringbuf drops detected' }}
                </a-tag>
                <a-tag :color="collectorHealth.collectorMapAvailable ? 'blue' : 'default'">
                  {{ collectorHealth.collectorMapAvailable ? 'collector_stats available' : 'collector_stats unavailable' }}
                </a-tag>
              </div>
              <div>ringbuf events: <strong>{{ collectorHealth.ringbufEventsTotal }}</strong></div>
              <div>ringbuf reserve failed: <strong>{{ collectorHealth.ringbufReserveFailedTotal }}</strong></div>
              <div>backend queue len: <strong>{{ collectorHealth.backendQueueLen }}</strong></div>
              <div>WS clients: <strong>{{ collectorHealth.wsClients }}</strong></div>
              <div>last persisted append latency: <strong>{{ formatLatencyMs(collectorHealth.persistAppendLatencyNs) }}</strong></div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 8px">
              <div style="font-weight: 600">Events by type</div>
              <a-empty v-if="Object.keys(collectorHealth.eventsByTypeTotal || {}).length === 0" description="No events yet" :image="false" />
              <div v-else style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag v-for="(count, type) in collectorHealth.eventsByTypeTotal" :key="type" color="default">
                  {{ type }}: {{ count }}
                </a-tag>
              </div>
              <a-alert
                v-if="collectorHealth.ringbufDroppedTotal > 0"
                type="warning"
                show-icon
                message="Sampling may be incomplete because the kernel ringbuf dropped events."
              />
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="OpenTelemetry Export" size="small">
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag :color="otelHealth.ready ? 'green' : (otelHealth.enabled ? 'orange' : 'default')">
                  {{ otelHealth.ready ? 'OTLP ready' : (otelHealth.enabled ? 'OTLP waiting / error' : 'OTLP disabled') }}
                </a-tag>
                <a-tag color="blue">{{ otelHealth.serviceName || runtimeSettings.otlpServiceName || 'agent-ebpf-filter' }}</a-tag>
              </div>
              <div>endpoint: <strong>{{ otelHealth.endpoint || runtimeSettings.otlpEndpoint || 'not configured' }}</strong></div>
              <div>queue len: <strong>{{ otelHealth.queueLen }}</strong></div>
              <div>exported spans: <strong>{{ otelHealth.exportedSpans }}</strong></div>
              <div>dropped events: <strong>{{ otelHealth.droppedEvents }}</strong></div>
              <div>last export: <strong>{{ formatMaybeDate(otelHealth.lastExportedAt) }}</strong></div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div>active run spans: <strong>{{ otelHealth.activeRunSpans }}</strong></div>
              <div>active task spans: <strong>{{ otelHealth.activeTaskSpans }}</strong></div>
              <div>active tool spans: <strong>{{ otelHealth.activeToolSpans }}</strong></div>
              <a-alert
                v-if="otelHealth.lastError"
                type="warning"
                show-icon
                :message="otelHealth.lastError"
              />
              <a-typography-text v-else type="secondary">
                Export health comes from <code>/system/otel-health</code>. If OTLP is enabled but not ready, check the endpoint, headers JSON, and collector reachability.
              </a-typography-text>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Data Management" size="small">
        <template #extra>
          <DeleteOutlined />
        </template>
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div style="font-weight: 600">Event Retention</div>
              <div style="display: flex; align-items: center; gap: 12px">
                <span>Max in-memory events:</span>
                <a-input-number v-model:value="runtimeSettings.maxEventCount" :min="100" :max="10000" :step="100"
                  style="width: 140px" />
              </div>
              <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap">
                <span>Max event age:</span>
                <a-input v-model:value="runtimeSettings.maxEventAge" placeholder="e.g. 24h, 168h, 0 = no limit"
                  style="width: 200px" />
                <a-typography-text type="secondary">Go duration format (24h, 30m, 168h)</a-typography-text>
              </div>
              <div style="display: flex; gap: 8px; flex-wrap: wrap; align-items: center">
                <a-button type="primary" @click="saveRuntime">
                  <ReloadOutlined /> Save Retention
                </a-button>
              </div>
            </div>
          </a-col>
          <a-col :xs="24" :md="12">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div style="font-weight: 600">Manual Cleanup</div>
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-popconfirm title="Clear in-memory event buffer?" @confirm="clearInMemoryEvents">
                  <a-button size="small" danger>Clear Memory Events</a-button>
                </a-popconfirm>
                <a-popconfirm title="Truncate persisted event log file?" @confirm="clearPersistedLog">
                  <a-button size="small" danger>Truncate Log File</a-button>
                </a-popconfirm>
                <a-popconfirm title="Clear all events (memory + file)?" @confirm="clearAllEvents">
                  <a-button size="small" type="primary" danger>Clear All Events</a-button>
                </a-popconfirm>
              </div>
              <a-typography-text type="secondary">
                These actions are irreversible. Memory events and/or the JSONL log file will be permanently deleted.
              </a-typography-text>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>
  </a-row>
</template>
