<script setup lang="ts">
import {
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import type { useConfigRuntime } from "../../../composables/config/useConfigRuntime";

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
}>();

const schemeOptions = [
  { value: "https", label: "https" },
  { value: "http", label: "http" },
];

const {
  runtimeSettings,
  mcpEndpoint,
  persistedEventLogPath,
  persistedEventLogAlive,
  otlpHeaderRows,
  domainForwardRoutes,
  domainForwardStatus,
  loopDetectionStatus,
  researchProcessingStatus,
  saveRuntime,
  rotateAccessToken,
  addOTLPHeaderRow,
  removeOTLPHeaderRow,
  addDomainForwardRoute,
  removeDomainForwardRoute,
  copyText,
  mcpQueryEndpoint,
  mcpQueryEndpointTemplate,
  featureManifest,
} = props.runtime;

const { mergedFeatures, isCompiledIn, featureStatusLabel, featureStatusColor } =
  featureManifest;
</script>

<template>
  <a-card title="TLS Capture" size="small">
    <div style="display: flex; flex-direction: column; gap: 14px">
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch
          v-model:checked="runtimeSettings.tlsCaptureEnabled"
          :disabled="!isCompiledIn('tls_capture')"
        />
        <span
          >Enable TLS plaintext capture (eBPF uprobes on
          OpenSSL/GnuTLS/NSS/Go)</span
        >
        <a-tag :color="featureStatusColor('tls_capture')">
          {{ featureStatusLabel("tls_capture") }}
        </a-tag>
      </div>
      <a-alert
        type="warning"
        show-icon
        message="Backend restart required."
        description="TLS capture hooks plaintext before encryption / after decryption via eBPF uprobes, then parses HTTP/SSE and defaults to sanitized payloads before API, WebSocket, and persistence output."
      />
      <a-alert
        type="info"
        show-icon
        message="AgentSight compatibility"
        description="HTTP messages, SSE chunks, LLM metadata, prompt digests, and redaction counters are emitted through the unified EventEnvelope stream for Dashboard, Execution Graph, metrics, and OTLP export."
      />
      <a-alert
        type="info"
        show-icon
        message="Codex adapter"
        description="Custom Codex builds can POST sanitized request metadata to /codex/capture; this does not depend on TLS uprobes and still uses the same bounded plaintext store and EventEnvelope stream."
      />
      <a-button type="primary" @click="saveRuntime">
        <ReloadOutlined /> Save TLS Capture Setting
      </a-button>
    </div>
  </a-card>
  <a-card title="OpenTelemetry Export" size="small">
    <a-row :gutter="[24, 16]">
      <a-col :xs="24" :lg="10">
        <div style="display: flex; flex-direction: column; gap: 12px">
          <div style="display: flex; align-items: center; gap: 12px">
            <a-switch
              v-model:checked="runtimeSettings.otlpEnabled"
              :disabled="!isCompiledIn('otlp')"
            />
            <span>Enable OTLP trace export</span>
            <a-tag :color="featureStatusColor('otlp')">
              {{ featureStatusLabel("otlp") }}
            </a-tag>
          </div>
          <a-input
            v-model:value="runtimeSettings.otlpEndpoint"
            placeholder="OTLP endpoint, e.g. http://127.0.0.1:4318 or https://collector.example.com/v1/traces"
          />
          <a-input
            v-model:value="runtimeSettings.otlpServiceName"
            placeholder="OTLP service name (defaults to agent-ebpf-filter)"
          />
          <a-typography-text type="secondary">
            OTLP export emits <code>agent.run</code>, <code>codex.task</code>,
            <code>tool.call</code>, <code>mcp.call</code>, process, file,
            network, and policy spans derived from EventEnvelope records.
          </a-typography-text>
          <a-button type="primary" @click="saveRuntime">
            <ReloadOutlined /> Save OTLP Settings
          </a-button>
        </div>
      </a-col>
      <a-col :xs="24" :lg="14">
        <div style="display: flex; flex-direction: column; gap: 10px">
          <div
            style="
              display: flex;
              align-items: center;
              justify-content: space-between;
              gap: 8px;
            "
          >
            <div style="font-weight: 600">OTLP Headers</div>
            <a-button size="small" @click="addOTLPHeaderRow">
              <PlusOutlined /> Add Header
            </a-button>
          </div>
          <a-empty
            v-if="otlpHeaderRows.length === 0"
            description="No custom headers"
            :image="false"
          />
          <div
            v-for="row in otlpHeaderRows"
            :key="row.id"
            style="
              display: grid;
              grid-template-columns:
                minmax(160px, 1fr) minmax(180px, 1fr)
                auto;
              gap: 8px;
              align-items: center;
            "
          >
            <a-input
              v-model:value="row.key"
              placeholder="Header name, e.g. Authorization"
            />
            <a-input v-model:value="row.value" placeholder="Header value" />
            <a-button danger @click="removeOTLPHeaderRow(row.id)">
              <DeleteOutlined />
            </a-button>
          </div>
          <a-typography-text type="secondary">
            Blank rows are ignored. A non-empty value must include a header
            name.
          </a-typography-text>
        </div>
      </a-col>
    </a-row>
  </a-card>
  <a-card title="Domain Forward Proxy (80 / 443)" size="small">
    <a-row :gutter="[24, 16]">
      <a-col :xs="24" :lg="10">
        <div style="display: flex; flex-direction: column; gap: 12px">
          <div style="display: flex; align-items: center; gap: 12px">
            <a-switch
              v-model:checked="runtimeSettings.domainForwardProxy.enabled"
              :disabled="!isCompiledIn('domain_forward')"
            />
            <span>Enable Host/SNI-based HTTP and HTTPS forwarding</span>
            <a-tag :color="featureStatusColor('domain_forward')">
              {{ featureStatusLabel("domain_forward") }}
            </a-tag>
          </div>
          <div style="display: flex; gap: 12px; flex-wrap: wrap">
            <div>
              <div style="margin-bottom: 6px; font-weight: 600">HTTP port</div>
              <a-input-number
                v-model:value="runtimeSettings.domainForwardProxy.httpPort"
                :min="1"
                :max="65535"
                style="width: 140px"
              />
            </div>
            <div>
              <div style="margin-bottom: 6px; font-weight: 600">HTTPS port</div>
              <a-input-number
                v-model:value="runtimeSettings.domainForwardProxy.httpsPort"
                :min="1"
                :max="65535"
                style="width: 140px"
              />
            </div>
            <div>
              <div style="margin-bottom: 6px; font-weight: 600">
                Default upstream scheme
              </div>
              <a-select
                v-model:value="runtimeSettings.domainForwardProxy.defaultScheme"
                style="width: 140px"
                :options="schemeOptions"
              />
            </div>
            <div>
              <div style="margin-bottom: 6px; font-weight: 600">
                Dial timeout
              </div>
              <a-input-number
                v-model:value="
                  runtimeSettings.domainForwardProxy.dialTimeoutSeconds
                "
                :min="1"
                :max="120"
                style="width: 140px"
              />
            </div>
          </div>
          <div style="display: flex; align-items: center; gap: 12px">
            <a-switch
              v-model:checked="runtimeSettings.domainForwardProxy.allowAnyHost"
            />
            <span>Allow any Host header and forward to the same domain</span>
          </div>
          <a-input
            v-model:value="runtimeSettings.domainForwardProxy.dnsResolver"
            placeholder="Optional DNS resolver override, e.g. 1.1.1.1:53"
          />
          <a-input
            v-model:value="runtimeSettings.domainForwardProxy.certFile"
            placeholder="Default TLS certificate path for :443 (PEM)"
          />
          <a-input
            v-model:value="runtimeSettings.domainForwardProxy.keyFile"
            placeholder="Default TLS private key path for :443 (PEM)"
          />
          <a-alert
            type="warning"
            show-icon
            message="Binding 80/443 requires root or CAP_NET_BIND_SERVICE. HTTPS forwarding requires certificate files."
            description="If test domains resolve back to this box, set a DNS resolver override or explicit upstreams to avoid forwarding loops."
          />
          <div
            style="
              display: flex;
              gap: 8px;
              flex-wrap: wrap;
              align-items: center;
            "
          >
            <a-button type="primary" @click="saveRuntime">
              <ReloadOutlined /> Save & Apply Forwarding
            </a-button>
            <a-tag
              :color="domainForwardStatus.httpRunning ? 'green' : 'default'"
            >
              HTTP
              {{ domainForwardStatus.httpRunning ? "running" : "stopped" }}
            </a-tag>
            <a-tag
              :color="domainForwardStatus.httpsRunning ? 'green' : 'default'"
            >
              HTTPS
              {{ domainForwardStatus.httpsRunning ? "running" : "stopped" }}
            </a-tag>
          </div>
        </div>
      </a-col>
      <a-col :xs="24" :lg="14">
        <div style="display: flex; flex-direction: column; gap: 12px">
          <div
            style="
              display: flex;
              align-items: center;
              justify-content: space-between;
              gap: 8px;
            "
          >
            <div style="font-weight: 600">Visual route overrides</div>
            <a-button size="small" @click="addDomainForwardRoute">
              <PlusOutlined /> Add Route
            </a-button>
          </div>
          <a-empty
            v-if="domainForwardRoutes.length === 0"
            description="No route overrides"
            :image="false"
          />
          <a-card
            v-for="(route, index) in domainForwardRoutes"
            :key="route.id"
            size="small"
            :title="`Route #${index + 1}`"
          >
            <template #extra>
              <a-button
                size="small"
                danger
                @click="removeDomainForwardRoute(route.id)"
              >
                <DeleteOutlined /> Remove
              </a-button>
            </template>
            <a-row :gutter="[12, 12]">
              <a-col :xs="24" :md="12">
                <a-input
                  v-model:value="route.host"
                  placeholder="Host, e.g. example.com or *.lab.test"
                />
              </a-col>
              <a-col :xs="24" :md="12">
                <a-input
                  v-model:value="route.upstream"
                  placeholder="Upstream, e.g. https://{host}"
                />
              </a-col>
              <a-col :xs="24" :md="12">
                <a-input
                  v-model:value="route.certFile"
                  placeholder="Route certificate path (optional)"
                />
              </a-col>
              <a-col :xs="24" :md="12">
                <a-input
                  v-model:value="route.keyFile"
                  placeholder="Route private key path (optional)"
                />
              </a-col>
            </a-row>
          </a-card>
          <a-typography-text type="secondary">
            Empty upstreams or <code>allowAnyHost</code> forward to
            <code>&lt;scheme&gt;://&lt;request-host&gt;</code>. Wildcards
            support <code>*.example.com</code>; <code>{host}</code> expands to
            the normalized request host.
          </a-typography-text>
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-tag :color="domainForwardStatus.enabled ? 'blue' : 'default'">
              {{ domainForwardStatus.enabled ? "enabled" : "disabled" }}
            </a-tag>
            <a-tag color="blue"
              >routes: {{ domainForwardStatus.routeCount }}</a-tag
            >
            <a-tag v-if="domainForwardStatus.httpAddress" color="green">
              {{ domainForwardStatus.httpAddress }}
            </a-tag>
            <a-tag v-if="domainForwardStatus.httpsAddress" color="green">
              {{ domainForwardStatus.httpsAddress }}
            </a-tag>
            <a-tag v-if="domainForwardStatus.dnsResolver" color="purple">
              DNS {{ domainForwardStatus.dnsResolver }}
            </a-tag>
          </div>
          <a-alert
            v-if="
              domainForwardStatus.errors &&
              domainForwardStatus.errors.length > 0
            "
            type="error"
            show-icon
            :message="domainForwardStatus.errors.join('; ')"
          />
        </div>
      </a-col>
    </a-row>
  </a-card>
</template>
