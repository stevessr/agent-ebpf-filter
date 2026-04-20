<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';

interface AgentEvent {
  key: string;
  pid: number;
  type: string;
  comm: string;
  path: string;
  time: string;
}

const events = ref<AgentEvent[]>([]);
const isConnected = ref(false);
let ws: WebSocket | null = null;

const columns = [
  {
    title: 'Time',
    dataIndex: 'time',
    key: 'time',
    width: 120,
  },
  {
    title: 'PID',
    dataIndex: 'pid',
    key: 'pid',
    width: 100,
  },
  {
    title: 'Command',
    dataIndex: 'comm',
    key: 'comm',
    width: 150,
  },
  {
    title: 'Event Type',
    dataIndex: 'type',
    key: 'type',
    width: 120,
  },
  {
    title: 'Path',
    dataIndex: 'path',
    key: 'path',
    ellipsis: true,
  },
];

const connectWebSocket = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  ws = new WebSocket(`${protocol}//${host}/ws`);

  ws.onopen = () => {
    isConnected.value = true;
    console.log('Connected to backend eBPF stream');
  };

  ws.onmessage = (message) => {
    try {
      const data = JSON.parse(message.data);
      const now = new Date();
      events.value.unshift({
        key: `${data.pid}-${data.path}-${Date.now()}-${Math.random()}`,
        pid: data.pid,
        type: data.type,
        comm: data.comm,
        path: data.path,
        time: now.toLocaleTimeString(),
      });
      
      // Keep only last 1000 events
      if (events.value.length > 1000) {
        events.value.pop();
      }
    } catch (e) {
      console.error('Failed to parse message', e);
    }
  };

  ws.onclose = () => {
    isConnected.value = false;
    console.log('Disconnected, retrying in 3s...');
    setTimeout(connectWebSocket, 3000);
  };
  
  ws.onerror = (error) => {
    console.error('WebSocket error:', error);
    ws?.close();
  };
};

const clearEvents = () => {
  events.value = [];
};

onMounted(() => {
  connectWebSocket();
});

onUnmounted(() => {
  if (ws) {
    ws.close();
  }
});
</script>

<template>
  <a-layout class="layout">
    <a-layout-header class="header">
      <div class="logo">
        <a-typography-title :level="3" style="color: white; margin: 0; line-height: 64px;">
          Agent eBPF Tracker
        </a-typography-title>
      </div>
    </a-layout-header>
    <a-layout-content style="padding: 0 50px; margin-top: 24px;">
      <div style="background: #fff; padding: 24px; min-height: 280px">
        <div style="display: flex; justify-content: space-between; margin-bottom: 16px;">
          <div>
            <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
            <span style="margin-left: 16px;">Total Events: {{ events.length }}</span>
          </div>
          <a-button type="primary" danger @click="clearEvents">Clear Events</a-button>
        </div>
        <a-table 
          :dataSource="events" 
          :columns="columns" 
          size="small"
          :pagination="{ pageSize: 20 }"
        />
      </div>
    </a-layout-content>
  </a-layout>
</template>

<style scoped>
.layout {
  min-height: 100vh;
}
.header {
  display: flex;
  align-items: center;
}
</style>
