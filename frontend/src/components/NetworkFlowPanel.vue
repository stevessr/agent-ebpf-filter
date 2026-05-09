<script setup lang="ts">
import { onMounted, onUnmounted, computed } from 'vue';
import { useNetworkEnrichment } from '../composables/useNetworkEnrichment';
import type { NetworkFlow, TCPConnection } from '../composables/useNetworkEnrichment';

const { flows, tcpConns, loading, error, fetchFlows, fetchTCPState, startAutoRefresh, stopAutoRefresh } = useNetworkEnrichment(5000);

onMounted(() => startAutoRefresh());
onUnmounted(() => stopAutoRefresh());

const columns = [
  { title: '目标', dataIndex: 'dstIp', key: 'dst', width: 180 },
  { title: '端口', dataIndex: 'dstPort', key: 'port', width: 70 },
  { title: '服务', dataIndex: 'dstService', key: 'svc', width: 100 },
  { title: '域名', dataIndex: 'dstDomain', key: 'domain', width: 160 },
  { title: '作用域', dataIndex: 'ipScope', key: 'scope', width: 90 },
  { title: '进程', dataIndex: 'comm', key: 'comm', width: 120 },
  { title: '出站', dataIndex: 'bytesOut', key: 'out', width: 90 },
  { title: '风险', dataIndex: 'riskScore', key: 'risk', width: 70 },
];

const flowData = computed(() =>
  flows.value.map((f) => ({
    ...f,
    comm: f.processComms[0] || '-',
    key: `${f.dstIp}:${f.dstPort}`,
  }))
);

const tcpColumns = [
  { title: '源', dataIndex: 'srcIp', key: 'src', width: 150 },
  { title: '目标', dataIndex: 'dstIp', key: 'dst', width: 150 },
  { title: '端口', dataIndex: 'dstPort', key: 'port', width: 70 },
  { title: '状态', dataIndex: 'state', key: 'state', width: 100 },
  { title: '进程', dataIndex: 'comm', key: 'comm', width: 120 },
];

const stateColor = (state: string) => {
  switch (state) {
    case 'ESTABLISHED': return 'green';
    case 'SYN_SENT': case 'SYN_RECV': return 'orange';
    case 'FIN_WAIT1': case 'FIN_WAIT2': case 'CLOSING': return 'gold';
    case 'TIME_WAIT': case 'CLOSE_WAIT': case 'LAST_ACK': return 'volcano';
    case 'CLOSED': case 'CLOSE': return 'default';
    default: return 'default';
  }
};

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B';
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
};

const riskColor = (score: number) => {
  if (score >= 0.8) return 'red';
  if (score >= 0.6) return 'orange';
  if (score >= 0.3) return 'gold';
  return 'green';
};
</script>

<template>
  <div class="network-flow-panel">
    <a-card title="网络流分析" size="small" :loading="loading">
      <template #extra>
        <a-space>
          <a-tag v-if="flows.length" color="blue">{{ flows.length }} 流</a-tag>
          <a-tag v-if="tcpConns.length" color="green">{{ tcpConns.filter(c => c.state === 'ESTABLISHED').length }} 活跃 TCP</a-tag>
        </a-space>
      </template>

      <a-tabs size="small">
        <a-tab-pane key="flows" tab="聚合流">
          <a-table
            :columns="columns"
            :data-source="flowData"
            :pagination="{ pageSize: 20, size: 'small' }"
            size="small"
            row-key="key"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'dst'">
                <span style="font-family: monospace; font-size: 12px">{{ record.dstIp }}</span>
              </template>
              <template v-else-if="column.key === 'svc'">
                <a-tag v-if="record.dstService" color="blue" size="small">{{ record.dstService }}</a-tag>
              </template>
              <template v-else-if="column.key === 'domain'">
                <span v-if="record.dstDomain" style="color: #1890ff">{{ record.dstDomain }}</span>
              </template>
              <template v-else-if="column.key === 'scope'">
                <a-tag :color="record.ipScope === 'Public' ? 'orange' : record.ipScope === 'Private' ? 'green' : 'default'" size="small">
                  {{ record.ipScope }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'out'">
                {{ formatBytes(record.bytesOut) }}
              </template>
              <template v-else-if="column.key === 'risk'">
                <a-tag :color="riskColor(record.riskScore)" size="small">
                  {{ (record.riskScore * 100).toFixed(0) }}%
                </a-tag>
              </template>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="tcp" tab="TCP 状态">
          <a-table
            :columns="tcpColumns"
            :data-source="tcpConns"
            :pagination="{ pageSize: 20, size: 'small' }"
            size="small"
            row-key="key"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'state'">
                <a-badge :color="stateColor(record.state)" :text="record.state" />
              </template>
              <template v-else-if="column.key === 'src' || column.key === 'dst'">
                <span style="font-family: monospace; font-size: 12px">{{ record[column.key as keyof TCPConnection] }}</span>
              </template>
            </template>
          </a-table>
        </a-tab-pane>
      </a-tabs>

      <div v-if="error" style="color: #ff4d4f; margin-top: 8px">{{ error }}</div>
    </a-card>
  </div>
</template>

<style scoped>
.network-flow-panel {
  margin-top: 16px;
}
</style>
