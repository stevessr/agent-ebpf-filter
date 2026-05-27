<script setup lang="ts">
import type { NetworkFlow } from '../../composables/network/useNetworkEnrichment';

defineProps<{
  showFlowDetail: boolean;
  selectedFlow: NetworkFlow | null;
  protocolColor: (p?: string) => string;
  staleColor: (level?: string) => string;
  riskColor: (score: number) => string;
  formatBytes: (value: number | string, decimals?: number) => string;
  formatRate: (bytesPerSecond: number) => string;
}>();

const emit = defineEmits<{
  (e: 'update:showFlowDetail', value: boolean): void;
}>();
</script>

<template>
  <a-modal :open="showFlowDetail" title="Flow Detail" :footer="null" width="800px" @update:open="emit('update:showFlowDetail', $event)">
    <template v-if="selectedFlow">
      <a-descriptions :column="2" size="small" bordered>
        <a-descriptions-item label="Flow ID">{{ selectedFlow.flowId }}</a-descriptions-item>
        <a-descriptions-item label="Transport">{{ selectedFlow.transport || selectedFlow.protocol }}</a-descriptions-item>
        <a-descriptions-item label="Source">{{ selectedFlow.srcIp }}:{{ selectedFlow.srcPort }}</a-descriptions-item>
        <a-descriptions-item label="Destination">{{ selectedFlow.dstIp }}:{{ selectedFlow.dstPort }}</a-descriptions-item>
        <a-descriptions-item label="App Protocol">
          <a-tag v-if="selectedFlow.appProtocol" :color="protocolColor(selectedFlow.appProtocol)" size="small">{{ selectedFlow.appProtocol }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Service">{{ selectedFlow.dstService || '-' }}</a-descriptions-item>
        <a-descriptions-item label="Domain">{{ selectedFlow.dstDomain || selectedFlow.dnsName || '-' }}</a-descriptions-item>
        <a-descriptions-item label="SNI">{{ selectedFlow.sni || '-' }}</a-descriptions-item>
        <a-descriptions-item label="HTTP Host">{{ selectedFlow.httpHost || '-' }}</a-descriptions-item>
        <a-descriptions-item label="TLS ALPN">{{ selectedFlow.tlsAlpn || '-' }}</a-descriptions-item>
        <a-descriptions-item label="IP Scope">
          <a-tag :color="selectedFlow.ipScope === 'Public' ? 'orange' : selectedFlow.ipScope === 'Private' ? 'green' : 'default'" size="small">{{ selectedFlow.ipScope }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Direction">{{ selectedFlow.direction }}</a-descriptions-item>
        <a-descriptions-item label="State">{{ selectedFlow.state || '-' }}</a-descriptions-item>
        <a-descriptions-item label="Stale">
          <a-tag :color="staleColor(selectedFlow.staleLevel)" size="small">{{ selectedFlow.staleLevel }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Bytes In">{{ formatBytes(selectedFlow.bytesIn) }}</a-descriptions-item>
        <a-descriptions-item label="Bytes Out">{{ formatBytes(selectedFlow.bytesOut) }}</a-descriptions-item>
        <a-descriptions-item label="Current Rate">
          ↓{{ formatRate(selectedFlow.currentBpsIn || 0) }} ↑{{ formatRate(selectedFlow.currentBpsOut || 0) }}
        </a-descriptions-item>
        <a-descriptions-item label="Peak Rate">
          ↓{{ formatRate(selectedFlow.peakBpsIn || 0) }} ↑{{ formatRate(selectedFlow.peakBpsOut || 0) }}
        </a-descriptions-item>
        <a-descriptions-item label="Risk">
          <a-tag :color="riskColor(selectedFlow.riskScore)">{{ (selectedFlow.riskScore * 100).toFixed(0) }}% {{ selectedFlow.riskLevel }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Historic">{{ selectedFlow.historic ? 'Yes' : 'No' }}</a-descriptions-item>
        <a-descriptions-item label="Processes" :span="2">{{ (selectedFlow.processComms || []).join(', ') || '-' }}</a-descriptions-item>
        <a-descriptions-item label="PIDs" :span="2">{{ (selectedFlow.processPids || []).join(', ') }}</a-descriptions-item>
        <a-descriptions-item label="Agent Run IDs" :span="2">{{ (selectedFlow.agentRunIds || []).join(', ') || '-' }}</a-descriptions-item>
        <a-descriptions-item label="Task IDs" :span="2">{{ (selectedFlow.taskIds || []).join(', ') || '-' }}</a-descriptions-item>
        <a-descriptions-item label="Tool Call IDs" :span="2">{{ (selectedFlow.toolCallIds || []).join(', ') || '-' }}</a-descriptions-item>
        <a-descriptions-item label="Risk Reasons" :span="2">
          <a-space wrap>
            <a-tag v-for="r in (selectedFlow.riskReasons || [])" :key="r" color="volcano" size="small">{{ r }}</a-tag>
          </a-space>
        </a-descriptions-item>
      </a-descriptions>
    </template>
  </a-modal>
</template>
