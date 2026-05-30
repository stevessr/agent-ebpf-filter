import { computed, onMounted, onUnmounted, ref } from 'vue';
import type { NetworkFlow } from '../../composables/network/useNetworkEnrichment';
import { useNetworkEnrichment } from '../../composables/network/useNetworkEnrichment';
import { useNetworkInterfaces } from '../../composables/network/useNetworkInterfaces';

export function useFlowFilters() {
  const megabyte = 1024 * 1024;

  // ── Flow enrichment ────────────────────────────────────────────────
  const {
    flows, tcpConns, loading: flowsLoading, error: flowsError,
    fetchFlows, fetchTCPState,
    totalBytesOut, totalBytesIn, suspiciousFlows, publicFlows, establishedConns,
  } = useNetworkEnrichment(5000);

  // ── Interface stats from REST API ──────────────────────────────────
  const {
    interfaces: apiInterfaces, interfaceRates: apiInterfaceRates, dnsMap,
    fetchInterfaces, fetchDNSCache,
    totalErrors, totalDrops,
  } = useNetworkInterfaces(5000);

  // ── Flow filter state ──────────────────────────────────────────────
  const filterQuery = ref('');
  const showHistoric = ref(false);
  const sortKey = ref('lastSeen');
  const filterError = ref('');

  const filterExamples = ['process:curl', 'dport:443', 'sni:github.com', 'state:ESTABLISHED', 'risk:0.7'];

  const validateFilter = (query: string) => {
    const allowed = new Set([
      'port', 'dport', 'sport', 'src', 'dst', 'process', 'comm', 'pid',
      'agent', 'task', 'tool', 'sni', 'host', 'domain', 'service', 'app',
      'state', 'proto', 'transport', 'scope', 'risk',
    ]);
    const invalid = query.trim().split(/\s+/).filter(Boolean)
      .filter((t) => t.includes(':') && !allowed.has(t.split(':', 1)[0].toLowerCase()));
    return invalid.length ? `Unknown filter: ${invalid.join(', ')}` : '';
  };

  const flowParams = computed(() => {
    const params: Record<string, string> = {
      showHistoric: String(showHistoric.value),
      sort: sortKey.value,
      limit: '100',
    };
    const f = filterQuery.value.trim();
    if (f) params.filter = f;
    return params;
  });

  const refreshFlows = async () => {
    filterError.value = validateFilter(filterQuery.value);
    if (!filterError.value) await fetchFlows(flowParams.value);
    await fetchTCPState();
  };

  const applyFilterExample = (example: string) => {
    const tokens = filterQuery.value.trim().split(/\s+/).filter(Boolean);
    if (!tokens.includes(example)) tokens.push(example);
    filterQuery.value = tokens.join(' ');
    void refreshFlows();
  };

  // ── Overview computed ──────────────────────────────────────────────
  const topProcesses = computed(() => {
    const counts: Record<string, number> = {};
    for (const f of flows.value) {
      for (const c of f.processComms) {
        counts[c] = (counts[c] || 0) + 1;
      }
    }
    return Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, 10);
  });

  const riskSummary = computed(() => ({
    high: flows.value.filter(f => f.riskScore >= 0.8).length,
    medium: flows.value.filter(f => f.riskScore >= 0.5 && f.riskScore < 0.8).length,
    low: flows.value.filter(f => f.riskScore > 0 && f.riskScore < 0.5).length,
  }));

  const flowProtocols = computed(() => {
    const counts: Record<string, number> = {};
    for (const f of flows.value) {
      const p = f.appProtocol || 'Unknown';
      counts[p] = (counts[p] || 0) + 1;
    }
    return Object.entries(counts).sort((a, b) => b[1] - a[1]);
  });

  // ── Color helpers ──────────────────────────────────────────────────
  const protocolColor = (p?: string) => {
    switch ((p || '').toUpperCase()) {
      case 'HTTP': return 'blue';
      case 'TLS': case 'HTTPS/TLS': return 'geekblue';
      case 'DNS': case 'MDNS': case 'LLMNR': return 'purple';
      case 'SSH': return 'volcano';
      case 'QUIC': return 'cyan';
      case 'SSDP': return 'orange';
      case 'NTP': return 'gold';
      case 'SNMP': return 'green';
      case 'NETBIOS': return 'red';
      case 'DHCP': return 'lime';
      default: return 'default';
    }
  };

  const stateColor = (s: string) => {
    switch (s) {
      case 'ESTABLISHED': return 'green';
      case 'SYN_SENT': case 'SYN_RECV': return 'orange';
      case 'FIN_WAIT1': case 'FIN_WAIT2': case 'CLOSING': return 'gold';
      case 'TIME_WAIT': case 'CLOSE_WAIT': case 'LAST_ACK': return 'volcano';
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

  const getTrafficLevelColor = (bps: number) => {
    if (bps >= 10 * megabyte) return 'red';
    if (bps >= megabyte) return 'gold';
    return 'green';
  };

  const getTrafficLevelLabel = (bps: number) => {
    if (bps >= 10 * megabyte) return 'hot';
    if (bps >= megabyte) return 'busy';
    return 'steady';
  };

  // ── Flow detail ────────────────────────────────────────────────────
  const selectedFlow = ref<NetworkFlow | null>(null);
  const showFlowDetail = ref(false);

  const openFlowDetail = (flow: NetworkFlow) => {
    selectedFlow.value = flow;
    showFlowDetail.value = true;
  };

  // ── Table columns ──────────────────────────────────────────────────
  const flowColumns = [
    { title: 'Destination', dataIndex: 'dstIp', key: 'dst', width: 200 },
    { title: 'Port', dataIndex: 'dstPort', key: 'port', width: 70 },
    { title: 'Protocol', dataIndex: 'appProtocol', key: 'app', width: 110 },
    { title: 'Domain/DPI', dataIndex: 'dstDomain', key: 'domain', width: 220 },
    { title: 'Scope', dataIndex: 'ipScope', key: 'scope', width: 90 },
    { title: 'Process', dataIndex: 'comm', key: 'comm', width: 120 },
    { title: 'Out', dataIndex: 'bytesOut', key: 'out', width: 90 },
    { title: 'Rate', dataIndex: 'currentBpsOut', key: 'rate', width: 90 },
    { title: 'State', dataIndex: 'staleLevel', key: 'stale', width: 90 },
    { title: 'Risk', dataIndex: 'riskScore', key: 'risk', width: 70 },
  ];

  const flowData = computed(() =>
    flows.value.map(f => ({ ...f, comm: f.processComms[0] || '-', key: f.flowId || `${f.srcIp}:${f.srcPort}->${f.dstIp}:${f.dstPort}` })));

  const tcpColumns = [
    { title: 'Source', dataIndex: 'srcIp', key: 'src', width: 150 },
    { title: 'Destination', dataIndex: 'dstIp', key: 'dst', width: 150 },
    { title: 'Port', dataIndex: 'dstPort', key: 'port', width: 70 },
    { title: 'State', dataIndex: 'state', key: 'state', width: 120 },
    { title: 'Process', dataIndex: 'comm', key: 'comm', width: 120 },
  ];

  // ── Lifecycle ──────────────────────────────────────────────────────
  let flowTimer: ReturnType<typeof setInterval> | null = null;

  onMounted(() => {
    void refreshFlows();
    void fetchInterfaces();
    fetchDNSCache();
    flowTimer = setInterval(() => { void refreshFlows(); void fetchInterfaces(); }, 5000);
  });

  onUnmounted(() => {
    if (flowTimer !== null) { clearInterval(flowTimer); flowTimer = null; }
  });

  return {
    flows,
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
    flowParams,
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
  };
}
