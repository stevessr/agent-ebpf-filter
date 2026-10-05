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
    <div style="display: flex; flex-direction: column; gap: 16px">
      <div>
        <div style="font-weight: 600; margin-bottom: 8px">
          Persistent Pebble history
        </div>
        <div style="display: flex; gap: 12px; flex-wrap: wrap">
          <label style="display: grid; gap: 6px">
            <span>Max records</span>
            <a-input-number
              v-model:value="runtimeSettings.eventStoreMaxRecords"
              :min="1000"
              :max="10000000"
              :step="10000"
              style="width: 180px"
            />
          </label>
          <label style="display: grid; gap: 6px">
            <span>Max age</span>
            <a-input
              v-model:value="runtimeSettings.eventStoreMaxAge"
              placeholder="168h"
              style="width: 220px"
            />
          </label>
        </div>
        <a-typography-text type="secondary">
          Defaults to 250,000 records / 168h. The first reached limit prunes
          the oldest database records; use 0 only for the age limit to disable
          age-based pruning.
        </a-typography-text>
      </div>

      <a-divider style="margin: 0" />

      <div>
        <div style="font-weight: 600; margin-bottom: 8px">
          In-memory hot archive
        </div>
        <div style="display: flex; gap: 12px; flex-wrap: wrap">
          <label style="display: grid; gap: 6px">
            <span>Max records</span>
            <a-input-number
              v-model:value="runtimeSettings.maxEventCount"
              :min="100"
              :max="2000000"
              :step="1000"
              style="width: 180px"
            />
          </label>
          <label style="display: grid; gap: 6px">
            <span>Max age</span>
            <a-input
              v-model:value="runtimeSettings.maxEventAge"
              placeholder="0, 24h, 168h"
              style="width: 220px"
            />
          </label>
        </div>
        <a-typography-text type="secondary">
          This short hot window backs in-process fallbacks and analysis. It is
          independent from the longer Pebble database history.
        </a-typography-text>
      </div>

      <a-button type="primary" @click="saveRuntime">
        <ReloadOutlined /> Save Retention
      </a-button>
    </div>
  </a-card>
</template>
