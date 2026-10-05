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

const rewriteDirectionOptions = [
  { value: "both", label: "Both" },
  { value: "request", label: "Request" },
  { value: "response", label: "Response" },
];

const addModelRewriteRule = () => {
  runtimeSettings.value.domainForwardProxy.rewrite.modelRules.push({
    host: "",
    from: "",
    to: "",
  });
};

const removeModelRewriteRule = (index: number) => {
  runtimeSettings.value.domainForwardProxy.rewrite.modelRules.splice(index, 1);
};

const addBodyRewriteRule = () => {
  runtimeSettings.value.domainForwardProxy.rewrite.rules.push({
    id: `rewrite-${Date.now()}`,
    enabled: true,
    direction: "both",
    host: "",
    pathPrefix: "",
    contentType: "application/json",
    find: "",
    replace: "",
  });
};

const removeBodyRewriteRule = (index: number) => {
  runtimeSettings.value.domainForwardProxy.rewrite.rules.splice(index, 1);
};

type DomainForwardRouteField = "host" | "upstream" | "certFile" | "keyFile";

const domainForwardRouteValue = (
  index: number,
  field: DomainForwardRouteField,
) => domainForwardRoutes.value[index]?.[field] || "";

const updateDomainForwardRouteField = (
  index: number,
  field: DomainForwardRouteField,
  value: string,
) => {
  const target = domainForwardRoutes.value[index];
  if (!target) return;
  target[field] = value;
};
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

          <a-card title="Allowlisted TLS inspection" size="small">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch
                  v-model:checked="
                    runtimeSettings.domainForwardProxy.tlsInterceptEnabled
                  "
                />
                <span>Issue per-SNI leaf certificates only for allowlisted hosts</span>
              </div>
              <a-textarea
                v-model:value="
                  runtimeSettings.domainForwardProxy.tlsInterceptAllowlist
                "
                :rows="3"
                placeholder="api.openai.com, *.example.internal"
              />
              <a-input
                v-model:value="
                  runtimeSettings.domainForwardProxy.tlsInterceptCaCertFile
                "
                placeholder="Local CA certificate path (PEM)"
              />
              <a-input
                v-model:value="
                  runtimeSettings.domainForwardProxy.tlsInterceptCaKeyFile
                "
                placeholder="Local CA private key path (PEM)"
              />
              <div>
                <div style="margin-bottom: 6px; font-weight: 600">
                  Leaf certificate TTL (seconds)
                </div>
                <a-input-number
                  v-model:value="
                    runtimeSettings.domainForwardProxy
                      .tlsInterceptLeafTtlSeconds
                  "
                  :min="300"
                  :max="604800"
                  style="width: 180px"
                />
              </div>
              <a-alert
                type="warning"
                show-icon
                message="Strict allowlist"
                description="Dynamic certificates are issued only for exact or *.suffix entries above. Clients must explicitly trust this local CA. A host outside the allowlist is not dynamically decrypted."
              />
            </div>
          </a-card>

          <a-card title="Request / response rewrite kernel" size="small">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch
                  v-model:checked="
                    runtimeSettings.domainForwardProxy.rewrite.enabled
                  "
                />
                <span>Enable bounded low-latency body rewriting</span>
              </div>
              <div>
                <div style="margin-bottom: 6px; font-weight: 600">
                  Maximum buffered body bytes
                </div>
                <a-input-number
                  v-model:value="
                    runtimeSettings.domainForwardProxy.rewrite.maxBodyBytes
                  "
                  :min="1024"
                  :max="67108864"
                  style="width: 220px"
                />
              </div>

              <a-card title="Native inference fast path" size="small">
                <div style="display: flex; flex-direction: column; gap: 10px">
                  <div
                    style="
                      display: flex;
                      align-items: center;
                      gap: 12px;
                      flex-wrap: wrap;
                    "
                  >
                    <a-switch
                      v-model:checked="
                        runtimeSettings.domainForwardProxy.rewrite.inference
                          .enabled
                      "
                    />
                    <span>Enable int8 feature-hash inference before literal rules</span>
                    <a-tag
                      v-if="domainForwardStatus.inferenceEnabled"
                      :color="
                        domainForwardStatus.inferenceReady ? 'green' : 'orange'
                      "
                    >
                      {{
                        domainForwardStatus.inferenceReady
                          ? "model ready"
                          : "model unavailable"
                      }}
                    </a-tag>
                  </div>
                  <a-input
                    v-model:value="
                      runtimeSettings.domainForwardProxy.rewrite.inference
                        .modelFile
                    "
                    placeholder="Native model JSON path"
                  />
                  <a-row :gutter="[8, 8]">
                    <a-col :xs="24" :md="6">
                      <a-select
                        v-model:value="
                          runtimeSettings.domainForwardProxy.rewrite.inference
                            .direction
                        "
                        :options="rewriteDirectionOptions"
                        style="width: 100%"
                      />
                    </a-col>
                    <a-col :xs="24" :md="6">
                      <a-input
                        v-model:value="
                          runtimeSettings.domainForwardProxy.rewrite.inference
                            .host
                        "
                        placeholder="Host scope"
                      />
                    </a-col>
                    <a-col :xs="24" :md="6">
                      <a-input
                        v-model:value="
                          runtimeSettings.domainForwardProxy.rewrite.inference
                            .pathPrefix
                        "
                        placeholder="Path prefix"
                      />
                    </a-col>
                    <a-col :xs="24" :md="6">
                      <a-input
                        v-model:value="
                          runtimeSettings.domainForwardProxy.rewrite.inference
                            .contentType
                        "
                        placeholder="Content-Type"
                      />
                    </a-col>
                    <a-col :xs="24" :md="6">
                      <a-input-number
                        v-model:value="
                          runtimeSettings.domainForwardProxy.rewrite.inference
                            .minTokenBytes
                        "
                        :min="1"
                        :max="4096"
                        style="width: 100%"
                        placeholder="Min token bytes"
                      />
                    </a-col>
                    <a-col :xs="24" :md="6">
                      <a-input-number
                        v-model:value="
                          runtimeSettings.domainForwardProxy.rewrite.inference
                            .maxTokenBytes
                        "
                        :min="1"
                        :max="4096"
                        style="width: 100%"
                        placeholder="Max token bytes"
                      />
                    </a-col>
                  </a-row>
                  <a-alert
                    type="info"
                    show-icon
                    message="Local quantized inference"
                    description="The model is loaded locally as int8 label weights. The proxy hashes 1-3 byte n-grams and uses integer accumulation only; JSON keys are skipped and only string values are eligible for replacement."
                  />
                </div>
              </a-card>

              <div
                style="
                  display: flex;
                  justify-content: space-between;
                  align-items: center;
                  gap: 8px;
                "
              >
                <div>
                  <div style="font-weight: 600">Fast model aliases</div>
                  <div style="font-size: 12px; color: #6b7280">
                    Rewrites only the top-level model field and restores the
                    client-visible model on responses / Responses WebSocket
                    streams.
                  </div>
                </div>
                <a-button size="small" @click="addModelRewriteRule">
                  <PlusOutlined /> Add
                </a-button>
              </div>
              <div
                v-for="(rule, index) in runtimeSettings.domainForwardProxy
                  .rewrite.modelRules"
                :key="`model-rewrite-${index}`"
                style="
                  display: grid;
                  grid-template-columns: 1fr 1fr 1fr auto;
                  gap: 8px;
                  align-items: center;
                "
              >
                <a-input v-model:value="rule.host" placeholder="Host (optional)" />
                <a-input v-model:value="rule.from" placeholder="Client model" />
                <a-input v-model:value="rule.to" placeholder="Upstream model" />
                <a-button danger @click="removeModelRewriteRule(index)">
                  <DeleteOutlined />
                </a-button>
              </div>

              <div
                style="
                  display: flex;
                  justify-content: space-between;
                  align-items: center;
                  gap: 8px;
                  margin-top: 4px;
                "
              >
                <div>
                  <div style="font-weight: 600">Literal body rules</div>
                  <div style="font-size: 12px; color: #6b7280">
                    Text/JSON only. SSE stays streaming; Responses WebSocket
                    text frames use the same rules.
                  </div>
                </div>
                <a-button size="small" @click="addBodyRewriteRule">
                  <PlusOutlined /> Add
                </a-button>
              </div>
              <a-card
                v-for="(rule, index) in runtimeSettings.domainForwardProxy
                  .rewrite.rules"
                :key="rule.id || `body-rewrite-${index}`"
                size="small"
              >
                <template #extra>
                  <a-button
                    size="small"
                    danger
                    @click="removeBodyRewriteRule(index)"
                  >
                    <DeleteOutlined />
                  </a-button>
                </template>
                <a-row :gutter="[8, 8]">
                  <a-col :xs="24" :md="4">
                    <a-switch v-model:checked="rule.enabled" />
                  </a-col>
                  <a-col :xs="24" :md="6">
                    <a-select
                      v-model:value="rule.direction"
                      :options="rewriteDirectionOptions"
                      style="width: 100%"
                    />
                  </a-col>
                  <a-col :xs="24" :md="7">
                    <a-input v-model:value="rule.host" placeholder="Host (optional)" />
                  </a-col>
                  <a-col :xs="24" :md="7">
                    <a-input
                      v-model:value="rule.pathPrefix"
                      placeholder="Path prefix (optional)"
                    />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-input
                      v-model:value="rule.contentType"
                      placeholder="Content-Type filter"
                    />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-input v-model:value="rule.find" placeholder="Find" />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-input v-model:value="rule.replace" placeholder="Replace" />
                  </a-col>
                </a-row>
              </a-card>
            </div>
          </a-card>

          <a-alert
            type="warning"
            show-icon
            message="Binding 80/443 requires root or CAP_NET_BIND_SERVICE. HTTPS forwarding requires a static certificate or an enabled allowlisted local CA."
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
            :title="`路由 #${index + 1}`"
          >
            <template #extra>
              <a-button
                size="small"
                danger
                @click="removeDomainForwardRoute(route.id)"
              >
                <DeleteOutlined /> 删除
              </a-button>
            </template>
            <a-row :gutter="[12, 12]">
              <a-col :span="24">
                <div
                  style="
                    display: grid;
                    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
                    gap: 12px;
                  "
                >
                  <a-input
                    :value="domainForwardRouteValue(index, 'host')"
                    placeholder="主机，例如 example.com 或 *.lab.test"
                    @update:value="
                      updateDomainForwardRouteField(index, 'host', $event)
                    "
                  />
                  <a-input
                    :value="domainForwardRouteValue(index, 'upstream')"
                    placeholder="上游地址，例如 https://{host}"
                    @update:value="
                      updateDomainForwardRouteField(index, 'upstream', $event)
                    "
                  />
                  <a-input
                    :value="domainForwardRouteValue(index, 'certFile')"
                    placeholder="路由证书路径（可选）"
                    @update:value="
                      updateDomainForwardRouteField(index, 'certFile', $event)
                    "
                  />
                  <a-input
                    :value="domainForwardRouteValue(index, 'keyFile')"
                    placeholder="路由私钥路径（可选）"
                    @update:value="
                      updateDomainForwardRouteField(index, 'keyFile', $event)
                    "
                  />
                </div>
              </a-col>
            </a-row>
          </a-card>
          <a-typography-text type="secondary">
            上游地址留空或启用 <code>allowAnyHost</code> 时，会转发到
            <code>&lt;scheme&gt;://&lt;request-host&gt;</code>。通配符支持
            <code>*.example.com</code>；<code>{host}</code> 会展开为规范化后的请求主机。
          </a-typography-text>
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <a-tag :color="domainForwardStatus.enabled ? 'blue' : 'default'">
              {{ domainForwardStatus.enabled ? "已启用" : "已停用" }}
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
