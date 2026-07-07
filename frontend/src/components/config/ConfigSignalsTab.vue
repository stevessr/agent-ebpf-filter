<script setup lang="ts">
import { computed, shallowRef } from "vue";
import {
  DeleteOutlined,
  DownloadOutlined,
  PlusOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons-vue";
import type { useConfigRuntime } from "../../composables/config/useConfigRuntime";
import type { useConfigRegistry } from "../../composables/config/useConfigRegistry";
import type { SignalRule, SignalRuleTestResponse } from "../../types/config";

const props = defineProps<{
  runtime: ReturnType<typeof useConfigRuntime>;
  registry?: ReturnType<typeof useConfigRegistry>;
}>();

const {
  runtimeSettings,
  signalProcessingStatus,
  saveRuntime,
  fetchSignalProcessingStatus,
  runSignalProcessingScan,
  resetSignalProcessing,
  expireSignalProcessing,
  signalProgramLogs,
  fetchSignalProgramLogs,
  downloadSignalProgramLog,
  testSignalRule,
} = props.runtime;

const scanLimit = shallowRef(1000);
const ruleTestLimit = shallowRef(500);
const lastRuleTest = shallowRef<SignalRuleTestResponse | null>(null);

const signalSettings = computed(() => runtimeSettings.value.signalProcessing);

const kindOptions = computed(() =>
  (signalProcessingStatus.value.availableKinds || []).map((kind) => ({
    label: kind.label,
    value: kind.kind,
  })),
);

const trackedProgramOptions = computed(() =>
  (props.registry?.trackedItems.value || [])
    .map((item) => item.comm)
    .filter((comm): comm is string => Boolean(comm))
    .map((comm) => ({ value: comm, label: comm })),
);

const signalStats = computed(() => [
  {
    title: "Active states",
    value: signalProcessingStatus.value.activeStates,
  },
  {
    title: "Signal updates",
    value: signalProcessingStatus.value.updatedTotal,
  },
  {
    title: "Queue",
    value: `${signalProcessingStatus.value.queueLen}/${signalProcessingStatus.value.queueCap}`,
  },
  {
    title: "Expired",
    value: signalProcessingStatus.value.expiredTotal,
  },
]);

const fieldOptions = [
  { label: "Path / extra path", value: "path" },
  { label: "Event type", value: "eventType" },
  { label: "Command / child command", value: "childCommand" },
  { label: "Repeated read key", value: "readKey" },
  { label: "Process comm", value: "comm" },
  { label: "Extra info", value: "extraInfo" },
  { label: "Network endpoint", value: "netEndpoint" },
  { label: "Tool name", value: "toolName" },
  { label: "Working directory", value: "cwd" },
  { label: "Decision", value: "decision" },
  { label: "Custom target", value: "target" },
];

const operatorOptions = [
  { label: "exists", value: "exists" },
  { label: "contains", value: "contains" },
  { label: "equals", value: "equals" },
  { label: "prefix", value: "prefix" },
  { label: "suffix", value: "suffix" },
  { label: "regex", value: "regex" },
  { label: "not contains", value: "not_contains" },
  { label: "not equals", value: "not_equals" },
];

const defaultCondition = () => ({
  field: "path",
  operator: "contains",
  value: "",
});

const addRule = () => {
  const nextIndex = signalSettings.value.rules.length + 1;
  signalSettings.value.rules.push({
    id: `custom_signal_${Date.now()}`,
    name: `Custom signal ${nextIndex}`,
    enabled: true,
    kind: "custom",
    ttlSeconds: signalSettings.value.defaultTTLSeconds,
    weight: 1,
    conditions: [defaultCondition()],
  });
};

const removeRule = (index: number) => {
  signalSettings.value.rules.splice(index, 1);
};

const addCondition = (rule: SignalRule) => {
  rule.conditions.push(defaultCondition());
};

const removeCondition = (rule: SignalRule, index: number) => {
  rule.conditions.splice(index, 1);
};

const addSelectedProgram = () => {
  signalSettings.value.selectedPrograms.push({
    program: "",
    enabled: true,
    path: "",
  });
};

const removeSelectedProgram = (index: number) => {
  signalSettings.value.selectedPrograms.splice(index, 1);
};

const runScan = () => runSignalProcessingScan(scanLimit.value);

const scoreText = (score: number) => Number(score || 0).toFixed(2);

const runRuleTest = async (rule: SignalRule) => {
  lastRuleTest.value = await testSignalRule(rule, ruleTestLimit.value);
};

const refreshSignals = async () => {
  await Promise.allSettled([
    fetchSignalProcessingStatus(),
    fetchSignalProgramLogs(),
  ]);
};
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-alert
        type="info"
        show-icon
        message="Signal processing adds lazy, TTL-decayed runtime signals."
        description="Configure signal kinds, TTL decay, ANDed conditions, and selected-program protobuf binary log persistence. The worker updates only matching signal keys on event updates; a cron pass evicts expired states in parallel."
      />
    </a-col>

    <a-col :span="24">
      <a-card title="Signal Engine" size="small">
        <template #extra>
          <ThunderboltOutlined />
        </template>
        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :md="8">
            <div class="switch-row">
              <a-switch v-model:checked="runtimeSettings.signalProcessing.enabled" />
              <span>Enable signal processing</span>
            </div>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-form-item label="Queue size">
              <a-input-number
                v-model:value="runtimeSettings.signalProcessing.queueSize"
                :min="128"
                :max="65536"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-form-item label="Cron interval (seconds)">
              <a-input-number
                v-model:value="
                  runtimeSettings.signalProcessing.cronIntervalSeconds
                "
                :min="1"
                :max="86400"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-form-item label="Default TTL decay (seconds)">
              <a-input-number
                v-model:value="runtimeSettings.signalProcessing.defaultTTLSeconds"
                :min="1"
                :max="2592000"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-form-item label="Max active states">
              <a-input-number
                v-model:value="runtimeSettings.signalProcessing.maxStates"
                :min="128"
                :max="100000"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-form-item label="Proto log compression">
              <a-select
                v-model:value="
                  runtimeSettings.signalProcessing.protoLogCompression
                "
                :options="[{ label: 'gzip', value: 'gzip' }]"
              />
            </a-form-item>
          </a-col>
          <a-col :span="24">
            <div class="action-row">
              <a-button type="primary" @click="saveRuntime">
                Save signal config
              </a-button>
              <a-button @click="refreshSignals">
                <ReloadOutlined /> Refresh status
              </a-button>
              <a-input-number
                v-model:value="scanLimit"
                :min="1"
                :max="50000"
                addon-before="Scan recent"
              />
              <a-button @click="runScan">Queue scan</a-button>
              <a-button @click="expireSignalProcessing">Run TTL cron</a-button>
              <a-button danger @click="resetSignalProcessing">Reset states</a-button>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-row :gutter="[16, 16]">
        <a-col
          v-for="stat in signalStats"
          :key="stat.title"
          :xs="24"
          :md="6"
        >
          <a-card size="small">
            <a-statistic :title="stat.title" :value="stat.value" />
          </a-card>
        </a-col>
      </a-row>
    </a-col>

    <a-col :span="24">
      <a-card title="Selected Program Proto Logs" size="small">
        <template #extra>
          <div class="action-row">
            <a-button size="small" @click="fetchSignalProgramLogs">
              <ReloadOutlined /> Refresh logs
            </a-button>
            <a-button size="small" type="primary" @click="addSelectedProgram">
              <PlusOutlined /> Add program
            </a-button>
          </div>
        </template>
        <a-alert
          type="success"
          show-icon
          message="When a selected program matches event comm/path/basename, the backend appends a length-framed gzip protobuf ProgramSignalLogRecord locally."
          style="margin-bottom: 12px"
        />
        <a-empty
          v-if="signalSettings.selectedPrograms.length === 0"
          description="No selected programs yet."
        />
        <div v-else class="program-list">
          <div
            v-for="(program, index) in signalSettings.selectedPrograms"
            :key="`${program.program || 'program'}-${index}`"
            class="program-row"
          >
            <a-switch v-model:checked="program.enabled" />
            <a-auto-complete
              v-model:value="program.program"
              :options="trackedProgramOptions"
              placeholder="Program comm/path/basename, e.g. python, codex, /usr/bin/node"
            />
            <a-input
              v-model:value="program.path"
              placeholder="Optional log path; default ~/.config/agent-ebpf-filter/signals/program-logs/<program>.pb.gzlog"
            />
            <a-button danger ghost @click="removeSelectedProgram(index)">
              <DeleteOutlined />
            </a-button>
          </div>
        </div>
        <a-divider style="margin: 12px 0" />
        <a-empty
          v-if="signalProgramLogs.length === 0"
          description="No backend program log status yet. Save selected programs and refresh."
        />
        <a-table
          v-else
          size="small"
          :pagination="false"
          :data-source="signalProgramLogs"
          row-key="program"
        >
          <a-table-column title="Program" data-index="program" key="program" />
          <a-table-column title="Status" key="status">
            <template #default="{ record }">
              <a-tag :color="record.enabled ? 'green' : 'default'">
                {{ record.enabled ? "enabled" : "disabled" }}
              </a-tag>
              <a-tag :color="record.exists ? 'blue' : 'orange'">
                {{ record.exists ? `${record.frameCount} frames` : "not created" }}
              </a-tag>
            </template>
          </a-table-column>
          <a-table-column title="Size" key="size">
            <template #default="{ record }">
              {{ record.sizeBytes || 0 }} bytes
            </template>
          </a-table-column>
          <a-table-column title="Path" data-index="path" key="path" />
          <a-table-column title="Action" key="action">
            <template #default="{ record }">
              <a-button
                size="small"
                :disabled="!record.exists"
                @click="downloadSignalProgramLog(record.program)"
              >
                <DownloadOutlined /> Download
              </a-button>
            </template>
          </a-table-column>
        </a-table>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Signal Rules" size="small">
        <template #extra>
          <a-button size="small" type="primary" @click="addRule">
            <PlusOutlined /> Add signal
          </a-button>
        </template>
        <div class="rule-list">
          <a-card
            v-for="(rule, ruleIndex) in signalSettings.rules"
            :key="`${rule.id}-${ruleIndex}`"
            size="small"
            class="rule-card"
          >
            <template #title>
              <div class="rule-title">
                <a-switch v-model:checked="rule.enabled" />
                <a-input v-model:value="rule.name" placeholder="Signal name" />
                <a-tag color="blue">{{ rule.kind }}</a-tag>
              </div>
            </template>
            <template #extra>
              <div class="action-row">
                <a-input-number
                  v-model:value="ruleTestLimit"
                  size="small"
                  :min="1"
                  :max="50000"
                  addon-before="test"
                />
                <a-button size="small" @click="runRuleTest(rule)">
                  Dry run
                </a-button>
                <a-button
                  danger
                  ghost
                  size="small"
                  @click="removeRule(ruleIndex)"
                >
                  <DeleteOutlined />
                </a-button>
              </div>
            </template>
            <a-row :gutter="[12, 12]">
              <a-col :xs="24" :md="6">
                <a-form-item label="Rule ID">
                  <a-input v-model:value="rule.id" placeholder="stable id" />
                </a-form-item>
              </a-col>
              <a-col :xs="24" :md="6">
                <a-form-item label="Kind">
                  <a-select
                    v-model:value="rule.kind"
                    :options="kindOptions"
                    show-search
                  />
                </a-form-item>
              </a-col>
              <a-col :xs="24" :md="6">
                <a-form-item label="TTL seconds">
                  <a-input-number
                    v-model:value="rule.ttlSeconds"
                    :min="1"
                    :max="2592000"
                    style="width: 100%"
                  />
                </a-form-item>
              </a-col>
              <a-col :xs="24" :md="6">
                <a-form-item label="Weight">
                  <a-input-number
                    v-model:value="rule.weight"
                    :min="0.1"
                    :max="1000"
                    :step="0.1"
                    style="width: 100%"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <div class="conditions-header">
                  <strong>Conditions (AND)</strong>
                  <a-button size="small" @click="addCondition(rule)">
                    <PlusOutlined /> Add condition
                  </a-button>
                </div>
                <a-empty
                  v-if="rule.conditions.length === 0"
                  description="No conditions; built-in kind defaults apply."
                />
                <div v-else class="condition-list">
                  <div
                    v-for="(condition, conditionIndex) in rule.conditions"
                    :key="`${rule.id}-${condition.field}-${conditionIndex}`"
                    class="condition-row"
                  >
                    <a-select
                      v-model:value="condition.field"
                      :options="fieldOptions"
                      show-search
                    />
                    <a-select
                      v-model:value="condition.operator"
                      :options="operatorOptions"
                    />
                    <a-input
                      v-model:value="condition.value"
                      placeholder="match value or regex"
                    />
                    <a-button
                      danger
                      ghost
                      @click="removeCondition(rule, conditionIndex)"
                    >
                      <DeleteOutlined />
                    </a-button>
                  </div>
                </div>
              </a-col>
            </a-row>
          </a-card>
        </div>
        <a-alert
          v-if="lastRuleTest"
          type="info"
          show-icon
          style="margin-top: 12px"
          :message="`Rule ${lastRuleTest.rule.name} matched ${lastRuleTest.matchedTotal}/${lastRuleTest.scannedTotal} recent events`"
        />
        <a-list
          v-if="lastRuleTest && lastRuleTest.matches.length > 0"
          size="small"
          :data-source="lastRuleTest.matches"
          style="margin-top: 8px"
        >
          <template #renderItem="{ item }">
            <a-list-item :key="`${item.timestamp}-${item.pid}-${item.path}`">
              <a-list-item-meta>
                <template #title>
                  {{ item.comm || "unknown" }} · {{ item.eventType }}
                </template>
                <template #description>
                  {{ item.target || item.path || item.extraPath || "no target" }}
                </template>
              </a-list-item-meta>
              <a-tag color="blue">score {{ scoreText(item.wouldScore) }}</a-tag>
            </a-list-item>
          </template>
        </a-list>
      </a-card>
    </a-col>

    <a-col :span="24">
      <a-card title="Recent Signal States" size="small">
        <a-alert
          v-if="signalProcessingStatus.lastError"
          type="warning"
          show-icon
          :message="signalProcessingStatus.lastError"
          style="margin-bottom: 12px"
        />
        <a-empty
          v-if="signalProcessingStatus.recentStates.length === 0"
          description="No signal state has been updated yet."
        />
        <a-list
          v-else
          :data-source="signalProcessingStatus.recentStates"
          item-layout="vertical"
        >
          <template #renderItem="{ item }">
            <a-list-item :key="item.id">
              <a-list-item-meta>
                <template #title>
                  <span>{{ item.ruleName }} · {{ item.kind }}</span>
                </template>
                <template #description>
                  <span>
                    {{ item.comm || "unknown" }} · {{ item.target }} · count
                    {{ item.count }} · score {{ scoreText(item.score) }}
                  </span>
                </template>
              </a-list-item-meta>
              <div class="state-tags">
                <a-tag color="blue">ttl {{ item.ttlSeconds }}s</a-tag>
                <a-tag v-if="item.lastEventType" color="purple">
                  {{ item.lastEventType }}
                </a-tag>
                <a-tag v-if="item.pid">pid {{ item.pid }}</a-tag>
                <a-tag>expires {{ item.expiresAt }}</a-tag>
              </div>
            </a-list-item>
          </template>
        </a-list>
      </a-card>
    </a-col>
  </a-row>
</template>

<style scoped>
.switch-row,
.action-row,
.rule-title,
.conditions-header,
.state-tags {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.program-list,
.rule-list,
.condition-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.program-row,
.condition-row {
  display: grid;
  grid-template-columns: auto minmax(180px, 1fr) minmax(220px, 2fr) auto;
  gap: 8px;
  align-items: center;
}

.condition-row {
  grid-template-columns: minmax(160px, 1fr) minmax(140px, 0.8fr) minmax(
      220px,
      2fr
    ) auto;
}

.rule-card {
  border-color: #d6e4ff;
}

@media (max-width: 768px) {
  .program-row,
  .condition-row {
    grid-template-columns: 1fr;
  }
}
</style>
