<script setup lang="ts">
import {
  ImportOutlined, ExportOutlined, CopyOutlined, DeleteOutlined,
  FileOutlined, StopOutlined, AlertOutlined, SearchOutlined, PlusOutlined,
  EyeOutlined, EyeInvisibleOutlined, BookOutlined, GlobalOutlined, ReloadOutlined,
} from '@ant-design/icons-vue';
import { getCategoryColor } from '../../../composables/useConfigRegistry';
import { classicSecurityDatasetPresets, highRiskPresets, safetyNetHighRiskPresets, type useConfigML } from '../../../composables/useConfigML';

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const {
  allSamples, loadingSamples, sampleTablePageSize, sampleSearchText,
  existingDataLimit, existingLabelMode, existingCommandCandidates,
  loadingExistingData, importingExistingData, existingDataSource,
  remoteDatasetUrl, remoteDatasetFormat, remoteDatasetLabelMode, remoteDatasetLimit,
  loadingRemoteDataset, importingRemoteDataset, remoteDatasetPreview, remoteDatasetMeta,
  trainingDatasetImportInput, importingClassicDataset, dataMaskEnabled,
  sampleCommandLine, sampleLabel, submittingSample,
  filteredSamples, existingDuplicateCount, importableExistingCount,
  fetchAllSamples, fetchExistingCommandData, importExistingCommandData,
  fetchRemoteDatasetPreview, importRemoteDataset,
  importClassicDataset, openClassicSecurityDatasetPage, copyClassicSecurityDatasetPage,
  maskSensitiveData, getLabelColor,
  labelSample, deleteSample, updateAnomaly,
  importTrainingDatasetFromFile, exportTrainingDataset, clearTrainingDataset,
  openTrainingDatasetImportPicker,
  submitManualSample, addPresetSample, importAllHighRiskPresets,
  importAllSafetyNetPresets,
} = props.ml;

void trainingDatasetImportInput;
</script>

<template>
  <!-- Classic OS Security Datasets -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span><BookOutlined /> 经典 OS 安全数据集</span>
        <a-tag color="green" style="margin-left: 8px">支持一键导入</a-tag>
      </template>
      <a-alert type="info" show-icon style="margin-bottom: 12px" message="有下载链接的数据集可一键导入；无下载链接的会跳转官方页面，下载后用“导入本地文件”上传。导入器支持 zip, gz, tar, tgz, bz2 等归档及 JSON, JSONL, CSV, TSV, 纯文本。" />
      <a-list :data-source="classicSecurityDatasetPresets" :split="false" size="small">
        <template #renderItem="{ item }">
          <a-list-item>
            <a-card size="small" style="width: 100%">
              <a-space direction="vertical" style="width: 100%">
                <div style="display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; flex-wrap: wrap;">
                  <div>
                    <div style="font-weight: 600">{{ item.name }}</div>
                    <div style="color: #666; font-size: 12px">{{ item.note }}</div>
                  </div>
                  <a-space wrap>
                    <a-tag color="blue">{{ item.family }}</a-tag>
                    <a-tag color="geekblue">{{ item.platform }}</a-tag>
                  </a-space>
                </div>
                <a-space wrap>
                  <a-button size="small" type="primary" :loading="importingClassicDataset" @click="importClassicDataset(item)"><ImportOutlined /> {{ item.downloadUrl ? '一键导入' : '前往下载' }}</a-button>
                  <a-button size="small" @click="openClassicSecurityDatasetPage(item)"><GlobalOutlined /> 打开官网</a-button>
                  <a-button size="small" @click="copyClassicSecurityDatasetPage(item)"><CopyOutlined /> 复制链接</a-button>
                </a-space>
              </a-space>
            </a-card>
          </a-list-item>
        </template>
      </a-list>
    </a-card>
  </a-col>

  <!-- Internet Dataset Import -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span><GlobalOutlined /> 互联网数据集拉取</span>
        <a-tag color="blue" style="margin-left: 8px">HTTP/HTTPS JSON、JSONL、CSV、TSV、文本</a-tag>
      </template>
      <template #extra>
        <a-space>
          <input type="file" ref="trainingDatasetImportInput" @change="importTrainingDatasetFromFile" style="display: none" accept=".json,.jsonl,.ndjson,.csv,.tsv,.txt,.log,.zip,.gz,.tgz,.tar,.bz2,.tbz,.tbz2,.txz" />
          <a-button size="small" @click="fetchRemoteDatasetPreview()" :loading="loadingRemoteDataset"><ReloadOutlined /> 拉取预览</a-button>
          <a-button size="small" @click="openTrainingDatasetImportPicker()" :loading="importingRemoteDataset"><FileOutlined /> 导入本地文件</a-button>
          <a-button size="small" type="primary" @click="importRemoteDataset()" :loading="importingRemoteDataset"><ImportOutlined /> 导入训练集</a-button>
        </a-space>
      </template>
      <a-alert type="info" show-icon style="margin-bottom: 12px" message="后端只接受可直接 GET 到的原始数据文件；如果地址返回的是 HTML 介绍页、下载页或归档页，会直接报错。也可以用“导入本地文件”上传 JSON, JSONL, CSV, TSV, 纯文本或常见压缩包，后端会自动尝试解压 zip, gz, tar, tar.gz, tgz, bz2 等归档。" />
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="10">
          <div style="display: flex; flex-direction: column; gap: 12px">
            <div>
              <div style="font-weight: 600; margin-bottom: 6px">数据集 URL</div>
              <a-input v-model:value="remoteDatasetUrl" placeholder="https://example.com/dataset.jsonl" allow-clear />
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
              <a-input-number v-model:value="remoteDatasetLimit" :min="1" :max="5000" :step="1" style="width: 100%" />
            </div>
            <a-typography-text type="secondary">支持公开数据集、实验室内网数据集或你自己的样本仓库，只要 URL 可直接 GET 访问即可。</a-typography-text>
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
            <a-alert v-if="remoteDatasetMeta" type="success" show-icon :message="`已拉取 ${remoteDatasetMeta.total} 条，当前预览显示 ${remoteDatasetPreview.length} 条`" :description="remoteDatasetMeta.truncated ? '列表已按 Limit 截断，导入时也会使用同样的条数上限。' : '列表展示的是当前请求返回的全部可见数据。'" />
            <a-alert v-else type="warning" show-icon message="输入数据集 URL 后点击“拉取预览”，即可先查看格式识别和样本解析情况。" />
            <a-table :dataSource="remoteDatasetPreview" :pagination="{ pageSize: 6, showSizeChanger: true, pageSizeOptions: ['6', '10', '20'] }" :scroll="{ x: 980 }" size="small" rowKey="row">
              <a-table-column title="#" dataIndex="row" :width="60" />
              <a-table-column title="Command" dataIndex="commandLine" :width="280" ellipsis>
                <template #default="{ record }"><code>{{ maskSensitiveData(record.commandLine) }}</code></template>
              </a-table-column>
              <a-table-column title="Label" dataIndex="label" :width="100">
                <template #default="{ record }"><a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag></template>
              </a-table-column>
              <a-table-column title="Category" dataIndex="category" :width="120">
                <template #default="{ record }">
                  <a-tag v-if="record.category" :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
                  <span v-else style="color: #999">—</span>
                </template>
              </a-table-column>
              <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="90">
                <template #default="{ record }">{{ record.anomalyScore?.toFixed(2) }}</template>
              </a-table-column>
              <a-table-column title="State" dataIndex="duplicate" :width="100">
                <template #default="{ record }"><a-tag :color="record.duplicate ? 'default' : 'green'" size="small">{{ record.duplicate ? '已存在' : '可导入' }}</a-tag></template>
              </a-table-column>
              <a-table-column title="Time" dataIndex="timestamp" :width="180">
                <template #default="{ record }"><span style="font-size: 12px; color: #666">{{ record.timestamp ? new Date(record.timestamp).toLocaleString() : '—' }}</span></template>
              </a-table-column>
            </a-table>
          </div>
        </a-col>
      </a-row>
    </a-card>
  </a-col>

  <!-- Existing Command Data -->
  <a-col :xs="24">
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
          <a-button size="small" @click="fetchExistingCommandData()" :loading="loadingExistingData"><ReloadOutlined /> 拉取已有数据</a-button>
          <a-button size="small" type="primary" @click="importExistingCommandData()" :loading="importingExistingData" :disabled="importableExistingCount <= 0"><ImportOutlined /> 导入 {{ importableExistingCount }}</a-button>
        </a-space>
      </template>
      <a-alert type="info" show-icon style="margin-bottom: 12px" message="从 /events/recent 读取历史 wrapper_intercept / native_hook 命令。默认导入为未标注样本；选择“按安全判断标注”会用当前规则/ML/网络审计结果自动给出 ALLOW/ALERT/BLOCK 标签。" />
      <div style="display: flex; gap: 8px; align-items: center; margin-bottom: 8px; flex-wrap: wrap">
        <a-tag v-if="existingDataSource" color="blue">source: {{ existingDataSource }}</a-tag>
        <a-tag color="purple">{{ existingCommandCandidates.length }} pulled</a-tag>
        <a-tag color="default">{{ existingDuplicateCount }} duplicates</a-tag>
      </div>
      <a-table :dataSource="existingCommandCandidates" :pagination="{ pageSize: 8, showSizeChanger: true, pageSizeOptions: ['8','15','30'] }" :scroll="{ x: 900 }" size="small" rowKey="commandLine">
        <a-table-column title="Command" dataIndex="commandLine" :width="300" ellipsis>
          <template #default="{ record }"><code>{{ maskSensitiveData(record.commandLine) }}</code></template>
        </a-table-column>
        <a-table-column title="Event" dataIndex="eventType" :width="120">
          <template #default="{ record }"><a-tag size="small" color="geekblue">{{ record.eventType }}</a-tag></template>
        </a-table-column>
        <a-table-column title="Category" dataIndex="category" :width="120">
          <template #default="{ record }">
            <a-tag v-if="record.category" :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
            <span v-else style="color: #999">—</span>
          </template>
        </a-table-column>
        <a-table-column title="Time" dataIndex="timestamp" :width="180">
          <template #default="{ record }"><span style="font-size: 12px; color: #666">{{ record.timestamp ? new Date(record.timestamp).toLocaleString() : '—' }}</span></template>
        </a-table-column>
        <a-table-column title="State" dataIndex="duplicate" :width="100">
          <template #default="{ record }"><a-tag :color="record.duplicate ? 'default' : 'green'" size="small">{{ record.duplicate ? '已存在' : '可导入' }}</a-tag></template>
        </a-table-column>
      </a-table>
    </a-card>
  </a-col>

  <!-- Training Data Browser -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span>Training Data Browser</span>
        <a-tag color="purple" style="margin-left: 8px">{{ filteredSamples.length }} / {{ allSamples.length }}</a-tag>
      </template>
      <template #extra>
        <a-space wrap>
          <a-button size="small" @click="dataMaskEnabled = !dataMaskEnabled" :type="dataMaskEnabled ? 'primary' : 'default'">
            <component :is="dataMaskEnabled ? EyeInvisibleOutlined : EyeOutlined" />
            {{ dataMaskEnabled ? '脱敏' : '明文' }}
          </a-button>
          <a-button size="small" @click="exportTrainingDataset()"><ExportOutlined /> 导出训练集</a-button>
          <a-popconfirm title="确定要清空当前训练集吗？" @confirm="clearTrainingDataset()">
            <a-button size="small" danger><DeleteOutlined /> 清空训练集</a-button>
          </a-popconfirm>
          <a-input v-model:value="sampleSearchText" placeholder="搜索命令或参数..." size="small" style="width: 200px" allow-clear>
            <template #prefix><SearchOutlined /></template>
          </a-input>
          <a-button size="small" @click="fetchAllSamples()" :loading="loadingSamples"><ReloadOutlined /> Refresh</a-button>
        </a-space>
      </template>
      <a-table :dataSource="filteredSamples" :pagination="{ pageSize: sampleTablePageSize, showSizeChanger: true, pageSizeOptions: ['10','15','30','50'], showTotal: (t:number) => `${t} samples` }" :scroll="{ x: 1100 }" size="small" rowKey="index">
        <a-table-column title="#" dataIndex="index" :width="50" />
        <a-table-column title="Command" dataIndex="commandLine" :width="240" ellipsis>
          <template #default="{ record }"><code>{{ maskSensitiveData(record.commandLine || [record.comm, ...(record.args || [])].filter(Boolean).join(' ')) }}</code></template>
        </a-table-column>
        <a-table-column title="Comm" dataIndex="comm" :width="100">
          <template #default="{ record }"><strong>{{ record.comm }}</strong></template>
        </a-table-column>
        <a-table-column title="Args" dataIndex="args" :width="200" ellipsis>
          <template #default="{ record }"><span style="font-size: 12px; color: #666">{{ maskSensitiveData((record.args || []).join(' ')) || '—' }}</span></template>
        </a-table-column>
        <a-table-column title="Category" dataIndex="category" :width="110">
          <template #default="{ record }"><a-tag :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag></template>
        </a-table-column>
        <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="100">
          <template #default="{ record }">
            <a-input-number v-model:value="record.anomalyScore" :min="0" :max="1" :step="0.01" :precision="2" size="small" style="width: 70px" @change="updateAnomaly(record.index, record.anomalyScore)" />
          </template>
        </a-table-column>
        <a-table-column title="Label" dataIndex="label" :width="90">
          <template #default="{ record }"><a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag></template>
        </a-table-column>
        <a-table-column title="Actions" :width="240">
          <template #default="{ record }">
            <a-space :size="4">
              <a-button size="small" type="primary" ghost @click="labelSample(record.index, 'ALLOW')" :disabled="record.label === 'ALLOW'">ALLOW</a-button>
              <a-button size="small" style="border-color: #faad14; color: #d48806" ghost @click="labelSample(record.index, 'ALERT')" :disabled="record.label === 'ALERT'">ALERT</a-button>
              <a-button size="small" danger ghost @click="labelSample(record.index, 'BLOCK')" :disabled="record.label === 'BLOCK'">BLOCK</a-button>
              <a-button size="small" danger type="text" @click="deleteSample(record.index)"><DeleteOutlined /></a-button>
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
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <div style="font-weight: 600">高危行为预设（点击即可添加已标注样本）</div>
            <a-button size="small" type="link" @click="importAllHighRiskPresets()">一键导入全部预设</a-button>
          </div>
          <a-space wrap>
            <a-tag v-for="(p, i) in highRiskPresets" :key="i" :color="p.label === 'BLOCK' ? 'red' : 'orange'" style="cursor: pointer; padding: 4px 8px; font-size: 13px" @click="addPresetSample(p)">
              {{ p.comm }} {{ p.args ? p.args.slice(0, 30) + '…' : '' }}
              <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
            </a-tag>
          </a-space>
        </a-col>
        <a-col :xs="24" :md="10">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
            <div style="font-weight: 600">Claude Code Safety Net 预设</div>
            <a-button size="small" type="link" @click="importAllSafetyNetPresets()">一键导入全部</a-button>
          </div>
          <a-space wrap>
            <a-tag v-for="(p, i) in safetyNetHighRiskPresets" :key="'sn'+i" :color="p.label === 'BLOCK' ? 'red' : p.label === 'ALLOW' ? 'green' : 'orange'" style="cursor: pointer; padding: 4px 8px; font-size: 12px" @click="addPresetSample(p)">
              <code>{{ p.comm }}</code> {{ p.args.slice(0, 35) }}{{ p.args.length > 35 ? '…' : '' }}
              <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
            </a-tag>
          </a-space>
        </a-col>
        <a-col :xs="24" :md="10">
          <div style="font-weight: 600; margin-bottom: 8px">Step 1: 输入完整命令行</div>
          <a-input v-model:value="sampleCommandLine" placeholder="完整命令 (支持管道: cat file.txt | grep error | wc -l)" size="small" style="margin-bottom: 10px" @keyup.enter="submitManualSample()" />
          <div style="font-weight: 600; margin-bottom: 8px">Step 2: 标注行为 <a-tag color="processing" size="small">选择标签</a-tag></div>
          <div style="display: flex; gap: 8px; margin-bottom: 6px">
            <a-radio-group v-model:value="sampleLabel" button-style="solid" size="small">
              <a-radio-button value="BLOCK" style="border-color: #ff4d4f; color: #ff4d4f"><StopOutlined /> BLOCK 拦截</a-radio-button>
              <a-radio-button value="ALERT" style="border-color: #faad14; color: #d48806"><AlertOutlined /> ALERT 警报</a-radio-button>
              <a-radio-button value="ALLOW" style="border-color: #52c41a; color: #52c41a"><span style="font-size: 11px">&#10003;</span> ALLOW 放行</a-radio-button>
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
          <a-button type="primary" @click="submitManualSample()" :loading="submittingSample" block><PlusOutlined /> 添加此标注样本</a-button>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
