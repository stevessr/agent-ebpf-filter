import { ref } from "vue";
import axios from "axios";

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
  const error = ref("");
  let timer: number | null = null;
  let autoRefreshActive = false;
  let autoRefreshGeneration = 0;
  let flowRequestGeneration = 0;
  let tcpRequestGeneration = 0;
  let flowController: AbortController | null = null;
  let tcpController: AbortController | null = null;

  async function fetchFlows(
    params?: Record<string, string>,
    signal?: AbortSignal,
  ) {
    flowController?.abort();
    const controller = new AbortController();
    flowController = controller;
    const generation = ++flowRequestGeneration;
    const abort = () => controller.abort();
    if (signal?.aborted) abort();
    else signal?.addEventListener("abort", abort, { once: true });

    try {
      loading.value = true;
      const res = await axios.get("/network/flows", {
        params,
        signal: controller.signal,
      });
      if (generation !== flowRequestGeneration) return;
      flows.value = res.data.flows || [];
      error.value = "";
    } catch (e: any) {
      if (controller.signal.aborted || generation !== flowRequestGeneration) {
        return;
      }
      error.value = e.message || "Failed to fetch flows";
    } finally {
      signal?.removeEventListener("abort", abort);
      if (generation === flowRequestGeneration) {
        loading.value = false;
        flowController = null;
      }
    }
  }

  async function fetchTCPState(signal?: AbortSignal) {
    tcpController?.abort();
    const controller = new AbortController();
    tcpController = controller;
    const generation = ++tcpRequestGeneration;
    const abort = () => controller.abort();
    if (signal?.aborted) abort();
    else signal?.addEventListener("abort", abort, { once: true });

    try {
      const res = await axios.get("/network/tcp-state", {
        signal: controller.signal,
      });
      if (generation !== tcpRequestGeneration) return;
      tcpConns.value = res.data.connections || [];
      error.value = "";
    } catch (e: any) {
      if (controller.signal.aborted || generation !== tcpRequestGeneration) {
        return;
      }
      error.value = e.message || "Failed to fetch TCP state";
    } finally {
      signal?.removeEventListener("abort", abort);
      if (generation === tcpRequestGeneration) tcpController = null;
    }
  }

  async function analyzeEndpoint(
    endpoint: string,
  ): Promise<EndpointAnalysis | null> {
    try {
      const res = await axios.get("/network/analyze", { params: { endpoint } });
      return res.data;
    } catch {
      return null;
    }
  }

  async function lookupGeoIP(ip: string): Promise<GeoIPRecord | null> {
    try {
      const res = await axios.get("/network/geoip", { params: { ip } });
      return res.data;
    } catch {
      return null;
    }
  }

  async function runAutoRefresh(generation: number) {
    await Promise.all([fetchFlows(), fetchTCPState()]);
    if (autoRefreshActive && generation === autoRefreshGeneration) {
      timer = window.setTimeout(
        () => void runAutoRefresh(generation),
        refreshMs,
      );
    }
  }

  function startAutoRefresh() {
    if (autoRefreshActive) return;
    autoRefreshActive = true;
    const generation = ++autoRefreshGeneration;
    void runAutoRefresh(generation);
  }

  function stopAutoRefresh() {
    autoRefreshActive = false;
    autoRefreshGeneration++;
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    cancelPendingRequests();
  }

  function cancelPendingRequests() {
    flowRequestGeneration++;
    tcpRequestGeneration++;
    flowController?.abort();
    tcpController?.abort();
    flowController = null;
    tcpController = null;
    loading.value = false;
  }

  const totalBytesOut = () => flows.value.reduce((s, f) => s + f.bytesOut, 0);
  const totalBytesIn = () => flows.value.reduce((s, f) => s + f.bytesIn, 0);
  const suspiciousFlows = () => flows.value.filter((f) => f.riskScore >= 0.7);
  const publicFlows = () => flows.value.filter((f) => f.ipScope === "Public");
  const establishedConns = () =>
    tcpConns.value.filter((c) => c.state === "ESTABLISHED");

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
    cancelPendingRequests,
    totalBytesOut,
    totalBytesIn,
    suspiciousFlows,
    publicFlows,
    establishedConns,
  };
}
