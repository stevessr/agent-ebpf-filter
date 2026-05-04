<script setup lang="ts">
import {
  PlusOutlined, SettingOutlined, EditOutlined, CopyOutlined,
  DeleteOutlined, ReloadOutlined,
} from '@ant-design/icons-vue';
import { useLaunchEnv } from '../composables/useLaunchEnv';

const {
  profiles, activeProfileId, activeProfile,
  newLaunchEnvKey, newLaunchEnvValue,
  launchEnvEntries, launchEnvEntriesCount, launchEnvColumns,
  launchEnvPreview, launchEnvScope,
  addLaunchEnvEntry, removeLaunchEnvEntry, clearDisabledLaunchEnvEntries,
  addNewProfile, copyProfile, deleteProfile, openRenameModal,
  detectedLaunchEnvEntries, detectedLaunchEnvSearch, detectedLaunchEnvLoading, detectedLaunchEnvError,
  filteredDetectedLaunchEnvEntries, detectedLaunchEnvColumns,
  refreshDetectedLaunchEnvEntries, importAllDetectedLaunchEnvEntries,
  isLaunchEnvImported, importDetectedLaunchEnvEntry,
  profileRenameModalOpen, profileRenameValue, applyRename,
} = useLaunchEnv();
</script>

<template>
  <a-tab-pane key="launch-env" tab="Launch Env">
    <a-row :gutter="[16, 16]">
      <a-col :span="24">
        <a-card title="Profile Management" :bordered="false" style="margin-bottom: 16px;">
          <template #extra>
            <a-button type="primary" size="small" @click="addNewProfile"><template #icon><PlusOutlined /></template>New Profile</a-button>
          </template>
          <a-space wrap :size="12">
            <template v-for="profile in profiles" :key="profile.id">
              <a-card-grid
                :style="{
                  width: '280px', padding: '12px', textAlign: 'left', cursor: 'pointer',
                  border: activeProfileId === profile.id ? '2px solid #1890ff' : '1px solid #f0f0f0',
                  boxShadow: activeProfileId === profile.id ? '0 0 8px rgba(24,144,255,0.2)' : 'none',
                  borderRadius: '4px',
                  background: activeProfileId === profile.id ? '#e6f7ff' : '#fff'
                }"
                @click="activeProfileId = profile.id"
              >
                <div style="display: flex; justify-content: space-between; align-items: flex-start;">
                  <div style="flex: 1; min-width: 0;">
                    <div style="font-weight: 600; margin-bottom: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" :title="profile.name">{{ profile.name }}</div>
                    <div style="font-size: 12px; color: #666;">{{ profile.entries.length }} variables</div>
                  </div>
                  <a-dropdown :trigger="['click']" @click.stop>
                    <SettingOutlined style="cursor: pointer; color: #1890ff;" />
                    <template #overlay>
                      <a-menu>
                        <a-menu-item key="rename" @click="openRenameModal(profile)"><template #icon><EditOutlined /></template>Rename</a-menu-item>
                        <a-menu-item key="copy" @click="copyProfile(profile)"><template #icon><CopyOutlined /></template>Duplicate</a-menu-item>
                        <a-menu-divider />
                        <a-menu-item key="delete" danger @click="deleteProfile(profile.id)"><template #icon><DeleteOutlined /></template>Delete</a-menu-item>
                      </a-menu>
                    </template>
                  </a-dropdown>
                </div>
              </a-card-grid>
            </template>
          </a-space>
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="11">
        <a-card :title="`Variables in: ${activeProfile.name}`" :bordered="false">
          <template #extra>
            <a-space :size="8">
              <a-tag color="green">{{ launchEnvEntriesCount }} active</a-tag>
              <a-button size="small" @click="clearDisabledLaunchEnvEntries">Clear disabled</a-button>
            </a-space>
          </template>
          <a-alert type="info" show-icon style="margin-bottom: 16px;"
            message="These key/value pairs are injected into every launch action from Executor."
            description="Remote Executor, Shell Manager, tmux coding launches, and script runners all receive the enabled variables." />
          <a-form layout="vertical">
            <a-row :gutter="12">
              <a-col :xs="24" :md="8">
                <a-form-item label="Key"><a-input v-model:value="newLaunchEnvKey" placeholder="FOO_BAR" @pressEnter="addLaunchEnvEntry" /></a-form-item>
              </a-col>
              <a-col :xs="24" :md="12">
                <a-form-item label="Value"><a-input v-model:value="newLaunchEnvValue" placeholder="value" @pressEnter="addLaunchEnvEntry" /></a-form-item>
              </a-col>
              <a-col :xs="24" :md="4" style="display: flex; align-items: flex-end;">
                <a-button type="primary" block @click="addLaunchEnvEntry"><template #icon><PlusOutlined /></template>Add</a-button>
              </a-col>
            </a-row>
          </a-form>
          <a-table :data-source="launchEnvEntries" :columns="launchEnvColumns" :pagination="false" size="small" row-key="id">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'enabled'"><a-switch v-model:checked="record.enabled" /></template>
              <template v-else-if="column.key === 'key'"><a-input v-model:value="record.key" placeholder="ENV_NAME" allow-clear /></template>
              <template v-else-if="column.key === 'value'"><a-input v-model:value="record.value" placeholder="value" allow-clear /></template>
              <template v-else-if="column.key === 'action'">
                <a-button size="small" danger @click="removeLaunchEnvEntry(record.id)"><template #icon><DeleteOutlined /></template>Delete</a-button>
              </template>
            </template>
          </a-table>
        </a-card>
      </a-col>

      <a-col :xs="24" :xl="13">
        <a-card title="Launch env preview" :bordered="false">
          <template #extra><a-space :size="8"><SettingOutlined /><span>Local browser persistence</span></a-space></template>
          <a-space direction="vertical" :size="12" style="width: 100%;">
            <a-descriptions bordered size="small" :column="1">
              <a-descriptions-item label="Active variables"><span>{{ launchEnvEntriesCount }}</span></a-descriptions-item>
              <a-descriptions-item label="Scope"><span>{{ launchEnvScope }}</span></a-descriptions-item>
              <a-descriptions-item label="Preview"><span>{{ launchEnvPreview }}</span></a-descriptions-item>
            </a-descriptions>
            <a-alert type="warning" show-icon message="Environment variable names should use the usual shell style: FOO or FOO_BAR." />
            <a-alert type="info" show-icon message="Because this is stored in your browser, each workstation/browser can keep its own launch env profile." />
          </a-space>
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="[16, 16]" style="margin-top: 16px;">
      <a-col :span="24">
        <a-card title="Detected environment from backend" :bordered="false">
          <template #extra>
            <a-space :size="8">
              <a-tag color="blue">{{ filteredDetectedLaunchEnvEntries.length }} visible</a-tag>
              <a-tag color="default">{{ detectedLaunchEnvEntries.length }} detected</a-tag>
              <a-button size="small" :loading="detectedLaunchEnvLoading" @click="refreshDetectedLaunchEnvEntries"><template #icon><ReloadOutlined /></template>Refresh</a-button>
              <a-button size="small" type="primary" :disabled="filteredDetectedLaunchEnvEntries.length === 0" @click="importAllDetectedLaunchEnvEntries">Import visible</a-button>
            </a-space>
          </template>
          <a-alert type="info" show-icon style="margin-bottom: 16px;" message="This list is read from the backend runtime environment. Backend configuration vars such as AGENT_*, GIN_MODE, DISABLE_AUTH, SUDO_*, and PKEXEC_UID are hidden." />
          <a-alert v-if="detectedLaunchEnvError" type="warning" show-icon style="margin-bottom: 16px;" :message="detectedLaunchEnvError" />
          <a-input-search v-model:value="detectedLaunchEnvSearch" placeholder="Filter detected env by key or value" enter-button="Search" style="margin-bottom: 16px;" />
          <a-table :data-source="filteredDetectedLaunchEnvEntries" :columns="detectedLaunchEnvColumns" :loading="detectedLaunchEnvLoading" :pagination="false" row-key="key" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'key'"><code>{{ record.key }}</code></template>
              <template v-else-if="column.key === 'value'"><span class="executor-env__value" :title="record.value">{{ record.value || '—' }}</span></template>
              <template v-else-if="column.key === 'action'">
                <a-space>
                  <a-tag v-if="isLaunchEnvImported(record.key)" color="green">Imported</a-tag>
                  <a-button size="small" @click="importDetectedLaunchEnvEntry(record)">{{ isLaunchEnvImported(record.key) ? 'Update' : 'Use' }}</a-button>
                </a-space>
              </template>
            </template>
            <template #emptyText><a-empty description="No backend runtime env vars detected" /></template>
          </a-table>
        </a-card>
      </a-col>
    </a-row>
  </a-tab-pane>

  <a-modal v-model:open="profileRenameModalOpen" title="Rename Profile" @ok="applyRename">
    <a-form layout="vertical">
      <a-form-item label="Profile Name">
        <a-input v-model:value="profileRenameValue" placeholder="Enter profile name" @pressEnter="applyRename" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<style scoped>
.executor-env__value {
  display: inline-block;
  max-width: 100%;
  word-break: break-all;
  white-space: normal;
  color: #333;
}
</style>
