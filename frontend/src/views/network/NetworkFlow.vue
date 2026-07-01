<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  GlobalOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  DashboardOutlined,
  NodeIndexOutlined,
  WifiOutlined,
  AlertOutlined,
} from "@ant-design/icons-vue";
import type { NetworkFlow } from "../../composables/network/useNetworkEnrichment";
import TrafficGraph from "../../components/network/TrafficGraph.vue";
import { useInterfaceMonitor } from "./useInterfaceMonitor";
import { useFlowFilters } from "./useFlowFilters";

// ── Composables ──────────────────────────────────────────────────────

const monitor = useInterfaceMonitor();
const {
  isConnected,
  wsTimeRange,
  cumRecv,
  cumSent,
  showInterfaceChartModal,
  selectedInterfaceName,
  interfaceChartTimeRange,
  netInterfaces,
  openInterfaceChart,
  interfaceChartOptions,
  interfaceChartSeries,
  formatBytes,
  formatRate,
  VueApexCharts,
} = monitor;

const flows = useFlowFilters();
const {
  flows: flowList,
  tcpConns,
  flowsLoading,
  flowsError,
  totalBytesOut,
  totalBytesIn,
  suspiciousFlows,
  publicFlows,
  establishedConns,
  apiInterfaces,
  apiInterfaceRates,
  dnsMap,
  totalErrors,
  totalDrops,
  filterQuery,
  showHistoric,
  sortKey,
  filterError,
  filterExamples,
  refreshFlows,
  applyFilterExample,
  topProcesses,
  riskSummary,
  flowProtocols,
  protocolColor,
  stateColor,
  staleColor,
  riskColor,
  getTrafficLevelColor,
  getTrafficLevelLabel,
  selectedFlow,
  showFlowDetail,
  openFlowDetail,
  flowColumns,
  flowData,
  tcpColumns,
} = flows;

// ── Traffic source ─────────────────────────────────────────────────
const trafficInterfaces = computed(() => {
  if (netInterfaces.value.length) return netInterfaces.value;
  if (apiInterfaceRates.value.length) return apiInterfaceRates.value;
  return apiInterfaces.value.map((item) => ({
    name: item.name,
    readSpeed: 0,
    writeSpeed: 0,
  }));
});
const displayTotalNetRecv = computed(() =>
  trafficInterfaces.value.reduce((sum, item) => sum + item.readSpeed, 0),
);
const displayTotalNetSent = computed(() =>
  trafficInterfaces.value.reduce((sum, item) => sum + item.writeSpeed, 0),
);

// ── Tab state ──────────────────────────────────────────────────────
const route = useRoute();
const router = useRouter();
const activeTab = ref((route.params.tab as string) || "overview");

watch(activeTab, (tab) => {
  const current = route.params.tab as string;
  if (tab !== current) {
    void router.replace({ name: "NetworkFlow", params: { tab } });
  }
});

watch(
  () => route.params.tab,
  (param) => {
    const tab = (param as string) || "overview";
    if (tab !== activeTab.value) {
      activeTab.value = tab;
    }
  },
);

// ── Graph resize ───────────────────────────────────────────────────
const graphHeight = ref(420);
const isResizing = ref(false);
const startResize = (e: MouseEvent) => {
  isResizing.value = true;
  const startY = e.clientY,
    startH = graphHeight.value;
  const onMove = (me: MouseEvent) => {
    if (isResizing.value)
      graphHeight.value = Math.max(
        200,
        Math.min(1200, startH + me.clientY - startY),
      );
  };
  const onUp = () => {
    isResizing.value = false;
    document.removeEventListener("mousemove", onMove);
    document.removeEventListener("mouseup", onUp);
  };
  document.addEventListener("mousemove", onMove);
  document.addEventListener("mouseup", onUp);
};
</script>

<template>
  <div style="padding: 20px; background: #f0f2f5; min-height: 100%">
    <!-- ── Stat cards (always visible) ──────────────────────────── -->
    <a-row :gutter="[16, 16]" style="margin-bottom: 16px">
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #e6f7ff">
          <div style="display: flex; align-items: center; gap: 12px">
            <ArrowDownOutlined style="font-size: 24px; color: #1890ff" />
            <div>
              <div style="font-size: 12px; color: #666">Download</div>
              <div style="font-size: 22px; font-weight: bold; color: #1890ff">
                {{ formatBytes(displayTotalNetRecv, 1) }}/s
              </div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #f6ffed">
          <div style="display: flex; align-items: center; gap: 12px">
            <ArrowUpOutlined style="font-size: 24px; color: #52c41a" />
            <div>
              <div style="font-size: 12px; color: #666">Upload</div>
              <div style="font-size: 22px; font-weight: bold; color: #52c41a">
                {{ formatBytes(displayTotalNetSent, 1) }}/s
              </div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #fff7e6">
          <div style="display: flex; align-items: center; gap: 12px">
            <GlobalOutlined style="font-size: 24px; color: #fa8c16" />
            <div>
              <div style="font-size: 12px; color: #666">Active Flows</div>
              <div style="font-size: 22px; font-weight: bold; color: #fa8c16">
                {{ flowList.length }}
              </div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #f9f0ff">
          <div style="display: flex; align-items: center; gap: 12px">
            <AlertOutlined style="font-size: 24px; color: #722ed1" />
            <div>
              <div style="font-size: 12px; color: #666">Suspicious</div>
              <div style="font-size: 22px; font-weight: bold; color: #722ed1">
                {{ suspiciousFlows().length }}
              </div>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <!-- ── Connectivity bar ────────────────────────────────────── -->
    <a-row :gutter="[16, 16]" style="margin-bottom: 16px">
      <a-col :span="24">
        <a-card size="small" :bordered="false">
          <div
            style="
              display: flex;
              align-items: center;
              gap: 12px;
              flex-wrap: wrap;
              justify-content: space-between;
            "
          >
            <div style="display: flex; align-items: center; gap: 12px">
              <a-badge
                :status="isConnected ? 'success' : 'error'"
                :text="isConnected ? 'Connected' : 'Disconnected'"
              />
              <a-divider type="vertical" />
              <span style="font-size: 12px; color: #475569">
                Session: ↓{{ formatBytes(cumRecv, 1) }} ↑{{
                  formatBytes(cumSent, 1)
                }}
              </span>
              <span style="font-size: 12px; color: #475569">
                TCP Est: {{ establishedConns().length }}
              </span>
              <span style="font-size: 12px; color: #475569">
                DNS: {{ dnsMap.length }}
              </span>
            </div>
            <a-radio-group
              v-model:value="wsTimeRange"
              size="small"
              button-style="solid"
            >
              <a-radio-button :value="30">30s</a-radio-button>
              <a-radio-button :value="60">60s</a-radio-button>
              <a-radio-button :value="120">2m</a-radio-button>
              <a-radio-button :value="300">5m</a-radio-button>
            </a-radio-group>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <!-- ── Tabbed workspace ────────────────────────────────────── -->
    <a-card size="small" :bordered="false">
      <a-tabs v-model:activeKey="activeTab" size="small">
        <!-- ── Overview tab ──────────────────────────────────── -->
        <a-tab-pane key="overview">
          <template #tab>
            <span><DashboardOutlined /> Overview</span>
          </template>
          <a-row :gutter="[16, 16]">
            <!-- Traffic graph -->
            <a-col :span="24">
              <a-card title="Traffic Graph" size="small">
                <div style="position: relative">
                  <TrafficGraph
                    :interfaces="trafficInterfaces"
                    :height="graphHeight"
                    @select-interface="openInterfaceChart"
                  />
                  <div
                    style="
                      position: absolute;
                      bottom: 0;
                      left: 0;
                      right: 0;
                      height: 8px;
                      cursor: ns-resize;
                      background: rgba(0, 0, 0, 0.02);
                      display: flex;
                      justify-content: center;
                      align-items: center;
                    "
                    @mousedown="startResize"
                  >
                    <div
                      style="
                        width: 32px;
                        height: 3px;
                        background: #d9d9d9;
                        border-radius: 2px;
                      "
                    />
                  </div>
                </div>
              </a-card>
            </a-col>
            <!-- Flow summary -->
            <a-col :xs="24" :lg="12">
              <a-card title="Top Processes" size="small">
                <a-table
                  :data-source="topProcesses"
                  :columns="[
                    { title: 'Process', dataIndex: '0', key: 'name' },
                    {
                      title: 'Flows',
                      dataIndex: '1',
                      key: 'count',
                      align: 'right',
                    },
                  ]"
                  :pagination="false"
                  size="small"
                  row-key="0"
                  :locale="{ emptyText: 'No flows yet' }"
                />
              </a-card>
            </a-col>
            <a-col :xs="24" :lg="12">
              <a-card title="Protocols" size="small">
                <a-table
                  :data-source="flowProtocols"
                  :columns="[
                    { title: 'Protocol', dataIndex: '0', key: 'name' },
                    {
                      title: 'Count',
                      dataIndex: '1',
                      key: 'count',
                      align: 'right',
                    },
                  ]"
                  :pagination="false"
                  size="small"
                  row-key="0"
                  :locale="{ emptyText: 'No flows yet' }"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'name'">
                      <a-tag :color="protocolColor(record[0])" size="small">{{
                        record[0]
                      }}</a-tag>
                    </template>
                  </template>
                </a-table>
              </a-card>
            </a-col>
            <!-- Risk summary -->
            <a-col :span="24">
              <a-card title="Risk Distribution" size="small">
                <div style="display: flex; gap: 24px; align-items: center">
                  <div style="display: flex; align-items: center; gap: 8px">
                    <a-tag color="red">High</a-tag>
                    <span style="font-size: 20px; font-weight: bold">{{
                      riskSummary.high
                    }}</span>
                  </div>
                  <div style="display: flex; align-items: center; gap: 8px">
                    <a-tag color="orange">Medium</a-tag>
                    <span style="font-size: 20px; font-weight: bold">{{
                      riskSummary.medium
                    }}</span>
                  </div>
                  <div style="display: flex; align-items: center; gap: 8px">
                    <a-tag color="gold">Low</a-tag>
                    <span style="font-size: 20px; font-weight: bold">{{
                      riskSummary.low
                    }}</span>
                  </div>
                  <a-divider type="vertical" />
                  <span style="font-size: 12px; color: #666">
                    Public: {{ publicFlows().length }} | ↓{{
                      formatBytes(totalBytesIn())
                    }}
                    ↑{{ formatBytes(totalBytesOut()) }}
                  </span>
                </div>
              </a-card>
            </a-col>
          </a-row>
        </a-tab-pane>

        <!-- ── Flows tab ─────────────────────────────────────── -->
        <a-tab-pane key="flows">
          <template #tab>
            <span><NodeIndexOutlined /> Flows</span>
          </template>
          <!-- Filter bar -->
          <div style="margin-bottom: 12px">
            <a-space wrap>
              <a-input-search
                v-model:value="filterQuery"
                placeholder="process:curl dport:443 sni:github.com state:ESTABLISHED"
                allow-clear
                size="small"
                style="width: 420px"
                @search="refreshFlows"
                @press-enter="refreshFlows"
              />
              <a-select
                v-model:value="sortKey"
                size="small"
                style="width: 140px"
                @change="refreshFlows"
              >
                <a-select-option value="lastSeen"
                  >Recently Updated</a-select-option
                >
                <a-select-option value="risk">Risk Priority</a-select-option>
                <a-select-option value="bandwidth">Bandwidth</a-select-option>
                <a-select-option value="+dst">By Destination</a-select-option>
              </a-select>
              <a-switch
                v-model:checked="showHistoric"
                size="small"
                checked-children="Historic"
                un-checked-children="Active"
                @change="refreshFlows"
              />
              <a-button size="small" @click="refreshFlows">Refresh</a-button>
            </a-space>
            <div
              style="
                margin-top: 8px;
                display: flex;
                align-items: center;
                gap: 6px;
                flex-wrap: wrap;
                font-size: 12px;
                color: #64748b;
              "
            >
              <span>Quick:</span>
              <a-tag
                v-for="ex in filterExamples"
                :key="ex"
                class="filter-chip"
                @click="applyFilterExample(ex)"
                >{{ ex }}</a-tag
              >
            </div>
            <a-alert
              v-if="filterError"
              type="warning"
              show-icon
              :message="filterError"
              style="margin-top: 8px"
            />
          </div>

          <a-tabs size="small">
            <a-tab-pane key="flows-table" tab="Aggregated Flows">
              <a-table
                :columns="flowColumns"
                :data-source="flowData"
                :pagination="{ pageSize: 20, size: 'small' }"
                size="small"
                row-key="key"
                :loading="flowsLoading"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'dst'">
                    <div
                      style="display: flex; flex-direction: column; gap: 2px"
                    >
                      <span
                        style="
                          font-family: monospace;
                          font-size: 12px;
                          cursor: pointer;
                          color: #1677ff;
                        "
                        role="button"
                        tabindex="0"
                        :aria-label="`Open flow detail for ${record.dstIp}`"
                        @click="openFlowDetail(record)"
                        @keydown.enter.prevent="openFlowDetail(record)"
                        @keydown.space.prevent="openFlowDetail(record)"
                        >{{ record.dstIp }}</span
                      >
                      <span
                        style="
                          font-family: monospace;
                          font-size: 11px;
                          color: #94a3b8;
                        "
                        >{{ record.srcIp }}:{{ record.srcPort }}</span
                      >
                    </div>
                  </template>
                  <template v-else-if="column.key === 'app'">
                    <a-space :size="4" wrap>
                      <a-tag
                        v-if="record.appProtocol"
                        :color="protocolColor(record.appProtocol)"
                        size="small"
                        >{{ record.appProtocol }}</a-tag
                      >
                      <a-tag
                        v-if="record.dstService"
                        color="blue"
                        size="small"
                        >{{ record.dstService }}</a-tag
                      >
                    </a-space>
                  </template>
                  <template v-else-if="column.key === 'domain'">
                    <div
                      style="display: flex; flex-direction: column; gap: 4px"
                    >
                      <span v-if="record.dstDomain" style="color: #1890ff">{{
                        record.dstDomain
                      }}</span>
                      <a-space :size="4" wrap>
                        <a-tag v-if="record.sni" color="geekblue" size="small"
                          >SNI {{ record.sni }}</a-tag
                        >
                        <a-tag v-if="record.httpHost" color="cyan" size="small"
                          >{{ record.httpMethod || "HTTP" }}
                          {{ record.httpHost }}</a-tag
                        >
                        <a-tag
                          v-if="
                            record.dnsName &&
                            record.dnsName !== record.dstDomain
                          "
                          color="purple"
                          size="small"
                          >DNS {{ record.dnsName }}</a-tag
                        >
                        <a-tag v-if="record.tlsAlpn" color="blue" size="small"
                          >ALPN {{ record.tlsAlpn }}</a-tag
                        >
                      </a-space>
                    </div>
                  </template>
                  <template v-else-if="column.key === 'scope'">
                    <a-tag
                      :color="
                        record.ipScope === 'Public'
                          ? 'orange'
                          : record.ipScope === 'Private'
                            ? 'green'
                            : 'default'
                      "
                      size="small"
                      >{{ record.ipScope }}</a-tag
                    >
                  </template>
                  <template v-else-if="column.key === 'out'">{{
                    formatBytes(record.bytesOut)
                  }}</template>
                  <template v-else-if="column.key === 'rate'">
                    <span style="font-family: monospace; font-size: 12px"
                      >↑{{ formatRate(record.currentBpsOut || 0) }}</span
                    >
                  </template>
                  <template v-else-if="column.key === 'stale'">
                    <a-tag
                      :color="staleColor(record.staleLevel)"
                      size="small"
                      >{{ record.staleLevel || "active" }}</a-tag
                    >
                  </template>
                  <template v-else-if="column.key === 'risk'">
                    <a-tooltip :title="(record.riskReasons || []).join('; ')">
                      <a-tag :color="riskColor(record.riskScore)" size="small"
                        >{{ record.riskLevel || "risk" }}
                        {{ (record.riskScore * 100).toFixed(0) }}%</a-tag
                      >
                    </a-tooltip>
                  </template>
                </template>
              </a-table>
            </a-tab-pane>
            <a-tab-pane key="tcp" tab="TCP State">
              <a-table
                :columns="tcpColumns"
                :data-source="tcpConns"
                :pagination="{ pageSize: 20, size: 'small' }"
                size="small"
                row-key="key"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'state'">
                    <a-badge
                      :color="stateColor(record.state)"
                      :text="record.state"
                    />
                  </template>
                </template>
              </a-table>
            </a-tab-pane>
          </a-tabs>
          <div v-if="flowsError" style="color: #ff4d4f; margin-top: 8px">
            {{ flowsError }}
          </div>
        </a-tab-pane>

        <!-- ── Interfaces tab ────────────────────────────────── -->
        <a-tab-pane key="interfaces">
          <template #tab>
            <span><WifiOutlined /> Interfaces</span>
          </template>
          <a-table
            :data-source="trafficInterfaces"
            :columns="[
              { title: 'Interface', dataIndex: 'name', key: 'name' },
              {
                title: 'Download',
                dataIndex: 'readSpeed',
                key: 'readSpeed',
                align: 'right',
              },
              {
                title: 'Upload',
                dataIndex: 'writeSpeed',
                key: 'writeSpeed',
                align: 'right',
              },
              { title: 'Total', key: 'total', align: 'right' },
              { title: 'Level', key: 'level', align: 'center' },
              { title: 'Errors', key: 'errors', align: 'right' },
              { title: 'Drops', key: 'drops', align: 'right' },
            ]"
            :pagination="false"
            size="small"
            row-key="name"
            :locale="{ emptyText: 'No interfaces detected' }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'name'">
                <span
                  style="color: #1677ff; cursor: pointer"
                  role="button"
                  tabindex="0"
                  :aria-label="`Open traffic chart for interface ${record.name}`"
                  @click="openInterfaceChart(record.name)"
                  @keydown.enter.prevent="openInterfaceChart(record.name)"
                  @keydown.space.prevent="openInterfaceChart(record.name)"
                  >{{ record.name }}</span
                >
              </template>
              <template v-else-if="column.key === 'readSpeed'">
                <span style="color: #1890ff"
                  >{{ formatBytes(record.readSpeed, 2) }}/s</span
                >
              </template>
              <template v-else-if="column.key === 'writeSpeed'">
                <span style="color: #52c41a"
                  >{{ formatBytes(record.writeSpeed, 2) }}/s</span
                >
              </template>
              <template v-else-if="column.key === 'total'">
                <span style="font-weight: 600"
                  >{{
                    formatBytes(record.readSpeed + record.writeSpeed, 2)
                  }}/s</span
                >
              </template>
              <template v-else-if="column.key === 'level'">
                <a-tag
                  :color="
                    getTrafficLevelColor(record.readSpeed + record.writeSpeed)
                  "
                  >{{
                    getTrafficLevelLabel(record.readSpeed + record.writeSpeed)
                  }}</a-tag
                >
              </template>
              <template v-else-if="column.key === 'errors'">
                {{
                  (apiInterfaces.find((i) => i.name === record.name)?.errin ||
                    0) +
                  (apiInterfaces.find((i) => i.name === record.name)?.errout ||
                    0)
                }}
              </template>
              <template v-else-if="column.key === 'drops'">
                {{
                  (apiInterfaces.find((i) => i.name === record.name)?.dropin ||
                    0) +
                  (apiInterfaces.find((i) => i.name === record.name)?.dropout ||
                    0)
                }}
              </template>
            </template>
          </a-table>
          <div
            v-if="totalErrors() > 0 || totalDrops() > 0"
            style="margin-top: 12px; display: flex; gap: 12px"
          >
            <a-tag v-if="totalErrors() > 0" color="red"
              >Total Errors: {{ totalErrors() }}</a-tag
            >
            <a-tag v-if="totalDrops() > 0" color="orange"
              >Total Drops: {{ totalDrops() }}</a-tag
            >
          </div>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <!-- ── Flow detail modal ──────────────────────────────────── -->
    <a-modal
      v-model:open="showFlowDetail"
      title="Flow Detail"
      :footer="null"
      width="800px"
    >
      <template v-if="selectedFlow">
        <a-descriptions :column="2" size="small" bordered>
          <a-descriptions-item label="Flow ID">{{
            selectedFlow.flowId
          }}</a-descriptions-item>
          <a-descriptions-item label="Transport">{{
            selectedFlow.transport || selectedFlow.protocol
          }}</a-descriptions-item>
          <a-descriptions-item label="Source"
            >{{ selectedFlow.srcIp }}:{{
              selectedFlow.srcPort
            }}</a-descriptions-item
          >
          <a-descriptions-item label="Destination"
            >{{ selectedFlow.dstIp }}:{{
              selectedFlow.dstPort
            }}</a-descriptions-item
          >
          <a-descriptions-item label="App Protocol">
            <a-tag
              v-if="selectedFlow.appProtocol"
              :color="protocolColor(selectedFlow.appProtocol)"
              size="small"
              >{{ selectedFlow.appProtocol }}</a-tag
            >
          </a-descriptions-item>
          <a-descriptions-item label="Service">{{
            selectedFlow.dstService || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="Domain">{{
            selectedFlow.dstDomain || selectedFlow.dnsName || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="SNI">{{
            selectedFlow.sni || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="HTTP Host">{{
            selectedFlow.httpHost || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="TLS ALPN">{{
            selectedFlow.tlsAlpn || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="IP Scope">
            <a-tag
              :color="
                selectedFlow.ipScope === 'Public'
                  ? 'orange'
                  : selectedFlow.ipScope === 'Private'
                    ? 'green'
                    : 'default'
              "
              size="small"
              >{{ selectedFlow.ipScope }}</a-tag
            >
          </a-descriptions-item>
          <a-descriptions-item label="Direction">{{
            selectedFlow.direction
          }}</a-descriptions-item>
          <a-descriptions-item label="State">{{
            selectedFlow.state || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="Stale">
            <a-tag :color="staleColor(selectedFlow.staleLevel)" size="small">{{
              selectedFlow.staleLevel
            }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Bytes In">{{
            formatBytes(selectedFlow.bytesIn)
          }}</a-descriptions-item>
          <a-descriptions-item label="Bytes Out">{{
            formatBytes(selectedFlow.bytesOut)
          }}</a-descriptions-item>
          <a-descriptions-item label="Current Rate">
            ↓{{ formatRate(selectedFlow.currentBpsIn || 0) }} ↑{{
              formatRate(selectedFlow.currentBpsOut || 0)
            }}
          </a-descriptions-item>
          <a-descriptions-item label="Peak Rate">
            ↓{{ formatRate(selectedFlow.peakBpsIn || 0) }} ↑{{
              formatRate(selectedFlow.peakBpsOut || 0)
            }}
          </a-descriptions-item>
          <a-descriptions-item label="Risk">
            <a-tag :color="riskColor(selectedFlow.riskScore)"
              >{{ (selectedFlow.riskScore * 100).toFixed(0) }}%
              {{ selectedFlow.riskLevel }}</a-tag
            >
          </a-descriptions-item>
          <a-descriptions-item label="Historic">{{
            selectedFlow.historic ? "Yes" : "No"
          }}</a-descriptions-item>
          <a-descriptions-item label="Processes" :span="2">{{
            (selectedFlow.processComms || []).join(", ") || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="PIDs" :span="2">{{
            (selectedFlow.processPids || []).join(", ")
          }}</a-descriptions-item>
          <a-descriptions-item label="Agent Run IDs" :span="2">{{
            (selectedFlow.agentRunIds || []).join(", ") || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="Task IDs" :span="2">{{
            (selectedFlow.taskIds || []).join(", ") || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="Tool Call IDs" :span="2">{{
            (selectedFlow.toolCallIds || []).join(", ") || "-"
          }}</a-descriptions-item>
          <a-descriptions-item label="Risk Reasons" :span="2">
            <a-space wrap>
              <a-tag
                v-for="r in selectedFlow.riskReasons || []"
                :key="r"
                color="volcano"
                size="small"
                >{{ r }}</a-tag
              >
            </a-space>
          </a-descriptions-item>
        </a-descriptions>
      </template>
    </a-modal>

    <!-- ── Interface chart modal ──────────────────────────────── -->
    <a-modal
      v-model:open="showInterfaceChartModal"
      :title="
        selectedInterfaceName
          ? `Interface History: ${selectedInterfaceName}`
          : 'Interface History'
      "
      :footer="null"
      width="900px"
    >
      <div
        style="
          margin-bottom: 16px;
          display: flex;
          flex-wrap: wrap;
          align-items: center;
          justify-content: space-between;
          gap: 12px;
        "
      >
        <a-radio-group
          v-model:value="interfaceChartTimeRange"
          size="small"
          button-style="solid"
        >
          <a-radio-button :value="30">30s</a-radio-button>
          <a-radio-button :value="60">60s</a-radio-button>
          <a-radio-button :value="120">2m</a-radio-button>
          <a-radio-button :value="300">5m</a-radio-button>
        </a-radio-group>
      </div>
      <div
        v-if="showInterfaceChartModal"
        style="
          background: #fff;
          padding: 10px;
          border-radius: 4px;
          border: 1px solid #f0f0f0;
        "
      >
        <VueApexCharts
          type="line"
          height="360"
          :options="interfaceChartOptions"
          :series="interfaceChartSeries"
        />
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.filter-chip {
  cursor: pointer;
}
</style>
