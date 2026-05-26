<script setup lang="ts">
import { computed } from 'vue';
import {
  CopyOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons-vue';
import type { useConfigRuntime } from '../../composables/useConfigRuntime';

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
}>();

const {
  runtimeSettings, mcpEndpoint,
  persistedEventLogPath, persistedEventLogAlive,
  otlpHeaderRows, domainForwardRoutes, domainForwardStatus,
  saveRuntime, rotateAccessToken,
  addOTLPHeaderRow, removeOTLPHeaderRow,
  addDomainForwardRoute, removeDomainForwardRoute,
  copyText, mcpQueryEndpoint, mcpQueryEndpointTemplate,
} = props.runtime;

const schemeOptions = [
  { value: 'https', label: 'https' },
  { value: 'http', label: 'http' },
];

type DomainForwardRoutePreview = {
  id: string;
  match: string;
  sampleHost: string;
  upstream: string;
  errors: string[];
  warnings: string[];
};

const normalizeForwardHost = (raw: string) => {
  let value = raw.trim().toLowerCase();
  if (!value) return '';

  if (value.includes('://')) {
    try {
      value = new URL(value).host;
    } catch (_) {
      return '';
    }
  }

  if (value.startsWith('[')) {
    const end = value.indexOf(']');
    if (end > 0) value = value.slice(1, end);
  } else if (value.indexOf(':') === value.lastIndexOf(':')) {
    value = value.split(':')[0];
  }

  return value.replace(/\.$/, '').replace(/^\[|\]$/g, '');
};

const normalizeDomainPattern = (raw: string) => {
  const value = raw.trim().toLowerCase();
  if (!value) return '';
  if (value.startsWith('*.')) {
    const suffix = normalizeForwardHost(value.slice(2));
    return suffix ? `*.${suffix}` : '';
  }
  return normalizeForwardHost(value);
};

const sampleHostForPattern = (pattern: string) => {
  if (!pattern) return '{host}';
  if (pattern.startsWith('*.')) return `app.${pattern.slice(2)}`;
  return pattern;
};

const buildUpstreamPreview = (raw: string, sampleHost: string) => {
  const scheme = runtimeSettings.value.domainForwardProxy.defaultScheme === 'http' ? 'http' : 'https';
  let upstream = raw.trim();
  if (!upstream) {
    upstream = `${scheme}://${sampleHost}`;
  } else {
    upstream = upstream.split('{host}').join(sampleHost);
    if (!upstream.includes('://')) upstream = `${scheme}://${upstream}`;
  }

  try {
    const parsed = new URL(upstream);
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return '';
    if (!parsed.host) return '';
    return parsed.toString();
  } catch (_) {
    return '';
  }
};

const routeDuplicateIndexes = computed(() => {
  const firstByMatch = new Map<string, number>();
  const duplicateById = new Map<string, number>();

  domainForwardRoutes.value.forEach((route, index) => {
    const match = normalizeDomainPattern(String(route.host || ''));
    if (!match) return;
    const firstIndex = firstByMatch.get(match);
    if (firstIndex === undefined) {
      firstByMatch.set(match, index);
      return;
    }
    duplicateById.set(route.id, firstIndex);
    const firstRoute = domainForwardRoutes.value[firstIndex];
    if (firstRoute) duplicateById.set(firstRoute.id, firstIndex);
  });

  return duplicateById;
});

const domainForwardRoutePreviews = computed<DomainForwardRoutePreview[]>(() => {
  const duplicates = routeDuplicateIndexes.value;

  return domainForwardRoutes.value.map((route, index) => {
    const rawHost = String(route.host || '');
    const match = normalizeDomainPattern(rawHost);
    const sampleHost = sampleHostForPattern(match);
    const upstream = buildUpstreamPreview(String(route.upstream || ''), sampleHost);
    const errors: string[] = [];
    const warnings: string[] = [];
    const duplicateFirstIndex = duplicates.get(route.id);
    const certFile = String(route.certFile || '').trim();
    const keyFile = String(route.keyFile || '').trim();

    if (!match && (rawHost.trim() || String(route.upstream || '').trim() || certFile || keyFile)) {
      errors.push('Host pattern is invalid or empty.');
    }
    if (match && !upstream) {
      errors.push('Upstream preview cannot be parsed as an HTTP/HTTPS URL.');
    }
    if (duplicateFirstIndex !== undefined) {
      if (duplicateFirstIndex === index) {
        warnings.push('Another route uses the same normalized host; backend keeps this first route.');
      } else {
        warnings.push(`Duplicate of route #${duplicateFirstIndex + 1}; backend ignores later duplicates.`);
      }
    }
    if ((certFile && !keyFile) || (!certFile && keyFile)) {
      warnings.push('Route TLS certificate and key should be configured together.');
    }

    return {
      id: route.id,
      match: match || 'invalid host',
      sampleHost,
      upstream: upstream || 'invalid upstream',
      errors,
      warnings,
    };
  });
});

const domainForwardConfigIssues = computed(() => {
  const proxy = runtimeSettings.value.domainForwardProxy;
  const issues: string[] = [];
  const hasDefaultCertPair = Boolean(proxy.certFile?.trim() && proxy.keyFile?.trim());
  const hasAnyRouteCertPair = domainForwardRoutes.value.some((route) => (
    Boolean(String(route.certFile || '').trim() && String(route.keyFile || '').trim())
  ));
  const invalidRouteCount = domainForwardRoutePreviews.value.filter((preview) => preview.errors.length > 0).length;
  const duplicateCount = Array.from(routeDuplicateIndexes.value.entries()).filter(([id, firstIndex]) => {
    return domainForwardRoutes.value[firstIndex]?.id !== id;
  }).length;

  if (!proxy.enabled) return issues;
  if (proxy.httpPort === proxy.httpsPort) {
    issues.push('HTTP and HTTPS listeners cannot bind the same port.');
  }
  if (!proxy.allowAnyHost && domainForwardRoutes.value.length === 0) {
    issues.push('No route overrides are configured, so every Host header will be rejected.');
  }
  if ((proxy.certFile?.trim() && !proxy.keyFile?.trim()) || (!proxy.certFile?.trim() && proxy.keyFile?.trim())) {
    issues.push('Default TLS certificate and key should be configured together.');
  }
  if (!hasDefaultCertPair && !hasAnyRouteCertPair) {
    issues.push('HTTPS listener needs a default cert/key or at least one route-level cert/key.');
  }
  if (invalidRouteCount > 0) {
    issues.push(`${invalidRouteCount} route preview${invalidRouteCount > 1 ? 's have' : ' has'} invalid host or upstream values.`);
  }
  if (duplicateCount > 0) {
    issues.push(`${duplicateCount} duplicate route${duplicateCount > 1 ? 's are' : ' is'} ignored by the backend.`);
  }
  if (proxy.allowAnyHost && !proxy.dnsResolver?.trim()) {
    issues.push('Allow-any-host mode uses system DNS; set a resolver override if test domains point back to this host.');
  }

  return issues;
});
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-alert
        type="info"
        show-icon
        message="Runtime configuration is now fully visual."
        description="Use the forms below to edit every exposed runtime switch, token, retention value, OTLP header, and domain-forward route without writing raw JSON. Save applies the current runtime snapshot."
      />
    </a-col>

    <a-col :xs="24" :xl="12">
      <a-card title="Runtime Feature Gates" size="small">
        <template #extra>
          <SafetyCertificateOutlined />
        </template>
        <div style="display: flex; flex-direction: column; gap: 14px">
          <div style="display: flex; align-items: center; gap: 12px">
            <a-switch v-model:checked="runtimeSettings.logPersistenceEnabled" />
            <span>Persist captured logs to file</span>
          </div>
          <a-input
            v-model:value="runtimeSettings.logFilePath"
            placeholder="Log file path (defaults to ~/.config/agent-ebpf-filter/events.jsonl)"
          />
          <div style="display: flex; gap: 8px; flex-wrap: wrap; align-items: center">
            <a-tag :color="persistedEventLogAlive ? 'green' : 'red'">
              {{ persistedEventLogAlive ? 'Log file ready' : 'Log file inactive' }}
            </a-tag>
            <a-tag color="blue">{{ persistedEventLogPath || 'No log path' }}</a-tag>
          </div>
          <a-divider style="margin: 4px 0" />
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
          <a-alert
            type="warning"
            show-icon
            message="High-risk capabilities stay disabled until explicitly enabled."
            description="PTY sessions, /system/run, hook injection, and policy mutations can change host state."
          />
          <a-button type="primary" @click="saveRuntime">
            <ReloadOutlined /> Save Runtime Gates
          </a-button>
        </div>
      </a-card>
    </a-col>

    <a-col :xs="24" :xl="12">
      <a-card title="Access Token & MCP" size="small">
        <div style="display: flex; flex-direction: column; gap: 14px">
          <div>
            <div style="margin-bottom: 6px; font-weight: 600">Access Token</div>
            <a-input
              :value="runtimeSettings.accessToken"
              readonly
              placeholder="Generate a token to access /config and /mcp"
            />
            <div style="display: flex; gap: 8px; flex-wrap: wrap; margin-top: 8px">
              <a-button @click="rotateAccessToken">
                <ReloadOutlined /> Generate / Rotate
              </a-button>
              <a-button @click="copyText(runtimeSettings.accessToken, 'Access token copied')">
                <CopyOutlined /> Copy Token
              </a-button>
            </div>
          </div>
          <div>
            <div style="margin-bottom: 6px; font-weight: 600">MCP Endpoint</div>
            <a-input :value="mcpEndpoint" readonly />
            <div style="display: flex; gap: 8px; flex-wrap: wrap; margin-top: 8px">
              <a-button @click="copyText(mcpEndpoint, 'MCP endpoint copied')">
                <CopyOutlined /> Copy Base URL
              </a-button>
            </div>
          </div>
          <div>
            <div style="margin-bottom: 6px; font-weight: 600">MCP Query URL</div>
            <a-input :value="mcpQueryEndpoint" readonly />
            <div style="display: flex; gap: 8px; flex-wrap: wrap; margin-top: 8px">
              <a-button @click="copyText(mcpQueryEndpoint, 'MCP query URL copied')">
                <CopyOutlined /> Copy Query URL
              </a-button>
              <a-button @click="copyText(mcpQueryEndpointTemplate, 'MCP query template copied')">
                <CopyOutlined /> Copy Template
              </a-button>
            </div>
          </div>
          <a-alert
            type="success"
            show-icon
            message="Query URL is generated live from the current token and updates when you rotate it."
          />
        </div>
      </a-card>
    </a-col>

    <a-col :xs="24" :xl="12">
      <a-card title="Event Retention" size="small">
        <div style="display: flex; flex-direction: column; gap: 14px">
          <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap">
            <span>Max in-memory events:</span>
            <a-input-number
              v-model:value="runtimeSettings.maxEventCount"
              :min="100"
              :max="10000"
              :step="100"
              style="width: 160px"
            />
          </div>
          <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap">
            <span>Max event age:</span>
            <a-input
              v-model:value="runtimeSettings.maxEventAge"
              placeholder="e.g. 24h, 168h, 0 = no limit"
              style="width: 220px"
            />
            <a-typography-text type="secondary">Go duration format (24h, 30m, 168h)</a-typography-text>
          </div>
          <a-button type="primary" @click="saveRuntime">
            <ReloadOutlined /> Save Retention
          </a-button>
        </div>
      </a-card>
    </a-col>

    <a-col :xs="24" :xl="12">
      <a-card title="TLS Capture" size="small">
        <div style="display: flex; flex-direction: column; gap: 14px">
          <div style="display: flex; align-items: center; gap: 12px">
            <a-switch v-model:checked="runtimeSettings.tlsCaptureEnabled" />
            <span>Enable TLS plaintext capture (eBPF uprobes on OpenSSL/GnuTLS/NSS/Go)</span>
          </div>
          <a-alert
            type="warning"
            show-icon
            message="Backend restart required."
            description="TLS capture hooks plaintext before encryption / after decryption via eBPF uprobes."
          />
          <a-button type="primary" @click="saveRuntime">
            <ReloadOutlined /> Save TLS Capture Setting
          </a-button>
        </div>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="OpenTelemetry Export" size="small">
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :lg="10">
            <div style="display: flex; flex-direction: column; gap: 12px">
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
              <a-typography-text type="secondary">
                OTLP export emits <code>agent.run</code>, <code>codex.task</code>, <code>tool.call</code>, <code>mcp.call</code>, process, file, network, and policy spans derived from EventEnvelope records.
              </a-typography-text>
              <a-button type="primary" @click="saveRuntime">
                <ReloadOutlined /> Save OTLP Settings
              </a-button>
            </div>
          </a-col>
          <a-col :xs="24" :lg="14">
            <div style="display: flex; flex-direction: column; gap: 10px">
              <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px">
                <div style="font-weight: 600">OTLP Headers</div>
                <a-button size="small" @click="addOTLPHeaderRow">
                  <PlusOutlined /> Add Header
                </a-button>
              </div>
              <a-empty v-if="otlpHeaderRows.length === 0" description="No custom headers" :image="false" />
              <div
                v-for="row in otlpHeaderRows"
                :key="row.id"
                style="display: grid; grid-template-columns: minmax(160px, 1fr) minmax(180px, 1fr) auto; gap: 8px; align-items: center"
              >
                <a-input v-model:value="row.key" placeholder="Header name, e.g. Authorization" />
                <a-input v-model:value="row.value" placeholder="Header value" />
                <a-button danger @click="removeOTLPHeaderRow(row.id)">
                  <DeleteOutlined />
                </a-button>
              </div>
              <a-typography-text type="secondary">
                Blank rows are ignored. A non-empty value must include a header name.
              </a-typography-text>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Domain Forward Proxy (80 / 443)" size="small">
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :lg="10">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.domainForwardProxy.enabled" />
                <span>Enable Host/SNI-based HTTP and HTTPS forwarding</span>
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
                  <div style="margin-bottom: 6px; font-weight: 600">Default upstream scheme</div>
                  <a-select
                    v-model:value="runtimeSettings.domainForwardProxy.defaultScheme"
                    style="width: 140px"
                    :options="schemeOptions"
                  />
                </div>
                <div>
                  <div style="margin-bottom: 6px; font-weight: 600">Dial timeout</div>
                  <a-input-number
                    v-model:value="runtimeSettings.domainForwardProxy.dialTimeoutSeconds"
                    :min="1"
                    :max="120"
                    style="width: 140px"
                  />
                </div>
              </div>
              <div style="display: flex; align-items: center; gap: 12px">
                <a-switch v-model:checked="runtimeSettings.domainForwardProxy.allowAnyHost" />
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
              <a-alert
                v-if="domainForwardConfigIssues.length > 0"
                type="warning"
                show-icon
                message="Configuration preview warnings"
                :description="domainForwardConfigIssues.join(' ')"
              />
              <a-alert
                v-else-if="runtimeSettings.domainForwardProxy.enabled"
                type="success"
                show-icon
                message="Local preview has no obvious conflicts."
                description="Save still depends on OS permissions, free listener ports, and readable certificate files."
              />
              <div style="display: flex; gap: 8px; flex-wrap: wrap; align-items: center">
                <a-button type="primary" @click="saveRuntime">
                  <ReloadOutlined /> Save & Apply Forwarding
                </a-button>
                <a-tag :color="domainForwardStatus.httpRunning ? 'green' : 'default'">
                  HTTP {{ domainForwardStatus.httpRunning ? 'running' : 'stopped' }}
                </a-tag>
                <a-tag :color="domainForwardStatus.httpsRunning ? 'green' : 'default'">
                  HTTPS {{ domainForwardStatus.httpsRunning ? 'running' : 'stopped' }}
                </a-tag>
              </div>
            </div>
          </a-col>
          <a-col :xs="24" :lg="14">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px">
                <div style="font-weight: 600">Visual route overrides</div>
                <a-button size="small" @click="addDomainForwardRoute">
                  <PlusOutlined /> Add Route
                </a-button>
              </div>
              <a-empty v-if="domainForwardRoutes.length === 0" description="No route overrides" :image="false" />
              <a-card
                v-for="(route, index) in domainForwardRoutes"
                :key="route.id"
                size="small"
                :title="`Route #${index + 1}`"
              >
                <template #extra>
                  <a-button size="small" danger @click="removeDomainForwardRoute(route.id)">
                    <DeleteOutlined /> Remove
                  </a-button>
                </template>
                <a-row :gutter="[12, 12]">
                  <a-col :xs="24" :md="12">
                    <a-input
                      v-model:value="route.host"
                      placeholder="Host, e.g. example.com or *.lab.test"
                      :status="domainForwardRoutePreviews[index]?.errors.length ? 'error' : undefined"
                    />
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <a-input
                      v-model:value="route.upstream"
                      placeholder="Upstream, e.g. https://{host}"
                      :status="domainForwardRoutePreviews[index]?.errors.length ? 'error' : undefined"
                    />
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <a-input
                      v-model:value="route.certFile"
                      placeholder="Route certificate path (optional)"
                      :status="domainForwardRoutePreviews[index]?.warnings.some((item) => item.includes('certificate')) ? 'warning' : undefined"
                    />
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <a-input
                      v-model:value="route.keyFile"
                      placeholder="Route private key path (optional)"
                      :status="domainForwardRoutePreviews[index]?.warnings.some((item) => item.includes('certificate')) ? 'warning' : undefined"
                    />
                  </a-col>
                </a-row>
                <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 12px">
                  <a-descriptions size="small" bordered :column="1">
                    <a-descriptions-item label="Normalized match">
                      <code>{{ domainForwardRoutePreviews[index]?.match }}</code>
                    </a-descriptions-item>
                    <a-descriptions-item label="Sample upstream">
                      <code>{{ domainForwardRoutePreviews[index]?.upstream }}</code>
                    </a-descriptions-item>
                  </a-descriptions>
                  <a-alert
                    v-if="domainForwardRoutePreviews[index]?.errors.length"
                    type="error"
                    show-icon
                    :message="domainForwardRoutePreviews[index].errors.join(' ')"
                  />
                  <a-alert
                    v-if="domainForwardRoutePreviews[index]?.warnings.length"
                    type="warning"
                    show-icon
                    :message="domainForwardRoutePreviews[index].warnings.join(' ')"
                  />
                </div>
              </a-card>
              <a-typography-text type="secondary">
                Empty upstreams or <code>allowAnyHost</code> forward to <code>&lt;scheme&gt;://&lt;request-host&gt;</code>. Wildcards support <code>*.example.com</code>; <code>{host}</code> expands to the normalized request host.
              </a-typography-text>
              <div style="display: flex; gap: 8px; flex-wrap: wrap">
                <a-tag :color="domainForwardStatus.enabled ? 'blue' : 'default'">
                  {{ domainForwardStatus.enabled ? 'enabled' : 'disabled' }}
                </a-tag>
                <a-tag color="blue">routes: {{ domainForwardStatus.routeCount }}</a-tag>
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
                v-if="domainForwardStatus.errors && domainForwardStatus.errors.length > 0"
                type="error"
                show-icon
                :message="domainForwardStatus.errors.join('; ')"
              />
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>
  </a-row>
</template>
