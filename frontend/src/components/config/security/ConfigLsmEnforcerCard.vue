<script setup lang="ts">
import { ReloadOutlined } from "@ant-design/icons-vue";
import type { useConfigSecurity } from "../../../composables/config/useConfigSecurity";

const props = defineProps<{
  security: ReturnType<typeof useConfigSecurity>;
}>();

const {
  lsmEnforcerStatus,
  lsmEnforcerLoading,
  lsmExecPath,
  lsmExecName,
  lsmFileName,
  fetchLsmEnforcerStatus,
  blockLsmExecPath,
  unblockLsmExecPath,
  blockLsmExecName,
  unblockLsmExecName,
  blockLsmFileName,
  unblockLsmFileName,
} = props.security;
</script>

<template>
<!-- OS-level BPF LSM interception -->
    <a-col :span="24">
      <a-card title="OS-Level BPF LSM File / Exec Interception" size="small">
        <template #extra>
          <a-space>
            <a-tag
              :color="
                lsmEnforcerStatus.available && lsmEnforcerStatus.attached
                  ? 'green'
                  : 'red'
              "
            >
              {{
                lsmEnforcerStatus.available && lsmEnforcerStatus.attached
                  ? "BPF LSM active"
                  : "not active"
              }}
            </a-tag>
            <a-button
              size="small"
              :loading="lsmEnforcerLoading"
              @click="fetchLsmEnforcerStatus"
            >
              <ReloadOutlined /> Refresh
            </a-button>
          </a-space>
        </template>
        <a-alert
          type="warning"
          show-icon
          style="margin-bottom: 16px"
          message="这里写入的是 BPF LSM map：bprm_check_security 可按执行路径或可执行文件 basename 拒绝 exec；file_open、file_permission、mmap_file、file_mprotect、inode_setattr、inode_create、inode_link、inode_symlink、inode_unlink、inode_mkdir、inode_rmdir、inode_mknod、inode_rename 可按文件或目录 basename 拒绝打开、既有 fd 读写、mmap、mprotect、setattr、创建、link、symlink、删除、mkdir、rmdir、mknod 与 rename。该路径在内核 LSM 决策点返回 EACCES。"
        />

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="9">
            <a-descriptions size="small" bordered :column="1">
              <a-descriptions-item label="Hooks">
                <a-space wrap>
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >bprm_check_security</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >file_open</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >file_permission</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >mmap_file</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >file_mprotect</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_setattr</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_create</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_link</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_symlink</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_unlink</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_mkdir</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_rmdir</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_mknod</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.attached ? 'green' : 'default'"
                    >inode_rename</a-tag
                  >
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Maps">
                <a-space wrap>
                  <a-tag
                    :color="
                      lsmEnforcerStatus.maps.execPathBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >exec paths</a-tag
                  >
                  <a-tag
                    :color="
                      lsmEnforcerStatus.maps.execNameBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >exec names</a-tag
                  >
                  <a-tag
                    :color="
                      lsmEnforcerStatus.maps.fileNameBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >file names</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.maps.stats ? 'green' : 'default'"
                    >stats</a-tag
                  >
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Pinned links">
                <span
                  v-if="!lsmEnforcerStatus.linkPins.length"
                  style="color: #6b7280"
                  >process-held or unavailable</span
                >
                <div v-for="pin in lsmEnforcerStatus.linkPins" :key="pin">
                  <code>{{ pin }}</code>
                </div>
              </a-descriptions-item>
              <a-descriptions-item label="Error">
                <span
                  v-if="
                    !lsmEnforcerStatus.error && !lsmEnforcerStatus.statsError
                  "
                  style="color: #52c41a"
                  >OK</span
                >
                <span v-else style="color: #cf1322">{{
                  lsmEnforcerStatus.error || lsmEnforcerStatus.statsError
                }}</span>
              </a-descriptions-item>
            </a-descriptions>
          </a-col>

          <a-col :xs="24" :lg="6">
            <a-card size="small" title="LSM decision counters">
              <a-row :gutter="[8, 8]">
                <a-col :span="12"
                  ><a-statistic
                    title="Exec checked"
                    :value="lsmEnforcerStatus.stats.execChecked"
                /></a-col>
                <a-col :span="12"
                  ><a-statistic
                    title="Exec blocked"
                    :value="lsmEnforcerStatus.stats.execBlocked"
                /></a-col>
                <a-col :span="12"
                  ><a-statistic
                    title="File checked"
                    :value="lsmEnforcerStatus.stats.fileChecked"
                /></a-col>
                <a-col :span="12"
                  ><a-statistic
                    title="File blocked"
                    :value="lsmEnforcerStatus.stats.fileBlocked"
                /></a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col :xs="24" :lg="9">
            <div style="display: grid; gap: 12px">
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Block / unblock executable path
                </div>
                <a-input-group compact>
                  <a-input
                    v-model:value="lsmExecPath"
                    style="width: calc(100% - 160px)"
                    placeholder="/usr/bin/nc"
                  />
                  <a-button
                    danger
                    :disabled="!lsmEnforcerStatus.available"
                    :loading="lsmEnforcerLoading"
                    @click="blockLsmExecPath"
                    >Block</a-button
                  >
                  <a-button
                    :disabled="!lsmEnforcerStatus.available"
                    :loading="lsmEnforcerLoading"
                    @click="unblockLsmExecPath()"
                    >Unblock</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Block / unblock executable basename
                </div>
                <a-input-group compact>
                  <a-input
                    v-model:value="lsmExecName"
                    style="width: calc(100% - 160px)"
                    placeholder="nc"
                  />
                  <a-button
                    danger
                    :disabled="!lsmEnforcerStatus.available"
                    :loading="lsmEnforcerLoading"
                    @click="blockLsmExecName"
                    >Block</a-button
                  >
                  <a-button
                    :disabled="!lsmEnforcerStatus.available"
                    :loading="lsmEnforcerLoading"
                    @click="unblockLsmExecName()"
                    >Unblock</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Block / unblock file or directory basename
                </div>
                <a-input-group compact>
                  <a-input
                    v-model:value="lsmFileName"
                    style="width: calc(100% - 160px)"
                    placeholder="id_rsa"
                  />
                  <a-button
                    danger
                    :disabled="!lsmEnforcerStatus.available"
                    :loading="lsmEnforcerLoading"
                    @click="blockLsmFileName"
                    >Block</a-button
                  >
                  <a-button
                    :disabled="!lsmEnforcerStatus.available"
                    :loading="lsmEnforcerLoading"
                    @click="unblockLsmFileName()"
                    >Unblock</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Active BPF LSM blocks
                </div>
                <a-space wrap>
                  <a-tag
                    v-for="path in lsmEnforcerStatus.blockedExecPaths"
                    :key="`exec-${path}`"
                    color="red"
                    closable
                    @close.prevent="unblockLsmExecPath(path)"
                  >
                    exec {{ path }}
                  </a-tag>
                  <a-tag
                    v-for="name in lsmEnforcerStatus.blockedExecNames"
                    :key="`exec-name-${name}`"
                    color="magenta"
                    closable
                    @close.prevent="unblockLsmExecName(name)"
                  >
                    exec-name {{ name }}
                  </a-tag>
                  <a-tag
                    v-for="name in lsmEnforcerStatus.blockedFileNames"
                    :key="`file-${name}`"
                    color="volcano"
                    closable
                    @close.prevent="unblockLsmFileName(name)"
                  >
                    file {{ name }}
                  </a-tag>
                  <span
                    v-if="
                      !lsmEnforcerStatus.blockedExecPaths.length &&
                      !lsmEnforcerStatus.blockedExecNames.length &&
                      !lsmEnforcerStatus.blockedFileNames.length
                    "
                    style="color: #6b7280"
                  >
                    No active BPF LSM block entries
                  </span>
                </a-space>
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>
</template>
