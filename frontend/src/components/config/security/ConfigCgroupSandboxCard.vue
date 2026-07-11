<script setup lang="ts">
import { ReloadOutlined } from "@ant-design/icons-vue";
import type { useConfigSecurity } from "../../../composables/config/useConfigSecurity";

const props = defineProps<{
  security: ReturnType<typeof useConfigSecurity>;
}>();

const {
  cgroupSandboxStatus,
  cgroupSandboxLoading,
  cgroupTargetID,
  cgroupTargetPID,
  cgroupTargetIP,
  cgroupTargetPort,
  fetchCgroupSandboxStatus,
  blockCgroupID,
  unblockCgroupID,
  blockCgroupPID,
  unblockCgroupPID,
  blockCgroupIP,
  unblockCgroupIP,
  blockCgroupPort,
  unblockCgroupPort,
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
<!-- OS-level cgroup interception -->
    <a-col :span="24">
      <a-card title="OS-Level cgroup Network Interception" size="small">
        <template #extra>
          <a-space>
            <a-tag
              :color="
                cgroupSandboxStatus.available && cgroupSandboxStatus.attached
                  ? 'green'
                  : 'red'
              "
            >
              {{
                cgroupSandboxStatus.available && cgroupSandboxStatus.attached
                  ? "kernel blocking active"
                  : "not active"
              }}
            </a-tag>
            <a-button
              size="small"
              :loading="cgroupSandboxLoading"
              @click="fetchCgroupSandboxStatus"
            >
              <ReloadOutlined /> Refresh
            </a-button>
          </a-space>
        </template>
        <a-alert
          type="warning"
          show-icon
          style="margin-bottom: 16px"
          message="这里写入的是 cgroup/connect4 + connect6 + sendmsg4 + sendmsg6 eBPF map，命中后连接或 UDP sendto/sendmsg 在内核阶段直接失败；支持 TCP/UDP connected sockets 与 UDP sendto/sendmsg 的 cgroup、IPv4/IPv6 目的地址和端口阻断，IPv4 block 也会覆盖 ::ffff:a.b.c.d 形式的 IPv4-mapped IPv6 socket，不同于 wrapper/hook，只覆盖网络出站拦截。"
        />

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :lg="10">
            <a-descriptions size="small" bordered :column="1">
              <a-descriptions-item label="Attach path">
                <code>{{
                  cgroupSandboxStatus.cgroupPath || "not attached"
                }}</code>
              </a-descriptions-item>
              <a-descriptions-item label="Maps">
                <a-space wrap>
                  <a-tag
                    :color="
                      cgroupSandboxStatus.maps.cgroupBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >cgroup</a-tag
                  >
                  <a-tag
                    :color="
                      cgroupSandboxStatus.maps.ipBlocklist ? 'green' : 'default'
                    "
                    >ipv4</a-tag
                  >
                  <a-tag
                    :color="
                      cgroupSandboxStatus.maps.ip6Blocklist
                        ? 'green'
                        : 'default'
                    "
                    >ipv6</a-tag
                  >
                  <a-tag
                    :color="
                      cgroupSandboxStatus.maps.portBlocklist
                        ? 'green'
                        : 'default'
                    "
                    >port</a-tag
                  >
                  <a-tag
                    :color="
                      cgroupSandboxStatus.maps.stats ? 'green' : 'default'
                    "
                    >stats</a-tag
                  >
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Pinned links">
                <span
                  v-if="!cgroupSandboxStatus.linkPins.length"
                  style="color: #6b7280"
                  >process-held or unavailable</span
                >
                <div v-for="pin in cgroupSandboxStatus.linkPins" :key="pin">
                  <code>{{ pin }}</code>
                </div>
              </a-descriptions-item>
              <a-descriptions-item label="Active blocks">
                <a-space wrap>
                  <a-tag
                    v-for="id in cgroupSandboxStatus.blockedCgroups"
                    :key="`cg-${id}`"
                    color="red"
                    closable
                    @close.prevent="unblockCgroupIDFromTag(id)"
                  >
                    cgroup {{ id }}
                  </a-tag>
                  <a-tag
                    v-for="ip in cgroupSandboxStatus.blockedIPs"
                    :key="`ip-${ip}`"
                    color="volcano"
                    closable
                    @close.prevent="unblockCgroupIPFromTag(ip)"
                  >
                    ip {{ ip }}
                  </a-tag>
                  <a-tag
                    v-for="port in cgroupSandboxStatus.blockedPorts"
                    :key="`port-${port}`"
                    color="orange"
                    closable
                    @close.prevent="unblockCgroupPortFromTag(port)"
                  >
                    port {{ port }}
                  </a-tag>
                  <span
                    v-if="
                      !cgroupSandboxStatus.blockedCgroups.length &&
                      !cgroupSandboxStatus.blockedIPs.length &&
                      !cgroupSandboxStatus.blockedPorts.length
                    "
                    style="color: #6b7280"
                  >
                    No active cgroup/connect or sendmsg blocks
                  </span>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="Error">
                <span
                  v-if="
                    !cgroupSandboxStatus.error &&
                    !cgroupSandboxStatus.statsError
                  "
                  style="color: #52c41a"
                  >OK</span
                >
                <span v-else style="color: #cf1322">{{
                  cgroupSandboxStatus.error || cgroupSandboxStatus.statsError
                }}</span>
              </a-descriptions-item>
            </a-descriptions>
          </a-col>

          <a-col :xs="24" :lg="6">
            <a-card size="small" title="Kernel decision counters">
              <a-row :gutter="[8, 8]">
                <a-col :span="8">
                  <a-statistic
                    title="Checked"
                    :value="cgroupSandboxStatus.stats.checked"
                  />
                </a-col>
                <a-col :span="8">
                  <a-statistic
                    title="Blocked"
                    :value="cgroupSandboxStatus.stats.blocked"
                  />
                </a-col>
                <a-col :span="8">
                  <a-statistic
                    title="Allowed"
                    :value="cgroupSandboxStatus.stats.allowed"
                  />
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col :xs="24" :lg="8">
            <div style="display: grid; gap: 12px">
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Block / unblock cgroup outbound
                </div>
                <a-input-group compact>
                  <a-input
                    v-model:value="cgroupTargetID"
                    style="width: calc(100% - 160px)"
                    placeholder="cgroup id from events"
                  />
                  <a-button
                    danger
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="blockCgroupID"
                    >Block</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupID"
                    >Unblock</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Block / unblock PID's cgroup
                </div>
                <a-input-group compact>
                  <a-input-number
                    v-model:value="cgroupTargetPID"
                    style="width: calc(100% - 160px)"
                    :min="1"
                    placeholder="PID"
                  />
                  <a-button
                    danger
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="blockCgroupPID"
                    >Block</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupPID"
                    >Unblock</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Block / unblock IP globally
                </div>
                <a-input-group compact>
                  <a-input
                    v-model:value="cgroupTargetIP"
                    style="width: calc(100% - 160px)"
                    placeholder="1.2.3.4, ::ffff:1.2.3.4, or ::1"
                  />
                  <a-button
                    danger
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="blockCgroupIP"
                    >Block</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupIP"
                    >Unblock</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  Block / unblock destination port globally
                </div>
                <a-input-group compact>
                  <a-input-number
                    v-model:value="cgroupTargetPort"
                    style="width: calc(100% - 160px)"
                    :min="1"
                    :max="65535"
                  />
                  <a-button
                    danger
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="blockCgroupPort"
                    >Block</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupPort"
                    >Unblock</a-button
                  >
                </a-input-group>
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>
</template>
