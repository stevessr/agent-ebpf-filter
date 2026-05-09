<script setup lang="ts">
import { onMounted, onUnmounted, computed, ref } from 'vue';
import { useNetworkEnrichment } from '../composables/useNetworkEnrichment';
import type { TCPConnection } from '../composables/useNetworkEnrichment';

const { flows, tcpConns, loading, error, fetchFlows, fetchTCPState } = useNetworkEnrichment(5000);

const filterQuery = ref('');
const showHistoric = ref(false);
const sortKey = ref('lastSeen');
const filterError = ref('');
let refreshTimer: number | null = null;

const filterExamples = ['process:curl', 'dport:443', 'sni:github.com', 'state:ESTABLISHED', 'risk:0.7'];

const validateFilter = (query: string) => {
  const allowedPrefixes = new Set([
    'port', 'dport', 'sport', 'src', 'dst', 'process', 'comm', 'pid', 'agent', 'task',
    'tool', 'sni', 'host', 'domain', 'service', 'app', 'state', 'proto', 'transport',
    'scope', 'risk',
  ]);
  const invalid = query
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .filter((token) => token.includes(':') && !allowedPrefixes.has(token.split(':', 1)[0].toLowerCase()));
  if (invalid.length) {
    return `未知过滤前缀: ${invalid.join(', ')}`;
  }
  return '';
};

const flowParams = computed(() => {
  const params: Record<string, string> = {
    showHistoric: String(showHistoric.value),
    sort: sortKey.value,
    limit: '100',
  };
  const filter = filterQuery.value.trim();
  if (filter) params.filter = filter;
  return params;
});

const refreshNetworkState = async () => {
  filterError.value = validateFilter(filterQuery.value);
  if (!filterError.value) {
    await fetchFlows(flowParams.value);
  }
  await fetchTCPState();
};

const applyFilterExample = (example: string) => {
  const tokens = filterQuery.value.trim().split(/\s+/).filter(Boolean);
  if (!tokens.includes(example)) {
    tokens.push(example);
  }
  filterQuery.value = tokens.join(' ');
  void refreshNetworkState();
};

onMounted(() => {
  void refreshNetworkState();
  refreshTimer = window.setInterval(refreshNetworkState, 5000);
});
onUnmounted(() => {
  if (refreshTimer !== null) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
});

const columns = [
  { title: '目标', dataIndex: 'dstIp', key: 'dst', width: 200 },
  { title: '端口', dataIndex: 'dstPort', key: 'port', width: 70 },
  { title: '协议', dataIndex: 'appProtocol', key: 'app', width: 110 },
  { title: '域名/DPI', dataIndex: 'dstDomain', key: 'domain', width: 220 },
  { title: '作用域', dataIndex: 'ipScope', key: 'scope', width: 90 },
  { title: '进程', dataIndex: 'comm', key: 'comm', width: 120 },
  { title: '出站', dataIndex: 'bytesOut', key: 'out', width: 90 },
  { title: '速率', dataIndex: 'currentBpsOut', key: 'rate', width: 90 },
  { title: '状态', dataIndex: 'staleLevel', key: 'stale', width: 90 },
  { title: '风险', dataIndex: 'riskScore', key: 'risk', width: 70 },
];

const flowData = computed(() =>
  flows.value.map((f) => ({
    ...f,
    comm: f.processComms[0] || '-',
    key: f.flowId || `${f.srcIp}:${f.srcPort}->${f.dstIp}:${f.dstPort}`,
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

const formatRate = (bytesPerSecond?: number) => `${formatBytes(bytesPerSecond || 0)}/s`;

const protocolColor = (protocol?: string) => {
  switch ((protocol || '').toUpperCase()) {
    case 'HTTP': return 'blue';
    case 'TLS':
    case 'HTTPS/TLS': return 'geekblue';
    case 'DNS':
    case 'MDNS': return 'purple';
    case 'SSH': return 'volcano';
    case 'QUIC': return 'cyan';
    default: return 'default';
  }
};

const staleColor = (level?: string) => {
  switch (level) {
    case 'active': return 'green';
    case 'warning': return 'gold';
    case 'critical': return 'red';
    case 'historic': return 'default';
    default: return 'default';
  }
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
          <div class="flow-filter-bar">
            <a-space wrap>
              <a-input-search
                v-model:value="filterQuery"
                placeholder="process:curl dport:443 sni:github.com state:ESTABLISHED"
                allow-clear
                size="small"
                style="width: 420px"
                @search="refreshNetworkState"
                @press-enter="refreshNetworkState"
              />
              <a-select v-model:value="sortKey" size="small" style="width: 140px" @change="refreshNetworkState">
                <a-select-option value="lastSeen">最近更新</a-select-option>
                <a-select-option value="risk">风险优先</a-select-option>
                <a-select-option value="bandwidth">流量优先</a-select-option>
                <a-select-option value="+dst">目标排序</a-select-option>
              </a-select>
              <a-switch
                v-model:checked="showHistoric"
                size="small"
                checked-children="Historic"
                un-checked-children="Active"
                @change="refreshNetworkState"
              />
              <a-button size="small" @click="refreshNetworkState">刷新</a-button>
            </a-space>
            <div class="flow-filter-examples">
              <span>快速过滤:</span>
              <a-tag
                v-for="example in filterExamples"
                :key="example"
                class="filter-example"
                @click="applyFilterExample(example)"
              >
                {{ example }}
              </a-tag>
            </div>
            <a-alert v-if="filterError" type="warning" show-icon :message="filterError" style="margin-top: 8px;" />
          </div>
          <a-table
            :columns="columns"
            :data-source="flowData"
            :pagination="{ pageSize: 20, size: 'small' }"
            size="small"
            row-key="key"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'dst'">
                <div style="display: flex; flex-direction: column; gap: 2px;">
                  <span style="font-family: monospace; font-size: 12px">{{ record.dstIp }}</span>
                  <span style="font-family: monospace; font-size: 11px; color: #94a3b8;">{{ record.srcIp }}:{{ record.srcPort }}</span>
                </div>
              </template>
              <template v-else-if="column.key === 'app'">
                <a-space :size="4" wrap>
                  <a-tag v-if="record.appProtocol" :color="protocolColor(record.appProtocol)" size="small">{{ record.appProtocol }}</a-tag>
                  <a-tag v-if="record.dstService" color="blue" size="small">{{ record.dstService }}</a-tag>
                </a-space>
              </template>
              <template v-else-if="column.key === 'domain'">
                <div style="display: flex; flex-direction: column; gap: 4px;">
                  <span v-if="record.dstDomain" style="color: #1890ff">{{ record.dstDomain }}</span>
                  <a-space :size="4" wrap>
                    <a-tag v-if="record.sni" color="geekblue" size="small">SNI {{ record.sni }}</a-tag>
                    <a-tag v-if="record.httpHost" color="cyan" size="small">
                      {{ record.httpMethod || 'HTTP' }} {{ record.httpHost }}
                    </a-tag>
                    <a-tag v-if="record.dnsName && record.dnsName !== record.dstDomain" color="purple" size="small">DNS {{ record.dnsName }}</a-tag>
                    <a-tag v-if="record.tlsAlpn" color="blue" size="small">ALPN {{ record.tlsAlpn }}</a-tag>
                  </a-space>
                </div>
              </template>
              <template v-else-if="column.key === 'scope'">
                <a-tag :color="record.ipScope === 'Public' ? 'orange' : record.ipScope === 'Private' ? 'green' : 'default'" size="small">
                  {{ record.ipScope }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'out'">
                {{ formatBytes(record.bytesOut) }}
              </template>
              <template v-else-if="column.key === 'rate'">
                <span style="font-family: monospace; font-size: 12px;">↑{{ formatRate(record.currentBpsOut) }}</span>
              </template>
              <template v-else-if="column.key === 'stale'">
                <a-tag :color="staleColor(record.staleLevel)" size="small">{{ record.staleLevel || 'active' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'risk'">
                <a-tooltip :title="(record.riskReasons || []).join('; ')">
                  <a-tag :color="riskColor(record.riskScore)" size="small">
                    {{ record.riskLevel || 'risk' }} {{ (record.riskScore * 100).toFixed(0) }}%
                  </a-tag>
                </a-tooltip>
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

.flow-filter-bar {
  margin-bottom: 12px;
}

.flow-filter-examples {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 8px;
  font-size: 12px;
  color: #64748b;
}

.filter-example {
  cursor: pointer;
}
</style>
