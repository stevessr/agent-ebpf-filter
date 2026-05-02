<script setup lang="ts">
import {
  SafetyCertificateOutlined, StopOutlined, AlertOutlined, SwapOutlined,
  EyeOutlined, EyeInvisibleOutlined, PlusOutlined, ImportOutlined,
  DownloadOutlined, ArrowRightOutlined, FileOutlined, FolderOutlined,
  GlobalOutlined, ThunderboltOutlined, ControlOutlined, AppstoreOutlined,
} from '@ant-design/icons-vue';
import { quickRulePresets, externalRuleSources, syscallGroups, type useConfigSecurity } from '../../composables/useConfigSecurity';

const props = defineProps<{
  security: ReturnType<typeof useConfigSecurity>;
}>();

const {
  wrapperRules,
  newRuleComm, newRuleAction, newRuleRewritten,
  newRuleRegex, newRuleReplacement, newRulePriority, previewTestInput,
  disabledEventTypes,
  fetchedExternalRules, fetchSourceLoading, importingExternalRules,
  saveRule, deleteRule,
  addQuickRulePreset, addAllQuickRulePresets,
  fetchExternalRules, importAllFetchedRules,
  toggleEventType,
  regexPreviewResult,
} = props.security;
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-card title="Wrapper Security Policies" size="small">
        <template #extra><SafetyCertificateOutlined /></template>
        <a-alert type="info" show-icon style="margin-bottom: 12px;"
          message="快捷按钮会按 comm 精确匹配直接写入规则；更细的参数条件仍可在下方手动补充 regex、rewrite 或 priority。" />

        <!-- Quick Presets -->
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
            <a-tooltip v-for="preset in quickRulePresets" :key="`${preset.comm}-${preset.action}`"
              :title="`${preset.source} · ${preset.summary}`">
              <a-button size="small" :type="preset.action === 'BLOCK' ? 'primary' : 'default'"
                :danger="preset.action === 'BLOCK'"
                :style="preset.action === 'ALERT' ? 'border-color: #faad14; color: #d48806;' : ''"
                @click="addQuickRulePreset(preset)">
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
              <div style="font-weight: 600;">外部规则一键获取</div>
              <div style="font-size: 12px; color: #999;">
                从社区维护的 AI 代理安全规则集中获取最新规则，支持预览后一键导入。
              </div>
            </div>
          </div>
          <a-row :gutter="[12, 12]">
            <a-col v-for="source in externalRuleSources" :key="source.id" :xs="24" :sm="8">
              <a-card size="small" :hoverable="true">
                <div style="font-size: 13px; font-weight: 600; margin-bottom: 4px;">{{ source.name }}</div>
                <div style="font-size: 11px; color: #999; margin-bottom: 8px; min-height: 32px;">{{ source.description }}
                </div>
                <div style="display: flex; gap: 6px;">
                  <a-tag :color="source.category === 'owasp' ? 'orange' : 'blue'" style="font-size: 10px;">
                    {{ source.category === 'owasp' ? 'OWASP' : '社区' }}
                  </a-tag>
                  <a-button size="small" type="primary" ghost :loading="fetchSourceLoading === source.id"
                    @click="fetchExternalRules(source)">
                    <DownloadOutlined /> 获取
                  </a-button>
                </div>
              </a-card>
            </a-col>
          </a-row>
          <div v-if="fetchedExternalRules.length > 0" style="margin-top: 12px;">
            <a-alert type="success" show-icon :message="`已获取 ${fetchedExternalRules.length} 条规则`"
              style="margin-bottom: 8px;" />
            <a-table :dataSource="fetchedExternalRules"
              :columns="[{ title:'Command', dataIndex:'comm', key:'comm' }, { title:'Action', dataIndex:'action', key:'action' }, { title:'Priority', dataIndex:'priority', key:'priority' }]"
              size="small" :pagination="false" rowKey="comm" :scroll="{ y: 200 }">
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'comm'"><code>{{ record.comm }}</code></template>
                <template v-if="column.key === 'action'">
                  <a-tag :color="record.action === 'BLOCK' ? 'red' : 'orange'">{{ record.action }}</a-tag>
                </template>
              </template>
            </a-table>
            <div style="margin-top: 8px; text-align: right;">
              <a-button type="primary" size="small" :loading="importingExternalRules" @click="importAllFetchedRules">
                <ImportOutlined /> 一键导入 {{ fetchedExternalRules.length }} 条规则
              </a-button>
            </div>
          </div>
        </div>

        <!-- Manual Rule Form -->
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
              <span style="color: #999; font-size: 12px;">Intercepts and blocks or warns when the command is called via
                agent-wrapper</span>
            </a-col>
            <a-col :xs="24" :span="24" v-if="newRuleRegex" style="margin-top: 8px;">
              <div style="background: #e6f7ff; padding: 12px; border-radius: 4px; border: 1px solid #91caff;">
                <div style="font-size: 12px; font-weight: bold; margin-bottom: 8px; color: #003a8c;">Regex Live Preview:
                </div>
                <a-row :gutter="8" align="middle">
                  <a-col :span="11">
                    <a-input v-model:value="previewTestInput" size="small"
                      placeholder="Type example command arguments to test..." />
                  </a-col>
                  <a-col :span="2" style="text-align: center;">
                    <ArrowRightOutlined />
                  </a-col>
                  <a-col :span="11">
                    <div
                      style="background: #fff; padding: 4px 11px; border: 1px solid #d9d9d9; border-radius: 2px; min-height: 24px; font-family: monospace;">
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

        <!-- Rules Table -->
        <a-table
          :dataSource="Object.values(wrapperRules).sort((a,b) => (b.priority || 0) - (a.priority || 0))"
          size="small" rowKey="comm" :pagination="false">
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
        <a-alert type="info" show-icon style="margin-bottom: 16px;"
          message="Toggle individual syscall monitoring. Disabled syscalls are silently dropped in the kernel event pipeline — no events will be generated for them." />
        <a-row :gutter="[16, 16]">
          <a-col v-for="group in syscallGroups" :key="group.key" :xs="24" :sm="12" :lg="6">
            <div style="border: 1px solid #f0f0f0; border-radius: 8px; overflow: hidden; height: 100%;">
              <div
                :style="`background: ${group.color}; color: #fff; padding: 10px 14px; display: flex; align-items: center; gap: 8px;`">
                <FileOutlined v-if="group.icon === 'file'" />
                <FolderOutlined v-else-if="group.icon === 'folder'" />
                <GlobalOutlined v-else-if="group.icon === 'global'" />
                <ThunderboltOutlined v-else-if="group.icon === 'thunderbolt'" />
                <ControlOutlined v-else-if="group.icon === 'control'" />
                <SafetyCertificateOutlined v-else-if="group.icon === 'safety'" />
                <AppstoreOutlined v-else />
                <span style="font-weight: 600; font-size: 13px;">{{ group.title }}</span>
                <span style="margin-left: auto; font-size: 11px; opacity: 0.85;">{{
                  group.syscalls.filter(s => !disabledEventTypes.has(s.type)).length }}/{{ group.syscalls.length }}</span>
              </div>
              <div style="padding: 0;">
                <div v-for="s in group.syscalls" :key="s.type"
                  style="display: flex; align-items: center; justify-content: space-between; padding: 7px 14px; border-bottom: 1px solid #fafafa; transition: background 0.15s;"
                  :style="disabledEventTypes.has(s.type) ? 'opacity: 0.45;' : ''">
                  <div style="min-width: 0; flex: 1;">
                    <div style="font-size: 12px; font-weight: 600; font-family: monospace; color: #1f1f1f;">{{ s.name }}
                    </div>
                    <div
                      style="font-size: 11px; color: #999; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                      {{ s.desc }}</div>
                  </div>
                  <a-switch :checked="!disabledEventTypes.has(s.type)" size="small"
                    @change="toggleEventType(s.type, disabledEventTypes.has(s.type))">
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
</template>
