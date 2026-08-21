<script setup lang="ts">
import {
  ReloadOutlined,
  SafetyCertificateOutlined,
} from "@ant-design/icons-vue";
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
  <a-card title="Build Feature Manifest" size="small">
    <div style="display: flex; gap: 8px; flex-wrap: wrap">
      <a-tag
        v-for="feature in mergedFeatures"
        :key="feature.id"
        :color="featureStatusColor(feature.id)"
      >
        {{ feature.name }} · {{ featureStatusLabel(feature.id) }}
      </a-tag>
    </div>
    <a-typography-text type="secondary">
      compiled-out 表示当前二进制或前端构建未携带该模块；runtime-disabled
      表示已编译但运行时 gate 未开启。
    </a-typography-text>
  </a-card>
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
      <div
        style="display: flex; gap: 8px; flex-wrap: wrap; align-items: center"
      >
        <a-tag :color="persistedEventLogAlive ? 'green' : 'red'">
          {{ persistedEventLogAlive ? "Log file ready" : "Log file inactive" }}
        </a-tag>
        <a-tag color="blue">{{ persistedEventLogPath || "No log path" }}</a-tag>
      </div>
      <a-divider style="margin: 4px 0" />
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch
          v-model:checked="runtimeSettings.shellSessionsEnabled"
          :disabled="!isCompiledIn('shell_sessions')"
        />
        <span>Enable PTY / shell sessions</span>
        <a-tag :color="featureStatusColor('shell_sessions')">
          {{ featureStatusLabel("shell_sessions") }}
        </a-tag>
      </div>
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch
          v-model:checked="runtimeSettings.systemRunEnabled"
          :disabled="!isCompiledIn('system_run')"
        />
        <span>Enable /system/run command launch</span>
        <a-tag :color="featureStatusColor('system_run')">
          {{ featureStatusLabel("system_run") }}
        </a-tag>
      </div>
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch
          v-model:checked="runtimeSettings.hookManagementEnabled"
          :disabled="!isCompiledIn('hooks')"
        />
        <span>Enable hook injection / config editing</span>
        <a-tag :color="featureStatusColor('hooks')">
          {{ featureStatusLabel("hooks") }}
        </a-tag>
      </div>
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch
          v-model:checked="runtimeSettings.policyManagementEnabled"
          :disabled="!isCompiledIn('policy_management')"
        />
        <span>Enable policy mutations (tags / comms / paths / rules)</span>
        <a-tag :color="featureStatusColor('policy_management')">
          {{ featureStatusLabel("policy_management") }}
        </a-tag>
      </div>
      <a-divider style="margin: 4px 0" />
      <div style="display: flex; flex-direction: column; gap: 12px">
        <div style="display: flex; align-items: center; gap: 12px">
          <a-switch
            v-model:checked="runtimeSettings.kernelRiskFeedback.enabled"
            :disabled="!isCompiledIn('policy_management')"
          />
          <span>Enable kernel-risk feedback into cgroup / BPF LSM maps</span>
          <a-tag
            :color="runtimeSettings.policyManagementEnabled ? 'blue' : 'orange'"
          >
            requires policy gate
          </a-tag>
        </div>
        <div style="display: flex; gap: 12px; flex-wrap: wrap">
          <div>
            <div style="margin-bottom: 6px; font-weight: 600">
              Min risk score
            </div>
            <a-input-number
              v-model:value="runtimeSettings.kernelRiskFeedback.minRiskScore"
              :min="1"
              :max="100"
              :step="1"
              style="width: 140px"
            />
          </div>
          <div>
            <div style="margin-bottom: 6px; font-weight: 600">
              Max actions / minute
            </div>
            <a-input-number
              v-model:value="
                runtimeSettings.kernelRiskFeedback.maxActionsPerMinute
              "
              :min="1"
              :max="600"
              :step="5"
              style="width: 160px"
            />
          </div>
        </div>
        <div style="display: flex; gap: 12px; flex-wrap: wrap">
          <label style="display: flex; align-items: center; gap: 8px">
            <a-switch
              v-model:checked="
                runtimeSettings.kernelRiskFeedback.enforceNetwork
              "
            />
            <span>Network IP / port</span>
          </label>
          <label style="display: flex; align-items: center; gap: 8px">
            <a-switch
              v-model:checked="
                runtimeSettings.kernelRiskFeedback.enforceFileNames
              "
            />
            <span>Sensitive file basenames</span>
          </label>
          <label style="display: flex; align-items: center; gap: 8px">
            <a-switch
              v-model:checked="runtimeSettings.kernelRiskFeedback.enforceExec"
            />
            <span>Suspicious exec paths / names</span>
          </label>
        </div>
        <a-alert
          type="info"
          show-icon
          message="Closed-loop enforcement remains opt-in and rate-limited."
          description="High-risk kernel events are scored in user space; when both this switch and policy management are enabled, public network destinations can be written to exact IP/port cgroup maps, sensitive basenames to BPF LSM file maps, and suspicious executable paths/names to BPF LSM exec maps."
        />
      </div>
      <a-alert
        type="warning"
        show-icon
        message="High-risk capabilities stay disabled until explicitly enabled."
        description="PTY sessions, /system/run, hook injection, policy mutations, and kernel-risk feedback can change host state."
      />
      <a-button type="primary" @click="saveRuntime">
        <ReloadOutlined /> Save Runtime Gates
      </a-button>
    </div>
  </a-card>
</template>
