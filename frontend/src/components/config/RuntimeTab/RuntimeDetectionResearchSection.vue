<script setup lang="ts">
import { ReloadOutlined } from "@ant-design/icons-vue";
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
  <a-card title="Dead-loop / Repeated Context Detection" size="small">
    <div style="display: flex; flex-direction: column; gap: 14px">
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch v-model:checked="runtimeSettings.loopDetection.enabled" />
        <span>Enable single-consumer loop detector</span>
        <a-tag
          :color="runtimeSettings.loopDetection.enabled ? 'blue' : 'default'"
        >
          {{
            runtimeSettings.loopDetection.enabled
              ? "streaming"
              : "manual scan only"
          }}
        </a-tag>
      </div>
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch
          v-model:checked="runtimeSettings.loopDetection.emitSemanticAlerts"
        />
        <span>Mirror findings as RESOURCE_WASTING_LOOP alerts</span>
      </div>
      <div style="display: flex; gap: 12px; flex-wrap: wrap">
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">Window seconds</div>
          <a-input-number
            v-model:value="runtimeSettings.loopDetection.windowSeconds"
            :min="1"
            :max="3600"
            style="width: 150px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">
            Repeat threshold
          </div>
          <a-input-number
            v-model:value="runtimeSettings.loopDetection.repeatThreshold"
            :min="2"
            :max="1000"
            style="width: 160px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">Max contexts</div>
          <a-input-number
            v-model:value="runtimeSettings.loopDetection.maxContexts"
            :min="16"
            :max="20000"
            :step="16"
            style="width: 150px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">
            Worker queue size
          </div>
          <a-input-number
            v-model:value="runtimeSettings.loopDetection.queueSize"
            :min="128"
            :max="65536"
            :step="128"
            style="width: 170px"
          />
        </div>
      </div>
      <div style="display: flex; gap: 8px; flex-wrap: wrap">
        <a-tag color="purple">
          findings: {{ loopDetectionStatus.findingsTotal }}
        </a-tag>
        <a-tag color="blue">
          windows: {{ loopDetectionStatus.windowCount }}
        </a-tag>
        <a-tag color="cyan">
          gc: {{ loopDetectionStatus.windowGCRunsTotal }} / evicted:
          {{ loopDetectionStatus.windowEvictedTotal }}
        </a-tag>
        <a-tag color="geekblue">
          queue:
          {{ loopDetectionStatus.queueLen }}/{{ loopDetectionStatus.queueCap }}
        </a-tag>
      </div>
      <a-alert
        type="info"
        show-icon
        message="Research-friendly repeated-context localization"
        description="The backend consumes event records on one worker thread, groups repeated fingerprints by run/task/tool/trace/PID context, and retains contexts in a bounded O(1) LRU with periodic expiry GC. Settings persist to the home config folder; AGENT_RUNTIME_LOOP_DETECTION_* environment variables override them on startup."
      />
      <a-button type="primary" @click="saveRuntime">
        <ReloadOutlined /> Save Loop Detection
      </a-button>
    </div>
  </a-card>
  <a-card title="Backend Research Processing" size="small">
    <div style="display: flex; flex-direction: column; gap: 14px">
      <div style="display: flex; align-items: center; gap: 12px">
        <a-switch
          v-model:checked="runtimeSettings.researchProcessing.enabled"
        />
        <span>Enable backend AgentSight-style processing worker</span>
        <a-tag
          :color="
            runtimeSettings.researchProcessing.enabled ? 'blue' : 'default'
          "
        >
          {{
            runtimeSettings.researchProcessing.enabled
              ? "streaming"
              : "manual scan"
          }}
        </a-tag>
      </div>
      <div style="display: flex; gap: 12px; flex-wrap: wrap">
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">
            Max backend events
          </div>
          <a-input-number
            v-model:value="runtimeSettings.researchProcessing.maxEvents"
            :min="100"
            :max="100000"
            :step="100"
            style="width: 170px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">
            Worker queue size
          </div>
          <a-input-number
            v-model:value="runtimeSettings.researchProcessing.queueSize"
            :min="128"
            :max="65536"
            :step="128"
            style="width: 170px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">
            Timeline bucket seconds
          </div>
          <a-input-number
            v-model:value="
              runtimeSettings.researchProcessing.timelineBucketSeconds
            "
            :min="1"
            :max="86400"
            style="width: 190px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">Top K</div>
          <a-input-number
            v-model:value="runtimeSettings.researchProcessing.topK"
            :min="1"
            :max="200"
            style="width: 120px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">Recent samples</div>
          <a-input-number
            v-model:value="runtimeSettings.researchProcessing.recentSamples"
            :min="1"
            :max="500"
            style="width: 150px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">
            Artifact retention days
          </div>
          <a-input-number
            v-model:value="
              runtimeSettings.researchProcessing.artifactRetentionDays
            "
            :min="1"
            :max="3650"
            style="width: 190px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">
            Max session events
          </div>
          <a-input-number
            v-model:value="runtimeSettings.researchProcessing.maxSessionEvents"
            :min="100"
            :max="100000"
            :step="1000"
            style="width: 180px"
          />
        </div>
        <div>
          <div style="margin-bottom: 6px; font-weight: 600">Export formats</div>
          <a-input
            v-model:value="runtimeSettings.researchProcessing.exportFormats"
            placeholder="jsonl,csv,bundle"
            style="width: 190px"
          />
        </div>
      </div>
      <div style="display: flex; gap: 8px; flex-wrap: wrap">
        <a-tag color="purple">
          buffered: {{ researchProcessingStatus.bufferedTotal }}
        </a-tag>
        <a-tag color="cyan">
          evicted: {{ researchProcessingStatus.bufferEvictedTotal }}
        </a-tag>
        <a-tag color="blue">
          summary total: {{ researchProcessingStatus.summary.total }}
        </a-tag>
        <a-tag color="geekblue">
          queue:
          {{ researchProcessingStatus.queueLen }}/{{
            researchProcessingStatus.queueCap
          }}
        </a-tag>
        <a-tag color="cyan">
          retention:
          {{ runtimeSettings.researchProcessing.artifactRetentionDays }}d
        </a-tag>
        <a-tag color="green">
          session cap:
          {{ runtimeSettings.researchProcessing.maxSessionEvents }}
        </a-tag>
      </div>
      <a-alert
        type="info"
        show-icon
        message="Move heavy research transforms and exports from browser to backend"
        description="The worker mirrors AgentSight-style normalization and keeps Research v2 session/export defaults in runtime config. Export formats are comma-separated (jsonl,csv,bundle); AGENT_RUNTIME_RESEARCH_PROCESSING_* overrides these values on startup."
      />
      <a-button type="primary" @click="saveRuntime">
        <ReloadOutlined /> Save Research Processing
      </a-button>
    </div>
  </a-card>
</template>
