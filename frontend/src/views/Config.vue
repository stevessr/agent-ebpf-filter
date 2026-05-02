<script setup lang="ts">
import { ref, onMounted, watch, defineAsyncComponent } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  PlusOutlined,
  TagOutlined,
  AppstoreOutlined,
  FolderOutlined,
  ExportOutlined,
  ImportOutlined,
  SafetyCertificateOutlined,
  BookOutlined,
  ClusterOutlined,
  SwapOutlined,
  StopOutlined,
  AlertOutlined,
  ArrowRightOutlined,
  CopyOutlined,
  ReloadOutlined,
  DeleteOutlined,
  FileOutlined,
  GlobalOutlined,
  ThunderboltOutlined,
  ControlOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
  SearchOutlined,
  DownloadOutlined,
} from "@ant-design/icons-vue";
import PathNavigatorDrawer from "../components/PathNavigatorDrawer.vue";
import DocsLookupPanel from "../components/docs/DocsLookupPanel.vue";
import { useConfigRegistry, getCategoryColor } from "../composables/useConfigRegistry";
import { useConfigSecurity } from "../composables/useConfigSecurity";
import { useConfigRuntime } from "../composables/useConfigRuntime";
import { useConfigML } from "../composables/useConfigML";
import { useConfigCluster } from "../composables/useConfigCluster";
import { quickRulePresets, externalRuleSources, syscallGroups } from "../composables/useConfigSecurity";
import { classicSecurityDatasetPresets, highRiskPresets, safetyNetHighRiskPresets } from "../composables/useConfigML";

// ── Composable Instantiations ──
const registry = useConfigRegistry();
const security = useConfigSecurity();
const runtime = useConfigRuntime();
const ml = useConfigML();
const cluster = useConfigCluster();

// Destructure commonly used values (templates reference these directly)
const {
  tags,
  newTagName, newCommName, newCommTag, newPathName, newPathTag, newPrefixValue, newPrefixTag,
  pathPickerOpen, pathPickerTarget,
  fetchTags, fetchTrackedComms, fetchTrackedPaths, fetchTrackedPrefixes,
  openImportPicker,
  addTag, addComm, removeComm, toggleCommDisabled,
  addPath, removePath, addPrefix, removePrefix,
  exportConfig, importConfig, clearAllConfig,
  openPathPicker, handlePathPicked,
  groupedTrackedItems, groupedTrackedPaths, groupedTrackedPrefixes,
} = registry;

const {
  wrapperRules,
  newRuleComm, newRuleAction, newRuleRewritten,
  newRuleRegex, newRuleReplacement, newRulePriority, previewTestInput,
  disabledEventTypes,
  fetchedExternalRules, fetchSourceLoading, importingExternalRules,
  fetchRules, saveRule, deleteRule,
  addQuickRulePreset, addAllQuickRulePresets,
  fetchExternalRules, importAllFetchedRules,
  fetchDisabledEventTypes, toggleEventType,
  regexPreviewResult,
} = security;

const {
  runtimeSettings,
  mcpEndpoint,
  persistedEventLogPath, persistedEventLogAlive,
  fetchRuntime, saveRuntime,
  rotateAccessToken, clearInMemoryEvents, clearPersistedLog, clearAllEvents,
  copyText, mcpQueryEndpoint, mcpQueryEndpointTemplate,
} = runtime;

const {
  mlEnabled, mlStatus, trainingModel, feedbackComm, feedbackAction,
  mlThresholds, mlTrainingConfig, llmScoringConfig, llmBatchConfig,
  llmBatchResponse, llmBatchLoading, trainingLogs,
  trainingHistory,
  hyperParams,
  allSamples, loadingSamples, sampleTablePageSize, sampleSearchText,
  existingDataLimit, existingLabelMode, existingCommandCandidates,
  loadingExistingData, importingExistingData, existingDataSource,
  remoteDatasetUrl, remoteDatasetFormat, remoteDatasetLabelMode, remoteDatasetLimit,
  loadingRemoteDataset, importingRemoteDataset, remoteDatasetPreview, remoteDatasetMeta,
  llmProductionDatasetLimit, llmProductionAllowHeuristic, llmProductionDeduplicate,
  llmProductionLoading, llmProductionPreview, llmProductionMeta,
  trainingDatasetImportInput, importingClassicDataset, dataMaskEnabled,
  sampleCommandLine, sampleLabel, submittingSample,
  backtestCommandLine, backtesting, backtestResult,
  fetchMLStatus, trainingChartOptions, trainingChartSeries,
  submitFeedback, saveMLThresholds, runLLMBatchScore, llmBatchRowKey, llmBatchCanApplyLabels,
  filteredSamples, existingDuplicateCount, importableExistingCount,
  fetchAllSamples, fetchExistingCommandData, importExistingCommandData,
  fetchRemoteDatasetPreview, importRemoteDataset,
  fetchLLMProductionDataset, exportLLMProductionDataset,
  importClassicDataset, openClassicSecurityDatasetPage, copyClassicSecurityDatasetPage,
  maskSensitiveData,
  labelSample, deleteSample, updateAnomaly,
  importTrainingDatasetFromFile, exportTrainingDataset, clearTrainingDataset,
  openTrainingDatasetImportPicker, getLabelColor, trainWithParams,
  importAllSafetyNetPresets, submitManualSample, addPresetSample, importAllHighRiskPresets,
  runBacktest, runBacktestPreset, riskLevelColor, riskMeterColor,
} = ml;

void trainingDatasetImportInput;

const {
  clusterState, clusterNodes, selectedClusterTarget,
  fetchClusterState, fetchClusterNodes,
  updateClusterTargetFromStorage, applyClusterTarget,
  getClusterRowClass,
  clusterNodeOptions, clusterRoleText, clusterRoleColor,
} = cluster;

// Lazy-loaded chart component (used in ML tab)
const VueApexCharts = defineAsyncComponent(() => import('vue3-apexcharts'));

// Routing state (kept in Config.vue for tab sync)
const route = useRoute();
const router = useRouter();

const activeTabKey = ref(route.params.tab as string || 'registry');
const registryTabKey = ref(route.params.subtab as string || 'tags');
const mlSubTabKey = ref((route.params.subsubtab as string) || (route.params.subtab as string) || localStorage.getItem('config_ml_subtab') || 'status');

watch(() => [route.params.tab, route.params.subtab, route.params.subsubtab], ([tab, subtab, subsub]) => {
  if (tab) activeTabKey.value = tab as string;
  if (subtab) registryTabKey.value = subtab as string;
  // For ML we accept either the second or third segment for backward compatibility
  if (subsub) mlSubTabKey.value = subsub as string;
  else if (tab === 'ml' && subtab) mlSubTabKey.value = subtab as string;
});

watch(activeTabKey, (val) => {
  if (val !== route.params.tab) {
    router.replace({
      name: 'Config',
      params: {
        tab: val,
        subtab: val === 'registry' ? registryTabKey.value : (val === 'ml' ? mlSubTabKey.value : undefined),
      },
    });
  }
});

watch(registryTabKey, (val) => {
  if (activeTabKey.value === 'registry' && val !== route.params.subtab) {
    router.replace({ name: 'Config', params: { tab: activeTabKey.value, subtab: val } });
  }
});

watch(mlSubTabKey, (val) => {
  localStorage.setItem('config_ml_subtab', val);
  if (activeTabKey.value === 'ml' && val !== (route.params.subsubtab || route.params.subtab)) {
    // Prefer the short (second segment) URL for ML subtabs
    router.replace({ name: 'Config', params: { tab: 'ml', subtab: val } });
  }
});


onMounted(async () => {
  updateClusterTargetFromStorage();
  await fetchClusterState();
  await fetchClusterNodes();
  await fetchRuntime();
  fetchTags();
  fetchTrackedComms();
  fetchTrackedPaths();
  fetchTrackedPrefixes();
  fetchRules();
  fetchDisabledEventTypes();
  await fetchMLStatus();
  fetchAllSamples();
  fetchExistingCommandData(true);
});
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%">
    <a-tabs
      v-model:activeKey="activeTabKey"
      type="card"
      size="large"
      :destroyInactiveTabPane="false"
    >
      <!-- Tab 1: eBPF Registry -->
      <a-tab-pane key="registry" tab="eBPF Registry">
        <template #tab>
          <span><TagOutlined /> eBPF Registry</span>
        </template>

        <a-tabs v-model:activeKey="registryTabKey" size="small">
          <!-- Sub-tab 1.1: Tags & Global Management -->
          <a-tab-pane key="tags" tab="Tags & Global">
            <a-row :gutter="[24, 24]">
              <a-col :span="24">
                <a-card title="Global Registry & Actions" size="small">
                  <template #extra>
                    <div style="display: flex; gap: 8px; align-items: center">
                      <input
                        type="file"
                        ref="importFileInput"
                        @change="importConfig"
                        style="display: none"
                        accept=".json"
                      />
                      <a-button size="small" @click="openImportPicker"
                        ><ImportOutlined /> Import</a-button
                      >
                      <a-button size="small" @click="exportConfig"
                        ><ExportOutlined /> Export</a-button
                      >
                      <a-popconfirm
                        title="Are you sure you want to clear all configurations?"
                        @confirm="clearAllConfig"
                      >
                        <a-button size="small" danger>Clear All</a-button>
                      </a-popconfirm>
                      <a-divider type="vertical" />
                      <TagOutlined />
                    </div>
                  </template>
                  <div style="display: flex; flex-direction: column; gap: 16px">
                    <div style="display: flex; gap: 8px; align-items: center">
                      <span style="color: #888; font-size: 13px; width: 80px"
                        >Add Tag:</span
                      >
                      <div style="display: flex; width: 320px">
                        <a-input
                          v-model:value="newTagName"
                          placeholder="New tag name..."
                          @pressEnter="addTag"
                          style="
                            border-top-right-radius: 0;
                            border-bottom-right-radius: 0;
                          "
                        />
                        <a-button
                          type="primary"
                          @click="addTag"
                          style="
                            border-top-left-radius: 0;
                            border-bottom-left-radius: 0;
                          "
                        >
                          <PlusOutlined />
                        </a-button>
                      </div>
                    </div>
                    <div
                      style="display: flex; gap: 8px; align-items: flex-start"
                    >
                      <span
                        style="
                          color: #888;
                          font-size: 13px;
                          width: 80px;
                          margin-top: 4px;
                        "
                        >Registered:</span
                      >
                      <div
                        style="
                          display: flex;
                          flex-wrap: wrap;
                          gap: 8px;
                          flex: 1;
                        "
                      >
                        <a-tag
                          v-for="tag in tags"
                          :key="tag"
                          :color="getCategoryColor(tag)"
                          >{{ tag }}</a-tag
                        >
                      </div>
                    </div>
                  </div>
                </a-card>
              </a-col>
              <a-col :span="24">
                <a-alert
                  type="info"
                  show-icon
                  message="Tags are used to categorize tracked processes, paths, and prefixes. They provide semantic context in the Monitor and Network views."
                />
              </a-col>
            </a-row>
          </a-tab-pane>

          <!-- Sub-tab 1.2: Tracked Binaries -->
          <a-tab-pane key="binaries" tab="Tracked Binaries">
            <a-row :gutter="[24, 24]">
              <a-col :span="24">
                <a-card title="Tracked Executables" size="small">
                  <template #extra><AppstoreOutlined /></template>
                  <div
                    style="
                      margin-bottom: 16px;
                      background: #fafafa;
                      padding: 12px;
                      border-radius: 8px;
                      display: flex;
                      gap: 8px;
                    "
                  >
                    <a-input
                      v-model:value="newCommName"
                      placeholder="Binary name (e.g. curl, git, python)"
                      style="flex: 2"
                    />
                    <a-select
                      v-model:value="newCommTag"
                      style="flex: 1"
                      placeholder="Assign Tag"
                    >
                      <a-select-option
                        v-for="tag in tags"
                        :key="tag"
                        :value="tag"
                        >{{ tag }}</a-select-option
                      >
                    </a-select>
                    <a-button type="primary" @click="addComm"
                      ><PlusOutlined /> Add</a-button
                    >
                  </div>
                  <a-row :gutter="[16, 16]">
                    <a-col
                      v-for="(comms, tag) in groupedTrackedItems"
                      :key="tag"
                      :xs="24"
                      :md="12"
                      :xl="8"
                    >
                      <div
                        style="
                          padding: 12px;
                          border: 1px solid #f0f0f0;
                          border-radius: 8px;
                          height: 100%;
                        "
                      >
                        <div
                          style="
                            margin-bottom: 8px;
                            border-bottom: 1px solid #f5f5f5;
                            padding-bottom: 4px;
                          "
                        >
                          <a-typography-text strong>{{
                            tag
                          }}</a-typography-text>
                        </div>
                        <div style="display: flex; flex-wrap: wrap; gap: 6px">
                          <a-tag
                            v-for="entry in comms"
                            :key="entry.comm"
                            closable
                            @close.prevent="removeComm(entry.comm!)"
                            :color="entry.disabled ? 'default' : getCategoryColor(tag)"
                          >
                            <span
                              :style="entry.disabled ? 'text-decoration: line-through; opacity: 0.55; cursor: pointer;' : 'cursor: pointer;'"
                              @click.stop="toggleCommDisabled(entry.comm!, entry.disabled || false)"
                            >{{ entry.comm }}</span>
                            <span v-if="entry.disabled" style="margin-left: 4px; font-size: 10px; opacity: 0.7;">off</span>
                          </a-tag>
                        </div>
                      </div>
                    </a-col>
                  </a-row>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>

          <!-- Sub-tab 1.3: Tracked Paths -->
          <a-tab-pane key="paths" tab="Paths & Prefixes">
            <a-row :gutter="[24, 24]">
              <a-col :xs="24" :lg="12">
                <a-card title="Exact File Paths" size="small">
                  <template #extra>
                    <a-space>
                      <a-tooltip title="Browse files">
                        <FolderOutlined style="cursor: pointer; color: #1890ff;" @click="openPathPicker('exact')" />
                      </a-tooltip>
                      <FolderOutlined />
                    </a-space>
                  </template>
                  <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px; display: flex; gap: 8px;">
                    <a-input v-model:value="newPathName" placeholder="Absolute path" style="flex: 2" />
                    <a-select v-model:value="newPathTag" style="flex: 1" placeholder="Tag">
                      <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
                    </a-select>
                    <a-button type="primary" @click="addPath"><PlusOutlined /></a-button>
                  </div>
                  <div v-for="(paths, tag) in groupedTrackedPaths" :key="tag" style="margin-bottom: 12px;">
                    <div style="margin-bottom: 4px;"><a-typography-text strong>{{ tag }}</a-typography-text></div>
                    <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                      <a-tag v-for="p in paths" :key="p.path" closable @close.prevent="removePath(p.path!)" :color="getCategoryColor(tag)">{{ p.path }}</a-tag>
                    </div>
                  </div>
                </a-card>
              </a-col>

              <a-col :xs="24" :lg="12">
                <a-card title="Path Prefixes (LPM)" size="small">
                  <template #extra>
                    <a-space>
                      <a-tooltip title="Browse directories">
                        <FolderOutlined style="cursor: pointer; color: #1890ff;" @click="openPathPicker('prefix')" />
                      </a-tooltip>
                      <FolderOutlined />
                    </a-space>
                  </template>
                  <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px; display: flex; gap: 8px;">
                    <a-input v-model:value="newPrefixValue" placeholder="Path prefix (e.g. /etc)" style="flex: 2" />
                    <a-select v-model:value="newPrefixTag" style="flex: 1" placeholder="Tag">
                      <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
                    </a-select>
                    <a-button type="primary" @click="addPrefix"><PlusOutlined /></a-button>
                  </div>
                  <a-alert
                    type="info"
                    show-icon
                    style="margin-bottom: 12px;"
                    message="Prefix matching applies to descendant paths."
                  />
                  <div v-for="(prefixes, tag) in groupedTrackedPrefixes" :key="tag" style="margin-bottom: 12px;">
                    <div style="margin-bottom: 4px;"><a-typography-text strong>{{ tag }}</a-typography-text></div>
                    <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                      <a-tag v-for="prefix in prefixes" :key="prefix.prefix" closable @close.prevent="removePrefix(prefix.prefix!)" :color="getCategoryColor(tag)">{{ prefix.prefix }}</a-tag>
                    </div>
                  </div>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>
        </a-tabs>
      </a-tab-pane>

      <!-- Tab 2: Security Policies -->
      <a-tab-pane key="security" tab="Security Policies">
        <template #tab>
          <span><SafetyCertificateOutlined /> Security Policies</span>
        </template>
        <a-row :gutter="[24, 24]">
          <a-col :span="24">
            <a-card title="Wrapper Security Policies" size="small">
              <template #extra><SafetyCertificateOutlined /></template>
              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px;"
                message="快捷按钮会按 comm 精确匹配直接写入规则；更细的参数条件仍可在下方手动补充 regex、rewrite 或 priority。"
              />
              <div style="margin-bottom: 16px; background: #fafafa; padding: 16px; border-radius: 8px;">
                <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 10px;">
                  <div>
                    <div style="font-weight: 600;">典型规则快捷添加</div>
                    <div style="font-size: 12px; color: #999;">
                      参考 Gemini CLI / Codex / Hermes 的常见高风险命令，点击即可写入预设规则。
                    </div>
                  </div>
                  <a-button size="small" type="link" @click="addAllQuickRulePresets">一键添加全部</a-button>
                </div>
                <a-space wrap>
                  <a-tooltip
                    v-for="preset in quickRulePresets"
                    :key="`${preset.comm}-${preset.action}`"
                    :title="`${preset.source} · ${preset.summary}`"
                  >
                    <a-button
                      size="small"
                      :type="preset.action === 'BLOCK' ? 'primary' : 'default'"
                      :danger="preset.action === 'BLOCK'"
                      :style="preset.action === 'ALERT' ? 'border-color: #faad14; color: #d48806;' : ''"
                      @click="addQuickRulePreset(preset)"
                    >
                      <component :is="preset.action === 'BLOCK' ? StopOutlined : AlertOutlined" />
                      <span style="margin-left: 4px;">{{ preset.comm }}</span>
                      <span style="margin-left: 4px; opacity: 0.72;">{{ preset.action }}</span>
                    </a-button>
                  </a-tooltip>
                </a-space>
              </div>

              <!-- External Rule Import -->
              <div style="margin-bottom: 16px; background: #fafafa; padding: 16px; border-radius: 8px;">
                <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 10px;">
                  <div>
                    <div style="font-weight: 600;">🌐 外部规则一键获取</div>
                    <div style="font-size: 12px; color: #999;">
                      从社区维护的 AI 代理安全规则集中获取最新规则，支持预览后一键导入。
                    </div>
                  </div>
                </div>
                <a-row :gutter="[12, 12]">
                  <a-col v-for="source in externalRuleSources" :key="source.id" :xs="24" :sm="8">
                    <a-card size="small" :hoverable="true">
                      <div style="font-size: 13px; font-weight: 600; margin-bottom: 4px;">{{ source.name }}</div>
                      <div style="font-size: 11px; color: #999; margin-bottom: 8px; min-height: 32px;">{{ source.description }}</div>
                      <div style="display: flex; gap: 6px;">
                        <a-tag :color="source.category === 'owasp' ? 'orange' : 'blue'" style="font-size: 10px;">
                          {{ source.category === 'owasp' ? 'OWASP' : '社区' }}
                        </a-tag>
                        <a-button
                          size="small"
                          type="primary"
                          ghost
                          :loading="fetchSourceLoading === source.id"
                          @click="fetchExternalRules(source)"
                        >
                          <DownloadOutlined /> 获取
                        </a-button>
                      </div>
                    </a-card>
                  </a-col>
                </a-row>
                <div v-if="fetchedExternalRules.length > 0" style="margin-top: 12px;">
                  <a-alert
                    type="success" show-icon
                    :message="`已获取 ${fetchedExternalRules.length} 条规则`"
                    style="margin-bottom: 8px;"
                  />
                  <a-table
                    :dataSource="fetchedExternalRules"
                    :columns="[{ title:'Command', dataIndex:'comm', key:'comm' }, { title:'Action', dataIndex:'action', key:'action' }, { title:'Priority', dataIndex:'priority', key:'priority' }]"
                    size="small"
                    :pagination="false"
                    rowKey="comm"
                    :scroll="{ y: 200 }"
                  >
                    <template #bodyCell="{ column, record }">
                      <template v-if="column.key === 'comm'"><code>{{ record.comm }}</code></template>
                      <template v-if="column.key === 'action'">
                        <a-tag :color="record.action === 'BLOCK' ? 'red' : 'orange'">{{ record.action }}</a-tag>
                      </template>
                    </template>
                  </a-table>
                  <div style="margin-top: 8px; text-align: right;">
                    <a-button
                      type="primary"
                      size="small"
                      :loading="importingExternalRules"
                      @click="importAllFetchedRules"
                    >
                      <ImportOutlined /> 一键导入 {{ fetchedExternalRules.length }} 条规则
                    </a-button>
                  </div>
                </div>
              </div>

              <div style="margin-bottom: 16px; background: #fafafa; padding: 16px; border-radius: 8px;">
                <a-row :gutter="[16, 16]" align="middle">
                  <a-col :xs="24" :md="5">
                    <a-input v-model:value="newRuleComm" placeholder="Command (e.g. rm)" />
                  </a-col>
                  <a-col :xs="24" :md="4">
                    <a-select v-model:value="newRuleAction" style="width: 100%">
                      <a-select-option value="BLOCK">Block Execution</a-select-option>
                      <a-select-option value="REWRITE">Rewrite Command</a-select-option>
                      <a-select-option value="ALERT">Alert Only</a-select-option>
                    </a-select>
                  </a-col>
                  <a-col :xs="24" :md="3">
                    <a-input-number v-model:value="newRulePriority" :min="0" placeholder="Priority" style="width: 100%" />
                  </a-col>
                  <template v-if="newRuleAction === 'REWRITE'">
                    <a-col :xs="24" :md="4">
                      <a-input v-model:value="newRuleRegex" placeholder="Regex (Optional)" />
                    </a-col>
                    <a-col :xs="24" :md="4">
                      <a-input v-model:value="newRuleReplacement" placeholder="Replacement" />
                    </a-col>
                    <a-col :xs="24" :md="4" v-if="!newRuleRegex">
                      <a-input v-model:value="newRuleRewritten" placeholder="Fixed cmd" />
                    </a-col>
                  </template>
                  <a-col v-else :xs="24" :md="12">
                    <span style="color: #999; font-size: 12px;">Intercepts and blocks or warns when the command is called via agent-wrapper</span>
                  </a-col>
                  
                  <a-col :xs="24" :span="24" v-if="newRuleRegex" style="margin-top: 8px;">
                    <div style="background: #e6f7ff; padding: 12px; border-radius: 4px; border: 1px solid #91caff;">
                       <div style="font-size: 12px; font-weight: bold; margin-bottom: 8px; color: #003a8c;">Regex Live Preview:</div>
                       <a-row :gutter="8" align="middle">
                         <a-col :span="11">
                           <a-input v-model:value="previewTestInput" size="small" placeholder="Type example command arguments to test..." />
                         </a-col>
                         <a-col :span="2" style="text-align: center;">
                           <ArrowRightOutlined />
                         </a-col>
                         <a-col :span="11">
                           <div style="background: #fff; padding: 4px 11px; border: 1px solid #d9d9d9; border-radius: 2px; min-height: 24px; font-family: monospace;">
                             {{ regexPreviewResult || '(Result will appear here)' }}
                           </div>
                         </a-col>
                       </a-row>
                    </div>
                  </a-col>

                  <a-col :xs="24" :md="24" style="text-align: right; margin-top: 8px;">
                    <a-button type="primary" @click="saveRule"><PlusOutlined /> Add Policy</a-button>
                  </a-col>
                </a-row>
              </div>

              <a-table :dataSource="Object.values(wrapperRules).sort((a,b) => (b.priority || 0) - (a.priority || 0))" size="small" rowKey="comm" :pagination="false">
                <a-table-column title="Priority" dataIndex="priority" key="priority" width="80px">
                  <template #default="{ text }"><a-tag color="blue">{{ text || 0 }}</a-tag></template>
                </a-table-column>
                <a-table-column title="Intercepted Command" dataIndex="comm" key="comm">
                  <template #default="{ text }"><code>{{ text }}</code></template>
                </a-table-column>
                <a-table-column title="Action" dataIndex="action" key="action">
                  <template #default="{ text }">
                    <a-tag :color="text === 'BLOCK' ? 'red' : (text === 'REWRITE' ? 'blue' : 'orange')">
                      <component :is="text === 'BLOCK' ? StopOutlined : (text === 'REWRITE' ? SwapOutlined : AlertOutlined)" />
                      {{ text }}
                    </a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Logic" key="logic">
                  <template #default="{ record }">
                    <div v-if="record.action === 'REWRITE'">
                      <div v-if="record.regex">
                        <a-tag color="cyan">Regex</a-tag> <code>{{ record.regex }}</code>
                        <div style="margin-top: 4px;"><ArrowRightOutlined /> <code>{{ record.replacement }}</code></div>
                      </div>
                      <div v-else-if="record.rewritten_cmd">
                        <a-tag color="blue">Fixed</a-tag> <code>{{ record.rewritten_cmd.join(' ') }}</code>
                      </div>
                    </div>
                    <span v-else>-</span>
                  </template>
                </a-table-column>
                <a-table-column title="Remove" key="action" width="100px">
                  <template #default="{ record }">
                    <a-button type="link" danger @click="deleteRule(record.comm)">Delete</a-button>
                  </template>
                </a-table-column>
              </a-table>
            </a-card>
          </a-col>

          <!-- eBPF Syscall Interception -->
          <a-col :span="24">
            <a-card title="eBPF Syscall Interception" size="small">
              <template #extra>
                <a-tag color="green">{{ syscallGroups.reduce((c, g) => c + g.syscalls.length, 0) }} syscalls monitored</a-tag>
              </template>
              <a-alert
                type="info" show-icon style="margin-bottom: 16px;"
                message="Toggle individual syscall monitoring. Disabled syscalls are silently dropped in the kernel event pipeline — no events will be generated for them."
              />
              <a-row :gutter="[16, 16]">
                <a-col v-for="group in syscallGroups" :key="group.key" :xs="24" :sm="12" :lg="6">
                  <div style="border: 1px solid #f0f0f0; border-radius: 8px; overflow: hidden; height: 100%;">
                    <div :style="`background: ${group.color}; color: #fff; padding: 10px 14px; display: flex; align-items: center; gap: 8px;`">
                      <FileOutlined v-if="group.icon === 'file'" />
                      <FolderOutlined v-else-if="group.icon === 'folder'" />
                      <GlobalOutlined v-else-if="group.icon === 'global'" />
                      <ThunderboltOutlined v-else-if="group.icon === 'thunderbolt'" />
                      <ControlOutlined v-else-if="group.icon === 'control'" />
                      <SafetyCertificateOutlined v-else-if="group.icon === 'safety'" />
                      <AppstoreOutlined v-else />
                      <span style="font-weight: 600; font-size: 13px;">{{ group.title }}</span>
                      <span style="margin-left: auto; font-size: 11px; opacity: 0.85;">{{ group.syscalls.filter(s => !disabledEventTypes.has(s.type)).length }}/{{ group.syscalls.length }}</span>
                    </div>
                    <div style="padding: 0;">
                      <div
                        v-for="s in group.syscalls" :key="s.type"
                        style="display: flex; align-items: center; justify-content: space-between; padding: 7px 14px; border-bottom: 1px solid #fafafa; transition: background 0.15s;"
                        :style="disabledEventTypes.has(s.type) ? 'opacity: 0.45;' : ''"
                      >
                        <div style="min-width: 0; flex: 1;">
                          <div style="font-size: 12px; font-weight: 600; font-family: monospace; color: #1f1f1f;">{{ s.name }}</div>
                          <div style="font-size: 11px; color: #999; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{{ s.desc }}</div>
                        </div>
                        <a-switch
                          :checked="!disabledEventTypes.has(s.type)"
                          size="small"
                          @change="toggleEventType(s.type, disabledEventTypes.has(s.type))"
                        >
                          <template #checkedChildren><EyeOutlined /></template>
                          <template #unCheckedChildren><EyeInvisibleOutlined /></template>
                        </a-switch>
                      </div>
                    </div>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- Tab 3: System & Runtime -->
      <a-tab-pane key="system" tab="System & Runtime">
        <template #tab>
          <span><ReloadOutlined /> System & Runtime</span>
        </template>
        <a-row :gutter="[24, 24]">
          <a-col :span="24">
            <a-card title="Runtime & MCP Access" size="small">
              <template #extra>
                <SafetyCertificateOutlined />
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div style="display: flex; align-items: center; gap: 12px">
                      <a-switch
                        v-model:checked="runtimeSettings.logPersistenceEnabled"
                      />
                      <span>Persist captured logs to file</span>
                    </div>
                    <a-input
                      v-model:value="runtimeSettings.logFilePath"
                      placeholder="Log file path (defaults to ~/.config/agent-ebpf-filter/events.jsonl)"
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
                        <ReloadOutlined /> Save Runtime
                      </a-button>
                      <a-tag :color="persistedEventLogAlive ? 'green' : 'red'">
                        {{
                          persistedEventLogAlive
                            ? "Log file ready"
                            : "Log file inactive"
                        }}
                      </a-tag>
                      <a-tag color="blue">{{
                        persistedEventLogPath || "No log path"
                      }}</a-tag>
                    </div>
                    <a-typography-text type="secondary">
                      When enabled, new events are appended as JSONL and can be
                      exported or tailed through MCP.
                    </a-typography-text>
                  </div>
                </a-col>
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div>
                      <div style="margin-bottom: 6px; font-weight: 600">
                        Access Token
                      </div>
                      <a-input
                        :value="runtimeSettings.accessToken"
                        readonly
                        placeholder="Generate a token to access /config and /mcp"
                      />
                      <div
                        style="
                          display: flex;
                          gap: 8px;
                          flex-wrap: wrap;
                          margin-top: 8px;
                        "
                      >
                        <a-button @click="rotateAccessToken">
                          <ReloadOutlined /> Generate / Rotate
                        </a-button>
                        <a-button
                          @click="
                            copyText(
                              runtimeSettings.accessToken,
                              'Access token copied',
                            )
                          "
                        >
                          <CopyOutlined /> Copy Token
                        </a-button>
                      </div>
                    </div>
                    <div
                      style="display: flex; flex-direction: column; gap: 8px"
                    >
                      <div style="margin-bottom: 2px; font-weight: 600">
                        MCP Endpoint
                      </div>
                      <a-input :value="mcpEndpoint" readonly />
                      <div style="display: flex; gap: 8px; flex-wrap: wrap">
                        <a-button
                          @click="copyText(mcpEndpoint, 'MCP endpoint copied')"
                        >
                          <CopyOutlined /> Copy Base URL
                        </a-button>
                      </div>
                      <div style="margin-top: 4px; font-weight: 600">
                        MCP Query URL
                      </div>
                      <a-input :value="mcpQueryEndpoint" readonly />
                      <div style="display: flex; gap: 8px; flex-wrap: wrap">
                        <a-button
                          @click="
                            copyText(mcpQueryEndpoint, 'MCP query URL copied')
                          "
                        >
                          <CopyOutlined /> Copy Query URL
                        </a-button>
                        <a-button
                          @click="
                            copyText(
                              mcpQueryEndpointTemplate,
                              'MCP query template copied',
                            )
                          "
                        >
                          <CopyOutlined /> Copy Template
                        </a-button>
                      </div>
                      <a-alert
                        type="success"
                        show-icon
                        :message="'Query URL is generated live from the current token and updates when you rotate it.'"
                        style="margin-top: 4px"
                      />
                    </div>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col :span="24">
            <a-card title="Data Management" size="small">
              <template #extra>
                <DeleteOutlined />
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div style="font-weight: 600">Event Retention</div>
                    <div style="display: flex; align-items: center; gap: 12px">
                      <span>Max in-memory events:</span>
                      <a-input-number
                        v-model:value="runtimeSettings.maxEventCount"
                        :min="100"
                        :max="10000"
                        :step="100"
                        style="width: 140px"
                      />
                    </div>
                    <div
                      style="
                        display: flex;
                        align-items: center;
                        gap: 12px;
                        flex-wrap: wrap;
                      "
                    >
                      <span>Max event age:</span>
                      <a-input
                        v-model:value="runtimeSettings.maxEventAge"
                        placeholder="e.g. 24h, 168h, 0 = no limit"
                        style="width: 200px"
                      />
                      <a-typography-text type="secondary">
                        Go duration format (24h, 30m, 168h)
                      </a-typography-text>
                    </div>
                    <div
                      style="
                        display: flex;
                        gap: 8px;
                        flex-wrap: wrap;
                        align-items: center;
                      "
                    >
                      <a-button type="primary" @click="saveRuntime">
                        <ReloadOutlined /> Save Retention
                      </a-button>
                    </div>
                  </div>
                </a-col>
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div style="font-weight: 600">Manual Cleanup</div>
                    <div style="display: flex; gap: 8px; flex-wrap: wrap">
                      <a-popconfirm
                        title="Clear in-memory event buffer?"
                        @confirm="clearInMemoryEvents"
                      >
                        <a-button size="small" danger
                          >Clear Memory Events</a-button
                        >
                      </a-popconfirm>
                      <a-popconfirm
                        title="Truncate persisted event log file?"
                        @confirm="clearPersistedLog"
                      >
                        <a-button size="small" danger
                          >Truncate Log File</a-button
                        >
                      </a-popconfirm>
                      <a-popconfirm
                        title="Clear all events (memory + file)?"
                        @confirm="clearAllEvents"
                      >
                        <a-button size="small" type="primary" danger
                          >Clear All Events</a-button
                        >
                      </a-popconfirm>
                    </div>
                    <a-typography-text type="secondary">
                      These actions are irreversible. Memory events and/or the
                      JSONL log file will be permanently deleted.
                    </a-typography-text>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- Tab: ML Classification -->
      <a-tab-pane key="ml" tab="ML Classification">
        <template #tab>
          <span><ThunderboltOutlined /> ML Classification</span>
        </template>
        <a-tabs
          v-model:activeKey="mlSubTabKey"
          size="small"
          type="card"
          style="margin: 8px 0 16px"
        >
          <a-tab-pane key="status" tab="状况"></a-tab-pane>
          <a-tab-pane key="params" tab="参数"></a-tab-pane>
          <a-tab-pane key="model" tab="模型管理"></a-tab-pane>
          <a-tab-pane key="llm" tab="LLM 打分"></a-tab-pane>
          <a-tab-pane key="training" tab="训练集管理"></a-tab-pane>
        </a-tabs>
        <a-row :gutter="[24, 24]">
          <!-- Row 1: Model Status + Training Controls -->
          <a-col v-if="mlSubTabKey === 'status'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Model Status</span>
              </template>
              <template #extra>
                <a-space>
                  <a-button size="small" @click="mlSubTabKey = 'training'">
                    <ImportOutlined /> 导入
                  </a-button>
                  <a-button size="small" @click="exportTrainingDataset">
                    <ExportOutlined /> 下载
                  </a-button>
                  <a-button size="small" type="link" @click="fetchMLStatus">
                    <ReloadOutlined />
                  </a-button>
                </a-space>
              </template>
              <a-row :gutter="[12, 12]">
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="ML Engine" :value="mlEnabled ? 'Active' : 'Inactive'" :value-style="{ color: mlEnabled ? '#3f8600' : '#cf1322', fontSize: '18px' }" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Model Loaded" :value="mlStatus.model_loaded ? 'Yes' : 'No'" :value-style="{ color: mlStatus.model_loaded ? '#3f8600' : '#d48806', fontSize: '18px' }" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Trees" :value="mlStatus.num_trees || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Train Accuracy" :value="mlStatus.train_accuracy ? (mlStatus.train_accuracy * 100).toFixed(1) : '—'" :suffix="mlStatus.train_accuracy ? '%' : ''" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Validation Acc" :value="mlStatus.validation_accuracy ? (mlStatus.validation_accuracy * 100).toFixed(1) : '—'" :suffix="mlStatus.validation_accuracy ? '%' : ''" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Train Samples" :value="mlStatus.train_samples || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Validation Samples" :value="mlStatus.validation_samples || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Labeled Samples" :value="mlStatus.num_labeled_samples || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Validation Split" :value="((mlStatus.validation_split_ratio || 0) * 100).toFixed(0)" suffix="%" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Last Trained" :value="mlStatus.last_trained || 'Never'" :value-style="{ fontSize: '14px' }" />
                  </a-card>
                </a-col>
                <a-col v-if="mlStatus.training_in_progress" :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <div style="font-weight: 600; margin-bottom: 8px; color: #999">Training</div>
                    <a-progress type="circle" :percent="Math.round((mlStatus.training_progress || 0) * 100)" :size="64" />
                  </a-card>
                </a-col>
              </a-row>
              <div v-if="mlStatus.model_path" style="margin-top: 12px; font-size: 12px; color: #999; word-break: break-all">
                Model Path: {{ mlStatus.model_path }}
              </div>
            </a-card>
          </a-col>
          <a-col v-if="mlSubTabKey === 'model'" :xs="24" :md="12">
            <a-card title="Training Controls" size="small">
              <a-space direction="vertical" style="width: 100%">
                <a-button type="primary" @click="trainWithParams" :loading="trainingModel" block>
                  Train Model Now
                </a-button>
                <a-divider style="margin: 8px 0">Batch Feedback</a-divider>
                <a-input-group compact>
                  <a-input v-model:value="feedbackComm" placeholder="Command (e.g. rm)" style="width: 40%" />
                  <a-select v-model:value="feedbackAction" style="width: 30%">
                    <a-select-option value="accepted">Accepted (ALLOW)</a-select-option>
                    <a-select-option value="rejected">Rejected (BLOCK)</a-select-option>
                    <a-select-option value="alerted">Alerted (ALERT)</a-select-option>
                  </a-select>
                  <a-button type="dashed" @click="submitFeedback" style="width: 30%">Submit</a-button>
                </a-input-group>
              </a-space>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'llm'" :xs="24" :md="12">
            <a-card title="LLM Scoring" size="small">
              <template #extra>
                <a-tag color="purple">OpenAI-compatible API</a-tag>
              </template>
              <a-space direction="vertical" style="width: 100%">
                <a-alert
                  type="info"
                  show-icon
                  message="这里配置外部 OpenAI 风格 LLM 的打分 API。API Key 留空会保留后端已保存的密钥。训练时会按验证集比例自动切分，并在后训练阶段对验证集进行 LLM 复核。"
                />
                <a-row :gutter="[12, 12]">
                  <a-col :xs="24">
                    <a-space align="center" wrap>
                      <a-switch v-model:checked="llmScoringConfig.enabled" />
                      <span>启用 LLM 打分</span>
                      <a-tag v-if="llmScoringConfig.apiKeyConfigured" color="green">Key 已配置</a-tag>
                      <a-tag v-else color="default">Key 未配置</a-tag>
                    </a-space>
                  </a-col>
                  <a-col :xs="24">
                    <div style="font-weight: 600; margin-bottom: 6px">API Base URL</div>
                    <a-input v-model:value="llmScoringConfig.baseUrl" placeholder="https://api.openai.com" allow-clear />
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <div style="font-weight: 600; margin-bottom: 6px">Model</div>
                    <a-input v-model:value="llmScoringConfig.model" placeholder="gpt-4o-mini / gpt-5 / 自建兼容模型" allow-clear />
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <div style="font-weight: 600; margin-bottom: 6px">API Key</div>
                    <a-input-password v-model:value="llmScoringConfig.apiKey" placeholder="留空则保留后端现有密钥" allow-clear />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Timeout (s)</div>
                    <a-input-number v-model:value="llmScoringConfig.timeoutSeconds" :min="5" :max="300" :step="5" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Temperature</div>
                    <a-input-number v-model:value="llmScoringConfig.temperature" :min="0" :max="2" :step="0.1" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Max Tokens</div>
                    <a-input-number v-model:value="llmScoringConfig.maxTokens" :min="32" :max="4096" :step="32" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24">
                    <div style="font-weight: 600; margin-bottom: 6px">System Prompt</div>
                    <a-textarea
                      v-model:value="llmScoringConfig.systemPrompt"
                      :auto-size="{ minRows: 3, maxRows: 8 }"
                      placeholder="你是安全行为分析器，只返回严格 JSON ..."
                    />
                  </a-col>
                </a-row>
                <a-divider style="margin: 8px 0">批量打分 / 后训练复核</a-divider>
                <a-row :gutter="[12, 12]">
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">数据源</div>
                    <a-select v-model:value="llmBatchConfig.source" style="width: 100%">
                      <a-select-option value="training">生成数据（生成新样本并打分）</a-select-option>
                      <a-select-option value="validation">已有数据（对现有样本重打分）</a-select-option>
                    </a-select>
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Limit</div>
                    <a-input-number v-model:value="llmBatchConfig.limit" :min="1" :max="5000" :step="1" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">只看未标注</div>
                    <a-switch v-model:checked="llmBatchConfig.onlyUnlabeled" />
                  </a-col>
                  <a-col :xs="24">
                    <a-space align="center" wrap>
                      <a-switch v-model:checked="llmBatchConfig.applyLabels" :disabled="!llmBatchCanApplyLabels" />
                      <span>把 LLM 结果回写为训练标签</span>
                      <a-tag v-if="!llmBatchCanApplyLabels" color="default">仅训练集可回写</a-tag>
                    </a-space>
                  </a-col>
                </a-row>
                <a-button type="primary" @click="runLLMBatchScore" :loading="llmBatchLoading" block>
                  <ThunderboltOutlined /> 开始批量打分
                </a-button>

                <div v-if="llmBatchResponse" style="display: flex; flex-direction: column; gap: 12px">
                  <a-alert
                    type="success"
                    show-icon
                    :message="`已处理 ${llmBatchResponse.scored}/${llmBatchResponse.total} 条，平均风险 ${(llmBatchResponse.averageRiskScore ?? 0).toFixed(1)}，一致性 ${(llmBatchResponse.agreement * 100).toFixed(0)}%`"
                    :description="llmBatchResponse.review?.validationSplitRatio !== undefined ? `验证集切分比例 ${(llmBatchResponse.review.validationSplitRatio * 100).toFixed(0)}%` : 'LLM 批量复核已完成。'"
                  />
                  <a-space wrap>
                    <a-tag color="blue">source: {{ llmBatchResponse.source }}</a-tag>
                    <a-tag color="geekblue">model: {{ llmBatchResponse.model }}</a-tag>
                    <a-tag color="green">applied: {{ llmBatchResponse.applied }}</a-tag>
                    <a-tag color="orange">skipped: {{ llmBatchResponse.skipped }}</a-tag>
                  </a-space>
                  <a-table
                    :dataSource="llmBatchResponse.entries"
                    :pagination="{ pageSize: 5, showSizeChanger: true, pageSizeOptions: ['5', '10', '20'] }"
                    size="small"
                    :rowKey="llmBatchRowKey"
                    :scroll="{ x: 980 }"
                  >
                    <a-table-column title="Command" dataIndex="commandLine" :width="280" ellipsis>
                      <template #default="{ record }">
                        <code>{{ maskSensitiveData(record.commandLine) }}</code>
                      </template>
                    </a-table-column>
                    <a-table-column title="Label" dataIndex="currentLabel" :width="100">
                      <template #default="{ record }">
                        <a-tag :color="getLabelColor(record.currentLabel || '-')">{{ record.currentLabel || '—' }}</a-tag>
                      </template>
                    </a-table-column>
                    <a-table-column title="Risk" dataIndex="riskScore" :width="90">
                      <template #default="{ record }">
                        {{ record.riskScore?.toFixed(0) }}
                      </template>
                    </a-table-column>
                    <a-table-column title="Action" dataIndex="recommendedAction" :width="110">
                      <template #default="{ record }">
                        <a-tag :color="record.recommendedAction === 'BLOCK' ? 'red' : record.recommendedAction === 'ALERT' ? 'orange' : record.recommendedAction === 'REWRITE' ? 'blue' : 'green'">
                          {{ record.recommendedAction }}
                        </a-tag>
                      </template>
                    </a-table-column>
                    <a-table-column title="Confidence" dataIndex="confidence" :width="110">
                      <template #default="{ record }">
                        {{ record.confidence ? (record.confidence * 100).toFixed(0) + '%' : '—' }}
                      </template>
                    </a-table-column>
                    <a-table-column title="State" dataIndex="applied" :width="100">
                      <template #default="{ record }">
                        <a-tag v-if="record.error" color="red">Error</a-tag>
                        <a-tag v-else-if="record.applied" color="green">Applied</a-tag>
                        <a-tag v-else color="blue">Scored</a-tag>
                      </template>
                    </a-table-column>
                    <a-table-column title="Reasoning" dataIndex="reasoning" ellipsis>
                      <template #default="{ record }">
                        <span>{{ record.reasoning || record.error || '—' }}</span>
                      </template>
                    </a-table-column>
                  </a-table>
                </div>
              </a-space>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'llm'" :xs="24">
            <a-card title="LLM 生产训练集" size="small">
              <template #extra>
                <a-tag color="green">来源：/config/ml/training</a-tag>
              </template>
              <a-space direction="vertical" style="width: 100%">
                <a-alert
                  type="info"
                  show-icon
                  message="直接从当前训练存储生成 OpenAI chat JSONL，不抓网页 HTML，也不会把未标注样本洗进训练集。默认只保留已标注样本，并按 commandLine + label 去重；如确实需要噪声样本，可手动打开启发式标签。"
                />
                <a-row :gutter="[12, 12]">
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">样本上限</div>
                    <a-input-number
                      v-model:value="llmProductionDatasetLimit"
                      :min="1"
                      :max="5000"
                      :step="1"
                      style="width: 100%"
                    />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-space direction="vertical" style="width: 100%">
                      <a-space align="center" wrap>
                        <a-switch v-model:checked="llmProductionDeduplicate" />
                        <span>命令 + 标签去重</span>
                      </a-space>
                      <a-space align="center" wrap>
                        <a-switch v-model:checked="llmProductionAllowHeuristic" />
                        <span>允许启发式 / LLM 自动标签</span>
                      </a-space>
                    </a-space>
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-space direction="vertical" style="width: 100%">
                      <a-button type="primary" @click="fetchLLMProductionDataset" :loading="llmProductionLoading" block>
                        <ReloadOutlined /> 拉取当前训练集
                      </a-button>
                      <a-button @click="exportLLMProductionDataset" :disabled="llmProductionPreview.length === 0 || llmProductionLoading" block>
                        <DownloadOutlined /> 导出 JSONL
                      </a-button>
                    </a-space>
                  </a-col>
                </a-row>

                <a-space wrap>
                  <a-tag v-if="llmProductionMeta" color="blue">source: {{ llmProductionMeta.source }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="cyan">format: {{ llmProductionMeta.format }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="geekblue">total: {{ llmProductionMeta.total }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="green">included: {{ llmProductionMeta.included }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="default">skip unlabeled: {{ llmProductionMeta.skippedUnlabeled }}</a-tag>
                  <a-tag v-if="llmProductionMeta && llmProductionMeta.skippedHeuristic > 0" color="orange">skip noisy: {{ llmProductionMeta.skippedHeuristic }}</a-tag>
                  <a-tag v-if="llmProductionMeta && llmProductionMeta.skippedDuplicates > 0" color="gold">skip dup: {{ llmProductionMeta.skippedDuplicates }}</a-tag>
                  <a-tag v-if="llmProductionMeta?.truncated" color="red">truncated</a-tag>
                </a-space>

                <a-alert
                  v-if="llmProductionMeta"
                  type="success"
                  show-icon
                  :message="`已生成 ${llmProductionMeta.included} 条 LLM 生产训练样本`"
                  :description="`导出 JSONL 每行仅包含 messages，适合 chat fine-tuning；系统提示词来自当前 LLM 配置：${llmProductionMeta.systemPrompt}`"
                />
                <a-alert
                  v-else
                  type="warning"
                  show-icon
                  message="点击“拉取当前训练集”后，会直接从训练存储生成可训练的 chat JSONL 预览。"
                />

                <a-table
                  v-if="llmProductionPreview.length > 0"
                  :dataSource="llmProductionPreview"
                  :pagination="{ pageSize: 5, showSizeChanger: true, pageSizeOptions: ['5', '10', '20'] }"
                  :scroll="{ x: 1280 }"
                  size="small"
                  rowKey="index"
                >
                  <a-table-column title="#" dataIndex="index" :width="70" />
                  <a-table-column title="Command" dataIndex="commandLine" :width="260" ellipsis>
                    <template #default="{ record }">
                      <code>{{ maskSensitiveData(record.commandLine) }}</code>
                    </template>
                  </a-table-column>
                  <a-table-column title="Label" dataIndex="label" :width="100">
                    <template #default="{ record }">
                      <a-tag :color="getLabelColor(record.label)">{{ record.label }}</a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column title="Risk" dataIndex="targetRiskScore" :width="90">
                    <template #default="{ record }">
                      {{ record.targetRiskScore?.toFixed(0) }}
                    </template>
                  </a-table-column>
                  <a-table-column title="Confidence" dataIndex="targetConfidence" :width="110">
                    <template #default="{ record }">
                      {{ record.targetConfidence ? (record.targetConfidence * 100).toFixed(0) + '%' : '—' }}
                    </template>
                  </a-table-column>
                  <a-table-column title="Source" dataIndex="userLabel" :width="140">
                    <template #default="{ record }">
                      <a-tag color="purple">{{ record.userLabel || '—' }}</a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column title="Signals" dataIndex="signals" :width="220">
                    <template #default="{ record }">
                      <a-space wrap size="small">
                        <a-tag v-for="(signal, i) in record.signals || []" :key="i" color="purple" size="small">
                          {{ signal }}
                        </a-tag>
                      </a-space>
                    </template>
                  </a-table-column>
                  <a-table-column title="Reasoning" dataIndex="reasoning" ellipsis>
                    <template #default="{ record }">
                      <span>{{ record.reasoning || '—' }}</span>
                    </template>
                  </a-table-column>
                </a-table>

                <a-card v-if="llmProductionPreview.length > 0" size="small" title="首条样本 JSON 预览">
                  <a-descriptions :column="2" size="small" bordered>
                    <a-descriptions-item label="Command">{{ maskSensitiveData(llmProductionPreview[0].commandLine) }}</a-descriptions-item>
                    <a-descriptions-item label="Label">
                      <a-tag :color="getLabelColor(llmProductionPreview[0].label)">{{ llmProductionPreview[0].label }}</a-tag>
                    </a-descriptions-item>
                    <a-descriptions-item label="Prompt" :span="2">
                      <a-textarea
                        :value="maskSensitiveData(llmProductionPreview[0].prompt)"
                        :auto-size="{ minRows: 4, maxRows: 8 }"
                        readonly
                      />
                    </a-descriptions-item>
                    <a-descriptions-item label="Completion" :span="2">
                      <a-textarea
                        :value="maskSensitiveData(llmProductionPreview[0].completion)"
                        :auto-size="{ minRows: 4, maxRows: 8 }"
                        readonly
                      />
                    </a-descriptions-item>
                  </a-descriptions>
                </a-card>
              </a-space>
            </a-card>
          </a-col>

          <!-- Row: Training Progress & Logs -->
          <a-col
            v-if="mlSubTabKey === 'status' && (mlStatus.training_in_progress || trainingLogs.length > 0)"
            :xs="24"
          >
            <a-card size="small">
              <template #title>
                <span>Training Progress</span>
                <a-tag color="processing" style="margin-left: 8px" v-if="mlStatus.training_in_progress">Running...</a-tag>
                <a-tag color="green" style="margin-left: 8px" v-else>Complete</a-tag>
              </template>
              <a-progress
                :percent="Math.round((mlStatus.training_progress || 0) * 100)"
                :status="mlStatus.training_in_progress ? 'active' : 'success'"
                style="margin-bottom: 12px"
              />
              <div
                ref="logContainer"
                style="background: #1e1e1e; color: #d4d4d4; border-radius: 6px; padding: 10px 14px; max-height: 320px; overflow-y: auto; font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace; font-size: 12px; line-height: 1.6"
              >
                <div v-for="(line, i) in trainingLogs" :key="i" style="white-space: pre-wrap; word-break: break-all">
                  <span style="color: #6a9955">{{ line.time }}</span>
                  <span v-if="line.message.startsWith('ERROR')" style="color: #f44747">{{ ' ' + line.message }}</span>
                  <span v-else-if="line.message.startsWith('═══')" style="color: #569cd6; font-weight: bold">{{ ' ' + line.message }}</span>
                  <span v-else style="color: #d4d4d4">{{ ' ' + line.message }}</span>
                </div>
                <div v-if="trainingLogs.length === 0 && mlStatus.training_in_progress" style="color: #888">
                  Waiting for training to start...
                </div>
              </div>
            </a-card>
          </a-col>

          <!-- Row: Training Curve Visualization -->
          <a-col v-if="mlSubTabKey === 'status' && trainingHistory.length > 0" :xs="24">
            <a-card title="Training History" size="small">
              <template #extra>
                <a-tag color="blue">{{ trainingHistory.length }} runs</a-tag>
              </template>
              <Suspense>
                <VueApexCharts
                  type="line"
                  height="280"
                  :options="trainingChartOptions"
                  :series="trainingChartSeries"
                />
                <template #fallback>
                  <div style="text-align: center; padding: 40px; color: #999">Loading chart...</div>
                </template>
              </Suspense>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'status' && mlStatus.llm_review" :xs="24">
            <a-card title="LLM Post-Training Review" size="small">
              <template #extra>
                <a-tag color="purple">OpenAI-style batch review</a-tag>
              </template>
              <a-descriptions :column="3" size="small" bordered>
                <a-descriptions-item label="Source">{{ mlStatus.llm_review?.source || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Model">{{ mlStatus.llm_review?.model || llmScoringConfig.model || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Scored Samples">{{ mlStatus.llm_review?.scoredSamples ?? 0 }}</a-descriptions-item>
                <a-descriptions-item label="Average Risk">
                  {{ mlStatus.llm_review ? mlStatus.llm_review.averageRiskScore.toFixed(1) : '—' }}
                </a-descriptions-item>
                <a-descriptions-item label="Agreement">
                  {{ mlStatus.llm_review ? (mlStatus.llm_review.agreement * 100).toFixed(0) + '%' : '—' }}
                </a-descriptions-item>
                <a-descriptions-item label="Validation Split">
                  {{ mlStatus.llm_review?.validationSplitRatio !== undefined ? (mlStatus.llm_review.validationSplitRatio * 100).toFixed(0) + '%' : '—' }}
                </a-descriptions-item>
                <a-descriptions-item label="Reviewed At" :span="3">
                  {{ mlStatus.llm_review?.reviewedAt ? new Date(mlStatus.llm_review.reviewedAt).toLocaleString() : '—' }}
                </a-descriptions-item>
              </a-descriptions>
            </a-card>
          </a-col>

          <!-- Row: Classic OS Security Datasets -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span><BookOutlined /> 经典 OS 安全数据集</span>
                <a-tag color="green" style="margin-left: 8px">支持一键导入</a-tag>
              </template>
              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px"
                message="有下载链接的数据集可一键导入；无下载链接的会跳转官方页面，下载后用“导入本地文件”上传。导入器支持 zip, gz, tar, tgz, bz2 等归档及 JSON, JSONL, CSV, TSV, 纯文本。"
              />

              <a-list :data-source="classicSecurityDatasetPresets" :split="false" size="small">
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
                            <div style="color: #666; font-size: 12px">
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
                          >
                            <ImportOutlined /> {{ item.downloadUrl ? '一键导入' : '前往下载' }}
                          </a-button>
                          <a-button size="small" @click="openClassicSecurityDatasetPage(item)">
                            <GlobalOutlined /> 打开官网
                          </a-button>
                          <a-button size="small" @click="copyClassicSecurityDatasetPage(item)">
                            <CopyOutlined /> 复制链接
                          </a-button>
                        </a-space>
                      </a-space>
                    </a-card>
                  </a-list-item>
                </template>
              </a-list>
            </a-card>
          </a-col>

          <!-- Row: Internet Dataset Import -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span><GlobalOutlined /> 互联网数据集拉取</span>
                <a-tag color="blue" style="margin-left: 8px">HTTP/HTTPS JSON、JSONL、CSV、TSV、文本</a-tag>
              </template>
              <template #extra>
                  <a-space>
                  <input
                    type="file"
                    ref="trainingDatasetImportInput"
                    @change="importTrainingDatasetFromFile"
                    style="display: none"
                    accept=".json,.jsonl,.ndjson,.csv,.tsv,.txt,.log,.zip,.gz,.tgz,.tar,.bz2,.tbz,.tbz2,.txz"
                  />
                  <a-button size="small" @click="fetchRemoteDatasetPreview" :loading="loadingRemoteDataset">
                    <ReloadOutlined /> 拉取预览
                  </a-button>
                  <a-button size="small" @click="openTrainingDatasetImportPicker" :loading="importingRemoteDataset">
                    <FileOutlined /> 导入本地文件
                  </a-button>
                  <a-button size="small" type="primary" @click="importRemoteDataset" :loading="importingRemoteDataset">
                    <ImportOutlined /> 导入训练集
                  </a-button>
                </a-space>
              </template>

              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px"
                message="后端只接受可直接 GET 到的原始数据文件；如果地址返回的是 HTML 介绍页、下载页或归档页，会直接报错。也可以用“导入本地文件”上传 JSON, JSONL, CSV, TSV, 纯文本或常见压缩包，后端会自动尝试解压 zip, gz, tar, tar.gz, tgz, bz2 等归档。"
              />

              <a-row :gutter="[16, 16]">
                <a-col :xs="24" :md="10">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">数据集 URL</div>
                      <a-input
                        v-model:value="remoteDatasetUrl"
                        placeholder="https://example.com/dataset.jsonl"
                        allow-clear
                      />
                    </div>
                    <div style="display: flex; gap: 12px; flex-wrap: wrap">
                      <div style="flex: 1; min-width: 180px">
                        <div style="font-weight: 600; margin-bottom: 6px">格式</div>
                        <a-select v-model:value="remoteDatasetFormat" style="width: 100%">
                          <a-select-option value="auto">自动识别</a-select-option>
                          <a-select-option value="json">JSON</a-select-option>
                          <a-select-option value="jsonl">JSONL / NDJSON</a-select-option>
                          <a-select-option value="csv">CSV</a-select-option>
                          <a-select-option value="tsv">TSV</a-select-option>
                          <a-select-option value="text">纯文本命令行</a-select-option>
                        </a-select>
                      </div>
                      <div style="flex: 1; min-width: 180px">
                        <div style="font-weight: 600; margin-bottom: 6px">标签模式</div>
                        <a-select v-model:value="remoteDatasetLabelMode" style="width: 100%">
                          <a-select-option value="preserve">保留原始标签</a-select-option>
                          <a-select-option value="unlabeled">统一未标注</a-select-option>
                          <a-select-option value="heuristic">按规则自动标注</a-select-option>
                        </a-select>
                      </div>
                    </div>
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">拉取条数</div>
                      <a-input-number
                        v-model:value="remoteDatasetLimit"
                        :min="1"
                        :max="5000"
                        :step="1"
                        style="width: 100%"
                      />
                    </div>
                    <a-typography-text type="secondary">
                      支持公开数据集、实验室内网数据集或你自己的样本仓库，只要 URL 可直接 GET 访问即可。
                    </a-typography-text>
                  </div>
                </a-col>
                <a-col :xs="24" :md="14">
                  <div style="display: flex; flex-direction: column; gap: 10px">
                    <a-space wrap>
                      <a-tag v-if="remoteDatasetMeta" color="blue">source: {{ remoteDatasetMeta.source }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="cyan">format: {{ remoteDatasetMeta.format }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="geekblue">type: {{ remoteDatasetMeta.contentType || 'unknown' }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="purple">total: {{ remoteDatasetMeta.total }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta?.truncated" color="orange">truncated</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="green">imported: {{ remoteDatasetMeta.imported ?? 0 }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="gold">skipped: {{ remoteDatasetMeta.skipped ?? 0 }}</a-tag>
                    </a-space>
                    <a-alert
                      v-if="remoteDatasetMeta"
                      type="success"
                      show-icon
                      :message="`已拉取 ${remoteDatasetMeta.total} 条，当前预览显示 ${remoteDatasetPreview.length} 条`"
                      :description="remoteDatasetMeta.truncated ? '列表已按 Limit 截断，导入时也会使用同样的条数上限。' : '列表展示的是当前请求返回的全部可见数据。'"
                    />
                    <a-alert
                      v-else
                      type="warning"
                      show-icon
                      message="输入数据集 URL 后点击“拉取预览”，即可先查看格式识别和样本解析情况。"
                    />
                    <a-table
                      :dataSource="remoteDatasetPreview"
                      :pagination="{ pageSize: 6, showSizeChanger: true, pageSizeOptions: ['6', '10', '20'] }"
                      :scroll="{ x: 980 }"
                      size="small"
                      rowKey="row"
                    >
                      <a-table-column title="#" dataIndex="row" :width="60" />
                      <a-table-column title="Command" dataIndex="commandLine" :width="280" ellipsis>
                        <template #default="{ record }">
                          <code>{{ maskSensitiveData(record.commandLine) }}</code>
                        </template>
                      </a-table-column>
                      <a-table-column title="Label" dataIndex="label" :width="100">
                        <template #default="{ record }">
                          <a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column title="Category" dataIndex="category" :width="120">
                        <template #default="{ record }">
                          <a-tag v-if="record.category" :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
                          <span v-else style="color: #999">—</span>
                        </template>
                      </a-table-column>
                      <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="90">
                        <template #default="{ record }">
                          {{ record.anomalyScore?.toFixed(2) }}
                        </template>
                      </a-table-column>
                      <a-table-column title="State" dataIndex="duplicate" :width="100">
                        <template #default="{ record }">
                          <a-tag :color="record.duplicate ? 'default' : 'green'" size="small">
                            {{ record.duplicate ? '已存在' : '可导入' }}
                          </a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column title="Time" dataIndex="timestamp" :width="180">
                        <template #default="{ record }">
                          <span style="font-size: 12px; color: #666">{{ record.timestamp ? new Date(record.timestamp).toLocaleString() : '—' }}</span>
                        </template>
                      </a-table-column>
                    </a-table>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <!-- Row: Pull existing command data -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Existing Command Data</span>
                <a-tag color="cyan" style="margin-left: 8px">拉取已有 wrapper / hook 事件</a-tag>
              </template>
              <template #extra>
                <a-space wrap>
                  <span style="font-size: 12px; color: #666">Limit</span>
                  <a-input-number v-model:value="existingDataLimit" :min="10" :max="5000" size="small" style="width: 100px" />
                  <a-select v-model:value="existingLabelMode" size="small" style="width: 150px">
                    <a-select-option value="unlabeled">导入为未标注</a-select-option>
                    <a-select-option value="heuristic">按安全判断标注</a-select-option>
                  </a-select>
                  <a-button size="small" @click="fetchExistingCommandData" :loading="loadingExistingData">
                    <ReloadOutlined /> 拉取已有数据
                  </a-button>
                  <a-button
                    size="small"
                    type="primary"
                    @click="importExistingCommandData"
                    :loading="importingExistingData"
                    :disabled="importableExistingCount <= 0"
                  >
                    <ImportOutlined /> 导入 {{ importableExistingCount }}
                  </a-button>
                </a-space>
              </template>
              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px"
                message="从 /events/recent 读取历史 wrapper_intercept / native_hook 命令。默认导入为未标注样本；选择“按安全判断标注”会用当前规则/ML/网络审计结果自动给出 ALLOW/ALERT/BLOCK 标签。"
              />
              <div style="display: flex; gap: 8px; align-items: center; margin-bottom: 8px; flex-wrap: wrap">
                <a-tag v-if="existingDataSource" color="blue">source: {{ existingDataSource }}</a-tag>
                <a-tag color="purple">{{ existingCommandCandidates.length }} pulled</a-tag>
                <a-tag color="default">{{ existingDuplicateCount }} duplicates</a-tag>
              </div>
              <a-table
                :dataSource="existingCommandCandidates"
                :pagination="{ pageSize: 8, showSizeChanger: true, pageSizeOptions: ['8','15','30'] }"
                :scroll="{ x: 900 }"
                size="small"
                rowKey="commandLine"
              >
                <a-table-column title="Command" dataIndex="commandLine" :width="300" ellipsis>
                  <template #default="{ record }">
                    <code>{{ maskSensitiveData(record.commandLine) }}</code>
                  </template>
                </a-table-column>
                <a-table-column title="Event" dataIndex="eventType" :width="120">
                  <template #default="{ record }">
                    <a-tag size="small" color="geekblue">{{ record.eventType }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Category" dataIndex="category" :width="120">
                  <template #default="{ record }">
                    <a-tag v-if="record.category" :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
                    <span v-else style="color: #999">—</span>
                  </template>
                </a-table-column>
                <a-table-column title="Time" dataIndex="timestamp" :width="180">
                  <template #default="{ record }">
                    <span style="font-size: 12px; color: #666">{{ record.timestamp ? new Date(record.timestamp).toLocaleString() : '—' }}</span>
                  </template>
                </a-table-column>
                <a-table-column title="State" dataIndex="duplicate" :width="100">
                  <template #default="{ record }">
                    <a-tag :color="record.duplicate ? 'default' : 'green'" size="small">
                      {{ record.duplicate ? '已存在' : '可导入' }}
                    </a-tag>
                  </template>
                </a-table-column>
              </a-table>
            </a-card>
          </a-col>

          <!-- Row: Sample Data Browser -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Training Data Browser</span>
                <a-tag color="purple" style="margin-left: 8px">{{ filteredSamples.length }} / {{ allSamples.length }}</a-tag>
              </template>
              <template #extra>
                <a-space wrap>
                  <a-button 
                    size="small" 
                    @click="dataMaskEnabled = !dataMaskEnabled"
                    :type="dataMaskEnabled ? 'primary' : 'default'"
                  >
                    <component :is="dataMaskEnabled ? EyeInvisibleOutlined : EyeOutlined" />
                    {{ dataMaskEnabled ? '脱敏' : '明文' }}
                  </a-button>
                  <a-button size="small" @click="exportTrainingDataset">
                    <ExportOutlined /> 导出训练集
                  </a-button>
                  <a-popconfirm title="确定要清空当前训练集吗？" @confirm="clearTrainingDataset">
                    <a-button size="small" danger>
                      <DeleteOutlined /> 清空训练集
                    </a-button>
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
                  <a-button size="small" @click="fetchAllSamples" :loading="loadingSamples">
                    <ReloadOutlined /> Refresh
                  </a-button>
                </a-space>
              </template>
              <a-table
                :dataSource="filteredSamples"
                :pagination="{ pageSize: sampleTablePageSize, showSizeChanger: true, pageSizeOptions: ['10','15','30','50'], showTotal: (t:number) => `${t} samples` }"
                :scroll="{ x: 900 }"
                size="small"
                rowKey="index"
              >
                <a-table-column title="#" dataIndex="index" :width="50" />
                <a-table-column title="Comm" dataIndex="comm" :width="100">
                  <template #default="{ record }">
                    <strong>{{ record.comm }}</strong>
                  </template>
                </a-table-column>
                <a-table-column title="Args" dataIndex="args" :width="200" ellipsis>
                  <template #default="{ record }">
                    <span style="font-size: 12px; color: #666">{{ maskSensitiveData((record.args || []).join(' ')) || '—' }}</span>
                  </template>
                </a-table-column>
                <a-table-column title="Category" dataIndex="category" :width="110">
                  <template #default="{ record }">
                    <a-tag :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
                  </template>
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
                  <template #default="{ record }">
                    <a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Actions" :width="240">
                  <template #default="{ record }">
                    <a-space :size="4">
                      <a-button size="small" type="primary" ghost @click="labelSample(record.index, 'ALLOW')" :disabled="record.label === 'ALLOW'">ALLOW</a-button>
                      <a-button size="small" style="border-color: #faad14; color: #d48806" ghost @click="labelSample(record.index, 'ALERT')" :disabled="record.label === 'ALERT'">ALERT</a-button>
                      <a-button size="small" danger ghost @click="labelSample(record.index, 'BLOCK')" :disabled="record.label === 'BLOCK'">BLOCK</a-button>
                      <a-button size="small" danger type="text" @click="deleteSample(record.index)">
                        <DeleteOutlined />
                      </a-button>
                    </a-space>
                  </template>
                </a-table-column>
              </a-table>
            </a-card>
          </a-col>

          <!-- Row: Model Hyperparameters -->
          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card title="Model Hyperparameters" size="small">
              <template #extra>
                <a-tag color="geekblue">调整神经元层数和训练参数</a-tag>
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Num Trees (树的数量)</span>
                  <a-slider v-model:value="hyperParams.numTrees" :min="5" :max="200" :step="1" />
                  <a-input-number v-model:value="hyperParams.numTrees" :min="5" :max="200" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">更多树 = 更高精度但更慢训练。推荐 31-101</div>
                </a-col>
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Max Depth (最大深度)</span>
                  <a-slider v-model:value="hyperParams.maxDepth" :min="3" :max="20" :step="1" />
                  <a-input-number v-model:value="hyperParams.maxDepth" :min="3" :max="20" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">更深的树 = 更复杂决策边界。推荐 6-12</div>
                </a-col>
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Min Samples Leaf (叶节点最小样本)</span>
                  <a-slider v-model:value="hyperParams.minSamplesLeaf" :min="1" :max="50" :step="1" />
                  <a-input-number v-model:value="hyperParams.minSamplesLeaf" :min="1" :max="50" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">更大值防止过拟合。推荐 2-10</div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <!-- Row 4: Manual Training Data -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Add Labeled Training Data</span>
                <a-tag color="blue" style="margin-left: 8px">手动添加标注样本</a-tag>
              </template>
              <a-row :gutter="[16, 16]">
                <!-- Quick presets -->
                <a-col :xs="24" :md="14">
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                    <div style="font-weight: 600">高危行为预设（点击即可添加已标注样本）</div>
                    <a-button size="small" type="link" @click="importAllHighRiskPresets">一键导入全部预设</a-button>
                  </div>
                  <a-space wrap>
                    <a-tag
                      v-for="(p, i) in highRiskPresets"
                      :key="i"
                      :color="p.label === 'BLOCK' ? 'red' : 'orange'"
                      style="cursor: pointer; padding: 4px 8px; font-size: 13px"
                      @click="addPresetSample(p)"
                    >
                      {{ p.comm }} {{ p.args ? p.args.slice(0, 30) + '…' : '' }}
                      <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
                    </a-tag>
                  </a-space>
                </a-col>

                <!-- Safety Net presets -->
                <a-col :xs="24" :md="10">
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                    <div style="font-weight: 600">Claude Code Safety Net 预设</div>
                    <a-button size="small" type="link" @click="importAllSafetyNetPresets">一键导入全部</a-button>
                  </div>
                  <a-space wrap>
                    <a-tag
                      v-for="(p, i) in safetyNetHighRiskPresets"
                      :key="'sn'+i"
                      :color="p.label === 'BLOCK' ? 'red' : p.label === 'ALLOW' ? 'green' : 'orange'"
                      style="cursor: pointer; padding: 4px 8px; font-size: 12px"
                      @click="addPresetSample(p)"
                    >
                      <code>{{ p.comm }}</code> {{ p.args.slice(0, 35) }}{{ p.args.length > 35 ? '…' : '' }}
                      <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
                    </a-tag>
                  </a-space>
                </a-col>

                <!-- Manual form with explicit labeling -->
                <a-col :xs="24" :md="10">
                  <div style="font-weight: 600; margin-bottom: 8px">Step 1: 输入完整命令行</div>
                  <a-input 
                    v-model:value="sampleCommandLine" 
                    placeholder="完整命令 (支持管道: cat file.txt | grep error | wc -l)" 
                    size="small" 
                    style="margin-bottom: 10px"
                    @keyup.enter="submitManualSample"
                  />

                  <div style="font-weight: 600; margin-bottom: 8px">Step 2: 标注行为 <a-tag color="processing" size="small">选择标签</a-tag></div>
                  <div style="display: flex; gap: 8px; margin-bottom: 6px">
                    <a-radio-group v-model:value="sampleLabel" button-style="solid" size="small">
                      <a-radio-button value="BLOCK" style="border-color: #ff4d4f; color: #ff4d4f">
                        <StopOutlined /> BLOCK 拦截
                      </a-radio-button>
                      <a-radio-button value="ALERT" style="border-color: #faad14; color: #d48806">
                        <AlertOutlined /> ALERT 警报
                      </a-radio-button>
                      <a-radio-button value="ALLOW" style="border-color: #52c41a; color: #52c41a">
                        <span style="font-size: 11px">&#10003;</span> ALLOW 放行
                      </a-radio-button>
                    </a-radio-group>
                  </div>
                  <div style="background: #fffbe6; border: 1px solid #ffe58f; border-radius: 4px; padding: 6px 10px; margin-bottom: 8px; font-size: 13px" v-if="sampleCommandLine.trim()">
                    <div v-for="(cmd, idx) in sampleCommandLine.trim().split('|').map(c => c.trim()).filter(c => c)" :key="idx" style="margin-bottom: 2px">
                      <span style="color: #666">{{ idx + 1 }}. </span>
                      <strong>{{ cmd.split(/\s+/)[0] }}</strong>
                      <span v-if="cmd.split(/\s+/).length > 1" style="color: #666"> {{ cmd.split(/\s+/).slice(1).join(' ').slice(0, 30) }}{{ cmd.split(/\s+/).slice(1).join(' ').length > 30 ? '…' : '' }}</span>
                      <span style="color: #666"> → </span>
                      <a-tag :color="sampleLabel === 'BLOCK' ? 'red' : sampleLabel === 'ALERT' ? 'orange' : 'green'" size="small">{{ sampleLabel }}</a-tag>
                    </div>
                  </div>

                  <a-button type="primary" @click="submitManualSample" :loading="submittingSample" block>
                    <PlusOutlined /> 添加此标注样本
                  </a-button>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <!-- Row 5: Command Safety Assessment -->
          <a-col v-if="mlSubTabKey === 'model'" :xs="24">
            <a-card title="Command Safety Assessment" size="small">
              <template #extra>
                <a-tag color="purple">输入完整命令进行安全性判断</a-tag>
              </template>
              <a-row :gutter="[16, 16]">
                <a-col :xs="24" :md="8">
                  <div style="font-weight: 600; margin-bottom: 8px">待判断命令</div>
                  <a-space direction="vertical" style="width: 100%">
                    <a-textarea
                      v-model:value="backtestCommandLine"
                      placeholder="完整命令 (e.g. sudo systemctl disable firewalld)"
                      :auto-size="{ minRows: 3, maxRows: 6 }"
                      @keyup.ctrl.enter="runBacktest"
                    />
                    <a-button type="primary" @click="runBacktest" :loading="backtesting" block>
                      <SearchOutlined /> 判断安全性
                    </a-button>
                  </a-space>
                  <div style="margin-top: 12px; font-size: 12px; color: #999">
                    快速测试：
                    <a v-for="(p, i) in highRiskPresets.slice(0, 5)" :key="i" @click="runBacktestPreset(p.comm, p.args)" style="margin-right: 8px; white-space: nowrap">{{ p.comm }}</a>
                  </div>
                </a-col>

                <a-col :xs="24" :md="16">
                  <div v-if="backtestResult" style="display: flex; flex-direction: column; gap: 16px">
                    <!-- Risk gauge -->
                    <div style="display: flex; align-items: center; gap: 16px">
                      <div style="flex: 1">
                        <div style="font-weight: 600; margin-bottom: 4px">
                          风险评分：{{ backtestResult.riskScore?.toFixed(0) || '-' }} / 100
                          <a-tag :color="riskLevelColor(backtestResult.riskLevel)" style="margin-left: 8px">
                            {{ backtestResult.riskLevel }}
                          </a-tag>
                        </div>
                        <div style="background: #f0f0f0; border-radius: 8px; height: 20px; overflow: hidden">
                          <div
                            :style="{
                              width: (backtestResult.riskScore || 0) + '%',
                              height: '100%',
                              background: riskMeterColor(backtestResult.riskScore || 0),
                              borderRadius: '8px',
                              transition: 'width 0.5s ease',
                            }"
                          ></div>
                        </div>
                      </div>
                      <div style="text-align: center; min-width: 80px">
                        <div style="font-size: 28px; font-weight: bold; color: riskMeterColor(backtestResult.riskScore || 0)">
                          {{ backtestResult.riskScore?.toFixed(0) || 0 }}
                        </div>
                        <div style="font-size: 11px; color: #999">/ 100</div>
                      </div>
                    </div>

                    <!-- Detail breakdown -->
                    <a-descriptions :column="3" size="small" bordered>
                      <a-descriptions-item label="Command">{{ backtestResult.commandLine || backtestResult.comm }}</a-descriptions-item>
                      <a-descriptions-item label="Args">{{ backtestResult.args?.join(' ') || '—' }}</a-descriptions-item>
                      <a-descriptions-item label="Recommended Action">
                        <a-tag :color="backtestResult.recommendedAction === 'BLOCK' ? 'red' : backtestResult.recommendedAction === 'ALERT' ? 'orange' : backtestResult.recommendedAction === 'REWRITE' ? 'blue' : 'green'">
                          {{ backtestResult.recommendedAction }}
                        </a-tag>
                      </a-descriptions-item>
                      <a-descriptions-item label="Category">
                        <a-tag>{{ backtestResult.classification?.primary_category || 'UNKNOWN' }}</a-tag>
                      </a-descriptions-item>
                      <a-descriptions-item label="Classify Confidence">{{ backtestResult.classification?.confidence || '—' }}</a-descriptions-item>
                      <a-descriptions-item label="Anomaly Score">
                        <span :style="{ color: (backtestResult.anomalyScore ?? 0) > 0.7 ? '#d4380d' : (backtestResult.anomalyScore ?? 0) > 0.3 ? '#d48806' : '#52c41a' }">
                          {{ backtestResult.anomalyScore?.toFixed(3) || '—' }}
                        </span>
                      </a-descriptions-item>
                      <a-descriptions-item label="ML Action">{{ backtestResult.mlPrediction?.action || '—' }}</a-descriptions-item>
                      <a-descriptions-item label="ML Confidence">
                        {{ backtestResult.mlPrediction?.confidence ? (backtestResult.mlPrediction.confidence * 100).toFixed(0) + '%' : '—' }}
                      </a-descriptions-item>
                      <a-descriptions-item label="Reasoning" :span="3">{{ backtestResult.reasoning || '—' }}</a-descriptions-item>
                    </a-descriptions>

                    <!-- LLM scoring breakdown -->
                    <div v-if="backtestResult.llmAssessment" style="margin-top: 8px">
                      <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                        <span>LLM 打分结果</span>
                        <a-tag :color="backtestResult.llmAssessment.error ? 'red' : 'purple'">
                          {{ backtestResult.llmAssessment.error ? 'Error' : 'OpenAI-style' }}
                        </a-tag>
                      </div>
                      <a-alert
                        v-if="backtestResult.llmAssessment.error"
                        type="error"
                        show-icon
                        :message="backtestResult.llmAssessment.error"
                        style="margin-bottom: 8px"
                      />
                      <a-descriptions v-else :column="3" size="small" bordered>
                        <a-descriptions-item label="Model">{{ backtestResult.llmAssessment.model || '—' }}</a-descriptions-item>
                        <a-descriptions-item label="Risk Score">{{ backtestResult.llmAssessment.riskScore?.toFixed(0) || '—' }}</a-descriptions-item>
                        <a-descriptions-item label="Confidence">
                          {{ backtestResult.llmAssessment.confidence ? (backtestResult.llmAssessment.confidence * 100).toFixed(0) + '%' : '—' }}
                        </a-descriptions-item>
                        <a-descriptions-item label="Recommended Action">
                          <a-tag :color="backtestResult.llmAssessment.recommendedAction === 'BLOCK' ? 'red' : backtestResult.llmAssessment.recommendedAction === 'ALERT' ? 'orange' : backtestResult.llmAssessment.recommendedAction === 'REWRITE' ? 'blue' : 'green'">
                            {{ backtestResult.llmAssessment.recommendedAction }}
                          </a-tag>
                        </a-descriptions-item>
                        <a-descriptions-item label="Reasoning" :span="2">{{ backtestResult.llmAssessment.reasoning || '—' }}</a-descriptions-item>
                        <a-descriptions-item label="Signals" :span="3">
                          <a-space wrap>
                            <a-tag v-for="(signal, i) in backtestResult.llmAssessment.signals || []" :key="i" color="purple">
                              {{ signal }}
                            </a-tag>
                            <span v-if="(backtestResult.llmAssessment.signals || []).length === 0" style="color: #999">—</span>
                          </a-space>
                        </a-descriptions-item>
                      </a-descriptions>
                    </div>

                    <!-- Existing labeled sample evidence -->
                    <div v-if="backtestResult.sampleEvidence?.totalMatches > 0">
                      <a-alert
                        show-icon
                        :type="backtestResult.sampleEvidence?.decision === 'BLOCK' ? 'error' : backtestResult.sampleEvidence?.decision === 'ALERT' ? 'warning' : 'info'"
                        :message="`命中已有样本 ${backtestResult.sampleEvidence.totalMatches} 条，已标注 ${backtestResult.sampleEvidence.labeledMatches} 条`"
                        :description="backtestResult.sampleEvidence?.decision ? `历史标注倾向：${backtestResult.sampleEvidence.decision}，置信度 ${(backtestResult.sampleEvidence.confidence * 100).toFixed(0)}%` : '暂无可直接用于判断的标注，但命令已存在于样本库。'"
                        style="margin-bottom: 8px"
                      />
                      <a-table
                        :dataSource="backtestResult.sampleMatches || []"
                        :pagination="false"
                        size="small"
                        rowKey="index"
                        :scroll="{ x: 700 }"
                      >
                        <a-table-column title="#" dataIndex="index" :width="60" />
                        <a-table-column title="Command" dataIndex="commandLine" :width="260" ellipsis>
                          <template #default="{ record }">
                            <code>{{ maskSensitiveData(record.commandLine) }}</code>
                          </template>
                        </a-table-column>
                        <a-table-column title="Label" dataIndex="label" :width="90">
                          <template #default="{ record }">
                            <a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag>
                          </template>
                        </a-table-column>
                        <a-table-column title="User Label" dataIndex="userLabel" :width="120" />
                        <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="90">
                          <template #default="{ record }">
                            {{ record.anomalyScore?.toFixed(2) }}
                          </template>
                        </a-table-column>
                      </a-table>
                    </div>

                    <!-- Network Audit Findings -->
                    <div v-if="backtestResult.networkAudit && backtestResult.networkAudit.findings?.length > 0" style="margin-top: 16px">
                      <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                        <span>网络审计发现</span>
                        <a-tag :color="backtestResult.networkAudit.riskLevel === 'CRITICAL' ? 'red' : backtestResult.networkAudit.riskLevel === 'HIGH' ? 'orange' : backtestResult.networkAudit.riskLevel === 'MEDIUM' ? 'gold' : 'blue'">
                          {{ backtestResult.networkAudit.riskLevel }}
                        </a-tag>
                        <span style="color: #999; font-size: 12px">风险分：{{ backtestResult.networkAudit.riskScore?.toFixed(0) }}</span>
                      </div>
                      <a-list size="small" bordered :data-source="backtestResult.networkAudit.findings">
                        <template #renderItem="{ item }">
                          <a-list-item>
                            <a-list-item-meta>
                              <template #title>
                                <span style="display: flex; align-items: center; gap: 8px">
                                  <a-tag :color="item.severity === 'critical' ? 'red' : item.severity === 'high' ? 'orange' : item.severity === 'medium' ? 'gold' : 'blue'" size="small">
                                    {{ item.severity.toUpperCase() }}
                                  </a-tag>
                                  <span>{{ item.type }}</span>
                                </span>
                              </template>
                              <template #description>{{ item.description }}</template>
                            </a-list-item-meta>
                          </a-list-item>
                        </template>
                      </a-list>
                    </div>
                  </div>
                  <div v-else style="color: #999; text-align: center; padding: 40px">
                    输入命令并点击“判断安全性”查看评估结果；若已有完全匹配的标注样本，会优先作为判断证据。
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <!-- Row 6: Detection Thresholds -->
          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card title="Training / Validation Split" size="small">
              <template #extra>
                <a-tag color="purple">训练后会自动切分验证集，并可选做 LLM 后训练复核</a-tag>
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="12">
                  <span>Validation Split Ratio</span>
                  <a-slider v-model:value="mlTrainingConfig.validationSplitRatio" :min="0.1" :max="0.4" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlTrainingConfig.validationSplitRatio" :min="0.1" :max="0.4" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="12">
                  <div style="font-size: 13px; color: #666; line-height: 1.7; margin-top: 24px">
                    <div>• 训练时会先随机切分训练集 / 验证集，再分别记录 train / validation accuracy。</div>
                    <div>• 后训练阶段可用外部 OpenAI 风格 LLM 对验证集做批量复核。</div>
                    <div>• 若训练集打分选择“回写标签”，仅训练集会被改写，验证集只读。</div>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card title="Detection Thresholds" size="small">
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="8">
                  <span>Block Confidence Threshold</span>
                  <a-slider v-model:value="mlThresholds.blockConfidenceThreshold" :min="0.5" :max="1.0" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.blockConfidenceThreshold" :min="0.5" :max="1.0" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>ML Minimum Confidence</span>
                  <a-slider v-model:value="mlThresholds.mlMinConfidence" :min="0.3" :max="1.0" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.mlMinConfidence" :min="0.3" :max="1.0" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>Rule Override Priority</span>
                  <a-slider v-model:value="mlThresholds.ruleOverridePriority" :min="0" :max="200" :step="10" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.ruleOverridePriority" :min="0" :max="200" :step="10" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>Low Anomaly Threshold (below = normal)</span>
                  <a-slider v-model:value="mlThresholds.lowAnomalyThreshold" :min="0.0" :max="0.5" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.lowAnomalyThreshold" :min="0.0" :max="0.5" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>High Anomaly Threshold (above = alert)</span>
                  <a-slider v-model:value="mlThresholds.highAnomalyThreshold" :min="0.5" :max="1.0" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.highAnomalyThreshold" :min="0.5" :max="1.0" :step="0.05" size="small" style="width: 100%" />
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- Tab 4: Linux 6.18 LTS Docs -->
      <a-tab-pane key="docs" tab="Linux 6.18 LTS">
        <template #tab>
          <span><BookOutlined /> Linux 6.18 LTS</span>
        </template>

        <DocsLookupPanel />
      </a-tab-pane>

      <!-- Tab 5: Cluster Control -->
      <a-tab-pane key="cluster" tab="Cluster Control">
        <template #tab>
          <span><ClusterOutlined /> Cluster Control</span>
        </template>
        <a-row :gutter="[24, 24]">
          <a-col :span="24">
            <a-card title="Cluster Status" size="small">
              <template #extra>
                <ClusterOutlined />
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="10">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div
                      style="
                        display: flex;
                        align-items: center;
                        gap: 8px;
                        flex-wrap: wrap;
                      "
                    >
                      <span style="font-weight: 600">Mode</span>
                      <a-tag :color="clusterRoleColor">{{
                        clusterRoleText
                      }}</a-tag>
                      <a-tag
                        :color="
                          clusterState?.role === 'slave' ? 'orange' : 'green'
                        "
                      >
                        {{
                          clusterState?.role === "slave"
                            ? "Managed by master_url"
                            : "Default master mode"
                        }}
                      </a-tag>
                    </div>
                    <a-descriptions bordered size="small" :column="1">
                      <a-descriptions-item label="Node ID">
                        <span class="cluster-value">{{
                          clusterState?.nodeId || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item label="Node Name">
                        <span class="cluster-value">{{
                          clusterState?.nodeName || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item label="Node URL">
                        <span class="cluster-value">{{
                          clusterState?.nodeUrl || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item
                        v-if="clusterState?.role === 'slave'"
                        label="Master URL"
                      >
                        <span class="cluster-value">{{
                          clusterState?.masterUrl || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item label="Cluster Auth">
                        <span>
                          {{
                            clusterState?.accountConfigured
                              ? "account set"
                              : "account missing"
                          }}
                          /
                          {{
                            clusterState?.passwordConfigured
                              ? "password set"
                              : "password missing"
                          }}
                        </span>
                      </a-descriptions-item>
                    </a-descriptions>
                  </div>
                </a-col>
                <a-col :xs="24" :md="14">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <a-alert
                      type="info"
                      show-icon
                      message="Select the backend you want to inspect. All API/WS traffic is forwarded by the master."
                    />
                    <div
                      style="
                        display: flex;
                        gap: 12px;
                        align-items: center;
                        flex-wrap: wrap;
                      "
                    >
                      <span style="font-weight: 600">Active Target</span>
                      <a-select
                        v-model:value="selectedClusterTarget"
                        :options="clusterNodeOptions"
                        style="min-width: 280px; flex: 1"
                        :disabled="clusterState?.role === 'slave'"
                        @change="applyClusterTarget"
                      />
                      <a-button @click="fetchClusterNodes">
                        <ReloadOutlined /> Refresh Nodes
                      </a-button>
                    </div>
                    <a-table
                      :data-source="clusterNodes"
                      row-key="id"
                      size="small"
                      :pagination="false"
                      :row-class-name="getClusterRowClass"
                    >
                      <a-table-column title="Name" data-index="name" key="name">
                        <template #default="{ text, record }">
                          <span style="font-weight: 600">{{ text }}</span>
                          <a-tag
                            v-if="record.isLocal"
                            color="green"
                            style="margin-left: 8px"
                            >local</a-tag
                          >
                        </template>
                      </a-table-column>
                      <a-table-column title="Role" data-index="role" key="role">
                        <template #default="{ text }">
                          <a-tag
                            :color="text === 'slave' ? 'orange' : 'green'"
                            >{{ text }}</a-tag
                          >
                        </template>
                      </a-table-column>
                      <a-table-column
                        title="Status"
                        data-index="status"
                        key="status"
                      >
                        <template #default="{ text }">
                          <a-tag
                            :color="
                              text === 'online'
                                ? 'green'
                                : text === 'stale'
                                  ? 'orange'
                                  : 'default'
                            "
                            >{{ text }}</a-tag
                          >
                        </template>
                      </a-table-column>
                      <a-table-column title="URL" data-index="url" key="url">
                        <template #default="{ text }">
                          <span class="cluster-url">{{ text }}</span>
                        </template>
                      </a-table-column>
                      <a-table-column
                        title="Last Seen"
                        data-index="lastSeen"
                        key="lastSeen"
                      >
                        <template #default="{ text }">
                          <span>{{
                            text ? new Date(text).toLocaleString() : "—"
                          }}</span>
                        </template>
                      </a-table-column>
                      <a-table-column title="Action" key="action" width="120px">
                        <template #default="{ record }">
                          <a-button
                            v-if="
                              !record.isLocal && clusterState?.role === 'master'
                            "
                            type="link"
                            @click="applyClusterTarget(record.id)"
                          >
                            Route here
                          </a-button>
                          <span v-else style="color: #999">—</span>
                        </template>
                      </a-table-column>
                    </a-table>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>
    </a-tabs>

    <PathNavigatorDrawer
      v-model:open="pathPickerOpen"
      :title="pathPickerTarget === 'exact' ? 'Pick File' : 'Pick Directory'"
      :pick-mode="pathPickerTarget === 'exact' ? 'file' : 'directory'"
      @confirm="handlePathPicked"
    />
  </div>
</template>

<style scoped>
:deep(.ant-card) {
  border-radius: 8px;
}
.cluster-value {
  display: block;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid #dbeafe;
  background: linear-gradient(180deg, #f8fbff 0%, #eef4ff 100%);
  color: #1f2937;
  font-family: var(--mono);
  word-break: break-all;
}
.cluster-url {
  display: inline-block;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  background: #f8fafc;
  color: #111827;
  font-family: var(--mono);
  word-break: break-all;
  white-space: normal;
}
:deep(.cluster-row-active > td) {
  background: #f0f9eb !important;
}
:deep(.cluster-row-local > td) {
  background: #fafcff !important;
}
</style>
