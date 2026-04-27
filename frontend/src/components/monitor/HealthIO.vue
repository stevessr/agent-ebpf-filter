<script setup lang="ts">
import { DatabaseOutlined } from '@ant-design/icons-vue';
import type { GlobalStats, StatsHistory, IOSpeed } from '../../composables/useMonitorData';

const props = defineProps<{
  systemStats: GlobalStats;
  statsHistory: StatsHistory;
  groupedNetInterfaces: Record<string, IOSpeed[]>;
  groupedDiskDevices: Record<string, { main: IOSpeed; partitions: IOSpeed[] }>;
  formatBytesWithUnit: (bytes: number) => string;
  openHistoryChart: (title: string, datasets: { name: string; data: { time: number; value: number }[]; color?: string }[]) => void;
}>();
</script>

<template>
  <a-row :gutter="16" style="padding-top: 16px;">
    <a-col :span="12">
      <a-card size="small" :bordered="false" style="background: #fafafa;">
        <template #title>
          <div style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
            <span>Network Activity</span>
            <span style="font-size: 11px; color: #888; font-weight: normal;">(Total: ↓{{ formatBytesWithUnit(systemStats.totalNetRecv) }}/s ↑{{ formatBytesWithUnit(systemStats.totalNetSent) }}/s)</span>
          </div>
        </template>
        <template #extra>
          <a-space>
            <a-button type="link" size="small" @click="openHistoryChart('Global Network Activity', [
              { name: 'Recv', data: statsHistory.netRecv, color: '#52c41a' },
              { name: 'Sent', data: statsHistory.netSent, color: '#1890ff' }
            ])">All</a-button>
            <a-button type="link" size="small" @click="openHistoryChart('Split Network Activity', Object.entries(statsHistory.netDevices).flatMap(([name, d]) => [
              { name: `${name} Recv`, data: d.recv, color: undefined },
              { name: `${name} Sent`, data: d.sent, color: undefined }
            ]))">Split</a-button>
          </a-space>
        </template>
        <div style="display: flex; flex-direction: column; gap: 8px;">
          <div v-for="(group, label) in groupedNetInterfaces" :key="label">
            <div v-if="group.length > 0">
              <div style="font-size: 11px; font-weight: bold; color: #888; margin: 8px 0 4px;">{{ label }}</div>
              <div v-for="iface in group" :key="iface.name" style="margin-bottom: 8px; padding: 10px; border-radius: 6px; background: #fff; border: 1px solid #f0f0f0; cursor: pointer; transition: all 0.2s;" class="io-card" @click="openHistoryChart(`${iface.name} Activity`, [
                { name: 'Recv', data: statsHistory.netDevices[iface.name]?.recv || [], color: '#52c41a' },
                { name: 'Sent', data: statsHistory.netDevices[iface.name]?.sent || [], color: '#1890ff' }
              ])">
                <div style="font-family: monospace; font-weight: bold; margin-bottom: 6px; display: flex; justify-content: space-between;">
                  <span>{{ iface.name }}</span>
                </div>
                <div style="display: flex; gap: 16px; font-size: 12px;">
                  <span style="color: #52c41a; flex: 1;">↓ {{ formatBytesWithUnit(iface.readSpeed) }}/s</span>
                  <span style="color: #1890ff; flex: 1;">↑ {{ formatBytesWithUnit(iface.writeSpeed) }}/s</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </a-card>
    </a-col>
    <a-col :span="12">
      <a-card size="small" :bordered="false" style="background: #fafafa;">
        <template #title>
          <div style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
            <span>Storage Activity</span>
            <span style="font-size: 11px; color: #888; font-weight: normal;">(Total: R:{{ formatBytesWithUnit(systemStats.totalDiskRead) }}/s W:{{ formatBytesWithUnit(systemStats.totalDiskWrite) }}/s)</span>
          </div>
        </template>
        <template #extra>
          <a-space>
            <a-button type="link" size="small" @click="openHistoryChart('Global Disk Activity', [
              { name: 'Read', data: statsHistory.diskRead, color: '#faad14' },
              { name: 'Write', data: statsHistory.diskWrite, color: '#722ed1' }
            ])">All</a-button>
            <a-button type="link" size="small" @click="openHistoryChart('Split Disk Activity', Object.entries(statsHistory.diskDevices).flatMap(([name, d]) => [
              { name: `${name} Read`, data: d.read, color: undefined },
              { name: `${name} Write`, data: d.write, color: undefined }
            ]))">Split</a-button>
          </a-space>
        </template>
        <div style="display: flex; flex-direction: column; gap: 12px;">
          <div v-for="(disk, name) in groupedDiskDevices" :key="name">
            <div style="padding: 10px; border-radius: 6px; background: #fff; border: 1px solid #e8e8e8; cursor: pointer;" class="io-card" @click="openHistoryChart(`${name} Activity`, [
              { name: 'Read', data: statsHistory.diskDevices[name]?.read || [], color: '#faad14' },
              { name: 'Write', data: statsHistory.diskDevices[name]?.write || [], color: '#722ed1' }
            ])">
              <div style="font-family: monospace; font-weight: bold; margin-bottom: 6px; display: flex; align-items: center; gap: 8px;">
                <DatabaseOutlined style="color: #1890ff;" />
                <span>{{ name }}</span>
              </div>
              <div style="display: flex; gap: 16px; font-size: 12px; border-bottom: 1px solid #f5f5f5; padding-bottom: 8px; margin-bottom: 8px;">
                <span style="color: #faad14; flex: 1; font-weight: 500;">Read: {{ formatBytesWithUnit(disk.main.readSpeed) }}/s</span>
                <span style="color: #722ed1; flex: 1; font-weight: 500;">Write: {{ formatBytesWithUnit(disk.main.writeSpeed) }}/s</span>
              </div>
              <div v-if="disk.partitions.length > 0">
                <div v-for="part in disk.partitions" :key="part.name" style="display: flex; gap: 12px; font-size: 10px; padding: 2px 0 2px 20px; color: #666; border-left: 2px solid #f0f0f0; margin-left: 6px;">
                  <span style="width: 80px; font-family: monospace;">└─ {{ part.name }}</span>
                  <span style="color: #faad14; opacity: 0.8;">R: {{ formatBytesWithUnit(part.readSpeed) }}/s</span>
                  <span style="color: #722ed1; opacity: 0.8;">W: {{ formatBytesWithUnit(part.writeSpeed) }}/s</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </a-card>
    </a-col>
  </a-row>
</template>
