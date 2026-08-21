<script setup lang="ts">
import { computed } from "vue";
import {
  FileOutlined,
  GlobalOutlined,
  ImportOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import { getCategoryColor } from "../../../composables/config/useConfigRegistry";
import type { useConfigML } from "../../../composables/config/useConfigML";

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();
const {
  remoteDatasetUrl,
  remoteDatasetFormat,
  remoteDatasetLabelMode,
  remoteDatasetCleanSensitive,
  remoteDatasetLimit,
  loadingRemoteDataset,
  importingRemoteDataset,
  remoteDatasetPreview,
  remoteDatasetMeta,
  trainingDatasetImportInput,
  fetchRemoteDatasetPreview,
  importRemoteDataset,
  importTrainingDatasetFromFile,
  openTrainingDatasetImportPicker,
  maskSensitiveData,
  getLabelColor,
} = props.ml;
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
</script>

<template>
  <!-- Internet Dataset Import -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span><GlobalOutlined /> 互联网数据集拉取</span>
        <a-tag color="blue" style="margin-left: 8px"
          >HTTP/HTTPS JSON、JSONL、CSV、TSV、SELinux .te</a-tag
        >
      </template>
      <template #extra>
        <a-space>
          <input
            type="file"
            ref="trainingDatasetImportInput"
            @change="importTrainingDatasetFromFile"
            style="display: none"
            accept=".json,.jsonl,.ndjson,.csv,.tsv,.txt,.log,.te,.if,.cil,.zip,.gz,.tgz,.tar,.bz2,.tbz,.tbz2,.txz"
          />
          <a-button
            size="small"
            @click="fetchRemoteDatasetPreview()"
            :loading="loadingRemoteDataset"
            ><ReloadOutlined /> 拉取预览</a-button
          >
          <a-button
            size="small"
            @click="openTrainingDatasetImportPicker()"
            :loading="importingRemoteDataset"
            ><FileOutlined /> 导入本地文件</a-button
          >
          <a-button
            size="small"
            type="primary"
            @click="importRemoteDataset()"
            :loading="importingRemoteDataset"
            ><ImportOutlined /> 导入训练集</a-button
          >
        </a-space>
      </template>
      <a-alert
        type="info"
        show-icon
        style="margin-bottom: 12px"
        message="后端只接受可直接 GET 到的原始数据文件；如果地址返回的是 HTML 介绍页、下载页或归档页，会直接报错。也可以用“导入本地文件”上传 JSON, JSONL, CSV, TSV, 纯文本、SELinux .te/.cil policy 或常见压缩包，后端会自动尝试解压 zip, gz, tar, tar.gz, tgz, bz2 等归档。"
        description="纯文本 policy 中的 allow/type_transition 会标为 ALLOW，neverallow 标为 BLOCK，dontaudit/auditallow/permissive 标为 ALERT；JSON rules[].rule / rules[].selinuxRule 也会按同样规则自动转换。"
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
                <a-select
                  v-model:value="remoteDatasetFormat"
                  style="width: 100%"
                >
                  <a-select-option value="auto">自动识别</a-select-option>
                  <a-select-option value="json">JSON</a-select-option>
                  <a-select-option value="jsonl"
                    >JSONL / NDJSON</a-select-option
                  >
                  <a-select-option value="csv">CSV</a-select-option>
                  <a-select-option value="tsv">TSV</a-select-option>
                  <a-select-option value="text">纯文本命令行</a-select-option>
                </a-select>
              </div>
              <div style="flex: 1; min-width: 180px">
                <div style="font-weight: 600; margin-bottom: 6px">标签模式</div>
                <a-select
                  v-model:value="remoteDatasetLabelMode"
                  style="width: 100%"
                >
                  <a-select-option value="preserve"
                    >保留原始标签</a-select-option
                  >
                  <a-select-option value="unlabeled"
                    >统一未标注</a-select-option
                  >
                  <a-select-option value="heuristic"
                    >按规则自动标注</a-select-option
                  >
                </a-select>
              </div>
            </div>
            <a-checkbox v-model:checked="remoteDatasetCleanSensitive">
              清洗敏感字段（推荐）
            </a-checkbox>
            <a-typography-text type="secondary"
              >会在导入前屏蔽密码、token、邮箱、IP、home
              目录等敏感片段，保留命令结构用于训练。</a-typography-text
            >
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
            <a-typography-text type="secondary"
              >支持公开数据集、实验室内网数据集或你自己的样本仓库，只要 URL
              可直接 GET 访问即可；SELinux policy 文本会在后端解析为
              selinux-rule 训练样本。</a-typography-text
            >
          </div>
        </a-col>
        <a-col :xs="24" :md="14">
          <div style="display: flex; flex-direction: column; gap: 10px">
            <a-space wrap>
              <a-tag v-if="remoteDatasetMeta" color="blue"
                >source: {{ remoteDatasetMeta.source }}</a-tag
              >
              <a-tag v-if="remoteDatasetMeta" color="cyan"
                >format: {{ remoteDatasetMeta.format }}</a-tag
              >
              <a-tag v-if="remoteDatasetMeta" color="geekblue"
                >type: {{ remoteDatasetMeta.contentType || "unknown" }}</a-tag
              >
              <a-tag v-if="remoteDatasetMeta" color="purple"
                >total: {{ remoteDatasetMeta.totalIsLowerBound ? "≥" : ""
                }}{{ remoteDatasetMeta.total }}</a-tag
              >
              <a-tag v-if="remoteDatasetMeta?.truncated" color="orange"
                >truncated</a-tag
              >
              <a-tag v-if="remoteDatasetMeta" color="green"
                >imported: {{ remoteDatasetMeta.imported ?? 0 }}</a-tag
              >
              <a-tag v-if="remoteDatasetMeta" color="gold"
                >skipped: {{ remoteDatasetMeta.skipped ?? 0 }}</a-tag
              >
              <a-tag v-if="remoteDatasetMeta?.quality" color="green">
                importable:
                {{ remoteDatasetMeta.quality.importableCount }}
              </a-tag>
              <a-tag v-if="remoteDatasetMeta?.quality" color="orange">
                unlabeled:
                {{ remoteDatasetMeta.quality.unlabeledCount }}
              </a-tag>
              <a-tag v-if="remoteDatasetMeta?.normalization" color="cyan">
                norm:
                {{ remoteDatasetMeta.normalization.mode }}
              </a-tag>
              <a-tag
                v-for="label in remoteDatasetMeta?.byLabel || []"
                :key="`remote-label-${label.key}`"
                :color="getLabelColor(label.key)"
              >
                {{ label.key }}: {{ label.count }}
              </a-tag>
            </a-space>
            <a-space
              v-if="
                (remoteDatasetMeta?.byCategory || []).length ||
                (remoteDatasetMeta?.bySource || []).length
              "
              wrap
            >
              <a-tag
                v-for="category in (remoteDatasetMeta?.byCategory || []).slice(
                  0,
                  6,
                )"
                :key="`remote-category-${category.key}`"
                :color="getCategoryColor(category.key)"
              >
                {{ category.key }}: {{ category.count }}
              </a-tag>
              <a-tag
                v-for="source in (remoteDatasetMeta?.bySource || []).slice(
                  0,
                  4,
                )"
                :key="`remote-source-${source.key}`"
                color="geekblue"
              >
                src {{ source.key }}: {{ source.count }}
              </a-tag>
            </a-space>
            <a-alert
              v-if="remoteDatasetMeta"
              type="success"
              show-icon
              :message="`已拉取 ${remoteDatasetMeta.total} 条，当前预览显示 ${remoteDatasetPreview.length} 条`"
              :description="
                remoteDatasetMeta.totalIsLowerBound
                  ? `数据集已达到 ${remoteDatasetMeta.recordLimit || remoteDatasetMeta.total} 条安全记录上限，实际总数可能更高。`
                  : remoteDatasetMeta.truncated
                    ? '列表已按 Limit 截断；importAll 仍受后端安全记录上限约束。'
                    : '列表展示的是当前请求返回的全部可见数据。'
              "
            />
            <a-alert
              v-if="remoteDatasetWarningText"
              type="warning"
              show-icon
              message="数据集质量提示"
              :description="remoteDatasetWarningText"
            />
            <a-alert
              v-if="!remoteDatasetMeta"
              type="warning"
              show-icon
              message="输入数据集 URL 后点击“拉取预览”，即可先查看格式识别和样本解析情况。"
            />
            <a-table
              :dataSource="remoteDatasetPreview"
              :pagination="{
                pageSize: 6,
                showSizeChanger: true,
                pageSizeOptions: ['6', '10', '20'],
              }"
              :scroll="{ x: 980 }"
              size="small"
              rowKey="row"
            >
              <a-table-column title="#" dataIndex="row" :width="60" />
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
              <a-table-column title="Label" dataIndex="label" :width="100">
                <template #default="{ record }"
                  ><a-tag :color="getLabelColor(record.label)" size="small">{{
                    record.label
                  }}</a-tag></template
                >
              </a-table-column>
              <a-table-column
                title="Category"
                dataIndex="category"
                :width="120"
              >
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
              <a-table-column
                title="Anomaly"
                dataIndex="anomalyScore"
                :width="90"
              >
                <template #default="{ record }">{{
                  record.anomalyScore?.toFixed(2)
                }}</template>
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
              <a-table-column title="Time" dataIndex="timestamp" :width="180">
                <template #default="{ record }"
                  ><span style="font-size: 12px; color: #4a4a4a">{{
                    record.timestamp
                      ? new Date(record.timestamp).toLocaleString()
                      : "—"
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
