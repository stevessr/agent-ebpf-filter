<script setup lang="ts">
import { computed } from "vue";
import {
  ImportOutlined,
  ExportOutlined,
  CopyOutlined,
  DeleteOutlined,
  FileOutlined,
  StopOutlined,
  AlertOutlined,
  SearchOutlined,
  PlusOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
  BookOutlined,
  GlobalOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import { getCategoryColor } from "../../../composables/config/useConfigRegistry";
import ConfigMLResearchTrainingCard from "./ConfigMLResearchTrainingCard.vue";
import ConfigMLInternetDatasetCard from "./ConfigMLInternetDatasetCard.vue";
import {
  classicSecurityDatasetPresets,
  highRiskPresets,
  safetyNetHighRiskPresets,
  syntheticExpansionPresets,
  type useConfigML,
} from "../../../composables/config/useConfigML";

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const {
  mlStatus,
  allSamples,
  loadingSamples,
  sampleTablePageSize,
  sampleSearchText,
  existingDataLimit,
  existingLabelMode,
  existingCommandCandidates,
  loadingExistingData,
  importingExistingData,
  existingDataSource,
  remoteDatasetUrl,
  remoteDatasetFormat,
  remoteDatasetLabelMode,
  remoteDatasetCleanSensitive,
  remoteDatasetLimit,
  agentLegalDatasetLimit,
  loadingRemoteDataset,
  importingRemoteDataset,
  importingAgentLegalDataset,
  remoteDatasetPreview,
  remoteDatasetMeta,
  researchSessions,
  selinuxPolicyDatasetLimit,
  importingSELinuxPolicyDataset,
  selectedResearchSessionId,
  selectedResearchSession,
  researchTrainingLabelPolicy,
  researchTrainingImportLimit,
  loadingResearchSessions,
  loadingResearchTraining,
  importingResearchTraining,
  exportingResearchTraining,
  researchTrainingPreview,
  researchTrainingPreviewSamples,
  researchTrainingImportResult,
  trainingDatasetImportInput,
  importingClassicDataset,
  dataMaskEnabled,
  sampleCommandLine,
  sampleLabel,
  submittingSample,
  filteredSamples,
  existingDuplicateCount,
  importableExistingCount,
  fetchAllSamples,
  fetchExistingCommandData,
  importExistingCommandData,
  fetchRemoteDatasetPreview,
  importRemoteDataset,
  importAgentLegalDataset,
  importSELinuxPolicyDataset,
  fetchResearchSessions,
  fetchResearchTrainingDataset,
  importResearchTrainingDataset,
  downloadResearchTrainingDataset,
  importClassicDataset,
  openClassicSecurityDatasetPage,
  copyClassicSecurityDatasetPage,
  maskSensitiveData,
  getLabelColor,
  labelSample,
  deleteSample,
  updateAnomaly,
  importTrainingDatasetFromFile,
  exportTrainingDataset,
  clearTrainingDataset,
  openTrainingDatasetImportPicker,
  submitManualSample,
  addPresetSample,
  importAllHighRiskPresets,
  importAllSafetyNetPresets,
  importAllSyntheticPresets,
  importAllInternetDatasets,
  importExpandedTrainingCorpus,
  fetchMLStatus,
} = props.ml;

void trainingDatasetImportInput;

const trainingSampleRowKey = (record: any) =>
  `${record.commandLine || [record.comm, ...(record.args || [])].filter(Boolean).join(" ")}:${record.label || ""}:${record.userLabel || ""}:${record.index ?? ""}`;

const downloadableInternetDatasetCount = classicSecurityDatasetPresets.filter(
  (preset) => Boolean(preset.downloadUrl || preset.bundledAsset),
).length;
const syntheticExpansionPresetCount = syntheticExpansionPresets.length;
const remoteDatasetQualityWarnings = computed(
  () => remoteDatasetMeta.value?.quality?.warnings || [],
);
const remoteDatasetParseWarnings = computed(
  () => remoteDatasetMeta.value?.parseWarnings || [],
);
const remoteDatasetWarningText = computed(() => {
  const quality = remoteDatasetQualityWarnings.value.join(", ");
  const parse = remoteDatasetParseWarnings.value
    .map((warning) =>
      [warning.source, warning.reason, warning.count ? `x${warning.count}` : ""]
        .filter(Boolean)
        .join(": "),
    )
    .join("; ");
  return [quality, parse].filter(Boolean).join(" | ");
});
const trainingReadiness = computed(() => mlStatus.value.training_readiness);
const trainingReadinessPercent = computed(() => {
  const readiness = trainingReadiness.value;
  if (!readiness?.minSamples) return 0;
  return Math.min(100, Math.round((readiness.labeledCount / readiness.minSamples) * 100));
});
const trainingReadinessAlertType = computed(() => {
  const readiness = trainingReadiness.value;
  if (!readiness) return "info";
  if (!readiness.ready) return "warning";
  return (readiness.warnings?.length || 0) > 0 ? "info" : "success";
});
const formatReadinessToken = (value: string) =>
  value.replaceAll("_", " ").replaceAll(":", ": ");
</script>

<template>
  <!-- Training Readiness Gate -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span><AlertOutlined /> 训练数据 Readiness / 质量门槛</span>
        <a-tag
          v-if="trainingReadiness"
          :color="trainingReadiness.ready ? 'green' : 'orange'"
          style="margin-left: 8px"
        >
          {{ trainingReadiness.ready ? "READY" : "BLOCKED" }}
        </a-tag>
      </template>
      <template #extra>
        <a-button size="small" type="link" @click="fetchMLStatus">
          <ReloadOutlined /> 刷新
        </a-button>
      </template>
      <template v-if="trainingReadiness">
        <a-alert
          show-icon
          :type="trainingReadinessAlertType"
          :message="
            trainingReadiness.ready
              ? '当前训练集满足最小样本数、类别数与特征归一化门槛'
              : '训练前需要先修复以下阻断项'
          "
          :description="
            trainingReadiness.ready
              ? '可以开始训练；如存在 warnings，建议先做去重、类别均衡或标签复核。'
              : (trainingReadiness.blockingReasons || [])
                  .map(formatReadinessToken)
                  .join('；')
          "
          style="margin-bottom: 12px"
        />
        <a-row :gutter="[12, 12]">
          <a-col :xs="24" :md="8">
            <a-card size="small">
              <a-statistic
                title="Labeled / Required"
                :value="`${trainingReadiness.labeledCount} / ${trainingReadiness.minSamples}`"
              />
              <a-progress
                :percent="trainingReadinessPercent"
                :status="trainingReadiness.ready ? 'success' : 'active'"
                size="small"
              />
            </a-card>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-card size="small">
              <a-statistic
                title="Classes"
                :value="`${trainingReadiness.classCount} / ${trainingReadiness.minClasses}`"
              />
              <a-space wrap size="small" style="margin-top: 8px">
                <a-tag
                  v-for="item in trainingReadiness.byLabel || []"
                  :key="item.key"
                  :color="getLabelColor(item.key)"
                >
                  {{ item.key }} {{ item.count }}
                </a-tag>
              </a-space>
            </a-card>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-card size="small">
              <a-statistic
                title="Feature Issues"
                :value="
                  (trainingReadiness.normalization?.nonFiniteValues || 0) +
                  (trainingReadiness.normalization?.belowZeroValues || 0) +
                  (trainingReadiness.normalization?.aboveOneValues || 0)
                "
              />
              <div style="font-size: 12px; color: #6b7280; margin-top: 8px">
                duplicate={{ trainingReadiness.quality?.duplicateCount || 0 }},
                unlabeled={{ trainingReadiness.unlabeledCount }}
              </div>
            </a-card>
          </a-col>
        </a-row>
        <a-space
          v-if="(trainingReadiness.warnings || []).length > 0"
          wrap
          size="small"
          style="margin-top: 10px"
        >
          <a-tag
            v-for="warning in trainingReadiness.warnings"
            :key="warning"
            color="orange"
          >
            {{ formatReadinessToken(warning) }}
          </a-tag>
        </a-space>
        <a-space
          v-if="(trainingReadiness.suggestedActions || []).length > 0"
          wrap
          size="small"
          style="margin-top: 8px"
        >
          <a-tag
            v-for="action in trainingReadiness.suggestedActions"
            :key="action"
            color="green"
          >
            {{ formatReadinessToken(action) }}
          </a-tag>
        </a-space>
      </template>
      <a-empty v-else description="等待 /config/ml/status 返回 readiness 信息" />
    </a-card>
  </a-col>

  <!-- Classic OS Security Datasets -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span><BookOutlined /> 经典 OS 安全数据集</span>
        <a-tag color="green" style="margin-left: 8px">支持一键导入</a-tag>
      </template>
      <a-alert
        type="info"
        show-icon
        style="margin-bottom: 12px"
        message="内置或有下载链接的数据集可一键导入；其他数据集会跳转官方页面，下载后用“导入本地文件”上传。导入器支持 zip, gz, tar, tgz, bz2 等归档及 JSON, JSONL, CSV, TSV, 纯文本。"
      />
      <a-list
        :data-source="classicSecurityDatasetPresets"
        :split="false"
        size="small"
      >
        <template #renderItem="{ item }">
          <a-list-item>
            <a-card size="small" style="width: 100%">
              <a-space direction="vertical" style="width: 100%">
                <div
                  style="
                    display: flex;
                    justify-content: space-between;
                    gap: 12px;
                    align-items: flex-start;
                    flex-wrap: wrap;
                  "
                >
                  <div>
                    <div style="font-weight: 600">{{ item.name }}</div>
                    <div style="color: #4a4a4a; font-size: 12px">
                      {{ item.note }}
                    </div>
                  </div>
                  <a-space wrap>
                    <a-tag color="blue">{{ item.family }}</a-tag>
                    <a-tag color="geekblue">{{ item.platform }}</a-tag>
                  </a-space>
                </div>
                <a-space wrap>
                  <a-button
                    size="small"
                    type="primary"
                    :loading="importingClassicDataset"
                    @click="importClassicDataset(item)"
                    ><ImportOutlined />
                    {{
                      item.downloadUrl || item.bundledAsset
                        ? "一键导入"
                        : "前往下载"
                    }}</a-button
                  >
                  <a-button
                    v-if="item.pageUrl"
                    size="small"
                    @click="openClassicSecurityDatasetPage(item)"
                    ><GlobalOutlined /> 打开官网</a-button
                  >
                  <a-button
                    v-if="item.pageUrl"
                    size="small"
                    @click="copyClassicSecurityDatasetPage(item)"
                    ><CopyOutlined /> 复制链接</a-button
                  >
                </a-space>
              </a-space>
            </a-card>
          </a-list-item>
        </template>
      </a-list>
    </a-card>
  </a-col>

  <!-- Dataset Expansion -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span><ReloadOutlined /> 数据集扩增</span>
        <a-tag color="purple" style="margin-left: 8px"
          >合成样本 + 互联网数据</a-tag
        >
      </template>
      <template #extra>
        <a-space wrap>
          <a-tag color="geekblue"
            >synthetic: {{ syntheticExpansionPresetCount }}</a-tag
          >
          <span style="font-size: 12px; color: #4a4a4a">agent legal</span>
          <a-input-number
            v-model:value="agentLegalDatasetLimit"
            :min="20"
            :max="500"
            size="small"
            style="width: 92px"
          />
          <a-button
            size="small"
            @click="importAgentLegalDataset()"
            :loading="importingAgentLegalDataset"
            ><ImportOutlined /> 导入合法 Agent 行为</a-button
          >
          <span style="font-size: 12px; color: #4a4a4a">SELinux rules</span>
          <a-input-number
            v-model:value="selinuxPolicyDatasetLimit"
            :min="10"
            :max="200"
            size="small"
            style="width: 92px"
          />
          <a-button
            size="small"
            @click="importSELinuxPolicyDataset()"
            :loading="importingSELinuxPolicyDataset"
            ><BookOutlined /> 导入 SELinux 规则</a-button
          >
          <a-tag color="blue"
            >一键导入数据集:
            {{ downloadableInternetDatasetCount }}</a-tag
          >
          <a-button
            size="small"
            @click="importAllSyntheticPresets()"
            :loading="importingClassicDataset"
            ><PlusOutlined /> 导入合成扩增样本</a-button
          >
          <a-button
            size="small"
            @click="importAllInternetDatasets()"
            :loading="importingClassicDataset"
            ><GlobalOutlined /> 导入全部互联网数据</a-button
          >
          <a-button
            size="small"
            type="primary"
            @click="importExpandedTrainingCorpus()"
            :loading="importingClassicDataset"
            ><ImportOutlined /> 扩增后立即训练</a-button
          >
        </a-space>
      </template>
      <a-alert
        type="info"
        show-icon
        message="合成扩增样本由命令模板自动生成，可快速补齐 ALLOW / ALERT / BLOCK 的边界样本；SELinux 规则会复用常见 allow/neverallow/dontaudit/auditallow/permissive 策略语义；互联网数据会批量拉取当前可直接下载的经典安全数据集。"
        description="如果你只想先看效果，可以先导入合成扩增样本或 SELinux 规则，再按需补充互联网数据。若要直接放大训练集并重新训练，直接点“扩增后立即训练”。"
      />
    </a-card>
  </a-col>

  <ConfigMLResearchTrainingCard :ml="props.ml" />

  <ConfigMLInternetDatasetCard :ml="props.ml" />

  <!-- Existing Command Data -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span>Existing Command Data</span>
        <a-tag color="cyan" style="margin-left: 8px"
          >拉取已有 wrapper / hook 事件</a-tag
        >
      </template>
      <template #extra>
        <a-space wrap>
          <span style="font-size: 12px; color: #4a4a4a">Limit</span>
          <a-input-number
            v-model:value="existingDataLimit"
            :min="10"
            :max="5000"
            size="small"
            style="width: 100px"
          />
          <a-select
            v-model:value="existingLabelMode"
            size="small"
            style="width: 150px"
          >
            <a-select-option value="unlabeled">导入为未标注</a-select-option>
            <a-select-option value="heuristic">按安全判断标注</a-select-option>
          </a-select>
          <a-button
            size="small"
            @click="fetchExistingCommandData()"
            :loading="loadingExistingData"
            ><ReloadOutlined /> 拉取已有数据</a-button
          >
          <a-button
            size="small"
            type="primary"
            @click="importExistingCommandData()"
            :loading="importingExistingData"
            :disabled="importableExistingCount <= 0"
            ><ImportOutlined /> 导入 {{ importableExistingCount }}</a-button
          >
        </a-space>
      </template>
      <a-alert
        type="info"
        show-icon
        style="margin-bottom: 12px"
        message="从 /events/recent 读取历史 wrapper_intercept / native_hook 命令。默认导入为未标注样本；选择“按安全判断标注”会用当前规则/ML/网络审计结果自动给出 ALLOW/ALERT/BLOCK 标签。"
      />
      <div
        style="
          display: flex;
          gap: 8px;
          align-items: center;
          margin-bottom: 8px;
          flex-wrap: wrap;
        "
      >
        <a-tag v-if="existingDataSource" color="blue"
          >source: {{ existingDataSource }}</a-tag
        >
        <a-tag color="purple"
          >{{ existingCommandCandidates.length }} pulled</a-tag
        >
        <a-tag color="default">{{ existingDuplicateCount }} duplicates</a-tag>
      </div>
      <a-table
        :dataSource="existingCommandCandidates"
        :pagination="{
          pageSize: 8,
          showSizeChanger: true,
          pageSizeOptions: ['8', '15', '30'],
        }"
        :scroll="{ x: 900 }"
        size="small"
        rowKey="commandLine"
      >
        <a-table-column
          title="Command"
          dataIndex="commandLine"
          :width="300"
          ellipsis
        >
          <template #default="{ record }"
            ><code>{{ maskSensitiveData(record.commandLine) }}</code></template
          >
        </a-table-column>
        <a-table-column title="Event" dataIndex="eventType" :width="120">
          <template #default="{ record }"
            ><a-tag size="small" color="geekblue">{{
              record.eventType
            }}</a-tag></template
          >
        </a-table-column>
        <a-table-column title="Category" dataIndex="category" :width="120">
          <template #default="{ record }">
            <a-tag
              v-if="record.category"
              :color="getCategoryColor(record.category)"
              size="small"
              >{{ record.category }}</a-tag
            >
            <span v-else style="color: #6b7280">—</span>
          </template>
        </a-table-column>
        <a-table-column title="Time" dataIndex="timestamp" :width="180">
          <template #default="{ record }"
            ><span style="font-size: 12px; color: #4a4a4a">{{
              record.timestamp
                ? new Date(record.timestamp).toLocaleString()
                : "—"
            }}</span></template
          >
        </a-table-column>
        <a-table-column title="State" dataIndex="duplicate" :width="100">
          <template #default="{ record }"
            ><a-tag
              :color="record.duplicate ? 'default' : 'green'"
              size="small"
              >{{ record.duplicate ? "已存在" : "可导入" }}</a-tag
            ></template
          >
        </a-table-column>
      </a-table>
    </a-card>
  </a-col>

  <!-- Training Data Browser -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span>Training Data Browser</span>
        <a-tag color="purple" style="margin-left: 8px"
          >{{ filteredSamples.length }} / {{ allSamples.length }}</a-tag
        >
      </template>
      <template #extra>
        <a-space wrap>
          <a-button
            size="small"
            @click="dataMaskEnabled = !dataMaskEnabled"
            :type="dataMaskEnabled ? 'primary' : 'default'"
          >
            <component
              :is="dataMaskEnabled ? EyeInvisibleOutlined : EyeOutlined"
            />
            {{ dataMaskEnabled ? "脱敏" : "明文" }}
          </a-button>
          <a-button size="small" @click="exportTrainingDataset()"
            ><ExportOutlined /> 导出训练集</a-button
          >
          <a-popconfirm
            title="确定要清空当前训练集吗？"
            @confirm="clearTrainingDataset()"
          >
            <a-button size="small" danger
              ><DeleteOutlined /> 清空训练集</a-button
            >
          </a-popconfirm>
          <a-input
            v-model:value="sampleSearchText"
            placeholder="搜索命令或参数..."
            size="small"
            style="width: 200px"
            allow-clear
          >
            <template #prefix><SearchOutlined /></template>
          </a-input>
          <a-button
            size="small"
            @click="fetchAllSamples()"
            :loading="loadingSamples"
            ><ReloadOutlined /> Refresh</a-button
          >
        </a-space>
      </template>
      <a-table
        :dataSource="filteredSamples"
        :pagination="{
          pageSize: sampleTablePageSize,
          showSizeChanger: true,
          pageSizeOptions: ['10', '15', '30', '50'],
          showTotal: (t: number) => `${t} samples`,
        }"
        :scroll="{ x: 1100 }"
        size="small"
        :rowKey="trainingSampleRowKey"
      >
        <a-table-column title="#" dataIndex="index" :width="50" />
        <a-table-column
          title="Command"
          dataIndex="commandLine"
          :width="240"
          ellipsis
        >
          <template #default="{ record }"
            ><code>{{
              maskSensitiveData(
                record.commandLine ||
                  [record.comm, ...(record.args || [])]
                    .filter(Boolean)
                    .join(" "),
              )
            }}</code></template
          >
        </a-table-column>
        <a-table-column title="Comm" dataIndex="comm" :width="100">
          <template #default="{ record }"
            ><strong>{{ record.comm }}</strong></template
          >
        </a-table-column>
        <a-table-column title="Args" dataIndex="args" :width="200" ellipsis>
          <template #default="{ record }"
            ><span style="font-size: 12px; color: #4a4a4a">{{
              maskSensitiveData((record.args || []).join(" ")) || "—"
            }}</span></template
          >
        </a-table-column>
        <a-table-column title="Category" dataIndex="category" :width="110">
          <template #default="{ record }"
            ><a-tag :color="getCategoryColor(record.category)" size="small">{{
              record.category
            }}</a-tag></template
          >
        </a-table-column>
        <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="100">
          <template #default="{ record }">
            <a-input-number
              v-model:value="record.anomalyScore"
              :min="0"
              :max="1"
              :step="0.01"
              :precision="2"
              size="small"
              style="width: 70px"
              @change="updateAnomaly(record.index, record.anomalyScore)"
            />
          </template>
        </a-table-column>
        <a-table-column title="Label" dataIndex="label" :width="90">
          <template #default="{ record }"
            ><a-tag :color="getLabelColor(record.label)" size="small">{{
              record.label
            }}</a-tag></template
          >
        </a-table-column>
        <a-table-column title="Actions" :width="240">
          <template #default="{ record }">
            <a-space :size="4">
              <a-button
                size="small"
                type="primary"
                ghost
                @click="labelSample(record.index, 'ALLOW')"
                :disabled="record.label === 'ALLOW'"
                >ALLOW</a-button
              >
              <a-button
                size="small"
                style="border-color: #faad14; color: #d48806"
                ghost
                @click="labelSample(record.index, 'ALERT')"
                :disabled="record.label === 'ALERT'"
                >ALERT</a-button
              >
              <a-button
                size="small"
                danger
                ghost
                @click="labelSample(record.index, 'BLOCK')"
                :disabled="record.label === 'BLOCK'"
                >BLOCK</a-button
              >
              <a-button
                size="small"
                danger
                type="text"
                @click="deleteSample(record.index)"
                ><DeleteOutlined
              /></a-button>
            </a-space>
          </template>
        </a-table-column>
      </a-table>
    </a-card>
  </a-col>

  <!-- Add Labeled Training Data -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span>Add Labeled Training Data</span>
        <a-tag color="blue" style="margin-left: 8px">手动添加标注样本</a-tag>
      </template>
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="14">
          <div
            style="
              display: flex;
              justify-content: space-between;
              align-items: center;
              margin-bottom: 8px;
            "
          >
            <div style="font-weight: 600">
              高危行为预设（点击即可添加已标注样本）
            </div>
            <a-button
              size="small"
              type="link"
              @click="importAllHighRiskPresets()"
              >一键导入全部预设</a-button
            >
          </div>
          <a-space wrap>
            <a-tag
              v-for="(p, i) in highRiskPresets"
              :key="i"
              :color="p.label === 'BLOCK' ? 'red' : 'orange'"
              style="cursor: pointer; padding: 4px 8px; font-size: 13px"
              @click="addPresetSample(p)"
            >
              {{ p.comm }} {{ p.args ? p.args.slice(0, 30) + "…" : "" }}
              <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
            </a-tag>
          </a-space>
        </a-col>
        <a-col :xs="24" :md="10">
          <div
            style="
              display: flex;
              justify-content: space-between;
              align-items: center;
              margin-bottom: 8px;
            "
          >
            <div style="font-weight: 600">Claude Code Safety Net 预设</div>
            <a-button
              size="small"
              type="link"
              @click="importAllSafetyNetPresets()"
              >一键导入全部</a-button
            >
          </div>
          <a-space wrap>
            <a-tag
              v-for="(p, i) in safetyNetHighRiskPresets"
              :key="'sn' + i"
              :color="
                p.label === 'BLOCK'
                  ? 'red'
                  : p.label === 'ALLOW'
                    ? 'green'
                    : 'orange'
              "
              style="cursor: pointer; padding: 4px 8px; font-size: 12px"
              @click="addPresetSample(p)"
            >
              <code>{{ p.comm }}</code> {{ p.args.slice(0, 35)
              }}{{ p.args.length > 35 ? "…" : "" }}
              <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
            </a-tag>
          </a-space>
        </a-col>
        <a-col :xs="24" :md="10">
          <div style="font-weight: 600; margin-bottom: 8px">
            Step 1: 输入完整命令行
          </div>
          <a-input
            v-model:value="sampleCommandLine"
            placeholder="完整命令 (支持管道: cat file.txt | grep error | wc -l)"
            size="small"
            style="margin-bottom: 10px"
            @keyup.enter="submitManualSample()"
          />
          <div style="font-weight: 600; margin-bottom: 8px">
            Step 2: 标注行为
            <a-tag color="processing" size="small">选择标签</a-tag>
          </div>
          <div style="display: flex; gap: 8px; margin-bottom: 6px">
            <a-radio-group
              v-model:value="sampleLabel"
              button-style="solid"
              size="small"
            >
              <a-radio-button
                value="BLOCK"
                style="border-color: #ff4d4f; color: #ff4d4f"
                ><StopOutlined /> BLOCK 拦截</a-radio-button
              >
              <a-radio-button
                value="ALERT"
                style="border-color: #faad14; color: #d48806"
                ><AlertOutlined /> ALERT 警报</a-radio-button
              >
              <a-radio-button
                value="ALLOW"
                style="border-color: #52c41a; color: #52c41a"
                ><span style="font-size: 11px">&#10003;</span> ALLOW
                放行</a-radio-button
              >
            </a-radio-group>
          </div>
          <div
            style="
              background: #fffbe6;
              border: 1px solid #ffe58f;
              border-radius: 4px;
              padding: 6px 10px;
              margin-bottom: 8px;
              font-size: 13px;
            "
            v-if="sampleCommandLine.trim()"
          >
            <div
              v-for="(cmd, idx) in sampleCommandLine
                .trim()
                .split('|')
                .map((c) => c.trim())
                .filter((c) => c)"
              :key="idx"
              style="margin-bottom: 2px"
            >
              <span style="color: #4a4a4a">{{ idx + 1 }}. </span>
              <strong>{{ cmd.split(/\s+/)[0] }}</strong>
              <span v-if="cmd.split(/\s+/).length > 1" style="color: #4a4a4a">
                {{ cmd.split(/\s+/).slice(1).join(" ").slice(0, 30)
                }}{{
                  cmd.split(/\s+/).slice(1).join(" ").length > 30 ? "…" : ""
                }}</span
              >
              <span style="color: #4a4a4a"> → </span>
              <a-tag
                :color="
                  sampleLabel === 'BLOCK'
                    ? 'red'
                    : sampleLabel === 'ALERT'
                      ? 'orange'
                      : 'green'
                "
                size="small"
                >{{ sampleLabel }}</a-tag
              >
            </div>
          </div>
          <a-button
            type="primary"
            @click="submitManualSample()"
            :loading="submittingSample"
            block
            ><PlusOutlined /> 添加此标注样本</a-button
          >
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
