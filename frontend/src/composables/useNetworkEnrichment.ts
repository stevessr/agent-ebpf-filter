import { ref } from 'vue';
import axios from 'axios';

export interface NetworkFlow {
  flowId?: string;
  protocol: string;
  transport?: string;
  srcIp: string;
  srcPort: number;
  dstIp: string;
  dstPort: number;
  dstService?: string;
  dstDomain?: string;
  dnsName?: string;
  sni?: string;
  httpHost?: string;
  httpMethod?: string;
  tlsAlpn?: string;
  ipScope: string;
  direction: string;
  state?: string;
  bytesIn: number;
  bytesOut: number;
  packetsIn: number;
  packetsOut: number;
  currentBpsIn?: number;
  currentBpsOut?: number;
  peakBpsIn?: number;
  peakBpsOut?: number;
  processPids: number[];
  processComms: string[];
  agentRunIds?: string[];
  taskIds?: string[];
  toolCallIds?: string[];
  traceIds?: string[];
  spanIds?: string[];
  containerIds?: string[];
  decisions?: string[];
  firstSeen: number;
  lastSeen: number;
  durationMs?: number;
  staleLevel?: string;
  historic?: boolean;
  riskScore: number;
  riskLevel?: string;
  riskReasons?: string[];
  appProtocol?: string;
}

export interface TCPConnection {
  key: string;
  srcIp: string;
  dstIp: string;
  srcPort: number;
  dstPort: number;
  state: string;
  pid: number;
  comm: string;
  lastUpdate: number;
}

export interface EndpointAnalysis {
  endpoint: string;
  ipScope: string;
  service: string;
  domain: string;
  riskScore: number;
  isSuspicious: boolean;
}

export interface GeoIPRecord {
  ip: string;
  country: string;
  countryCode: string;
  asnOrg?: string;
  ipScope: string;
  service: string;
  domain: string;
  riskScore: number;
  isHighRisk: boolean;
}

export function useNetworkEnrichment(refreshMs = 5000) {
  const flows = ref<NetworkFlow[]>([]);
  const tcpConns = ref<TCPConnection[]>([]);
  const loading = ref(false);
  const error = ref('');
  let timer: number | null = null;

  async function fetchFlows(params?: Record<string, string>) {
    try {
      loading.value = true;
      const res = await axios.get('/network/flows', { params });
      flows.value = res.data.flows || [];
      error.value = '';
    } catch (e: any) {
      error.value = e.message || 'Failed to fetch flows';
    } finally {
      loading.value = false;
    }
  }

  async function fetchTCPState() {
    try {
      const res = await axios.get('/network/tcp-state');
      tcpConns.value = res.data.connections || [];
      error.value = '';
    } catch (e: any) {
      error.value = e.message || 'Failed to fetch TCP state';
    }
  }

  async function analyzeEndpoint(endpoint: string): Promise<EndpointAnalysis | null> {
    try {
      const res = await axios.get('/network/analyze', { params: { endpoint } });
      return res.data;
    } catch {
      return null;
    }
  }

  async function lookupGeoIP(ip: string): Promise<GeoIPRecord | null> {
    try {
      const res = await axios.get('/network/geoip', { params: { ip } });
      return res.data;
    } catch {
      return null;
    }
  }

  function startAutoRefresh() {
    fetchFlows();
    fetchTCPState();
    timer = window.setInterval(() => {
      fetchFlows();
      fetchTCPState();
    }, refreshMs);
  }

  function stopAutoRefresh() {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  const totalBytesOut = () => flows.value.reduce((s, f) => s + f.bytesOut, 0);
  const totalBytesIn = () => flows.value.reduce((s, f) => s + f.bytesIn, 0);
  const suspiciousFlows = () => flows.value.filter(f => f.riskScore >= 0.7);
  const publicFlows = () => flows.value.filter(f => f.ipScope === 'Public');
  const establishedConns = () => tcpConns.value.filter(c => c.state === 'ESTABLISHED');

  return {
    flows,
    tcpConns,
    loading,
    error,
    fetchFlows,
    fetchTCPState,
    analyzeEndpoint,
    lookupGeoIP,
    startAutoRefresh,
    stopAutoRefresh,
    totalBytesOut,
    totalBytesIn,
    suspiciousFlows,
    publicFlows,
    establishedConns,
  };
}
