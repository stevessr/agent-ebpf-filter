<script setup lang="ts">
import {
  ReloadOutlined, CheckCircleOutlined, ExclamationCircleOutlined,
  ThunderboltOutlined, DownloadOutlined,
} from '@ant-design/icons-vue';
import type { useConfigML } from '../../../composables/useConfigML';

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const {
  llmScoringConfig, llmBatchConfig, llmBatchResponse, llmBatchLoading,
  llmSaveStatus, saveLLMConfigNow, llmApiKeyStatus,
  llmProductionDatasetLimit, llmProductionAllowHeuristic, llmProductionDeduplicate,
  llmProductionLoading, llmProductionPreview, llmProductionMeta,
  runLLMBatchScore, llmBatchRowKey, llmBatchCanApplyLabels,
  fetchLLMProductionDataset, exportLLMProductionDataset,
  maskSensitiveData, getLabelColor,
} = props.ml;
</script>

<template>
  <a-col :xs="24" :md="12">
    <a-card title="LLM Scoring" size="small">
      <template #extra>
        <a-space>
          <a-tag v-if="llmSaveStatus === 'saving'" color="processing"><ReloadOutlined spin /> Saving...</a-tag>
          <a-tag v-else-if="llmSaveStatus === 'saved'" color="success"><CheckCircleOutlined /> Saved</a-tag>
          <a-tag v-else-if="llmSaveStatus === 'error'" color="error"><ExclamationCircleOutlined /> Save Failed</a-tag>
          <a-tag color="purple">OpenAI-compatible API</a-tag>
        </a-space>
      </template>
      <a-space direction="vertical" style="width: 100%">
        <a-alert type="info" show-icon message="修改自动保存到浏览器本地并同步后端。API Key 留空则保留后端已保存的密钥。" />
        <a-row :gutter="[12, 12]">
          <a-col :xs="24">
            <a-space align="center" wrap>
              <a-switch v-model:checked="llmScoringConfig.enabled" />
              <span>启用 LLM 打分</span>
              <a-tag :color="llmApiKeyStatus.color">{{ llmApiKeyStatus.text }}</a-tag>
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
            <a-textarea v-model:value="llmScoringConfig.systemPrompt" :auto-size="{ minRows: 3, maxRows: 8 }" placeholder="你是安全行为分析器，只返回严格 JSON ..." />
          </a-col>
        </a-row>
        <div style="display: flex; align-items: center; gap: 8px;">
          <a-button size="small" type="primary" @click="saveLLMConfigNow" :loading="llmSaveStatus === 'saving'">
            <CheckCircleOutlined /> 保存 LLM 配置
          </a-button>
          <span v-if="llmSaveStatus === 'saved'" style="color: #52c41a; font-size: 12px;">已保存到后端</span>
          <span v-else-if="llmSaveStatus === 'error'" style="color: #ff4d4f; font-size: 12px;">保存失败，请重试</span>
          <span v-else-if="llmSaveStatus === 'idle'" style="color: #999; font-size: 12px;">修改后自动保存</span>
        </div>
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
          <a-alert type="success" show-icon
            :message="`已处理 ${llmBatchResponse.scored}/${llmBatchResponse.total} 条，平均风险 ${(llmBatchResponse.averageRiskScore ?? 0).toFixed(1)}，一致性 ${(llmBatchResponse.agreement * 100).toFixed(0)}%`"
            :description="llmBatchResponse.review?.validationSplitRatio !== undefined ? `验证集切分比例 ${(llmBatchResponse.review.validationSplitRatio * 100).toFixed(0)}%` : 'LLM 批量复核已完成。'" />
          <a-space wrap>
            <a-tag color="blue">source: {{ llmBatchResponse.source }}</a-tag>
            <a-tag color="geekblue">model: {{ llmBatchResponse.model }}</a-tag>
            <a-tag color="green">applied: {{ llmBatchResponse.applied }}</a-tag>
            <a-tag color="orange">skipped: {{ llmBatchResponse.skipped }}</a-tag>
          </a-space>
          <a-table :dataSource="llmBatchResponse.entries" :pagination="{ pageSize: 5, showSizeChanger: true, pageSizeOptions: ['5', '10', '20'] }" size="small" :rowKey="llmBatchRowKey" :scroll="{ x: 980 }">
            <a-table-column title="Command" dataIndex="commandLine" :width="280" ellipsis>
              <template #default="{ record }"><code>{{ maskSensitiveData(record.commandLine) }}</code></template>
            </a-table-column>
            <a-table-column title="Label" dataIndex="currentLabel" :width="100">
              <template #default="{ record }"><a-tag :color="getLabelColor(record.currentLabel || '-')">{{ record.currentLabel || '—' }}</a-tag></template>
            </a-table-column>
            <a-table-column title="Risk" dataIndex="riskScore" :width="90">
              <template #default="{ record }">{{ record.riskScore?.toFixed(0) }}</template>
            </a-table-column>
            <a-table-column title="Action" dataIndex="recommendedAction" :width="110">
              <template #default="{ record }">
                <a-tag :color="record.recommendedAction === 'BLOCK' ? 'red' : record.recommendedAction === 'ALERT' ? 'orange' : record.recommendedAction === 'REWRITE' ? 'blue' : 'green'">{{ record.recommendedAction }}</a-tag>
              </template>
            </a-table-column>
            <a-table-column title="Confidence" dataIndex="confidence" :width="110">
              <template #default="{ record }">{{ record.confidence ? (record.confidence * 100).toFixed(0) + '%' : '—' }}</template>
            </a-table-column>
            <a-table-column title="State" dataIndex="applied" :width="100">
              <template #default="{ record }">
                <a-tag v-if="record.error" color="red">Error</a-tag>
                <a-tag v-else-if="record.applied" color="green">Applied</a-tag>
                <a-tag v-else color="blue">Scored</a-tag>
              </template>
            </a-table-column>
            <a-table-column title="Reasoning" dataIndex="reasoning" ellipsis>
              <template #default="{ record }"><span>{{ record.reasoning || record.error || '—' }}</span></template>
            </a-table-column>
          </a-table>
        </div>
      </a-space>
    </a-card>
  </a-col>

  <!-- LLM Production Training Dataset -->
  <a-col :xs="24">
    <a-card title="LLM 生产训练集" size="small">
      <template #extra><a-tag color="green">来源：/config/ml/training</a-tag></template>
      <a-space direction="vertical" style="width: 100%">
        <a-alert type="info" show-icon message="直接从当前训练存储生成 OpenAI chat JSONL，不抓网页 HTML，也不会把未标注样本洗进训练集。默认只保留已标注样本，并按 commandLine + label 去重；如确实需要噪声样本，可手动打开启发式标签。" />
        <a-row :gutter="[12, 12]">
          <a-col :xs="24" :md="8">
            <div style="font-weight: 600; margin-bottom: 6px">样本上限</div>
            <a-input-number v-model:value="llmProductionDatasetLimit" :min="1" :max="5000" :step="1" style="width: 100%" />
          </a-col>
          <a-col :xs="24" :md="8">
            <a-space direction="vertical" style="width: 100%">
              <a-space align="center" wrap><a-switch v-model:checked="llmProductionDeduplicate" /><span>命令 + 标签去重</span></a-space>
              <a-space align="center" wrap><a-switch v-model:checked="llmProductionAllowHeuristic" /><span>允许启发式 / LLM 自动标签</span></a-space>
            </a-space>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-space direction="vertical" style="width: 100%">
              <a-button type="primary" @click="fetchLLMProductionDataset()" :loading="llmProductionLoading" block><ReloadOutlined /> 拉取当前训练集</a-button>
              <a-button @click="exportLLMProductionDataset()" :disabled="llmProductionPreview.length === 0 || llmProductionLoading" block><DownloadOutlined /> 导出 JSONL</a-button>
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
        <a-alert v-if="llmProductionMeta" type="success" show-icon :message="`已生成 ${llmProductionMeta.included} 条 LLM 生产训练样本`" :description="`导出 JSONL 每行仅包含 messages，适合 chat fine-tuning；系统提示词来自当前 LLM 配置：${llmProductionMeta.systemPrompt}`" />
        <a-alert v-else type="warning" show-icon message="点击“拉取当前训练集”后，会直接从训练存储生成可训练的 chat JSONL 预览。" />
        <a-table v-if="llmProductionPreview.length > 0" :dataSource="llmProductionPreview" :pagination="{ pageSize: 5, showSizeChanger: true, pageSizeOptions: ['5', '10', '20'] }" :scroll="{ x: 1280 }" size="small" rowKey="index">
          <a-table-column title="#" dataIndex="index" :width="70" />
          <a-table-column title="Command" dataIndex="commandLine" :width="260" ellipsis>
            <template #default="{ record }"><code>{{ maskSensitiveData(record.commandLine) }}</code></template>
          </a-table-column>
          <a-table-column title="Label" dataIndex="label" :width="100">
            <template #default="{ record }"><a-tag :color="getLabelColor(record.label)">{{ record.label }}</a-tag></template>
          </a-table-column>
          <a-table-column title="Risk" dataIndex="targetRiskScore" :width="90">
            <template #default="{ record }">{{ record.targetRiskScore?.toFixed(0) }}</template>
          </a-table-column>
          <a-table-column title="Confidence" dataIndex="targetConfidence" :width="110">
            <template #default="{ record }">{{ record.targetConfidence ? (record.targetConfidence * 100).toFixed(0) + '%' : '—' }}</template>
          </a-table-column>
          <a-table-column title="Source" dataIndex="userLabel" :width="140">
            <template #default="{ record }"><a-tag color="purple">{{ record.userLabel || '—' }}</a-tag></template>
          </a-table-column>
          <a-table-column title="Signals" dataIndex="signals" :width="220">
            <template #default="{ record }">
              <a-space wrap size="small"><a-tag v-for="(signal, i) in record.signals || []" :key="i" color="purple" size="small">{{ signal }}</a-tag></a-space>
            </template>
          </a-table-column>
          <a-table-column title="Reasoning" dataIndex="reasoning" ellipsis>
            <template #default="{ record }"><span>{{ record.reasoning || '—' }}</span></template>
          </a-table-column>
        </a-table>
        <a-card v-if="llmProductionPreview.length > 0" size="small" title="首条样本 JSON 预览">
          <a-descriptions :column="2" size="small" bordered>
            <a-descriptions-item label="Command">{{ maskSensitiveData(llmProductionPreview[0].commandLine) }}</a-descriptions-item>
            <a-descriptions-item label="Label"><a-tag :color="getLabelColor(llmProductionPreview[0].label)">{{ llmProductionPreview[0].label }}</a-tag></a-descriptions-item>
            <a-descriptions-item label="Prompt" :span="2"><a-textarea :value="maskSensitiveData(llmProductionPreview[0].prompt)" :auto-size="{ minRows: 4, maxRows: 8 }" readonly /></a-descriptions-item>
            <a-descriptions-item label="Completion" :span="2"><a-textarea :value="maskSensitiveData(llmProductionPreview[0].completion)" :auto-size="{ minRows: 4, maxRows: 8 }" readonly /></a-descriptions-item>
          </a-descriptions>
        </a-card>
      </a-space>
    </a-card>
  </a-col>
</template>
