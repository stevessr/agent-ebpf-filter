import { ref } from 'vue';
import axios from 'axios';

export interface InterfaceStats {
  name: string;
  bytesRecv: number;
  bytesSent: number;
  packetsRecv: number;
  packetsSent: number;
  errin: number;
  errout: number;
  dropin: number;
  dropout: number;
  fifoin?: number;
  fifoout?: number;
  timestamp: number;
}

export interface DNSMapsEntry {
  domain: string;
  ip: string;
  resolvedAt: number;
  expiresAt: number;
  ttlSeconds: number;
}

export function useNetworkInterfaces(refreshMs = 5000) {
  const interfaces = ref<InterfaceStats[]>([]);
  const dnsMap = ref<DNSMapsEntry[]>([]);
  const loading = ref(false);
  const error = ref('');
  let timer: ReturnType<typeof setInterval> | null = null;

  async function fetchInterfaces() {
    try {
      loading.value = true;
      const res = await axios.get('/network/interfaces');
      interfaces.value = res.data.interfaces || [];
      error.value = '';
    } catch (e: any) {
      error.value = e.message || 'Failed to fetch interfaces';
    } finally {
      loading.value = false;
    }
  }

  async function fetchDNSCache() {
    try {
      const res = await axios.get('/network/dns-cache');
      dnsMap.value = res.data.entries || [];
    } catch {
      // non-critical
    }
  }

  function startAutoRefresh() {
    fetchInterfaces();
    timer = setInterval(fetchInterfaces, refreshMs);
  }

  function stopAutoRefresh() {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  const totalRecvRate = () => {
    if (interfaces.value.length < 2) return 0;
    const now = Date.now();
    let rate = 0;
    for (const iface of interfaces.value) {
      // estimate from cumulative counters over last refresh interval
      const elapsed = (now - iface.timestamp) / 1000;
      if (elapsed > 0) {
        rate += iface.bytesRecv / elapsed;
      }
    }
    return rate;
  };

  const totalSentRate = () => {
    if (interfaces.value.length < 2) return 0;
    const now = Date.now();
    let rate = 0;
    for (const iface of interfaces.value) {
      const elapsed = (now - iface.timestamp) / 1000;
      if (elapsed > 0) {
        rate += iface.bytesSent / elapsed;
      }
    }
    return rate;
  };

  const totalErrors = () => interfaces.value.reduce((s, i) => s + i.errin + i.errout, 0);
  const totalDrops = () => interfaces.value.reduce((s, i) => s + i.dropin + i.dropout, 0);
  const totalBytesRecv = () => interfaces.value.reduce((s, i) => s + i.bytesRecv, 0);
  const totalBytesSent = () => interfaces.value.reduce((s, i) => s + i.bytesSent, 0);

  return {
    interfaces, dnsMap, loading, error,
    fetchInterfaces, fetchDNSCache,
    startAutoRefresh, stopAutoRefresh,
    totalRecvRate, totalSentRate, totalErrors, totalDrops,
    totalBytesRecv, totalBytesSent,
  };
}
