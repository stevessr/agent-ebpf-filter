<script setup lang="ts">
import {
  ThunderboltOutlined, ReloadOutlined, StopOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue';
import { highRiskPresets, type useConfigML } from '../../../composables/useConfigML';

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const {
  mlStatus, trainingModel, feedbackComm, feedbackAction,
  cudaAvailable, cudaMemUsedMB, cudaMemTotalMB, mlCRuntime, cancellingTraining,
  backtestCommandLine, backtesting, backtestResult,
  trainWithParams, cancelTraining, submitFeedback,
  runBacktest, runBacktestPreset, riskLevelColor, riskMeterColor,
  getLabelColor, maskSensitiveData,
} = props.ml;

type RuntimeBackendView = {
  id: string;
  label: string;
  available: boolean;
  accelerated: boolean;
  detail?: string;
};

const runtimeBackendColor = (backend: RuntimeBackendView) => {
  if (backend.accelerated) return 'green';
  if (backend.available) return 'blue';
  return 'default';
};

const runtimeBackendSuffix = (backend: RuntimeBackendView) => {
  if (backend.accelerated) return 'accelerated';
  if (backend.available) return 'available';
  return 'missing';
};

const runtimeBackendLabel = (backendId?: string) => {
  const labels: Record<string, string> = {
    c_cpu: 'Native C CPU',
    cuda: 'NVIDIA CUDA',
    intel_igpu: 'Intel iGPU',
  };
  return backendId ? labels[backendId] || backendId : '—';
};

const formatRuntimeMs = (value?: number) => {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '—';
  if (value < 0.001) return `${(value * 1000).toFixed(2)} µs`;
  if (value < 1) return `${value.toFixed(4)} ms`;
  return `${value.toFixed(2)} ms`;
};

const formatRuntimeSpeedup = (value?: number) => {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '—';
  return `${value.toFixed(2)}×`;
};
</script>

<template>
  <!-- Training Controls -->
  <a-col :xs="24" :md="12">
    <a-card title="Training Controls" size="small">
      <a-space direction="vertical" style="width: 100%">
        <div v-if="mlStatus.training_in_progress" style="background: #f6ffed; border: 1px solid #b7eb8f; border-radius: 6px; padding: 8px 12px; font-size: 12px;">
          <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 4px;">
            <span><ReloadOutlined spin style="margin-right: 4px; color: #52c41a;" /><b>训练中</b> {{ Math.round((mlStatus.training_progress || 0) * 100) }}%</span>
            <span v-if="cudaAvailable && cudaMemTotalMB > 0" style="color: #666;">GPU: {{ cudaMemUsedMB }} / {{ cudaMemTotalMB }} MB</span>
          </div>
          <a-progress :percent="Math.round((mlStatus.training_progress || 0) * 100)" :show-info="false" style="margin-top: 4px;" />
        </div>
        <div style="display: flex; gap: 8px;">
          <a-button type="primary" @click="trainWithParams" :loading="trainingModel" :disabled="mlStatus.training_in_progress" style="flex: 1">
            <ThunderboltOutlined /> Train Model Now
          </a-button>
          <a-popconfirm
            v-if="mlStatus.training_in_progress"
            title="确定要中止当前训练吗？"
            @confirm="cancelTraining"
            ok-text="中止训练"
            cancel-text="继续等待"
            ok-type="danger"
          >
            <a-button danger :loading="cancellingTraining"><StopOutlined /> 中止</a-button>
          </a-popconfirm>
        </div>
        <a-divider style="margin: 8px 0">Native C Runtime</a-divider>
        <div v-if="mlCRuntime" style="border: 1px solid #f0f0f0; border-radius: 6px; padding: 8px 10px; background: #fafafa;">
          <div style="display: flex; justify-content: space-between; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 8px;">
            <div style="font-weight: 600">
              C running time
              <a-tag :color="mlCRuntime.cSupported ? 'green' : 'orange'" style="margin-left: 6px">
                {{ mlCRuntime.cSupported ? 'kernel ready' : 'detect only' }}
              </a-tag>
            </div>
            <a-tag color="geekblue">active: {{ runtimeBackendLabel(mlCRuntime.activeBackend) }}</a-tag>
          </div>
          <a-space wrap size="small" style="margin-bottom: 8px">
            <a-tooltip v-for="backend in mlCRuntime.backends || []" :key="backend.id" :title="backend.detail || backend.label">
              <a-tag :color="runtimeBackendColor(backend)">
                {{ backend.label }} · {{ runtimeBackendSuffix(backend) }}
              </a-tag>
            </a-tooltip>
          </a-space>
          <a-descriptions :column="2" size="small" bordered>
            <a-descriptions-item label="Go inference">{{ formatRuntimeMs(mlCRuntime.goMsPerSample) }}/sample</a-descriptions-item>
            <a-descriptions-item label="C inference">{{ formatRuntimeMs(mlCRuntime.cMsPerSample) }}/sample</a-descriptions-item>
            <a-descriptions-item label="Speedup">{{ formatRuntimeSpeedup(mlCRuntime.speedup) }}</a-descriptions-item>
            <a-descriptions-item label="Samples">{{ mlCRuntime.sampleCount || 0 }}</a-descriptions-item>
            <a-descriptions-item label="Benchmark backend">{{ runtimeBackendLabel(mlCRuntime.benchmarkBackend) }}</a-descriptions-item>
            <a-descriptions-item label="Model">{{ mlCRuntime.modelType || mlStatus.model_type || '—' }}</a-descriptions-item>
          </a-descriptions>
          <div v-if="mlCRuntime.note" style="margin-top: 6px; color: #8c8c8c; font-size: 12px; line-height: 1.45">
            {{ mlCRuntime.note }}
          </div>
        </div>
        <a-alert v-else type="info" show-icon message="等待后端返回 C runtime / CUDA / Intel iGPU 状态" />
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

  <!-- Command Safety Assessment -->
  <a-col :xs="24">
    <a-card title="Command Safety Assessment" size="small">
      <template #extra><a-tag color="purple">输入完整命令进行安全性判断</a-tag></template>
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="8">
          <div style="font-weight: 600; margin-bottom: 8px">待判断命令</div>
          <a-space direction="vertical" style="width: 100%">
            <a-textarea v-model:value="backtestCommandLine" placeholder="完整命令 (e.g. sudo systemctl disable firewalld)" :auto-size="{ minRows: 3, maxRows: 6 }" @keyup.ctrl.enter="runBacktest" />
            <a-button type="primary" @click="runBacktest" :loading="backtesting" block><SearchOutlined /> 判断安全性</a-button>
          </a-space>
          <div style="margin-top: 12px; font-size: 12px; color: #999">
            快速测试：
            <a v-for="(p, i) in highRiskPresets.slice(0, 5)" :key="i" @click="runBacktestPreset(p.comm, p.args)" style="margin-right: 8px; white-space: nowrap">{{ p.comm }}</a>
          </div>
        </a-col>
        <a-col :xs="24" :md="16">
          <div v-if="backtestResult" style="display: flex; flex-direction: column; gap: 16px">
            <div style="display: flex; align-items: center; gap: 16px">
              <div style="flex: 1">
                <div style="font-weight: 600; margin-bottom: 4px">
                  风险评分：{{ backtestResult.riskScore?.toFixed(0) || '-' }} / 100
                  <a-tag :color="riskLevelColor(backtestResult.riskLevel)" style="margin-left: 8px">{{ backtestResult.riskLevel }}</a-tag>
                </div>
                <div style="background: #f0f0f0; border-radius: 8px; height: 20px; overflow: hidden">
                  <div :style="{ width: (backtestResult.riskScore || 0) + '%', height: '100%', background: riskMeterColor(backtestResult.riskScore || 0), borderRadius: '8px', transition: 'width 0.5s ease' }"></div>
                </div>
              </div>
              <div style="text-align: center; min-width: 80px">
                <div style="font-size: 28px; font-weight: bold; color: riskMeterColor(backtestResult.riskScore || 0)">{{ backtestResult.riskScore?.toFixed(0) || 0 }}</div>
                <div style="font-size: 11px; color: #999">/ 100</div>
              </div>
            </div>
            <a-descriptions :column="3" size="small" bordered>
              <a-descriptions-item label="Command">{{ backtestResult.commandLine || backtestResult.comm }}</a-descriptions-item>
              <a-descriptions-item label="Args">{{ backtestResult.args?.join(' ') || '—' }}</a-descriptions-item>
              <a-descriptions-item label="Recommended Action">
                <a-tag :color="backtestResult.recommendedAction === 'BLOCK' ? 'red' : backtestResult.recommendedAction === 'ALERT' ? 'orange' : backtestResult.recommendedAction === 'REWRITE' ? 'blue' : 'green'">{{ backtestResult.recommendedAction }}</a-tag>
              </a-descriptions-item>
              <a-descriptions-item label="Category"><a-tag>{{ backtestResult.classification?.primary_category || 'UNKNOWN' }}</a-tag></a-descriptions-item>
              <a-descriptions-item label="Classify Confidence">{{ backtestResult.classification?.confidence || '—' }}</a-descriptions-item>
              <a-descriptions-item label="Anomaly Score">
                <span :style="{ color: (backtestResult.anomalyScore ?? 0) > 0.7 ? '#d4380d' : (backtestResult.anomalyScore ?? 0) > 0.3 ? '#d48806' : '#52c41a' }">{{ backtestResult.anomalyScore?.toFixed(3) || '—' }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="ML Action">{{ backtestResult.mlPrediction?.action || '—' }}</a-descriptions-item>
              <a-descriptions-item label="ML Confidence">{{ backtestResult.mlPrediction?.confidence ? (backtestResult.mlPrediction.confidence * 100).toFixed(0) + '%' : '—' }}</a-descriptions-item>
              <a-descriptions-item label="Reasoning" :span="3">{{ backtestResult.reasoning || '—' }}</a-descriptions-item>
            </a-descriptions>
            <div v-if="backtestResult.llmAssessment" style="margin-top: 8px">
              <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                <span>LLM 打分结果</span>
                <a-tag :color="backtestResult.llmAssessment.error ? 'red' : 'purple'">{{ backtestResult.llmAssessment.error ? 'Error' : 'OpenAI-style' }}</a-tag>
              </div>
              <a-alert v-if="backtestResult.llmAssessment.error" type="error" show-icon :message="backtestResult.llmAssessment.error" style="margin-bottom: 8px" />
              <a-descriptions v-else :column="3" size="small" bordered>
                <a-descriptions-item label="Model">{{ backtestResult.llmAssessment.model || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Risk Score">{{ backtestResult.llmAssessment.riskScore?.toFixed(0) || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Confidence">{{ backtestResult.llmAssessment.confidence ? (backtestResult.llmAssessment.confidence * 100).toFixed(0) + '%' : '—' }}</a-descriptions-item>
                <a-descriptions-item label="Recommended Action">
                  <a-tag :color="backtestResult.llmAssessment.recommendedAction === 'BLOCK' ? 'red' : backtestResult.llmAssessment.recommendedAction === 'ALERT' ? 'orange' : backtestResult.llmAssessment.recommendedAction === 'REWRITE' ? 'blue' : 'green'">{{ backtestResult.llmAssessment.recommendedAction }}</a-tag>
                </a-descriptions-item>
                <a-descriptions-item label="Reasoning" :span="2">{{ backtestResult.llmAssessment.reasoning || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Signals" :span="3">
                  <a-space wrap>
                    <a-tag v-for="(signal, i) in backtestResult.llmAssessment.signals || []" :key="i" color="purple">{{ signal }}</a-tag>
                    <span v-if="(backtestResult.llmAssessment.signals || []).length === 0" style="color: #999">—</span>
                  </a-space>
                </a-descriptions-item>
              </a-descriptions>
            </div>
            <div v-if="backtestResult.sampleEvidence?.totalMatches > 0">
              <a-alert show-icon :type="backtestResult.sampleEvidence?.decision === 'BLOCK' ? 'error' : backtestResult.sampleEvidence?.decision === 'ALERT' ? 'warning' : 'info'"
                :message="`命中已有样本 ${backtestResult.sampleEvidence.totalMatches} 条，已标注 ${backtestResult.sampleEvidence.labeledMatches} 条`"
                :description="backtestResult.sampleEvidence?.decision ? `历史标注倾向：${backtestResult.sampleEvidence.decision}，置信度 ${(backtestResult.sampleEvidence.confidence * 100).toFixed(0)}%` : '暂无可直接用于判断的标注，但命令已存在于样本库。'"
                style="margin-bottom: 8px" />
              <a-table :dataSource="backtestResult.sampleMatches || []" :pagination="false" size="small" rowKey="index" :scroll="{ x: 700 }">
                <a-table-column title="#" dataIndex="index" :width="60" />
                <a-table-column title="Command" dataIndex="commandLine" :width="260" ellipsis>
                  <template #default="{ record }"><code>{{ maskSensitiveData(record.commandLine) }}</code></template>
                </a-table-column>
                <a-table-column title="Label" dataIndex="label" :width="90">
                  <template #default="{ record }"><a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag></template>
                </a-table-column>
                <a-table-column title="User Label" dataIndex="userLabel" :width="120" />
                <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="90">
                  <template #default="{ record }">{{ record.anomalyScore?.toFixed(2) }}</template>
                </a-table-column>
              </a-table>
            </div>
            <div v-if="backtestResult.networkAudit && backtestResult.networkAudit.findings?.length > 0" style="margin-top: 16px">
              <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                <span>网络审计发现</span>
                <a-tag :color="backtestResult.networkAudit.riskLevel === 'CRITICAL' ? 'red' : backtestResult.networkAudit.riskLevel === 'HIGH' ? 'orange' : backtestResult.networkAudit.riskLevel === 'MEDIUM' ? 'gold' : 'blue'">{{ backtestResult.networkAudit.riskLevel }}</a-tag>
                <span style="color: #999; font-size: 12px">风险分：{{ backtestResult.networkAudit.riskScore?.toFixed(0) }}</span>
              </div>
              <a-list size="small" bordered :data-source="backtestResult.networkAudit.findings">
                <template #renderItem="{ item }">
                  <a-list-item>
                    <a-list-item-meta>
                      <template #title>
                        <span style="display: flex; align-items: center; gap: 8px">
                          <a-tag :color="item.severity === 'critical' ? 'red' : item.severity === 'high' ? 'orange' : item.severity === 'medium' ? 'gold' : 'blue'" size="small">{{ item.severity.toUpperCase() }}</a-tag>
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
            输入命令并点击"判断安全性"查看评估结果；若已有完全匹配的标注样本，会优先作为判断证据。
          </div>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
