<script setup lang="ts">
import {
  CopyOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
} from "@ant-design/icons-vue";
import RuntimeAccessRetentionSection from "./RuntimeTab/RuntimeAccessRetentionSection.vue";
import RuntimeDetectionResearchSection from "./RuntimeTab/RuntimeDetectionResearchSection.vue";
import RuntimeFeatureGatesSection from "./RuntimeTab/RuntimeFeatureGatesSection.vue";
import RuntimeNetworkExportSection from "./RuntimeTab/RuntimeNetworkExportSection.vue";
import type { useConfigRuntime } from "../../composables/config/useConfigRuntime";

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
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-alert
        type="info"
        show-icon
        message="Runtime configuration is now fully visual."
        description="Use the forms below to edit every exposed runtime switch, token, retention value, OTLP header, and domain-forward route without writing raw JSON. Save applies the current runtime snapshot."
      />
    </a-col>

    <RuntimeFeatureGatesSection :runtime="runtime" />
    <RuntimeDetectionResearchSection :runtime="runtime" />
    <RuntimeAccessRetentionSection :runtime="runtime" />
    <RuntimeNetworkExportSection :runtime="runtime" />
  </a-row>
</template>
