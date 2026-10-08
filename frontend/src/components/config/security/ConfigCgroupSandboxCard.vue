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
<!-- 操作系统级 cgroup 网络拦截 -->
    <a-col :span="24">
      <a-card title="操作系统级 cgroup 网络拦截" size="small">
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
                  ? "内核拦截已启用"
                  : "未启用"
              }}
            </a-tag>
            <a-button
              size="small"
              :loading="cgroupSandboxLoading"
              @click="fetchCgroupSandboxStatus"
            >
              <ReloadOutlined /> 刷新
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
              <a-descriptions-item label="挂载路径">
                <code>{{
                  cgroupSandboxStatus.cgroupPath || "未挂载"
                }}</code>
              </a-descriptions-item>
              <a-descriptions-item label="eBPF 映射">
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
                    >统计</a-tag
                  >
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="固定链接">
                <span
                  v-if="!cgroupSandboxStatus.linkPins.length"
                  style="color: #6b7280"
                  >由进程持有或不可用</span
                >
                <div v-for="pin in cgroupSandboxStatus.linkPins" :key="pin">
                  <code>{{ pin }}</code>
                </div>
              </a-descriptions-item>
              <a-descriptions-item label="生效中的拦截">
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
                    当前没有生效的 cgroup/connect 或 sendmsg 拦截
                  </span>
                </a-space>
              </a-descriptions-item>
              <a-descriptions-item label="错误">
                <span
                  v-if="
                    !cgroupSandboxStatus.error &&
                    !cgroupSandboxStatus.statsError
                  "
                  style="color: #52c41a"
                  >正常</span
                >
                <span v-else style="color: #cf1322">{{
                  cgroupSandboxStatus.error || cgroupSandboxStatus.statsError
                }}</span>
              </a-descriptions-item>
            </a-descriptions>
          </a-col>

          <a-col :xs="24" :lg="6">
            <a-card size="small" title="内核决策计数">
              <a-row :gutter="[8, 8]">
                <a-col :span="8">
                  <a-statistic
                    title="已检查"
                    :value="cgroupSandboxStatus.stats.checked"
                  />
                </a-col>
                <a-col :span="8">
                  <a-statistic
                    title="已阻断"
                    :value="cgroupSandboxStatus.stats.blocked"
                  />
                </a-col>
                <a-col :span="8">
                  <a-statistic
                    title="已允许"
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
                  阻断 / 放行 cgroup 出站
                </div>
                <a-input-group compact>
                  <a-input
                    v-model:value="cgroupTargetID"
                    style="width: calc(100% - 160px)"
                    placeholder="来自事件的 cgroup ID"
                  />
                  <a-button
                    danger
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="blockCgroupID"
                    >阻断</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupID"
                    >放行</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  阻断 / 放行 PID 所属 cgroup
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
                    >阻断</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupPID"
                    >放行</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  全局阻断 / 放行 IP
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
                    >阻断</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupIP"
                    >放行</a-button
                  >
                </a-input-group>
              </div>
              <div>
                <div style="font-weight: 600; margin-bottom: 6px">
                  全局阻断 / 放行目标端口
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
                    >阻断</a-button
                  >
                  <a-button
                    :disabled="!cgroupSandboxStatus.available"
                    :loading="cgroupSandboxLoading"
                    @click="unblockCgroupPort"
                    >放行</a-button
                  >
                </a-input-group>
              </div>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>
</template>
