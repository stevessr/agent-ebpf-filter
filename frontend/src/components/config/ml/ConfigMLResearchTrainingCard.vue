<script setup lang="ts">
import {
  ExportOutlined,
  EyeOutlined,
  ImportOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import type { useConfigML } from "../../../composables/config/useConfigML";

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();
const {
  researchSessions,
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
  fetchResearchSessions,
  fetchResearchTrainingDataset,
  importResearchTrainingDataset,
  downloadResearchTrainingDataset,
  maskSensitiveData,
  getLabelColor,
} = props.ml;
</script>

<template>
  <!-- Research Session Training Dataset -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span>Research Session Training Dataset</span>
        <a-tag color="geekblue" style="margin-left: 8px"
          >Research v2 → 128-dim ML samples</a-tag
        >
      </template>
      <template #extra>
        <a-space wrap>
          <a-button
            size="small"
            @click="fetchResearchSessions()"
            :loading="loadingResearchSessions"
            ><ReloadOutlined /> 刷新会话</a-button
          >
          <a-button
            size="small"
            @click="fetchResearchTrainingDataset()"
            :loading="loadingResearchTraining"
            :disabled="!selectedResearchSessionId"
            ><EyeOutlined /> 预览训练集</a-button
          >
          <a-button
            size="small"
            type="primary"
            @click="importResearchTrainingDataset()"
            :loading="importingResearchTraining"
            :disabled="
              !selectedResearchSessionId ||
              researchTrainingLabelPolicy === 'unlabeled'
            "
            ><ImportOutlined /> 导入 ML 训练库</a-button
          >
          <a-button
            size="small"
            @click="downloadResearchTrainingDataset('jsonl')"
            :loading="exportingResearchTraining"
            :disabled="!selectedResearchSessionId"
            ><ExportOutlined /> JSONL</a-button
          >
          <a-button
            size="small"
            @click="downloadResearchTrainingDataset('csv')"
            :loading="exportingResearchTraining"
            :disabled="!selectedResearchSessionId"
            ><ExportOutlined /> CSV</a-button
          >
        </a-space>
      </template>
      <a-alert
        type="info"
        show-icon
        style="margin-bottom: 12px"
        message="把后端 Research Session 中已归一化、已脱敏的事件转换为 128 维训练样本；可按 decision/heuristic/unlabeled 策略生成标签，并导入现有 ML TrainingDataStore。"
        description="推荐先在 Research 页面/接口 build_session，再选择 decision 标签策略导入；unlabeled 适合导出给离线标注，不会直接导入有监督训练库。"
      />
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="10">
          <div style="display: flex; flex-direction: column; gap: 12px">
            <div>
              <div style="font-weight: 600; margin-bottom: 6px">
                Research Session
              </div>
              <a-select
                v-model:value="selectedResearchSessionId"
                show-search
                option-filter-prop="label"
                placeholder="选择已构建的研究会话"
                style="width: 100%"
                :loading="loadingResearchSessions"
              >
                <a-select-option
                  v-for="session in researchSessions"
                  :key="session.id"
                  :value="session.id"
                  :label="`${session.name} ${session.id} ${session.status}`"
                >
                  <div
                    style="
                      display: flex;
                      justify-content: space-between;
                      gap: 8px;
                    "
                  >
                    <span>{{ session.name }}</span>
                    <span style="color: #595959; font-size: 12px"
                      >{{ session.summary?.eventCount || 0 }} events ·
                      {{ session.status }}</span
                    >
                  </div>
                </a-select-option>
              </a-select>
            </div>
            <div style="display: flex; gap: 12px; flex-wrap: wrap">
              <div style="flex: 1; min-width: 180px">
                <div style="font-weight: 600; margin-bottom: 6px">标签策略</div>
                <a-select
                  v-model:value="researchTrainingLabelPolicy"
                  style="width: 100%"
                >
                  <a-select-option value="decision"
                    >decision（推荐导入）</a-select-option
                  >
                  <a-select-option value="heuristic"
                    >decision + 风险启发式</a-select-option
                  >
                  <a-select-option value="unlabeled"
                    >未标注（仅导出）</a-select-option
                  >
                </a-select>
              </div>
              <div style="flex: 1; min-width: 160px">
                <div style="font-weight: 600; margin-bottom: 6px">导入上限</div>
                <a-input-number
                  v-model:value="researchTrainingImportLimit"
                  :min="0"
                  :max="50000"
                  :step="100"
                  style="width: 100%"
                />
              </div>
            </div>
            <a-typography-text type="secondary">
              导入上限填 0
              表示按当前会话可转换样本全部导入；后端会自动跳过未标注、重复或无效命令样本。
            </a-typography-text>
            <a-alert
              v-if="selectedResearchSession"
              type="success"
              show-icon
              :message="`${selectedResearchSession.name} · ${selectedResearchSession.status}`"
              :description="`events=${selectedResearchSession.summary?.eventCount || 0}, top=${selectedResearchSession.summary?.topComm || selectedResearchSession.summary?.topEventType || 'n/a'}, maxRisk=${(selectedResearchSession.summary?.maxRiskScore || 0).toFixed(1)}`"
            />
            <a-alert
              v-else
              type="warning"
              show-icon
              message="暂无可选研究会话"
              description="点击“刷新会话”；如果列表仍为空，请先通过 /research/sessions 创建并执行 build_session。"
            />
          </div>
        </a-col>
        <a-col :xs="24" :md="14">
          <div style="display: flex; flex-direction: column; gap: 10px">
            <a-space wrap>
              <a-tag v-if="researchTrainingPreview" color="blue"
                >samples: {{ researchTrainingPreview.sampleCount }}</a-tag
              >
              <a-tag v-if="researchTrainingPreview" color="green"
                >labeled: {{ researchTrainingPreview.labeledCount }}</a-tag
              >
              <a-tag v-if="researchTrainingPreview" color="geekblue"
                >featureDim: {{ researchTrainingPreview.featureDim }}</a-tag
              >
              <a-tag v-if="researchTrainingPreview" color="cyan"
                >norm: {{ researchTrainingPreview.normalization.mode }}</a-tag
              >
              <a-tag
                v-for="label in researchTrainingPreview?.byLabel || []"
                :key="label.key"
                :color="getLabelColor(label.key)"
                >{{ label.key }}: {{ label.count }}</a-tag
              >
            </a-space>
            <a-alert
              v-if="researchTrainingPreview"
              type="success"
              show-icon
              :message="`训练视图已生成：${researchTrainingPreview.sampleCount} samples / ${researchTrainingPreview.labeledCount} labeled`"
              :description="`feature range ${researchTrainingPreview.normalization.minObserved.toFixed(3)}–${researchTrainingPreview.normalization.maxObserved.toFixed(3)}, nonFinite=${researchTrainingPreview.normalization.nonFiniteValues}, outOfRange=${researchTrainingPreview.normalization.belowZeroValues + researchTrainingPreview.normalization.aboveOneValues}`"
            />
            <a-alert
              v-if="researchTrainingImportResult"
              type="success"
              show-icon
              :message="`导入结果：新增 ${researchTrainingImportResult.imported}，跳过 ${researchTrainingImportResult.skipped}`"
              :description="`当前训练库：total=${researchTrainingImportResult.totalSamples}, labeled=${researchTrainingImportResult.labeledSamples}`"
            />
            <a-empty
              v-if="!researchTrainingPreview"
              description="选择 Research Session 后点击“预览训练集”"
            />
            <a-table
              v-else
              :dataSource="researchTrainingPreviewSamples"
              :pagination="false"
              :scroll="{ x: 980 }"
              size="small"
              rowKey="sampleId"
            >
              <a-table-column
                title="Command"
                dataIndex="commandLine"
                :width="280"
                ellipsis
              >
                <template #default="{ record }"
                  ><code>{{
                    maskSensitiveData(record.commandLine)
                  }}</code></template
                >
              </a-table-column>
              <a-table-column title="Event" dataIndex="eventType" :width="120">
                <template #default="{ record }"
                  ><a-tag size="small" color="geekblue">{{
                    record.eventType
                  }}</a-tag></template
                >
              </a-table-column>
              <a-table-column title="Decision" dataIndex="decision" :width="90">
                <template #default="{ record }">
                  <a-tag
                    v-if="record.decision"
                    :color="getLabelColor(record.decision)"
                    size="small"
                    >{{ record.decision }}</a-tag
                  >
                  <span v-else style="color: #6b7280">—</span>
                </template>
              </a-table-column>
              <a-table-column title="Label" dataIndex="labelName" :width="100">
                <template #default="{ record }"
                  ><a-tag
                    :color="getLabelColor(record.labelName)"
                    size="small"
                    >{{ record.labelName }}</a-tag
                  ></template
                >
              </a-table-column>
              <a-table-column title="Risk" dataIndex="riskScore" :width="80">
                <template #default="{ record }">{{
                  (record.riskScore || 0).toFixed(1)
                }}</template>
              </a-table-column>
              <a-table-column
                title="Feature"
                dataIndex="featureVector"
                :width="90"
              >
                <template #default="{ record }"
                  >{{ record.featureVector?.length || 0 }} dim</template
                >
              </a-table-column>
              <a-table-column title="Time" dataIndex="time" :width="180">
                <template #default="{ record }"
                  ><span style="font-size: 12px; color: #4a4a4a">{{
                    record.time ? new Date(record.time).toLocaleString() : "—"
                  }}</span></template
                >
              </a-table-column>
            </a-table>
          </div>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
