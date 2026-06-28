<script setup lang="ts">
import { PlayCircleOutlined, ClearOutlined } from "@ant-design/icons-vue";

interface RecentLaunch {
  program: string;
  user: string;
  cwd: string;
  args: string;
}

interface SysUser {
  username: string;
  uid: number;
  home: string;
  shell: string;
}

const props = defineProps<{
  open: boolean;
  launchPath: string;
  launchUser: string;
  launchCwd: string;
  launchArgs: string;
  launching: boolean;
  launchError: string;
  sysUsers: SysUser[];
  usersLoading: boolean;
  recentLaunches: RecentLaunch[];
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  "update:launchPath": [value: string];
  "update:launchUser": [value: string];
  "update:launchCwd": [value: string];
  "update:launchArgs": [value: string];
  launch: [];
  browse: [target: "program" | "cwd"];
  applyRecent: [value: RecentLaunch];
}>();

const onLaunch = () => emit("launch");
</script>

<template>
  <a-modal
    :open="open"
    title="Launch & Observe"
    :footer="null"
    width="620px"
    :destroy-on-close="false"
    @update:open="emit('update:open', $event)"
  >
    <div class="launch-modal-body">
      <!-- Program path -->
      <div class="launch-field">
        <span class="launch-label">Program</span>
        <div class="launch-row">
          <a-input
            :value="launchPath"
            placeholder="/usr/bin/python3"
            size="small"
            spellcheck="false"
            style="flex: 1"
            @update:value="emit('update:launchPath', $event)"
          />
          <a-button size="small" @click="emit('browse', 'program')">
            Browse
          </a-button>
        </div>
      </div>

      <!-- User -->
      <div class="launch-field">
        <span class="launch-label">User</span>
        <a-select
          :value="launchUser"
          size="small"
          show-search
          :filter-option="
            (input: string, option: any) =>
              option.value.toLowerCase().includes(input.toLowerCase())
          "
          :loading="usersLoading"
          placeholder="Select user..."
          style="width: 100%"
          @update:value="emit('update:launchUser', $event)"
        >
          <a-select-option
            v-for="u in sysUsers"
            :key="u.username"
            :value="u.username"
          >
            {{ u.username }}
            <span class="user-uid">({{ u.uid }})</span>
          </a-select-option>
        </a-select>
      </div>

      <!-- Working Directory -->
      <div class="launch-field">
        <span class="launch-label">Working Directory</span>
        <div class="launch-row">
          <a-input
            :value="launchCwd"
            placeholder="/home/..."
            size="small"
            spellcheck="false"
            style="flex: 1"
            @update:value="emit('update:launchCwd', $event)"
          />
          <a-button size="small" @click="emit('browse', 'cwd')">
            Browse
          </a-button>
        </div>
      </div>

      <!-- Arguments -->
      <div class="launch-field">
        <span class="launch-label">Arguments</span>
        <a-input
          :value="launchArgs"
          placeholder="--verbose --output /tmp/out"
          size="small"
          @update:value="emit('update:launchArgs', $event)"
        />
      </div>

      <!-- Launch button + error -->
      <div class="launch-actions">
        <a-button
          type="primary"
          :loading="launching"
          @click="onLaunch"
        >
          <template #icon><PlayCircleOutlined /></template>
          Launch & Observe
        </a-button>
        <span v-if="launchError" class="launch-error">{{ launchError }}</span>
      </div>

      <!-- Recent launches -->
      <div v-if="recentLaunches.length > 0" class="recent-section">
        <div class="recent-title">
          <ClearOutlined style="margin-right: 4px" />
          Recent
        </div>
        <div class="recent-chips">
          <a-tag
            v-for="(rl, i) in recentLaunches"
            :key="i"
            class="recent-chip"
            color="default"
            @click="emit('applyRecent', rl)"
          >
            <span class="recent-prog">{{ rl.program.split('/').pop() || rl.program }}</span>
            <span class="recent-args" v-if="rl.args">{{ rl.args.slice(0, 40) }}{{ rl.args.length > 40 ? '…' : '' }}</span>
          </a-tag>
        </div>
      </div>
    </div>
  </a-modal>
</template>

<style scoped>
.launch-modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.launch-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.launch-label {
  font-size: 12px;
  color: #4b5563;
  font-weight: 500;
}

.launch-row {
  display: flex;
  gap: 6px;
  align-items: center;
}

.user-uid {
  font-size: 11px;
  color: #6b7280;
  margin-left: 4px;
}

.launch-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-top: 4px;
}

.launch-error {
  color: #ff4d4f;
  font-size: 12px;
}

.recent-section {
  padding-top: 10px;
  border-top: 1px dashed #e8e8e8;
}

.recent-title {
  font-size: 12px;
  color: #6b7280;
  font-weight: 500;
  margin-bottom: 6px;
}

.recent-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.recent-chip {
  cursor: pointer;
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: all 0.15s;
}

.recent-chip:hover {
  border-color: #1677ff;
  color: #1677ff;
}

.recent-prog {
  font-family: ui-monospace, monospace;
  font-weight: 600;
  font-size: 12px;
}

.recent-args {
  font-family: ui-monospace, monospace;
  font-size: 10px;
  color: #6b7280;
  margin-left: 4px;
}
</style>
