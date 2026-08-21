<script setup lang="ts">
import { CopyOutlined, ReloadOutlined } from "@ant-design/icons-vue";
import type { useConfigRuntime } from "../../../composables/config/useConfigRuntime";

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
}>();

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
          <a-button
            @click="
              copyText(runtimeSettings.accessToken, 'Access token copied')
            "
          >
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
          <a-button
            @click="
              copyText(mcpQueryEndpointTemplate, 'MCP query template copied')
            "
          >
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
  <a-card title="Event Retention" size="small">
    <div style="display: flex; flex-direction: column; gap: 14px">
      <div
        style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap"
      >
        <span>Max in-memory events:</span>
        <a-input-number
          v-model:value="runtimeSettings.maxEventCount"
          :min="100"
          :max="10000"
          :step="100"
          style="width: 160px"
        />
      </div>
      <div
        style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap"
      >
        <span>Max event age:</span>
        <a-input
          v-model:value="runtimeSettings.maxEventAge"
          placeholder="e.g. 24h, 168h, 0 = no limit"
          style="width: 220px"
        />
        <a-typography-text type="secondary"
          >Go duration format (24h, 30m, 168h)</a-typography-text
        >
      </div>
      <a-button type="primary" @click="saveRuntime">
        <ReloadOutlined /> Save Retention
      </a-button>
    </div>
  </a-card>
</template>
