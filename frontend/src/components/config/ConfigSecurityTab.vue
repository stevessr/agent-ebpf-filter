<script setup lang="ts">
import {
  SafetyCertificateOutlined, StopOutlined, AlertOutlined, SwapOutlined,
  EyeOutlined, EyeInvisibleOutlined, PlusOutlined, ImportOutlined,
  DownloadOutlined, ArrowRightOutlined, FileOutlined, FolderOutlined,
  GlobalOutlined, ThunderboltOutlined, ControlOutlined, AppstoreOutlined,
  ReloadOutlined,
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
  cgroupSandboxStatus, cgroupSandboxLoading,
  cgroupTargetID, cgroupTargetPID, cgroupTargetIP, cgroupTargetPort,
  lsmEnforcerStatus, lsmEnforcerLoading,
  lsmExecPath, lsmExecName, lsmFileName,
  fetchedExternalRules, fetchSourceLoading, importingExternalRules,
  saveRule, deleteRule,
  addQuickRulePreset, addAllQuickRulePresets,
  fetchExternalRules, importAllFetchedRules,
  toggleEventType,
  fetchCgroupSandboxStatus,
  blockCgroupID, unblockCgroupID,
  blockCgroupPID, unblockCgroupPID,
  blockCgroupIP, unblockCgroupIP,
  blockCgroupPort, unblockCgroupPort,
  fetchLsmEnforcerStatus,
  blockLsmExecPath, unblockLsmExecPath,
  blockLsmExecName, unblockLsmExecName,
  blockLsmFileName, unblockLsmFileName,
  regexPreviewResult,
} = props.security;

const unblockCgroupIDFromTag = async (id: string) => {
  cgroupTargetID.value = id;
  await unblockCgroupID();
};

const unblockCgroupIPFromTag = async (ip: string) => {
  cgroupTargetIP.value = ip;
  await unblockCgroupIP();
};

const unblockCgroupPortFromTag = async (port: number) => {
  cgroupTargetPort.value = port;
  await unblockCgroupPort();
};
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

    <!-- OS-level cgroup interception -->
    <a-col :span="24">
      <a-card title="OS-Level cgroup Network Interception" size="small">
        <template #extra>
          <a-space>
            <a-tag :color="cgroupSandboxStatus.available && cgroupSandboxStatus.attached ? 'green' : 'red'">
              {{ cgroupSandboxStatus.available && cgroupSandboxStatus.attached ? 'kernel blocking active' : 'not active' }}
            </a-tag>
            <a-button size="small" :loading="cgroupSandboxLoading" @click="fetchCgroupSandboxStatus">
              <ReloadOutlined /> Refresh
            </a-button>
          </a-space>
        </template>
        <a-alert type="warning" show-icon style="margin-bottom: 16px;"
          message="这里写入的是 cgroup/connect4 + connect6 + sendmsg4 + sendmsg6 eBPF map，命中后连接或 UDP sendto/sendmsg 在内核阶段直接失败；支持 TCP/UDP connected sockets 与 UDP sendto/sendmsg 的 cgroup、IPv4/IPv6 目的地址和端口阻断，IPv4 block 也会覆盖 ::ffff:a.b.c.d 形式的 IPv4-mapped IPv6 socket，不同于 wrapper/hook，只覆盖网络出站拦截。" />

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="10">
            <a-descriptions size="small" bordered :column="1">
              <a-descriptions-item label="Attach path">
                <code>{{ cgroupSandboxStatus.cgroupPath || 'not attached' }}</code>
              </a-descriptions-item>
              <a-descriptions-item label="Maps">
                <a-space wrap>
                  <a-tag :color="cgroupSandboxStatus.maps.cgroupBlocklist ? 'green' : 'default'">cgroup</a-tag>
                  <a-tag :color="cgroupSandboxStatus.maps.ipBlocklist ? 'green' : 'default'">ipv4</a-tag>
                  <a-tag :color="cgroupSandboxStatus.maps.ip6Blocklist ? 'green' : 'default'">ipv6</a-tag>
                  <a-tag :color="cgroupSandboxStatus.maps.portBlocklist ? 'green' : 'default'">port</a-tag>
                  <a-tag :color="cgroupSandboxStatus.maps.stats ? 'green' : 'default'">stats</a-tag>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Pinned links">
                <span v-if="!cgroupSandboxStatus.linkPins.length" style="color: #999;">process-held or unavailable</span>
                <div v-for="pin in cgroupSandboxStatus.linkPins" :key="pin"><code>{{ pin }}</code></div>
              </a-descriptions-item>
              <a-descriptions-item label="Active blocks">
                <a-space wrap>
                  <a-tag v-for="id in cgroupSandboxStatus.blockedCgroups" :key="`cg-${id}`" color="red" closable @close.prevent="unblockCgroupIDFromTag(id)">
                    cgroup {{ id }}
                  </a-tag>
                  <a-tag v-for="ip in cgroupSandboxStatus.blockedIPs" :key="`ip-${ip}`" color="volcano" closable @close.prevent="unblockCgroupIPFromTag(ip)">
                    ip {{ ip }}
                  </a-tag>
                  <a-tag v-for="port in cgroupSandboxStatus.blockedPorts" :key="`port-${port}`" color="orange" closable @close.prevent="unblockCgroupPortFromTag(port)">
                    port {{ port }}
                  </a-tag>
                  <span v-if="!cgroupSandboxStatus.blockedCgroups.length && !cgroupSandboxStatus.blockedIPs.length && !cgroupSandboxStatus.blockedPorts.length" style="color: #999;">
                    No active cgroup/connect or sendmsg blocks
                  </span>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Error">
                <span v-if="!cgroupSandboxStatus.error && !cgroupSandboxStatus.statsError" style="color: #52c41a;">OK</span>
                <span v-else style="color: #cf1322;">{{ cgroupSandboxStatus.error || cgroupSandboxStatus.statsError }}</span>
              </a-descriptions-item>
            </a-descriptions>
          </a-col>

          <a-col :xs="24" :lg="6">
            <a-card size="small" title="Kernel decision counters">
              <a-row :gutter="[8, 8]">
                <a-col :span="8">
                  <a-statistic title="Checked" :value="cgroupSandboxStatus.stats.checked" />
                </a-col>
                <a-col :span="8">
                  <a-statistic title="Blocked" :value="cgroupSandboxStatus.stats.blocked" />
                </a-col>
                <a-col :span="8">
                  <a-statistic title="Allowed" :value="cgroupSandboxStatus.stats.allowed" />
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col :xs="24" :lg="8">
            <div style="display: grid; gap: 12px;">
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Block / unblock cgroup outbound</div>
                <a-input-group compact>
                  <a-input v-model:value="cgroupTargetID" style="width: calc(100% - 160px)" placeholder="cgroup id from events" />
                  <a-button danger :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="blockCgroupID">Block</a-button>
                  <a-button :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="unblockCgroupID">Unblock</a-button>
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Block / unblock PID's cgroup</div>
                <a-input-group compact>
                  <a-input-number v-model:value="cgroupTargetPID" style="width: calc(100% - 160px)" :min="1" placeholder="PID" />
                  <a-button danger :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="blockCgroupPID">Block</a-button>
                  <a-button :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="unblockCgroupPID">Unblock</a-button>
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Block / unblock IP globally</div>
                <a-input-group compact>
                  <a-input v-model:value="cgroupTargetIP" style="width: calc(100% - 160px)" placeholder="1.2.3.4, ::ffff:1.2.3.4, or ::1" />
                  <a-button danger :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="blockCgroupIP">Block</a-button>
                  <a-button :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="unblockCgroupIP">Unblock</a-button>
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Block / unblock destination port globally</div>
                <a-input-group compact>
                  <a-input-number v-model:value="cgroupTargetPort" style="width: calc(100% - 160px)" :min="1" :max="65535" />
                  <a-button danger :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="blockCgroupPort">Block</a-button>
                  <a-button :disabled="!cgroupSandboxStatus.available" :loading="cgroupSandboxLoading" @click="unblockCgroupPort">Unblock</a-button>
                </a-input-group>
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>

    <!-- OS-level BPF LSM interception -->
    <a-col :span="24">
      <a-card title="OS-Level BPF LSM File / Exec Interception" size="small">
        <template #extra>
          <a-space>
            <a-tag :color="lsmEnforcerStatus.available && lsmEnforcerStatus.attached ? 'green' : 'red'">
              {{ lsmEnforcerStatus.available && lsmEnforcerStatus.attached ? 'BPF LSM active' : 'not active' }}
            </a-tag>
            <a-button size="small" :loading="lsmEnforcerLoading" @click="fetchLsmEnforcerStatus">
              <ReloadOutlined /> Refresh
            </a-button>
          </a-space>
        </template>
        <a-alert type="warning" show-icon style="margin-bottom: 16px;"
          message="这里写入的是 BPF LSM map：bprm_check_security 可按执行路径或可执行文件 basename 拒绝 exec；file_open、file_permission、mmap_file、file_mprotect、inode_setattr、inode_create、inode_link、inode_symlink、inode_unlink、inode_mkdir、inode_rmdir、inode_mknod、inode_rename 可按文件或目录 basename 拒绝打开、既有 fd 读写、mmap、mprotect、setattr、创建、link、symlink、删除、mkdir、rmdir、mknod 与 rename。该路径在内核 LSM 决策点返回 EACCES。" />

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="9">
            <a-descriptions size="small" bordered :column="1">
              <a-descriptions-item label="Hooks">
                <a-space wrap>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">bprm_check_security</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">file_open</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">file_permission</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">mmap_file</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">file_mprotect</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_setattr</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_create</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_link</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_symlink</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_unlink</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_mkdir</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_rmdir</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_mknod</a-tag>
                  <a-tag :color="lsmEnforcerStatus.attached ? 'green' : 'default'">inode_rename</a-tag>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Maps">
                <a-space wrap>
                  <a-tag :color="lsmEnforcerStatus.maps.execPathBlocklist ? 'green' : 'default'">exec paths</a-tag>
                  <a-tag :color="lsmEnforcerStatus.maps.execNameBlocklist ? 'green' : 'default'">exec names</a-tag>
                  <a-tag :color="lsmEnforcerStatus.maps.fileNameBlocklist ? 'green' : 'default'">file names</a-tag>
                  <a-tag :color="lsmEnforcerStatus.maps.stats ? 'green' : 'default'">stats</a-tag>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Pinned links">
                <span v-if="!lsmEnforcerStatus.linkPins.length" style="color: #999;">process-held or unavailable</span>
                <div v-for="pin in lsmEnforcerStatus.linkPins" :key="pin"><code>{{ pin }}</code></div>
              </a-descriptions-item>
              <a-descriptions-item label="Error">
                <span v-if="!lsmEnforcerStatus.error && !lsmEnforcerStatus.statsError" style="color: #52c41a;">OK</span>
                <span v-else style="color: #cf1322;">{{ lsmEnforcerStatus.error || lsmEnforcerStatus.statsError }}</span>
              </a-descriptions-item>
            </a-descriptions>
          </a-col>

          <a-col :xs="24" :lg="6">
            <a-card size="small" title="LSM decision counters">
              <a-row :gutter="[8, 8]">
                <a-col :span="12"><a-statistic title="Exec checked" :value="lsmEnforcerStatus.stats.execChecked" /></a-col>
                <a-col :span="12"><a-statistic title="Exec blocked" :value="lsmEnforcerStatus.stats.execBlocked" /></a-col>
                <a-col :span="12"><a-statistic title="File checked" :value="lsmEnforcerStatus.stats.fileChecked" /></a-col>
                <a-col :span="12"><a-statistic title="File blocked" :value="lsmEnforcerStatus.stats.fileBlocked" /></a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col :xs="24" :lg="9">
            <div style="display: grid; gap: 12px;">
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Block / unblock executable path</div>
                <a-input-group compact>
                  <a-input v-model:value="lsmExecPath" style="width: calc(100% - 160px)" placeholder="/usr/bin/nc" />
                  <a-button danger :disabled="!lsmEnforcerStatus.available" :loading="lsmEnforcerLoading" @click="blockLsmExecPath">Block</a-button>
                  <a-button :disabled="!lsmEnforcerStatus.available" :loading="lsmEnforcerLoading" @click="unblockLsmExecPath()">Unblock</a-button>
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Block / unblock executable basename</div>
                <a-input-group compact>
                  <a-input v-model:value="lsmExecName" style="width: calc(100% - 160px)" placeholder="nc" />
                  <a-button danger :disabled="!lsmEnforcerStatus.available" :loading="lsmEnforcerLoading" @click="blockLsmExecName">Block</a-button>
                  <a-button :disabled="!lsmEnforcerStatus.available" :loading="lsmEnforcerLoading" @click="unblockLsmExecName()">Unblock</a-button>
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Block / unblock file or directory basename</div>
                <a-input-group compact>
                  <a-input v-model:value="lsmFileName" style="width: calc(100% - 160px)" placeholder="id_rsa" />
                  <a-button danger :disabled="!lsmEnforcerStatus.available" :loading="lsmEnforcerLoading" @click="blockLsmFileName">Block</a-button>
                  <a-button :disabled="!lsmEnforcerStatus.available" :loading="lsmEnforcerLoading" @click="unblockLsmFileName()">Unblock</a-button>
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px;">Active BPF LSM blocks</div>
                <a-space wrap>
                  <a-tag v-for="path in lsmEnforcerStatus.blockedExecPaths" :key="`exec-${path}`" color="red" closable @close.prevent="unblockLsmExecPath(path)">
                    exec {{ path }}
                  </a-tag>
                  <a-tag v-for="name in lsmEnforcerStatus.blockedExecNames" :key="`exec-name-${name}`" color="magenta" closable @close.prevent="unblockLsmExecName(name)">
                    exec-name {{ name }}
                  </a-tag>
                  <a-tag v-for="name in lsmEnforcerStatus.blockedFileNames" :key="`file-${name}`" color="volcano" closable @close.prevent="unblockLsmFileName(name)">
                    file {{ name }}
                  </a-tag>
                  <span v-if="!lsmEnforcerStatus.blockedExecPaths.length && !lsmEnforcerStatus.blockedExecNames.length && !lsmEnforcerStatus.blockedFileNames.length" style="color: #999;">
                    No active BPF LSM block entries
                  </span>
                </a-space>
              </div>
            </div>
          </a-col>
        </a-row>
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
