<script setup lang="ts">
import { ReloadOutlined } from "@ant-design/icons-vue";
import type { useConfigRuntime } from "../../../composables/config/useConfigRuntime";

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
}>();

const {
  runtimeSettings,
  bootstrapHealth,
  collectorHealth,
  otelHealth,
  domainForwardStatus,
  loopDetectionStatus,
  researchProcessingStatus,
  fetchCollectorHealth,
  fetchLoopDetectionStatus,
  runLoopDetectionScan,
  resetLoopDetection,
  fetchResearchProcessingStatus,
  runResearchProcessingScan,
  resetResearchProcessing,
  clearInMemoryEvents,
  clearPersistedLog,
  clearAllEvents,
} = props.runtime;

const formatLatencyMs = (value?: number) => {
  if (!value) return "0 ms";
  return `${(value / 1_000_000).toFixed(value >= 1_000_000 ? 2 : 3)} ms`;
};

const formatMaybeDate = (value?: string) => {
  if (!value) return "Not exported yet";
  return value;
};

const compactList = (items?: Array<string | number>) => {
  if (!items || items.length === 0) return "—";
  return items.join(", ");
};
</script>

<template>
  <a-col :span="24">
    <a-card title="Dead-loop Detection Worker" size="small">
      <template #extra>
        <div style="display: flex; gap: 8px; flex-wrap: wrap">
          <a-button size="small" @click="fetchLoopDetectionStatus">
            <ReloadOutlined /> Refresh
          </a-button>
          <a-button size="small" type="primary" @click="runLoopDetectionScan()">
            Scan Recent Events
          </a-button>
          <a-popconfirm
            title="Reset loop windows and recent findings?"
            @confirm="resetLoopDetection"
          >
            <a-button size="small" danger>Reset</a-button>
          </a-popconfirm>
        </div>
      </template>
      <div style="display: flex; flex-direction: column; gap: 12px">
        <div style="display: flex; gap: 8px; flex-wrap: wrap">
          <a-tag :color="loopDetectionStatus.enabled ? 'blue' : 'default'">
            {{
              loopDetectionStatus.enabled ? "streaming enabled" : "manual scan"
            }}
          </a-tag>
          <a-tag color="purple">
            consumed: {{ loopDetectionStatus.consumedTotal }}
          </a-tag>
          <a-tag color="orange">
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
            {{ loopDetectionStatus.queueLen }}/{{
              loopDetectionStatus.queueCap
            }}
          </a-tag>
          <a-tag color="default">
            dropped: {{ loopDetectionStatus.droppedTotal }}
          </a-tag>
        </div>
        <a-alert
          v-if="loopDetectionStatus.lastError"
          type="warning"
          show-icon
          :message="loopDetectionStatus.lastError"
        />
        <a-alert
          v-else
          type="info"
          show-icon
          message="Repeated-context findings are localized to run / task / tool / trace / PID."
          description="Use Scan Recent Events for offline research even when streaming detection is disabled. Enable and tune thresholds from Runtime Config."
        />
        <a-empty
          v-if="loopDetectionStatus.recentFindings.length === 0"
          description="No repeated contexts detected yet"
          :image="false"
        />
        <a-card
          v-for="finding in loopDetectionStatus.recentFindings"
          :key="finding.id"
          size="small"
          :title="`${finding.contextType}: ${finding.contextKey}`"
        >
          <div style="display: flex; flex-direction: column; gap: 8px">
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <a-tag color="red">
                repeats: {{ finding.repeatCount }} /
                {{ finding.windowSeconds }}s
              </a-tag>
              <a-tag color="purple">{{ finding.target || "no target" }}</a-tag>
              <a-tag v-if="finding.agentRunId" color="blue">
                run {{ finding.agentRunId }}
              </a-tag>
              <a-tag v-if="finding.taskId" color="geekblue">
                task {{ finding.taskId }}
              </a-tag>
              <a-tag v-if="finding.toolCallId" color="cyan">
                tool {{ finding.toolCallId }}
              </a-tag>
            </div>
            <div>
              <strong>reason:</strong>
              {{ finding.reason }}
            </div>
            <div>
              <strong>fingerprint:</strong>
              <code>{{ finding.fingerprint }}</code>
            </div>
            <div>
              <strong>events:</strong>
              {{ compactList(finding.eventTypes) }}
            </div>
            <div>
              <strong>pids:</strong>
              {{ compactList(finding.pids) }}
            </div>
            <div>
              <strong>comms:</strong>
              {{ compactList(finding.comms) }}
            </div>
            <div>
              <strong>paths:</strong>
              {{ compactList(finding.paths) }}
            </div>
            <a-typography-text type="secondary">
              {{ finding.suggestedAction }}
            </a-typography-text>
          </div>
        </a-card>
      </div>
    </a-card>
  </a-col>

  <a-col :span="24">
    <a-card title="Backend Research Processing" size="small">
      <template #extra>
        <div style="display: flex; gap: 8px; flex-wrap: wrap">
          <a-button size="small" @click="fetchResearchProcessingStatus">
            <ReloadOutlined /> Refresh
          </a-button>
          <a-button
            size="small"
            type="primary"
            @click="runResearchProcessingScan()"
          >
            Scan Recent Events
          </a-button>
          <a-popconfirm
            title="Reset backend research summaries?"
            @confirm="resetResearchProcessing"
          >
            <a-button size="small" danger>Reset</a-button>
          </a-popconfirm>
        </div>
      </template>
      <div style="display: flex; flex-direction: column; gap: 12px">
        <div style="display: flex; gap: 8px; flex-wrap: wrap">
          <a-tag :color="researchProcessingStatus.enabled ? 'blue' : 'default'">
            {{
              researchProcessingStatus.enabled
                ? "streaming enabled"
                : "manual scan"
            }}
          </a-tag>
          <a-tag color="purple">
            consumed: {{ researchProcessingStatus.consumedTotal }}
          </a-tag>
          <a-tag color="blue">
            buffered: {{ researchProcessingStatus.bufferedTotal }}
          </a-tag>
          <a-tag color="cyan">
            evicted: {{ researchProcessingStatus.bufferEvictedTotal }}
          </a-tag>
          <a-tag color="orange">
            summarized: {{ researchProcessingStatus.summary.total }}
          </a-tag>
          <a-tag color="geekblue">
            queue:
            {{ researchProcessingStatus.queueLen }}/{{
              researchProcessingStatus.queueCap
            }}
          </a-tag>
          <a-tag color="default">
            dropped: {{ researchProcessingStatus.droppedTotal }}
          </a-tag>
          <a-tag color="cyan">
            retention:
            {{ researchProcessingStatus.settings.artifactRetentionDays }}d
          </a-tag>
          <a-tag color="green">
            session cap:
            {{ researchProcessingStatus.settings.maxSessionEvents }}
          </a-tag>
          <a-tag color="gold">
            exports: {{ researchProcessingStatus.settings.exportFormats }}
          </a-tag>
        </div>
        <a-alert
          v-if="researchProcessingStatus.lastError"
          type="warning"
          show-icon
          :message="researchProcessingStatus.lastError"
        />
        <a-alert
          v-else
          type="info"
          show-icon
          message="Backend mirrors the frontend AgentSight research transforms."
          description="The worker builds source/type/comm/trace counts, timeline buckets, process summaries, recent samples, and Research v2 export defaults outside the browser. Use manual scan for offline research when streaming is disabled."
        />
        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="12">
            <div style="font-weight: 600; margin-bottom: 8px">
              Top sources / types
            </div>
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <a-tag
                v-for="item in researchProcessingStatus.summary.bySource"
                :key="`source-${item.key}`"
                color="blue"
              >
                {{ item.key }}: {{ item.count }}
              </a-tag>
              <a-tag
                v-for="item in researchProcessingStatus.summary.byType"
                :key="`type-${item.key}`"
                color="purple"
              >
                {{ item.key }}: {{ item.count }}
              </a-tag>
            </div>
          </a-col>
          <a-col :xs="24" :lg="12">
            <div style="font-weight: 600; margin-bottom: 8px">
              Timeline buckets
            </div>
            <a-empty
              v-if="researchProcessingStatus.summary.timeline.length === 0"
              description="No timeline buckets"
              :image="false"
            />
            <div v-else style="display: flex; gap: 8px; flex-wrap: wrap">
              <a-tag
                v-for="bucket in researchProcessingStatus.summary.timeline"
                :key="bucket.start"
                color="geekblue"
              >
                {{ bucket.time }}: {{ bucket.count }}
              </a-tag>
            </div>
          </a-col>
        </a-row>
        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="12">
            <div style="font-weight: 600; margin-bottom: 8px">
              Top processes
            </div>
            <a-empty
              v-if="researchProcessingStatus.summary.topProcesses.length === 0"
              description="No process summaries"
              :image="false"
            />
            <template v-else>
              <a-card
                v-for="process in researchProcessingStatus.summary.topProcesses"
                :key="process.pid"
                size="small"
                style="margin-bottom: 8px"
              >
                <div style="display: flex; gap: 8px; flex-wrap: wrap">
                  <a-tag color="blue">
                    {{ process.comm }}#{{ process.pid }}
                  </a-tag>
                  <a-tag color="purple">
                    events: {{ process.eventCount }}
                  </a-tag>
                  <a-tag v-if="process.ppid" color="default">
                    ppid: {{ process.ppid }}
                  </a-tag>
                </div>
                <div style="margin-top: 6px">
                  <strong>sources:</strong>
                  {{ compactList(process.sources) }}
                </div>
                <div>
                  <strong>children:</strong>
                  {{ compactList(process.childPids) }}
                </div>
              </a-card>
            </template>
          </a-col>
          <a-col :xs="24" :lg="12">
            <div style="font-weight: 600; margin-bottom: 8px">
              Recent backend samples
            </div>
            <a-empty
              v-if="researchProcessingStatus.summary.recentSamples.length === 0"
              description="No backend samples"
              :image="false"
            />
            <template v-else>
              <a-card
                v-for="sample in researchProcessingStatus.summary.recentSamples"
                :key="sample.id"
                size="small"
                style="margin-bottom: 8px"
              >
                <div style="display: flex; gap: 8px; flex-wrap: wrap">
                  <a-tag color="blue">{{ sample.source }}</a-tag>
                  <a-tag color="purple">{{ sample.eventType }}</a-tag>
                  <a-tag v-if="sample.pid" color="default">
                    pid: {{ sample.pid }}
                  </a-tag>
                </div>
                <div style="margin-top: 6px">{{ sample.title }}</div>
                <a-typography-text type="secondary">
                  {{ sample.time }}
                </a-typography-text>
              </a-card>
            </template>
          </a-col>
        </a-row>
      </div>
    </a-card>
  </a-col>
</template>
