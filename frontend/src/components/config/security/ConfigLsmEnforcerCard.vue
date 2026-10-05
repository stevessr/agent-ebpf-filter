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
<!-- 操作系统级 BPF LSM 拦截 -->
    <a-col :span="24">
      <a-card title="操作系统级 BPF LSM 文件 / 执行拦截" size="small">
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
                  ? "BPF LSM 已启用"
                  : "未启用"
              }}
            </a-tag>
            <a-button
              size="small"
              :loading="lsmEnforcerLoading"
              @click="fetchLsmEnforcerStatus"
            >
              <ReloadOutlined /> 刷新
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
              <a-descriptions-item label="LSM 钩子">
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
              <a-descriptions-item label="eBPF 映射">
                <a-space wrap>
                  <a-tag
                    :color="
                      lsmEnforcerStatus.maps.execPathBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >执行路径</a-tag
                  >
                  <a-tag
                    :color="
                      lsmEnforcerStatus.maps.execNameBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >可执行文件名</a-tag
                  >
                  <a-tag
                    :color="
                      lsmEnforcerStatus.maps.fileNameBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >文件名</a-tag
                  >
                  <a-tag
                    :color="lsmEnforcerStatus.maps.stats ? 'green' : 'default'"
                    >统计</a-tag
                  >
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="固定链接">
                <span
                  v-if="!lsmEnforcerStatus.linkPins.length"
                  style="color: #6b7280"
                  >由进程持有或不可用</span
                >
                <div v-for="pin in lsmEnforcerStatus.linkPins" :key="pin">
                  <code>{{ pin }}</code>
                </div>
              </a-descriptions-item>
              <a-descriptions-item label="错误">
                <span
                  v-if="
                    !lsmEnforcerStatus.error && !lsmEnforcerStatus.statsError
                  "
                  style="color: #52c41a"
                  >正常</span
                >
                <span v-else style="color: #cf1322">{{
                  lsmEnforcerStatus.error || lsmEnforcerStatus.statsError
                }}</span>
              </a-descriptions-item>
            </a-descriptions>
          </a-col>

          <a-col :xs="24" :lg="6">
            <a-card size="small" title="LSM 决策计数">
              <a-row :gutter="[8, 8]">
                <a-col :span="12"
                  ><a-statistic
                    title="执行检查"
                    :value="lsmEnforcerStatus.stats.execChecked"
                /></a-col>
                <a-col :span="12"
                  ><a-statistic
                    title="执行阻断"
                    :value="lsmEnforcerStatus.stats.execBlocked"
                /></a-col>
                <a-col :span="12"
                  ><a-statistic
                    title="文件检查"
                    :value="lsmEnforcerStatus.stats.fileChecked"
                /></a-col>
                <a-col :span="12"
                  ><a-statistic
                    title="文件阻断"
                    :value="lsmEnforcerStatus.stats.fileBlocked"
                /></a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col :xs="24" :lg="9">
            <div style="display: grid; gap: 12px">
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  阻断 / 放行可执行文件路径
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
                    >阻断</a-button
                  >
                  <a-button
                    :disabled="!lsmEnforcerStatus.available"
                    :loading="lsmEnforcerLoading"
                    @click="unblockLsmExecPath()"
                    >放行</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  阻断 / 放行可执行文件名
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
                  阻断 / 放行文件或目录名
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
                  生效中的 BPF LSM 拦截
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
                    当前没有生效的 BPF LSM 拦截项
                  </span>
                </a-space>
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>
</template>
